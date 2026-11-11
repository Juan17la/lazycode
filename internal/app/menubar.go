package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/Juan17la/lazycode/internal/buffer"
	"github.com/Juan17la/lazycode/internal/project"
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

// minWidthForMenuNames is where the menu bar's names give way to one ≡ holding everything.
const minWidthForMenuNames = 78

// menuItem runs an action from the keymap, a : command, or something of its own. An empty one is a
// separator.
type menuItem struct {
	label, action, command string
	run                    func()
	do                     func(a *app)
	checked                func(a *app) bool
}

// menuGroup is a name in the bar. It drops down its items, or with open, shows them in a window.
type menuGroup struct {
	name  string
	items []menuItem
	open  func(a *app, items []menuItem)
}

// gitThen shows the Git view, which reads the status, and then does a Git job.
func gitThen(job func(a *app)) func(a *app) {
	return func(a *app) {
		a.do("view.git")
		job(a)
	}
}

// menuBar is filled in by init, because its items call code that opens the menus.
var menuBar []menuGroup

// What the project and folder names in the bar offer under their lists.
var projectItems, folderItems []menuItem

func init() {
	projectItems = []menuItem{
		{label: "New Project…", action: "project.new"},
		{label: "Open Folder…", command: "dir open"},
		{label: "Close Project", command: "project close"},
		{label: "Delete Project…", command: "project delete"},
	}
	folderItems = []menuItem{
		{label: "Add Folder to Project…", action: "directory.add"},
		{label: "Open Folder…", command: "dir open"},
	}
	menuBar = []menuGroup{
		{"File", []menuItem{
			{label: "New File…", action: "file.new"},
			{label: "Open File by Name…", action: "file.open_by_name"},
			{},
			{label: "Save", action: "file.save"},
			{label: "Save All", do: (*app).saveAll},
			{},
			{label: "Close Tab", action: "tab.close"},
			{label: "Close Saved Tabs", do: (*app).closeSavedTabs},
			{},
			{label: "Settings…", action: "app.settings"},
			{label: "Plugins…", action: "app.plugins"},
			{},
			{label: "Quit", action: "app.quit"},
		}, nil},
		{"Project", []menuItem{
			{label: "Switch Project…", action: "project.switch"},
			{label: "Switch Folder…", action: "directory.switch"},
			{},
			{label: "New Project…", action: "project.new"},
			{label: "Close Project", command: "project close"},
			{label: "Delete Project…", command: "project delete"},
			{},
			{label: "Add Folder to Project…", action: "directory.add"},
			{label: "Open Folder…", command: "dir open"},
			{label: "Remove Folder from Project", command: "dir remove"},
			{},
			{label: "All Folders in Files", command: "tree", checked: func(a *app) bool { return a.showAll }},
		}, nil},
		{"Edit", []menuItem{
			{label: "Undo", action: "edit.undo"},
			{label: "Redo", action: "edit.redo"},
			{},
			{label: "Cut", action: "edit.cut"},
			{label: "Copy", action: "edit.copy"},
			{label: "Paste", action: "edit.paste"},
			{},
			{label: "Find", action: "find.open"},
			{label: "Replace", action: "find.replace"},
			{label: "Find in Folder", action: "view.search"},
			{},
			{label: "Comment or Uncomment", action: "edit.toggle_comment"},
			{label: "Move Line Up", action: "line.move_up"},
			{label: "Move Line Down", action: "line.move_down"},
			{label: "Copy Line Down", action: "line.copy_down"},
			{label: "Delete Line", action: "line.delete"},
			{},
			{label: "Add Cursor Above", action: "cursor.add_above"},
			{label: "Add Cursor Below", action: "cursor.add_below"},
			{label: "Select All", action: "select.all"},
			{label: "Select Line", action: "select.line"},
			{label: "Add Next Match", action: "select.next_occurrence"},
			{label: "Select All Matches", action: "select.all_occurrences"},
			{label: "Select to Start of File", action: "select.file_start"},
			{label: "Select to End of File", action: "select.file_end"},
			{},
			{label: "Format File", action: "lsp.format"},
			{label: "Rename Symbol", action: "lsp.rename"},
		}, nil},
		{"View", []menuItem{
			{label: "Command Bar", action: "command.open"},
			{},
			{label: "Files", action: "view.files"},
			{label: "Search", action: "view.search"},
			{label: "Git", action: "view.git"},
			{label: "AI Sessions", action: "view.sessions"},
			{},
			{label: "Terminal Dock", action: "view.terminal", checked: func(a *app) bool { return a.dockVisible(a.dir()) }},
			{label: "Second Panel", action: "panel.toggle_second", checked: func(a *app) bool { return a.showSecond }},
			{label: "All Folders in Files", command: "tree", checked: func(a *app) bool { return a.showAll }},
			{label: "Move View to Other Panel", command: "view"},
			{},
			{label: "Split Right", action: "split.right"},
			{label: "Split Below", action: "split.down"},
			{},
			{label: "Word Wrap", action: "view.wrap", checked: func(a *app) bool { return a.cfg.Editor.WordWrap }},
			{label: "Appearance…", action: "app.appearance"},
			{label: "Themes…", action: "app.themes"},
		}, nil},
		{"Go", []menuItem{
			{label: "Go to File…", action: "file.open_by_name"},
			{label: "Go to Line…", action: "file.go_to_line"},
			{label: "Go to Definition", action: "lsp.definition"},
			{},
			{label: "Start of File", action: "move.file_start"},
			{label: "End of File", action: "move.file_end"},
			{},
			{label: "Next Tab", action: "tab.next"},
			{label: "Previous Tab", action: "tab.previous"},
			{label: "Next Area", action: "focus.next"},
		}, nil},
		{"Terminal", []menuItem{
			{label: "New Terminal", action: "terminal.new"},
			{label: "New AI Session…", action: "session.new"},
			{label: "Hidden AI Sessions…", command: "session hidden"},
			{},
			{label: "Run This Folder", action: "run.play"},
			{},
			{label: "Show or Hide Dock", action: "view.terminal"},
		}, (*app).openTerminalHub},
		{"Git", []menuItem{
			{label: "Show Changes", action: "view.git"},
			{label: "Commit…", do: gitThen((*app).gitCommit)},
			{},
			{label: "Push", command: "push"},
			{label: "Pull", command: "pull"},
			{label: "Fetch", command: "fetch"},
			{},
			{label: "Switch Branch…", do: gitThen(func(a *app) { a.gitBranches(false) })},
			{label: "New Branch…", do: gitThen((*app).gitNewBranch)},
			{label: "Merge Branch…", do: gitThen(func(a *app) { a.gitBranches(true) })},
			{},
			{label: "Create GitHub Repository", command: "repo create"},
		}, nil},
		{"Help", []menuItem{
			{label: "Welcome Screen…", action: "app.onboarding"},
			{label: "Shortcuts…", action: "app.shortcuts"},
			{label: "All Commands…", do: (*app).openCommandList},
			{},
			{label: "Documentation and Support…", do: (*app).openLinks},
			{},
			{label: "Install Latest Version", command: "update"},
			{label: "About LazyCode…", do: (*app).openAbout},
		}, (*app).openHelpHub},
	}
}

