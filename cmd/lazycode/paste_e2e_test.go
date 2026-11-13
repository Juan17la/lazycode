package main

import (
	"fmt"
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

// A paste arrives as one key event per character, which must not cost one frame per character.
func TestBigPasteIsQuick(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ConPTY delivers pastes differently")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "lazycode") // Built first, as in TestPrototypeEndToEnd.
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	dir := filepath.Join(root, "work")
	mkdirs(t, dir)
	answerFirstQuestions(t)
	if err := os.WriteFile(filepath.Join(dir, "a.go"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tab, err := term.Start(dir, []string{bin}, 120, 30, func() {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tab.Close)
	d := &driver{t: t, tab: tab}
	d.wait("a.go")
	d.key(tcell.KeyEnter, 0)
	d.wait("Ln 1, Col 1")

	const lines = 2000
	var text strings.Builder
	for i := range lines {
		fmt.Fprintf(&text, "\tfunc line%04d() { return strings.Repeat(\"some text\", 2) }\n", i)
	}
	start := time.Now()
	tab.Paste(text.String())
	d.wait(fmt.Sprintf("Ln %d, Col 1", lines+1))
	took := time.Since(start)
	t.Logf("%d lines, %d bytes: %v", lines, text.Len(), took)
	if took > 15*time.Second {
		t.Fatalf("pasting took %v", took)
	}
	d.key(tcell.KeyCtrlS, tcell.ModCtrl)
	d.wait("Saved a.go")
	if got, _ := os.ReadFile(filepath.Join(dir, "a.go")); string(got) != text.String() {
		t.Fatalf("saved %d bytes, %d lines, want %d, %d", len(got), strings.Count(string(got), "\n"), text.Len(), lines)
	}

	// The same paste through a real shell in a terminal tab.
	d.alt('t')
	d.wait("Terminal")
	d.typeText(`echo re""ady; cat > out.txt`)
	d.enter()
	d.wait("ready")
	tab.Paste(text.String())
	d.key(tcell.KeyCtrlD, tcell.ModCtrl)
	out := filepath.Join(dir, "out.txt")
	deadline := time.Now().Add(15 * time.Second)
	for {
		got, _ := os.ReadFile(out)
		if string(got) == text.String() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the shell got %d bytes, want %d:\n%s", len(got), text.Len(), tab.Text())
		}
		time.Sleep(20 * time.Millisecond)
	}
}
