package editor

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Juan17la/lazycode/internal/buffer"
	"github.com/rivo/uniseg"
)

// indentScanLines is how much of a file is checked for tabs or spaces.
const indentScanLines = 1000

type editKind int

const (
	editOther editKind = iota
	editTyping
	editErasing
)

// cursor is a place in the text, with a selection running from anchor to pos.
type cursor struct {
	pos, anchor int
	goal        int // Column that up and down keep to, or -1 to use pos's.
}

func (c cursor) start() int { return min(c.pos, c.anchor) }
func (c cursor) end() int   { return max(c.pos, c.anchor) }

type cachedPosition struct {
	valid     bool
	cursor    int
	tabSize   int
	text      buffer.Snapshot
	line, col int
}

// mark is what an undo step remembers beside the text.
type mark struct {
	cursors []cursor
	primary int
}

// Editor is one view of a file: its cursors and a scroll position. All offsets are
// bytes. The UI goroutine owns it. Several editors can share a File, as in a split.
type Editor struct {
	File       *buffer.File
	TabSize    int
	ScrollLine int    // First line on screen.
	ScrollCol  int    // First column on screen.
	Highlight  string // Text to mark wherever it's on screen, for find.

	cursors      []cursor // In order, and never overlapping.
	primary      int      // The one scrolling follows, and the one most recently added.
	height       int      // Lines on screen, for page up and down.
	seen         buffer.Snapshot
	seenReloads  int
	last         editKind
	lastWasSpace bool
	position     cachedPosition
	followed     struct { // What Scroll last kept on screen.
		valid bool
		pos   int
		text  buffer.Snapshot
	}
}

// New returns an editor at the start of f.
func New(f *buffer.File, tabSize int) *Editor {
	return &Editor{File: f, TabSize: tabSize, height: 1, cursors: []cursor{{goal: -1}}, seen: f.Snapshot, seenReloads: f.Reloads()}
}

func (e *Editor) cur() cursor { return e.cursors[e.primary] }

// Selection returns the main cursor's selected range in order. start == end means none.
func (e *Editor) Selection() (start, end int) {
	c := e.cur()
	return c.start(), c.end()
}

// CursorCount returns how many cursors there are.
func (e *Editor) CursorCount() int { return len(e.cursors) }

// Position returns the main cursor's line and display column, counting from 0.
func (e *Editor) Position() (line, col int) {
	pos := e.cur().pos
	// A frame asks several times, and on a very long line each answer is slow to work out.
	p := &e.position
	if !p.valid || p.cursor != pos || p.tabSize != e.TabSize || !p.text.Same(e.File.Snapshot) {
		line = e.File.LineOf(pos)
		col = e.columnOf(e.File.LineStart(line), pos)
		*p = cachedPosition{true, pos, e.TabSize, e.File.Snapshot, line, col}
	}
	return e.position.line, e.position.col
}

// lineSpan returns where a line starts, where its text ends, and where the next line
// starts. Its line break sits between the last two.
func (e *Editor) lineSpan(line int) (start, end, next int) {
	f := e.File
	start = f.LineStart(line)
	if line+1 >= f.LineCount() {
		return start, f.Len(), f.Len()
	}
	next = f.LineStart(line + 1)
	end = next - 1
	if end > start && f.Slice(end-1, end)[0] == '\r' {
		end--
	}
	return start, end, next
}

// tidy puts the cursors in order and joins any that overlap, or where one with nothing
// selected touches another. The main cursor stays the main cursor, whatever it joins.
func (e *Editor) tidy() {
	main := e.cursors[e.primary]
	slices.SortStableFunc(e.cursors, func(a, b cursor) int { return a.start() - b.start() })
	merged := e.cursors[:0]
	mainIndex := 0
	for _, c := range e.cursors {
		if n := len(merged); n > 0 {
			last := merged[n-1]
			touching := c.start() == last.end() && (c.pos == c.anchor || last.pos == last.anchor)
			if c.start() < last.end() || touching {
				start, end := last.start(), max(last.end(), c.end())
				joined := cursor{pos: end, anchor: start, goal: -1}
				if c.pos < c.anchor {
					joined.pos, joined.anchor = start, end
				}
				merged[n-1] = joined
				if c == main || last == main {
					mainIndex = n - 1
				}
				continue
			}
		}
		if c == main {
			mainIndex = len(merged)
		}
		merged = append(merged, c)
	}
	e.cursors = merged
	e.primary = mainIndex
}

