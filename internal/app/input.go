package app

import (
	"strings"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
)

// inputBar asks for one line of text on the status line.
type inputBar struct {
	label string
	text  string
	done  func(text string)
}

func (a *app) ask1Line(label, text string, done func(string)) {
	a.input = &inputBar{label: label, text: text, done: done}
}

func (b *inputBar) paste(text string) {
	b.text += strings.NewReplacer("\r", "", "\n", "").Replace(text)
}

func (a *app) handleInputKey(ev *tcell.EventKey) {
	b := a.input
	switch ev.Key() {
	case tcell.KeyEsc:
		a.input = nil
	case tcell.KeyEnter:
		a.input = nil
		if text := strings.TrimSpace(b.text); text != "" {
			b.done(text)
		}
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		_, size := utf8.DecodeLastRuneInString(b.text)
		b.text = b.text[:len(b.text)-size]
	case tcell.KeyRune:
		if ev.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) == 0 {
			b.text += string(ev.Rune())
		}
	}
}

func (a *app) drawInput(w, y int) {
	x := a.put(1, y, w, a.input.label+" ", a.th.accent)
	end := a.put(x, y, w, a.input.text, a.th.primary)
	a.screen.ShowCursor(min(end, max(0, w-1)), y)
}
