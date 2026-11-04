// Package app is the UI loop. One goroutine owns all state here (P-01), and other
// goroutines only ask it to redraw.
package app

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/Juan17la/lazycode/internal/editor"
	"github.com/Juan17la/lazycode/internal/project"
	"github.com/Juan17la/lazycode/internal/term"
	"github.com/Juan17la/lazycode/internal/tree"
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

const frame = time.Second / 60 // Terminal output redraws at most this often.

var (
	accent = tcell.StyleDefault.Foreground(tcell.PaletteColor(208)).Bold(true)
	muted  = tcell.StyleDefault.Dim(true)
	active = tcell.StyleDefault.Reverse(true)
)

// dirState is what belongs to one directory: its tree and its tabs (D-30).
type dirState struct {
	tree *tree.Tree
	tabs []*tabItem
	cur  int
	top  int // First tree row on screen.
}

type app struct {
	screen   tcell.Screen
	redraw   chan struct{}
	projects []project.Project
	project  int
	dirOf    []int // Active directory of each project.
	dirs     map[string]*dirState

	tabFocused bool
	chord      bool // Ctrl+K was pressed, and the next key finishes it.
	escaped    bool // Esc was pressed in a file, so : opens the command bar next.
	pasting    bool // Between the two bracketed paste events.
	paste      strings.Builder
	command    *string   // The : bar's text, when it's open.
	switcher   *switcher // The open switcher.
	prompts    []*prompt // Questions waiting for an answer. The first one is showing.
	message    string
	quit       bool

	recovery      *editor.Recovery
	register      string      // The last text copied here, for when there's no clipboard tool.
	clipboardJobs chan func() // Run in order, off the UI goroutine.
	tasks         chan func() // Run on the UI goroutine, for other goroutines.
	done          chan struct{}
}

// Run shows the UI until the user quits, with dir as the starting folder. startup
// is shown first if it's set, such as a config error.
func Run(screen tcell.Screen, projects []project.Project, dir, startup string) error {
	if err := screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()
	screen.EnablePaste()

	all, p, d := project.Locate(projects, dir)
	a := &app{
		screen:   screen,
		redraw:   make(chan struct{}, 1),
		projects: all,
		project:  p,
		dirOf:    make([]int, len(all)),
		dirs:     map[string]*dirState{},
		message:  startup,

		clipboardJobs: make(chan func(), 16),
		tasks:         make(chan func()),
		done:          make(chan struct{}),
	}
	a.dirOf[p] = d
	defer a.closeAllTabs()

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

	a.resizeTabs()
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
		st = &dirState{tree: tree.New(path)}
		a.dirs[path] = st
	}
	return st
}

func (a *app) tab() *tabItem {
	if st := a.dir(); st.cur < len(st.tabs) {
		return st.tabs[st.cur]
	}
	return nil
}

func (a *app) closeAllTabs() {
	var wg sync.WaitGroup
	for _, st := range a.dirs {
		for _, it := range st.tabs {
			if it.term != nil {
				wg.Go(it.term.Close)
			}
		}
	}
	wg.Wait()
}

// Input.

func (a *app) handle(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventResize:
		a.screen.Sync()
		a.resizeTabs()
	case *tcell.EventPaste:
		a.pasting = ev.Start()
		if ev.End() {
			if it := a.tab(); a.tabFocused && it != nil && a.command == nil && a.switcher == nil && len(a.prompts) == 0 {
				if it.term != nil {
					it.term.Paste(a.paste.String())
				} else {
					a.pasteInto(it, a.paste.String())
				}
			}
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
		return
	}
	switch {
	case a.switcher != nil:
		a.handleSwitcherKey(ev)
	case a.command != nil:
		a.handleCommandKey(ev)
	case a.tabFocused:
		switch it := a.tab(); {
		case it == nil:
			a.tabFocused = false
		case it.term != nil:
			it.term.Key(ev)
		default:
			a.handleEditorKey(it, ev)
		}
	default:
		a.handlePanelKey(ev)
	}
}

