package app

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Juan17la/lazycode/internal/editor"
	"github.com/Juan17la/lazycode/internal/session"
	"github.com/Juan17la/lazycode/internal/syntax"
	"github.com/Juan17la/lazycode/internal/tree"
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

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
	if a.message != a.shownMsg {
		a.shownMsg = a.message
		if a.message != "" {
			a.messages = append(a.messages, a.message)
		}
	}
	s.Clear()
	s.HideCursor()
	clear(a.hits) // Their closures hold closed tabs, whose screens are the biggest thing in memory.
	a.hits = a.hits[:0]
	w, h := s.Size()
	st := a.dir()

	a.floating = a.floating[:0]
	a.drawMenuBar(w)

	first, second, _ := a.regions()
	a.drawPanel(st, 0, first)
	a.drawPanel(st, 1, second)

	dividers := a.layout(st)
	a.resizeTabs(st)
	for _, p := range a.shownPanes(st) {
		a.drawPane(st, p)
	}
	for _, d := range dividers {
		a.fillLine(d.r, a.dividerStyle(st, d.r))
		a.hits = append(a.hits, hit{area: d.r, click: func(int, int, tcell.ModMask) {}, drag: d.drag})
	}
	a.hits = append(a.hits, a.deferred...)
	a.deferred = a.deferred[:0]
	if a.review != nil {
		a.drawReview(w, h)
	}
	if a.onboard != nil {
		a.drawOnboard(w, h)
	}
	if a.menu != nil {
		a.drawMenu(w, h)
	}
	if len(a.modals) > 0 {
		a.drawModals(w, h)
	}
	a.drawStatus(w, h)
	if a.switcher != nil {
		a.drawSwitcher(w, h)
	}
	a.paintBackground(st, w, h)
	s.Show()
}

// branchLabel is the title line's branch, with how far it is from its upstream.
func (a *app) branchLabel() string {
	g := a.git.st
	if g == nil || a.git.err != nil || a.git.dir != a.dirPath() || g.Branch == "" {
		return ""
	}
	label := a.viewIcon(viewGit) + " " + g.Branch
	if g.Ahead > 0 {
		label += fmt.Sprintf(" ↑%d", g.Ahead)
	}
	if g.Behind > 0 {
		label += fmt.Sprintf(" ↓%d", g.Behind)
	}
	return label
}

// fillLine draws a divider in r, which is one cell wide or one cell high.
func (a *app) fillLine(r rect, style tcell.Style) {
	char := '│'
	if r.w > 1 {
		char = '─'
	}
	for y := r.y; y < r.y+r.h; y++ {
		for x := r.x; x < r.x+r.w; x++ {
			a.screen.SetContent(x, y, char, nil, style)
		}
	}
}

// drawPanel draws one panel and its divider. r is empty if it's hidden.
func (a *app) drawPanel(st *dirState, index int, r rect) {
	if r.w == 0 {
		return
	}
	w, _ := a.screen.Size()
	style := a.th.border
	if a.panelFocused(index) {
		style = a.th.borderFocus
	}
	line := rect{r.x - 1, r.y, 1, r.h}
	if r.x == 0 {
		line.x = r.x + r.w
	}
	if line.x >= 0 && line.x < w {
		a.fillLine(line, style)
		a.deferred = append(a.deferred, hit{area: line, click: func(int, int, tcell.ModMask) {}, drag: func(x, _ int) {
			if r.x == 0 {
				a.panelW = x
			} else {
				a.panelW = w - x - 1
			}
		}})
	}
	// Views keep their first row for the title. Below it goes a rule, then the view, inset from the edges.
	body := r
	if r.h >= 6 && r.w >= 20 {
		body = rect{r.x + 1, r.y + 1, r.w - 2, r.h - 1}
		a.fillLine(rect{r.x, r.y + 1, r.w, 1}, a.th.muted)
	}
	switch a.viewIn(index) {
	case viewFiles:
		a.drawTree(st, body)
	case viewAI:
		a.drawSessions(body)
	case viewGit:
		a.drawGit(body)
	case viewSearch:
		a.drawSearch(body)
	default:
		a.put(body.x+1, body.y+1, body.x+body.w, "No views here", a.th.muted)
		a.put(body.x+1, body.y+2, body.x+body.w, ":view moves one", a.th.muted)
	}
	a.drawViewTitles(index, r)
}

