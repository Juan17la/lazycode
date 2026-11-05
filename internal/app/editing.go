package app

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/Juan17la/lazycode/internal/buffer"
	"github.com/Juan17la/lazycode/internal/clipboard"
	"github.com/Juan17la/lazycode/internal/editor"
	"github.com/Juan17la/lazycode/internal/term"
	"github.com/gdamore/tcell/v2"
)

// autoSaveDelay is the wait after typing stops, when auto-save is on (Q-16).
const autoSaveDelay = time.Second

// tabItem is a tab, holding either a terminal or a view of a file.
type tabItem struct {
	term *term.Tab
	w, h int // The size the terminal was last told.
	ed   *editor.Editor
	sess *sessionEntry // Set if it's listed in the AI sessions view.
	play bool          // Started by :play.
}

// marker tells a terminal tab apart from a file, and changes while output is arriving (Q-25): the
// filled shape means active.
func (it *tabItem) marker() string {
	active := time.Since(it.term.LastOutput()) < activeOutput
	switch {
	case it.sess != nil && !it.sess.isShell && active:
		return "✦"
	case it.sess != nil && !it.sess.isShell:
		return "✧"
	case active:
		return "▶"
	}
	return "▷"
}

func (it *tabItem) name() string {
	if it.ed != nil {
		return filepath.Base(it.ed.File.Path)
	}
	return it.term.Name
}

// Opening files.

// openSelected opens the tree's selection: a folder expands, and a file gets a tab.
func (a *app) openSelected() {
	t := a.tree()
	row, ok := t.Selected()
	switch {
	case !ok:
	case row.IsDir:
		t.SetOpen(!row.Open)
	default:
		if a.showAll { // Tabs belong to directories, so the file's directory comes first.
			for d, dir := range a.projects[a.project].Dirs {
				if d != a.dirOf[a.project] && inDir(dir, row.Path) {
					a.dirOf[a.project] = d
					a.afterSwitch()
					break
				}
			}
		}
		a.openFile(a.dir(), row.Path)
	}
}

// openFile opens a file in the focused pane. An already open file isn't opened twice:
// it says so, and goes there (Q-15).
func (a *app) openFile(st *dirState, path string) {
	for _, p := range st.root.panes() {
		for i, it := range p.tabs {
			if it.ed != nil && it.ed.File.Path == path {
				st.focus, p.cur = p, i
				a.focusTab()
				if a.message == "" {
					a.message = it.name() + " is already open."
				}
				return
			}
		}
	}
	f, err := buffer.Open(path)
	if err != nil {
		a.message = openError(path, err)
		return
	}
	a.addEditor(st, f)
	if f.ReadOnly() {
		a.message = filepath.Base(path) + " is over 100 MB, so it's read-only, and has no syntax colours."
	}
}

func openError(path string, err error) string {
	name := filepath.Base(path)
	switch {
	case errors.Is(err, buffer.ErrBinary):
		return name + " is a binary file, so it can't be opened."
	case errors.Is(err, buffer.ErrNotRegular):
		return name + " isn't a regular file, so it can't be opened."
	}
	return "Can't open " + name + ": " + err.Error()
}

func (a *app) addEditor(st *dirState, f *buffer.File) *tabItem {
	it := &tabItem{ed: editor.New(f, a.cfg.Editor.TabSize)}
	st.focus.tabs = append(st.focus.tabs, it)
	st.focus.cur = len(st.focus.tabs) - 1
	a.tabFocused = true
	return it
}

// focusTab moves focus to the current tab. Until live updates arrive in D5, this is
// when a file's changes on disk get noticed, such as after an AI tool edits it (P-18).
func (a *app) focusTab() {
	it := a.tab()
	if it == nil {
		return
	}
	a.tabFocused = true
	if it.ed == nil {
		return
	}
	changed, err := it.ed.File.ChangedOnDisk()
	switch {
	case err != nil:
		a.message = "Can't check " + it.name() + ": " + err.Error()
	case !changed:
	case it.ed.File.Modified():
		a.message = it.name() + " changed on disk. Your edits are kept, and saving will ask what to do."
	default:
		if err := it.ed.Reload(); err != nil {
			a.message = it.name() + " changed on disk, and can't be reloaded: " + err.Error()
		} else {
			a.message = it.name() + " changed on disk, and was reloaded."
		}
	}
}

// edited keeps the recovery file and auto-save in step with the text.
func (a *app) edited(it *tabItem) {
	f := it.ed.File
	if !f.Modified() {
		a.recovery.Discard(f.Path) // Undone back to what's saved.
		return
	}
	a.recovery.Schedule(f.Path, f.Snapshot)
	if a.cfg.Editor.AutoSave {
		if timer := a.autoSaves[f]; timer != nil {
			timer.Stop()
		}
		a.autoSaves[f] = time.AfterFunc(autoSaveDelay, func() { a.post(func() { a.autoSave(f) }) })
	}
}