func (e *Editor) clamp(offset int) int { return max(0, min(offset, e.File.Len())) }

// Movement.

// move sends every cursor where to says. Without extend, selections end.
func (e *Editor) move(extend bool, to func(c cursor) int) {
	for i, c := range e.cursors {
		pos := e.clamp(to(c))
		e.cursors[i].pos, e.cursors[i].goal = pos, -1
		if !extend {
			e.cursors[i].anchor = pos
		}
	}
	e.tidy()
	e.last = editOther
}

// moveTo leaves just the main cursor, at offset.
func (e *Editor) moveTo(offset int, extend bool) {
	c := e.cur()
	c.pos, c.goal = e.clamp(offset), -1
	if !extend {
		c.anchor = c.pos
	}
	e.cursors, e.primary = []cursor{c}, 0
	e.last = editOther
}

// Left moves back one character, which can be several code points. Without extend, a
// selection collapses to its start instead.
func (e *Editor) Left(extend bool) {
	e.move(extend, func(c cursor) int {
		if c.pos != c.anchor && !extend {
			return c.start()
		}
		return e.previous(c.pos)
	})
}

// Right moves forward one character. Without extend, a selection collapses to its end.
func (e *Editor) Right(extend bool) {
	e.move(extend, func(c cursor) int {
		if c.pos != c.anchor && !extend {
			return c.end()
		}
		return e.next(c.pos)
	})
}

func (e *Editor) previous(pos int) int {
	line := e.File.LineOf(pos)
	start := e.File.LineStart(line)
	if pos == start {
		if line == 0 {
			return 0
		}
		_, end, _ := e.lineSpan(line - 1)
		return end
	}
	// ponytail: only looks back 4 KB, so a longer single character splits. Fine: none
	// exist outside of deliberately broken text.
	from := max(start, pos-4096)
	text := e.File.Slice(from, pos)
	state, at := -1, 0
	for {
		var cluster []byte
		cluster, text, _, state = uniseg.FirstGraphemeCluster(text, state)
		if len(text) == 0 {
			return from + at
		}
		at += len(cluster)
	}
}

func (e *Editor) next(pos int) int {
	_, end, next := e.lineSpan(e.File.LineOf(pos))
	if pos >= end {
		return next
	}
	cluster, _, _, _ := uniseg.FirstGraphemeCluster(e.File.Slice(pos, min(end, pos+4096)), -1)
	return pos + len(cluster)
}

// Up moves up a line, keeping to the same column where the line is long enough.
func (e *Editor) Up(extend bool) { e.moveLines(-1, extend) }

// Down moves down a line.
func (e *Editor) Down(extend bool) { e.moveLines(1, extend) }

// PageUp moves up a screen.
func (e *Editor) PageUp(extend bool) {
	e.moveLines(-e.height, extend)
	e.ScrollLine = max(0, e.ScrollLine-e.height)
}

// PageDown moves down a screen.
func (e *Editor) PageDown(extend bool) {
	e.moveLines(e.height, extend)
	e.ScrollLine = min(e.File.LineCount()-1, e.ScrollLine+e.height)
}

func (e *Editor) moveLines(delta int, extend bool) {
	for i, c := range e.cursors {
		line := e.File.LineOf(c.pos)
		goal := c.goal
		if goal < 0 {
			goal = e.columnOf(e.File.LineStart(line), c.pos)
		}
		pos := 0
		switch target := line + delta; {
		case target < 0:
		case target >= e.File.LineCount():
			pos = e.File.Len()
		default:
			pos = e.offsetAtColumn(target, goal)
		}
		e.cursors[i].pos, e.cursors[i].goal = pos, goal
		if !extend {
			e.cursors[i].anchor = pos
		}
	}
	e.tidy()
	e.last = editOther
}

// Home moves to the first non-blank character, or from there to the start of the line.
func (e *Editor) Home(extend bool) {
	e.move(extend, func(c cursor) int {
		start, end, _ := e.lineSpan(e.File.LineOf(c.pos))
		text := e.File.Slice(start, end)
		first := start + len(text) - len(bytes.TrimLeft(text, " \t"))
		if c.pos == first {
			return start
		}
		return first
	})
}

// End moves to the end of the line.
func (e *Editor) End(extend bool) {
	e.move(extend, func(c cursor) int {
		_, end, _ := e.lineSpan(e.File.LineOf(c.pos))
		return end
	})
}

