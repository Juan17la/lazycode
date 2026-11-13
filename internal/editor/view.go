package editor

import (
	"bytes"
	"slices"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// maxBytesPerColumn bounds how much of a long line is read to fill the screen. Text
// needing more than this per column just shows less of the line.
const maxBytesPerColumn = 32

// Cell is one thing to draw on a row. X is relative to the text area.
type Cell struct {
	X        int
	Offset   int // The byte in the file where the character starts, for syntax colours.
	Text     string
	Width    int
	Selected bool
	Cursor   bool // A cursor other than the main one, which the terminal draws itself.
	Match    bool // Part of a find match.
	Special  bool // Text stands in for something that can't be shown as it is.
}

// glyph is one character of a line, as it appears on screen.
type glyph struct {
	offset, col, width int // offset is relative to the text given to walk.
	text               string
	special            bool
}

// walk goes through text, which starts at column col, until fn returns false.
func walk(text []byte, col, tabSize int, fn func(g glyph) bool) {
	offset, state := 0, -1
	for len(text) > 0 {
		// Printable ASCII followed by ASCII is always a whole character, and skipping
		// segmentation is what keeps a line of minified code near the frame budget.
		// ponytail: a 1 MB line of non-ASCII text is several times slower than ASCII.
		// Remembering columns along the line is the upgrade if that shows up.
		if c := text[0]; c >= 0x20 && c < 0x7f && (len(text) == 1 || text[1] < 0x80) {
			if !fn(glyph{offset: offset, col: col, width: 1, text: string(text[:1])}) {
				return
			}
			text = text[1:]
			offset++
			col++
			state = -1
			continue
		}
		var cluster []byte
		var width int
		cluster, text, width, state = uniseg.FirstGraphemeCluster(text, state)
		g := glyph{offset: offset, col: col, width: width}
		r, size := utf8.DecodeRune(cluster)
		switch {
		case r == '\t':
			g.width = tabSize - col%tabSize
			g.text = " "
		case r == utf8.RuneError && size <= 1: // Invalid UTF-8.
			g.width, g.text, g.special = 1, "\uFFFD", true
		case unsafeToShow(r) || width == 0:
			// Escape sequences in a file must never reach the terminal. Bidirectional
			// controls can reorder what's shown, hiding what the code really does.
			g.width, g.text, g.special = 1, "?", true
		default:
			g.text = string(cluster)
		}
		if !fn(g) {
			return
		}
		offset += len(cluster)
		col += g.width
	}
}

func unsafeToShow(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029'
}

// columnOf returns the column of offset, on the line starting at lineStart.
func (e *Editor) columnOf(lineStart, offset int) int {
	col := 0
	walk(e.File.Slice(lineStart, offset), 0, e.TabSize, func(g glyph) bool {
		col = g.col + g.width
		return true
	})
	return col
}

// offsetAtColumn returns the offset of the character covering col, or the end of the
// line if it's shorter.
func (e *Editor) offsetAtColumn(line, col int) int {
	start, end, _ := e.lineSpan(line)
	result := end
	limit := min(end, start+(col+1)*maxBytesPerColumn)
	walk(e.File.Slice(start, limit), 0, e.TabSize, func(g glyph) bool {
		if g.col+g.width > col {
			result = start + g.offset
			return false
		}
		return true
	})
	return result
}

// Scroll sets the size of the text area, and scrolls to keep the main cursor in it if the
// cursor or the text has changed. So the wheel can look elsewhere until the next edit or move.
func (e *Editor) Scroll(width, height int) {
	e.height = max(1, height)
	e.ScrollLine = max(0, min(e.ScrollLine, e.File.LineCount()-1))
	if e.followed.pos == e.cur().pos && e.followed.text.Same(e.File.Snapshot) && e.followed.valid {
		return
	}
	e.followed.valid, e.followed.pos, e.followed.text = true, e.cur().pos, e.File.Snapshot
	line, col := e.Position()
	if line < e.ScrollLine {
		e.ScrollLine = line
	} else if line >= e.ScrollLine+e.height {
		e.ScrollLine = line - e.height + 1
	}
	if col < e.ScrollCol {
		e.ScrollCol = col
	} else if col >= e.ScrollCol+width {
		e.ScrollCol = col - width + 1
	}
}

// ScrollBy moves the view without moving the cursors, as the mouse wheel does.
func (e *Editor) ScrollBy(lines int) {
	e.ScrollLine = max(0, min(e.ScrollLine+lines, e.File.LineCount()-1))
}

// WrapBreaks returns the column each row of a wrapped line starts at, the first being 0. It breaks after
// a space when the row has one, and otherwise wherever the width runs out. Only a screenful of the line
// is looked at, so that a very long line stays cheap.
func (e *Editor) WrapBreaks(line, width int) []int {
	start, end, _ := e.lineSpan(line)
	breaks := []int{0}
	rowStart, afterSpace := 0, 0
	limit := min(end, start+width*max(1, e.height)*maxBytesPerColumn)
	walk(e.File.Slice(start, limit), 0, e.TabSize, func(g glyph) bool {
		if g.col+g.width-rowStart > width && g.col > rowStart {
			rowStart = g.col
			if afterSpace > breaks[len(breaks)-1] {
				rowStart = afterSpace
			}
			breaks = append(breaks, rowStart)
		}
		if g.text == " " {
			afterSpace = g.col + g.width
		}
		return true
	})
	return breaks
}

// WrapRowOf returns which row of a wrapped line holds a column.
func WrapRowOf(breaks []int, col int) int {
	i, found := slices.BinarySearch(breaks, col)
	if found {
		return i
	}
	return max(0, i-1)
}

// ScrollWrapped is Scroll for word wrap: nothing scrolls sideways, and the cursor's row has to fit
// after the rows of the lines above it. Like Scroll, it leaves the wheel alone until the cursor moves.
func (e *Editor) ScrollWrapped(width, height int) {
	e.WrapWidth = width
	moved := !e.followed.valid || e.followed.pos != e.cur().pos || !e.followed.text.Same(e.File.Snapshot)
	e.ScrollCol = 0
	e.Scroll(1<<30, height)
	if !moved {
		return
	}
	for {
		if _, y, ok := e.ScreenCursorWrapped(width); ok && y < e.height {
			return
		}
		line, _ := e.Position()
		if e.ScrollLine >= line {
			return // Its line alone is taller than the screen.
		}
		e.ScrollLine++
	}
}

// ScreenCursorWrapped returns the cursor's place on screen under word wrap. ok is false above the screen.
func (e *Editor) ScreenCursorWrapped(width int) (x, y int, ok bool) {
	line, col := e.Position()
	if line < e.ScrollLine {
		return 0, 0, false
	}
	for l := e.ScrollLine; l < line; l++ {
		if y += len(e.WrapBreaks(l, width)); y >= e.height {
			return 0, y, true
		}
	}
	breaks := e.WrapBreaks(line, width)
	row := WrapRowOf(breaks, col)
	return col - breaks[row], y + row, true
}

// SelectWord selects the word at a place, as a double click does. Outside any word it's just a click.
func (e *Editor) SelectWord(line, col int) {
	e.GoTo(line, col, false)
	start, end := e.wordAt(e.cur().pos)
	e.moveTo(start, false)
	e.moveTo(end, true)
}

// SelectLine selects a whole line and its line break, as a triple click does.
func (e *Editor) SelectLine(line int) {
	start, _, next := e.lineSpan(max(0, min(line, e.File.LineCount()-1)))
	e.moveTo(start, false)
	e.moveTo(next, true)
}

// ScreenCursor returns the cursor's position in the text area.
func (e *Editor) ScreenCursor() (x, y int) {
	line, col := e.Position()
	return col - e.ScrollCol, line - e.ScrollLine
}

// selectedAt reports whether offset is inside a selection.
func (e *Editor) selectedAt(offset int) bool {
	i, _ := slices.BinarySearchFunc(e.cursors, offset, func(c cursor, at int) int {
		if c.end() <= at {
			return -1
		}
		return 1
	})
	return i < len(e.cursors) && e.cursors[i].start() <= offset
}

// otherCursorAt reports whether a cursor other than the main one sits at offset. The
// terminal only draws one cursor, so the view draws the rest.
func (e *Editor) otherCursorAt(offset int) bool {
	i, _ := slices.BinarySearchFunc(e.cursors, offset, func(c cursor, at int) int {
		if c.end() < at {
			return -1
		}
		return 1
	})
	for ; i < len(e.cursors) && e.cursors[i].start() <= offset; i++ {
		if e.cursors[i].pos == offset && i != e.primary {
			return true
		}
	}
	return false
}

// matchesOn returns where Highlight appears in text, as start and end pairs.
func (e *Editor) matchesOn(text []byte) [][2]int {
	if e.Highlight == "" {
		return nil
	}
	needle := []byte(e.Highlight)
	var found [][2]int
	for at := 0; ; {
		i := bytes.Index(text[at:], needle)
		if i < 0 {
			return found
		}
		found = append(found, [2]int{at + i, at + i + len(needle)})
		at += i + len(needle)
	}
}

// Row returns what to draw for a line, for a text area of this width.
func (e *Editor) Row(line, width int) []Cell {
	start, end, next := e.lineSpan(line)
	var cells []Cell
	right := e.ScrollCol + width
	endCol := 0
	limit := min(end, start+right*maxBytesPerColumn)
	text := e.File.Slice(start, limit)
	matches := e.matchesOn(text)
	walk(text, 0, e.TabSize, func(g glyph) bool {
		offset := start + g.offset
		endCol = g.col + g.width
		if g.col >= right {
			return false
		}
		if endCol <= e.ScrollCol {
			return true // Off the left edge. On a very long line, most of it is.
		}
		for len(matches) > 0 && matches[0][1] <= g.offset {
			matches = matches[1:]
		}
		cell := Cell{
			Offset:   offset,
			Selected: e.selectedAt(offset),
			Cursor:   e.otherCursorAt(offset),
			Match:    len(matches) > 0 && matches[0][0] <= g.offset,
		}
		if g.text == " " || g.col < e.ScrollCol || endCol > right { // A tab, or cut off by an edge.
			cell.Text, cell.Width = " ", 1
			for x := max(g.col, e.ScrollCol); x < min(endCol, right); x++ {
				cell.X = x - e.ScrollCol
				cells = append(cells, cell)
				cell.Cursor = false
			}
			return true
		}
		cell.X, cell.Text, cell.Width, cell.Special = g.col-e.ScrollCol, g.text, g.width, g.special
		cells = append(cells, cell)
		return true
	})
	// Show a selected line break, and a cursor at the end of a line.
	selected := end < next && e.selectedAt(end)
	if other := e.otherCursorAt(end); (selected || other) && endCol >= e.ScrollCol && endCol < right && limit == end {
		cells = append(cells, Cell{X: endCol - e.ScrollCol, Offset: end, Text: " ", Width: 1, Selected: selected, Cursor: other})
	}
	return cells
}

// GutterWidth returns the columns to keep for line numbers, including a space each side.
func (e *Editor) GutterWidth() int {
	width := 1
	for n := e.File.LineCount(); n >= 10; n /= 10 {
		width++
	}
	return width + 2
}

// CursorOffset returns the byte offset of the main cursor.
func (e *Editor) CursorOffset() int { return e.cur().pos }

// WordAtCursor returns the word at or just before the main cursor, as byte offsets.
func (e *Editor) WordAtCursor() (start, end int) { return e.wordAt(e.cur().pos) }

// GoToOffset puts the only cursor at a byte offset, kept inside the text.
func (e *Editor) GoToOffset(offset int) { e.moveTo(offset, false) }