// autoSave never overwrites a change made on disk, and never asks. The edits stay unsaved
// and protected by the recovery file, until the user saves and chooses.
func (a *app) autoSave(f *buffer.File) {
	delete(a.autoSaves, f)
	// Closing a tab with "Don't save" must not be undone by a timer still waiting.
	if !f.Modified() || !a.isOpen(f) {
		return
	}
	if err := f.Save(); err != nil {
		a.message = "Not auto-saved: " + err.Error()
		return
	}
	a.recovery.Discard(f.Path)
	a.savedSettings(f)
}

// Clipboard. The tools can block, so they run in order on their own goroutine.

func (a *app) copyText(text string) {
	a.register = text
	a.clipboardJobs <- func() {
		if err := clipboard.Write(text); err != nil {
			// No tool, as over SSH: ask the terminal to set it instead (OSC 52).
			a.post(func() { a.screen.SetClipboard([]byte(text)) })
		}
	}
}

func (a *app) pasteFromClipboard(it *tabItem) {
	a.clipboardJobs <- func() {
		text, err := clipboard.Read()
		a.post(func() {
			if err != nil {
				text = a.register // At least what was copied in this window.
			}
			if a.tab() == it {
				a.pasteInto(it, text)
			}
		})
	}
}

func (a *app) pasteInto(it *tabItem, text string) {
	if text != "" {
		a.editWith(it, func() error { return it.ed.Paste(text) })
	}
}

// Saving and closing.

// save writes the file, then runs then. If it can't, then doesn't run.
func (a *app) save(it *tabItem, then func()) {
	err := it.ed.File.Save()
	switch {
	case err == nil:
		a.saved(it)
		then()
	case errors.Is(err, buffer.ErrChangedOnDisk):
		a.ask(fmt.Sprintf("%s changed on disk.", it.name()),
			choice{'o', "Overwrite", func() {
				if err := it.ed.File.Overwrite(); err != nil {
					a.message = "Can't save " + it.name() + ": " + err.Error()
					return
				}
				a.saved(it)
				then()
			}},
			choice{'r', "Reload, losing edits", func() {
				if err := it.ed.Reload(); err != nil {
					a.message = "Can't reload " + it.name() + ": " + err.Error()
					return
				}
				a.recovery.Discard(it.ed.File.Path)
				a.message = "Reloaded " + it.name()
				then()
			}},
			choice{'c', "Cancel", func() {}})
	default:
		a.message = "Can't save " + it.name() + ": " + err.Error()
	}
}

func (a *app) saved(it *tabItem) {
	a.recovery.Discard(it.ed.File.Path)
	a.message = "Saved " + it.name()
	a.savedSettings(it.ed.File)
}

// savedSettings applies LazyCode's own config files as soon as they're saved.
func (a *app) savedSettings(f *buffer.File) {
	if f.Path == a.settingsPath("config.toml") || f.Path == a.settingsPath("keys.toml") {
		a.loadSettings()
	}
	if f.Path == a.settingsPath("projects.toml") {
		a.reloadProjects()
	}
}

func (a *app) isOpen(f *buffer.File) bool {
	for _, st := range a.dirs {
		for _, p := range st.root.panes() {
			for _, it := range p.tabs {
				if it.ed != nil && it.ed.File == f {
					return true
				}
			}
		}
	}
	return false
}

// lastView reports whether no other tab shows its file, across every directory.
func (a *app) lastView(it *tabItem) bool {
	for _, st := range a.dirs {
		for _, p := range st.root.panes() {
			for _, other := range p.tabs {
				if other != it && other.ed != nil && other.ed.File == it.ed.File {
					return false
				}
			}
		}
	}
	return true
}

// closeTab closes the focused tab, asking first about unsaved work (D-29). Closing one
// view of a file loses nothing while another view is open.
func (a *app) closeTab() {
	st := a.dir()
	p := st.focus
	it := p.current()
	switch {
	case it == nil && st.root.pane == nil: // An empty split.
		st.root = st.root.without(p)
		st.focus = st.root.panes()[0]
		a.tabFocused = false
	case it == nil:
	case it.ed == nil || !it.ed.File.Modified() || !a.lastView(it):
		a.removeTab(st, p, it)
	default:
		a.ask(fmt.Sprintf("Save changes to %s?", it.name()),
			choice{'s', "Save", func() { a.save(it, func() { a.removeTab(st, p, it) }) }},
			choice{'d', "Don't save", func() {
				a.recovery.Discard(it.ed.File.Path)
				a.removeTab(st, p, it)
			}},
			choice{'c', "Cancel", func() {}})
	}
}

// askQuit quits once each file with unsaved work has been decided on. Nothing is thrown
// away until then, so cancelling part way loses nothing.
func (a *app) askQuit(discard map[*buffer.File]bool) {
	a.askUnsaved(func(string) bool { return true }, "before quitting", discard, func() { a.quit = true })
}

