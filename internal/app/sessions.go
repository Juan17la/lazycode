package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/Juan17la/lazycode/internal/buffer"
	"github.com/Juan17la/lazycode/internal/session"
	"github.com/Juan17la/lazycode/internal/term"
	"github.com/gdamore/tcell/v2"
)

const (
	// activeOutput is how recent output must be for a tab's marker to show it as active.
	activeOutput = 1500 * time.Millisecond
	// quietPeriod is how long after working a tool must be silent to count as finished (doc 10).
	quietPeriod = 5 * time.Second
	// resizeNoise is how long a resized tool's redrawing isn't taken for work.
	resizeNoise = 2 * time.Second
	// sleepSample is how far apart CPU use is measured before a session can sleep.
	sleepSample = time.Minute
	// failedResume is how soon an exit counts as a resume that didn't work.
	failedResume = 5 * time.Second
)

// sessionEntry is one line in the AI sessions view: an AI session, or a plain shell (Q-60).
type sessionEntry struct {
	session.Session
	isShell     bool
	tab         *tabItem // Nil while it's sleeping.
	status      session.Status
	resumedAt   time.Time
	seenOutput  time.Time // Output up to here has been looked at, or reported.
	seenBells   int64
	ignoreUntil time.Time
	cpu         time.Duration // The last CPU sample, for sleeping.
	cpuAt       time.Time     // When it was taken, or zero if there's none.
}

// sessionList is the AI sessions view's state. The tests replace its functions, so that they don't
// show real notifications or wait for real CPU use.
type sessionList struct {
	entries []*sessionEntry
	sel     int
	path    string
	send    func(title, body string)
	cpuOf   func(pid int) (time.Duration, error)
}

func (a *app) loadSessions() {
	s := &a.sessions
	path, err := session.StatePath()
	if err != nil {
		return // Nothing is saved between runs, and the rest still works.
	}
	s.path = path
	saved, err := session.Load(path)
	if err != nil {
		a.message = err.Error() // Losing this only loses the list (doc 08).
	}
	for _, one := range saved {
		s.entries = append(s.entries, &sessionEntry{Session: one})
	}
}

func (a *app) saveSessions() {
	s := &a.sessions
	if s.path == "" {
		return
	}
	saved := []session.Session{}
	for _, e := range s.entries {
		if !e.isShell { // A shell can't come back, so it isn't kept.
			saved = append(saved, e.Session)
		}
	}
	if err := session.Save(s.path, saved); err != nil {
		a.message = "Sessions weren't saved: " + err.Error()
	}
}

func (a *app) harness(name string) (session.Harness, bool) {
	all := session.Merge(a.cfg.Harness)
	i := slices.IndexFunc(all, func(h session.Harness) bool { return h.Name == name })
	if i < 0 {
		return session.Harness{}, false
	}
	return all[i], true
}

// listed returns the entries for this project's view. Hidden ones stay saved, but aren't shown.
func (a *app) listed() []*sessionEntry { return a.inProject(false) }

// inProject returns this project's entries that are hidden, or that aren't.
func (a *app) inProject(hidden bool) []*sessionEntry {
	var out []*sessionEntry
	for _, e := range a.sessions.entries {
		if e.Hidden == hidden && slices.ContainsFunc(a.projects[a.project].Dirs, func(dir string) bool { return inDir(dir, e.Dir) }) {
			out = append(out, e)
		}
	}
	return out
}

// Starting.

// openTerminal starts a command in a new tab in the dock, under the code.
func (a *app) openTerminal(dir string, args []string) (*tabItem, error) {
	st := a.dir()
	_, _, tabs := a.regions()
	_, _, dock := a.dockArea(st, tabs)
	size := (&pane{area: dock}).content()
	t, err := term.Start(dir, args, size.w, size.h, a.requestRedraw)
	if err != nil {
		return nil, err
	}
	t.SetScrollback(a.cfg.Terminal.Scrollback)
	it := &tabItem{term: t, w: size.w, h: size.h}
	st.dock.tabs = append(st.dock.tabs, it)
	st.dock.cur = len(st.dock.tabs) - 1
	st.focus = st.dock
	a.dockHidden = false
	a.tabFocused = true
	return it, nil
}

// addShellEntry lists a plain terminal tab (FR-5.12).
func (a *app) addShellEntry(it *tabItem) {
	e := &sessionEntry{isShell: true, tab: it, status: session.Running}
	e.Name, e.Dir = it.name(), a.dirPath()
	it.sess = e
	a.sessions.entries = append(a.sessions.entries, e)
}

