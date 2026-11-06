package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Juan17la/lazycode/internal/buffer"
)

func applyEditor(t *testing.T, text string) *Editor {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := buffer.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return New(f, 4)
}

func text(e *Editor) string { return string(e.File.Slice(0, e.File.Len())) }

func TestApplyEditsOneUndoStep(t *testing.T) {
	e := applyEditor(t, "foo bar foo")
	// Out of order on purpose.
	err := e.ApplyEdits([]Edit{{Start: 8, End: 11, Old: "foo", Text: "x"}, {Start: 0, End: 3, Old: "foo", Text: "yy"}})
	if err != nil || text(e) != "yy bar x" {
		t.Fatalf("got %q, %v", text(e), err)
	}
	if !e.Undo() || text(e) != "foo bar foo" {
		t.Fatalf("one undo should restore everything, got %q", text(e))
	}
	if e.Undo() {
		t.Fatal("there should have been one step")
	}
}

func TestApplyEditsChangesNothingOnMismatchOrOverlap(t *testing.T) {
	cases := map[string][]Edit{
		"old differs": {{Start: 0, End: 3, Old: "foo", Text: "a"}, {Start: 4, End: 7, Old: "nope", Text: "b"}},
		"overlap":     {{Start: 0, End: 5, Old: "foo b", Text: "a"}, {Start: 4, End: 7, Old: "bar", Text: "b"}},
		"outside":     {{Start: 5, End: 99, Old: "", Text: "b"}},
		"negative":    {{Start: -1, End: 2, Old: "", Text: "b"}},
	}
	for name, edits := range cases {
		e := applyEditor(t, "foo bar foo")
		if err := e.ApplyEdits(edits); err == nil {
			t.Errorf("%s: want an error", name)
		}
		if text(e) != "foo bar foo" || e.File.Modified() {
			t.Errorf("%s: text changed to %q", name, text(e))
		}
	}
}

func TestApplyEditsKeepsCursorWithItsText(t *testing.T) {
	e := applyEditor(t, "aa\nbb")
	e.GoTo(1, 1, false)
	if err := e.ApplyEdits([]Edit{{Start: 0, End: 2, Old: "aa", Text: "aaaa"}}); err != nil {
		t.Fatal(err)
	}
	if line, col := e.Position(); line != 1 || col != 1 {
		t.Fatalf("cursor at %d,%d", line, col)
	}
}
