package app

import (
	"slices"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
)

// wheelLines is how far one wheel notch scrolls a file.
const wheelLines = 3

// doubleClick is the longest wait between clicks that count as one double or triple click.
const doubleClick = 400 * time.Millisecond

// hit is something on screen that the mouse can reach, recorded as it's drawn. So the mouse
// follows the layout.
type hit struct {
	area    rect
	click   func(x, y int, mods tcell.ModMask)
	wheel   func(delta int)
	raw     func(ev *tcell.EventMouse) // Gets every event in its area, for a terminal tab.
	drag    func(x, y int)             // Follows the mouse from a press until its release.
	middle  func()
	context func(x, y int) // The right button.
}

// mouseState follows the left button between events.
type mouseState struct {
	held     bool
	dragging func(x, y int)
	released func(x, y int)
	clicks   int // 1, 2 or 3 for the press being handled.
	lastAt   time.Time
	lastX    int
	lastY    int
}

func (a *app) handleMouse(ev *tcell.EventMouse) {
	x, y := ev.Position()
	buttons := ev.Buttons()
	m := &a.mouse
	left := buttons&tcell.Button1 != 0
	if m.dragging != nil || m.released != nil { // A drag owns the mouse wherever it goes.
		if left {
			if m.dragging != nil {
				m.dragging(x, y)
			}
			return
		}
		if m.released != nil {
			m.released(x, y)
		}
		m.dragging, m.released, m.held = nil, nil, false
		return
	}
	pressed := left && !m.held
	repeat := left && m.held // The same press, moved. Only a drag wants that, and none is going on.
	m.held = left
	a.message = ""
	if pressed {
		if now := time.Now(); x == m.lastX && y == m.lastY && now.Sub(m.lastAt) < doubleClick {
			m.clicks = m.clicks%3 + 1
			m.lastAt = now
		} else {
			m.clicks, m.lastAt, m.lastX, m.lastY = 1, now, x, y
		}
		a.termSel = termSelection{}
	}
	if !repeat {
		if first, second, _ := a.regions(); first.contains(x, y) {
			a.focusPanel = 0
		} else if second.contains(x, y) {
			a.focusPanel = 1
		}
	}
	// Later entries were drawn on top.
	for _, h := range slices.Backward(a.hits) {
		if !h.area.contains(x, y) {
			continue
		}
		switch {
		case h.raw != nil:
			if !repeat {
				h.raw(ev)
			}
		case buttons&tcell.WheelUp != 0 && h.wheel != nil:
			h.wheel(-1)
		case buttons&tcell.WheelDown != 0 && h.wheel != nil:
			h.wheel(1)
		case buttons&tcell.Button3 != 0 && h.middle != nil:
			h.middle()
		case buttons&tcell.Button2 != 0 && h.context != nil:
			h.context(x, y)
		case repeat:
		case left && h.click != nil:
			a.closeFind()
			h.click(x, y, ev.Modifiers())
			m.dragging = h.drag
		default:
			continue
		}
		return
	}
}

// termSelection is text being picked in a terminal tab with the mouse, in screen cells. Only what's on
// screen can be picked: the tab's scrollback isn't reachable from here, but the wheel can bring it on screen first.
type termSelection struct {
	it             *tabItem
	area           rect
	x0, y0, x1, y1 int
}

// ordered returns the selection's ends, in reading order.
func (s termSelection) ordered() (x0, y0, x1, y1 int) {
	if s.y0 < s.y1 || (s.y0 == s.y1 && s.x0 <= s.x1) {
		return s.x0, s.y0, s.x1, s.y1
	}
	return s.x1, s.y1, s.x0, s.y0
}

// rowSpan returns the columns selected on a row, as a half-open range.
func (s termSelection) rowSpan(y int) (from, to int) {
	x0, y0, x1, y1 := s.ordered()
	from, to = s.area.x, s.area.x+s.area.w
	if y == y0 {
		from = x0
	}
	if y == y1 {
		to = x1 + 1
	}
	return from, to
}

// paintTermSelection reverses what's selected, over what the tab drew.
func (a *app) paintTermSelection(it *tabItem) {
	s := a.termSel
	if s.it != it {
		return
	}
	_, y0, _, y1 := s.ordered()
	for y := y0; y <= y1; y++ {
		from, to := s.rowSpan(y)
		for x := from; x < to; x++ {
			text, style, _ := a.screen.Get(x, y)
			a.screen.Put(x, y, text, style.Reverse(true))
		}
	}
}

// copyTermSelection copies what's selected. A press with no drag just clears it.
func (a *app) copyTermSelection() {
	s := a.termSel
	if s.it == nil || (s.x0 == s.x1 && s.y0 == s.y1) {
		a.termSel = termSelection{}
		return
	}
	_, y0, _, y1 := s.ordered()
	var lines []string
	for y := y0; y <= y1; y++ {
		from, to := s.rowSpan(y)
		var line strings.Builder
		for x := from; x < to; {
			text, _, width := a.screen.Get(x, y)
			line.WriteString(text)
			x += max(1, width)
		}
		lines = append(lines, strings.TrimRight(line.String(), " "))
	}
	a.copyText(strings.Join(lines, "\n"))
	a.message = "Copied."
}

// addTerminalHit gives focus on a click, and passes mouse events to the tool if it asked for them
// (doc 07). If it didn't, dragging selects text and copies it on release.
func (a *app) addTerminalHit(st *dirState, p *pane, it *tabItem, r rect) {
	a.hits = append(a.hits, hit{
		area: r,
		raw: func(ev *tcell.EventMouse) {
			x, y := ev.Position()
			buttons := ev.Buttons()
			if buttons&tcell.Button1 != 0 {
				a.closeFind()
				st.focus = p
				a.focusTab()
				if !it.term.WantsMouse() {
					clampX := func(v int) int { return max(r.x, min(v, r.x+r.w-1)) }
					clampY := func(v int) int { return max(r.y, min(v, r.y+r.h-1)) }
					a.termSel = termSelection{it: it, area: r, x0: x, y0: y, x1: x, y1: y}
					a.mouse.dragging = func(x, y int) { a.termSel.x1, a.termSel.y1 = clampX(x), clampY(y) }
					a.mouse.released = func(int, int) { a.copyTermSelection() }
					return
				}
			}
			it.term.Mouse(x-r.x, y-r.y, buttons&tcell.WheelUp != 0, buttons&tcell.WheelDown != 0, buttons&tcell.Button1 != 0)
		},
	})
}