// dividerStyle lights up a line that borders what has focus.
func (a *app) dividerStyle(st *dirState, r rect) tcell.Style {
	if a.tabFocused {
		f := st.focus.area
		grown := rect{f.x - 1, f.y - 1, f.w + 2, f.h + 2}
		if grown.contains(r.x, r.y) || grown.contains(r.x+r.w-1, r.y+r.h-1) {
			return a.th.borderFocus
		}
	}
	return a.th.border
}

func accentOf(th theme) tcell.Color {
	fg, _, _ := th.accent.Decompose()
	return fg
}

func (a *app) drawTree(st *dirState, r rect) {
	maxX := r.x + r.w
	t := a.tree()
	y0, rows := r.y+1, r.h-1
	if t.Err != nil {
		a.put(r.x+1, y0, maxX, "Can't read this folder", a.th.primary)
		return
	}
	if len(t.Rows) == 0 {
		a.put(r.x+1, y0, maxX, "Empty folder", a.th.muted)
		return
	}
	// Keep the selection on screen.
	st.top = max(0, min(st.top, t.Sel))
	if t.Sel >= st.top+rows {
		st.top = t.Sel - rows + 1
	}
	top := st.top
	a.hits = append(a.hits, hit{
		area: rect{r.x, y0, r.w, rows},
		click: func(_, y int, _ tcell.ModMask) {
			i := top + y - y0
			a.tabFocused = false
			if i >= 0 && i < len(t.Rows) {
				t.Move(i - t.Sel)
				a.openSelected()
			}
		},
		wheel: func(delta int) { t.Move(delta) },
		context: func(x, y int) {
			i := top + y - y0
			row, onRow := tree.Row{}, i >= 0 && i < len(t.Rows)
			if onRow {
				t.Move(i - t.Sel)
				row = t.Rows[i]
			}
			a.menu = &openMenu{group: -1, x: x, y: y + 1, items: a.treeMenu(row, onRow)}
		},
	})
	badges := a.badges()
	for i := st.top; i < len(t.Rows) && i < st.top+rows; i++ {
		row := t.Rows[i]
		y := y0 + i - st.top
		if row.Missing {
			row.Name += " (missing)" // Q-66: marked, never removed silently.
		}
		marker := "  "
		if row.IsDir {
			marker = "▸ "
			if row.Open {
				marker = "▾ "
			}
		}
		letter := badges.of(row.Path, row.IsDir)
		name := a.th.primary
		if row.IsDir {
			name = a.th.primary.Bold(true)
		}
		if letter != 0 {
			name = a.badgeStyle(letter)
		}
		icon, iconStyle := a.iconFor(row.Name, row.IsDir, row.Open)
		selected := i == t.Sel
		over := func(st tcell.Style) tcell.Style { return st } // A selection keeps each part's colour.
		switch {
		case selected && !a.tabFocused:
			_, bg, _ := a.th.active.Decompose()
			over = func(st tcell.Style) tcell.Style { return st.Background(bg) }
			if a.lowColour() {
				over = func(tcell.Style) tcell.Style { return a.th.active }
			}
			for col := r.x; col < maxX; col++ {
				a.screen.SetContent(col, y, ' ', nil, a.th.active)
			}
		case selected:
			name = name.Underline(true)
		}
		x := a.put(r.x+1, y, maxX, strings.Repeat("  ", row.Depth), over(a.th.muted))
		x = a.put(x, y, maxX, marker, over(a.th.muted))
		if icon != "" {
			x = a.put(x, y, maxX, icon+" ", over(iconStyle))
		}
		// The letter is the part that doesn't depend on colour. A folder only gets a dot.
		end := maxX
		if letter != 0 {
			end = maxX - 2
			mark := string(rune(letter))
			if row.IsDir {
				mark = "●"
			}
			a.put(maxX-2, y, maxX, mark, over(a.badgeStyle(letter)))
		}
		a.put(x, y, end, row.Name, over(name))
	}
}

