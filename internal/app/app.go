// Package app is the UI loop. One goroutine owns all state here (P-01), and other
// goroutines only ask it to redraw or to run something.
package app

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Juan17la/lazycode/internal/buffer"
	"github.com/Juan17la/lazycode/internal/config"
	"github.com/Juan17la/lazycode/internal/editor"
	"github.com/Juan17la/lazycode/internal/keymap"
	"github.com/Juan17la/lazycode/internal/project"
	"github.com/Juan17la/lazycode/internal/term"
	"github.com/Juan17la/lazycode/internal/tree"
	"github.com/gdamore/tcell/v2"
)

const frame = time.Second / 60 // Terminal output redraws at most this often.

// dirState is what belongs to one directory: its tree and its tabs (D-30).
type dirState struct {
	tree  *tree.Tree
	root  *split // The arrangement of panes.
	focus *pane  // The pane new tabs go in, and that keys go to.
	top   int    // First tree row on screen.
}

type app struct {
	screen   tcell.Screen
	redraw   chan struct{}
	projects []project.Project
	project  int
	dirOf    []int // Active directory of each project.
	dirs     map[string]*dirState

	cfg        config.Config
	keys       *keymap.Map
	th         theme
	showSecond bool // The second panel is open (Q-18).
	filesPanel int  // Which panel holds the Files view: 0 is the first, 1 the second.

	tabFocused bool
	chord      bool // Ctrl+K was pressed, and the next key finishes it.
	escaped    bool // Esc was pressed in a file, so : opens the command bar next.
	pasting    bool // Between the two bracketed paste events.
	paste      strings.Builder
	command    *string   // The : bar's text, when it's open.
	switcher   *switcher // The open switcher.
	find       *findBar  // The open find bar.
	prompts    []*prompt // Questions waiting for an answer. The first one is showing.
	message    string
	quit       bool
	hits       []hit // What the mouse can click, from the last frame.

	recovery      *editor.Recovery
	autoSaves     map[*buffer.File]*time.Timer
	register      string      // The last text copied here, for when there's no clipboard tool.
	clipboardJobs chan func() // Run in order, off the UI goroutine.
	tasks         chan func() // Run on the UI goroutine, for other goroutines.
	done          chan struct{}
}

func newApp(screen tcell.Screen, projects []project.Project, dir, startup string) *app {
	all, p, d := project.Locate(projects, dir)
	a := &app{
		screen:   screen,
		redraw:   make(chan struct{}, 1),
		projects: all,
		project:  p,
		dirOf:    make([]int, len(all)),
		dirs:     map[string]*dirState{},
		cfg:      config.Default(),
		keys:     keymap.Default(),
		message:  startup,

		autoSaves:     map[*buffer.File]*time.Timer{},
		clipboardJobs: make(chan func(), 16),
		tasks:         make(chan func()),
		done:          make(chan struct{}),
	}
	a.dirOf[p] = d
	a.th = themeFor(a.cfg.UI.Theme)
	return a
}

// Run shows the UI until the user quits, with dir as the starting folder. startup
// is shown first if it's set, such as a config error.
func Run(screen tcell.Screen, projects []project.Project, dir, startup string) error {
	if err := screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()
	screen.EnablePaste()
	// Clicks and the wheel only (Q-13). Shift+drag still selects text in most terminals.
	// ponytail: dragging later means asking for MouseDragEvents here too.
	screen.EnableMouse(tcell.MouseButtonEvents)

	a := newApp(screen, projects, dir, startup)
	defer a.closeAllTabs()
	a.loadSettings()

	// If the folder can't be found, writes fail and report it through this message.
	recoveryDir, _ := editor.RecoveryDir()
	a.recovery = editor.NewRecovery(recoveryDir, func(path string, err error) {
		a.post(func() { a.message = "Your unsaved work isn't protected: " + err.Error() })
	})
	defer a.recovery.Wait() // Lets waiting recovery files finish when stopped by a signal.
	a.offerRecovery()

	// SIGHUP arrives when the terminal window is closed.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(stop)

	events := make(chan tcell.Event)
	defer close(a.done)
	go screen.ChannelEvents(events, a.done)
	go func() {
		for {
			select {
			case job := <-a.clipboardJobs:
				job()
			case <-a.done:
				return
			}
		}
	}()

	a.draw()
	var tick <-chan time.Time
	for !a.quit {
		select {
		case ev := <-events:
			a.handle(ev)
		case task := <-a.tasks:
			task()
		case <-a.redraw:
			if tick == nil {
				tick = time.After(frame)
			}
			continue
		case <-tick:
			tick = nil
		case <-stop:
			return nil
		}
		a.leaveFinishedTab()
		a.draw()
	}
	return nil
}

