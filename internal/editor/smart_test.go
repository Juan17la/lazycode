package editor

import (
	"strings"
	"testing"
)

func typeAll(t *testing.T, e *Editor, text string) {
	t.Helper()
	for _, r := range text {
		must(t, e.Type(string(r)))
	}
}

func named(e *Editor, name string) *Editor {
	e.File.Path = strings.TrimSuffix(e.File.Path, "file.txt") + name
	return e
}

func TestPairs(t *testing.T) {
	for _, test := range []struct{ name, start, typed, want string }{
		{"file.txt", "", "(", "()"},
		{"file.txt", "", "[", "[]"},
		{"file.txt", "", "{", "{}"},
		{"file.txt", "", "'", "''"},
		{"file.txt", "", `"`, `""`},
		{"file.txt", "", "`", "``"},
		{"file.txt", "", "f(x)", "f(x)"},   // The closer steps over.
		{"file.txt", "", `"a"`, `"a"`},     // So does a quote.
		{"file.txt", "", "don't", "don't"}, // An apostrophe stays alone.
		{"file.txt", "word", "(", "(word"}, // Not in front of text.
		{"page.html", "", "<a>", "<a>"},    // Markup closes, and steps over.
		{"page.html", "", "<", "<>"},       //
		{"main.go", "", "a < b", "a < b"},  // A comparison doesn't.
		{"main.go", "", "Vec<", "Vec<>"},   // A generic does.
	} {
		e := named(open(t, test.start), test.name)
		typeAll(t, e, test.typed)
		if got := content(e); got != test.want {
			t.Errorf("%s: typing %q in %q gave %q, want %q", test.name, test.typed, test.start, got, test.want)
		}
	}
}

func TestPairSurroundsEverySelection(t *testing.T) {
	e := open(t, "one two")
	e.SelectAll()
	must(t, e.Type("("))
	if got := content(e); got != "(one two)" {
		t.Fatal(got)
	}
	if s, end := e.Selection(); s != 1 || end != 8 {
		t.Fatalf("selection %d to %d", s, end)
	}
	e.Undo()
	if got := content(e); got != "one two" {
		t.Fatal(got)
	}
}

func TestPairsWithSeveralCursors(t *testing.T) {
	e := open(t, "\n\n")
	e.AddCursorLine(1)
	e.AddCursorLine(1)
	typeAll(t, e, "(x")
	if got := content(e); got != "(x)\n(x)\n(x)" {
		t.Fatalf("%q", got)
	}
	must(t, e.Backspace())
	must(t, e.Backspace())
	if got := content(e); got != "\n\n" {
		t.Fatalf("backspace leaves %q", got)
	}
}

func TestEnterBetweenBracketsOpensABlock(t *testing.T) {
	e := open(t, "  ")
	e.DocEnd(false)
	must(t, e.Type("{"))
	must(t, e.Newline())
	if got := content(e); got != "  {\n    \n  }" {
		t.Fatalf("%q", got)
	}
	wantCursor(t, e, len("  {\n    "))
}

func TestMoveLines(t *testing.T) {
	e := open(t, "a\nb\nc")
	must(t, e.MoveLines(1))
	if got := content(e); got != "b\na\nc" {
		t.Fatal(got)
	}
	wantCursor(t, e, 2)
	must(t, e.MoveLines(1)) // Onto the last line, which has no line break.
	if got := content(e); got != "b\nc\na" {
		t.Fatalf("%q", got)
	}
	must(t, e.MoveLines(1)) // Nowhere further.
	must(t, e.MoveLines(-1))
	must(t, e.MoveLines(-1))
	if got := content(e); got != "a\nb\nc" {
		t.Fatalf("%q", got)
	}
	wantCursor(t, e, 0)
}

func TestMoveAndCopyLinesWithSeveralCursors(t *testing.T) {
	e := open(t, "a\nb\nc\nd")
	e.AddCursorLine(1)
	must(t, e.MoveLines(1))
	if got := content(e); got != "c\na\nb\nd" {
		t.Fatalf("%q", got)
	}
	must(t, e.CopyLines(1))
	if got := content(e); got != "c\na\nb\na\nb\nd" {
		t.Fatalf("%q", got)
	}
	if e.CursorCount() != 2 {
		t.Fatal(e.CursorCount())
	}
	wantCursor(t, e, len("c\na\nb\na\n"))
}

func TestDeleteLines(t *testing.T) {
	e := open(t, "a\nb\nc")
	e.AddCursorLine(1)
	must(t, e.DeleteLines())
	if got := content(e); got != "c" {
		t.Fatalf("%q", got)
	}
	last := open(t, "a\nb")
	last.DocEnd(false)
	must(t, last.DeleteLines())
	if got := content(last); got != "a" {
		t.Fatalf("%q", got)
	}
}

func TestInsertLine(t *testing.T) {
	e := open(t, "  a")
	must(t, e.InsertLine(true))
	if got := content(e); got != "  a\n  " {
		t.Fatalf("%q", got)
	}
	up := open(t, "  a")
	up.DocEnd(false)
	must(t, up.InsertLine(false))
	if got := content(up); got != "  \n  a" {
		t.Fatalf("%q", got)
	}
	wantCursor(t, up, 2)
}

func TestAddCursorKeepsTheColumnAcrossShortLines(t *testing.T) {
	e := open(t, "abcd\na\nabcd")
	e.End(false)
	e.AddCursorLine(1)
	e.AddCursorLine(1)
	must(t, e.Type("!"))
	if got := content(e); got != "abcd!\na!\nabcd!" {
		t.Fatalf("%q", got)
	}
	e.AddCursorLine(-1) // Already at the top, on line 0: nothing above.
	if e.CursorCount() != 3 {
		t.Fatal(e.CursorCount())
	}
}

func TestToggleCursor(t *testing.T) {
	e := open(t, "ab\ncd")
	e.ToggleCursor(1, 1)
	if e.CursorCount() != 2 {
		t.Fatal(e.CursorCount())
	}
	e.ToggleCursor(1, 1)
	if e.CursorCount() != 1 {
		t.Fatal(e.CursorCount())
	}
}

func TestSelectLinesGrows(t *testing.T) {
	e := open(t, "a\nb\nc")
	e.SelectLines()
	if s, end := e.Selection(); s != 0 || end != 2 {
		t.Fatal(s, end)
	}
	e.SelectLines()
	if s, end := e.Selection(); s != 0 || end != 4 {
		t.Fatal(s, end)
	}
}

func TestDeleteWords(t *testing.T) {
	e := open(t, "one two")
	e.DocEnd(false)
	must(t, e.DeleteWordLeft())
	if got := content(e); got != "one " {
		t.Fatalf("%q", got)
	}
	r := open(t, "one two")
	must(t, r.DeleteWordRight())
	if got := content(r); got != " two" {
		t.Fatalf("%q", got)
	}
}

func TestToggleComment(t *testing.T) {
	e := named(open(t, "\tif x {\n\n\t\ty()\n"), "main.go")
	e.SelectAll()
	must(t, e.ToggleComment())
	if got := content(e); got != "\t// if x {\n\n\t// \ty()\n" {
		t.Fatalf("%q", got)
	}
	must(t, e.ToggleComment())
	if got := content(e); got != "\tif x {\n\n\t\ty()\n" {
		t.Fatalf("back: %q", got)
	}
	if err := open(t, "x").ToggleComment(); err != ErrNoLineComment {
		t.Fatal(err)
	}
}
