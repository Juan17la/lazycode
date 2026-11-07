package editor

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Juan17la/lazycode/internal/buffer"
)

// Pairs: typing an opener closes it, surrounds a selection, and a closer steps over its twin.

var closerOf = map[byte]byte{'(': ')', '[': ']', '{': '}', '<': '>', '\'': '\'', '"': '"', '`': '`'}

var markupFiles = map[string]bool{".html": true, ".htm": true, ".xml": true, ".svg": true, ".vue": true, ".svelte": true, ".jsx": true, ".tsx": true, ".md": true}

func (e *Editor) byteAt(pos int) byte {
	if pos < 0 || pos >= e.File.Len() {
		return 0
	}
	return e.File.Slice(pos, pos+1)[0]
}

func (e *Editor) wordBefore(pos int) bool {
	r, _ := utf8.DecodeLastRune(e.File.Slice(max(0, pos-utf8.UTFMax), pos))
	return pos > 0 && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')
}

// shouldClose decides whether an opener typed at pos gets its closer. It stays out of the way in
// front of existing text, of an apostrophe in a word, and of a less-than in code.
func (e *Editor) shouldClose(open byte, pos int) bool {
	next := e.byteAt(pos)
	quote := open == closerOf[open]
	if next != 0 && !strings.ContainsRune(" \t\r\n", rune(next)) && (quote || !strings.ContainsRune(")]}>,;:.'\"`", rune(next))) {
		return false
	}
	switch {
	case quote:
		return !e.wordBefore(pos)
	case open == '<': // ponytail: by file extension and the character before, a parser could tell generics from comparisons.
		return markupFiles[strings.ToLower(filepath.Ext(e.File.Path))] || e.wordBefore(pos)
	}
	return true
}

func isCloser(ch byte) bool { return ch == ')' || ch == ']' || ch == '}' || ch == '>' }

// typePaired handles one typed character that's part of a pair, and reports whether it did.
func (e *Editor) typePaired(ch byte, join bool) (bool, error) {
	closer, opener := closerOf[ch]
	if !opener && !isCloser(ch) {
		return false, nil
	}
	err := e.editBack(join, func(_ int, c cursor) (buffer.Edit, int, int) {
		switch {
		case c.pos != c.anchor && opener:
			selected := string(e.File.Slice(c.start(), c.end()))
			return buffer.Edit{Start: c.start(), End: c.end(), Text: string(ch) + selected + string(closer)}, 1, len(selected)
		case c.pos != c.anchor:
			return buffer.Edit{Start: c.start(), End: c.end(), Text: string(ch)}, 0, 0
		case e.byteAt(c.pos) == ch && (isCloser(ch) || ch == closer):
			return buffer.Edit{Start: c.pos, End: c.pos + 1, Text: string(ch)}, 0, 0
		case opener && e.shouldClose(ch, c.pos):
			return buffer.Edit{Start: c.pos, End: c.pos, Text: string(ch) + string(closer)}, 1, 0
		}
		return buffer.Edit{Start: c.pos, End: c.pos, Text: string(ch)}, 0, 0
	})
	return true, err
}

// editBack is edit, with each cursor then placed back bytes before its change's end, and selected
// bytes selected behind that.
func (e *Editor) editBack(join bool, change func(i int, c cursor) (edit buffer.Edit, back, selected int)) error {
	backs, selections := make([]int, len(e.cursors)), make([]int, len(e.cursors))
	err := e.edit(join, func(i int, c cursor) buffer.Edit {
		edit, back, selected := change(i, c)
		backs[i], selections[i] = back, selected
		return edit
	})
	if err != nil || len(e.cursors) != len(backs) {
		return err
	}
	for i := range e.cursors {
		pos := e.cursors[i].pos - backs[i]
		e.cursors[i] = cursor{pos: pos, anchor: pos - selections[i], goal: -1}
	}
	e.edited()
	return nil
}

// Whole lines.

// lineBlock is lines first to last, all holding cursors, with no neighbouring block touching it.
type lineBlock struct{ first, last int }

func (e *Editor) cursorLines(c cursor) (first, last int) {
	first, last = e.File.LineOf(c.start()), e.File.LineOf(c.end())
	if last > first && e.File.LineStart(last) == c.end() {
		last-- // A selection ending at the start of a line doesn't include it.
	}
	return first, last
}

func (e *Editor) lineBlocks() (blocks []lineBlock, owner []int) {
	owner = make([]int, len(e.cursors))
	for i, c := range e.cursors {
		first, last := e.cursorLines(c)
		if n := len(blocks); n > 0 && first <= blocks[n-1].last+1 {
			blocks[n-1].last = max(blocks[n-1].last, last)
		} else {
			blocks = append(blocks, lineBlock{first, last})
		}
		owner[i] = len(blocks) - 1
	}
	return blocks, owner
}

