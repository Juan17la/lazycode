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

// The real binary in a real terminal: a Go file comes up coloured, and a text file doesn't.
func TestGoFileIsColouredInARealTerminal(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "lazycode")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("AppData", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("LocalAppData", filepath.Join(root, "cache"))
	t.Setenv("TERM", "xterm-256color") // The test's own may be a terminal without colour.
	dir := filepath.Join(root, "work")
	mkdirs(t, dir)
	answerFirstQuestions(t)
	src := "package main\n\n// note\nfunc main() {\n\tprintln(\"hi\", 42)\n}\n"
	for name, text := range map[string]string{"a.go": src, "b.txt": src} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tab, err := term.Start(dir, []string{bin}, 100, 20, func() {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tab.Close)
	d := &driver{t: t, tab: tab}
	d.wait("a.go")
	d.enter() // The first file in the tree.

	// styleOf draws the terminal's screen and reports how the first match of text looks.
	styleOf := func(text string) (fg tcell.Color, attrs tcell.AttrMask) {
		screen := tcell.NewSimulationScreen("UTF-8")
		if err := screen.Init(); err != nil {
			t.Fatal(err)
		}
		defer screen.Fini()
		screen.SetSize(100, 20)
		tab.Draw(screen, 0, 0, 100, 20)
		screen.Show()
		cells, w, _ := screen.GetContents()
		for y, line := range strings.Split(tab.Text(), "\n") {
			if x := strings.Index(line, text); x >= 0 {
				col := len([]rune(line[:x]))
				fg, _, attrs = cells[y*w+col].Style.Decompose()
				return fg, attrs
			}
		}
		t.Fatalf("%q isn't on screen:\n%s", text, tab.Text())
		return
	}
	d.wait("println")
	deadline := time.Now().Add(10 * time.Second)
	for {
		fg, attrs := styleOf("package")
		if fg != tcell.ColorDefault && attrs&tcell.AttrBold != 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("package never became a coloured keyword: fg=%v attrs=%v\n%s", fg, attrs, tab.Text())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, attrs := styleOf("// note"); attrs&tcell.AttrItalic == 0 {
		t.Errorf("the comment isn't italic: %v", attrs)
	}
	if fg, _ := styleOf("42"); fg == tcell.ColorDefault {
		t.Error("the number has no colour")
	}
	d.wait("Go  Ln") // The status bar names the language.
}