// askUnsaved asks about each modified file in the directories inScope selects, and runs then once
// none is left undecided. A file that's also open outside the scope loses nothing, so it isn't asked.
func (a *app) askUnsaved(inScope func(dir string) bool, when string, discard map[*buffer.File]bool, then func()) {
	for dir, st := range a.dirs {
		if !inScope(dir) {
			continue
		}
		for _, p := range st.root.panes() {
			for _, it := range p.tabs {
				if it.ed == nil || !it.ed.File.Modified() || discard[it.ed.File] || a.shownOutside(it.ed.File, inScope) {
					continue
				}
				a.ask(fmt.Sprintf("Save changes to %s %s?", it.name(), when),
					choice{'s', "Save", func() { a.save(it, func() { a.askUnsaved(inScope, when, discard, then) }) }},
					choice{'d', "Don't save", func() {
						discard[it.ed.File] = true
						a.askUnsaved(inScope, when, discard, then)
					}},
					choice{'c', "Cancel", func() {}})
				return
			}
		}
	}
	for f := range discard {
		a.recovery.Discard(f.Path)
	}
	then()
}

// shownOutside reports whether a tab outside the scope shows the file.
func (a *app) shownOutside(f *buffer.File, inScope func(dir string) bool) bool {
	for dir, st := range a.dirs {
		if inScope(dir) {
			continue
		}
		for _, p := range st.root.panes() {
			for _, it := range p.tabs {
				if it.ed != nil && it.ed.File == f {
					return true
				}
			}
		}
	}
	return false
}

// Recovery.

// offerRecovery asks about work left behind by a crash.
func (a *app) offerRecovery() {
	found, err := a.recovery.List()
	if err != nil {
		a.message = "Some recovery files can't be read: " + err.Error()
	}
	for _, rec := range found {
		a.ask(fmt.Sprintf("Unsaved work from %s: %s in %s", rec.Time.Format("Jan 2 15:04"), filepath.Base(rec.Path), filepath.Dir(rec.Path)),
			choice{'r', "Restore", func() { a.restore(rec) }},
			choice{'d', "Delete", func() { a.recovery.Discard(rec.Path) }},
			choice{'l', "Later", func() {}})
	}
}

// restore opens a recovered file with the saved text as unsaved edits. The recovery file
// stays until those edits are saved or discarded.
func (a *app) restore(rec editor.Recovered) {
	text, err := rec.Read()
	if err != nil {
		a.message = "Can't read the recovery file: " + err.Error()
		return
	}
	f, err := buffer.Open(rec.Path)
	if errors.Is(err, fs.ErrNotExist) {
		f, err = buffer.Create(rec.Path)
	}
	if err != nil {
		a.message = openError(rec.Path, err) + " The recovery file is kept."
		return
	}
	if f.ReadOnly() {
		f.Close() //nolint:errcheck // Nothing was read from it.
		a.message = filepath.Base(rec.Path) + " is now over 100 MB, so it can't be restored. The recovery file is kept."
		return
	}
	a.moveToPath(rec.Path)
	a.dropRestoredTabs(a.dir(), rec.Path)
	if bytes.Equal(text, f.Slice(0, f.Len())) {
		a.recovery.Discard(rec.Path)
		a.addEditor(a.dir(), f)
		a.message = filepath.Base(rec.Path) + " on disk already matches the recovered text."
		return
	}
	it := a.addEditor(a.dir(), f)
	if err := it.ed.SetText(string(text)); err != nil {
		a.message = "Can't restore " + it.name() + ": " + err.Error()
		return
	}
	a.edited(it) // Replaces the old recovery file with this process's, so no other window offers it.
	a.message = "Restored " + it.name() + ". Save to keep it."
}

// moveToPath switches to the project directory holding path, if there is one.
func (a *app) moveToPath(path string) {
	for p, project := range a.projects {
		for d, dir := range project.Dirs {
			if inDir(dir, path) {
				a.project = p
				a.dirOf[p] = d
				a.afterSwitch()
				return
			}
		}
	}
}

func inDir(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Prompts.

type choice struct {
	key   rune
	label string
	run   func()
}

// prompt asks one question. Esc picks the last choice, so make that the safe one.
type prompt struct {
	text    string
	choices []choice
}

func (a *app) ask(text string, choices ...choice) {
	a.prompts = append(a.prompts, &prompt{text, choices})
}

func (a *app) answer(c choice) {
	a.prompts = a.prompts[1:]
	c.run()
}

func (a *app) handlePromptKey(ev *tcell.EventKey) {
	p := a.prompts[0]
	switch ev.Key() {
	case tcell.KeyEsc:
		a.answer(p.choices[len(p.choices)-1])
	case tcell.KeyRune:
		for _, c := range p.choices {
			if c.key == unicode.ToLower(ev.Rune()) {
				a.answer(c)
				return
			}
		}
	}
}