// applyBlocks makes one edit per block, and moves each block's cursors by its shift.
func (e *Editor) applyBlocks(edits []buffer.Edit, shifts, owner []int) error {
	e.saveMark()
	if err := e.File.Apply(edits...); err != nil {
		return err
	}
	moved, delta := make([]int, len(edits)), 0
	for i, edit := range edits {
		moved[i] = delta + shifts[i]
		delta += len(edit.Text) - (edit.End - edit.Start)
	}
	for i, c := range e.cursors {
		by := moved[owner[i]]
		e.cursors[i] = cursor{pos: c.pos + by, anchor: c.anchor + by, goal: -1}
	}
	e.edited()
	return nil
}

// MoveLines moves the lines with cursors up or down one, with delta -1 or 1.
func (e *Editor) MoveLines(delta int) error {
	blocks, owner := e.lineBlocks()
	if blocks[0].first+delta < 0 || blocks[len(blocks)-1].last+delta >= e.File.LineCount() {
		return nil
	}
	edits, shifts := make([]buffer.Edit, len(blocks)), make([]int, len(blocks))
	for i, b := range blocks {
		start, _, _ := e.lineSpan(b.first)
		_, end, _ := e.lineSpan(b.last)
		block := string(e.File.Slice(start, end))
		if delta < 0 {
			before, beforeEnd, _ := e.lineSpan(b.first - 1)
			other, sep := string(e.File.Slice(before, beforeEnd)), string(e.File.Slice(beforeEnd, start))
			edits[i] = buffer.Edit{Start: before, End: end, Text: block + sep + other}
			shifts[i] = -len(other) - len(sep)
		} else {
			after, afterEnd, _ := e.lineSpan(b.last + 1)
			other, sep := string(e.File.Slice(after, afterEnd)), string(e.File.Slice(end, after))
			edits[i] = buffer.Edit{Start: start, End: afterEnd, Text: other + sep + block}
			shifts[i] = len(other) + len(sep)
		}
	}
	return e.applyBlocks(edits, shifts, owner)
}

// CopyLines duplicates the lines with cursors. The cursors go with the copy above or below.
func (e *Editor) CopyLines(delta int) error {
	blocks, owner := e.lineBlocks()
	edits, shifts := make([]buffer.Edit, len(blocks)), make([]int, len(blocks))
	for i, b := range blocks {
		start, _, _ := e.lineSpan(b.first)
		_, end, _ := e.lineSpan(b.last)
		text := e.lineEnding(end) + string(e.File.Slice(start, end))
		edits[i] = buffer.Edit{Start: end, End: end, Text: text}
		if delta > 0 {
			shifts[i] = len(text)
		}
	}
	return e.applyBlocks(edits, shifts, owner)
}

// DeleteLines removes every line with a cursor.
func (e *Editor) DeleteLines() error {
	return e.edit(false, func(_ int, c cursor) buffer.Edit {
		first, last := e.cursorLines(c)
		start, _, _ := e.lineSpan(first)
		_, _, next := e.lineSpan(last)
		if next == e.File.Len() && first > 0 && last == e.File.LineCount()-1 {
			_, start, _ = e.lineSpan(first - 1) // A last line takes the break before it instead.
		}
		return buffer.Edit{Start: start, End: next}
	})
}

// indentOf returns the blank start of a line.
func (e *Editor) indentOf(line int) string {
	start, end, _ := e.lineSpan(line)
	text := e.File.Slice(start, end)
	return string(text[:len(text)-len(bytes.TrimLeft(text, " \t"))])
}

// InsertLine starts an empty line below or above each cursor's line, keeping its indent.
func (e *Editor) InsertLine(below bool) error {
	return e.editBack(false, func(_ int, c cursor) (buffer.Edit, int, int) {
		line := e.File.LineOf(c.pos)
		start, end, _ := e.lineSpan(line)
		ending, indent := e.lineEnding(c.pos), e.indentOf(line)
		if below {
			return buffer.Edit{Start: end, End: end, Text: ending + indent}, 0, 0
		}
		return buffer.Edit{Start: start, End: start, Text: indent + ending}, len(ending), 0
	})
}

// SelectLines selects each cursor's whole line, and pressed again takes in the next one too.
func (e *Editor) SelectLines() {
	for i, c := range e.cursors {
		last := e.File.LineOf(c.end())
		_, _, next := e.lineSpan(last)
		e.cursors[i] = cursor{pos: next, anchor: e.File.LineStart(e.File.LineOf(c.start())), goal: -1}
	}
	e.tidy()
	e.last = editOther
}

