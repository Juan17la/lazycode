package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Juan17la/lazycode/internal/config"
	"github.com/Juan17la/lazycode/internal/keymap"
	"github.com/Juan17la/lazycode/internal/plugin"
	"github.com/Juan17la/lazycode/internal/project"
	"github.com/Juan17la/lazycode/internal/tree"
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

// modal is a window over everything else. Its rows are rebuilt for every key and frame, so that a
// setting shows its new value with nothing to keep in step.
type modal struct {
	title    string
	hint     string
	width    int
	build    func() []modalRow
	sel, top int
	filter   string
	filtered bool                     // Typing narrows the rows.
	capture  func(ev *tcell.EventKey) // Takes every key, to learn a new shortcut.
	submit   func()                   // What Enter does in a text field.
	closed   func()                   // Runs if it's dismissed, to undo a preview.
}

type modalRow struct {
	label    string
	value    string // Right-aligned.
	header   bool
	input    *string // A text field.
	swatches []tcell.Color
	run      func()          // Enter or a click.
	change   func(delta int) // Left and Right, and Space.
	moved    func()          // The row was highlighted.
}

func (r modalRow) pickable() bool {
	return !r.header && (r.input != nil || r.run != nil || r.change != nil)
}

func (m *modal) rows() []modalRow {
	rows := m.build()
	if m.filter == "" {
		return rows
	}
	needle := strings.ToLower(m.filter)
	return slices.DeleteFunc(rows, func(r modalRow) bool { return !strings.Contains(strings.ToLower(r.label), needle) })
}

// settle puts the selection on a row that can be picked, looking in the direction given.
func (m *modal) settle(rows []modalRow, direction int) {
	m.sel = max(0, min(m.sel, len(rows)-1))
	for range rows {
		if rows[m.sel].pickable() {
			return
		}
		m.sel = (m.sel + direction + len(rows)) % len(rows)
	}
}

func (a *app) openModal(m *modal) {
	if m.width == 0 {
		m.width = 64
	}
	m.settle(m.rows(), 1)
	a.menu = nil
	a.modals = append(a.modals, m)
}

// closeModal drops the top window. dismissed says it was cancelled, rather than used.
func (a *app) closeModal(dismissed bool) {
	m := a.modals[len(a.modals)-1]
	a.modals = a.modals[:len(a.modals)-1]
	if dismissed && m.closed != nil {
		m.closed()
	}
}

func (a *app) handleModalKey(ev *tcell.EventKey) {
	m := a.modals[len(a.modals)-1]
	if m.capture != nil {
		m.capture(ev)
		return
	}
	rows := m.rows()
	var row modalRow
	if m.sel < len(rows) {
		row = rows[m.sel]
	}
	move := func(direction int) {
		if len(rows) == 0 {
			return
		}
		m.sel = (m.sel + direction + len(rows)) % len(rows)
		m.settle(rows, direction)
		if rows[m.sel].moved != nil {
			rows[m.sel].moved()
		}
	}
	switch ev.Key() {
	case tcell.KeyEsc:
		if m.filter != "" {
			m.filter = ""
			return
		}
		a.closeModal(true)
	case tcell.KeyUp, tcell.KeyBacktab:
		move(-1)
	case tcell.KeyDown, tcell.KeyTab:
		move(1)
	case tcell.KeyLeft:
		if row.change != nil {
			row.change(-1)
		}
	case tcell.KeyRight:
		if row.change != nil {
			row.change(1)
		}
	case tcell.KeyEnter:
		switch {
		case row.input != nil && m.submit != nil:
			m.submit()
		case row.run != nil:
			row.run()
		case row.change != nil:
			row.change(1)
		}
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		switch {
		case row.input != nil:
			_, size := utf8.DecodeLastRuneInString(*row.input)
			*row.input = (*row.input)[:len(*row.input)-size]
		case m.filter != "":
			_, size := utf8.DecodeLastRuneInString(m.filter)
			m.filter = m.filter[:len(m.filter)-size]
			m.sel = 0
			m.settle(m.rows(), 1)
		}
	case tcell.KeyRune:
		if ev.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) != 0 {
			return
		}
		switch {
		case row.input != nil:
			*row.input += string(ev.Rune())
		case ev.Rune() == ' ' && row.change != nil:
			row.change(1)
		case ev.Rune() == ' ' && row.run != nil:
			row.run()
		case m.filtered:
			m.filter += string(ev.Rune())
			m.sel = 0
			m.settle(m.rows(), 1)
		}
	}
}

