package app

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Juan17la/lazycode/internal/editor"
	"github.com/Juan17la/lazycode/internal/session"
	"github.com/Juan17la/lazycode/internal/syntax"
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
	s.Clear()
	s.HideCursor()
	clear(a.hits) // Their closures hold closed tabs, whose screens are the biggest thing in memory.
	a.hits = a.hits[:0]
	w, h := s.Size()
	st := a.dir()

	x := a.put(0, 0, w, " "+a.projects[a.project].Name, a.th.primary.Bold(true))
	a.put(x, 0, w, " › "+filepath.Base(a.dirPath()), a.th.primary)

	first, second, _ := a.regions()
	a.drawPanel(st, 0, first)
	a.drawPanel(st, 1, second)

	dividers := a.layout(st)
	a.resizeTabs(st)
	for _, p := range st.root.panes() {
		a.drawPane(st, p)
	}
	for _, d := range dividers {
		a.fillLine(d)
	}
	if a.review != nil {
		a.drawReview(w, h)
	}
	if a.onboard != nil {
		a.drawOnboard(w, h)
	}
	a.drawStatus(w, h)
	if a.switcher != nil {
		a.drawSwitcher(w, h)
	}
	s.Show()
}

// fillLine draws a divider in r, which is one cell wide or one cell high.
func (a *app) fillLine(r rect) {
	char := '│'
	if r.w > 1 {
		char = '─'
	}
	for y := r.y; y < r.y+r.h; y++ {
		for x := r.x; x < r.x+r.w; x++ {
			a.screen.SetContent(x, y, char, nil, a.th.muted)
		}
	}
}

// drawPanel draws one panel and its divider. r is empty if it's hidden.
func (a *app) drawPanel(st *dirState, index int, r rect) {
	if r.w == 0 {
		return
	}
	w, _ := a.screen.Size()
	if r.x == 0 {
		a.fillLine(rect{r.x + r.w, r.y, 1, r.h})
	} else if r.x > 0 && r.x+r.w <= w {
		a.fillLine(rect{r.x - 1, r.y, 1, r.h})
	}
	switch a.viewIn(index) {
	case viewFiles:
		a.drawTree(st, r)
	case viewAI:
		a.drawSessions(r)
	case viewGit:
		a.drawGit(r)
	case viewSearch:
		a.drawSearch(r)
	default:
		a.put(r.x+1, r.y, r.x+r.w, "No views here", a.th.muted)
		a.put(r.x+1, r.y+1, r.x+r.w, ":view moves one", a.th.muted)
	}
	a.drawViewTitles(index, r)
}

func (a *app) drawTree(st *dirState, r rect) {
	maxX := r.x + r.w
	t := a.tree()
	y0, rows := r.y+1, r.h-1
	if t.Err != nil {
		a.put(r.x+1, y0, maxX, "Can't read this directory", a.th.primary)
		return
	}
	if len(t.Rows) == 0 {
		a.put(r.x+1, y0, maxX, "Empty directory", a.th.muted)
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
	})
	for i := st.top; i < len(t.Rows) && i < st.top+rows; i++ {
		row := t.Rows[i]
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
		style := a.th.primary
		if i == t.Sel {
			style = a.th.primary.Underline(true)
			if !a.tabFocused {
				style = a.th.active
			}
		}
		a.put(r.x+1, y0+i-st.top, maxX, strings.Repeat("  ", row.Depth)+marker+row.Name, style)
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
		a.put(content.x+1, content.y+2, content.x+content.w, "Enter opens a file. To start a tool, press : then type: term claude", a.th.muted)
		a.hits = append(a.hits, hit{area: content, click: func(int, int, tcell.ModMask) { st.focus = p }})
	case it.ed != nil:
		a.drawEditor(st, p, it, content, focused)
	default:
		it.term.Draw(a.screen, content.x, content.y, content.w, content.h)
		if x, y, visible := it.term.Cursor(); visible && focused && a.overlayFree() && a.tabFocused {
			a.screen.ShowCursor(content.x+x, content.y+y)
		}
		a.addTerminalHit(st, p, it, content)
	}
}

// overlayFree reports whether nothing else is taking the keys and showing a cursor.
func (a *app) overlayFree() bool {
	return a.switcher == nil && a.command == nil && a.find == nil && a.input == nil && a.review == nil && len(a.prompts) == 0
}