// sessionCommand is :session, or :session hidden to bring back what Delete took off the list.
func (a *app) sessionCommand(args []string) {
	if len(args) == 1 && args[0] == "hidden" {
		a.openHiddenPicker()
		return
	}
	a.openSessionPicker()
}

// openHiddenPicker lists this project's hidden sessions. Picking one puts it back on the list, still
// asleep or still running, exactly as it was (FR-5.11).
func (a *app) openHiddenPicker() {
	hidden := a.inProject(true)
	if len(hidden) == 0 {
		a.message = "No hidden sessions in this project."
		return
	}
	s := &switcher{title: "Bring back a session"}
	for _, e := range hidden {
		s.labels = append(s.labels, e.Name+"  ("+e.Harness+")")
	}
	s.pick = func(i int) {
		hidden[i].Hidden = false
		a.saveSessions()
		a.message = hidden[i].Name + " is back on the list."
	}
	a.switcher = s
}

// openSessionPicker asks which tool to start (doc 10).
func (a *app) openSessionPicker() {
	tools := session.Merge(a.cfg.Harness)
	s := &switcher{title: "Start a session"}
	for _, h := range tools {
		label := h.Name
		if _, err := exec.LookPath(h.Command); err != nil {
			label += "  (not installed)"
		}
		s.labels = append(s.labels, label)
	}
	s.labels = append(s.labels, "Shell")
	s.pick = func(i int) {
		if i == len(tools) {
			a.startTab(nil)
			return
		}
		a.startSession(tools[i])
	}
	a.switcher = s
}

// startSession runs a new session of a tool in the active directory.
func (a *app) startSession(h session.Harness) {
	dir := a.dirPath()
	id, err := session.NewID()
	if err != nil {
		a.message = err.Error()
		return
	}
	e := &sessionEntry{Session: session.Session{ID: id, Name: a.nextName(h.Name), Harness: h.Name, Dir: dir}}
	if a.launch(e, h, false) {
		a.sessions.entries = append(a.sessions.entries, e)
		a.saveSessions()
	}
}

func (a *app) nextName(tool string) string {
	for n := 1; ; n++ {
		name := fmt.Sprintf("%s %d", tool, n)
		if !slices.ContainsFunc(a.sessions.entries, func(e *sessionEntry) bool { return e.Name == name }) {
			return name
		}
	}
}

// launch runs the tool and gives the entry its tab. It reports whether that worked.
func (a *app) launch(e *sessionEntry, h session.Harness, resume bool) bool {
	if _, err := exec.LookPath(h.Command); err != nil {
		a.message = h.Name + " isn't installed: can't find " + h.Command
		return false
	}
	if _, err := os.Stat(e.Dir); err != nil {
		a.message = e.Name + "'s folder is missing: " + e.Dir // Q-66: the entry stays.
		return false
	}
	it, err := a.openTerminal(e.Dir, h.Args(e.ID, resume))
	if err != nil {
		a.message = err.Error()
		return false
	}
	it.term.Name = e.Name
	it.sess = e
	e.tab, e.status = it, session.Running
	e.seenOutput, e.seenBells, e.cpuAt = time.Now(), 0, time.Time{}
	e.resumedAt = time.Time{}
	if resume {
		e.resumedAt = time.Now()
	}
	return true
}

// openEntry goes to a session, resuming it if it's asleep (FR-5.8). Another directory's session
// switches there first (doc 10).
func (a *app) openEntry(e *sessionEntry) {
	if !inDir(a.dirPath(), e.Dir) {
		a.moveToPath(e.Dir)
	}
	if e.tab != nil {
		st, p := a.findTab(e.tab)
		if st != nil {
			st.focus, p.cur = p, slices.Index(p.tabs, e.tab)
			a.tabFocused = true
		}
		return
	}
	h, ok := a.harness(e.Harness)
	if !ok {
		a.message = e.Harness + " isn't in the tool list any more, so " + e.Name + " can't start."
		return
	}
	a.launch(e, h, h.CanResume()) // Without resume fields it starts afresh (Q-59).
}

func (a *app) findTab(it *tabItem) (*dirState, *pane) {
	for _, st := range a.dirs {
		for _, p := range st.panes() {
			if slices.Contains(p.tabs, it) {
				return st, p
			}
		}
	}
	return nil, nil
}

