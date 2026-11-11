package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Juan17la/lazycode/internal/term"
	"github.com/gdamore/tcell/v2"
)

// press sends one listed key through a real PTY or ConPTY. That proves the
// decoding, on each OS. A real terminal can still take keys for itself, so it
// doesn't replace running keytest by hand.
func press(tab *term.Tab, label string) {
	keys := map[string]*tcell.EventKey{
		`Ctrl+\`:     tcell.NewEventKey(tcell.KeyCtrlBackslash, 0, tcell.ModCtrl),
		"Ctrl+Space": tcell.NewEventKey(tcell.KeyCtrlSpace, 0, tcell.ModCtrl),
		"F1":         tcell.NewEventKey(tcell.KeyF1, 0, 0),
		"F2":         tcell.NewEventKey(tcell.KeyF2, 0, 0),
		"F3":         tcell.NewEventKey(tcell.KeyF3, 0, 0),
		"F10":        tcell.NewEventKey(tcell.KeyF10, 0, 0),
		"F12":        tcell.NewEventKey(tcell.KeyF12, 0, 0),
		"Esc":        tcell.NewEventKey(tcell.KeyEsc, 0, 0),
		"Ctrl+F2":    tcell.NewEventKey(tcell.KeyF2, 0, tcell.ModCtrl),
	}
	// A capital letter with Alt held, which is how a terminal sends Shift+Alt+letter.
	if letter, ok := strings.CutPrefix(label, "Shift+Alt+"); ok {
		tab.Key(tcell.NewEventKey(tcell.KeyRune, []rune(letter)[0], tcell.ModAlt))
		return
	}
	if ev, ok := keys[label]; ok {
		tab.Key(ev)
		return
	}
	name, key, _ := strings.Cut(label, "+")
	r := []rune(strings.ToLower(key))[0]
	if name == "Alt" {
		tab.Key(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModAlt))
		return
	}
	tab.Key(tcell.NewEventKey(tcell.KeyCtrlA+tcell.Key(r-'a'), 0, tcell.ModCtrl))
}

func TestEveryKeyDecodesThroughAPseudoTerminal(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "keytest")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	report := filepath.Join(dir, "report.txt")
	tab, err := term.Start(dir, []string{bin, "--report", report}, 100, 30, func() {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tab.Close)
	deadline := time.Now().Add(30 * time.Second)
	for !strings.Contains(tab.Text(), "Press each key") {
		if time.Now().After(deadline) {
			t.Fatalf("keytest never started:\n%s", tab.Text())
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, c := range checks(runtime.GOOS) {
		press(tab, c.label)
		time.Sleep(30 * time.Millisecond) // Esc must not join the next key as Alt.
		if c.label == "Esc" {
			time.Sleep(200 * time.Millisecond)
		}
	}
	time.Sleep(300 * time.Millisecond)
	tab.Key(tcell.NewEventKey(tcell.KeyRune, 'q', 0))
	select {
	case <-tab.Done():
	case <-time.After(30 * time.Second):
		t.Fatalf("keytest didn't exit:\n%s", tab.Text())
	}
	data, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("report:\n%s", data)
	if strings.Contains(string(data), "MISSING") {
		t.Errorf("keys didn't arrive:\n%s", data)
	}
}
