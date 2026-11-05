package lsp

import (
	"bytes"
	"unicode/utf8"
)

// Positions in the protocol count UTF-16 code units, so an emoji is two and most CJK text is one,
// where an editor's offsets are bytes. Text with bytes that aren't UTF-8 counts one unit for each
// bad byte, which is what a server sees after JSON replaces each with U+FFFD.

// Position is a line and a character in UTF-16 units, both counted from zero.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is the text from Start up to End.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Doc finds positions in one version of a text. Make one for each text, and reuse it for every edit.
type Doc struct {
	text   []byte
	starts []int // Where each line begins.
}

// NewDoc indexes the lines of text. Only \n ends a line, as in the editor.
func NewDoc(text []byte) *Doc {
	d := &Doc{text: text, starts: []int{0}}
	for at := 0; ; {
		i := bytes.IndexByte(text[at:], '\n')
		if i < 0 {
			return d
		}
		at += i + 1
		d.starts = append(d.starts, at)
	}
}

// content returns a line without its line break, which is "\n" or "\r\n".
func (d *Doc) content(line int) []byte {
	end := len(d.text)
	if line+1 < len(d.starts) {
		end = d.starts[line+1] - 1
	}
	if end > d.starts[line] && d.text[end-1] == '\r' && end < len(d.text) {
		end--
	}
	return d.text[d.starts[line]:end]
}

// Offset returns the byte offset of p. A line past the end is the end of the text, and a character
// past the end of a line is the end of the line, as the protocol says.
func (d *Doc) Offset(p Position) int {
	if p.Line < 0 {
		return 0
	}
	if p.Line >= len(d.starts) {
		return len(d.text)
	}
	return d.starts[p.Line] + ColumnOffset(d.content(p.Line), p.Character)
}

// Position returns where a byte offset is. An offset inside a character counts as its start.
func (d *Doc) Position(offset int) Position {
	offset = max(0, min(offset, len(d.text)))
	line := 0
	for lo, hi := 0, len(d.starts); lo < hi; {
		mid := (lo + hi) / 2
		if d.starts[mid] <= offset {
			line, lo = mid, mid+1
		} else {
			hi = mid
		}
	}
	text := d.content(line)
	return Position{line, UnitsBefore(text, min(offset-d.starts[line], len(text)))}
}

// ColumnOffset returns how many bytes of a line come before UTF-16 unit character.
func ColumnOffset(line []byte, character int) int {
	at := 0
	for units := 0; at < len(line) && units < character; {
		r, size := utf8.DecodeRune(line[at:])
		units++
		if r >= 0x10000 {
			units++
		}
		at += size
	}
	return at
}

// UnitsBefore returns how many UTF-16 units come before byte offset of a line.
func UnitsBefore(line []byte, offset int) int {
	units := 0
	for at := 0; at < offset && at < len(line); {
		r, size := utf8.DecodeRune(line[at:])
		if at+size > offset { // Inside a character.
			break
		}
		units++
		if r >= 0x10000 {
			units++
		}
		at += size
	}
	return units
}