// needsFile reports whether an item can only work with a file showing, so it can look that way.
func (item menuItem) needsFile() bool {
	for _, prefix := range []string{"edit.", "select.", "find.", "lsp.", "move.", "file.save", "file.go_to_line"} {
		if strings.HasPrefix(item.action, prefix) {
			return true
		}
	}
	return false
}

// hint is what's shown at an item's right: its key, or else the command that does the same.
func (a *app) menuHint(item menuItem) string {
	return a.keyFor(item.action) // A : command would be typed into a file, so it's no hint here.
}

// openMenu is a menu on screen: from the bar, or a context menu at the mouse.
type openMenu struct {
	group int // In the bar, or -1.
	x, y  int
	items []menuItem
	sel   int
	top   int // First item on screen, in a menu too tall for the window.
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

// stepMenuBar goes to the next name along, whether it drops down or opens a window.
func (a *app) stepMenuBar(group, direction int) {
	a.openMenuBar((group + direction + len(menuBar)) % len(menuBar))
}

// openMenuBar opens a menu by its place in the bar. x is where its name was drawn, if known.
func (a *app) openMenuBar(group int) {
	group = (group + len(menuBar)) % len(menuBar)
	a.closeFind()
	if w, _ := a.screen.Size(); w >= minWidthForMenuNames && menuBar[group].open != nil {
		menuBar[group].open(a, menuBar[group].items)
		a.modals[len(a.modals)-1].bar = group + 1 // So that Left and Right go on along the bar from its window.
		return
	}
	w, _ := a.screen.Size()
	if w < minWidthForMenuNames {
		a.menu = &openMenu{group: 0, x: 0, y: 1, items: allMenus()}
		return
	}
	x := 1
	for _, g := range menuBar[:group] {
		x += runewidth.StringWidth(g.name) + 1
	}
	a.menu = &openMenu{group: group, x: x, y: 1, items: menuBar[group].items}
}

func (a *app) runMenuItem(item menuItem) {
	a.menu = nil
	switch {
	case item.run != nil:
		item.run()
	case item.do != nil:
		item.do(a)
	case item.command != "":
		a.run(item.command)
	default:
		a.do(item.action)
	}
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
			a.stepMenuBar(m.group, -1)
		}
	case tcell.KeyRight:
		if m.group >= 0 {
			a.stepMenuBar(m.group, 1)
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
			x = a.put(x, 0, w, group.name, style)
			at := i
			a.hits = append(a.hits, hit{area: rect{start, 0, x - start, 1}, click: func(int, int, tcell.ModMask) {
				if a.menu != nil && a.menu.group == at {
					a.menu = nil
					return
				}
				a.openMenuBar(at)
			}})
			x++
		}
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
	button("? ", a.th.icon["green"], "app.shortcuts")
	button("⚙ ", a.th.icon["cyan"], "app.settings")
	maxX := max(0, right)
	x = a.put(x+1, 0, maxX, "✻ ", a.th.icon["orange"])
	start := x
	x = a.put(x, 0, maxX, a.projects[a.project].Name, a.th.accent)
	a.hits = append(a.hits, hit{area: rect{start, 0, x - start, 1}, click: func(int, int, tcell.ModMask) { a.openProjectHub(projectItems) }})
	x = a.put(x, 0, maxX, " › ", a.th.muted)
	start = x
	x = a.put(x, 0, maxX, filepath.Base(a.dirPath()), a.th.primary)
	a.hits = append(a.hits, hit{area: rect{start, 0, x - start, 1}, click: func(int, int, tcell.ModMask) { a.openDirHub(folderItems) }})
	if branch := a.branchLabel(); branch != "" {
		a.put(max(x+2, maxX-runewidth.StringWidth(branch)-1), 0, maxX, branch, a.th.icon["purple"])
	}
}

