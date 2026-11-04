package app

import (
	"slices"

	"github.com/gdamore/tcell/v2"
)

// wheelLines is how far one wheel notch scrolls a file.
const wheelLines = 3

// hit is something on screen that the mouse can reach, recorded as it's drawn. So the mouse
// follows the layout, and dragging can use the same regions later (Q-13).
type hit struct {
	area  rect
	click func(x, y int, mods tcell.ModMask)
	wheel func(delta int)
	raw   func(ev *tcell.EventMouse) // Gets every event in its area, for a terminal tab.
}

func (a *app) handleMouse(ev *tcell.EventMouse) {
	x, y := ev.Position()
	buttons := ev.Buttons()
	a.message = ""
	// Later entries were drawn on top.
	for _, h := range slices.Backward(a.hits) {
		if !h.area.contains(x, y) {
			continue
		}
		switch {
		case h.raw != nil:
			h.raw(ev)
		case buttons&tcell.WheelUp != 0 && h.wheel != nil:
			h.wheel(-1)
		case buttons&tcell.WheelDown != 0 && h.wheel != nil:
			h.wheel(1)
		case buttons&tcell.Button1 != 0 && h.click != nil:
			a.closeFind()
			h.click(x, y, ev.Modifiers())
		default:
			continue
		}
		return
	}
}

// addTerminalHit gives focus on a click, and passes mouse events to the tool if it asked
// for them (doc 07). The emulator ignores them if it didn't.
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
			}
			it.term.Mouse(x-r.x, y-r.y, buttons&tcell.WheelUp != 0, buttons&tcell.WheelDown != 0, buttons&tcell.Button1 != 0)
		},
	})
}