func (a *app) drawPane(st *dirState, p *pane) {
	focused := p == st.focus
	a.drawTabBar(st, p, focused)
	content := p.content()
	if p.area.h < 2 {
		return
	}
	it := p.current()
	switch {
	case it == nil:
		a.put(content.x+1, content.y+1, content.x+content.w, "No tabs here.", a.th.primary)
		for i, line := range []string{"Enter  open the selected file", a.keyFor("view.terminal") + "  terminal", a.keyFor("session.new") + "  AI session", a.keyFor("app.menu") + "  menus"} {
			a.put(content.x+1, content.y+3+i, content.x+content.w, line, a.th.muted)
		}
		a.hits = append(a.hits, hit{area: content, click: func(int, int, tcell.ModMask) { st.focus = p }})
	case it.ed != nil:
		a.drawEditor(st, p, it, content, focused)
	default:
		it.term.Draw(a.screen, content.x, content.y, content.w, content.h)
		a.paintTermSelection(it)
		if x, y, visible := it.term.Cursor(); visible && focused && a.overlayFree() && a.tabFocused {
			a.screen.ShowCursor(content.x+x, content.y+y)
		}
		a.addTerminalHit(st, p, it, content)
	}
}

// overlayFree reports whether nothing else is taking the keys and showing a cursor.
func (a *app) overlayFree() bool {
	return len(a.modals) == 0 && a.menu == nil && a.switcher == nil && a.command == nil && a.find == nil && a.input == nil && a.review == nil && len(a.prompts) == 0
}

// tabLabel returns what a tab says, its icon, and that icon's colour.
func (a *app) tabLabel(it *tabItem) (label, icon string, iconStyle tcell.Style) {
	if it.ed != nil {
		icon, iconStyle = a.iconFor(it.name(), false, false)
		return it.name() + either(it.ed.File.Modified(), "*", ""), icon, iconStyle
	}
	colour := "cyan"
	if it.sess != nil && !it.sess.isShell {
		colour = "purple"
	}
	label = it.name()
	if done, code := it.term.Exited(); done {
		label = fmt.Sprintf("%s [exit %d]", label, code)
	}
	if it.sess != nil && it.sess.status == session.Attention {
		return label, "●", a.th.success // The green detail (Q-36). The ● says it without colour.
	}
	return label, it.marker(), a.th.icon[colour]
}

// tabWidth is a tab's full width: space, icon, space, label, space, close button, space.
func tabWidth(label, icon string) int {
	return runewidth.StringWidth(icon) + runewidth.StringWidth(label) + 5
}

// firstShownTab scrolls the bar just enough for the current tab to fit.
func (a *app) firstShownTab(p *pane, room int) int {
	first := 0
	for i := range p.tabs {
		used := 0
		for j := first; j <= min(p.cur, len(p.tabs)-1); j++ {
			label, icon, _ := a.tabLabel(p.tabs[j])
			used += tabWidth(label, icon) + 1
		}
		if used <= room || first >= p.cur {
			return first
		}
		first = i + 1
	}
	return first
}