// drawMenu draws the open menu, with each item's key or command beside it. A click anywhere else
// closes it, and a menu taller than the window scrolls.
func (a *app) drawMenu(w, h int) {
	m := a.menu
	labelWidth, keyWidth := 0, 0
	for _, item := range m.items {
		labelWidth = max(labelWidth, runewidth.StringWidth(item.label))
		keyWidth = max(keyWidth, runewidth.StringWidth(a.menuHint(item)))
	}
	bw := min(labelWidth+keyWidth+8, w)
	bh := max(3, min(len(m.items)+2, h-1-m.y))
	x0 := max(0, min(m.x, w-bw))
	y0 := max(0, min(m.y, h-1-bh))
	shown := bh - 2
	if m.sel < m.top {
		m.top = m.sel
	} else if m.sel >= m.top+shown {
		m.top = m.sel - shown + 1
	}
	m.top = max(0, min(m.top, len(m.items)-shown))
	a.hits = append(a.hits[:0], hit{area: rect{0, 0, w, h}, click: func(int, int, tcell.ModMask) { a.menu = nil }})
	if m.group >= 0 { // So another name can be clicked while a menu is open.
		a.drawMenuBar(w)
	}
	a.frame(rect{x0, y0, bw, bh}, a.th.border)
	a.hits = append(a.hits, hit{area: rect{x0, y0, bw, bh}, click: func(int, int, tcell.ModMask) {}, wheel: func(delta int) {
		m.top = max(0, min(m.top+delta, len(m.items)-shown))
		m.sel = max(m.top, min(m.sel, m.top+shown-1))
		m.settle(delta)
	}})
	hasFile := a.tab() != nil && a.tab().ed != nil
	maxX := x0 + bw - 1
	for row := range shown {
		i := m.top + row
		if i >= len(m.items) {
			break
		}
		item := m.items[i]
		y := y0 + 1 + row
		if item.label == "" {
			for x := x0 + 1; x < maxX; x++ {
				a.screen.SetContent(x, y, '─', nil, a.th.muted)
			}
			continue
		}
		style, keyStyle := a.th.primary, a.th.icon["cyan"]
		if item.needsFile() && !hasFile {
			style, keyStyle = a.th.muted, a.th.muted
		}
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
		hint := a.menuHint(item)
		a.put(maxX-runewidth.StringWidth(hint)-1, y, maxX, hint, keyStyle)
		a.hits = append(a.hits, hit{area: rect{x0 + 1, y, bw - 2, 1}, click: func(int, int, tcell.ModMask) { a.runMenuItem(item) }})
	}
}

