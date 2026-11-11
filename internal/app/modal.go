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
	title     string
	hint      string
	width     int
	build     func() []modalRow
	sel, top  int
	filter    string
	filtered  bool                     // Typing narrows the rows.
	escCloses bool                     // Esc closes even with something typed, where it would first clear it.
	labelOnly bool                     // The filter reads the labels alone, not the values beside them.
	again     string                   // An action whose key, pressed again, goes to the next numbered row.
	capture   func(ev *tcell.EventKey) // Takes every key, to learn a new shortcut.
	submit    func()                   // What Enter does in a text field.
	closed    func()                   // Runs if it's dismissed, to undo a preview.
}

type modalRow struct {
	label    string
	danger   bool   // Can't be undone, so it's in the error colour.
	action   bool   // Does something, rather than being one of the things listed: its own colour.
	value    string // Right-aligned.
	header   bool
	hotkey   rune    // A digit that picks the row when nothing has been typed.
	input    *string // A text field.
	swatches []tcell.Color
	run      func()          // Enter or a click.
	change   func(delta int) // Left and Right, and Space.
	moved    func()          // The row was highlighted.
	remove   func()          // Delete, or a click on the row's ×.
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
	return slices.DeleteFunc(rows, func(r modalRow) bool {
		return !strings.Contains(strings.ToLower(r.label), needle) && (m.labelOnly || !strings.Contains(strings.ToLower(r.value), needle)) // So a key can be looked up.
	})
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
	if m.again != "" && a.keys.Lookup(keymap.Global, keymap.Name(ev)) == m.again {
		for step := 1; step <= len(rows); step++ { // The same key again: on to the next numbered row.
			if i := (m.sel + step) % len(rows); rows[i].hotkey != 0 {
				m.sel = i
				break
			}
		}
		return
	}
	switch ev.Key() {
	case tcell.KeyEsc:
		if m.filter != "" && !m.escCloses {
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
	case tcell.KeyDelete:
		if row.remove != nil {
			row.remove()
			m.settle(m.rows(), 1)
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
		case m.filter == "" && hotRow(rows, ev.Rune()) != nil:
			hotRow(rows, ev.Rune()).run()
		case m.filtered:
			m.filter += string(ev.Rune())
			m.sel = 0
			m.settle(m.rows(), 1)
		}
	}
}

func hotRow(rows []modalRow, r rune) *modalRow {
	for i := range rows {
		if rows[i].hotkey == r && rows[i].run != nil {
			return &rows[i]
		}
	}
	return nil
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

// modalRule divides a window across its full width, joined to its border.
func (a *app) modalRule(x0, y, bw int) {
	a.fillLine(rect{x0, y, bw, 1}, a.th.borderFocus)
	a.screen.SetContent(x0, y, '├', nil, a.th.borderFocus)
	a.screen.SetContent(x0+bw-1, y, '┤', nil, a.th.borderFocus)
}

// gapBefore is the blank rows above row i. A section gets one above its header and stays tight below
// it. Lines of plain text stay together as a paragraph. gap is what's left between options.
func gapBefore(rows []modalRow, i, gap int) int {
	switch {
	case rows[i].header:
		return 1
	case rows[i-1].header, !rows[i].pickable() && !rows[i-1].pickable():
		return 0
	}
	return gap
}

// rowsHeight is how many screen rows it takes to draw rows from and to, both included.
func rowsHeight(rows []modalRow, from, to, gap int) int {
	n := 0
	for i := from; i <= to && i < len(rows); i++ {
		n++
		if i > from {
			n += gapBefore(rows, i, gap)
		}
	}
	return n
}

// drawModals draws the top window. The ones under it wait, and clicks can't reach them. The spacing
// goes first when the window is short: the gaps between options if the list wouldn't fit with them, and
// the padding and rules on a very small terminal.
func (a *app) drawModals(w, h int) {
	m := a.modals[len(a.modals)-1]
	rows := m.rows()
	bw := max(1, min(m.width, w-4))
	roomy := h >= 14
	extra := 3 // Border and hint.
	if roomy {
		extra += 2 // Padding under the title, and the rule over the hint.
	}
	if m.filtered {
		extra++
		if roomy {
			extra++ // The rule under the filter.
		}
	}
	gap := 1
	if rowsHeight(rows, 0, len(rows)-1, gap) > h-2-extra {
		gap = 0
	}
	bh := max(3, min(max(1, rowsHeight(rows, 0, len(rows)-1, gap))+extra, h-2))
	x0, y0 := (w-bw)/2, max(0, (h-bh)/2)
	a.frame(rect{x0, y0, bw, bh}, a.th.borderFocus)
	a.hits = append(a.hits[:0], hit{area: rect{0, 0, w, h}, click: func(int, int, tcell.ModMask) {
		if m.submit == nil { // A stray click mustn't lose what was typed in a form.
			a.closeModal(true)
		}
	}})
	box := rect{x0, y0, bw, bh}
	a.hits = append(a.hits, hit{area: box, click: func(int, int, tcell.ModMask) {}, wheel: func(delta int) {
		if len(rows) > 0 {
			m.sel = max(0, min(m.sel+delta, len(rows)-1))
			m.settle(rows, delta)
		}
	}})

	maxX := x0 + bw - 2
	a.put(x0+2, y0, maxX, " "+m.title+" ", a.th.accent)
	a.put(x0+bw-4, y0, x0+bw-1, " ✗ ", a.th.err)
	a.hits = append(a.hits, hit{area: rect{x0 + bw - 4, y0, 3, 1}, click: func(int, int, tcell.ModMask) { a.closeModal(true) }})

	y := y0 + 1
	if roomy {
		y++
	}
	if m.filtered {
		end := a.put(x0+2, y, maxX, "❯ ", a.th.icon["cyan"])
		end = a.put(end, y, maxX, m.filter, a.th.primary)
		if m.filter == "" {
			a.put(end, y, maxX, "Type to filter", a.th.muted)
		}
		a.screen.ShowCursor(min(end, maxX), y)
		y++
		if roomy {
			a.modalRule(x0, y, bw)
			y++
		}
	}
	bottom := y0 + bh - 2 // The hint's row: the list stops above it.
	if roomy {
		bottom--
		a.modalRule(x0, bottom, bw)
	}
	shown := max(0, bottom-y)
	if m.sel < m.top {
		m.top = m.sel
		if m.top > 0 && rows[m.top-1].header { // Scrolling up brings a section's name with its first row.
			m.top--
		}
	}
	for m.top < m.sel && rowsHeight(rows, m.top, m.sel, gap) > shown {
		m.top++
	}
	if len(rows) == 0 {
		a.put(x0+2, y, maxX, "No matches", a.th.muted)
	}
	last := m.top - 1
	for i := m.top; i < len(rows); i++ {
		if i > m.top {
			y += gapBefore(rows, i, gap)
		}
		if y >= bottom {
			break
		}
		a.drawModalRow(m, rows[i], i, rect{x0 + 1, y, bw - 2, 1})
		y++
		last = i
	}
	if roomy && (m.top > 0 || last < len(rows)-1) { // Say that there's more, on the rule under the list.
		more := either(m.top > 0, fmt.Sprintf(" ↑ %d", m.top), "") + either(last < len(rows)-1, fmt.Sprintf(" ↓ %d", len(rows)-1-last), "") + " more "
		a.put(x0+bw-2-runewidth.StringWidth(more), bottom, x0+bw-1, more, a.th.accent)
	}
	a.put(x0+2, y0+bh-2, maxX, m.hint, a.th.muted)
}

func (a *app) drawModalRow(m *modal, row modalRow, i int, r rect) {
	maxX := r.x + r.w - 1
	y := r.y
	if row.header { // A section's name, with a rule running to the far side.
		end := a.put(r.x+1, y, maxX, row.label, a.th.icon["cyan"].Bold(true))
		if end+1 < maxX {
			a.fillLine(rect{end + 1, y, maxX - end - 1, 1}, a.th.muted)
		}
		return
	}
	selected := i == m.sel
	style, valueStyle := a.th.primary, a.th.icon["cyan"]
	if !row.pickable() {
		style = a.th.muted
	}
	if row.action && row.pickable() {
		style, valueStyle = a.th.icon["yellow"].Bold(true), a.th.muted
	}
	if row.danger {
		style = a.th.err
	}
	if selected {
		style, valueStyle = a.th.active, a.th.active
		for x := r.x; x < r.x+r.w; x++ {
			a.screen.SetContent(x, y, ' ', nil, a.th.active)
		}
		a.put(r.x+1, y, maxX, "❯", a.th.active)
	}
	x := r.x + 3 // Under its section's name.
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
	if row.remove != nil {
		maxX -= 3
		a.put(maxX, y, maxX+3, " × ", a.th.err)
	}
	if row.hotkey != 0 {
		hotStyle := a.th.muted
		if selected {
			hotStyle = a.th.active
		}
		x = a.put(x, y, maxX, string(row.hotkey)+" ", hotStyle)
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
	if row.remove != nil {
		a.hits = append(a.hits, hit{area: rect{maxX, y, 3, 1}, click: func(int, int, tcell.ModMask) {
			row.remove()
			m.settle(m.rows(), 1)
		}})
	}
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
	command := func(text string) func() {
		return func() {
			a.closeModal(false)
			a.run(text)
		}
	}
	file := func(name string) func() { return command("config " + name) }
	a.openModal(&modal{title: "⚙ Settings", hint: modalHint, build: func() []modalRow {
		toggle := func(label, table, key string, on bool) modalRow {
			flip := func(int) { a.setConfig(table, key, fmt.Sprint(!on)) }
			return modalRow{label: label, value: onOff(on), change: flip}
		}
		sizes := []string{"2", "4", "8"}
		tab := fmt.Sprint(a.cfg.Editor.TabSize)
		return []modalRow{
			{label: "Editor", header: true},
			toggle("Word wrap", "editor", "word_wrap", a.cfg.Editor.WordWrap),
			toggle("Auto-save", "editor", "auto_save", a.cfg.Editor.AutoSave),
			{label: "Tab size", value: "‹ " + tab + " ›", change: func(delta int) {
				a.setConfig("editor", "tab_size", cycle(sizes, tab, delta))
			}},
			{label: "Appearance", header: true},
			{label: "Themes…", value: a.cfg.UI.Theme, run: open("app.themes")},
			{label: "Icons, sidebar and text…", run: open("app.appearance")},
			{label: "Keys and plugins", header: true},
			{label: "Shortcuts…", value: a.keyFor("app.shortcuts"), run: open("app.shortcuts")},
			{label: "Plugins…", run: open("app.plugins")},
			{label: "Plugin store…", run: open("app.store")},
			{label: "Advanced", header: true},
			toggle("Check for updates", "updates", "check", a.cfg.Updates.Check),
			{label: "Open config.toml", run: file("")},
			{label: "Open keys.toml", run: file("keys")},
			{label: "Open projects.toml", run: file("projects")},
			{label: "Help", header: true},
			{label: "Show the welcome screen again…", run: func() {
				a.closeModal(false)
				a.do("app.onboarding")
			}},
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
			{label: "Font and size belong to your terminal: zoom it for more text."},
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
			a.message = filepath.Base(dir) + " isn't a folder."
			return
		}
		a.closeModal(false)
		a.newProjectAt(strings.TrimSpace(name), dir)
	}
	a.openModal(&modal{title: "✚ New project", hint: "Enter create   Tab next field   Esc cancel", submit: submit, build: func() []modalRow {
		return []modalRow{
			{label: "Name  ", input: &name},
			{label: "Folder", input: &folder},
			{label: "▸ Browse for a folder…", run: func() {
				a.openFolderBrowser("Folder for the new project", "Use this folder", a.resolvePath(folder), func(path string) { folder = path })
			}},
			{label: "✓ Create project", run: submit},
		}
	}})
}

// openAddDir picks a folder to add. :dir add PATH does the same without asking.
func (a *app) openAddDir() {
	a.openFolderBrowser("Add a folder to "+a.projects[a.project].Name, "Add this folder", a.dirPath(), a.addDir)
}

// openFolder picks any folder on the device and goes to it.
func (a *app) openFolder() {
	a.openFolderBrowser("Open a folder", "Open this folder", a.dirPath(), a.goToFolder)
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
			menuItem{label: "Delete", run: func() { a.deleteToTrash(row.Path) }})
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

// openFolderBrowser walks the disk from start, listing only folders. done gets the folder chosen.
func (a *app) openFolderBrowser(title, confirm, start string, done func(path string)) {
	var m *modal
	var dir string
	var folders []string
	showHidden := false
	load := func(path string) bool {
		entries, err := os.ReadDir(path)
		if err != nil {
			a.message = "Can't open " + filepath.Base(path) + ": " + err.Error()
			return false
		}
		dir, folders = path, nil
		for _, entry := range entries {
			isDir := entry.IsDir()
			if entry.Type()&os.ModeSymlink != 0 {
				info, err := os.Stat(filepath.Join(path, entry.Name()))
				isDir = err == nil && info.IsDir()
			}
			if isDir {
				folders = append(folders, entry.Name())
			}
		}
		if m != nil {
			m.filter, m.sel, m.top = "", 0, 0
			m.settle(m.rows(), 1)
		}
		return true
	}
	// The folder to start from may have gone, as a missing project directory does.
	for path := filepath.Clean(start); !load(path); path = filepath.Dir(path) {
		if filepath.Dir(path) == path {
			return
		}
	}
	a.message = ""
	up := func() {
		if parent := filepath.Dir(dir); parent != dir {
			load(parent)
		}
	}
	arrows := func(enter func()) func(int) {
		return func(delta int) {
			if delta < 0 {
				up()
			} else if enter != nil {
				enter()
			}
		}
	}
	typePath := func() {
		text := dir + string(filepath.Separator)
		a.openModal(&modal{title: "Go to a folder", hint: "Enter go   Esc back", width: 72, submit: func() {
			if load(filepath.Clean(a.resolvePath(text))) {
				a.closeModal(false)
			}
		}, build: func() []modalRow { return []modalRow{{label: "Path", input: &text}} }})
	}
	m = &modal{title: title, hint: "Enter open   ← up   Type to filter   Esc cancel", filtered: true, width: 72, build: func() []modalRow {
		var rows []modalRow
		if m.filter == "" { // Typing is looking for a folder, so the path and actions step aside.
			chosen := dir
			toggle := func() { showHidden = !showHidden }
			rows = append(rows, modalRow{label: dir, header: true}, modalRow{label: "✓ " + confirm, run: func() {
				a.closeModal(false)
				done(chosen)
			}, change: arrows(nil)})
			if filepath.Dir(dir) != dir {
				rows = append(rows, modalRow{label: "↑ Parent folder", run: up, change: arrows(up)})
			}
			rows = append(rows,
				modalRow{label: "✎ Type a path…", run: typePath, change: arrows(typePath)},
				modalRow{label: "◌ Hidden folders", value: map[bool]string{true: "shown", false: "hidden"}[showHidden], run: toggle, change: arrows(toggle)})
		}
		for _, name := range folders {
			if !showHidden && strings.HasPrefix(name, ".") {
				continue
			}
			open := func() { load(filepath.Join(dir, name)) }
			rows = append(rows, modalRow{label: "▸ " + name, run: open, change: arrows(open)})
		}
		return rows
	}}
	a.openModal(m)
}