// drawTabBar draws a pane's tabs, each with a close button, and the buttons for that kind of pane at
// the far end. Clicks pick a tab, dragging moves it, the middle button closes it, the right one asks.
func (a *app) drawTabBar(st *dirState, p *pane, focused bool) {
	y := p.area.y
	right := p.area.x + p.area.w
	button := func(label string, style tcell.Style, run func()) {
		width := runewidth.StringWidth(label)
		if right-width <= p.area.x+8 {
			return
		}
		right -= width
		a.put(right, y, right+width, label, style)
		a.hits = append(a.hits, hit{area: rect{right, y, width, 1}, click: func(int, int, tcell.ModMask) { run() }})
	}
	isDock := p == st.dock
	if isDock {
		button(" ▾ ", a.th.muted, a.toggleDock)
		button(" ✻+ ", a.th.icon["purple"], a.openSessionPicker)
		button(" ❯+ ", a.th.icon["cyan"], func() { a.startTab(nil) })
	} else {
		button(" ⊟ ", a.th.icon["blue"], func() { st.focus = p; a.splitPane(false) })
		button(" ◫ ", a.th.icon["blue"], func() { st.focus = p; a.splitPane(true) })
	}
	maxX := right
	x := p.area.x + 1
	if p.area.w >= 40 { // The name of the area, filled in when it has the keys.
		name, colour := "Editor", "blue"
		if isDock {
			name, colour = "Terminal", "cyan"
			if it := p.current(); it != nil && it.sess != nil && !it.sess.isShell {
				name, colour = "AI", "purple"
			}
		}
		style := a.th.icon[colour].Bold(true)
		if focused && a.tabFocused {
			style = a.pill(colour)
		}
		x = a.put(x, y, maxX, " "+name+" ", style) + 1
	}
	first := a.firstShownTab(p, maxX-x-2)
	if first > 0 {
		x = a.put(x, y, maxX, "‹", a.th.muted)
	}
	clear(p.spans) // It holds tabs, which must be freed once closed.
	p.spans = p.spans[:0]
	for i := first; i < len(p.tabs); i++ {
		it := p.tabs[i]
		label, icon, iconStyle := a.tabLabel(it)
		if x >= maxX {
			break
		}
		style := a.th.muted
		if i == p.cur {
			style = a.th.primary.Bold(true)
			if focused {
				style = a.th.tabCurrent
			}
			if focused && a.tabFocused {
				style = a.th.tabCurrent.Foreground(accentOf(a.th))
				if isDock {
					style = iconStyle.Background(bgOf(a.th.tabCurrent)).Bold(true)
				}
			}
		}
		_, bg, _ := style.Decompose()
		start := x
		x = a.put(x, y, maxX, " ", style)
		x = a.put(x, y, maxX, icon+" ", iconStyle.Background(bg))
		x = a.put(x, y, maxX, label+" ", style)
		closeAt := x
		closeStyle := a.th.err.Background(bg)
		x = a.put(x, y, maxX, "× ", closeStyle)
		p.spans = append(p.spans, tabSpan{it, start, x - start})
		index := i
		pick := func() {
			if index < len(p.tabs) && p.tabs[index] == it {
				st.focus, p.cur = p, index
			}
		}
		closeIt := func() {
			pick()
			a.closeTab()
		}
		a.hits = append(a.hits, hit{
			area: rect{start, y, x - start, 1},
			click: func(int, int, tcell.ModMask) {
				pick()
				a.focusTab()
			},
			drag: func() func(int, int) {
				holder := p
				return func(mouseX, mouseY int) { holder = a.dragTab(st, holder, it, mouseX, mouseY) }
			}(),
			middle: closeIt,
			context: func(mouseX, mouseY int) {
				pick()
				a.menu = &openMenu{group: -1, x: mouseX, y: mouseY + 1, items: a.tabMenu(st, p, it)}
			},
		})
		a.hits = append(a.hits, hit{area: rect{closeAt, y, 2, 1}, click: func(int, int, tcell.ModMask) { closeIt() }})
		x++
	}
}

// tabMenu is what the right button offers on a tab.
func (a *app) tabMenu(st *dirState, p *pane, it *tabItem) []menuItem {
	return []menuItem{
		{label: "Close", action: "tab.close"},
		{label: "Close Others", run: func() {
			for _, other := range slices.Clone(p.tabs) {
				if other != it && (other.ed == nil || !other.ed.File.Modified() || !a.lastView(other)) {
					a.removeTab(st, p, other)
				}
			}
			p.cur = max(0, slices.Index(p.tabs, it))
		}},
		{},
		{label: "Split Right", action: "split.right"},
		{label: "Split Below", action: "split.down"},
	}
}

