package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Juan17la/lazycode/internal/buffer"
	"github.com/Juan17la/lazycode/internal/editor"
	"github.com/gdamore/tcell/v2"
)

const (
	// maxReviewRows bounds the preview of a huge replace.
	// ponytail: the changes past it are still applied, just not listed. Paging the list is the upgrade.
	maxReviewRows = 20000
	maxReviewLine = 400
	maxChangeFile = 100 << 20 // The editor's read-only limit: bigger files aren't changed here.
)

type reviewRow struct {
	text  string
	style int // 0 file heading, 1 removed, 2 added, 3 note.
}

// review is the screen that shows what a change will do before it does it.
type review struct {
	title   string
	changes []editor.FileChange
	rows    []reviewRow
	top     int
	ready   bool // The rows are worked out.
	working bool // It was confirmed and is being applied.
	done    func(applied int, failed []string)
}

// reviewChanges shows every change, file by file with the old and new line, and applies them only
// when the user confirms. A file is skipped, and named in failed, if it has unsaved edits, changed on
// disk, or no longer holds the text the change was worked out from, as AI tools edit files all the
// time. Open files change through their editor, so undo works. done runs on the UI goroutine.
func (a *app) reviewChanges(title string, changes []editor.FileChange, done func(applied int, failed []string)) {
	if len(changes) == 0 {
		done(0, nil)
		return
	}
	dir := a.dirPath()
	changes = append([]editor.FileChange(nil), changes...)
	for i := range changes {
		if !filepath.IsAbs(changes[i].Path) {
			changes[i].Path = filepath.Join(dir, changes[i].Path)
		}
		changes[i].Path = filepath.Clean(changes[i].Path)
	}
	unsaved := map[string]bool{}
	for f := range a.openFiles() {
		unsaved[f.Path] = f.Modified()
	}
	r := &review{title: title, changes: changes, done: done}
	a.review = r
	go func() {
		rows := previewRows(dir, changes, unsaved)
		a.post(func() {
			if a.review == r {
				r.rows, r.ready = rows, true
			}
		})
	}()
}

// shown makes text safe and short enough for one screen row.
func shown(text string) string {
	text = strings.ToValidUTF8(text, "?")
	text = strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case r == '\n':
			return '⏎'
		case unicode.IsControl(r):
			return '?'
		}
		return r
	}, text)
	if len(text) > maxReviewLine {
		cut := maxReviewLine
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut] + "…"
	}
	return text
}

// displayPath shows a path relative to the directory when it's inside.
func displayPath(dir, path string) string {
	if inDir(dir, path) {
		if rel, err := filepath.Rel(dir, path); err == nil {
			return rel
		}
	}
	return path
}

// previewRows reads each file as it is on disk and lays out its changes. It runs off the UI goroutine.
func previewRows(dir string, changes []editor.FileChange, unsaved map[string]bool) []reviewRow {
	var rows []reviewRow
	for _, c := range changes {
		name := shown(displayPath(dir, c.Path))
		var content []byte
		var err error
		if info, statErr := os.Stat(c.Path); statErr != nil {
			err = statErr
		} else if info.Size() > maxChangeFile {
			err = errors.New("too large to change here")
		} else {
			content, err = os.ReadFile(c.Path)
		}
		switch {
		case err != nil:
			rows = append(rows, reviewRow{name + "  (can't change: " + shown(err.Error()) + ")", 0})
			continue
		case unsaved[c.Path]:
			rows = append(rows, reviewRow{name + "  (has unsaved edits, so it will be skipped)", 0})
			continue
		}
		rows = append(rows, reviewRow{fmt.Sprintf("%s  (%d changes)", name, len(c.Edits)), 0})
		edits := append([]editor.Edit(nil), c.Edits...)
		slices.SortStableFunc(edits, func(a, b editor.Edit) int { return a.Start - b.Start })
		line, at := 1, 0
		for _, e := range edits {
			if len(rows) >= maxReviewRows {
				rows = append(rows, reviewRow{"…more changes, not listed", 3})
				return rows
			}
			if e.Start < at || e.End < e.Start || e.End > len(content) || string(content[e.Start:e.End]) != e.Old {
				rows = append(rows, reviewRow{"  the text here changed since, so it will be skipped", 3})
				continue
			}
			line += bytes.Count(content[at:e.Start], []byte{'\n'})
			at = e.Start
			ls := bytes.LastIndexByte(content[:e.Start], '\n') + 1
			le := len(content)
			if i := bytes.IndexByte(content[e.End:], '\n'); i >= 0 {
				le = e.End + i
			}
			old := string(content[ls:le])
			fresh := string(content[ls:e.Start]) + e.Text + string(content[e.End:le])
			rows = append(rows,
				reviewRow{fmt.Sprintf("  %4d - %s", line, shown(old)), 1},
				reviewRow{fmt.Sprintf("  %4d + %s", line, shown(fresh)), 2})
		}
	}
	return rows
}

