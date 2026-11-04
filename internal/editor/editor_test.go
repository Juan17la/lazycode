package editor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Juan17la/lazycode/internal/buffer"
)

func open(t testing.TB, content string) *Editor {
	t.Helper()
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := buffer.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return New(f, 4)
}

func content(e *Editor) string { return string(e.File.Slice(0, e.File.Len())) }

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func wantCursor(t *testing.T, e *Editor, want int) {
	t.Helper()
	if e.cursor != want {
		t.Fatalf("cursor at %d, want %d, in %q", e.cursor, want, content(e))
	}
}

func TestLeftRightWalkWholeCharacters(t *testing.T) {
	// e + combining accent, a flag, a family emoji, a CJK character, and invalid UTF-8.
	characters := []string{"e\u0301", "🇨🇴", "👨\u200d👩\u200d👧", "世", "\xff", "a"}
	e := open(t, strings.Join(characters, ""))
	at := 0
	for _, c := range characters {
		e.Right(false)
		at += len(c)
		wantCursor(t, e, at)
	}
	e.Right(false)
	wantCursor(t, e, at)
	for i := len(characters) - 1; i >= 0; i-- {
		e.Left(false)
		at -= len(characters[i])
		wantCursor(t, e, at)
	}
	e.Left(false)
	wantCursor(t, e, 0)
}

func TestMovingAcrossLineBreaks(t *testing.T) {
	for name, ending := range map[string]string{"LF": "\n", "CRLF": "\r\n"} {
		t.Run(name, func(t *testing.T) {
			e := open(t, "ab"+ending+"cd")
			e.End(false)
			wantCursor(t, e, 2)
			e.Right(false)
			wantCursor(t, e, 2+len(ending))
			e.Left(false)
			wantCursor(t, e, 2)
		})
	}
}

func TestUpDownKeepTheColumn(t *testing.T) {
	e := open(t, "long line here\nab\n世界世界世界\nanother long line")
	e.moveTo(8, false)
	e.Down(false)
	wantCursor(t, e, 17) // End of "ab", which is shorter.
	e.Down(false)
	// Column 8 is the start of the fifth two-column character.
	wantCursor(t, e, 18+4*3)
	e.Down(false)
	if _, col := e.Position(); col != 8 {
		t.Fatalf("col %d", col)
	}
	e.Up(false)
	e.Up(false)
	e.Up(false)
	wantCursor(t, e, 8)
	e.Up(false)
	wantCursor(t, e, 0)
	e.DocEnd(false)
	end := e.cursor
	e.Down(false)
	wantCursor(t, e, end)
}

func TestWideCharacterRoundsToItsStart(t *testing.T) {
	e := open(t, "abcd\n世界")
	e.moveTo(3, false) // Column 3 is the second half of 界.
	e.Down(false)
	wantCursor(t, e, 5+3)
}

func TestColumnsCountTabs(t *testing.T) {
	e := open(t, "\tx\nab\tx")
	e.moveTo(1, false)
	if _, col := e.Position(); col != 4 {
		t.Fatalf("after a tab: col %d", col)
	}
	e.moveTo(6, false)
	if _, col := e.Position(); col != 4 {
		t.Fatalf("a tab reaches the next stop: col %d", col)
	}
}

func TestHome(t *testing.T) {
	e := open(t, "    code here")
	e.End(false)
	e.Home(false)
	wantCursor(t, e, 4)
	e.Home(false)
	wantCursor(t, e, 0)
	e.Home(false)
	wantCursor(t, e, 4)
}

func TestWords(t *testing.T) {
	e := open(t, "foo_bar  baz(qux)\nnext")
	stops := []int{7, 12, 13, 16, 17, 18, 22}
	for _, stop := range stops {
		e.WordRight(false)
		wantCursor(t, e, stop)
	}
	e.WordRight(false)
	wantCursor(t, e, 22)
	for _, stop := range []int{18, 17, 16, 13, 12, 9, 0} {
		e.WordLeft(false)
		wantCursor(t, e, stop)
	}
	e.WordLeft(false)
	wantCursor(t, e, 0)
}

