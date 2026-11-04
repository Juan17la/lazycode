package editor

import (
	"bytes"
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

type cachedPosition struct {
	valid     bool
	cursor    int
	text      buffer.Snapshot
	line, col int
}

// mark is what an undo step remembers beside the text.
type mark struct{ cursor, anchor int }

// Editor is one view of a file: a cursor, a selection and a scroll position. All
// offsets are bytes. The UI goroutine owns it.
type Editor struct {
	File       *buffer.File
	TabSize    int
	ScrollLine int // First line on screen.
	ScrollCol  int // First column on screen.

	cursor, anchor int // The selection runs between them. Equal means none.
	goal           int // Column that up and down keep to, or -1 to use the cursor's.
	height         int // Lines on screen, for page up and down.
	last           editKind
	lastWasSpace   bool
	position       cachedPosition
}

// New returns an editor at the start of f.
func New(f *buffer.File, tabSize int) *Editor {
	return &Editor{File: f, TabSize: tabSize, goal: -1, height: 1}
}

// Selection returns the selected range in order. start == end means none.
func (e *Editor) Selection() (start, end int) {
	return min(e.cursor, e.anchor), max(e.cursor, e.anchor)
}

// Position returns the cursor's line and display column, counting from 0.
func (e *Editor) Position() (line, col int) {
	// A frame asks several times, and on a very long line each answer is slow to work out.
	if !e.position.valid || e.position.cursor != e.cursor || !e.position.text.Same(e.File.Snapshot) {
		line = e.File.LineOf(e.cursor)
		col = e.columnOf(e.File.LineStart(line), e.cursor)
		e.position = cachedPosition{true, e.cursor, e.File.Snapshot, line, col}
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

// Movement.

func (e *Editor) moveTo(offset int, extend bool) {
	e.cursor = max(0, min(offset, e.File.Len()))
	if !extend {
		e.anchor = e.cursor
	}
	e.goal = -1
	e.last = editOther
}

// Left moves back one character, which can be several code points. Without extend, a
// selection collapses to its start instead.
func (e *Editor) Left(extend bool) {
	if start, end := e.Selection(); start != end && !extend {
		e.moveTo(start, false)
		return
	}
	e.moveTo(e.previous(), extend)
}

// Right moves forward one character. Without extend, a selection collapses to its end.
func (e *Editor) Right(extend bool) {
	if start, end := e.Selection(); start != end && !extend {
		e.moveTo(end, false)
		return
	}
	e.moveTo(e.next(), extend)
}

func (e *Editor) previous() int {
	line := e.File.LineOf(e.cursor)
	start := e.File.LineStart(line)
	if e.cursor == start {
		if line == 0 {
			return 0
		}
		_, end, _ := e.lineSpan(line - 1)
		return end
	}
	// ponytail: only looks back 4 KB, so a longer single character splits. Fine: none
	// exist outside of deliberately broken text.
	from := max(start, e.cursor-4096)
	text := e.File.Slice(from, e.cursor)
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

func (e *Editor) next() int {
	line := e.File.LineOf(e.cursor)
	_, end, next := e.lineSpan(line)
	if e.cursor >= end {
		return next
	}
	cluster, _, _, _ := uniseg.FirstGraphemeCluster(e.File.Slice(e.cursor, min(end, e.cursor+4096)), -1)
	return e.cursor + len(cluster)
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
	line, col := e.Position()
	goal := e.goal
	if goal < 0 {
		goal = col
	}
	target := line + delta
	switch {
	case target < 0:
		e.moveTo(0, extend)
	case target >= e.File.LineCount():
		e.moveTo(e.File.Len(), extend)
	default:
		e.moveTo(e.offsetAtColumn(target, goal), extend)
	}
	e.goal = goal
}

// Home moves to the first non-blank character, or from there to the start of the line.
func (e *Editor) Home(extend bool) {
	start, end, _ := e.lineSpan(e.File.LineOf(e.cursor))
	text := e.File.Slice(start, end)
	first := start + len(text) - len(bytes.TrimLeft(text, " \t"))
	if e.cursor == first {
		first = start
	}
	e.moveTo(first, extend)
}

// End moves to the end of the line.
func (e *Editor) End(extend bool) {
	_, end, _ := e.lineSpan(e.File.LineOf(e.cursor))
	e.moveTo(end, extend)
}

// DocStart moves to the start of the file.
func (e *Editor) DocStart(extend bool) { e.moveTo(0, extend) }

// DocEnd moves to the end of the file.
func (e *Editor) DocEnd(extend bool) { e.moveTo(e.File.Len(), extend) }

// SelectAll selects the whole file.
func (e *Editor) SelectAll() {
	e.moveTo(e.File.Len(), false)
	e.anchor = 0
}

// ClearSelection keeps the cursor where it is.
func (e *Editor) ClearSelection() { e.moveTo(e.cursor, false) }

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
	line := e.File.LineOf(e.cursor)
	start := e.File.LineStart(line)
	if e.cursor == start {
		e.moveTo(e.previous(), extend)
		return
	}
	text := e.File.Slice(start, e.cursor)
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
	e.moveTo(start+len(text), extend)
}

// WordRight moves to the end of the next word.
func (e *Editor) WordRight(extend bool) {
	_, end, next := e.lineSpan(e.File.LineOf(e.cursor))
	if e.cursor >= end {
		e.moveTo(next, extend)
		return
	}
	text := e.File.Slice(e.cursor, end)
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
	e.moveTo(e.cursor+at, extend)
}

// Editing. Every method returns buffer.ErrReadOnly for a read-only file.

func (e *Editor) replace(start, end int, text string, join bool) error {
	e.File.Mark = mark{e.cursor, e.anchor}
	edit := buffer.Edit{Start: start, End: end, Text: text}
	apply := e.File.Apply
	if join {
		apply = e.File.Continue
	}
	if err := apply(edit); err != nil {
		return err
	}
	e.moveTo(start+len(text), false)
	e.File.Mark = mark{e.cursor, e.anchor}
	return nil
}

// Type inserts typed text over any selection. Undo takes back a word at a time.
func (e *Editor) Type(text string) error {
	start, end := e.Selection()
	space := strings.TrimSpace(text) == ""
	join := e.last == editTyping && start == end && (space || !e.lastWasSpace)
	if err := e.replace(start, end, text, join); err != nil {
		return err
	}
	e.last, e.lastWasSpace = editTyping, space
	return nil
}

// Newline starts a new line with this line's indent and line ending.
func (e *Editor) Newline() error {
	start, end := e.Selection()
	lineStart, lineEnd, _ := e.lineSpan(e.File.LineOf(start))
	text := e.File.Slice(lineStart, min(start, lineEnd))
	indent := text[:len(text)-len(bytes.TrimLeft(text, " \t"))]
	return e.replace(start, end, e.lineEnding()+string(indent), false)
}

// lineEnding returns this line's ending, or the previous one's for a last line without
// one, so mixed files keep each part's own style.
func (e *Editor) lineEnding() string {
	line := e.File.LineOf(e.cursor)
	if line+1 >= e.File.LineCount() {
		line--
	}
	if line < 0 {
		return "\n"
	}
	_, end, next := e.lineSpan(line)
	return string(e.File.Slice(end, next))
}

// Paste inserts text over any selection, with line breaks converted to this file's.
func (e *Editor) Paste(text string) error {
	ending := e.lineEnding()
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if ending != "\n" {
		text = strings.ReplaceAll(text, "\n", ending)
	}
	start, end := e.Selection()
	return e.replace(start, end, text, false)
}

// Backspace removes the selection, or the character before the cursor.
func (e *Editor) Backspace() error {
	start, end := e.Selection()
	if start != end {
		return e.replace(start, end, "", false)
	}
	return e.erase(e.previous(), e.cursor)
}

// Delete removes the selection, or the character after the cursor.
func (e *Editor) Delete() error {
	start, end := e.Selection()
	if start != end {
		return e.replace(start, end, "", false)
	}
	return e.erase(e.cursor, e.next())
}

func (e *Editor) erase(start, end int) error {
	if start == end {
		return nil
	}
	if err := e.replace(start, end, "", e.last == editErasing); err != nil {
		return err
	}
	e.last = editErasing
	return nil
}

// Copy returns the selection, or the current line with its line break, as VS Code does.
func (e *Editor) Copy() string {
	start, end := e.copyRange()
	return string(e.File.Slice(start, end))
}

// Cut removes and returns what Copy would return.
func (e *Editor) Cut() (string, error) {
	start, end := e.copyRange()
	text := string(e.File.Slice(start, end))
	return text, e.replace(start, end, "", false)
}

func (e *Editor) copyRange() (start, end int) {
	if start, end = e.Selection(); start != end {
		return start, end
	}
	start, _, next := e.lineSpan(e.File.LineOf(e.cursor))
	return start, next
}

// indentUnit returns one level of indent for inserting at a column, following what the
// file already uses.
func (e *Editor) indentUnit(col int) string {
	f := e.File
	for line := range min(f.LineCount(), indentScanLines) {
		start := f.LineStart(line)
		switch first := f.Slice(start, start+1); {
		case len(first) == 1 && first[0] == '\t':
			return "\t"
		case len(first) == 1 && first[0] == ' ':
			return strings.Repeat(" ", e.TabSize-col%e.TabSize)
		}
	}
	return strings.Repeat(" ", e.TabSize-col%e.TabSize)
}

// Tab indents the selected lines, or inserts one indent at the cursor.
func (e *Editor) Tab() error {
	start, end := e.Selection()
	if start == end {
		_, col := e.Position()
		return e.Type(e.indentUnit(col))
	}
	first, last := e.selectedLines(start, end)
	edits := make([]buffer.Edit, 0, last-first+1)
	unit := e.indentUnit(0)
	for line := first; line <= last; line++ {
		at := e.File.LineStart(line)
		edits = append(edits, buffer.Edit{Start: at, End: at, Text: unit})
	}
	return e.indentEdits(edits, first, last)
}

// Outdent removes one indent from the selected lines, or the cursor's line.
func (e *Editor) Outdent() error {
	first, last := e.selectedLines(e.Selection())
	edits := make([]buffer.Edit, 0, last-first+1)
	for line := first; line <= last; line++ {
		at, end, _ := e.lineSpan(line)
		text := e.File.Slice(at, min(end, at+e.TabSize))
		remove := 0
		if len(text) > 0 && text[0] == '\t' {
			remove = 1
		} else {
			remove = len(text) - len(bytes.TrimLeft(text, " "))
		}
		if remove > 0 {
			edits = append(edits, buffer.Edit{Start: at, End: at + remove})
		}
	}
	return e.indentEdits(edits, first, last)
}

func (e *Editor) selectedLines(start, end int) (first, last int) {
	first, last = e.File.LineOf(start), e.File.LineOf(end)
	if last > first && e.File.LineStart(last) == end {
		last-- // A selection ending at the start of a line doesn't include it.
	}
	return first, last
}

// indentEdits applies the edits as one step, and selects the whole lines afterwards.
func (e *Editor) indentEdits(edits []buffer.Edit, first, last int) error {
	if len(edits) == 0 {
		return nil
	}
	e.File.Mark = mark{e.cursor, e.anchor}
	if err := e.File.Apply(edits...); err != nil {
		return err
	}
	_, end, _ := e.lineSpan(last)
	e.moveTo(end, false)
	e.anchor = e.File.LineStart(first)
	e.File.Mark = mark{e.cursor, e.anchor}
	return nil
}

// ReplaceAll makes text the whole content as one undo step, and goes to the start.
func (e *Editor) ReplaceAll(text string) error {
	if err := e.replace(0, e.File.Len(), text, false); err != nil {
		return err
	}
	e.moveTo(0, false)
	return nil
}

// Undo reverses the last step, and reports whether there was one.
func (e *Editor) Undo() bool { return e.restore(e.File.Undo()) }

// Redo restores the last undone step, and reports whether there was one.
func (e *Editor) Redo() bool { return e.restore(e.File.Redo()) }

func (e *Editor) restore(changed bool) bool {
	if changed {
		if m, ok := e.File.Mark.(mark); ok {
			e.moveTo(m.anchor, false)
			e.moveTo(m.cursor, true)
		}
	}
	return changed
}

// Reload replaces the text with the file on disk, keeping the cursor on the same line
// and column where those still exist.
func (e *Editor) Reload() error {
	line, col := e.Position()
	if err := e.File.Reload(); err != nil {
		return err
	}
	e.moveTo(e.offsetAtColumn(line, col), false)
	return nil
}