// handleShortcutKey handles the shortcuts that work outside terminal tabs, and reports
// whether it used the key. Each has two forms (Q-80): Alt+key on Linux and Windows, and
// Ctrl+K then key on macOS, where Option types special characters by default. Both work
// everywhere, so no OS check is needed.
func (a *app) handleShortcutKey(ev *tcell.EventKey) bool {
	chorded := a.chord
	a.chord = false
	if ev.Key() == tcell.KeyCtrlK || ctrlLetter(ev) == 'k' {
		a.chord = true
		a.message = "Ctrl+K: press d, o, comma or period"
		return true
	}
	if ev.Key() == tcell.KeyRune && (chorded || ev.Modifiers()&tcell.ModAlt != 0) {
		a.handleShortcut(string(ev.Rune()))
		return true
	}
	return chorded // Any other key cancels the chord.
}

func (a *app) handlePanelKey(ev *tcell.EventKey) {
	if a.handleShortcutKey(ev) {
		return
	}
	st := a.dir()
	key := ev.Key()
	switch key {
	case tcell.KeyUp:
		st.tree.Move(-1)
	case tcell.KeyDown:
		st.tree.Move(1)
	case tcell.KeyLeft:
		st.tree.SetOpen(false)
	case tcell.KeyRight:
		st.tree.SetOpen(true)
	case tcell.KeyEnter:
		a.openSelected(st)
	case tcell.KeyTab:
		if a.tab() != nil {
			a.focusTab()
		} else {
			a.message = "No tabs here. Enter opens a file, and :term starts a tool."
		}
	case tcell.KeyCtrlW:
		a.closeTab(st)
	case tcell.KeyRune:
		switch string(ev.Rune()) {
		case ":":
			empty := ""
			a.command = &empty
		case "r":
			st.tree.Refresh()
		}
	}
}

func (a *app) handleShortcut(key string) {
	switch strings.ToLower(key) {
	case "o":
		a.openProjectSwitcher()
	case "d":
		a.openDirSwitcher()
	case ",":
		a.moveTab(-1)
	case ".":
		a.moveTab(1)
	}
}

func (a *app) openSelected(st *dirState) {
	row, ok := st.tree.Selected()
	switch {
	case !ok:
	case row.IsDir:
		st.tree.SetOpen(!row.Open)
	default:
		a.openFile(st, row.Path)
	}
}

// moveTab goes to the previous or next tab in this directory, and focuses it.
func (a *app) moveTab(delta int) {
	st := a.dir()
	if len(st.tabs) == 0 {
		a.message = "No tabs here. Enter opens a file, and :term starts a tool."
		return
	}
	st.cur = (st.cur + delta + len(st.tabs)) % len(st.tabs)
	a.focusTab()
}

func (a *app) removeTab(st *dirState, it *tabItem) {
	i := slices.Index(st.tabs, it)
	if i < 0 {
		return
	}
	st.tabs = slices.Delete(st.tabs, i, i+1) // Clears the old slot, so the tab can be freed.
	st.cur = max(0, min(st.cur, len(st.tabs)-1))
	go func() { // It can wait for a stubborn tool, and the UI mustn't.
		if it.term != nil {
			it.term.Close()
		} else {
			it.ed.File.Close() //nolint:errcheck // Only frees memory, and the tab is gone.
		}
		debug.FreeOSMemory() // Closing is when users expect RAM back (D-28), not minutes later.
	}()
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

func (a *app) run(line string) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return
	}
	switch fields[0] {
	case "q", "quit":
		a.askQuit(map[*tabItem]bool{})
	case "w", "save":
		if it := a.tab(); it != nil && it.ed != nil {
			a.save(it, func() {})
		} else {
			a.message = "There's no file to save here."
		}
	case "project":
		a.openProjectSwitcher()
	case "dir":
		a.openDirSwitcher()
	case "close":
		a.closeTab(a.dir())
	case "term":
		// The command is split into arguments and never passed through a shell.
		// ponytail: no quoting yet, D3 replaces this with the harness list.
		a.startTab(fields[1:])
	default:
		a.message = "Unknown command: " + fields[0] + ". Try save, term, project, dir, close, or q."
	}
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
	w, h := a.termSize()
	t, err := term.Start(a.dirPath(), args, w, h, a.requestRedraw)
	if err != nil {
		a.message = err.Error()
		return
	}
	st := a.dir()
	st.tabs = append(st.tabs, &tabItem{term: t})
	st.cur = len(st.tabs) - 1
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