// tabClosed runs as any tab closes. A closed AI session can come back. A shell can't.
func (a *app) tabClosed(st *dirState, it *tabItem) {
	if st.play == it {
		st.play = nil
	}
	e := it.sess
	if e == nil {
		return
	}
	if e.isShell {
		s := &a.sessions
		s.entries = slices.DeleteFunc(s.entries, func(other *sessionEntry) bool { return other == e })
		return
	}
	e.tab, e.status = nil, session.Sleeping
	a.saveSessions()
}

// The view's keys.

func (a *app) doInSessionsView(action string) bool {
	list := a.listed()
	s := &a.sessions
	s.sel = max(0, min(s.sel, len(list)-1))
	switch action {
	case "tree.up":
		s.sel = max(0, s.sel-1)
	case "tree.down":
		s.sel = max(0, min(s.sel+1, len(list)-1))
	case "tree.select":
		if len(list) > 0 {
			a.openEntry(list[s.sel])
		}
	case "tree.refresh": // r renames here.
		if len(list) > 0 {
			e := list[s.sel]
			a.ask1Line("Rename to:", e.Name, func(name string) {
				e.Name = name
				if e.tab != nil && e.tab.term != nil {
					e.tab.term.Name = name
				}
				a.saveSessions()
			})
		}
	case "session.hide":
		if len(list) > 0 {
			a.hideEntry(list[s.sel])
		}
	case "tree.open", "tree.close":
	default:
		return false
	}
	return true
}

// hideEntry removes an entry from the list for good, stopping it if it's running. It never touches a
// tool's own data (Q-60), so the chat is still in the tool.
func (a *app) hideEntry(e *sessionEntry) {
	s := &a.sessions
	s.entries = slices.DeleteFunc(s.entries, func(other *sessionEntry) bool { return other == e })
	if e.tab != nil {
		if st, p := a.findTab(e.tab); st != nil {
			a.removeTab(st, p, e.tab)
		}
	}
	a.saveSessions()
	a.message = e.Name + " is deleted from the list. The tool still has its chat."
}

// Drawing.

func (e *sessionEntry) marker() string {
	switch e.status {
	case session.Running:
		if e.isShell {
			return "❯"
		}
		return "✻"
	case session.Attention:
		return "●"
	case session.Exited:
		return "✗"
	}
	return "○"
}

func (a *app) drawSessions(r rect) {
	maxX := r.x + r.w
	y0, rows := r.y+1, r.h-1
	list := a.listed()
	s := &a.sessions
	s.sel = max(0, min(s.sel, len(list)-1))
	if len(list) == 0 {
		a.put(r.x+1, y0, maxX, "No sessions yet", a.th.muted)
		a.put(r.x+1, y0+1, maxX, "n starts one", a.th.muted)
		a.put(r.x+1, y0+2, maxX, "Del removes one", a.th.muted)
		return
	}
	top := max(0, s.sel-rows+1)
	focused := !a.tabFocused && a.focusedView() == viewAI
	for i := top; i < len(list) && i < top+rows; i++ {
		e := list[i]
		style := a.th.primary
		if e.status == session.Attention {
			style = a.th.success
		}
		if i == s.sel && focused {
			style = a.th.active
		}
		row := y0 + i - top
		x := a.put(r.x+1, row, maxX-3, e.marker()+" "+e.Name, style)
		if e.Dir != a.dirPath() {
			a.put(x, row, maxX-3, "  "+filepath.Base(e.Dir), a.th.muted)
		}
		a.put(maxX-3, row, maxX, " × ", a.th.err)
		a.hits = append(a.hits, hit{area: rect{maxX - 3, row, 3, 1}, click: func(int, int, tcell.ModMask) { a.hideEntry(e) }})
	}
	a.hits = append(a.hits, hit{
		area: rect{r.x, y0, r.w, rows},
		click: func(_, y int, _ tcell.ModMask) {
			if i := top + y - y0; i >= 0 && i < len(list) {
				s.sel = i
				a.openEntry(list[i])
			}
		},
		wheel: func(delta int) { s.sel = max(0, min(s.sel+delta, len(list)-1)) },
	})
}

// Time.

// isViewing reports whether the user is in this tab now. Notifications skip it (doc 10).
func (a *app) isViewing(it *tabItem) bool { return a.tabFocused && a.tab() == it }

