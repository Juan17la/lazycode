package app

import (
	"runtime/debug"
	"slices"
	"time"

	"github.com/Juan17la/lazycode/internal/editor"
	"github.com/gdamore/tcell/v2"
)

// Below this width panels hide, the second first (doc 07).
const (
	minWidthForPanel       = 40
	minWidthForSecondPanel = 80
	maxPanelWidth          = 32
)

type rect struct{ x, y, w, h int }

func (r rect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

// pane is one editor area with its own tab bar. It's mutable: splitting a pane leaves it
// in place and adds another.
type pane struct {
	tabs []*tabItem
	cur  int
	area rect // Set by layout, including the tab bar.
	// Where each tab was drawn, so that dragging one can tell what it's over.
	spans []tabSpan
}

type tabSpan struct {
	it   *tabItem
	x, w int
}

func (p *pane) current() *tabItem {
	if p.cur < len(p.tabs) {
		return p.tabs[p.cur]
	}
	return nil
}

// content returns the area below the tab bar.
func (p *pane) content() rect {
	return rect{p.area.x, p.area.y + 1, max(1, p.area.w), max(1, p.area.h-1)}
}

// split is a node in the tree of editor areas (Q-15): a pane, or two children. ratio is the first
// child's share, set by dragging the divider, and 0 means an even split.
type split struct {
	pane          *pane
	sideBySide    bool
	first, second *split
	ratio         float64
}

// divider is a line between two areas. drag moves it, if it can be moved.
type divider struct {
	r    rect
	drag func(x, y int)
}

// firstShare is how much of size the first child gets, leaving room for the line.
func (s *split) firstShare(size int) int {
	ratio := s.ratio
	if ratio <= 0 || ratio >= 1 { // Even, including anything odd from the saved state.
		return max(0, (size-1)/2)
	}
	return max(1, min(int(ratio*float64(size)), size-2))
}

func (s *split) panes() []*pane {
	if s.pane != nil {
		return []*pane{s.pane}
	}
	return append(s.first.panes(), s.second.panes()...)
}

// find returns the node holding p.
func (s *split) find(p *pane) *split {
	if s.pane == p {
		return s
	}
	if s.pane != nil {
		return nil
	}
	if found := s.first.find(p); found != nil {
		return found
	}
	return s.second.find(p)
}

// without returns the tree with p's node removed, its sibling taking the space.
func (s *split) without(p *pane) *split {
	if s.pane != nil {
		return s
	}
	if s.first.pane == p {
		return s.second
	}
	if s.second.pane == p {
		return s.first
	}
	s.first, s.second = s.first.without(p), s.second.without(p)
	return s
}

// place sets each pane's area, and reports the lines dividing them.
func (s *split) place(r rect, add func(divider)) {
	if s.pane != nil {
		s.pane.area = r
		return
	}
	if s.sideBySide {
		left := s.firstShare(r.w)
		add(divider{rect{r.x + left, r.y, 1, r.h}, func(x, _ int) { s.ratio = float64(x-r.x) / float64(max(1, r.w)) }})
		s.first.place(rect{r.x, r.y, left, r.h}, add)
		s.second.place(rect{r.x + left + 1, r.y, r.w - left - 1, r.h}, add)
		return
	}
	top := s.firstShare(r.h)
	add(divider{rect{r.x, r.y + top, r.w, 1}, func(_, y int) { s.ratio = float64(y-r.y) / float64(max(1, r.h)) }})
	s.first.place(rect{r.x, r.y, r.w, top}, add)
	s.second.place(rect{r.x, r.y + top + 1, r.w, r.h - top - 1}, add)
}

// resolution is what ui.resolution means for this window. A terminal app can't change pixels or font
// size, so this is how much room the panels and the dock take. auto follows the window's width.
func (a *app) resolution() string {
	if r := a.cfg.UI.Resolution; r != "auto" && r != "" {
		return r
	}
	switch w, _ := a.screen.Size(); {
	case w < 80:
		return "compact"
	case w >= 180:
		return "spacious"
	}
	return "normal"
}

// panelWidth is a panel's width: the one dragged to, or else the resolution's.
func (a *app) panelWidth(w int) int {
	if a.panelW > 0 {
		return max(10, min(a.panelW, w/2))
	}
	switch a.resolution() {
	case "compact":
		return min(24, w/3)
	case "spacious":
		return min(48, w/4)
	}
	return min(maxPanelWidth, w/3)
}

// regions returns the areas of the two panels, which are empty if hidden, and of the
// tabs. Title and status take the first and last lines.
func (a *app) regions() (first, second, tabs rect) {
	w, h := a.screen.Size()
	tabs = rect{0, 1, w, max(0, h-2)}
	if w < minWidthForPanel {
		return first, second, tabs
	}
	width := a.panelWidth(w)
	onLeft := a.cfg.UI.Sidebar != "right"
	first = rect{0, 1, width, tabs.h}
	if !onLeft {
		first.x = w - width
	} else {
		tabs.x = width + 1
	}
	tabs.w -= width + 1
	if a.showSecond && w >= minWidthForSecondPanel {
		second = rect{0, 1, width, tabs.h}
		if onLeft {
			second.x = w - width
		} else {
			tabs.x = width + 1
		}
		tabs.w -= width + 1
	}
	return first, second, tabs
}

// dockArea cuts the tabs' area into code above and the dock below, with a line between them. With no
// file open, the dock has nearly all of it, because the space above would only hold a hint.
func (a *app) dockArea(st *dirState, tabs rect) (code, line, dock rect) {
	height := tabs.h * 2 / 5
	switch {
	case a.dockH > 0:
		height = a.dockH
	case a.resolution() == "compact":
		height = tabs.h / 3
	case !slices.ContainsFunc(st.root.panes(), func(p *pane) bool { return len(p.tabs) > 0 }):
		height = tabs.h - 4
	}
	height = max(min(3, tabs.h/2), min(height, tabs.h-3))
	top := tabs.y + tabs.h - height
	return rect{tabs.x, tabs.y, tabs.w, tabs.h - height - 1}, rect{tabs.x, top - 1, tabs.w, 1}, rect{tabs.x, top, tabs.w, height}
}

// layout places the panes of a directory for this frame.
func (a *app) layout(st *dirState) (dividers []divider) {
	_, _, tabs := a.regions()
	add := func(d divider) { dividers = append(dividers, d) }
	if a.dockVisible(st) {
		code, line, dock := a.dockArea(st, tabs)
		tabs = code
		st.dock.area = dock
		bottom := dock.y + dock.h
		add(divider{line, func(_, y int) { a.dockH = bottom - y - 1 }})
	}
	st.root.place(tabs, add)
	return dividers
}

// The dock: terminal and AI tabs, under the code (a line away from the code's own tabs).

// panes returns every pane of a directory, shown or not.
func (st *dirState) panes() []*pane { return append(st.root.panes(), st.dock) }

// dockVisible reports whether the dock is on screen. With no terminals, it takes no room.
func (a *app) dockVisible(st *dirState) bool { return len(st.dock.tabs) > 0 && !a.dockHidden }

// shownPanes returns the panes on screen.
func (a *app) shownPanes(st *dirState) []*pane {
	if a.dockVisible(st) {
		return st.panes()
	}
	return st.root.panes()
}

// toggleDock shows or hides the dock. Tools keep running while it's hidden. With none yet, it starts a shell.
func (a *app) toggleDock() {
	st := a.dir()
	switch {
	case len(st.dock.tabs) == 0:
		a.dockHidden = false
		a.startTab(nil)
	case a.dockHidden:
		a.dockHidden = false
		st.focus = st.dock
		a.focusTab()
	default:
		a.dockHidden = true
		a.leaveDock(st)
	}
}

// leaveDock gives focus back to the code, if the dock has it.
func (a *app) leaveDock(st *dirState) {
	if st.focus == st.dock {
		st.focus = st.root.panes()[0]
		a.tabFocused = a.tabFocused && st.focus.current() != nil
	}
}

// moveTabTo moves a dragged tab to where another tab is, in its own pane.
func moveTabTo(p *pane, it *tabItem, x int) {
	for _, span := range p.spans {
		if x >= span.x && x < span.x+span.w && span.it != it {
			from, to := slices.Index(p.tabs, it), slices.Index(p.tabs, span.it)
			if from < 0 || to < 0 {
				return
			}
			p.tabs = slices.Insert(slices.Delete(p.tabs, from, from+1), to, it)
			p.cur = to
			return
		}
	}
}

// dragTab follows a tab being dragged, along its bar or to another split's. It stays on its side of
// the dock: code above and terminals below is the layout's rule (G1). It returns the tab's pane now.
func (a *app) dragTab(st *dirState, from *pane, it *tabItem, x, y int) *pane {
	if (rect{from.area.x, from.area.y, from.area.w, 1}).contains(x, y) || from == st.dock {
		moveTabTo(from, it, x)
		return from
	}
	for _, to := range st.root.panes() {
		bar := rect{to.area.x, to.area.y, to.area.w, 1}
		if to == from || !bar.contains(x, y) || holdsFile(to, it) {
			continue
		}
		from.tabs = slices.DeleteFunc(from.tabs, func(other *tabItem) bool { return other == it })
		from.spans = nil
		from.cur = max(0, min(from.cur, len(from.tabs)-1))
		to.tabs = append(to.tabs, it)
		to.cur = len(to.tabs) - 1
		st.focus = to
		if len(from.tabs) == 0 {
			st.root = st.root.without(from)
		}
		return to
	}
	return from
}

// holdsFile reports whether a pane already shows the tab's file, which one pane never does twice.
func holdsFile(p *pane, it *tabItem) bool {
	return slices.ContainsFunc(p.tabs, func(other *tabItem) bool {
		return it.ed != nil && other.ed != nil && other.ed.File == it.ed.File
	})
}

// resizeTabs tells terminal tabs in this directory about their pane's size. Tabs in
// other directories catch up when they're next shown.
func (a *app) resizeTabs(st *dirState) {
	for _, p := range a.shownPanes(st) {
		size := p.content()
		for _, it := range p.tabs {
			if it.term != nil && (it.w != size.w || it.h != size.h) {
				it.term.Resize(size.w, size.h)
				it.w, it.h = size.w, size.h
				if it.sess != nil { // A tool redraws when resized, which isn't it working.
					it.sess.ignoreUntil = time.Now().Add(resizeNoise)
				}
			}
		}
	}
}

// Splits.

// splitPane adds a pane beside or below the focused one. A file gets a second view, as in
// VS Code. A terminal can only be shown in one place, so the new pane starts empty.
func (a *app) splitPane(sideBySide bool) {
	st := a.dir()
	a.leaveDock(st) // The dock holds one row of tabs: only code splits.
	node := st.root.find(st.focus)
	fresh := &pane{}
	if it := st.focus.current(); it != nil && it.ed != nil {
		view := editor.New(it.ed.File, it.ed.TabSize)
		line, col := it.ed.Position()
		view.GoTo(line, col, false)
		view.ScrollLine = it.ed.ScrollLine
		fresh.tabs = []*tabItem{{ed: view}}
		a.tabFocused = true
	} else {
		a.tabFocused = false
		a.message = "Empty split. Enter opens a file here, and :term starts a tool."
	}
	node.first, node.second = &split{pane: st.focus}, &split{pane: fresh}
	node.pane, node.sideBySide = nil, sideBySide
	st.focus = fresh
}

// nextPane moves focus to the next pane, going round, and through the dock.
func (a *app) nextPane() {
	st := a.dir()
	panes := a.shownPanes(st)
	if len(panes) == 1 {
		a.message = "There's only one split. " + a.keyFor("split.right") + " adds another."
		return
	}
	st.focus = panes[(slices.Index(panes, st.focus)+1)%len(panes)]
	if st.focus.current() != nil {
		a.focusTab()
	} else {
		a.tabFocused = false
		a.message = "Empty split. Enter opens a file here, and :term starts a tool."
	}
}

// Tabs.

// moveTab goes to the previous or next tab in this pane, and focuses it.
func (a *app) moveTab(delta int) {
	p := a.dir().focus
	if len(p.tabs) == 0 {
		a.message = "No tabs here. Enter opens a file, and :term starts a tool."
		return
	}
	p.cur = (p.cur + delta + len(p.tabs)) % len(p.tabs)
	a.focusTab()
}

// removeTab closes a tab. A pane left empty goes too, unless it's the only one.
func (a *app) removeTab(st *dirState, p *pane, it *tabItem) {
	i := slices.Index(p.tabs, it)
	if i < 0 {
		return
	}
	lastView := it.ed != nil && a.lastView(it)
	if lastView {
		delete(a.marks, it.ed.File)
	}
	a.tabClosed(st, it)
	p.tabs = slices.Delete(p.tabs, i, i+1) // Clears the old slot, so the tab can be freed.
	p.spans = nil                          // A hidden or empty pane isn't drawn again to clear them.
	p.cur = max(0, min(p.cur, len(p.tabs)-1))
	if p == st.dock && len(p.tabs) == 0 {
		a.leaveDock(st)
	} else if len(p.tabs) == 0 && st.root.pane == nil {
		st.root = st.root.without(p)
		if st.focus == p {
			st.focus = st.root.panes()[0]
			if st.focus.current() == nil {
				a.tabFocused = false
			}
		}
	}
	go func() { // It can wait for a stubborn tool, and the UI mustn't.
		switch {
		case it.term != nil:
			it.term.Close()
		case lastView:
			it.ed.File.Close() //nolint:errcheck // Only frees memory, and the tab is gone.
		}
		debug.FreeOSMemory() // Closing is when users expect RAM back (D-28), not minutes later.
	}()
}

// Panels (Q-18). Four views share the panels: each sits in one, and one of the panel's views is on top.

const (
	viewFiles  = "Files"
	viewGit    = "Git"
	viewSearch = "Search"
	viewAI     = "AI"
)

// allViews are in the order the header row shows them (doc 07).
var allViews = []string{viewFiles, viewGit, viewSearch, viewAI}

// viewsIn returns the views in a panel, in header order.
func (a *app) viewsIn(panel int) []string {
	var in []string
	for _, v := range allViews {
		if a.panelOf[v] == panel {
			in = append(in, v)
		}
	}
	return in
}

// viewIn returns the view showing in a panel, or "" if it has none.
func (a *app) viewIn(panel int) string {
	in := a.viewsIn(panel)
	if len(in) == 0 {
		return ""
	}
	if slices.Contains(in, a.onTop[panel]) {
		return a.onTop[panel]
	}
	return in[0]
}

// focusedView returns the view that keys go to when a tab hasn't got them.
func (a *app) focusedView() string {
	if view := a.viewIn(a.focusPanel); view != "" {
		return view
	}
	return a.viewIn(1 - a.focusPanel)
}

// showView brings a view to the front of its panel, opening the panel if needed, and focuses it.
func (a *app) showView(view string) {
	panel := a.panelOf[view]
	a.onTop[panel] = view
	a.focusPanel = panel
	a.tabFocused = false
	a.closeFind()
	if panel == 1 {
		a.showSecond = true
	}
}

func (a *app) toggleSecondPanel() {
	a.showSecond = !a.showSecond
	if !a.showSecond {
		// What's showing stays, unless the closing panel held Files, which comes back on top.
		a.onTop[0] = a.viewIn(0)
		if a.panelOf[viewFiles] == 1 {
			a.onTop[0] = viewFiles
		}
		clear(a.panelOf) // Closing a panel can't lose what's in it.
		a.focusPanel = 0
	}
	if w, _ := a.screen.Size(); a.showSecond && w < minWidthForSecondPanel {
		a.message = "This window is too narrow for a second panel."
	}
}

// setFilesPanel puts Files, Git and Search together in a panel, as the mockups in doc 07 show them.
func (a *app) setFilesPanel(panel int) {
	for _, v := range []string{viewFiles, viewGit, viewSearch} {
		a.panelOf[v] = panel
	}
}

// moveFilesView moves the focused view to the other panel, opening it if needed. AI moves alone, and
// the other three move together. The Files group is what moves when focus is on a tab.
func (a *app) moveFilesView() {
	if !a.tabFocused && a.focusedView() == viewAI {
		a.panelOf[viewAI] = 1 - a.panelOf[viewAI]
		a.focusPanel = a.panelOf[viewAI]
		a.onTop[a.focusPanel] = viewAI
	} else {
		view := a.focusedView()
		if view == "" || view == viewAI {
			view = viewFiles
		}
		a.setFilesPanel(1 - a.panelOf[view])
		a.focusPanel = a.panelOf[view]
		a.onTop[a.focusPanel] = view
	}
	if a.panelOf[viewFiles] == 1 || a.panelOf[viewAI] == 1 {
		a.showSecond = true
	}
}

// drawViewTitles draws the header row of a panel. A click on an icon brings that view forward.
func (a *app) drawViewTitles(panel int, r rect) {
	top := a.viewIn(panel)
	focused := !a.tabFocused && a.focusedView() == top
	x, maxX := r.x+1, r.x+r.w
	for _, v := range a.viewsIn(panel) {
		// The icon alone, except the view on top, which also says its name (the part that isn't colour).
		label, style := " "+a.viewIcon(v)+" ", a.th.icon[viewColour[v]]
		if v == top {
			label, style = " "+a.viewIcon(v)+" "+v+" ", a.th.icon[viewColour[v]].Bold(true)
			if v == viewFiles && a.showAll {
				label = " " + a.viewIcon(v) + " Files, all directories "
			}
			if focused {
				style = style.Underline(true)
			}
		}
		start := x
		x = a.put(x, r.y, maxX, label, style)
		view := v
		a.hits = append(a.hits, hit{area: rect{start, r.y, x - start, 1}, click: func(int, int, tcell.ModMask) { a.showView(view) }})
	}
}

// handlePanelKey gives keys to the Git and Search views, which have their own. It reports whether it
// used the key.
func (a *app) handlePanelKey(ev *tcell.EventKey) bool {
	switch a.focusedView() {
	case viewSearch:
		return a.searchKey(ev)
	case viewGit:
		return a.gitKey(ev)
	}
	return false
}