// Drawing.

// treeWidth is the file tree's width. It hides in very narrow windows.
func (a *app) treeWidth() int {
	w, _ := a.screen.Size()
	if w < 40 {
		return 0
	}
	return min(32, w/3)
}

func (a *app) termSize() (width, height int) {
	w, h := a.screen.Size()
	if tw := a.treeWidth(); tw > 0 {
		w -= tw + 1
	}
	return max(1, w), max(1, h-3) // Title, tab bar and status lines.
}

func (a *app) resizeTabs() {
	w, h := a.termSize()
	for _, st := range a.dirs {
		for _, it := range st.tabs {
			if it.term != nil {
				it.term.Resize(w, h)
			}
		}
	}
}

// put draws text, stops at maxX, and returns the next column. Control characters
// become "?", so that a file name can't send escape sequences to the terminal.
func (a *app) put(x, y, maxX int, text string, style tcell.Style) int {
	for _, r := range text {
		if unicode.IsControl(r) {
			r = '?'
		}
		w := runewidth.RuneWidth(r)
		if x+w > maxX {
			break
		}
		a.screen.SetContent(x, y, r, nil, style)
		x += w
	}
	return x
}

func (a *app) draw() {
	s := a.screen
	s.Clear()
	s.HideCursor()
	w, h := s.Size()
	st := a.dir()

	x := a.put(0, 0, w, " "+a.projects[a.project].Name, tcell.StyleDefault.Bold(true))
	a.put(x, 0, w, " › "+filepath.Base(a.dirPath()), tcell.StyleDefault)

	left := 0
	if tw := a.treeWidth(); tw > 0 {
		a.drawTree(st, tw, h-2)
		for y := 1; y < h-1; y++ {
			s.SetContent(tw, y, '│', nil, muted)
		}
		left = tw + 1
	}
	a.drawTabs(st, left, w)
	a.drawTabArea(st, left)
	a.drawStatus(w, h)
	if a.switcher != nil {
		a.drawSwitcher(w, h)
	}
	s.Show()
}

func (a *app) drawTree(st *dirState, width, height int) {
	title := muted
	if !a.tabFocused {
		title = accent
	}
	a.put(1, 1, width, "Files", title)

	y0, rows := 2, height-1
	if st.tree.Err != nil {
		a.put(1, y0, width, "Can't read this directory", tcell.StyleDefault)
		return
	}
	if len(st.tree.Rows) == 0 {
		a.put(1, y0, width, "Empty directory", muted)
		return
	}
	// Keep the selection on screen.
	st.top = max(0, min(st.top, st.tree.Sel))
	if st.tree.Sel >= st.top+rows {
		st.top = st.tree.Sel - rows + 1
	}
	for i := st.top; i < len(st.tree.Rows) && i < st.top+rows; i++ {
		row := st.tree.Rows[i]
		marker := "  "
		if row.IsDir {
			marker = "▸ "
			if row.Open {
				marker = "▾ "
			}
		}
		style := tcell.StyleDefault
		if i == st.tree.Sel {
			style = tcell.StyleDefault.Underline(true)
			if !a.tabFocused {
				style = active
			}
		}
		a.put(1, y0+i-st.top, width, strings.Repeat("  ", row.Depth)+marker+row.Name, style)
	}
}

