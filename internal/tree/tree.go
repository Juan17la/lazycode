// Package tree is the file tree's model: which folders are open, and what is selected.
package tree

import (
	"os"
	"path/filepath"
	"sort"
)

// Row is one visible line in the tree.
type Row struct {
	Path  string
	Name  string
	Depth int
	IsDir bool
	Open  bool
}

// Tree holds one directory's tree. Folders load only when opened.
// ponytail: reads on the UI goroutine and refreshes by hand; M2's A1 moves it to a
// background goroutine with fsnotify, once huge folders or AI edits need it.
type Tree struct {
	Root string
	Err  error // Why Root couldn't be read, if it couldn't.
	Rows []Row
	Sel  int

	open map[string]bool
}

// New loads the top level of root.
func New(root string) *Tree {
	t := &Tree{Root: root, open: map[string]bool{}}
	t.Refresh()
	return t
}

// Refresh reloads every open folder from disk, keeping the selection in range.
func (t *Tree) Refresh() {
	t.Rows = t.Rows[:0]
	t.Err = t.add(t.Root, 0)
	t.Move(0)
}

func (t *Tree) add(dir string, depth int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].IsDir() && !entries[j].IsDir() // ReadDir already sorted by name.
	})
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		// A symlink's IsDir is false, so link loops can't be followed.
		row := Row{Path: path, Name: e.Name(), Depth: depth, IsDir: e.IsDir(), Open: t.open[path]}
		t.Rows = append(t.Rows, row)
		if row.Open {
			if err := t.add(path, depth+1); err != nil {
				// An unreadable subfolder stays visible but closed.
				t.Rows[len(t.Rows)-1].Open = false
				delete(t.open, path)
			}
		}
	}
	return nil
}

// Move shifts the selection, stopping at either end.
func (t *Tree) Move(delta int) {
	t.Sel = max(0, min(t.Sel+delta, len(t.Rows)-1))
}

// Selected returns the selected row, if there is one.
func (t *Tree) Selected() (Row, bool) {
	if t.Sel < len(t.Rows) {
		return t.Rows[t.Sel], true
	}
	return Row{}, false
}

// SetOpen opens or closes the selected folder. Files are left alone.
func (t *Tree) SetOpen(open bool) {
	r, ok := t.Selected()
	if !ok || !r.IsDir {
		return
	}
	if open {
		t.open[r.Path] = true
	} else {
		delete(t.open, r.Path)
	}
	t.Refresh()
}
