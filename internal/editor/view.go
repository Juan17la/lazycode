package editor

import (
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
	Text     string
	Width    int
	Selected bool
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
		// ponytail: a 1 MB line of non-ASCII text still takes about 30 ms per move.
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

// Scroll sets the size of the text area, and scrolls to keep the cursor in it.
func (e *Editor) Scroll(width, height int) {
	e.height = max(1, height)
	line, col := e.Position()
	e.ScrollLine = max(0, min(e.ScrollLine, e.File.LineCount()-1))
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

// ScreenCursor returns the cursor's position in the text area.
func (e *Editor) ScreenCursor() (x, y int) {
	line, col := e.Position()
	return col - e.ScrollCol, line - e.ScrollLine
}

// Row returns what to draw for a line, for a text area of this width.
func (e *Editor) Row(line, width int) []Cell {
	start, end, next := e.lineSpan(line)
	selStart, selEnd := e.Selection()
	selected := func(offset int) bool { return selStart <= offset && offset < selEnd }

	var cells []Cell
	right := e.ScrollCol + width
	blank := func(from, to int, sel bool) {
		for x := max(from, e.ScrollCol); x < min(to, right); x++ {
			cells = append(cells, Cell{X: x - e.ScrollCol, Text: " ", Width: 1, Selected: sel})
		}
	}
	endCol := 0
	limit := min(end, start+right*maxBytesPerColumn)
	walk(e.File.Slice(start, limit), 0, e.TabSize, func(g glyph) bool {
		sel := selected(start + g.offset)
		endCol = g.col + g.width
		switch {
		case g.col >= right:
			return false
		case g.text == " " || g.col < e.ScrollCol || endCol > right: // A tab, or cut off by an edge.
			blank(g.col, endCol, sel)
		default:
			cells = append(cells, Cell{X: g.col - e.ScrollCol, Text: g.text, Width: g.width, Selected: sel, Special: g.special})
		}
		return true
	})
	if end < next && selected(end) { // Show that the line break is selected too.
		blank(endCol, endCol+1, true)
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
