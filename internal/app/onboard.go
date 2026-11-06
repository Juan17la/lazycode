package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/Juan17la/lazycode/internal/config"
	"github.com/Juan17la/lazycode/internal/lsp"
	"github.com/Juan17la/lazycode/internal/run"
	"github.com/gdamore/tcell/v2"
)

// askFirstQuestions turns off onboarding and the trust question, which would otherwise stop every
// test that isn't about them at the first key.
var askFirstQuestions = true

// onboarding is the first-run screen (doc 07). It isn't shown again once config.toml exists, so it
// needs no marker of its own, and anyone who already has settings never sees it.
type onboarding struct {
	light, autoSave bool
	tools           []toolChoice
	row             int
}

type toolChoice struct {
	lsp.Language
	status string // "installed", "missing", or "needs" and the tool it can't install without.
	picked bool
}

// The rows that keys move between. The language tools follow these.
const (
	rowTheme = iota
	rowAutoSave
	rowTools
)

// firstQuestions opens onboarding on a first run, and otherwise asks about this folder.
func (a *app) firstQuestions() {
	if _, err := os.Stat(a.settingsPath("config.toml")); askFirstQuestions && errors.Is(err, fs.ErrNotExist) {
		a.onboard = newOnboarding()
		return
	}
	a.askToTrust()
}

func newOnboarding() *onboarding {
	o := &onboarding{}
	for _, lang := range lsp.Languages {
		status := "missing"
		needs := strings.Fields(lang.Server.Install)[0]
		if _, err := exec.LookPath(lang.Server.Command); err == nil {
			status = "installed"
		} else if _, err := exec.LookPath(needs); err != nil {
			status = "needs " + needs
		}
		o.tools = append(o.tools, toolChoice{Language: lang, status: status})
	}
	return o
}

func (o *onboarding) rows() int { return rowTools + len(o.tools) }

// changeOnboardRow flips what the row holds. Only a tool that's missing can be picked.
func (a *app) changeOnboardRow() {
	o := a.onboard
	switch o.row {
	case rowTheme:
		o.light = !o.light
		a.setTheme(o.theme()) // So the choice can be seen while making it.
	case rowAutoSave:
		o.autoSave = !o.autoSave
	default:
		if t := &o.tools[o.row-rowTools]; t.status == "missing" {
			t.picked = !t.picked
		}
	}
}

func (o *onboarding) theme() string {
	if o.light {
		return "light"
	}
	return "dark"
}

func (a *app) handleOnboardKey(ev *tcell.EventKey) {
	o := a.onboard
	switch ev.Key() {
	case tcell.KeyEsc:
		a.finishOnboarding(&onboarding{})
	case tcell.KeyEnter:
		a.finishOnboarding(o)
	case tcell.KeyUp, tcell.KeyBacktab:
		o.row = (o.row + o.rows() - 1) % o.rows()
	case tcell.KeyDown, tcell.KeyTab:
		o.row = (o.row + 1) % o.rows()
	case tcell.KeyLeft, tcell.KeyRight:
		a.changeOnboardRow()
	case tcell.KeyRune:
		if ev.Rune() == ' ' {
			a.changeOnboardRow()
		}
	}
}

// finishOnboarding saves the choices, then asks before running each picked install. Skipping
// passes an empty one: the defaults are saved too, because a skipped screen stays skipped.
func (a *app) finishOnboarding(o *onboarding) {
	a.onboard = nil
	a.cfg.UI.Theme, a.cfg.Editor.AutoSave = o.theme(), o.autoSave
	a.setTheme(o.theme())
	if err := config.EnsureFile(a.settingsPath("config.toml"), config.FirstConfig(o.theme(), o.autoSave)); err != nil {
		a.message = "Your choices weren't saved, so this screen will come back: " + err.Error()
	}
	for _, t := range o.tools {
		if t.picked {
			command := t.Server.Install
			a.ask(fmt.Sprintf("Install %s for %s by running: %s", t.Server.Name, t.Name, command),
				choice{'r', "Run", func() { a.runInstall(command) }},
				choice{'s', "Skip", func() {}})
		}
	}
	a.askToTrust()
}

