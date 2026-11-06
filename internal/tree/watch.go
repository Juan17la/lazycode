package tree

import (
	"errors"
	"io/fs"
	"slices"
	"time"

	"github.com/fsnotify/fsnotify"
)

// watchDelay combines a burst of changes, such as an AI tool creating many files.
const watchDelay = 100 * time.Millisecond

// Watcher reports when files appear, go or get renamed in the folders it's told about.
// Writes to a file don't change the tree, so they're ignored.
// ponytail: on macOS, kqueue holds one descriptor per file in a watched folder, so a huge
// open folder can run out. The error reaches the user, and refreshing by hand still works.
type Watcher struct {
	fs *fsnotify.Watcher
}

// Watch starts watching nothing yet. onChange runs on the watcher's own goroutine.
func Watch(onChange func()) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	go func() {
		var wait <-chan time.Time
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if wait == nil && ev.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
					wait = time.After(watchDelay)
				}
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
				// Usually an overflow, meaning that events were lost. Look again.
				if wait == nil {
					wait = time.After(watchDelay)
				}
			case <-wait:
				wait = nil
				onChange()
			}
		}
	}()
	return &Watcher{w}, nil
}

// Set makes these the watched folders. A folder that's already gone isn't a problem. Any
// other failure is returned, after trying the rest.
func (w *Watcher) Set(folders []string) error {
	current := w.fs.WatchList()
	for _, path := range current {
		if !slices.Contains(folders, path) {
			w.fs.Remove(path) //nolint:errcheck // It's already gone, if this fails.
		}
	}
	var problem error
	for _, path := range folders {
		if slices.Contains(current, path) {
			continue
		}
		if err := w.fs.Add(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			problem = err
		}
	}
	return problem
}

// Close stops watching.
func (w *Watcher) Close() error { return w.fs.Close() }
