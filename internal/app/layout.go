package app

import (
	"runtime/debug"
	"slices"

	"github.com/Juan17la/lazycode/internal/editor"
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

// split is a node in the tree of editor areas (Q-15): a pane, or two children. Dragging
// can set a ratio here later (Q-13). For now they share the space evenly.
type split struct {
	pane          *pane
	sideBySide    bool
	first, second *split
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
func (s *split) place(r rect, divider func(rect)) {
	if s.pane != nil {
		s.pane.area = r
		return
	}
	if s.sideBySide {
		left := max(0, (r.w-1)/2)
		divider(rect{r.x + left, r.y, 1, r.h})
		s.first.place(rect{r.x, r.y, left, r.h}, divider)
		s.second.place(rect{r.x + left + 1, r.y, r.w - left - 1, r.h}, divider)
		return
	}
	top := max(0, (r.h-1)/2)
	divider(rect{r.x, r.y + top, r.w, 1})
	s.first.place(rect{r.x, r.y, r.w, top}, divider)
	s.second.place(rect{r.x, r.y + top + 1, r.w, r.h - top - 1}, divider)
}

// regions returns the areas of the two panels, which are empty if hidden, and of the
// tabs. Title and status take the first and last lines.
func (a *app) regions() (first, second, tabs rect) {
	w, h := a.screen.Size()
	tabs = rect{0, 1, w, max(0, h-2)}
	if w < minWidthForPanel {
		return first, second, tabs
	}
	width := min(maxPanelWidth, w/3)
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

// layout places the panes of a directory for this frame.
func (a *app) layout(st *dirState) (dividers []rect) {
	_, _, tabs := a.regions()
	st.root.place(tabs, func(r rect) { dividers = append(dividers, r) })
	return dividers
}

// resizeTabs tells terminal tabs in this directory about their pane's size. Tabs in
// other directories catch up when they're next shown.
func (a *app) resizeTabs(st *dirState) {
	for _, p := range st.root.panes() {
		size := p.content()
		for _, it := range p.tabs {
			if it.term != nil && (it.w != size.w || it.h != size.h) {
				it.term.Resize(size.w, size.h)
				it.w, it.h = size.w, size.h
			}
		}
	}
}

// Splits.

// splitPane adds a pane beside or below the focused one. A file gets a second view, as in
// VS Code. A terminal can only be shown in one place, so the new pane starts empty.
func (a *app) splitPane(sideBySide bool) {
	st := a.dir()
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

// nextPane moves focus to the next pane, going round.
func (a *app) nextPane() {
	st := a.dir()
	panes := st.root.panes()
	if len(panes) == 1 {
		a.message = "There's only one split. Alt+S adds another."
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
	p.tabs = slices.Delete(p.tabs, i, i+1) // Clears the old slot, so the tab can be freed.
	p.cur = max(0, min(p.cur, len(p.tabs)-1))
	if len(p.tabs) == 0 && st.root.pane == nil {
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

// Panels (Q-18). Only the Files view exists so far. Git, search and AI sessions join it
// in tracks A to D.

func (a *app) toggleSecondPanel() {
	a.showSecond = !a.showSecond
	if !a.showSecond && a.filesPanel == 1 {
		a.filesPanel = 0 // Closing a panel can't lose what's in it.
	}
	if w, _ := a.screen.Size(); a.showSecond && w < minWidthForSecondPanel {
		a.message = "This window is too narrow for a second panel."
	}
}

// moveFilesView moves the Files view to the other panel, opening it if needed.
func (a *app) moveFilesView() {
	a.filesPanel = 1 - a.filesPanel
	if a.filesPanel == 1 {
		a.showSecond = true
	}
}