func (a *app) modalPaste(text string) {
	m := a.modals[len(a.modals)-1]
	rows := m.rows()
	if m.sel < len(rows) && rows[m.sel].input != nil {
		*rows[m.sel].input += strings.NewReplacer("\r", "", "\n", "").Replace(text)
	}
}

// frame draws a rounded box, and keeps its inside clear of whatever is under it.
func (a *app) frame(r rect, style tcell.Style) {
	if r.w < 2 || r.h < 2 || r.x < 0 || r.y < 0 {
		return
	}
	a.floating = append(a.floating, r)
	for y := r.y; y < r.y+r.h; y++ {
		for x := r.x; x < r.x+r.w; x++ {
			char := ' '
			cell := a.th.primary
			switch {
			case x == r.x && y == r.y:
				char = '╭'
			case x == r.x+r.w-1 && y == r.y:
				char = '╮'
			case x == r.x && y == r.y+r.h-1:
				char = '╰'
			case x == r.x+r.w-1 && y == r.y+r.h-1:
				char = '╯'
			case y == r.y || y == r.y+r.h-1:
				char = '─'
			case x == r.x || x == r.x+r.w-1:
				char = '│'
			}
			if char != ' ' {
				cell = style
			}
			a.screen.SetContent(x, y, char, nil, cell)
		}
	}
}

// drawModals draws the top window. The ones under it wait, and clicks can't reach them.
func (a *app) drawModals(w, h int) {
	m := a.modals[len(a.modals)-1]
	rows := m.rows()
	bw := max(1, min(m.width, w-4))
	extra := 3 // Border and hint.
	if m.filtered {
		extra++
	}
	bh := max(3, min(len(rows)+extra, h-2))
	x0, y0 := (w-bw)/2, max(0, (h-bh)/2)
	a.frame(rect{x0, y0, bw, bh}, a.th.borderFocus)
	a.hits = append(a.hits[:0], hit{area: rect{0, 0, w, h}, click: func(int, int, tcell.ModMask) { a.closeModal(true) }})
	box := rect{x0, y0, bw, bh}
	a.hits = append(a.hits, hit{area: box, click: func(int, int, tcell.ModMask) {}, wheel: func(delta int) {
		m.top = max(0, min(m.top+delta, len(rows)-1))
	}})

	maxX := x0 + bw - 2
	a.put(x0+2, y0, maxX, " "+m.title+" ", a.th.accent)
	a.put(x0+bw-4, y0, x0+bw-1, " ✗ ", a.th.err)
	a.hits = append(a.hits, hit{area: rect{x0 + bw - 4, y0, 3, 1}, click: func(int, int, tcell.ModMask) { a.closeModal(true) }})

	y := y0 + 1
	if m.filtered {
		end := a.put(x0+2, y, maxX, "❯ ", a.th.icon["cyan"])
		end = a.put(end, y, maxX, m.filter, a.th.primary)
		if m.filter == "" {
			a.put(end, y, maxX, "Type to filter", a.th.muted)
		}
		a.screen.ShowCursor(min(end, maxX), y)
		y++
	}
	shown := max(0, y0+bh-1-y-1)
	if m.sel < m.top {
		m.top = m.sel
	} else if m.sel >= m.top+shown {
		m.top = m.sel - shown + 1
	}
	if len(rows) == 0 {
		a.put(x0+2, y, maxX, "No matches", a.th.muted)
	}
	for i := m.top; i < len(rows) && i < m.top+shown; i++ {
		a.drawModalRow(m, rows[i], i, rect{x0 + 1, y, bw - 2, 1})
		y++
	}
	a.put(x0+2, y0+bh-2, maxX, m.hint, a.th.muted)
}