// The windows that open from the bar.

const hubHint = "↑↓ move   1-9 pick   Enter choose   Del remove   Esc close"

// barHint is the same for the windows that sit in the menu bar, where Left and Right go on along it.
const barHint = "↑↓ move   ←→ menus   Enter choose   Del remove   Esc close"

// itemRows shows menu items in a window. Separators have no place in a list under a header.
func (a *app) itemRows(items []menuItem) []modalRow {
	var rows []modalRow
	for _, item := range items {
		if item.label == "" {
			continue
		}
		label := item.label
		if item.checked != nil && item.checked(a) {
			label = "✓ " + label
		}
		rows = append(rows, modalRow{label: label, action: true, value: a.menuHint(item), run: func() {
			a.closeModal(false)
			a.runMenuItem(item)
		}})
	}
	return rows
}

// hotkeyFor numbers the first nine things to switch to.
func hotkeyFor(i int) rune {
	if i < 9 {
		return rune('1' + i)
	}
	return 0
}

// openProjectHub is Alt+P, a click on the project's name, and :project. It starts on the project before
// this one, so that Alt+P and Enter go back, and the same key again goes on down the list.
func (a *app) openProjectHub(items []menuItem) {
	start := a.project
	if i := slices.IndexFunc(a.projects, func(p project.Project) bool { return p.Name == a.prevProject }); i >= 0 && i != a.project {
		start = i
	}
	a.openModal(&modal{title: "✻ Switch project", hint: hubHint, filtered: true, compact: true, labelOnly: true, escCloses: true, again: "project.switch", sel: start + 1, build: func() []modalRow {
		rows := []modalRow{{label: "Switch to", header: true}}
		for i, p := range a.projects {
			rows = append(rows, modalRow{label: either(i == a.project, "✓ ", "  ") + p.Name, hotkey: hotkeyFor(i), value: fmt.Sprintf("%d dirs", len(p.Dirs)), run: func() {
				a.closeModal(false)
				a.switchTo(i, -1)
			}, remove: func() { a.deleteProject(p) }})
		}
		return append(append(rows, modalRow{label: "Manage", header: true}), a.itemRows(items)...)
	}})
}

// openDirHub is the same for the folders of this project: Alt+D, a click on the folder's name, and :dir.
func (a *app) openDirHub(items []menuItem) {
	dirs := a.projects[a.project].Dirs
	missing := make([]bool, len(dirs)) // Looked at once, so that drawing doesn't touch the disk.
	for i, d := range dirs {
		_, err := os.Stat(d)
		missing[i] = err != nil
	}
	start := a.dirOf[a.project]
	if i := slices.Index(dirs, a.prevDir); i >= 0 && i != start {
		start = i
	}
	a.openModal(&modal{title: "▸ Switch folder", hint: hubHint, width: 72, filtered: true, compact: true, labelOnly: true, escCloses: true, again: "directory.switch", sel: start + 1, build: func() []modalRow {
		current := a.projects[a.project]
		rows := []modalRow{{label: "In " + current.Name, header: true}}
		for i, dir := range current.Dirs {
			label := either(i == a.dirOf[a.project], "✓ ", "  ") + filepath.Base(dir)
			if i < len(missing) && missing[i] {
				label += "  (missing)" // Q-66: marked, never removed silently.
			}
			rows = append(rows, modalRow{label: label, hotkey: hotkeyFor(i), value: dir, run: func() {
				a.closeModal(false)
				a.switchTo(a.project, i)
			}, remove: func() { a.removeDir(current.Name, dir) }}) // It says so if it refuses.
		}
		return append(append(rows, modalRow{label: "Manage", header: true}), a.itemRows(items)...)
	}})
}

