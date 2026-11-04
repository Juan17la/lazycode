package app

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Juan17la/lazycode/internal/keymap"
	"github.com/gdamore/tcell/v2"
)

// countLimit stops counting matches from slowing typing in a large file.
const countLimit = 10_000

// findBar is find and replace in one file (Q-17). It takes the keys while it's open.
type findBar struct {
	it            *tabItem
	query         string
	replacement   string
	replacing     bool
	inReplacement bool // Typing goes to the replacement, not the query.
	origin        int  // Where matching starts as the query is typed.
	count         int
}

func (a *app) openFind(it *tabItem, replacing bool) {
	start, end := it.ed.Selection()
	if a.find == nil || a.find.it != it {
		a.closeFind()
		a.find = &findBar{it: it, origin: start}
		// Selected text on one line is what you'd want to look for.
		if selected := string(it.ed.File.Slice(start, end)); selected != "" && !strings.ContainsAny(selected, "\r\n") {
			a.find.query = selected
		}
		a.find.update()
	}
	a.find.replacing = a.find.replacing || replacing
	a.find.inReplacement = replacing
}

func (a *app) closeFind() {
	if a.find != nil {
		a.find.it.ed.Highlight = ""
		a.find = nil
	}
}

func (f *findBar) update() {
	f.it.ed.Highlight = f.query
	f.count = f.it.ed.CountMatches(f.query, countLimit)
}

// edit changes whichever field has focus.
func (f *findBar) edit(change func(string) string) {
	if f.inReplacement {
		f.replacement = change(f.replacement)
		return
	}
	f.query = change(f.query)
	f.update()
	f.it.ed.FindFrom(f.query, f.origin)
}

func (f *findBar) paste(a *app, text string) {
	// Plain text on one line (Q-17).
	text = strings.NewReplacer("\r", "", "\n", "").Replace(text)
	f.edit(func(s string) string { return s + text })
}

func (a *app) handleFindKey(ev *tcell.EventKey) {
	f := a.find
	e := f.it.ed
	switch name := keymap.Name(ev); name {
	case "esc":
		a.closeFind()
	case "enter":
		if f.inReplacement {
			a.replaceNext()
		} else {
			e.FindNext(f.query)
		}
	case "down", "f3":
		e.FindNext(f.query)
	case "up", "shift+enter", "shift+f3":
		e.FindPrevious(f.query)
	case "tab", "shift+tab":
		f.inReplacement = f.replacing && !f.inReplacement
	case "ctrl+f":
		f.inReplacement = false
	case "alt+h":
		f.replacing, f.inReplacement = true, true
	case "alt+a", "ctrl+a":
		if f.replacing {
			a.replaceAll()
		}
	case "backspace":
		f.edit(func(s string) string {
			_, size := utf8.DecodeLastRuneInString(s)
			return s[:len(s)-size]
		})
	default:
		if ev.Key() == tcell.KeyRune && ev.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) == 0 {
			f.edit(func(s string) string { return s + string(ev.Rune()) })
		}
	}
}

func (a *app) replaceNext() {
	f := a.find
	a.editWith(f.it, func() error { return f.it.ed.ReplaceNext(f.query, f.replacement) })
	f.update()
	if f.count == 0 {
		a.message = "No more matches."
	}
}

func (a *app) replaceAll() {
	f := a.find
	a.editWith(f.it, func() error {
		n, err := f.it.ed.ReplaceAll(f.query, f.replacement)
		if err == nil {
			a.message = fmt.Sprintf("Replaced %d.", n)
		}
		return err
	})
	f.update()
}

func (a *app) drawFind(w, y int) {
	f := a.find
	count := fmt.Sprintf("%d matches", f.count)
	if f.count >= countLimit {
		count = fmt.Sprintf("%d+ matches", countLimit)
	}
	x := a.put(1, y, w, "Find: ", a.th.accent)
	cursor := a.put(x, y, w, f.query, a.th.primary)
	x = cursor
	if f.replacing {
		x = a.put(x, y, w, "  Replace: ", a.th.accent)
		end := a.put(x, y, w, f.replacement, a.th.primary)
		if f.inReplacement {
			cursor = end
		}
		x = end
	}
	hint := "  ↑↓ move  Esc close"
	if f.replacing {
		hint = "  Enter replace  Alt+A all  Tab field  Esc close"
	}
	x = a.put(x, y, w, "  "+count, a.th.primary)
	if a.message != "" { // What just happened matters more than the hint.
		a.put(x, y, w, "  "+a.message, a.th.accent)
	} else {
		a.put(x, y, w, hint, a.th.muted)
	}
	a.screen.ShowCursor(min(cursor, max(0, w-1)), y)
}
