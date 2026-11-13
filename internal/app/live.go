package app

import (
	"maps"
	"path/filepath"
	"time"

	"github.com/Juan17la/lazycode/internal/buffer"
	"github.com/Juan17la/lazycode/internal/marks"
	"github.com/fsnotify/fsnotify"
)

const (
	// liveDelay combines a burst of writes into one update, so the screen stays smooth (doc 06).
	liveDelay = 100 * time.Millisecond
	// marksDelay combines keystrokes into one comparison.
	marksDelay = 150 * time.Millisecond
	// maxMarkedFile is the largest file that gets markers. Comparing costs memory and time, and
	// it matches where highlighting stops mattering most.
	maxMarkedFile = 4 << 20
)

// liveUpdates watches the folders holding open files (Q-53). It watches folders and not files,
// because tools usually save by writing a new file and renaming it over the old one.
type liveUpdates struct {
	watcher *fsnotify.Watcher
	folders map[string]bool
}

func (a *app) startLive() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		a.message = "Open files won't update live: " + err.Error()
		return
	}
	a.live = liveUpdates{watcher: watcher, folders: map[string]bool{}}
	go func() {
		changed := map[string]bool{}
		var fire <-chan time.Time
		for {
			select {
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) != 0 {
					changed[ev.Name] = true
					if fire == nil { // Not reset by later writes, so constant writing still shows.
						fire = time.After(liveDelay)
					}
				}
			case <-fire:
				paths := changed
				changed, fire = map[string]bool{}, nil
				a.post(func() { a.filesChanged(paths) })
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()
}

func (a *app) stopLive() {
	if a.live.watcher != nil {
		a.live.watcher.Close() //nolint:errcheck // The app is stopping.
	}
}

// openFiles returns one tab for each open file, in every directory.
func (a *app) openFiles() map[*buffer.File]*tabItem {
	files := map[*buffer.File]*tabItem{}
	for _, st := range a.dirs {
		for _, p := range st.panes() {
			for _, it := range p.tabs {
				if it.ed != nil {
					files[it.ed.File] = it
				}
			}
		}
	}
	return files
}

// syncLive keeps watching exactly the folders with open files. It runs after every event, which
// also covers every way of opening or closing a file.
func (a *app) syncLive() {
	if a.live.watcher == nil {
		return
	}
	want := map[string]bool{}
	for f := range a.openFiles() {
		want[filepath.Dir(f.Path)] = true
	}
	if maps.Equal(want, a.live.folders) {
		return
	}
	for folder := range a.live.folders {
		if !want[folder] {
			a.live.watcher.Remove(folder) //nolint:errcheck // It may be gone already.
		}
	}
	for folder := range want {
		if !a.live.folders[folder] {
			// A folder that can't be watched still gets noticed on focus, and when saving.
			a.live.watcher.Add(folder) //nolint:errcheck // See above.
		}
	}
	a.live.folders = want
}

// filesChanged updates open files that changed on disk. One with unsaved edits is never replaced
// (doc 06). It compares contents, so LazyCode's own saves do nothing here.
func (a *app) filesChanged(paths map[string]bool) {
	for f, it := range a.openFiles() {
		if !paths[f.Path] {
			continue
		}
		changed, err := f.ChangedOnDisk()
		if err != nil || !changed {
			continue
		}
		switch {
		case f.ReadOnly():
			// Reloading means scanning all of it again, on every append to a growing log.
			a.message = it.name() + " changed on disk. Close and reopen it to see that."
		case f.Modified():
			a.message = it.name() + " changed on disk. Your edits are kept, and saving will ask what to do."
		default:
			if err := it.ed.Reload(); err != nil {
				a.message = it.name() + " changed on disk, and can't be reloaded: " + err.Error()
			} else if m := a.marks[f]; m != nil {
				m.baseKnown = false // Whoever changed it may have committed too.
			}
		}
	}
}

// fileMarks holds which lines of an open file were modified (Q-76).
type fileMarks struct {
	base      []byte
	baseKnown bool
	lines     map[int]bool
	seen      buffer.Snapshot // The text lines describes.
	valid     bool
	pending   bool
}

// marksFor returns the marked lines for drawing, and notices if they're out of date. So anything that
// changes the text is covered: typing, undo, a reload, or a tab restored at startup.
func (a *app) marksFor(f *buffer.File) map[int]bool {
	if f.Len() > maxMarkedFile {
		delete(a.marks, f)
		return nil
	}
	m := a.marks[f]
	if m == nil {
		m = &fileMarks{}
		a.marks[f] = m
	}
	if !m.pending && (!m.valid || !m.seen.Same(f.Snapshot)) {
		m.pending = true
		time.AfterFunc(marksDelay, func() { a.post(func() { a.compareMarks(f, m) }) })
	}
	return m.lines
}

// compareMarks works out the lines on another goroutine, from a Snapshot, which is safe to read there.
func (a *app) compareMarks(f *buffer.File, m *fileMarks) {
	if a.marks[f] != m {
		return // The file has closed, or got too large.
	}
	snapshot := f.Snapshot
	path, base, baseKnown := f.Path, m.base, m.baseKnown
	go func() {
		text := snapshot.Slice(0, snapshot.Len())
		if !baseKnown {
			if committed, inGit := marks.Base(path); inGit {
				base = committed
			} else if base == nil {
				base = text // Outside Git, it's compared with the file as it was when opened (doc 06).
			}
		}
		changed := map[int]bool{}
		for _, line := range marks.Changed(base, text) {
			changed[line] = true
		}
		a.post(func() {
			m.base, m.baseKnown, m.lines, m.seen, m.valid, m.pending = base, true, changed, snapshot, true, false
			a.requestRedraw()
		})
	}()
}