// isVisible reports whether a tab is on screen, even if another has focus.
func (a *app) isVisible(it *tabItem) bool {
	return slices.ContainsFunc(a.shownPanes(a.dir()), func(p *pane) bool { return p.current() == it })
}

// housekeeping runs once a second, keeping each session's status up to date (doc 10). Detection is
// approximate (Q-72): a bell, or a stretch of output followed by quiet.
func (a *app) housekeeping(now time.Time) {
	for _, e := range slices.Clone(a.sessions.entries) {
		if e.tab == nil || e.tab.term == nil {
			continue
		}
		a.updateEntry(e, now)
		if e.tab != nil && !e.isShell {
			a.maybeSleep(e, now)
		}
	}
}

func (a *app) updateEntry(e *sessionEntry, now time.Time) {
	t := e.tab.term
	viewing := a.isViewing(e.tab)
	if done, code := t.Exited(); done {
		if e.status != session.Exited {
			e.status = session.Exited
			if !e.isShell && !e.resumedAt.IsZero() && code != 0 && now.Sub(e.resumedAt) < failedResume {
				a.message = e.Name + " didn't resume. Its tab shows why, and n starts a new session."
			} else if !e.isShell && !viewing {
				a.sessions.send(e.Name, "Finished")
			}
		}
		return
	}
	if viewing || e.isShell {
		if e.status == session.Attention {
			e.status = session.Running
		}
		e.seenOutput, e.seenBells = now, t.Bells()
		return
	}
	output := t.LastOutput()
	fresh := output.After(e.seenOutput) && output.After(e.ignoreUntil)
	if e.status == session.Attention && fresh {
		e.status = session.Running // It's working again.
	}
	bell := t.Bells() > e.seenBells
	if bell || (fresh && now.Sub(output) >= quietPeriod) {
		e.status = session.Attention
		e.seenOutput, e.seenBells = now, t.Bells()
		a.sessions.send(e.Name, "Finished, or asking for something")
	}
}

// maybeSleep stops a session once Q-77's rules hold, measuring CPU use over a while first.
func (a *app) maybeSleep(e *sessionEntry, now time.Time) {
	h, ok := a.harness(e.Harness)
	after := time.Duration(a.cfg.Sessions.SleepAfterMinutes) * time.Minute
	check := session.SleepCheck{
		After:     after,
		SinceOut:  now.Sub(e.tab.term.LastOutput()),
		Focused:   a.isVisible(e.tab),
		CanResume: ok && h.CanResume(),
	}
	if done, _ := e.tab.term.Exited(); done {
		return
	}
	// Everything but CPU use has to hold before measuring it, which isn't free.
	check.SampleOK = true
	if !session.ShouldSleep(check) {
		e.cpuAt = time.Time{}
		return
	}
	if !e.cpuAt.IsZero() && now.Sub(e.cpuAt) < sleepSample {
		return
	}
	cpu, err := a.sessions.cpuOf(e.tab.term.Pid())
	if err != nil {
		e.cpuAt = time.Time{}
		return // Unknown means stay awake.
	}
	check.CPUSamples = [2]time.Duration{e.cpu, cpu}
	check.SampleOK = !e.cpuAt.IsZero()
	e.cpu, e.cpuAt = cpu, now
	if session.ShouldSleep(check) {
		a.sleepEntry(e)
	}
}

// sleepEntry stops the tool and frees its screen. Selecting it resumes the same chat.
func (a *app) sleepEntry(e *sessionEntry) {
	st, p := a.findTab(e.tab)
	if st != nil {
		a.removeTab(st, p, e.tab) // Through tabClosed, which marks it as sleeping.
	}
}

// Quitting.

// quitCommand warns if a session looks like it's still working (doc 10), then asks about files.
func (a *app) quitCommand() {
	working := 0
	for _, e := range a.sessions.entries {
		if !e.isShell && e.tab != nil && e.tab.term != nil {
			if done, _ := e.tab.term.Exited(); !done && time.Since(e.tab.term.LastOutput()) < quietPeriod {
				working++
			}
		}
	}
	if working == 0 {
		a.askQuit(map[*buffer.File]bool{})
		return
	}
	a.ask(fmt.Sprintf("%d session(s) look busy, and quitting stops them.", working),
		choice{'q', "Quit", func() { a.askQuit(map[*buffer.File]bool{}) }},
		choice{'c', "Cancel", func() {}})
}