func (a *app) requestRedraw() {
	select {
	case a.redraw <- struct{}{}:
	default: // One is already waiting.
	}
}

// post runs f on the UI goroutine.
func (a *app) post(f func()) {
	select {
	case a.tasks <- f:
	case <-a.done:
	}
}

// leaveFinishedTab gives the keys back when a focused terminal tab's command ends.
// Otherwise they'd keep going to a tool that's gone, and : would seem to do nothing.
func (a *app) leaveFinishedTab() {
	if !a.tabFocused {
		return
	}
	it := a.tab()
	if it == nil {
		a.tabFocused = false
		return
	}
	if it.term != nil {
		if done, code := it.term.Exited(); done {
			a.tabFocused = false
			a.message = fmt.Sprintf("%s exited with code %d. Ctrl+W closes its tab.", it.name(), code)
		}
	}
}

func (a *app) dirPath() string { return a.projects[a.project].Dirs[a.dirOf[a.project]] }

func (a *app) dir() *dirState {
	path := a.dirPath()
	st := a.dirs[path]
	if st == nil {
		first := &pane{}
		st = &dirState{tree: tree.New(path), root: &split{pane: first}, focus: first}
		a.dirs[path] = st
	}
	return st
}

// tab returns the focused pane's current tab, if it has one.
func (a *app) tab() *tabItem { return a.dir().focus.current() }

func (a *app) closeAllTabs() {
	var wg sync.WaitGroup
	for _, st := range a.dirs {
		for _, p := range st.root.panes() {
			for _, it := range p.tabs {
				if it.term != nil {
					wg.Go(it.term.Close)
				}
			}
		}
	}
	wg.Wait()
}

// Settings.

func (a *app) settingsPath(name string) string {
	dir, err := config.Dir()
	if err != nil {
		return name
	}
	return filepath.Join(dir, name)
}

// loadSettings reads the config files. A broken config.toml keeps the settings in use,
// and keys.toml applies whatever it can (doc 07).
func (a *app) loadSettings() {
	var problems []string
	cfg, err := config.Load(a.settingsPath("config.toml"))
	if err != nil {
		problems = append(problems, err.Error())
	} else {
		a.cfg = cfg
	}
	keys, err := keymap.Load(a.settingsPath("keys.toml"))
	if err != nil {
		problems = append(problems, err.Error())
	}
	a.keys = keys
	a.th = themeFor(a.cfg.UI.Theme)
	for _, st := range a.dirs {
		for _, p := range st.root.panes() {
			for _, it := range p.tabs {
				if it.ed != nil {
					it.ed.TabSize = a.cfg.Editor.TabSize
				}
			}
		}
	}
	if len(problems) > 0 {
		a.message = strings.Join(append(problems, a.message), "  ")
	}
}

// Input.

func (a *app) handle(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventResize:
		a.screen.Sync()
	case *tcell.EventMouse:
		a.handleMouse(ev)
	case *tcell.EventPaste:
		a.pasting = ev.Start()
		if ev.End() {
			a.finishPaste(a.paste.String())
			a.paste.Reset()
		}
	case *tcell.EventKey:
		switch {
		case a.pasting:
			a.addPaste(ev)
		default:
			a.handleKey(ev)
		}
	}
}