// Cursors.

// AddCursorLine adds a cursor on the line above or below the outermost one, at the same column.
func (e *Editor) AddCursorLine(delta int) {
	edge := e.cursors[0]
	if delta > 0 {
		edge = e.cursors[len(e.cursors)-1]
	}
	target := e.File.LineOf(edge.pos) + delta
	if target < 0 || target >= e.File.LineCount() {
		return
	}
	goal := edge.goal
	if goal < 0 {
		goal = e.columnOf(e.File.LineStart(e.File.LineOf(edge.pos)), edge.pos)
	}
	for i := range e.cursors {
		if e.cursors[i] == edge {
			e.cursors[i].goal = goal
		}
	}
	pos := e.offsetAtColumn(target, goal)
	e.cursors = append(e.cursors, cursor{pos: pos, anchor: pos, goal: goal})
	e.primary = len(e.cursors) - 1
	e.tidy()
	e.last = editOther
}

// ToggleCursor adds a cursor at a place, as Alt+click does, or removes the one already there.
func (e *Editor) ToggleCursor(line, col int) {
	offset := e.offsetAtColumn(min(line, e.File.LineCount()-1), col)
	for i, c := range e.cursors {
		if c.pos == offset && c.anchor == offset && len(e.cursors) > 1 {
			e.cursors = append(e.cursors[:i], e.cursors[i+1:]...)
			e.primary = max(0, min(e.primary, len(e.cursors)-1))
			return
		}
	}
	e.cursors = append(e.cursors, cursor{pos: offset, anchor: offset, goal: -1})
	e.primary = len(e.cursors) - 1
	e.tidy()
	e.last = editOther
}

// Words.

// DeleteWordLeft removes each selection, or back to the start of the word.
func (e *Editor) DeleteWordLeft() error {
	if !e.hasSelection() {
		e.WordLeft(true)
	}
	return e.Backspace()
}

// DeleteWordRight removes each selection, or on to the end of the word.
func (e *Editor) DeleteWordRight() error {
	if !e.hasSelection() {
		e.WordRight(true)
	}
	return e.Delete()
}

// Comments.

// ErrNoLineComment says this kind of file has no line comment that LazyCode knows.
var ErrNoLineComment = errors.New("this kind of file has no line comment")

var lineComments = map[string]string{
	".go": "//", ".js": "//", ".jsx": "//", ".ts": "//", ".tsx": "//", ".c": "//", ".h": "//", ".cpp": "//", ".hpp": "//", ".cc": "//",
	".cs": "//", ".java": "//", ".kt": "//", ".rs": "//", ".swift": "//", ".php": "//", ".dart": "//", ".scss": "//", ".zig": "//",
	".py": "#", ".rb": "#", ".sh": "#", ".bash": "#", ".zsh": "#", ".fish": "#", ".toml": "#", ".yaml": "#", ".yml": "#", ".ps1": "#", ".r": "#", ".pl": "#",
	".lua": "--", ".sql": "--", ".hs": "--",
}

// ToggleComment comments the lines with cursors, or uncomments them if all of them are comments.
func (e *Editor) ToggleComment() error {
	prefix, ok := lineComments[strings.ToLower(filepath.Ext(e.File.Path))]
	if !ok {
		return ErrNoLineComment
	}
	uncomment, indent := true, -1
	// Without an edit this only looks, so the choice covers all the lines.
	if err := e.editLines(func(line int) (buffer.Edit, bool) {
		start, end, _ := e.lineSpan(line)
		text := e.File.Slice(start, end)
		rest := bytes.TrimLeft(text, " \t")
		if len(rest) > 0 {
			uncomment = uncomment && bytes.HasPrefix(rest, []byte(prefix))
			if width := len(text) - len(rest); indent < 0 || width < indent {
				indent = width
			}
		}
		return buffer.Edit{}, false
	}); err != nil {
		return err
	}
	if indent < 0 {
		return nil // Only blank lines.
	}
	return e.editLines(func(line int) (buffer.Edit, bool) {
		start, end, _ := e.lineSpan(line)
		text := e.File.Slice(start, end)
		rest := bytes.TrimLeft(text, " \t")
		if len(rest) == 0 {
			return buffer.Edit{}, false
		}
		if !uncomment {
			return buffer.Edit{Start: start + indent, End: start + indent, Text: prefix + " "}, true
		}
		at := start + len(text) - len(rest)
		size := len(prefix)
		if bytes.HasPrefix(rest[size:], []byte(" ")) {
			size++
		}
		return buffer.Edit{Start: at, End: at + size}, true
	})
}