func (a *app) drawModalRow(m *modal, row modalRow, i int, r rect) {
	maxX := r.x + r.w - 1
	y := r.y
	if row.header {
		a.put(r.x+1, y, maxX, row.label, a.th.icon["cyan"].Bold(true))
		return
	}
	selected := i == m.sel
	style, valueStyle := a.th.primary, a.th.icon["cyan"]
	if !row.pickable() {
		style = a.th.muted
	}
	if selected {
		style, valueStyle = a.th.active, a.th.active
		for x := r.x; x < r.x+r.w; x++ {
			a.screen.SetContent(x, y, ' ', nil, a.th.active)
		}
		a.put(r.x, y, maxX, "❯", a.th.active)
	}
	x := r.x + 2
	if row.input != nil {
		x = a.put(x, y, maxX, row.label+" ", a.th.icon["cyan"])
		boxStyle := a.th.muted
		if selected {
			boxStyle = a.th.borderFocus
		}
		x = a.put(x, y, maxX, "▏", boxStyle)
		// The end of a long path is the part being typed.
		text := *row.input
		for room := maxX - x - 1; room > 0 && runewidth.StringWidth(text) > room; {
			_, size := utf8.DecodeRuneInString(text)
			text = text[size:]
		}
		end := a.put(x, y, maxX, text, style)
		if selected {
			a.screen.ShowCursor(min(end, maxX), y)
		}
		return
	}
	x = a.put(x, y, maxX, row.label, style)
	if len(row.swatches) > 0 {
		sx := max(x+1, maxX-2*len(row.swatches))
		for _, c := range row.swatches {
			sx = a.put(sx, y, maxX, "██", tcell.StyleDefault.Foreground(c))
		}
	} else if row.value != "" {
		a.put(max(x+1, maxX-runewidth.StringWidth(row.value)), y, maxX, row.value, valueStyle)
	}
	index := i
	a.hits = append(a.hits, hit{area: r, click: func(int, int, tcell.ModMask) {
		if !row.pickable() {
			return
		}
		m.sel = index
		if row.moved != nil {
			row.moved()
		}
		switch {
		case row.run != nil:
			row.run()
		case row.change != nil:
			row.change(1)
		}
	}})
}

// Settings. The file is what's true: a change is written, then read back through the checks.

func (a *app) setConfig(table, key, value string) {
	if err := config.SetValue(a.settingsPath("config.toml"), table, key, value); err != nil {
		a.message = "Couldn't save that: " + err.Error()
		return
	}
	a.loadSettings()
}

func onOff(on bool) string { return either(on, "✓ On", "○ Off") }

// cycle returns the choice delta steps from current, going round.
func cycle(choices []string, current string, delta int) string {
	return choices[(slices.Index(choices, current)+delta+len(choices))%len(choices)]
}

func (a *app) toggleWrap() {
	a.setConfig("editor", "word_wrap", fmt.Sprint(!a.cfg.Editor.WordWrap))
	a.message = "Word wrap: " + either(a.cfg.Editor.WordWrap, "on", "off")
}

const modalHint = "↑↓ move   Enter open   ←→ change   Esc close"