func (a *app) drawEditor(st *dirState, p *pane, it *tabItem, r rect, focused bool) {
	e := it.ed
	e.Sync()
	marked := a.marksFor(e.File)
	gutter := e.GutterWidth()
	if r.w <= gutter+1 {
		gutter = 0 // Too narrow for line numbers.
	}
	scrollbar := e.File.LineCount() > r.h && r.w > gutter+2
	textWidth := max(1, r.w-gutter-scrollbarWidth(scrollbar))
	wrap := a.cfg.Editor.WordWrap
	if wrap {
		e.ScrollWrapped(textWidth, r.h)
	} else {
		e.WrapWidth = 0
		e.Scroll(textWidth, r.h)
	}
	current, _ := e.Position()
	colours := syntax.NewCursor(a.highlightSpans(e.File)) // Only what's on screen is looked up.
	// Which line and how far along it each row shows, so that the mouse can go the other way.
	type place struct{ line, col, end int } // end is where the next row of the line starts, or -1.
	rows := make([]place, 0, r.h)
	drawGutter := func(row, line int, first bool) {
		if gutter == 0 {
			return
		}
		style := a.th.muted
		if line == current {
			style = a.th.accent
		}
		if first {
			a.put(r.x, r.y+row, r.x+gutter, fmt.Sprintf(" %*d", gutter-2, line+1), style)
		}
		if marked[line] {
			a.screen.SetContent(r.x, r.y+row, '▌', nil, a.th.gitModified)
		}
	}
	for line := e.ScrollLine; len(rows) < r.h && line < e.File.LineCount(); line++ {
		if !wrap {
			row := len(rows)
			rows = append(rows, place{line, e.ScrollCol, -1})
			drawGutter(row, line, true)
			for _, c := range e.Row(line, textWidth) {
				a.screen.Put(r.x+gutter+c.X, r.y+row, c.Text, a.cellStyle(c, colours.At(c.Offset)))
			}
			continue
		}
		top := len(rows)
		breaks := e.WrapBreaks(line, textWidth)
		for i, start := range breaks {
			if len(rows) == r.h {
				break
			}
			drawGutter(len(rows), line, i == 0)
			end := -1
			if i+1 < len(breaks) {
				end = breaks[i+1]
			}
			rows = append(rows, place{line, start, end})
		}
		for _, c := range e.Row(line, textWidth*r.h) {
			i := editor.WrapRowOf(breaks, c.X)
			if row := top + i; row < r.h {
				a.screen.Put(r.x+gutter+c.X-breaks[i], r.y+row, c.Text, a.cellStyle(c, colours.At(c.Offset)))
			}
		}
	}
	if focused && a.tabFocused && a.overlayFree() {
		x, y := e.ScreenCursor()
		visible := true
		if wrap {
			x, y, visible = e.ScreenCursorWrapped(textWidth)
		}
		if visible && x >= 0 && x < textWidth && y >= 0 && y < r.h {
			a.screen.ShowCursor(r.x+gutter+x, r.y+y)
		}
	}
	// A place on screen, in the file. Past the last row means the end of the last line.
	placeAt := func(x, y int) (line, col int) {
		row := y - r.y
		if row < 0 || len(rows) == 0 {
			return e.ScrollLine, 0
		}
		if row >= len(rows) {
			last := rows[len(rows)-1]
			if !wrap {
				return last.line + row - len(rows) + 1, e.ScrollCol + x - r.x - gutter
			}
			return last.line, last.col + textWidth - 1
		}
		col = rows[row].col + max(0, x-r.x-gutter)
		if end := rows[row].end; end >= 0 { // Past a short row's end stays on that row.
			col = min(col, end-1)
		}
		return rows[row].line, col
	}
	a.hits = append(a.hits, hit{
		area: r,
		click: func(x, y int, mods tcell.ModMask) {
			st.focus = p
			wasFocused := a.tabFocused && a.tab() == it
			if !wasFocused {
				a.focusTab()
			}
			line, col := placeAt(x, y)
			if mods&tcell.ModAlt != 0 {
				e.ToggleCursor(line, col)
				return
			}
			switch a.mouse.clicks {
			case 2:
				e.SelectWord(line, col)
			case 3:
				e.SelectLine(line)
			default:
				e.GoTo(line, col, mods&tcell.ModShift != 0)
			}
		},
		drag: func(x, y int) { // Past either end, it scrolls as it selects.
			if y < r.y {
				e.ScrollBy(-1)
			} else if y >= r.y+r.h {
				e.ScrollBy(1)
			}
			line, col := placeAt(x, y)
			e.GoTo(line, col, true)
		},
		wheel: func(delta int) { e.ScrollBy(delta * wheelLines) },
		context: func(x, y int) {
			st.focus = p
			a.menu = &openMenu{group: -1, x: x, y: y + 1, items: editorMenu}
		},
	})
	if scrollbar {
		a.drawScrollbar(e, rect{r.x + r.w - 1, r.y, 1, r.h})
	}
}

