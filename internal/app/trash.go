package app

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"
)

// A delete from the file tree moves the file here, so Ctrl+Z can bring it back. Each run has its own
// folder, emptied on quit. One left by a crash is kept for a while, as a last way to get files back.
const trashKeptAfterCrash = 7 * 24 * time.Hour

type trashedFile struct{ from, to string }

func trashRoot() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "lazycode", "trash"), nil
}

// deleteSelected is the Delete key in the file tree.
func (a *app) deleteSelected() {
	t := a.tree()
	if t.Sel < 0 || t.Sel >= len(t.Rows) {
		return
	}
	path := t.Rows[t.Sel].Path
	if slices.Contains(a.projects[a.project].Dirs, path) {
		a.message = "Remove this folder from the project with :dir."
		return
	}
	a.deleteToTrash(path)
}

func (a *app) deleteToTrash(path string) {
	if a.openInside(path) {
		a.message = "Close the tabs in " + filepath.Base(path) + " first."
		return
	}
	if a.trashDir == "" {
		root, err := trashRoot()
		if err == nil {
			err = os.MkdirAll(root, 0o700)
		}
		if err == nil {
			a.trashDir, err = os.MkdirTemp(root, "run-")
		}
		if err != nil {
			a.message = "Can't delete: " + err.Error()
			return
		}
	}
	to := filepath.Join(a.trashDir, strconv.Itoa(len(a.trashed)))
	if err := moveFiles(path, to); err != nil {
		a.message = "Can't delete: " + err.Error()
		return
	}
	a.trashed = append(a.trashed, trashedFile{path, to})
	a.tabFocused = false // Ctrl+Z only reaches the tree from here, not the file that was showing.
	a.message = "Deleted " + filepath.Base(path) + ". Ctrl+Z brings it back."
	a.tree().Refresh()
}

// undoDelete brings back the last deleted file. If that fails it stays in the trash to try again.
func (a *app) undoDelete() {
	if len(a.trashed) == 0 {
		a.message = "Nothing deleted to bring back."
		return
	}
	last := a.trashed[len(a.trashed)-1]
	name := filepath.Base(last.from)
	if _, err := os.Lstat(last.from); err == nil {
		a.message = "Can't bring back " + name + ": something else has that name now."
		return
	}
	err := os.MkdirAll(filepath.Dir(last.from), 0o755)
	if err == nil {
		err = moveFiles(last.to, last.from)
	}
	if err != nil {
		a.message = "Can't bring back " + name + ": " + err.Error()
		return
	}
	a.trashed = a.trashed[:len(a.trashed)-1]
	a.message = "Brought back " + name + "."
	a.tree().Refresh()
}

func (a *app) emptyTrash() {
	if a.trashDir != "" {
		os.RemoveAll(a.trashDir) //nolint:errcheck // Cleaned by a later start otherwise.
	}
}

// cleanOldTrash leaves newer folders alone, because they may belong to another running LazyCode.
func cleanOldTrash() {
	root, err := trashRoot()
	if err != nil {
		return
	}
	entries, _ := os.ReadDir(root) //nolint:errcheck // Missing is the usual case.
	for _, entry := range entries {
		if info, err := entry.Info(); err == nil && time.Since(info.ModTime()) > trashKeptAfterCrash {
			os.RemoveAll(filepath.Join(root, entry.Name())) //nolint:errcheck // Tried again next start.
		}
	}
}

// moveFiles renames, and copies when from and to are on different drives. The original goes only
// after the whole copy has worked.
// ponytail: a big folder on another drive blocks the UI while it copies; move it off the UI goroutine if that bites.
func moveFiles(from, to string) error {
	if os.Rename(from, to) == nil {
		return nil
	}
	if err := copyFiles(from, to); err != nil {
		return errors.Join(err, os.RemoveAll(to))
	}
	return os.RemoveAll(from)
}

func copyFiles(from, to string) error {
	return filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case info.Mode()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case !info.Mode().IsRegular():
			return errors.New(path + " isn't a regular file")
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(from, to string, mode fs.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck // Only read.
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close() //nolint:errcheck // The copy error is the one to report.
		return err
	}
	return out.Close()
}