func (a *app) openSettingsMenu() {
	open := func(action string) func() { return func() { a.do(action) } }
	file := func(name string) func() {
		return func() {
			a.closeModal(false)
			a.run("config " + name)
		}
	}
	a.openModal(&modal{title: "⚙ Settings", hint: modalHint, build: func() []modalRow {
		toggle := func(label, table, key string, on bool) modalRow {
			flip := func(int) { a.setConfig(table, key, fmt.Sprint(!on)) }
			return modalRow{label: label, value: onOff(on), change: flip}
		}
		sizes := []string{"2", "4", "8"}
		tab := fmt.Sprint(a.cfg.Editor.TabSize)
		return []modalRow{
			{label: "Look", header: true},
			{label: "Appearance…", run: open("app.appearance")},
			{label: "Themes…", value: a.cfg.UI.Theme, run: open("app.themes")},
			{label: "Extend", header: true},
			{label: "Plugins…", run: open("app.plugins")},
			{label: "Shortcuts…", value: a.keyFor("app.shortcuts"), run: open("app.shortcuts")},
			{label: "Editor", header: true},
			toggle("Word wrap", "editor", "word_wrap", a.cfg.Editor.WordWrap),
			toggle("Auto-save", "editor", "auto_save", a.cfg.Editor.AutoSave),
			{label: "Tab size", value: "‹ " + tab + " ›", change: func(delta int) {
				a.setConfig("editor", "tab_size", cycle(sizes, tab, delta))
			}},
			toggle("Check for updates", "updates", "check", a.cfg.Updates.Check),
			{label: "Help", header: true},
			{label: "Show the welcome screen again…", run: func() {
				a.closeModal(false)
				a.do("app.onboarding")
			}},
			{label: "Files", header: true},
			{label: "Open config.toml", run: file("")},
			{label: "Open keys.toml", run: file("keys")},
			{label: "Open projects.toml", run: file("projects")},
		}
	}})
}

func (a *app) openAppearance() {
	a.openModal(&modal{title: "◐ Appearance", hint: "↑↓ move   ←→ or Space change   Esc close", build: func() []modalRow {
		ui := a.cfg.UI
		choose := func(label, key, current string, choices []string) modalRow {
			return modalRow{label: label, value: "‹ " + current + " ›", change: func(delta int) {
				a.setConfig("ui", key, fmt.Sprintf("%q", cycle(choices, current, delta)))
			}}
		}
		return []modalRow{
			{label: "Themes…", value: ui.Theme, run: func() { a.do("app.themes") }},
			choose("Icons", "icons", ui.Icons, []string{"basic", "nerd"}),
			choose("Text style", "text_style", ui.TextStyle, []string{"rich", "plain"}),
			choose("Sidebar", "sidebar", ui.Sidebar, []string{"left", "right"}),
			{label: "The font and its size belong to your terminal: zoom it for more text."},
			{label: "nerd icons need a Nerd Font: nerdfonts.com"},
		}
	}})
}

// openThemes shows each theme while it's highlighted, and goes back if nothing is chosen.
func (a *app) openThemes() {
	palette := func(name string) []tcell.Color {
		var colours []tcell.Color
		for _, key := range []string{"bg", "panel", "accent", "red", "green", "yellow", "blue", "purple", "cyan", "orange"} {
			colours = append(colours, tcell.GetColor(palettes[name][key]))
		}
		return colours
	}
	m := &modal{title: "◉ Themes", hint: "↑↓ preview   Enter choose   Esc go back", sel: max(0, slices.Index(config.Themes, a.cfg.UI.Theme))}
	m.closed = func() { a.setTheme(a.cfg.UI.Theme) }
	m.build = func() []modalRow {
		var rows []modalRow
		for _, name := range config.Themes {
			rows = append(rows, modalRow{
				label:    name,
				swatches: palette(name),
				moved:    func() { a.setTheme(name) },
				run: func() {
					a.closeModal(false)
					a.chooseTheme(name)
				},
			})
		}
		return rows
	}
	a.openModal(m)
}