// editorMenu is what the right button offers in a file.
var editorMenu = []menuItem{
	{label: "Cut", action: "edit.cut"},
	{label: "Copy", action: "edit.copy"},
	{label: "Paste", action: "edit.paste"},
	{},
	{label: "Select All", action: "select.all"},
	{label: "Find", action: "find.open"},
	{label: "Go to Definition", action: "lsp.definition"},
	{label: "Rename", action: "lsp.rename"},
	{label: "Format", action: "lsp.format"},
}

// drawScrollbar draws where the view is in the file. Clicking or dragging on it goes there.
func (a *app) drawScrollbar(e *editor.Editor, r rect) {
	lines := e.File.LineCount()
	size := max(1, r.h*r.h/lines)
	top := e.ScrollLine * r.h / lines
	for y := range r.h {
		char, style := '│', a.th.muted
		if y >= top && y < top+size {
			char, style = '█', a.th.border
		}
		a.screen.SetContent(r.x, r.y+y, char, nil, style)
	}
	jump := func(_, y int) { e.ScrollBy((y-r.y)*lines/r.h - e.ScrollLine) }
	a.hits = append(a.hits, hit{
		area:  r,
		click: func(x, y int, _ tcell.ModMask) { jump(x, y) },
		drag:  jump,
		wheel: func(delta int) { e.ScrollBy(delta * wheelLines) },
	})
}

// cellStyle gives selections, cursors and matches priority over syntax colour.
func (a *app) cellStyle(c editor.Cell, class syntax.Class) tcell.Style {
	switch {
	case c.Cursor && c.Selected:
		return a.th.otherCursor.Underline(true)
	case c.Cursor:
		return a.th.otherCursor
	case c.Selected:
		return a.th.active
	case c.Match:
		return a.th.match
	case c.Special:
		return a.th.muted
	}
	switch class {
	case syntax.Keyword:
		return a.th.keyword
	case syntax.String:
		return a.th.str
	case syntax.Number:
		return a.th.number
	case syntax.Constant:
		return a.th.constant
	case syntax.Function:
		return a.th.function
	case syntax.Type:
		return a.th.typ
	case syntax.Variable:
		return a.th.variable
	case syntax.Property:
		return a.th.property
	case syntax.Operator:
		return a.th.operator
	case syntax.Tag:
		return a.th.tag
	case syntax.Comment:
		return a.th.comment
	}
	return a.th.primary
}