func (a *app) finishPaste(text string) {
	switch it := a.tab(); {
	case a.find != nil:
		a.find.paste(a, text)
	case !a.tabFocused || it == nil || a.command != nil || a.switcher != nil || len(a.prompts) > 0:
	case it.term != nil:
		it.term.Paste(text)
	default:
		a.pasteInto(it, text)
	}
}

// addPaste keeps text and line breaks only. Dropping Escape means pasted content
// can't forge the end of a bracketed paste and run as typed input.
func (a *app) addPaste(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyRune:
		a.paste.WriteString(string(ev.Rune()))
	case tcell.KeyEnter:
		a.paste.WriteByte('\r')
	case tcell.KeyTab:
		a.paste.WriteByte('\t')
	}
}

func (a *app) handleKey(ev *tcell.EventKey) {
	a.message = ""
	if len(a.prompts) > 0 { // Asked before anything else, so no answer gets skipped.
		a.handlePromptKey(ev)
		return
	}
	if term.IsLeaveKey(ev) { // Q-74: the one key a terminal tab never receives.
		a.tabFocused, a.chord, a.escaped, a.command, a.switcher = false, false, false, nil, nil
		a.closeFind()
		return
	}
	switch {
	case a.switcher != nil:
		a.handleSwitcherKey(ev)
		return
	case a.command != nil:
		a.handleCommandKey(ev)
		return
	case a.find != nil:
		a.handleFindKey(ev)
		return
	}
	it := a.tab()
	if a.tabFocused && it == nil {
		a.tabFocused = false
	}
	if a.tabFocused && it.term != nil {
		it.term.Key(ev)
		return
	}
	context := keymap.Panel
	if a.tabFocused {
		context = keymap.Editor
	}
	a.handleBoundKey(context, it, ev)
}

// handleBoundKey runs what the keymap says, and types what it doesn't claim. Every
// shortcut has two forms (Q-80): Alt+key on Linux and Windows, and Ctrl+K then key on
// macOS, where Option types special characters by default. Both work everywhere, so no
// OS check is needed.
func (a *app) handleBoundKey(context keymap.Context, it *tabItem, ev *tcell.EventKey) {
	name := keymap.Name(ev)
	escaped := a.escaped
	a.escaped = false
	if a.chord {
		a.chord = false
		if action := a.keys.LookupChord(context, name); action != "" {
			a.do(action)
		}
		return // Any other key cancels the chord.
	}
	if escaped && context == keymap.Editor && name == ":" {
		a.openCommand()
		return
	}
	if action := a.keys.Lookup(context, name); action != "" {
		a.do(action)
		return
	}
	if name == "ctrl+k" {
		a.chord = true
		a.message = "Ctrl+K: press d, o, comma, period or a"
		return
	}
	mods := ev.Modifiers()
	if context == keymap.Editor && ev.Key() == tcell.KeyRune && mods&(tcell.ModCtrl|tcell.ModAlt) == 0 {
		a.editWith(it, func() error { return it.ed.Type(string(ev.Rune())) })
	}
}

func (a *app) openCommand() {
	empty := ""
	a.command = &empty
}

func (a *app) handleCommandKey(ev *tcell.EventKey) {
	text := *a.command
	switch ev.Key() {
	case tcell.KeyEsc:
		a.command = nil
	case tcell.KeyEnter:
		a.command = nil
		a.run(text)
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if text == "" {
			a.command = nil
		} else {
			r := []rune(text)
			text = string(r[:len(r)-1])
			a.command = &text
		}
	case tcell.KeyRune:
		text += string(ev.Rune())
		a.command = &text
	}
}

