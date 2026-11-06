package app

import (
	"fmt"
	"sync"
	"time"

	"github.com/Juan17la/lazycode/internal/buffer"
	"github.com/Juan17la/lazycode/internal/syntax"
)

const (
	// maxHighlighted is the largest file that gets colours: memory scales with size, and the first parse
	// of a megabyte takes about 260 ms (Q-69, doc 12).
	maxHighlighted = 1 << 20
	// highlightDelay combines a burst of typing into one reparse.
	highlightDelay = 40 * time.Millisecond
)

// highlightState is the colouring of one file, however many views show it. Parsing happens on its
// own goroutine, one at a time, and the drawing only reads finished spans on the UI goroutine (P-01).
type highlightState struct {
	file *buffer.File
	lang *syntax.Lang
	h    *syntax.Highlighter // Used by the parsing goroutine while busy, and otherwise by the UI.

	spans []syntax.Span
	seen  buffer.Snapshot // The text the spans describe.
	valid bool

	busy    bool // A parse is running.
	pending bool // A parse is waiting for typing to pause.
	drop    bool // Free the tree when the running parse ends: the tab was hidden.
	closed  bool // Close the highlighter when the running parse ends.
	failed  bool // It can't be done for this file, so it isn't retried.
}

type highlighting struct {
	files map[*buffer.File]*highlightState
	all   map[*highlightState]bool // Includes closed ones that are still parsing, so that stopping frees them.
	wg    sync.WaitGroup           // Parses in flight.
}

// highlightSpans returns the finished spans of a file, which may be a little behind the text.
func (a *app) highlightSpans(f *buffer.File) []syntax.Span {
	if s := a.hl.files[f]; s != nil {
		return s.spans
	}
	return nil
}

// syncHighlight runs after every event, like syncLive, so that anything that changes the text
// is covered: typing, undo, a live reload, a restored tab. Comparing Snapshots costs a pointer.
// Trees exist only for files on screen (doc 06) and go when a tab is hidden or closed.
func (a *app) syncHighlight() {
	hl := &a.hl
	if hl.files == nil {
		hl.files, hl.all = map[*buffer.File]*highlightState{}, map[*highlightState]bool{}
	}
	shown := map[*buffer.File]bool{}
	for _, p := range a.dir().root.panes() {
		if it := p.current(); it != nil && it.ed != nil {
			shown[it.ed.File] = true
		}
	}
	if len(hl.files) > 0 {
		open := a.openFiles()
		for f, s := range hl.files {
			if _, ok := open[f]; !ok {
				a.closeHighlight(s)
			} else if !shown[f] {
				a.hideHighlight(s)
			}
		}
	}
	for f := range shown {
		a.syncFileHighlight(f)
	}
}

func (a *app) syncFileHighlight(f *buffer.File) {
	lang := syntax.ForPath(f.Path)
	s := a.hl.files[f]
	if lang == nil || f.ReadOnly() || f.Len() > maxHighlighted {
		if s != nil { // It grew past the limit.
			a.closeHighlight(s)
		}
		return
	}
	if s == nil {
		s = &highlightState{file: f, lang: lang}
		a.hl.files[f], a.hl.all[s] = s, true
	}
	s.drop = false
	if s.failed || s.busy || s.pending {
		return
	}
	if s.valid && s.seen.Same(f.Snapshot) {
		return
	}
	if !s.valid {
		a.startHighlight(s) // Nothing is coloured yet, so there's nothing to wait for.
		return
	}
	s.pending = true
	time.AfterFunc(highlightDelay, func() {
		a.post(func() {
			s.pending = false
			if !s.closed && !s.busy {
				a.startHighlight(s)
			}
		})
	})
}

// startHighlight parses the file's current text on another goroutine.
func (a *app) startHighlight(s *highlightState) {
	snap := s.file.Snapshot
	s.busy = true
	a.hl.wg.Add(1)
	go func() {
		spans, err := s.parse(snap)
		a.hl.wg.Done() // Before posting, so that stopping never waits on a UI that's gone.
		a.post(func() { a.highlighted(s, snap, spans, err) })
	}()
}

// parse runs on the worker. A panic here must not take down the editor and its unsaved work.
func (s *highlightState) parse(snap buffer.Snapshot) (spans []syntax.Span, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	if s.h == nil {
		if s.h, err = syntax.NewHighlighter(s.lang); err != nil {
			return nil, err
		}
	}
	return s.h.Update(snap.Slice(0, snap.Len()))
}

func (a *app) highlighted(s *highlightState, snap buffer.Snapshot, spans []syntax.Span, err error) {
	s.busy = false
	switch {
	case s.closed:
		s.h.Close()
		delete(a.hl.all, s)
		go syntax.ReleaseMemory()
	case err != nil:
		s.failed = true
		a.message = "Colours are off for " + a.nameOf(s.file) + ": " + err.Error()
	case s.drop:
		s.drop = false
		s.h.Drop()
		go syntax.ReleaseMemory()
	default:
		s.spans, s.seen, s.valid = spans, snap, true
		a.requestRedraw()
	}
}

func (a *app) nameOf(f *buffer.File) string {
	if it := a.openFiles()[f]; it != nil {
		return it.name()
	}
	return f.Path
}

// hideHighlight frees the tree and spans of a tab that isn't showing. They're rebuilt when it is.
func (a *app) hideHighlight(s *highlightState) {
	if s.busy {
		s.drop = true
		return
	}
	if s.valid {
		s.h.Drop() // Freeing a tree isn't a parse, and takes well under a millisecond.
		s.spans, s.valid = nil, false
		go syntax.ReleaseMemory()
	}
}

// closeHighlight forgets a file, as its last tab closes. A running parse finishes first, because the
// parser can't be freed from under it.
func (a *app) closeHighlight(s *highlightState) {
	s.closed = true
	delete(a.hl.files, s.file)
	if !s.busy {
		s.h.Close()
		delete(a.hl.all, s)
		go syntax.ReleaseMemory()
	}
}

// stopHighlight frees every parser as the app stops, waiting for parses that are running.
func (a *app) stopHighlight() {
	a.hl.wg.Wait()
	for s := range a.hl.all {
		s.h.Close()
	}
	if len(a.hl.all) > 0 {
		syntax.ReleaseMemory()
	}
	clear(a.hl.all)
	clear(a.hl.files)
}