// DocStart moves to the start of the file.
func (e *Editor) DocStart(extend bool) { e.move(extend, func(cursor) int { return 0 }) }

// DocEnd moves to the end of the file.
func (e *Editor) DocEnd(extend bool) { e.move(extend, func(cursor) int { return e.File.Len() }) }

// SelectAll selects the whole file.
func (e *Editor) SelectAll() {
	e.moveTo(e.File.Len(), false)
	e.cursors[0].anchor = 0
}

// ClearSelection leaves just the main cursor where it is, with nothing selected.
func (e *Editor) ClearSelection() { e.moveTo(e.cur().pos, false) }

// GoTo puts a single cursor at a line and display column, or extends the selection
// there. It's what a mouse click does.
func (e *Editor) GoTo(line, col int, extend bool) {
	e.moveTo(e.offsetAtColumn(max(0, line), max(0, col)), extend)
}

func wordClass(r rune) int {
	switch {
	case unicode.IsSpace(r):
		return 0
	case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r):
		return 1
	}
	return 2
}

// WordLeft moves to the start of the previous word.
func (e *Editor) WordLeft(extend bool) {
	e.move(extend, func(c cursor) int {
		start := e.File.LineStart(e.File.LineOf(c.pos))
		if c.pos == start {
			return e.previous(c.pos)
		}
		text := e.File.Slice(start, c.pos)
		r, size := utf8.DecodeLastRune(text)
		for wordClass(r) == 0 && len(text) > 0 {
			text = text[:len(text)-size]
			r, size = utf8.DecodeLastRune(text)
		}
		class := wordClass(r)
		for len(text) > 0 && wordClass(r) == class {
			text = text[:len(text)-size]
			r, size = utf8.DecodeLastRune(text)
		}
		return start + len(text)
	})
}

// WordRight moves to the end of the next word.
func (e *Editor) WordRight(extend bool) {
	e.move(extend, func(c cursor) int {
		_, end, next := e.lineSpan(e.File.LineOf(c.pos))
		if c.pos >= end {
			return next
		}
		text := e.File.Slice(c.pos, end)
		at := 0
		r, size := utf8.DecodeRune(text)
		for at < len(text) && wordClass(r) == 0 {
			at += size
			r, size = utf8.DecodeRune(text[at:])
		}
		class := wordClass(r)
		for at < len(text) && wordClass(r) == class {
			at += size
			r, size = utf8.DecodeRune(text[at:])
		}
		return c.pos + at
	})
}

// Editing. Every method returns buffer.ErrReadOnly for a read-only file.

func (e *Editor) saveMark() { e.File.Mark = mark{slices.Clone(e.cursors), e.primary} }

// edited records that this editor's cursors now fit the text.
func (e *Editor) edited() {
	e.tidy()
	e.saveMark()
	e.seen = e.File.Snapshot
	e.last = editOther
}

// edit makes one change per cursor as a single undo step, and leaves each cursor after
// its change. Changes that overlap are joined.
func (e *Editor) edit(join bool, change func(i int, c cursor) buffer.Edit) error {
	edits := make([]buffer.Edit, 0, len(e.cursors))
	owner := make([]int, len(e.cursors)) // Which edit each cursor ended up in.
	for i, c := range e.cursors {
		edit := change(i, c)
		if n := len(edits); n > 0 && edit.Start < edits[n-1].End {
			edits[n-1].Start = min(edits[n-1].Start, edit.Start)
			edits[n-1].End = max(edits[n-1].End, edit.End)
			edits[n-1].Text += edit.Text
		} else {
			edits = append(edits, edit)
		}
		owner[i] = len(edits) - 1
	}

	e.saveMark()
	apply := e.File.Apply
	if join {
		apply = e.File.Continue
	}
	if err := apply(edits...); err != nil {
		return err
	}

	// Where each edit's text now ends, after the size changes of the ones before it.
	ends := make([]int, len(edits))
	delta := 0
	for i, edit := range edits {
		ends[i] = edit.Start + delta + len(edit.Text)
		delta += len(edit.Text) - (edit.End - edit.Start)
	}
	for i := range e.cursors {
		pos := ends[owner[i]]
		e.cursors[i] = cursor{pos: pos, anchor: pos, goal: -1}
	}
	e.edited()
	return nil
}