func (a *app) drawTabs(st *dirState, left, width int) {
	x := left + 1
	for i, it := range st.tabs {
		label := " " + it.name() + " "
		switch {
		case it.ed != nil:
			if it.ed.File.Modified() {
				label = " " + it.name() + "* "
			}
		default:
			label = " ▶ " + it.name() + " "
			if done, code := it.term.Exited(); done {
				label = fmt.Sprintf(" ▶ %s [exit %d] ", it.name(), code)
			}
		}
		style := muted
		if i == st.cur {
			style = tcell.StyleDefault.Bold(true)
			if a.tabFocused {
				style = active
			}
		}
		x = a.put(x, 1, width, label, style) + 1
	}
}

func (a *app) drawTabArea(st *dirState, left int) {
	w, h := a.termSize()
	it := a.tab()
	switch {
	case it == nil:
		a.put(left+1, 3, left+w, "No tabs here.", tcell.StyleDefault)
		a.put(left+1, 4, left+w, "Enter opens a file. To start a tool, press : then type: term claude", muted)
	case it.ed != nil:
		a.drawEditor(it.ed, left, 2, w, h)
	default:
		it.term.Draw(a.screen, left, 2, w, h)
		if x, y, visible := it.term.Cursor(); visible && a.tabFocused && a.switcher == nil && a.command == nil && len(a.prompts) == 0 {
			a.screen.ShowCursor(left+x, 2+y)
		}
	}
}

func (a *app) drawStatus(w, h int) {
	y := h - 1
	switch {
	case len(a.prompts) > 0:
		a.drawPrompt(w, y)
	case a.command != nil:
		x := a.put(0, y, w, ":"+*a.command, tcell.StyleDefault)
		a.screen.ShowCursor(min(x, w-1), y)
	case a.message != "":
		a.put(1, y, w, a.message, tcell.StyleDefault)
	case a.tabFocused && a.tab() != nil && a.tab().ed != nil:
		e := a.tab().ed
		line, col := e.Position()
		where := fmt.Sprintf("Ln %d, Col %d", line+1, col+1)
		if e.File.ReadOnly() {
			where = "Read-only  " + where
		}
		a.put(max(1, w-len(where)-1), y, w, where, tcell.StyleDefault)
		a.put(1, y, max(1, w-len(where)-2), `Ctrl+S save   Ctrl+\ files   Esc : commands`, muted)
	case a.tabFocused:
		a.put(1, y, w, `Ctrl+\ leaves the terminal`, muted)
	default:
		a.put(1, y, w, "Enter open   Alt+O or :project   Alt+D or :dir   Alt+, Alt+. tabs   Tab enter tab   : commands", muted)
	}
}

func (a *app) drawSwitcher(w, h int) {
	s := a.switcher
	matches := s.matches()
	bw := max(1, min(50, w-4))
	bh := max(1, min(len(matches), h-6))
	x0, y0 := (w-bw)/2, max(0, (h-bh)/2-2)

	for y := y0; y < y0+bh+3; y++ {
		for x := x0; x < x0+bw; x++ {
			a.screen.SetContent(x, y, ' ', nil, tcell.StyleDefault)
		}
	}
	maxX := x0 + bw
	a.put(x0+1, y0, maxX, s.title, accent)
	end := a.put(x0+1, y0+1, maxX, "> "+s.filter, tcell.StyleDefault)
	a.screen.ShowCursor(min(end, maxX-1), y0+1)
	if len(matches) == 0 {
		a.put(x0+1, y0+2, maxX, "No matches", muted)
	}
	for i := 0; i < bh && i < len(matches); i++ {
		style := tcell.StyleDefault
		prefix := "  "
		if i == s.sel {
			style, prefix = active, "› "
		}
		a.put(x0+1, y0+2+i, maxX, prefix+s.labels[matches[i]], style)
	}
}