func (a *app) openPlugins() {
	a.openModal(&modal{title: "⬢ Plugins", hint: "↑↓ move   Enter or Space on, off   Esc close", build: func() []modalRow {
		rows := []modalRow{{label: "Browse the store…", run: func() {
			a.closeModal(false)
			a.do("app.store")
		}}, {label: "Installed", header: true}}
		entries, _ := os.ReadDir(a.pluginsDir()) //nolint:errcheck // No folder means none installed.
		found := false
		for _, entry := range entries {
			m, err := readInstalled(filepath.Join(a.pluginsDir(), entry.Name()))
			if err != nil {
				continue
			}
			found = true
			rows = append(rows, a.pluginRow(m, entry.Name()))
		}
		if !found {
			rows = append(rows, modalRow{label: "None yet. They go in " + a.pluginsDir()})
		}
		return rows
	}})
}

func (a *app) pluginRow(m plugin.Manifest, folder string) modalRow {
	on := slices.ContainsFunc(a.plugins.loaded, func(p *plugin.Plugin) bool { return p.Name == m.Name && p.Err == nil })
	return modalRow{label: m.Name + " " + m.Version, value: onOff(on), change: func(int) {
		if on {
			a.pluginOff(folder)
		} else {
			a.pluginOn(folder)
		}
	}}
}

// keyFor names an action's key for showing to the user. On macOS an Alt key is really Ctrl+K, then
// the key (Q-80). An action with no key of its own, or none left, shows nothing.
func (a *app) keyFor(action string) string {
	keys := a.keys.KeysFor(action)
	// A plain letter only works in one panel, so it's the last one worth naming.
	for _, key := range keys {
		if len([]rune(key)) > 1 {
			return prettyKey(key, runtime.GOOS)
		}
	}
	return ""
}

func prettyKey(key, goos string) string {
	parts := strings.Split(key, "+")
	if goos == "darwin" && parts[0] == "alt" {
		return "Ctrl+K " + strings.Join(titled(parts[1:]), "+")
	}
	return strings.Join(titled(parts), "+")
}