// run handles the : commands. Each full name has a short form, as in Neovim (Q-22).
func (a *app) run(line string) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return
	}
	it := a.tab()
	switch fields[0] {
	case "q", "quit":
		a.askQuit(map[*buffer.File]bool{})
	case "w", "save":
		if it != nil && it.ed != nil {
			a.save(it, func() {})
		} else {
			a.message = "There's no file to save here."
		}
	case "project":
		a.openProjectSwitcher()
	case "dir":
		a.openDirSwitcher()
	case "close":
		a.closeTab()
	case "term":
		// The command is split into arguments and never passed through a shell.
		// ponytail: no quoting yet, D3 replaces this with the harness list.
		a.startTab(fields[1:])
	case "split":
		a.splitPane(len(fields) < 2 || fields[1] != "down")
	case "selectall":
		if it != nil && it.ed != nil {
			a.do("select.all_occurrences")
		} else {
			a.message = "There's no file to select in here."
		}
	case "find":
		a.do("find.open")
	case "replace":
		a.do("find.replace")
	case "view":
		a.moveFilesView()
	case "config":
		name, content := "config.toml", config.Template
		if len(fields) > 1 && fields[1] == "keys" {
			name, content = "keys.toml", keymap.Template
		}
		a.openSettings(name, content)
	default:
		a.message = "Unknown command: " + fields[0] + ". Try save, term, split, find, config, project, dir, close, or q."
	}
}

func (a *app) openSettings(name, content string) {
	path := a.settingsPath(name)
	if err := config.EnsureFile(path, content); err != nil {
		a.message = "Can't create " + name + ": " + err.Error()
		return
	}
	a.openFile(a.dir(), path)
}

func defaultShell() []string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return []string{shell}
	}
	if comspec := os.Getenv("COMSPEC"); comspec != "" {
		return []string{comspec}
	}
	return []string{"sh"}
}

func (a *app) startTab(args []string) {
	if len(args) == 0 {
		args = defaultShell()
	}
	st := a.dir()
	a.layout(st)
	size := st.focus.content()
	t, err := term.Start(a.dirPath(), args, size.w, size.h, a.requestRedraw)
	if err != nil {
		a.message = err.Error()
		return
	}
	t.SetScrollback(a.cfg.Terminal.Scrollback)
	st.focus.tabs = append(st.focus.tabs, &tabItem{term: t, w: size.w, h: size.h})
	st.focus.cur = len(st.focus.tabs) - 1
	a.tabFocused = true
}

// Switcher, the same for projects and directories.

type switcher struct {
	title  string
	labels []string
	filter string
	sel    int
	pick   func(index int)
}

func (s *switcher) matches() []int {
	var out []int
	for i, label := range s.labels {
		if strings.Contains(strings.ToLower(label), strings.ToLower(s.filter)) {
			out = append(out, i)
		}
	}
	return out
}

func (a *app) openProjectSwitcher() {
	s := &switcher{title: "Switch project"}
	for _, p := range a.projects {
		s.labels = append(s.labels, fmt.Sprintf("%s  (%d)", p.Name, len(p.Dirs)))
	}
	s.pick = func(i int) { a.project = i; a.afterSwitch() }
	a.switcher = s
}

func (a *app) openDirSwitcher() {
	s := &switcher{title: "Switch directory"}
	dirs := a.projects[a.project].Dirs
	for _, d := range dirs {
		label := filepath.Base(d)
		if _, err := os.Stat(d); err != nil {
			label += "  (missing)" // Q-66: marked, never removed silently.
		}
		s.labels = append(s.labels, label)
	}
	s.pick = func(i int) { a.dirOf[a.project] = i; a.afterSwitch() }
	a.switcher = s
}

// afterSwitch shows the new directory's views. Tabs elsewhere keep running (Q-34).
func (a *app) afterSwitch() {
	a.tabFocused = false
	a.closeFind()
	a.dir().tree.Refresh()
}

func (a *app) handleSwitcherKey(ev *tcell.EventKey) {
	s := a.switcher
	matches := s.matches()
	switch ev.Key() {
	case tcell.KeyEsc:
		a.switcher = nil
	case tcell.KeyEnter:
		a.switcher = nil
		if s.sel < len(matches) {
			s.pick(matches[s.sel])
		}
	case tcell.KeyUp:
		s.sel = max(0, s.sel-1)
	case tcell.KeyDown:
		s.sel = max(0, min(s.sel+1, len(matches)-1))
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if r := []rune(s.filter); len(r) > 0 {
			s.filter, s.sel = string(r[:len(r)-1]), 0
		}
	case tcell.KeyRune:
		s.filter += string(ev.Rune())
		s.sel = 0
	}
}
