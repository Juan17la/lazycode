package app

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Juan17la/lazycode/internal/buffer"
	"github.com/Juan17la/lazycode/internal/clipboard"
	"github.com/Juan17la/lazycode/internal/editor"
	"github.com/Juan17la/lazycode/internal/term"
	"github.com/gdamore/tcell/v2"
)

// tabSize is fixed until the config arrives in CORE-09.
const tabSize = 4

// tabItem is a tab, holding either a terminal or a file.
type tabItem struct {
	term *term.Tab
	ed   *editor.Editor
}

func (it *tabItem) name() string {
	if it.ed != nil {
		return filepath.Base(it.ed.File.Path)
	}
	return it.term.Name
}

// Opening files.

func (a *app) openFile(st *dirState, path string) {
	for i, it := range st.tabs {
		if it.ed != nil && it.ed.File.Path == path {
			st.cur = i
			a.focusTab()
			return
		}
	}
	f, err := buffer.Open(path)
	if err != nil {
		a.message = openError(path, err)
		return
	}
	a.addEditor(st, f)
	if f.ReadOnly() {
		a.message = filepath.Base(path) + " is over 100 MB, so it's read-only."
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
	it := &tabItem{ed: editor.New(f, tabSize)}
	st.tabs = append(st.tabs, it)
	st.cur = len(st.tabs) - 1
	a.tabFocused = true
	return it
}

// focusTab moves focus to the current tab. Until live updates arrive in D5, this is
// when a file's changes on disk get noticed, such as after an AI tool edits it.
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

// Keys.

// ctrlLetter returns the letter of a Ctrl+letter key, however this OS delivers it, or 0.
func ctrlLetter(ev *tcell.EventKey) rune {
	key := ev.Key()
	if key == tcell.KeyRune && ev.Modifiers()&tcell.ModCtrl != 0 {
		return unicode.ToLower(ev.Rune())
	}
	if key >= tcell.KeyCtrlA && key <= tcell.KeyCtrlZ {
		return 'a' + rune(key-tcell.KeyCtrlA)
	}
	return 0
}

func (a *app) handleEditorKey(it *tabItem, ev *tcell.EventKey) {
	escaped := a.escaped
	a.escaped = false
	if escaped && ev.Key() == tcell.KeyRune && ev.Rune() == ':' {
		empty := ""
		a.command = &empty
		return
	}
	if a.handleShortcutKey(ev) {
		return
	}
	e := it.ed
	before := e.File.Snapshot
	if err := a.editorKey(it, ev); errors.Is(err, buffer.ErrReadOnly) {
		a.message = it.name() + " is read-only."
	} else if err != nil {
		a.message = err.Error()
	}
	if !before.Same(e.File.Snapshot) {
		a.edited(it)
	}
}

func (a *app) editorKey(it *tabItem, ev *tcell.EventKey) error {
	e := it.ed
	mods := ev.Modifiers()
	shift, ctrl := mods&tcell.ModShift != 0, mods&tcell.ModCtrl != 0
	switch ev.Key() {
	case tcell.KeyEsc:
		e.ClearSelection()
		a.escaped = true
		a.message = "Press : for commands"
	case tcell.KeyLeft:
		if ctrl {
			e.WordLeft(shift)
		} else {
			e.Left(shift)
		}
	case tcell.KeyRight:
		if ctrl {
			e.WordRight(shift)
		} else {
			e.Right(shift)
		}
	case tcell.KeyUp:
		e.Up(shift)
	case tcell.KeyDown:
		e.Down(shift)
	case tcell.KeyPgUp:
		e.PageUp(shift)
	case tcell.KeyPgDn:
		e.PageDown(shift)
	case tcell.KeyHome:
		if ctrl {
			e.DocStart(shift)
		} else {
			e.Home(shift)
		}
	case tcell.KeyEnd:
		if ctrl {
			e.DocEnd(shift)
		} else {
			e.End(shift)
		}
	case tcell.KeyEnter:
		return e.Newline()
	case tcell.KeyTab:
		if shift {
			return e.Outdent()
		}
		return e.Tab()
	case tcell.KeyBacktab:
		return e.Outdent()
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		return e.Backspace()
	case tcell.KeyDelete:
		return e.Delete()
	default:
		switch ctrlLetter(ev) {
		case 0:
			if ev.Key() == tcell.KeyRune {
				return e.Type(string(ev.Rune()))
			}
		case 'a':
			e.SelectAll()
		case 'c':
			a.copyText(e.Copy())
		case 'x':
			text, err := e.Cut()
			if err != nil {
				return err
			}
			a.copyText(text)
		case 'v':
			a.pasteFromClipboard(it)
		case 's':
			a.save(it, func() {})
		case 'w':
			a.closeTab(a.dir())
		case 'z':
			if shift {
				e.Redo()
			} else {
				e.Undo()
			}
		case 'y':
			e.Redo()
		}
	}
	return nil
}

// edited keeps the recovery file in step with the text.
func (a *app) edited(it *tabItem) {
	if it.ed.File.Modified() {
		a.recovery.Schedule(it.ed.File.Path, it.ed.File.Snapshot)
	} else {
		a.recovery.Discard(it.ed.File.Path) // Undone back to what's saved.
	}
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
	if text == "" {
		return
	}
	if err := it.ed.Paste(text); err != nil {
		a.message = it.name() + " is read-only."
		return
	}
	a.edited(it)
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
			choice{'o', "Overwrite it", func() {
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
}

// closeTab closes the current tab, asking first about unsaved work (D-29).
func (a *app) closeTab(st *dirState) {
	if st.cur >= len(st.tabs) {
		return
	}
	it := st.tabs[st.cur]
	if it.ed == nil || !it.ed.File.Modified() {
		a.removeTab(st, it)
		return
	}
	a.ask(fmt.Sprintf("Save changes to %s?", it.name()),
		choice{'s', "Save", func() { a.save(it, func() { a.removeTab(st, it) }) }},
		choice{'d', "Don't save", func() {
			a.recovery.Discard(it.ed.File.Path)
			a.removeTab(st, it)
		}},
		choice{'c', "Cancel", func() {}})
}

// askQuit quits once each file with unsaved work has been decided on. Nothing is thrown
// away until then, so cancelling part way loses nothing.
func (a *app) askQuit(discard map[*tabItem]bool) {
	for _, st := range a.dirs {
		for _, it := range st.tabs {
			if it.ed == nil || !it.ed.File.Modified() || discard[it] {
				continue
			}
			a.ask(fmt.Sprintf("Save changes to %s before quitting?", it.name()),
				choice{'s', "Save", func() { a.save(it, func() { a.askQuit(discard) }) }},
				choice{'d', "Don't save", func() {
					discard[it] = true
					a.askQuit(discard)
				}},
				choice{'c', "Cancel", func() {}})
			return
		}
	}
	for it := range discard {
		a.recovery.Discard(it.ed.File.Path)
	}
	a.quit = true
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
	if bytes.Equal(text, f.Slice(0, f.Len())) {
		a.recovery.Discard(rec.Path)
		a.addEditor(a.dir(), f)
		a.message = filepath.Base(rec.Path) + " on disk already matches the recovered text."
		return
	}
	it := a.addEditor(a.dir(), f)
	if err := it.ed.ReplaceAll(string(text)); err != nil {
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
			rel, err := filepath.Rel(dir, path)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				a.project = p
				a.dirOf[p] = d
				a.afterSwitch()
				return
			}
		}
	}
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

func (a *app) handlePromptKey(ev *tcell.EventKey) {
	p := a.prompts[0]
	var picked *choice
	switch ev.Key() {
	case tcell.KeyEsc:
		picked = &p.choices[len(p.choices)-1]
	case tcell.KeyRune:
		for i := range p.choices {
			if p.choices[i].key == unicode.ToLower(ev.Rune()) {
				picked = &p.choices[i]
			}
		}
	}
	if picked != nil {
		a.prompts = a.prompts[1:]
		picked.run()
	}
}

// Drawing.

func (a *app) drawEditor(e *editor.Editor, left, top, width, height int) {
	gutter := e.GutterWidth()
	if width <= gutter+1 {
		gutter = 0 // Too narrow for line numbers.
	}
	textWidth := max(1, width-gutter)
	e.Scroll(textWidth, height)
	current, _ := e.Position()
	for row := range height {
		line := e.ScrollLine + row
		if line >= e.File.LineCount() {
			break
		}
		if gutter > 0 {
			style := muted
			if line == current {
				style = tcell.StyleDefault
			}
			a.put(left, top+row, left+gutter, fmt.Sprintf(" %*d", gutter-2, line+1), style)
		}
		for _, c := range e.Row(line, textWidth) {
			style := tcell.StyleDefault
			if c.Special {
				style = muted
			}
			if c.Selected {
				style = active
			}
			a.screen.Put(left+gutter+c.X, top+row, c.Text, style)
		}
	}
	if a.tabFocused && a.switcher == nil && a.command == nil && len(a.prompts) == 0 {
		x, y := e.ScreenCursor()
		a.screen.ShowCursor(left+gutter+x, top+y)
	}
}

func (a *app) drawPrompt(w, y int) {
	p := a.prompts[0]
	var options strings.Builder
	for _, c := range p.choices {
		fmt.Fprintf(&options, "  [%c] %s", c.key, c.label)
	}
	x := a.put(1, y, max(1, w-utf8.RuneCountInString(options.String())), p.text, accent)
	a.put(x, y, w, options.String(), accent)
}
