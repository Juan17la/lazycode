package app

import (
	"path/filepath"
	"slices"
	"time"
)

// AI mode: one key swaps the editor for two panes, the AI tree on the left and the chosen tool's own
// screen on the right. Its tab still belongs to its folder's dock, so leaving puts it back as it was.

// toggleAIMode is Alt+A. Coming in goes back to the session used last, in its own project.
func (a *app) toggleAIMode() {
	a.closeFind()
	if a.aiMode {
		a.aiMode = false
		if st := a.dir(); a.tabFocused && st.focus == st.dock {
			a.dockHidden = false // The tool that was on screen stays on screen.
		}
		return
	}
	a.aiMode = true
	a.tabFocused = false
	e := a.sessions.cur
	if e == nil || !slices.Contains(a.sessions.entries, e) || e.Hidden {
		e = nil
		for _, one := range a.sessions.entries {
			if !one.Hidden && (e == nil || one.Used.After(e.Used)) {
				e = one
			}
		}
	}
	a.sessions.cur = e
	if e == nil {
		a.selectASession()
		return
	}
	a.selectEntry(e)
	if e.tab != nil {
		a.openEntry(e)
	}
}

// aiSync keeps AI mode and the focused tab in step. A session that gets focus is the one shown, and a
// file that gets it (opened by name, say) belongs to the editor, so AI mode ends.
func (a *app) aiSync() {
	if !a.aiMode || !a.tabFocused {
		return
	}
	switch it := a.tab(); {
	case it == nil:
		a.tabFocused = false
	case it.sess != nil:
		a.sessions.cur = it.sess
	default:
		a.aiMode = false
	}
}

// agentTab is the tab Ctrl+Space goes into: the shown session's in AI mode, or else the current one.
func (a *app) agentTab() *tabItem {
	if !a.aiMode {
		return a.tab()
	}
	if e := a.sessions.cur; e != nil {
		return e.tab
	}
	return nil
}

// enterTab focuses the agentTab.
func (a *app) enterTab() {
	if a.aiMode {
		a.openEntry(a.sessions.cur)
		return
	}
	a.focusTab()
}

// doInAIMode handles the keys that mean something else in AI mode. It reports whether it did.
func (a *app) doInAIMode(action string) bool {
	switch action {
	case "focus.next", "tab.enter": // There are two areas: the tree and the tool.
		if a.tabFocused {
			a.tabFocused = false
		} else if a.agentTab() != nil {
			a.enterTab()
		}
	case "tab.previous":
		a.stepSession(-1)
	case "tab.next":
		a.stepSession(1)
	case "split.right", "split.down", "view.move", "file.new", "view.terminal":
		a.aiMode = false
		return false // Editor things, done in the editor.
	default:
		return false
	}
	return true
}

// stepSession shows the session above or below the shown one in the tree. A sleeping one is only
// shown: Enter resumes it.
func (a *app) stepSession(delta int) {
	var list []*sessionEntry
	for _, row := range a.aiRows() {
		if row.e != nil {
			list = append(list, row.e)
		}
	}
	if len(list) == 0 {
		return
	}
	i := (slices.Index(list, a.sessions.cur) + delta + len(list)) % len(list)
	if a.sessions.cur == nil && delta < 0 {
		i = len(list) - 1
	}
	e := list[i]
	a.selectEntry(e)
	if e.tab != nil {
		a.openEntry(e)
		return
	}
	a.sessions.cur, a.tabFocused = e, false
}

// drawAIMode draws the tree in the first region and the shown tool in the rest.
func (a *app) drawAIMode(st *dirState) {
	left, _, right := a.regions()
	style := a.th.border
	if !a.tabFocused {
		style = a.th.borderFocus
	}
	a.fillBorder(rect{left.x + left.w, left.y, 1, left.h}, style)
	a.put(left.x+1, left.y, left.x+left.w, a.viewIcon(viewAI)+" AI sessions", a.th.primary)
	a.fillBorder(rect{left.x, left.y + 1, left.w, 1}, style)
	a.drawSessions(rect{left.x + 1, left.y + 1, left.w - 2, left.h - 1})

	e := a.sessions.cur
	maxX := right.x + right.w
	title := "No session"
	if e != nil {
		title = e.marker() + " " + e.Name
	}
	x := a.put(right.x+1, right.y, maxX, title, a.th.primary)
	if e != nil {
		a.put(x, right.y, maxX, "  "+filepath.Base(e.Dir), a.th.muted)
	}
	a.fillBorder(rect{right.x, right.y + 1, right.w, 1}, a.th.border)
	content := rect{right.x, right.y + 2, right.w, max(0, right.h-2)}
	switch {
	case e == nil:
		a.put(content.x+1, content.y+1, maxX, "No sessions yet. n starts one, from the tree.", a.th.muted)
		a.put(content.x+1, content.y+2, maxX, a.keyFor("ai.mode")+" goes back to the editor.", a.th.muted)
	case e.tab == nil || e.tab.term == nil:
		a.put(content.x+1, content.y+1, maxX, e.Name+" is asleep, and Enter in the tree resumes it.", a.th.muted)
	default:
		it := e.tab
		a.fitTerm(it, content)
		it.term.Draw(a.screen, content.x, content.y, content.w, content.h)
		a.paintTermSelection(it)
		if x, y, visible := it.term.Cursor(); visible && a.tabFocused && a.overlayFree() {
			a.screen.ShowCursor(content.x+x, content.y+y)
		}
		if owner, p := a.findTab(it); owner == st {
			a.addTerminalHit(st, p, it, content)
		}
	}
}

// fitTerm tells a terminal tab the size it's drawn at.
func (a *app) fitTerm(it *tabItem, size rect) {
	if it.term == nil || (it.w == size.w && it.h == size.h) {
		return
	}
	it.term.Resize(size.w, size.h)
	it.w, it.h = size.w, size.h
	if it.sess != nil { // A tool redraws when resized, which isn't it working.
		it.sess.ignoreUntil = time.Now().Add(resizeNoise)
	}
}