func (a *app) drawStatus(w, h int) {
	y := h - 1
	switch it := a.tab(); {
	case len(a.prompts) > 0:
		a.drawPrompt(w, y)
	case a.command != nil:
		x := a.put(0, y, w, ":"+*a.command, a.th.primary)
		a.screen.ShowCursor(min(x, w-1), y)
	case a.find != nil:
		a.drawFind(w, y)
	case a.input != nil:
		a.drawInput(w, y)
	case a.message != "":
		a.put(1, y, w, a.message, a.th.primary)
	case a.onboard != nil: // Its keys are on its own screen.
	case a.git.busy != "":
		a.put(1, y, w, a.git.busy, a.th.accent)
	case a.tabFocused && it != nil && it.term != nil && it.term.ScrolledBack() > 0:
		a.put(1, y, w, fmt.Sprintf("Looking back %d lines. Scroll down or type to return.", it.term.ScrolledBack()), a.th.muted)
	default:
		a.drawStatusBar(it, w, y)
	}
}

// drawStatusBar is the usual status line: where the keys are going, what's useful there, and on the
// right where the cursor is and the buttons for the dock and word wrap.
func (a *app) drawStatusBar(it *tabItem, w, y int) {
	label, colour := a.mode()
	x := a.put(0, y, w, " "+label+" ", a.pill(colour)) + 1
	segments := []statusSegment{}
	hints := a.hints("file.save", "save", "find.open", "find", "command.open", "commands", "app.menu", "menu", "app.shortcuts", "keys")
	switch {
	case a.tabFocused && it != nil && it.ed != nil:
		line, col := it.ed.Position()
		if lang := syntax.ForPath(it.ed.File.Path); lang != nil {
			segments = append(segments, statusSegment{lang.Name + "  ", a.th.icon["cyan"], nil})
		}
		if it.ed.File.ReadOnly() {
			segments = append(segments, statusSegment{"Read-only  ", a.th.icon["red"], nil})
		}
		if n := it.ed.CursorCount(); n > 1 {
			segments = append(segments, statusSegment{fmt.Sprintf("%d cursors  ", n), a.th.icon["purple"], nil})
		}
		segments = append(segments, statusSegment{fmt.Sprintf("Ln %d, Col %d ", line+1, col+1), a.th.icon["yellow"], nil})
		wrap := either(a.cfg.Editor.WordWrap, " ↵ Wrap ", " → NoWrap ")
		segments = append(segments, statusSegment{wrap, a.th.icon["green"], a.toggleWrap})
	case a.tabFocused:
		hints = a.hints("tab.close", "close") + `   Ctrl+\ leaves`
		segments = append(segments,
			statusSegment{" ‹ Leave ", a.th.icon["green"], func() { a.tabFocused = false }},
			statusSegment{" × Close ", a.th.icon["red"], a.closeTab})
	case a.focusedView() == viewGit:
		hints = "c commit   s stage   u unstage   p push   l pull   Enter diff"
	case a.focusedView() == viewSearch:
		hints = "Enter open   Tab next field   Esc back"
	case a.focusedView() == viewAI:
		hints = "Enter open   n new   Del hide"
	default:
		hints = "Enter open   Tab go to editor   : commands   " + a.hints("app.menu", "menu", "project.switch", "projects", "directory.switch", "folders", "view.terminal", "terminal", "app.shortcuts", "keys")
	}
	segments = append(segments, a.dockSegment())
	free := a.drawSegments(segments, y, w)
	a.put(x, y, max(x, free-1), fitHints(hints, free-1-x), a.th.muted)
}

// fitHints drops whole hints from the end, so that none is ever cut in the middle of a word.
func fitHints(hints string, room int) string {
	parts := strings.Split(hints, "   ")
	for len(parts) > 0 && runewidth.StringWidth(strings.Join(parts, "   ")) > room {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, "   ")
}

// hints writes action and word pairs as "Key word", using whatever the keys are now.
func (a *app) hints(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		if key := a.keyFor(pairs[i]); key != "" {
			parts = append(parts, key+" "+pairs[i+1])
		}
	}
	return strings.Join(parts, "   ")
}