// runInstall shows the install in a terminal tab, so its output and any failure can be read. It runs
// in the home folder, because from a project a go.mod could pick the Go toolchain and an .npmrc the
// package registry, before that folder has been trusted. Install is a fixed string, so a shell is
// safe, and on Windows it's what finds npm.cmd.
func (a *app) runInstall(command string) {
	home, err := os.UserHomeDir()
	if err != nil {
		a.message = "Can't install without a home folder: " + err.Error()
		return
	}
	it, err := a.openTerminal(home, run.Shell(command))
	if err != nil {
		a.message = err.Error()
		return
	}
	it.term.Name = command
}

// detail explains the row the cursor is on. A command gets a line of its own, so it can be read whole.
func (o *onboarding) detail() []string {
	if o.row < rowTools {
		return []string{"Space changes this."}
	}
	t := o.tools[o.row-rowTools]
	switch t.status {
	case "installed":
		return []string{t.Server.Name + " is ready to use."}
	case "missing":
		return []string{"Space selects it. You'll be asked before this runs:", t.Server.Install}
	}
	return []string{strings.TrimPrefix(t.status, "needs ") + " isn't installed, and this needs it:", t.Server.Install}
}

// onboardNotes are what the screen discloses or explains (Q-79, Q-80).
func onboardNotes(goos string) []string {
	notes := []string{"LazyCode looks on GitHub for a new version once a day. Set check = false under [updates] in :config to stop that."}
	if goos == "darwin" {
		notes = append(notes, "On macOS, shortcuts shown as Alt+key are Ctrl+K, then that key. Option types special characters by default.")
	}
	return notes
}

func (a *app) drawOnboard(w, h int) {
	o := a.onboard
	width := max(1, min(62, w-2))
	var notes []string
	for _, part := range o.detail() {
		notes = append(notes, wrapText(part, width)...)
	}
	notes = append(notes, "")
	for _, note := range onboardNotes(runtime.GOOS) {
		notes = append(notes, append(wrapText(note, width), "")...)
	}
	x0, y := (w-width)/2, max(0, (h-(8+len(o.tools)+len(notes)))/2)
	maxX := x0 + width

	a.hits = append(a.hits[:0], hit{area: rect{0, 0, w, h}, click: func(int, int, tcell.ModMask) {}})
	for row := range h {
		for col := range w {
			a.screen.SetContent(col, row, ' ', nil, a.th.primary)
		}
	}
	line := func(text string, style tcell.Style) {
		a.put(x0, y, maxX, text, style)
		y++
	}
	// A row is clicked like it's pressed: that moves to it, and changes it.
	option := func(row int, text string) {
		style := a.th.primary
		if o.row == row {
			style = a.th.active
		}
		a.hits = append(a.hits, hit{area: rect{x0, y, width, 1}, click: func(int, int, tcell.ModMask) {
			o.row = row
			a.changeOnboardRow()
		}})
		line(text, style)
	}

	line("Welcome to LazyCode", a.th.accent)
	y++
	option(rowTheme, "Theme      "+either(o.light, "Dark   [Light]", "[Dark]  Light"))
	option(rowAutoSave, "Auto-save  "+either(o.autoSave, "Off    [On]", "[Off]   On"))
	y++
	line("Language tools", a.th.primary.Bold(true))
	for i, t := range o.tools {
		box := either(t.picked || t.status == "installed", "[x]", "[ ]")
		option(rowTools+i, fmt.Sprintf("%s %-15s %-27s %s", box, t.Name, t.Server.Name, t.status))
	}
	y++
	for _, note := range notes {
		line(note, a.th.muted)
	}
	skip := "[Esc] Skip"
	done := "[Enter] Continue"
	a.hits = append(a.hits,
		hit{area: rect{x0, y, len(skip), 1}, click: func(int, int, tcell.ModMask) { a.finishOnboarding(&onboarding{}) }},
		hit{area: rect{x0 + len(skip) + 3, y, len(done), 1}, click: func(int, int, tcell.ModMask) { a.finishOnboarding(o) }})
	line(skip+"   "+done, a.th.accent)
}

func either(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}
