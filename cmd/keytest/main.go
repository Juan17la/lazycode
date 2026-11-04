// Keytest checks that LazyCode's default keys reach it in a real terminal (M0-07).
// Run it in the terminal to check, press each key listed, then press q.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/Juan17la/lazycode/internal/term"
	"github.com/gdamore/tcell/v2"
)

type check struct {
	label string
	match func(*tcell.EventKey) bool
	seen  bool
}

func ctrl(k tcell.Key, label string) check {
	return check{label: label, match: func(e *tcell.EventKey) bool { return e.Key() == k }}
}

func plain(k tcell.Key, label string) check {
	return check{label: label, match: func(e *tcell.EventKey) bool {
		return e.Key() == k && e.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) == 0
	}}
}

// alt matches the same way the app does: a character with Alt held.
func alt(r rune, label string) check {
	return check{label: label, match: func(e *tcell.EventKey) bool {
		return e.Key() == tcell.KeyRune && e.Modifiers()&tcell.ModAlt != 0 && strings.EqualFold(string(e.Rune()), string(r))
	}}
}

func checks(goos string) []check {
	list := []check{
		{label: `Ctrl+\`, match: term.IsLeaveKey},
		ctrl(tcell.KeyCtrlB, "Ctrl+B"), ctrl(tcell.KeyCtrlC, "Ctrl+C"), ctrl(tcell.KeyCtrlD, "Ctrl+D"),
		ctrl(tcell.KeyCtrlF, "Ctrl+F"), ctrl(tcell.KeyCtrlN, "Ctrl+N"),
		ctrl(tcell.KeyCtrlO, "Ctrl+O"), ctrl(tcell.KeyCtrlP, "Ctrl+P"), ctrl(tcell.KeyCtrlS, "Ctrl+S"),
		ctrl(tcell.KeyCtrlV, "Ctrl+V"), ctrl(tcell.KeyCtrlW, "Ctrl+W"), ctrl(tcell.KeyCtrlX, "Ctrl+X"),
		ctrl(tcell.KeyCtrlY, "Ctrl+Y"), ctrl(tcell.KeyCtrlZ, "Ctrl+Z"),
		plain(tcell.KeyF2, "F2"), plain(tcell.KeyF12, "F12"), plain(tcell.KeyEsc, "Esc"),
	}
	if goos == "darwin" { // Q-80: Ctrl+K then the same key replaces Alt here.
		return append(list, ctrl(tcell.KeyCtrlK, "Ctrl+K"))
	}
	return append(list,
		alt('d', "Alt+D"), alt('o', "Alt+O"), alt('p', "Alt+P"), alt('f', "Alt+F"),
		alt('h', "Alt+H"), alt('s', "Alt+S"), alt(',', "Alt+,"), alt('.', "Alt+."),
		check{label: "Ctrl+F2", match: func(e *tcell.EventKey) bool {
			return e.Key() == tcell.KeyF2 && e.Modifiers()&tcell.ModCtrl != 0
		}},
	)
}

func main() {
	reportPath := flag.String("report", "", "also write the result to this file")
	flag.Parse()

	screen, err := tcell.NewScreen()
	if err != nil {
		fmt.Fprintln(os.Stderr, "keytest:", err)
		os.Exit(1)
	}
	if err := screen.Init(); err != nil {
		fmt.Fprintln(os.Stderr, "keytest:", err)
		os.Exit(1)
	}
	screen.EnablePaste() // As the app does, so that keys arrive the same way.

	list := checks(runtime.GOOS)
	var unmatched []string
	last := ""
	pastes := 0
	for {
		draw(screen, list, last)
		switch ev := screen.PollEvent().(type) {
		case *tcell.EventResize:
			screen.Sync()
		case *tcell.EventPaste:
			if ev.Start() {
				pastes++
				last = "a paste. The terminal took this key for itself."
				// Windows Terminal takes Ctrl+V and pastes. That's fine: the app handles a
				// paste event the way it would handle Ctrl+V.
				for i := range list {
					if list[i].label == "Ctrl+V" {
						list[i].seen = true
					}
				}
			}
		case *tcell.EventKey:
			if ev.Key() == tcell.KeyRune && ev.Rune() == 'q' && ev.Modifiers() == 0 {
				screen.Fini()
				finish(list, unmatched, pastes, *reportPath)
				return
			}
			last = ev.Name()
			matched := false
			for i := range list {
				if list[i].match(ev) {
					list[i].seen, matched = true, true
				}
			}
			if !matched {
				unmatched = append(unmatched, ev.Name())
			}
		}
	}
}

func draw(s tcell.Screen, list []check, last string) {
	s.Clear()
	put := func(x, y int, text string) {
		for i, r := range text {
			s.SetContent(x+i, y, r, nil, tcell.StyleDefault)
		}
	}
	put(0, 0, "Press each key below. Press q when finished.")
	for i, c := range list {
		mark := "[ ]"
		if c.seen {
			mark = "[x]"
		}
		put(0+(i/14)*18, 2+i%14, mark+" "+c.label)
	}
	put(0, 17, "Last key: "+last)
	s.Show()
}

func finish(list []check, unmatched []string, pastes int, reportPath string) {
	var b strings.Builder
	fmt.Fprintf(&b, "OS=%s TERM=%s TERM_PROGRAM=%s WT_SESSION=%s\n", runtime.GOOS, os.Getenv("TERM"), os.Getenv("TERM_PROGRAM"), os.Getenv("WT_SESSION"))
	for _, c := range list {
		status := "MISSING"
		if c.seen {
			status = "ok"
		}
		fmt.Fprintf(&b, "%-8s %s\n", c.label, status)
	}
	fmt.Fprintf(&b, "pastes: %d\nother keys received: %s\n", pastes, strings.Join(unmatched, ", "))
	fmt.Print(b.String())
	if reportPath != "" {
		if err := os.WriteFile(reportPath, []byte(b.String()), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "keytest:", err)
			os.Exit(1)
		}
	}
}