// drawPrompt asks on the status line. A question too long for it wraps upward, so the whole of
// it can be read.
func (a *app) drawPrompt(w, y int) {
	p := a.prompts[0]
	var options strings.Builder
	for _, c := range p.choices {
		fmt.Fprintf(&options, "  [%c] %s", c.key, c.label)
	}
	x := 1
	if room := w - utf8.RuneCountInString(options.String()) - 1; runewidth.StringWidth(p.text) <= room {
		x = a.put(1, y, w, p.text, a.th.accent)
	} else {
		lines := wrapText(p.text, max(1, w-2))
		for i, line := range lines {
			row := y - len(lines) + i
			for col := range w {
				a.screen.SetContent(col, row, ' ', nil, a.th.primary)
			}
			a.put(1, row, w, line, a.th.accent)
		}
	}
	a.hits = a.hits[:0] // A question takes everything until it's answered.
	for _, c := range p.choices {
		start := x + 2
		x = a.put(x, y, w, fmt.Sprintf("  [%c] %s", c.key, c.label), a.th.accent)
		choice := c
		a.hits = append(a.hits, hit{
			area:  rect{start, y, max(0, x-start), 1},
			click: func(int, int, tcell.ModMask) { a.answer(choice) },
		})
	}
}

// wrapText cuts text into lines at most width columns wide, between words when it can.
func wrapText(text string, width int) []string {
	var lines []string
	line := []rune{}
	used, space := 0, -1 // The last space in line, if any.
	for _, r := range text {
		w := runewidth.RuneWidth(r)
		if used+w > width {
			rest := []rune{}
			if space >= 0 {
				rest = slices.Clone(line[space+1:])
				line = line[:space]
			}
			lines = append(lines, string(line))
			line, used, space = rest, runewidth.StringWidth(string(rest)), -1
		}
		if r == ' ' {
			space = len(line)
		}
		line = append(line, r)
		used += w
	}
	return append(lines, string(line))
}

func (a *app) drawSwitcher(w, h int) {
	s := a.switcher
	matches := s.matches()
	bw := max(1, min(56, w-4))
	bh := max(1, min(len(matches), h-6))
	x0, y0 := (w-bw)/2, max(0, (h-bh)/2-2)
	a.frame(rect{x0, y0, bw, bh + 4}, a.th.borderFocus)
	maxX := x0 + bw - 2
	a.put(x0+2, y0, maxX, " "+s.title+" ", a.th.accent)
	end := a.put(x0+2, y0+1, maxX, "❯ ", a.th.icon["cyan"])
	end = a.put(end, y0+1, maxX, s.filter, a.th.primary)
	a.screen.ShowCursor(min(end, maxX), y0+1)
	a.modalRule(x0, y0+2, bw)
	if len(matches) == 0 {
		a.put(x0+2, y0+3, maxX, "No matches", a.th.muted)
	}
	// The switcher takes the mouse too: a row picks, and elsewhere cancels.
	a.hits = append(a.hits[:0], hit{
		area:  rect{0, 0, w, h},
		click: func(int, int, tcell.ModMask) { a.cancelSwitcher() },
	})
	top := max(0, s.sel-bh+1)
	for i := 0; i < bh && top+i < len(matches); i++ {
		style := a.th.primary
		prefix := "  "
		if top+i == s.sel {
			style, prefix = a.th.active, "❯ "
			for x := x0 + 1; x < x0+bw-1; x++ {
				a.screen.SetContent(x, y0+3+i, ' ', nil, a.th.active)
			}
		}
		a.put(x0+2, y0+3+i, maxX, prefix+s.labels[matches[top+i]], style)
		picked := matches[top+i]
		a.hits = append(a.hits, hit{
			area: rect{x0, y0 + 3 + i, bw, 1},
			click: func(int, int, tcell.ModMask) {
				a.switcher = nil
				s.pick(picked)
			},
		})
	}
}

func scrollbarWidth(shown bool) int {
	if shown {
		return 1
	}
	return 0
}