// openTerminalHub lists the terminals and AI sessions running here, each with its way out.
func (a *app) openTerminalHub(items []menuItem) {
	a.openModal(&modal{title: "❯ Terminals and AI sessions", hint: barHint, width: 72, compact: true, build: func() []modalRow {
		st := a.dir()
		rows := []modalRow{{label: "Running here", header: true}}
		for _, p := range st.panes() {
			for _, it := range p.tabs {
				if it.term == nil {
					continue
				}
				label, icon, _ := a.tabLabel(it)
				rows = append(rows, modalRow{label: icon + " " + label, value: either(it.sess != nil && !it.sess.isShell, "AI", "terminal"), run: func() {
					a.closeModal(false)
					if p == st.dock {
						a.dockHidden = false
					}
					st.focus, p.cur = p, slices.Index(p.tabs, it)
					a.focusTab()
				}, remove: func() { a.removeTab(st, p, it) }})
			}
		}
		if len(rows) == 1 {
			rows = append(rows, modalRow{label: "Nothing yet"})
		}
		return append(append(rows, modalRow{label: "Actions", header: true}), a.itemRows(items)...)
	}})
}

func (a *app) openHelpHub(items []menuItem) {
	a.openModal(&modal{title: "? Help", hint: "↑↓ move   ←→ menus   Enter choose   Esc close", width: 72, compact: true, build: func() []modalRow {
		rows := append([]modalRow{{label: "Get help", header: true}}, a.itemRows(items)...)
		return append(rows,
			modalRow{label: "Good to know", header: true},
			modalRow{label: "Drag selects text. Double click takes a word, triple a line."},
			modalRow{label: "In a terminal, dragging copies what you select."},
			modalRow{label: "Close a tab: its ×, the middle button, or " + a.keyFor("tab.close") + "."},
			modalRow{label: "Leave a terminal: click elsewhere, or Ctrl+Space, which goes back in too."},
			modalRow{label: a.keyFor("project.switch") + " and " + a.keyFor("directory.switch") + " switch project and folder, even from a terminal."},
			modalRow{label: "The right button opens a menu on tabs, files and code."})
	}})
}

// What the menus do that nothing else did.

func (a *app) saveAll() {
	seen := map[*buffer.File]bool{}
	for _, p := range a.dir().panes() {
		for _, it := range p.tabs {
			if it.ed != nil && it.ed.File.Modified() && !seen[it.ed.File] {
				seen[it.ed.File] = true
				a.save(it, func() {})
			}
		}
	}
	if a.message == "" {
		a.message = fmt.Sprintf("Saved %d files.", len(seen))
	}
}

// closeSavedTabs closes the code tabs with nothing to lose, and leaves the rest alone.
func (a *app) closeSavedTabs() {
	st := a.dir()
	kept := 0
	for _, p := range st.root.panes() {
		for _, it := range slices.Clone(p.tabs) {
			if it.ed.File.Modified() && a.lastView(it) {
				kept++
				continue
			}
			a.removeTab(st, p, it)
		}
	}
	if kept > 0 {
		a.message = fmt.Sprintf("%d tabs with unsaved changes stay open.", kept)
	}
}

func (a *app) openGoToLine() {
	it := a.tab()
	if it == nil || it.ed == nil {
		a.message = "That needs a file."
		return
	}
	text := ""
	submit := func() {
		line, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil || line < 1 {
			a.message = "That isn't a line number."
			return
		}
		a.closeModal(false)
		it.ed.GoTo(min(line, it.ed.File.LineCount())-1, 0, false)
		a.focusTab()
	}
	a.openModal(&modal{title: "Go to line", hint: fmt.Sprintf("1 to %d   Enter go   Esc cancel", it.ed.File.LineCount()), width: 40, submit: submit, build: func() []modalRow {
		return []modalRow{{label: "Line", input: &text}}
	}})
}