func (a *app) drawTabBar(st *dirState, p *pane, focused bool) {
	x, y, maxX := p.area.x+1, p.area.y, p.area.x+p.area.w
	for i, it := range p.tabs {
		label := " " + it.name() + " "
		switch {
		case it.ed != nil:
			if it.ed.File.Modified() {
				label = " " + it.name() + "* "
			}
		default:
			label = " " + it.marker() + " " + it.name() + " "
			if done, code := it.term.Exited(); done {
				label = fmt.Sprintf(" %s %s [exit %d] ", it.marker(), it.name(), code)
			}
		}
		style := a.th.muted
		if i == p.cur {
			style = a.th.primary.Bold(true)
			if focused && a.tabFocused {
				style = a.th.active
			}
		}
		if it.sess != nil && it.sess.status == session.Attention {
			style = a.th.success // The green detail (Q-36). The label's ● says it without colour.
			label = " ● " + it.name() + " "
		}
		start := x
		x = a.put(x, y, maxX, label, style)
		index := i
		a.hits = append(a.hits, hit{
			area: rect{start, y, x - start, 1},
			click: func(int, int, tcell.ModMask) {
				st.focus, p.cur = p, index
				a.focusTab()
			},
		})
		x++
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
	textWidth := max(1, r.w-gutter)
	e.Scroll(textWidth, r.h)
	current, _ := e.Position()
	colours := syntax.NewCursor(a.highlightSpans(e.File)) // Only what's on screen is looked up.
	for row := range r.h {
		line := e.ScrollLine + row
		if line >= e.File.LineCount() {
			break
		}
		if gutter > 0 {
			style := a.th.muted
			if line == current {
				style = a.th.primary
			}
			a.put(r.x, r.y+row, r.x+gutter, fmt.Sprintf(" %*d", gutter-2, line+1), style)
			if marked[line] {
				a.screen.SetContent(r.x, r.y+row, '▌', nil, a.th.accent)
			}
		}
		for _, c := range e.Row(line, textWidth) {
			a.screen.Put(r.x+gutter+c.X, r.y+row, c.Text, a.cellStyle(c, colours.At(c.Offset)))
		}
	}
	if focused && a.tabFocused && a.overlayFree() {
		if x, y := e.ScreenCursor(); x >= 0 && x < textWidth && y >= 0 && y < r.h {
			a.screen.ShowCursor(r.x+gutter+x, r.y+y)
		}
	}
	a.hits = append(a.hits, hit{
		area: r,
		click: func(x, y int, mods tcell.ModMask) {
			st.focus = p
			wasFocused := a.tabFocused && a.tab() == it
			if !wasFocused {
				a.focusTab()
			}
			e.GoTo(e.ScrollLine+y-r.y, e.ScrollCol+x-r.x-gutter, mods&tcell.ModShift != 0)
		},
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
	case syntax.Literal:
		return a.th.literal
	case syntax.Name:
		return a.th.name
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
	case a.tabFocused && it != nil && it.ed != nil:
		line, col := it.ed.Position()
		where := fmt.Sprintf("Ln %d, Col %d", line+1, col+1)
		if n := it.ed.CursorCount(); n > 1 {
			where = fmt.Sprintf("%d cursors  %s", n, where)
		}
		if it.ed.File.ReadOnly() {
			where = "Read-only  " + where
		}
		if lang := syntax.ForPath(it.ed.File.Path); lang != nil {
			where = lang.Name + "  " + where
		}
		a.put(max(1, w-utf8.RuneCountInString(where)-1), y, w, where, a.th.primary)
		a.put(1, y, max(1, w-utf8.RuneCountInString(where)-2), `Ctrl+S save   Ctrl+F find   Ctrl+\ files   Esc : commands`, a.th.muted)
	case a.tabFocused && it != nil && it.term != nil && it.term.ScrolledBack() > 0:
		a.put(1, y, w, fmt.Sprintf("Looking back %d lines. Scroll down or type to return.", it.term.ScrolledBack()), a.th.muted)
	case a.tabFocused:
		a.put(1, y, w, `Ctrl+\ leaves the terminal`, a.th.muted)
	default:
		a.put(1, y, w, "Enter open   Alt+O or :project   Alt+D or :dir   Alt+, Alt+. tabs   Tab enter tab   : commands", a.th.muted)
	}
}

// drawPrompt asks on the status line. A question too long for it wraps upward, because a trust
// question must show the whole of what would run.
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
	bw := max(1, min(50, w-4))
	bh := max(1, min(len(matches), h-6))
	x0, y0 := (w-bw)/2, max(0, (h-bh)/2-2)

	for y := y0; y < y0+bh+3; y++ {
		for x := x0; x < x0+bw; x++ {
			a.screen.SetContent(x, y, ' ', nil, a.th.primary)
		}
	}
	maxX := x0 + bw
	a.put(x0+1, y0, maxX, s.title, a.th.accent)
	end := a.put(x0+1, y0+1, maxX, "> "+s.filter, a.th.primary)
	a.screen.ShowCursor(min(end, maxX-1), y0+1)
	if len(matches) == 0 {
		a.put(x0+1, y0+2, maxX, "No matches", a.th.muted)
	}
	// The switcher takes the mouse too: a row picks, and elsewhere cancels.
	a.hits = append(a.hits[:0], hit{
		area:  rect{0, 0, w, h},
		click: func(int, int, tcell.ModMask) { a.switcher = nil },
	})
	for i := 0; i < bh && i < len(matches); i++ {
		style := a.th.primary
		prefix := "  "
		if i == s.sel {
			style, prefix = a.th.active, "› "
		}
		a.put(x0+1, y0+2+i, maxX, prefix+s.labels[matches[i]], style)
		picked := matches[i]
		a.hits = append(a.hits, hit{
			area: rect{x0, y0 + 2 + i, bw, 1},
			click: func(int, int, tcell.ModMask) {
				a.switcher = nil
				s.pick(picked)
			},
		})
	}
}