// ReplaceRange changes bytes start to end as one undo step, for changes that don't come
// from a cursor. Cursors after it move with their text, and those inside go to its end.
func (e *Editor) ReplaceRange(start, end int, text string) error {
	if start < 0 || end < start || end > e.File.Len() {
		return fmt.Errorf("%d to %d is outside the file, which has %d bytes", start, end, e.File.Len())
	}
	e.saveMark()
	if err := e.File.Apply(buffer.Edit{Start: start, End: end, Text: text}); err != nil {
		return err
	}
	shift := func(pos int) int {
		switch {
		case pos >= end && pos > start:
			return pos + len(text) - (end - start)
		case pos > start:
			return start + len(text)
		}
		return pos
	}
	for i, c := range e.cursors {
		e.cursors[i] = cursor{pos: shift(c.pos), anchor: shift(c.anchor), goal: -1}
	}
	e.edited()
	return nil
}

// replaceSelections puts text over every cursor's selection.
func (e *Editor) replaceSelections(join bool, text func(i int, c cursor) string) error {
	return e.edit(join, func(i int, c cursor) buffer.Edit {
		return buffer.Edit{Start: c.start(), End: c.end(), Text: text(i, c)}
	})
}

func (e *Editor) hasSelection() bool {
	return slices.ContainsFunc(e.cursors, func(c cursor) bool { return c.pos != c.anchor })
}

// Type inserts typed text over any selections. Undo takes back a word at a time.
func (e *Editor) Type(text string) error {
	space := strings.TrimSpace(text) == ""
	join := e.last == editTyping && !e.hasSelection() && (space || !e.lastWasSpace)
	if err := e.replaceSelections(join, func(int, cursor) string { return text }); err != nil {
		return err
	}
	e.last, e.lastWasSpace = editTyping, space
	return nil
}

// Newline starts a new line with this line's indent and line ending.
func (e *Editor) Newline() error {
	return e.replaceSelections(false, func(_ int, c cursor) string {
		lineStart, lineEnd, _ := e.lineSpan(e.File.LineOf(c.start()))
		text := e.File.Slice(lineStart, min(c.start(), lineEnd))
		indent := text[:len(text)-len(bytes.TrimLeft(text, " \t"))]
		return e.lineEnding(c.pos) + string(indent)
	})
}

// lineEnding returns the ending of pos's line, or the previous one's for a last line
// without one, so mixed files keep each part's own style.
func (e *Editor) lineEnding(pos int) string {
	line := e.File.LineOf(pos)
	if line+1 >= e.File.LineCount() {
		line--
	}
	if line < 0 {
		return "\n"
	}
	_, end, next := e.lineSpan(line)
	return string(e.File.Slice(end, next))
}

// Paste inserts text over any selections, with line breaks converted to this file's.
// With several cursors and one line for each, each cursor gets its own line.
func (e *Editor) Paste(text string) error {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var perCursor []string
	if len(e.cursors) > 1 {
		lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
		if len(lines) == len(e.cursors) {
			perCursor = lines
		}
	}
	return e.replaceSelections(false, func(i int, c cursor) string {
		piece := text
		if perCursor != nil {
			piece = perCursor[i]
		}
		if ending := e.lineEnding(c.pos); ending != "\n" {
			piece = strings.ReplaceAll(piece, "\n", ending)
		}
		return piece
	})
}

// Backspace removes each selection, or the character before each cursor.
func (e *Editor) Backspace() error {
	return e.erase(func(c cursor) (int, int) { return e.previous(c.pos), c.pos })
}

// Delete removes each selection, or the character after each cursor.
func (e *Editor) Delete() error {
	return e.erase(func(c cursor) (int, int) { return c.pos, e.next(c.pos) })
}

func (e *Editor) erase(span func(c cursor) (start, end int)) error {
	selected := e.hasSelection()
	join := e.last == editErasing && !selected
	err := e.edit(join, func(_ int, c cursor) buffer.Edit {
		if selected {
			return buffer.Edit{Start: c.start(), End: c.end()}
		}
		start, end := span(c)
		return buffer.Edit{Start: start, End: end}
	})
	if err == nil && !selected {
		e.last = editErasing
	}
	return err
}

// Copy returns what's selected, joined by line breaks. With nothing selected, it takes
// each cursor's whole line, as VS Code does.
func (e *Editor) Copy() string {
	selected := e.hasSelection()
	var pieces []string
	for _, c := range e.cursors {
		start, end := e.copyRange(c, selected)
		pieces = append(pieces, string(e.File.Slice(start, end)))
	}
	if selected {
		return strings.Join(pieces, e.lineEnding(e.cur().pos))
	}
	return strings.Join(pieces, "")
}