func titled(parts []string) []string {
	out := make([]string, len(parts))
	for i, part := range parts {
		if part != "" {
			out[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return out
}

// openShortcuts lists the shortcuts by family, and Enter changes one.
func (a *app) openShortcuts() {
	a.openModal(&modal{title: "⌨ Shortcuts", hint: "Type to filter   Enter change the key   Esc close", filtered: true, width: 70, build: func() []modalRow {
		var rows []modalRow
		family := ""
		for _, entry := range keymap.Catalog {
			if entry.Family != family {
				family = entry.Family
				rows = append(rows, modalRow{label: family, header: true})
			}
			rows = append(rows, modalRow{label: entry.Label, value: a.keyFor(entry.Action), run: func() { a.learnKey(entry) }})
		}
		return rows
	}})
}

// learnKey waits for the new key, and writes it to keys.toml.
func (a *app) learnKey(entry keymap.Entry) {
	m := &modal{title: "⌨ " + entry.Label, width: 50, hint: "Esc cancels", build: func() []modalRow {
		return []modalRow{{label: "Press the new key"}}
	}}
	m.capture = func(ev *tcell.EventKey) {
		key := keymap.Name(ev)
		if key == "esc" {
			a.closeModal(true)
			return
		}
		if key == "" {
			return
		}
		a.closeModal(false)
		if err := a.keys.Bind(a.settingsPath("keys.toml"), key, entry.Action); err != nil {
			a.message = "Not changed: " + err.Error()
			return
		}
		a.loadSettings()
		a.message = entry.Label + " is now " + prettyKey(key, runtime.GOOS)
	}
	a.openModal(m)
}

// openOnboarding shows the first-run screen again.
func (a *app) openOnboarding() {
	o := newOnboarding()
	o.again, o.light, o.autoSave = true, a.cfg.UI.Theme == "light", a.cfg.Editor.AutoSave
	a.modals, a.menu = nil, nil
	a.onboard = o
}

// Forms.

// openNewProject asks for a name and a folder. :project new NAME does the same without asking.
func (a *app) openNewProject() {
	name, folder := "", a.dirPath()
	submit := func() {
		if strings.TrimSpace(name) == "" {
			a.message = "A project needs a name."
			return
		}
		dir := a.resolvePath(folder)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			a.message = filepath.Base(dir) + " isn't a directory."
			return
		}
		a.closeModal(false)
		a.newProjectAt(strings.TrimSpace(name), dir)
	}
	a.openModal(&modal{title: "✚ New project", hint: "Enter create   Tab next field   Esc cancel", submit: submit, build: func() []modalRow {
		rows := []modalRow{
			{label: "Name  ", input: &name},
			{label: "Folder", input: &folder},
			{label: "Create project", run: submit},
		}
		return append(rows, a.folderRows(&folder)...)
	}})
}

// openAddDir asks for a folder. :dir add PATH does the same without asking.
func (a *app) openAddDir() {
	if a.projects[a.project].Unsaved {
		a.message = "This folder isn't in a project yet, so make one first."
		a.openNewProject()
		return
	}
	folder := filepath.Dir(a.dirPath()) + string(filepath.Separator)
	submit := func() {
		path := strings.TrimSpace(folder)
		if path == "" {
			return
		}
		a.closeModal(false)
		a.addDir(path)
	}
	a.openModal(&modal{title: "✚ Add a directory to " + a.projects[a.project].Name, hint: "Enter add   ↓ pick a folder   Esc cancel", submit: submit, build: func() []modalRow {
		rows := []modalRow{
			{label: "Folder", input: &folder},
			{label: "Add this directory", run: submit},
		}
		return append(rows, a.folderRows(&folder)...)
	}})
}

// treeMenu is what the right button offers in the file tree. New things go in the folder under the
// mouse, or beside the file under it.
func (a *app) treeMenu(row tree.Row, onRow bool) []menuItem {
	base := a.dirPath()
	if onRow {
		base = row.Path
		if !row.IsDir {
			base = filepath.Dir(row.Path)
		}
	}
	items := []menuItem{
		{label: "New File…", run: func() { a.openNewIn(base, false) }},
		{label: "New Folder…", run: func() { a.openNewIn(base, true) }},
	}
	// A project's own directories come and go through :dir, not from here.
	if onRow && !slices.Contains(a.projects[a.project].Dirs, row.Path) {
		items = append(items, menuItem{},
			menuItem{label: "Rename…", run: func() { a.openRename(row.Path) }},
			menuItem{label: "Delete…", run: func() { a.askDelete(row.Path, row.IsDir) }})
	}
	return append(items, menuItem{}, menuItem{label: "Refresh", action: "tree.refresh"})
}

func (a *app) openNewIn(base string, folder bool) {
	name := ""
	submit := func() {
		text := strings.TrimSpace(name)
		if text == "" {
			return
		}
		a.closeModal(false)
		path := filepath.Join(base, text)
		if !folder {
			a.newFile(path)
			return
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			a.message = "Can't create " + text + ": " + err.Error()
			return
		}
		a.tree().Refresh()
		a.message = "Created " + text + "."
	}
	a.openModal(&modal{title: "✚ New " + either(folder, "folder", "file") + " in " + filepath.Base(base), hint: "Enter create   Esc cancel", submit: submit, build: func() []modalRow {
		return []modalRow{{label: "Name", input: &name}}
	}})
}

// openInside reports whether a tab shows path or something under it. A tab keeps its path, so renaming
// or deleting under it would have the next save bring the old file back.
func (a *app) openInside(path string) bool {
	for _, st := range a.dirs {
		for _, p := range st.panes() {
			for _, it := range p.tabs {
				if it.ed != nil && (it.ed.File.Path == path || strings.HasPrefix(it.ed.File.Path, path+string(filepath.Separator))) {
					return true
				}
			}
		}
	}
	return false
}

func (a *app) openRename(path string) {
	if a.openInside(path) {
		a.message = "Close the tabs in " + filepath.Base(path) + " first."
		return
	}
	name := filepath.Base(path)
	submit := func() {
		text := strings.TrimSpace(name)
		if text == "" || text == filepath.Base(path) {
			a.closeModal(false)
			return
		}
		if text == "." || text == ".." || strings.ContainsAny(text, `/\\`) {
			a.message = "A new name can't be a path."
			return
		}
		a.closeModal(false)
		target := filepath.Join(filepath.Dir(path), text)
		if _, err := os.Lstat(target); err == nil { // os.Rename would replace it without a word.
			a.message = text + " already exists."
			return
		}
		if err := os.Rename(path, target); err != nil {
			a.message = "Can't rename: " + err.Error()
			return
		}
		a.tree().Refresh()
	}
	a.openModal(&modal{title: "✎ Rename " + filepath.Base(path), hint: "Enter rename   Esc cancel", submit: submit, build: func() []modalRow {
		return []modalRow{{label: "Name", input: &name}}
	}})
}

// askDelete always asks, and a folder's question says everything in it goes too. It can't be undone.
func (a *app) askDelete(path string, isDir bool) {
	if a.openInside(path) {
		a.message = "Close the tabs in " + filepath.Base(path) + " first."
		return
	}
	text := "Delete " + filepath.Base(path) + " for good?"
	if isDir {
		text = "Delete the folder " + filepath.Base(path) + " and everything in it, for good?"
	}
	a.ask(text,
		choice{'d', "Delete", func() {
			if err := os.RemoveAll(path); err != nil {
				a.message = "Can't delete: " + err.Error()
			}
			a.tree().Refresh()
		}},
		choice{'c', "Cancel", func() {}})
}

// openNewFile asks for a name, makes the file, and opens it. Folders on the way are made too.
func (a *app) openNewFile() {
	name := ""
	submit := func() {
		if strings.TrimSpace(name) == "" {
			return
		}
		a.closeModal(false)
		a.newFile(a.resolvePath(strings.TrimSpace(name)))
	}
	a.openModal(&modal{title: "✚ New file in " + filepath.Base(a.dirPath()), hint: "Enter create   Esc cancel", submit: submit, build: func() []modalRow {
		return []modalRow{{label: "Name", input: &name}}
	}})
}

func (a *app) newFile(path string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		a.message = "Can't create " + filepath.Base(path) + ": " + err.Error()
		return
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		a.message = "Can't create " + filepath.Base(path) + ": " + err.Error()
		return
	}
	f.Close() //nolint:errcheck // Nothing was written.
	a.tree().Refresh()
	a.openFile(a.dir(), path)
}

// resolvePath reads a typed path: ~ works, and a relative one starts from the current directory.
func (a *app) resolvePath(path string) string {
	path = project.Expand(strings.TrimSpace(path))
	if !filepath.IsAbs(path) {
		path = filepath.Join(a.dirPath(), path)
	}
	return path
}

// maxFolderRows keeps a huge folder's listing from being read and drawn in full.
const maxFolderRows = 50

// folderRows lists the subfolders of what's typed. Picking one fills it in, so a folder can be found
// without knowing its path.
func (a *app) folderRows(folder *string) []modalRow {
	typed := *folder
	base := a.resolvePath(typed)
	prefix := ""
	if !strings.HasSuffix(typed, string(filepath.Separator)) {
		if info, err := os.Stat(base); err != nil || !info.IsDir() {
			base, prefix = filepath.Dir(base), filepath.Base(base)
		}
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	rows := []modalRow{{label: "Folders in " + base, header: true}}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || strings.HasPrefix(name, ".") || !strings.HasPrefix(name, prefix) || len(rows) > maxFolderRows {
			continue
		}
		full := filepath.Join(base, name) + string(filepath.Separator)
		rows = append(rows, modalRow{label: "▸ " + name, run: func() { *folder = full }})
	}
	return rows
}
