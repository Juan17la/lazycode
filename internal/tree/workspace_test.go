package tree

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSelectionFollowsItsPathWhenFilesAppear(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "m.txt"))
	tr := New(root)
	touch(t, filepath.Join(root, "a.txt"))
	tr.Refresh()
	if row, _ := tr.Selected(); row.Name != "m.txt" {
		t.Errorf("selection moved to %q", row.Name)
	}
}

func TestCombinedShowsEveryDirectoryAndMarksMissingOnes(t *testing.T) {
	root := t.TempDir()
	one, gone := filepath.Join(root, "one"), filepath.Join(root, "gone")
	if err := os.Mkdir(one, 0o755); err != nil {
		t.Fatal(err)
	}
	touch(t, filepath.Join(one, "a.txt"))
	tr := NewCombined([]string{one, gone})
	if got := names(tr); !slices.Equal(got, []string{"one", "a.txt", "gone"}) {
		t.Fatalf("rows: %v", got)
	}
	if tr.Rows[0].Missing || !tr.Rows[2].Missing || tr.Rows[2].Open {
		t.Errorf("rows: %+v", tr.Rows)
	}
	if tr.Rows[1].Depth != 1 {
		t.Errorf("a file under a directory is at depth %d", tr.Rows[1].Depth)
	}
	// It comes back by itself, as an unplugged drive would.
	if err := os.Mkdir(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	tr.Refresh()
	if tr.Rows[2].Missing || !tr.Rows[2].Open {
		t.Errorf("after coming back: %+v", tr.Rows[2])
	}
}

func TestOpenFoldersRoundTrip(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	touch(t, filepath.Join(sub, "a.txt"))
	first := New(root)
	first.SetOpen(true)
	again := New(root)
	again.Reopen(append(first.OpenFolders(), filepath.Join(root, "deleted")))
	if got := names(again); !slices.Equal(got, []string{"sub", "a.txt"}) {
		t.Errorf("rows: %v", got)
	}
}

func TestWatcherReportsNamesChangingButNotWrites(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	touch(t, path)
	changed := make(chan struct{}, 10)
	w, err := Watch(func() { changed <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.Set([]string{root, filepath.Join(root, "gone")}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
		t.Fatal("a write changed the tree")
	case <-time.After(3 * watchDelay):
	}
	touch(t, filepath.Join(root, "b.txt"))
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("a new file wasn't reported")
	}
	if err := w.Set(nil); err != nil {
		t.Fatal(err)
	}
	touch(t, filepath.Join(root, "c.txt"))
	select {
	case <-changed:
		t.Fatal("reported a folder no longer watched")
	case <-time.After(3 * watchDelay):
	}
}