// allCommands is what Help lists.
var allCommands = [][2]string{
	{"save", "Save the file"}, {"new", "New file"}, {"close", "Close the tab"}, {"q", "Quit"},
	{"project", "Switch project"}, {"project new", "New project"}, {"project close", "Close this project"}, {"project delete", "Delete this project from the list"},
	{"dir", "Switch folder"}, {"dir open", "Open any folder"}, {"dir add", "Add a folder"}, {"dir remove", "Take this folder out of the project"},
	{"tree", "Show all folders in Files"}, {"view", "Move a view to the other panel"},
	{"move", "Move a terminal between the editor area and the bottom panel, or the views to the other panel"}, {"term", "New terminal"}, {"dock", "Show or hide the dock"}, {"session", "New AI session"}, {"session hidden", "Hidden AI sessions"},
	{"play", "Run this folder"}, {"split", "Split right"}, {"split down", "Split below"}, {"wrap", "Word wrap"},
	{"find", "Find"}, {"replace", "Replace"}, {"search", "Find in folder"}, {"selectall", "Select all matches"},
	{"definition", "Go to definition"}, {"rename", "Rename the symbol"}, {"format", "Format the file"},
	{"git", "Show Git"}, {"push", "Push"}, {"pull", "Pull"}, {"fetch", "Fetch"}, {"repo create", "Create a GitHub repository"},
	{"settings", "Settings"}, {"appearance", "Appearance"}, {"themes", "Themes"}, {"plugins", "Plugins"}, {"store", "Plugin store"},
	{"shortcuts", "Shortcuts"}, {"onboard", "Welcome screen"}, {"menu", "Menus"}, {"messages", "Recent messages"},
	{"config", "Open config.toml"}, {"config keys", "Open keys.toml"}, {"config projects", "Open projects.toml"},
	{"update", "Install the latest version"},
}

func (a *app) openCommandList() {
	a.openModal(&modal{title: "All commands", hint: "Type to filter   Enter run   Esc close", filtered: true, width: 70, build: func() []modalRow {
		var rows []modalRow
		for _, c := range allCommands {
			rows = append(rows, modalRow{label: c[1], value: ":" + c[0], run: func() {
				a.closeModal(false)
				a.run(c[0])
			}})
		}
		return rows
	}})
}

const repoURL = "https://github.com/Juan17la/lazycode"

// openLinks shows where help is. A terminal app shouldn't assume a browser, so Enter copies the link.
func (a *app) openLinks() {
	link := func(label, url string) modalRow {
		return modalRow{label: label, run: func() {
			a.closeModal(false)
			a.copyText(url)
			a.message = "Copied " + url
		}}
	}
	a.openModal(&modal{title: "Help and links", hint: "Enter copies the link   Esc close", build: func() []modalRow {
		return []modalRow{
			link("Getting started", repoURL+"/blob/main/docs/guide/getting-started.md"),
			link("Keys and settings", repoURL+"/blob/main/docs/guide/keys-and-config.md"),
			link("Coming from VS Code", repoURL+"/blob/main/docs/guide/coming-from-vscode.md"),
			link("Coming from Neovim", repoURL+"/blob/main/docs/guide/coming-from-neovim.md"),
			link("Writing plugins", repoURL+"/blob/main/docs/guide/plugins.md"),
			link("Troubleshooting", repoURL+"/blob/main/docs/guide/troubleshooting.md"),
			link("Report a problem", repoURL+"/issues"),
		}
	}})
}

// openMessages shows what the status line said, newest first, since it can pass before it's read.
func (a *app) openMessages() {
	a.openModal(&modal{title: "Recent messages", hint: "Esc close", width: 72, build: func() []modalRow {
		rows := []modalRow{}
		for _, text := range slices.Backward(a.messages) {
			rows = append(rows, modalRow{label: text})
		}
		if len(rows) == 0 {
			rows = append(rows, modalRow{label: "Nothing yet"})
		}
		return rows
	}})
}

func (a *app) openAbout() {
	version := a.upd.version
	if version == "" {
		version = "dev"
	}
	a.openModal(&modal{title: "✻ About LazyCode", hint: "Esc close", width: 56, build: func() []modalRow {
		return []modalRow{
			{label: "Version " + version},
			{label: runtime.GOOS + " " + runtime.GOARCH + ", " + runtime.Version()},
			{label: repoURL},
		}
	}})
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