// Cut removes and returns what Copy would return.
func (e *Editor) Cut() (string, error) {
	text := e.Copy()
	selected := e.hasSelection()
	err := e.edit(false, func(_ int, c cursor) buffer.Edit {
		start, end := e.copyRange(c, selected)
		return buffer.Edit{Start: start, End: end}
	})
	return text, err
}

func (e *Editor) copyRange(c cursor, selected bool) (start, end int) {
	if selected {
		return c.start(), c.end()
	}
	start, _, next := e.lineSpan(e.File.LineOf(c.pos))
	return start, next
}

// indentUnit returns one level of indent for inserting at a column, following what the
// file already uses.
func (e *Editor) indentUnit(col int) string {
	f := e.File
	for line := range min(f.LineCount(), indentScanLines) {
		start := f.LineStart(line)
		if first := f.Slice(start, start+1); len(first) == 1 && first[0] == '\t' {
			return "\t"
		} else if len(first) == 1 && first[0] == ' ' {
			break
		}
	}
	return strings.Repeat(" ", e.TabSize-col%e.TabSize)
}

// Tab indents the selected lines, or inserts one indent at each cursor.
func (e *Editor) Tab() error {
	if !e.hasSelection() {
		join := e.last == editTyping
		err := e.replaceSelections(join, func(_ int, c cursor) string {
			return e.indentUnit(e.columnOf(e.File.LineStart(e.File.LineOf(c.pos)), c.pos))
		})
		if err == nil {
			e.last, e.lastWasSpace = editTyping, true
		}
		return err
	}
	unit := e.indentUnit(0)
	return e.editLines(func(line int) (buffer.Edit, bool) {
		at := e.File.LineStart(line)
		return buffer.Edit{Start: at, End: at, Text: unit}, true
	})
}

// Outdent removes one indent from the selected lines, or each cursor's line.
func (e *Editor) Outdent() error {
	return e.editLines(func(line int) (buffer.Edit, bool) {
		at, end, _ := e.lineSpan(line)
		text := e.File.Slice(at, min(end, at+e.TabSize))
		remove := len(text) - len(bytes.TrimLeft(text, " "))
		if len(text) > 0 && text[0] == '\t' {
			remove = 1
		}
		return buffer.Edit{Start: at, End: at + remove}, remove > 0
	})
}

// editLines changes every line a cursor is on as one step, keeping the cursors with
// their text.
func (e *Editor) editLines(change func(line int) (buffer.Edit, bool)) error {
	var edits []buffer.Edit
	nextLine := 0
	for _, c := range e.cursors {
		first, last := e.File.LineOf(c.start()), e.File.LineOf(c.end())
		if last > first && e.File.LineStart(last) == c.end() {
			last-- // A selection ending at the start of a line doesn't include it.
		}
		for line := max(first, nextLine); line <= last; line++ {
			if edit, ok := change(line); ok {
				edits = append(edits, edit)
			}
		}
		nextLine = max(nextLine, last+1)
	}
	if len(edits) == 0 {
		return nil
	}
	e.saveMark()
	if err := e.File.Apply(edits...); err != nil {
		return err
	}
	for i, c := range e.cursors {
		e.cursors[i] = cursor{pos: mapOffset(c.pos, edits), anchor: mapOffset(c.anchor, edits), goal: -1}
	}
	e.edited()
	return nil
}

// mapOffset returns where offset ends up after edits, which are in order.
func mapOffset(offset int, edits []buffer.Edit) int {
	moved := offset
	for _, edit := range edits {
		switch {
		case edit.Start > offset:
			return moved
		case edit.End <= offset:
			moved += len(edit.Text) - (edit.End - edit.Start)
		default: // Inside what was replaced.
			return moved - (offset - edit.Start)
		}
	}
	return moved
}

// SetText makes text the whole content as one undo step, and goes to the start.
func (e *Editor) SetText(text string) error {
	e.SelectAll()
	if err := e.replaceSelections(false, func(int, cursor) string { return text }); err != nil {
		e.moveTo(0, false)
		return err
	}
	e.moveTo(0, false)
	e.saveMark()
	return nil
}

// Undo reverses the last step, and reports whether there was one.
func (e *Editor) Undo() bool { return e.restore(e.File.Undo()) }

