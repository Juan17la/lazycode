package app

import (
	"github.com/Juan17la/lazycode/internal/run"
)

// playCommand is :play (Q-56, Q-57). With a command it remembers that for this directory. Without
// one it goes to the tab if it's still running, or works out what to run.
func (a *app) playCommand(command string) {
	st := a.dir()
	dir := a.dirPath()
	if command != "" {
		a.rememberAndRun(dir, command)
		return
	}
	if it := st.play; it != nil {
		if done, _ := it.term.Exited(); !done {
			_, p := a.findTab(it)
			if p != nil {
				st.focus, p.cur = p, indexOf(p, it)
				a.tabFocused = true
			}
			return
		}
	}
	saved, ok, err := a.playSaved.Command(dir)
	if err != nil {
		a.message = err.Error()
		return
	}
	if ok {
		a.runPlay(dir, saved)
		return
	}
	if detected, ok := run.Detect(dir); ok {
		a.runPlay(dir, detected)
		return
	}
	a.ask1Line("Command to run here:", "", func(typed string) { a.rememberAndRun(dir, typed) })
}

func indexOf(p *pane, it *tabItem) int {
	for i, other := range p.tabs {
		if other == it {
			return i
		}
	}
	return p.cur
}

func (a *app) rememberAndRun(dir, command string) {
	if err := a.playSaved.Remember(dir, command); err != nil {
		a.message = "That won't be remembered: " + err.Error()
	}
	a.runPlay(dir, command)
}

// runPlay runs the command in a new tab, replacing the finished tab from the last run. The command is
// shown before anything runs in a folder that isn't trusted (doc 11).
func (a *app) runPlay(dir, command string) {
	a.requireTrust(dir, command, func() {
		st := a.dir()
		if old := st.play; old != nil {
			if _, p := a.findTab(old); p != nil {
				a.removeTab(st, p, old)
			}
		}
		it, err := a.openTerminal(dir, run.Shell(command))
		if err != nil {
			a.message = err.Error()
			return
		}
		it.term.Name = command
		it.play = true
		st.play = it
	})
}
