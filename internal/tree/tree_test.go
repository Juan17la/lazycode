package tree

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func names(t *Tree) []string {
	var out []string
	for _, r := range t.Rows {
		out = append(out, r.Name)
	}
	return out
}

func TestTree(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"b.txt", "zdir/inner.txt", "adir/x.txt", ".git/config"} {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tr := New(root)
	if want := []string{"adir", "zdir", "b.txt"}; !slices.Equal(names(tr), want) {
		t.Fatalf("folders first, no .git: got %v, want %v", names(tr), want)
	}

	tr.Move(1)
	tr.SetOpen(true)
	if want := []string{"adir", "zdir", "inner.txt", "b.txt"}; !slices.Equal(names(tr), want) {
		t.Fatalf("after opening: got %v, want %v", names(tr), want)
	}
	if tr.Rows[2].Depth != 1 {
		t.Errorf("inner.txt depth = %d, want 1", tr.Rows[2].Depth)
	}

	tr.Move(100)
	if r, _ := tr.Selected(); r.Name != "b.txt" {
		t.Errorf("Move past the end selected %q", r.Name)
	}
	tr.SetOpen(true) // A file: nothing happens.
	if len(tr.Rows) != 4 {
		t.Errorf("opening a file changed the rows: %v", names(tr))
	}

	tr.Move(-100)
	tr.Move(1)
	tr.SetOpen(false)
	if want := []string{"adir", "zdir", "b.txt"}; !slices.Equal(names(tr), want) {
		t.Fatalf("after closing: got %v, want %v", names(tr), want)
	}
}

func TestDeletedFilesLeaveSelectionInRange(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "only.txt")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tr := New(root)
	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}
	tr.Refresh()
	if _, ok := tr.Selected(); ok {
		t.Error("an empty tree must have no selection")
	}
}

func TestMissingRootReportsAnError(t *testing.T) {
	tr := New(filepath.Join(t.TempDir(), "gone"))
	if tr.Err == nil {
		t.Error("want an error for a missing directory")
	}
	if _, ok := tr.Selected(); ok {
		t.Error("a missing directory must have no selection")
	}
}