func TestWordsOnBlankEnds(t *testing.T) {
	e := open(t, "   ")
	e.WordRight(false)
	wantCursor(t, e, 3)
	e.WordLeft(false)
	wantCursor(t, e, 0)
}

func TestSelection(t *testing.T) {
	e := open(t, "hello\nworld")
	e.Right(true)
	e.Right(true)
	if s, end := e.Selection(); s != 0 || end != 2 {
		t.Fatalf("selection %d..%d", s, end)
	}
	if got := e.Copy(); got != "he" {
		t.Fatalf("Copy = %q", got)
	}
	e.Right(false)
	wantCursor(t, e, 2)
	if s, end := e.Selection(); s != end {
		t.Fatal("moving didn't clear the selection")
	}
	e.Down(true)
	e.Left(true)
	if s, end := e.Selection(); s != 2 || end != 7 {
		t.Fatalf("selection %d..%d", s, end)
	}
	e.Left(false)
	wantCursor(t, e, 2)

	e.moveTo(3, false)
	e.Left(true) // Backwards.
	if s, end := e.Selection(); s != 2 || end != 3 {
		t.Fatalf("backwards selection %d..%d", s, end)
	}

	e.SelectAll()
	if got := e.Copy(); got != "hello\nworld" {
		t.Fatalf("select all: %q", got)
	}
}

func TestTypingReplacesTheSelection(t *testing.T) {
	e := open(t, "hello world")
	e.moveTo(6, false)
	e.End(true)
	must(t, e.Type("x"))
	if got := content(e); got != "hello x" {
		t.Fatalf("got %q", got)
	}
	wantCursor(t, e, 7)
}

func TestUndoTakesBackAWordAtATime(t *testing.T) {
	e := open(t, "")
	for _, r := range "hello big world" {
		must(t, e.Type(string(r)))
	}
	for _, want := range []string{"hello big ", "hello ", ""} {
		if !e.Undo() {
			t.Fatal("nothing to undo")
		}
		if got := content(e); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		wantCursor(t, e, len(want))
	}
}

func TestMovingEndsTheUndoStep(t *testing.T) {
	e := open(t, "")
	must(t, e.Type("a"))
	e.Left(false)
	e.Right(false)
	must(t, e.Type("b"))
	e.Undo()
	if got := content(e); got != "a" {
		t.Fatalf("got %q", got)
	}
}

func TestUndoRestoresCursorAndSelection(t *testing.T) {
	e := open(t, "hello world")
	e.moveTo(0, false)
	e.moveTo(5, true)
	must(t, e.Type("bye"))
	wantCursor(t, e, 3)
	e.DocEnd(false)

	e.Undo()
	if s, end := e.Selection(); s != 0 || end != 5 {
		t.Fatalf("selection after undo: %d..%d", s, end)
	}
	e.Redo()
	wantCursor(t, e, 3)
	if s, end := e.Selection(); s != end {
		t.Fatal("selection after redo")
	}
}

func TestBackspaceAndDelete(t *testing.T) {
	e := open(t, "a世\r\nb")
	e.DocEnd(false)
	must(t, e.Backspace())
	if got := content(e); got != "a世\r\n" {
		t.Fatalf("got %q", got)
	}
	must(t, e.Backspace()) // The whole line break.
	if got := content(e); got != "a世" {
		t.Fatalf("got %q", got)
	}
	must(t, e.Backspace()) // All three bytes.
	if got := content(e); got != "a" {
		t.Fatalf("got %q", got)
	}
	e.Undo()
	if got := content(e); got != "a世\r\nb" {
		t.Fatalf("backspaces should undo together: %q", got)
	}

	e = open(t, "e\u0301x\r\ny")
	must(t, e.Delete())
	if got := content(e); got != "x\r\ny" {
		t.Fatalf("got %q", got)
	}
	e.Right(false)
	must(t, e.Delete())
	if got := content(e); got != "xy" {
		t.Fatalf("got %q", got)
	}
	e.DocEnd(false)
	must(t, e.Delete()) // Nothing there.
	e.DocStart(false)
	must(t, e.Backspace()) // Nothing there.
	if got := content(e); got != "xy" {
		t.Fatalf("got %q", got)
	}
}