// Redo restores the last undone step, and reports whether there was one.
func (e *Editor) Redo() bool { return e.restore(e.File.Redo()) }

func (e *Editor) restore(changed bool) bool {
	if !changed {
		return false
	}
	if m, ok := e.File.Mark.(mark); ok {
		e.cursors, e.primary = slices.Clone(m.cursors), m.primary
	}
	for i, c := range e.cursors {
		e.cursors[i].pos, e.cursors[i].anchor = e.clamp(c.pos), e.clamp(c.anchor)
	}
	e.tidy()
	e.last = editOther
	e.seen = e.File.Snapshot
	return true
}

// Reload replaces the text with the file on disk, keeping the cursor on the same line
// and column where those still exist.
func (e *Editor) Reload() error {
	// Worked out first, because the old text may not be readable afterwards.
	places := make([][4]int, len(e.cursors))
	for i, c := range e.cursors {
		places[i] = [4]int{e.File.LineOf(c.pos), e.columnOf(e.File.LineStart(e.File.LineOf(c.pos)), c.pos),
			e.File.LineOf(c.anchor), e.columnOf(e.File.LineStart(e.File.LineOf(c.anchor)), c.anchor)}
	}
	if err := e.File.Reload(); err != nil {
		return err
	}
	for i, p := range places {
		e.cursors[i] = cursor{pos: e.offsetAtColumn(p[0], p[1]), anchor: e.offsetAtColumn(p[2], p[3]), goal: -1}
	}
	e.tidy()
	e.seen, e.seenReloads = e.File.Snapshot, e.File.Reloads()
	e.last = editOther
	return nil
}

// Sync moves the cursors to the same lines and columns if the text changed without this
// editor knowing, as when another view of the file edits it or the file reloads.
//
// ponytail: an edit above the cursor in the other view leaves this one on the same line
// number, not the same text. Mapping through each edit is the upgrade if that annoys.
func (e *Editor) Sync() {
	if e.seen.Same(e.File.Snapshot) {
		return
	}
	reloaded := e.seenReloads != e.File.Reloads()
	place := func(offset int) int {
		if reloaded {
			// Another view reloaded the file. The old text may be an unmapped file, so
			// reading it would crash. The start of the same offset's line is close enough.
			return e.File.LineStart(e.File.LineOf(offset))
		}
		line := e.seen.LineOf(offset)
		col := 0
		walk(e.seen.Slice(e.seen.LineStart(line), offset), 0, e.TabSize, func(g glyph) bool {
			col = g.col + g.width
			return true
		})
		return e.offsetAtColumn(line, col)
	}
	for i, c := range e.cursors {
		e.cursors[i] = cursor{pos: place(c.pos), anchor: place(c.anchor), goal: -1}
	}
	e.tidy()
	e.seen, e.seenReloads = e.File.Snapshot, e.File.Reloads()
	e.last = editOther
}

// Occurrences.

// wordAt returns the word at or just before pos.
func (e *Editor) wordAt(pos int) (start, end int) {
	lineStart, lineEnd, _ := e.lineSpan(e.File.LineOf(pos))
	before := e.File.Slice(lineStart, pos)
	after := e.File.Slice(pos, lineEnd)
	start, end = pos, pos
	for len(before) > 0 {
		r, size := utf8.DecodeLastRune(before)
		if wordClass(r) != 1 {
			break
		}
		before = before[:len(before)-size]
		start -= size
	}
	for len(after) > 0 {
		r, size := utf8.DecodeRune(after)
		if wordClass(r) != 1 {
			break
		}
		after = after[size:]
		end += size
	}
	return start, end
}

// needle returns the main cursor's selection, first selecting the word under it if
// nothing is selected.
func (e *Editor) needle() []byte {
	c := e.cur()
	if c.pos == c.anchor {
		start, end := e.wordAt(c.pos)
		e.cursors[e.primary].anchor, e.cursors[e.primary].pos = start, end
		e.tidy() // The word may run into another cursor.
		c = e.cur()
	}
	e.last = editOther
	return e.File.Slice(c.start(), c.end())
}

