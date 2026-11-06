package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

// minWidthForMenuNames is where the menu bar's names give way to one ≡ holding everything.
const minWidthForMenuNames = 70

// menuItem runs an action from the keymap, or something of its own. An empty one is a separator.
type menuItem struct {
	label, action string
	run           func()
	checked       func(a *app) bool
}

type menuGroup struct {
	name  string
	items []menuItem
}

var menuBar = []menuGroup{
	{"File", []menuItem{
		{label: "New File…", action: "file.new"},
		{label: "Open File by Name…", action: "file.open_by_name"},
		{label: "Save", action: "file.save"},
		{label: "Close Tab", action: "tab.close"},
		{},
		{label: "New Project…", action: "project.new"},
		{label: "Add Directory…", action: "directory.add"},
		{label: "Switch Project…", action: "project.switch"},
		{label: "Switch Directory…", action: "directory.switch"},
		{},
		{label: "Quit", action: "app.quit"},
	}},
	{"Edit", []menuItem{
		{label: "Undo", action: "edit.undo"},
		{label: "Redo", action: "edit.redo"},
		{},
		{label: "Cut", action: "edit.cut"},
		{label: "Copy", action: "edit.copy"},
		{label: "Paste", action: "edit.paste"},
		{label: "Select All", action: "select.all"},
		{},
		{label: "Find", action: "find.open"},
		{label: "Replace", action: "find.replace"},
		{label: "Find in Directory", action: "view.search"},
	}},
	{"View", []menuItem{
		{label: "Explorer", action: "view.files"},
		{label: "Git", action: "view.git"},
		{label: "AI Sessions", action: "view.sessions"},
		{label: "Terminal Dock", action: "view.terminal", checked: func(a *app) bool { return a.dockVisible(a.dir()) }},
		{label: "Second Panel", action: "panel.toggle_second", checked: func(a *app) bool { return a.showSecond }},
		{},
		{label: "Split Right", action: "split.right"},
		{label: "Split Below", action: "split.down"},
		{label: "Word Wrap", action: "view.wrap", checked: func(a *app) bool { return a.cfg.Editor.WordWrap }},
		{},
		{label: "Command Bar", action: "command.open"},
	}},
	{"Run", []menuItem{
		{label: "Run This Directory", action: "run.play"},
		{label: "New Terminal", action: "terminal.new"},
		{label: "New AI Session…", action: "session.new"},
	}},
	{"Tools", []menuItem{
		{label: "Settings…", action: "app.settings"},
		{label: "Appearance…", action: "app.appearance"},
		{label: "Themes…", action: "app.themes"},
		{label: "Plugins…", action: "app.plugins"},
		{label: "Plugin Store…", action: "app.store"},
	}},
	{"Help", []menuItem{
		{label: "Shortcuts…", action: "app.shortcuts"},
		{label: "Welcome Screen…", action: "app.onboarding"},
	}},
}

// openMenu is a menu on screen: from the bar, or a context menu at the mouse.
type openMenu struct {
	group int // In the bar, or -1.
	x, y  int
	items []menuItem
	sel   int
}

func (m *openMenu) settle(direction int) {
	if len(m.items) == 0 {
		return
	}
	for m.items[m.sel].label == "" {
		m.sel = (m.sel + direction + len(m.items)) % len(m.items)
	}
}

// allMenus is every menu together, for the ≡ of a narrow window.
func allMenus() []menuItem {
	var items []menuItem
	for i, group := range menuBar {
		if i > 0 {
			items = append(items, menuItem{})
		}
		items = append(items, group.items...)
	}
	return items
}

// openMenuBar opens a menu by its place in the bar. x is where its name was drawn, if known.
func (a *app) openMenuBar(group int) {
	group = (group + len(menuBar)) % len(menuBar)
	a.closeFind()
	w, _ := a.screen.Size()
	if w < minWidthForMenuNames {
		a.menu = &openMenu{group: 0, x: 0, y: 1, items: allMenus()}
		return
	}
	x := 1
	for _, g := range menuBar[:group] {
		x += runewidth.StringWidth(g.name) + 2
	}
	a.menu = &openMenu{group: group, x: x, y: 1, items: menuBar[group].items}
}

func (a *app) runMenuItem(item menuItem) {
	a.menu = nil
	if item.run != nil {
		item.run()
		return
	}
	a.do(item.action)
}

func (a *app) handleMenuKey(ev *tcell.EventKey) {
	m := a.menu
	switch ev.Key() {
	case tcell.KeyEsc:
		a.menu = nil
	case tcell.KeyUp:
		m.sel = (m.sel + len(m.items) - 1) % len(m.items)
		m.settle(-1)
	case tcell.KeyDown, tcell.KeyTab:
		m.sel = (m.sel + 1) % len(m.items)
		m.settle(1)
	case tcell.KeyLeft:
		if m.group >= 0 {
			a.openMenuBar(m.group - 1)
		}
	case tcell.KeyRight:
		if m.group >= 0 {
			a.openMenuBar(m.group + 1)
		}
	case tcell.KeyEnter:
		a.runMenuItem(m.items[m.sel])
	}
}