func TestNewlineKeepsIndentAndEnding(t *testing.T) {
	e := open(t, "\t  code\r\nnext\nlast")
	e.moveTo(5, false)
	must(t, e.Newline())
	if got := content(e); got != "\t  co\r\n\t  de\r\nnext\nlast" {
		t.Fatalf("CRLF: %q", got)
	}

	e = open(t, "one\r\ntwo\nlast")
	e.DocEnd(false) // The last line has no ending, so it follows the line before.
	must(t, e.Newline())
	if got := content(e); got != "one\r\ntwo\nlast\n" {
		t.Fatalf("last line: %q", got)
	}

	e = open(t, "")
	must(t, e.Newline())
	if got := content(e); got != "\n" {
		t.Fatalf("empty file: %q", got)
	}

	e = open(t, "    ")
	e.moveTo(2, false) // In the middle of the indent, only what's before it is copied.
	must(t, e.Newline())
	if got := content(e); got != "  \n    " {
		t.Fatalf("middle of indent: %q", got)
	}
}

func TestPasteUsesTheFilesLineEndings(t *testing.T) {
	e := open(t, "a\r\nb")
	must(t, e.Paste("1\n2\r\n3\r4"))
	if got := content(e); got != "1\r\n2\r\n3\r\n4a\r\nb" {
		t.Fatalf("got %q", got)
	}
	e = open(t, "a\nb")
	must(t, e.Paste("1\r\n2"))
	if got := content(e); got != "1\n2a\nb" {
		t.Fatalf("got %q", got)
	}
	if !e.Undo() || content(e) != "a\nb" {
		t.Fatal("paste should undo as one step")
	}
}

func TestCopyAndCutWithoutASelectionTakeTheLine(t *testing.T) {
	e := open(t, "one\r\ntwo\nthree")
	e.moveTo(6, false)
	if got := e.Copy(); got != "two\n" {
		t.Fatalf("Copy = %q", got)
	}
	got, err := e.Cut()
	must(t, err)
	if got != "two\n" || content(e) != "one\r\nthree" {
		t.Fatalf("Cut = %q, left %q", got, content(e))
	}
	e.moveTo(1, false)
	got, err = e.Cut()
	must(t, err)
	if got != "one\r\n" {
		t.Fatalf("CRLF: %q", got)
	}
}

func TestTabFollowsTheFile(t *testing.T) {
	e := open(t, "\tindented with a tab\nx")
	e.DocEnd(false)
	must(t, e.Tab())
	if !strings.HasSuffix(content(e), "x\t") {
		t.Fatalf("got %q", content(e))
	}
	e = open(t, "  spaces\nx")
	e.DocEnd(false)
	must(t, e.Tab())
	if !strings.HasSuffix(content(e), "x   ") { // Up to the next stop, at 4.
		t.Fatalf("got %q", content(e))
	}
}

func TestIndentingSelectedLines(t *testing.T) {
	e := open(t, "one\n  two\nthree\nfour")
	e.moveTo(1, false)
	e.moveTo(16, true) // Through the line break after "three": "four" isn't included.
	must(t, e.Tab())
	if got := content(e); got != "    one\n      two\n    three\nfour" {
		t.Fatalf("got %q", got)
	}
	must(t, e.Outdent())
	if got := content(e); got != "one\n  two\nthree\nfour" {
		t.Fatalf("outdent: %q", got)
	}
	e.Undo()
	e.Undo()
	if got := content(e); got != "one\n  two\nthree\nfour" {
		t.Fatalf("each should undo as one step: %q", got)
	}

	e = open(t, "\tx\n  y\nz")
	e.SelectAll()
	must(t, e.Outdent())
	if got := content(e); got != "x\ny\nz" {
		t.Fatalf("mixed outdent: %q", got)
	}
	must(t, e.Outdent()) // Nothing left to remove.
	if got := content(e); got != "x\ny\nz" {
		t.Fatalf("got %q", got)
	}
}