// AddNextOccurrence is Ctrl+D (Q-17). With nothing selected it selects the word under the
// cursor. Otherwise it adds a cursor on the next match of the selection, going round the
// end of the file. It reports false if there was nothing to do.
func (e *Editor) AddNextOccurrence() bool {
	wasEmpty := e.cur().pos == e.cur().anchor
	needle := e.needle()
	if len(needle) == 0 {
		return false
	}
	if wasEmpty {
		return true
	}
	from := e.cur().end()
	for range 2 { // After the main cursor, then from the top.
		for at := e.File.Index(needle, from); at >= 0; at = e.File.Index(needle, at+1) {
			if slices.ContainsFunc(e.cursors, func(c cursor) bool { return c.start() < at+len(needle) && at < c.end() }) {
				continue
			}
			e.cursors = append(e.cursors, cursor{pos: at + len(needle), anchor: at, goal: -1})
			e.primary = len(e.cursors) - 1
			e.tidy()
			return true
		}
		from = 0
	}
	return false
}

// SelectAllOccurrences is Ctrl+F2 (Q-17). It puts a cursor on every match of the selection
// or the word under the cursor, and returns how many there are.
func (e *Editor) SelectAllOccurrences() int {
	needle := e.needle()
	if len(needle) == 0 {
		return 0
	}
	mainStart := e.cur().start()
	var cursors []cursor
	primary := 0
	for _, at := range e.findAll(needle) {
		if at <= mainStart {
			primary = len(cursors)
		}
		cursors = append(cursors, cursor{pos: at + len(needle), anchor: at, goal: -1})
	}
	e.cursors, e.primary = cursors, primary
	return len(cursors)
}

func (e *Editor) findAll(needle []byte) []int {
	var found []int
	for at := e.File.Index(needle, 0); at >= 0; at = e.File.Index(needle, at+len(needle)) {
		found = append(found, at)
	}
	return found
}

// Find and replace (Q-17): plain text in this file, matching case exactly.

// FindNext selects the next match after the cursor, going round the end of the file.
func (e *Editor) FindNext(query string) bool {
	needle := []byte(query)
	_, end := e.Selection()
	at := e.File.Index(needle, end)
	if at < 0 {
		at = e.File.Index(needle, 0)
	}
	return e.selectMatch(at, len(needle))
}

// FindPrevious selects the previous match before the cursor, going round the start.
func (e *Editor) FindPrevious(query string) bool {
	needle := []byte(query)
	start, _ := e.Selection()
	at := e.File.LastIndex(needle, start)
	if at < 0 {
		at = e.File.LastIndex(needle, e.File.Len())
	}
	return e.selectMatch(at, len(needle))
}

func (e *Editor) selectMatch(at, length int) bool {
	if at < 0 {
		return false
	}
	e.moveTo(at+length, false)
	e.cursors[0].anchor = at
	return true
}

// FindFrom selects the first match at or after from, going round the end of the file. With
// no match it goes back to from, as a find that's being typed should.
func (e *Editor) FindFrom(query string, from int) bool {
	needle := []byte(query)
	at := e.File.Index(needle, from)
	if at < 0 {
		at = e.File.Index(needle, 0)
	}
	if at < 0 {
		e.moveTo(from, false)
		return false
	}
	return e.selectMatch(at, len(needle))
}

// CountMatches returns how many matches there are, stopping at limit.
func (e *Editor) CountMatches(query string, limit int) int {
	needle := []byte(query)
	count := 0
	for at := e.File.Index(needle, 0); at >= 0 && count < limit; at = e.File.Index(needle, at+len(needle)) {
		count++
	}
	return count
}

// ReplaceNext replaces the selection if it's a match, then selects the next match.
func (e *Editor) ReplaceNext(query, replacement string) error {
	start, end := e.Selection()
	if len(e.cursors) == 1 && query != "" && string(e.File.Slice(start, end)) == query {
		if err := e.replaceSelections(false, func(int, cursor) string { return replacement }); err != nil {
			return err
		}
	}
	e.FindNext(query)
	return nil
}

// ReplaceAll replaces every match as one undo step, and returns how many there were.
func (e *Editor) ReplaceAll(query, replacement string) (int, error) {
	found := e.findAll([]byte(query))
	if len(found) == 0 {
		return 0, nil
	}
	edits := make([]buffer.Edit, len(found))
	for i, at := range found {
		edits[i] = buffer.Edit{Start: at, End: at + len(query), Text: replacement}
	}
	e.saveMark()
	if err := e.File.Apply(edits...); err != nil {
		return 0, err
	}
	pos := mapOffset(e.cur().pos, edits)
	e.cursors, e.primary = []cursor{{pos: pos, anchor: pos, goal: -1}}, 0
	e.edited()
	return len(found), nil
}