func (a *app) handleReviewKey(ev *tcell.EventKey) {
	r := a.review
	if r.working {
		return
	}
	_, h := a.screen.Size()
	page := max(1, h-5)
	switch ev.Key() {
	case tcell.KeyEsc:
		a.review = nil
		r.done(0, nil)
	case tcell.KeyEnter:
		if r.ready {
			r.working = true
			a.applyChanges(r)
		}
	case tcell.KeyUp:
		r.scroll(-1)
	case tcell.KeyDown:
		r.scroll(1)
	case tcell.KeyPgUp:
		r.scroll(-page)
	case tcell.KeyPgDn:
		r.scroll(page)
	case tcell.KeyHome:
		r.top = 0
	case tcell.KeyEnd:
		r.top = len(r.rows)
		r.scroll(0)
	}
}

func (r *review) scroll(delta int) { r.top = max(0, min(r.top+delta, len(r.rows)-1)) }

// applyChanges changes open files here, where their editors live, and closed ones on another
// goroutine, so a big change doesn't freeze the screen. One failing never stops the others.
func (a *app) applyChanges(r *review) {
	dir := a.dirPath()
	open := map[string]*tabItem{}
	for f, it := range a.openFiles() {
		open[f.Path] = it
	}
	var failed []string
	applied := 0
	var closed []editor.FileChange
	fail := func(path string, err error) {
		failed = append(failed, shown(displayPath(dir, path))+": "+shown(err.Error()))
	}
	for _, c := range r.changes {
		it := open[c.Path]
		if it == nil {
			closed = append(closed, c)
		} else if err := a.changeOpen(it, c); err != nil {
			fail(c.Path, err)
		} else {
			applied++
		}
	}
	finish := func() {
		a.review = nil
		r.done(applied, failed)
	}
	if len(closed) == 0 {
		finish()
		return
	}
	go func() {
		type result struct {
			path string
			err  error
		}
		var results []result
		for _, c := range closed {
			results = append(results, result{c.Path, changeClosed(c)})
		}
		a.post(func() {
			for _, res := range results {
				if res.err != nil {
					fail(res.path, res.err)
				} else {
					applied++
				}
			}
			finish()
		})
	}()
}

// changeOpen goes through the file's editor, so Ctrl+Z undoes it, and saves it the way Ctrl+S does.
// Unsaved edits are never saved over: the user decides about those.
func (a *app) changeOpen(it *tabItem, c editor.FileChange) error {
	f := it.ed.File
	switch {
	case f.Modified():
		return errors.New("has unsaved edits, so it was skipped")
	case f.ReadOnly():
		return errors.New("is read-only, so it was skipped")
	}
	if changed, err := f.ChangedOnDisk(); err != nil || changed {
		return errors.New("changed on disk, so it was skipped")
	}
	if err := it.ed.ApplyEdits(c.Edits); err != nil {
		return fmt.Errorf("changed since it was searched, so it was skipped (%w)", err)
	}
	if err := f.Save(); err != nil {
		it.ed.Undo() // Nothing half done: the editor shows the file as it is on disk.
		return fmt.Errorf("can't be saved, so it was left as it was (%w)", err)
	}
	a.saved(it)
	return nil
}

// changeClosed is changeOpen for a file nobody has open, using the same safe write.
func changeClosed(c editor.FileChange) error {
	f, err := buffer.Open(c.Path)
	if err != nil {
		return fmt.Errorf("can't be opened, so it was skipped (%w)", err)
	}
	defer f.Close() //nolint:errcheck // Only frees memory.
	if f.ReadOnly() {
		return errors.New("is too large to change here, so it was skipped")
	}
	if err := editor.ApplyToFile(f, c.Edits); err != nil {
		return fmt.Errorf("changed since it was searched, so it was skipped (%w)", err)
	}
	if err := f.Save(); err != nil {
		return fmt.Errorf("can't be saved, so it was left as it was (%w)", err)
	}
	return nil
}

func (a *app) drawReview(w, h int) {
	r := a.review
	y0, bottom := 1, h-1
	for y := y0; y < bottom; y++ {
		for x := range w {
			a.screen.SetContent(x, y, ' ', nil, a.th.primary)
		}
	}
	a.put(1, y0, w, r.title, a.th.accent)
	rows := max(1, bottom-y0-2)
	switch {
	case !r.ready:
		a.put(1, y0+2, w, "Reading the files...", a.th.muted)
	case len(r.rows) == 0:
		a.put(1, y0+2, w, "Nothing to change.", a.th.muted)
	}
	for i := 0; i < rows && r.top+i < len(r.rows); i++ {
		row := r.rows[r.top+i]
		style := map[int]tcell.Style{0: a.th.accent, 1: a.th.muted, 2: a.th.primary, 3: a.th.muted}[row.style]
		a.put(1, y0+2+i, w, row.text, style)
	}
	foot := "Enter applies   Esc cancels   Up and Down scroll"
	switch {
	case r.working:
		foot = "Applying..."
	case !r.ready: // Enter does nothing yet, so it isn't offered.
		foot = "Esc cancels"
	}
	a.put(1, bottom-1, w, foot, a.th.accent)
	a.hits = append(a.hits[:0], hit{
		area:  rect{0, 0, w, h},
		click: func(int, int, tcell.ModMask) {},
		wheel: func(delta int) { r.scroll(delta * wheelLines) },
	})
}