func TestReadOnlyRefusesEveryEdit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("line\n", 2000)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, 100<<20+1); err != nil {
		t.Fatal(err)
	}
	f, err := buffer.Open(path)
	must(t, err)
	defer f.Close()
	e := New(f, 4)
	e.Right(false)
	_, cutErr := e.Cut()
	for name, err := range map[string]error{
		"type": e.Type("x"), "newline": e.Newline(), "paste": e.Paste("x"),
		"backspace": e.Backspace(), "delete": e.Delete(), "cut": cutErr, "tab": e.Tab(),
	} {
		if !errors.Is(err, buffer.ErrReadOnly) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if f.Modified() {
		t.Fatal("modified")
	}
}

func TestReloadKeepsThePlace(t *testing.T) {
	e := open(t, "one\ntwo\nthree\n")
	e.moveTo(10, false)
	if err := os.WriteFile(e.File.Path, []byte("ONE\nTWO\nTHREE!\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	must(t, e.Reload())
	wantCursor(t, e, 10)
	if err := os.WriteFile(e.File.Path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	must(t, e.Reload())
	if e.cursor > e.File.Len() {
		t.Fatalf("cursor %d past the end", e.cursor)
	}
}

func TestReplaceAllIsOneStep(t *testing.T) {
	e := open(t, "disk")
	must(t, e.ReplaceAll("recovered"))
	wantCursor(t, e, 0)
	if !e.File.Modified() {
		t.Fatal("not modified")
	}
	e.Undo()
	if got := content(e); got != "disk" {
		t.Fatalf("got %q", got)
	}
}

// Rows.

func rowText(cells []Cell) string {
	var b strings.Builder
	x := 0
	for _, c := range cells {
		for ; x < c.X; x++ {
			b.WriteByte('.')
		}
		b.WriteString(c.Text)
		x += c.Width
	}
	return b.String()
}

func TestRowShowsWhatTheTerminalCanShow(t *testing.T) {
	for name, tc := range map[string]struct{ line, want string }{
		"plain":      {"abc", "abc"},
		"tab":        {"a\tb", "a   b"},
		"wide":       {"世界", "世界"},
		"combining":  {"e\u0301x", "e\u0301x"},
		"emoji":      {"👨\u200d👩\u200d👧!", "👨\u200d👩\u200d👧!"},
		"invalid":    {"a\xffb", "a\uFFFDb"},
		"escape":     {"a\x1b[31mb", "a?[31mb"},
		"bell":       {"\x07", "?"},
		"bidi":       {"a\u202eb", "a?b"},
		"lone CR":    {"a\rb", "a?b"},
		"zero width": {"\u0301a", "?a"},
	} {
		t.Run(name, func(t *testing.T) {
			got := rowText(open(t, tc.line).Row(0, 80))
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNoControlCharactersEverReachTheScreen(t *testing.T) {
	var all strings.Builder
	for r := rune(0); r < 0x100; r++ {
		if r != '\n' {
			all.WriteRune(r)
		}
		all.WriteByte(byte(r))
	}
	e := open(t, strings.Repeat(" ", 8192)+all.String())
	e.ScrollCol = 8192
	for _, c := range e.Row(0, 5000) {
		for _, r := range c.Text {
			if unsafeToShow(r) && r != '\u200d' {
				t.Fatalf("%q would reach the terminal", c.Text)
			}
		}
	}
}

func TestRowScrolledSideways(t *testing.T) {
	e := open(t, "abcdefghij")
	e.ScrollCol = 3
	if got := rowText(e.Row(0, 4)); got != "defg" {
		t.Fatalf("got %q", got)
	}

	// A wide character cut by either edge becomes a blank, not half a character.
	e = open(t, "a世b")
	e.ScrollCol = 2
	if got := rowText(e.Row(0, 10)); got != " b" {
		t.Fatalf("left edge: %q", got)
	}
	e = open(t, "a世b")
	if got := rowText(e.Row(0, 2)); got != "a " {
		t.Fatalf("right edge: %q", got)
	}

	// Past the end of the line, there's nothing to draw.
	e = open(t, "abc\n")
	e.ScrollCol = 50
	if got := e.Row(0, 10); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	// A tab partly off the left edge still fills what's visible.
	e = open(t, "\tx")
	e.ScrollCol = 2
	if got := rowText(e.Row(0, 10)); got != "  x" {
		t.Fatalf("tab: %q", got)
	}
}

func TestRowMarksTheSelection(t *testing.T) {
	e := open(t, "abc\ndef")
	e.moveTo(1, false)
	e.moveTo(5, true) // "bc", the line break, and "d".
	var selected []string
	for _, c := range e.Row(0, 80) {
		if c.Selected {
			selected = append(selected, c.Text)
		}
	}
	if got := strings.Join(selected, ""); got != "bc " {
		t.Fatalf("first row: %q", got)
	}
	selected = nil
	for _, c := range e.Row(1, 80) {
		if c.Selected {
			selected = append(selected, c.Text)
		}
	}
	if got := strings.Join(selected, ""); got != "d" {
		t.Fatalf("second row: %q", got)
	}
}

func TestRowOfAHugeLineReadsOnlyWhatItNeeds(t *testing.T) {
	e := open(t, strings.Repeat("x", 5<<20))
	cells := e.Row(0, 80)
	if len(cells) != 80 {
		t.Fatalf("%d cells", len(cells))
	}
}

func TestScrollFollowsTheCursor(t *testing.T) {
	e := open(t, strings.Repeat("line\n", 100)+strings.Repeat("x", 200))
	e.Scroll(80, 10)
	if e.ScrollLine != 0 {
		t.Fatalf("Top %d", e.ScrollLine)
	}
	for range 15 {
		e.Down(false)
	}
	e.Scroll(80, 10)
	if e.ScrollLine != 6 {
		t.Fatalf("Top %d, want the cursor on the last row", e.ScrollLine)
	}
	if x, y := e.ScreenCursor(); x != 0 || y != 9 {
		t.Fatalf("cursor at %d,%d", x, y)
	}
	e.Up(false)
	e.Scroll(80, 10)
	if e.ScrollLine != 6 {
		t.Fatalf("Top moved to %d with the cursor still on screen", e.ScrollLine)
	}
	e.DocEnd(false)
	e.Scroll(80, 10)
	if e.ScrollCol != 121 {
		t.Fatalf("Left %d", e.ScrollCol)
	}
	if x, _ := e.ScreenCursor(); x != 79 {
		t.Fatalf("cursor at x=%d", x)
	}
	e.Home(false)
	e.Scroll(80, 10)
	if e.ScrollCol != 0 {
		t.Fatalf("Left %d", e.ScrollCol)
	}
}

func TestPaging(t *testing.T) {
	e := open(t, strings.Repeat("line\n", 100))
	e.Scroll(80, 10)
	e.PageDown(false)
	e.Scroll(80, 10)
	if line, _ := e.Position(); line != 10 || e.ScrollLine != 10 {
		t.Fatalf("line %d, Top %d", line, e.ScrollLine)
	}
	e.PageUp(false)
	e.Scroll(80, 10)
	if line, _ := e.Position(); line != 0 || e.ScrollLine != 0 {
		t.Fatalf("line %d, Top %d", line, e.ScrollLine)
	}
	for range 30 {
		e.PageDown(false)
	}
	e.Scroll(80, 10)
	if line, _ := e.Position(); line != 100 {
		t.Fatalf("line %d", line)
	}
	if e.ScrollLine > 100 {
		t.Fatalf("Top %d", e.ScrollLine)
	}
}

func TestShrinkingTheFileKeepsScrollValid(t *testing.T) {
	e := open(t, strings.Repeat("line\n", 100))
	e.DocEnd(false)
	e.Scroll(80, 10)
	e.SelectAll()
	must(t, e.Backspace())
	e.Scroll(80, 10)
	if e.ScrollLine != 0 {
		t.Fatalf("Top %d", e.ScrollLine)
	}
}

func TestGutterWidth(t *testing.T) {
	for lines, want := range map[int]int{1: 3, 9: 3, 10: 4, 100: 5} {
		if got := open(t, strings.Repeat("\n", lines-1)).GutterWidth(); got != want {
			t.Errorf("%d lines: %d, want %d", lines, got, want)
		}
	}
}

// Random keys must never leave the cursor out of range, or split the text wrong.
func FuzzKeys(f *testing.F) {
	f.Add("a世\r\n\tb\n", []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19})
	f.Add("e\u0301\xff\n\n", []byte{19, 18, 17, 1, 1, 1, 12, 13})
	f.Fuzz(func(t *testing.T, text string, keys []byte) {
		if strings.IndexByte(text, 0) >= 0 {
			t.Skip("a binary file can't be opened")
		}
		e := open(t, text)
		for _, key := range keys {
			extend := key&0x80 != 0
			switch key & 0x7f % 22 {
			case 0:
				e.Left(extend)
			case 1:
				e.Right(extend)
			case 2:
				e.Up(extend)
			case 3:
				e.Down(extend)
			case 4:
				e.Home(extend)
			case 5:
				e.End(extend)
			case 6:
				e.WordLeft(extend)
			case 7:
				e.WordRight(extend)
			case 8:
				e.PageUp(extend)
			case 9:
				e.PageDown(extend)
			case 10:
				e.DocStart(extend)
			case 11:
				e.DocEnd(extend)
			case 12:
				must(t, e.Type("x"))
			case 13:
				must(t, e.Type("世"))
			case 14:
				must(t, e.Newline())
			case 15:
				must(t, e.Backspace())
			case 16:
				must(t, e.Delete())
			case 17:
				must(t, e.Tab())
			case 18:
				must(t, e.Outdent())
			case 19:
				e.Undo()
			case 20:
				e.Redo()
			case 21:
				if _, err := e.Cut(); err != nil {
					t.Fatal(err)
				}
			}
			if e.cursor < 0 || e.cursor > e.File.Len() || e.anchor < 0 || e.anchor > e.File.Len() {
				t.Fatalf("cursor %d, anchor %d, length %d", e.cursor, e.anchor, e.File.Len())
			}
			e.Scroll(20, 5)
			if x, y := e.ScreenCursor(); x < 0 || x >= 20 || y < 0 || y >= 5 {
				t.Fatalf("cursor off screen: %d,%d", x, y)
			}
			for line := e.ScrollLine; line < e.ScrollLine+5 && line < e.File.LineCount(); line++ {
				for _, c := range e.Row(line, 20) {
					if c.X < 0 || c.X+c.Width > 20 {
						t.Fatalf("cell outside the row: %+v", c)
					}
				}
			}
		}
		for e.Undo() {
		}
		if got := content(e); got != text {
			t.Fatalf("undoing everything: %q, want %q", got, text)
		}
	})
}

// A frame with the cursor at the end of a 1 MB line, such as minified JavaScript.
func BenchmarkFrameLongLine(b *testing.B) {
	e := open(b, strings.Repeat("x", 1<<20))
	e.DocEnd(false)
	for b.Loop() {
		e.Scroll(100, 50)
		e.Row(0, 100)
		e.ScreenCursor()
	}
}

// One frame's editor work, on a 100 MiB file. The budget for a key press is 16 ms.
func BenchmarkFrame100MB(b *testing.B) {
	e := open(b, strings.Repeat("the quick brown fox jumps over the lazy dog, line\n", 100<<20/50))
	for b.Loop() {
		e.PageDown(false)
		e.Scroll(100, 50)
		for line := e.ScrollLine; line < e.ScrollLine+50; line++ {
			e.Row(line, 100)
		}
		if e.File.LineOf(e.cursor) > 2_000_000 {
			e.DocStart(false)
		}
	}
}
