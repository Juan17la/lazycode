// Package tree is the file tree's model: which folders are open, and what is selected.
package tree

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
)

// Row is one visible line in the tree.
type Row struct {
	Path    string
	Name    string
	Depth   int
	IsDir   bool
	Open    bool
	Missing bool // A combined view's directory that can't be found (Q-66).
}

// Tree holds one directory's tree, or all of a project's in a combined view (Q-41).
// Folders load only when opened.
// ponytail: reads on the UI goroutine, so a folder with 100k+ files stalls a frame when
// opened or changed. Move Refresh off the UI goroutine if that shows up.
type Tree struct {
	Root  string
	Roots []string // Set for a combined view, with one top row for each.
	Err   error    // Why Root couldn't be read, if it couldn't.
	Rows  []Row
	Sel   int

	open    map[string]bool
	folders []string // What's on screen and could change, for the watcher.
}

// New loads the top level of root.
func New(root string) *Tree {
	t := &Tree{Root: root, open: map[string]bool{}}
	t.Refresh()
	return t
}

// NewCombined shows several directories in one tree, each starting open.
func NewCombined(roots []string) *Tree {
	t := &Tree{Roots: slices.Clone(roots), open: map[string]bool{}}
	for _, root := range roots {
		t.open[root] = true
	}
	t.Refresh()
	return t
}

// Refresh reloads every open folder from disk. The selection stays on the same path, so
// that a file appearing above it can't move it. If that path has gone, it stays in range.
func (t *Tree) Refresh() {
	selected, hadSelection := t.Selected()
	t.Rows, t.folders = t.Rows[:0], t.folders[:0]
	if t.Roots == nil {
		t.Err = t.add(t.Root, 0)
	}
	for _, root := range t.Roots {
		t.addRoot(root)
	}
	if hadSelection {
		if i := slices.IndexFunc(t.Rows, func(r Row) bool { return r.Path == selected.Path }); i >= 0 {
			t.Sel = i
		}
	}
	t.Move(0)
}

func (t *Tree) addRoot(root string) {
	_, err := os.Stat(root)
	row := Row{Path: root, Name: filepath.Base(root), IsDir: true, Open: t.open[root] && err == nil, Missing: err != nil}
	t.Rows = append(t.Rows, row)
	i := len(t.Rows) - 1
	if row.Open && t.add(root, 1) != nil {
		t.Rows[i].Open = false // It stays marked open, in case it comes back.
	}
}

func (t *Tree) add(dir string, depth int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	t.folders = append(t.folders, dir)
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
		i := len(t.Rows) - 1
		if row.Open {
			if err := t.add(path, depth+1); err != nil {
				// An unreadable subfolder stays visible but closed.
				t.Rows[i].Open = false
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

// OpenFolders lists the open folders, in order, for saving.
func (t *Tree) OpenFolders() []string {
	out := make([]string, 0, len(t.open))
	for path := range t.open {
		out = append(out, path)
	}
	slices.Sort(out)
	return out
}

// Reopen opens these folders, as saved by OpenFolders. Any that have gone are dropped.
func (t *Tree) Reopen(folders []string) {
	for _, path := range folders {
		t.open[path] = true
	}
	t.Refresh()
}

// Folders lists the folders being shown. It's valid until the next Refresh.
func (t *Tree) Folders() []string { return t.folders }