// drawMenuBar is the top line: the menus, where we are, and the Git branch and buttons at the far end.
func (a *app) drawMenuBar(w int) {
	x := 0
	if w < minWidthForMenuNames {
		x = a.put(0, 0, w, " ≡ ", a.th.accent)
		a.hits = append(a.hits, hit{area: rect{0, 0, x, 1}, click: func(int, int, tcell.ModMask) { a.openMenuBar(0) }})
	} else {
		x = 1
		for i, group := range menuBar {
			style := a.th.primary
			if a.menu != nil && a.menu.group == i {
				style = a.pill("accent")
			}
			start := x
			x = a.put(x, 0, w, " "+group.name+" ", style)
			at := i
			a.hits = append(a.hits, hit{area: rect{start, 0, x - start, 1}, click: func(int, int, tcell.ModMask) {
				if a.menu != nil && a.menu.group == at {
					a.menu = nil
					return
				}
				a.openMenuBar(at)
			}})
		}
		x += 2
	}
	// The buttons at the far end stay, whatever else has to give way.
	right := w
	button := func(label string, style tcell.Style, action string) {
		right -= runewidth.StringWidth(label)
		if right < 0 {
			return
		}
		a.put(right, 0, w, label, style)
		a.hits = append(a.hits, hit{area: rect{right, 0, runewidth.StringWidth(label), 1}, click: func(int, int, tcell.ModMask) { a.do(action) }})
	}
	button(" ? ", a.th.icon["green"], "app.shortcuts")
	button(" ⚙ ", a.th.icon["cyan"], "app.settings")
	maxX := max(0, right)
	x = a.put(x, 0, maxX, "✻ ", a.th.icon["orange"])
	x = a.put(x, 0, maxX, a.projects[a.project].Name, a.th.accent)
	x = a.put(x, 0, maxX, " › ", a.th.muted)
	x = a.put(x, 0, maxX, filepath.Base(a.dirPath()), a.th.primary)
	if branch := a.branchLabel(); branch != "" {
		a.put(max(x+2, maxX-runewidth.StringWidth(branch)-1), 0, maxX, branch, a.th.icon["purple"])
	}
}

// drawMenu draws the open menu, with each item's key beside it. A click anywhere else closes it.
func (a *app) drawMenu(w, h int) {
	m := a.menu
	labelWidth, keyWidth := 0, 0
	for _, item := range m.items {
		labelWidth = max(labelWidth, runewidth.StringWidth(item.label))
		keyWidth = max(keyWidth, runewidth.StringWidth(a.keyFor(item.action)))
	}
	bw := min(labelWidth+keyWidth+8, w)
	bh := min(len(m.items)+2, h-m.y)
	x0 := max(0, min(m.x, w-bw))
	y0 := max(0, min(m.y, h-bh))
	a.hits = append(a.hits[:0], hit{area: rect{0, 0, w, h}, click: func(int, int, tcell.ModMask) { a.menu = nil }})
	if m.group >= 0 { // So another name can be clicked while a menu is open.
		a.drawMenuBar(w)
	}
	a.frame(rect{x0, y0, bw, bh}, a.th.border)
	a.hits = append(a.hits, hit{area: rect{x0, y0, bw, bh}, click: func(int, int, tcell.ModMask) {}})
	maxX := x0 + bw - 1
	for i, item := range m.items {
		y := y0 + 1 + i
		if y >= y0+bh-1 {
			break
		}
		if item.label == "" {
			for x := x0 + 1; x < maxX; x++ {
				a.screen.SetContent(x, y, '─', nil, a.th.muted)
			}
			continue
		}
		style, keyStyle := a.th.primary, a.th.icon["cyan"]
		if i == m.sel {
			style, keyStyle = a.th.active, a.th.active
			for x := x0 + 1; x < maxX; x++ {
				a.screen.SetContent(x, y, ' ', nil, a.th.active)
			}
		}
		mark := "  "
		if item.checked != nil && item.checked(a) {
			mark = "✓ "
		}
		x := a.put(x0+1, y, maxX, mark, style)
		a.put(x, y, maxX, item.label, style)
		key := a.keyFor(item.action)
		a.put(maxX-runewidth.StringWidth(key)-1, y, maxX, key, keyStyle)
		chosen := item
		a.hits = append(a.hits, hit{area: rect{x0 + 1, y, bw - 2, 1}, click: func(int, int, tcell.ModMask) { a.runMenuItem(chosen) }})
	}
}

// statusSegment is a part of the right end of the status line.
type statusSegment struct {
	text  string
	style tcell.Style
	click func()
}

// drawSegments draws them from the right edge, and returns the column left free.
func (a *app) drawSegments(segments []statusSegment, y, w int) int {
	x := w
	for i := len(segments) - 1; i >= 0; i-- {
		s := segments[i]
		width := runewidth.StringWidth(s.text)
		if x-width < 1 {
			break
		}
		x -= width
		a.put(x, y, w, s.text, s.style)
		if s.click != nil {
			click := s.click
			a.hits = append(a.hits, hit{area: rect{x, y, width, 1}, click: func(int, int, tcell.ModMask) { click() }})
		}
	}
	return x
}

// mode names where the keys are going, in the colour used for that part of the app.
func (a *app) mode() (label, colour string) {
	it := a.tab()
	switch {
	case a.tabFocused && it != nil && it.ed != nil:
		return "EDIT", "blue"
	case a.tabFocused && it != nil && it.sess != nil && !it.sess.isShell:
		return "AI", "purple"
	case a.tabFocused && it != nil:
		return "TERM", "cyan"
	}
	switch view := a.focusedView(); view {
	case viewGit, viewSearch, viewAI:
		return strings.ToUpper(view), viewColour[view]
	}
	return "FILES", "green"
}

// dockSegment is the status line's way to the dock, and shows how many tabs it's holding while hidden.
func (a *app) dockSegment() statusSegment {
	st := a.dir()
	text := " ❯ Terminal "
	if n := len(st.dock.tabs); n > 0 {
		text = fmt.Sprintf(" ❯ %d ", n)
	}
	return statusSegment{text, a.th.icon["cyan"], a.toggleDock}
}
