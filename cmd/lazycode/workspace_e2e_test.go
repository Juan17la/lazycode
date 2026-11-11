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

// The real binary in a real terminal: making a project, the tree noticing a new file, and
// everything coming back in a second run.
func TestWorkspaceSurvivesARestart(t *testing.T) {
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

	one, two := filepath.Join(root, "one"), filepath.Join(root, "two")
	mkdirs(t, one, two)
	answerFirstQuestions(t, one, two)
	if err := os.WriteFile(filepath.Join(one, "a.txt"), []byte("first\nsecond\nthird\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	launch := func() *driver {
		tab, err := term.Start(one, []string{bin}, 120, 30, func() {})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tab.Close)
		return &driver{t: t, tab: tab}
	}
	command := func(d *driver, text string) {
		d.typeText(":" + text)
		d.enter()
	}

	d := launch()
	d.wait("Current folder › one")
	command(d, "project new Demo")
	d.wait("Demo › one")
	command(d, "dir add ../two")
	d.wait("Demo › two")
	if err := os.WriteFile(filepath.Join(two, "live.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	d.wait("live.txt") // Nothing pressed: the watcher saw it.

	// Back to the first directory, with a file open and the cursor moved.
	command(d, "dir")
	d.wait("Switch folder")
	d.typeText("one")
	d.enter()
	d.wait("Demo › one")
	d.enter()
	d.wait("Ln 1, Col 1")
	d.key(tcell.KeyDown, 0)
	d.wait("Ln 2, Col 1")
	d.key(tcell.KeyCtrlBackslash, tcell.ModCtrl)
	d.wait("Tab go to editor")
	command(d, "q")
	deadline := time.Now().Add(30 * time.Second)
	for done, _ := d.tab.Exited(); !done; done, _ = d.tab.Exited() {
		if time.Now().After(deadline) {
			t.Fatalf("didn't quit:\n%s", d.tab.Text())
		}
		time.Sleep(20 * time.Millisecond)
	}

	configDir, err := os.UserConfigDir() // macOS ignores XDG_CONFIG_HOME.
	if err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(filepath.Join(configDir, "lazycode", "projects.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), `name = "Demo"`) || !strings.Contains(string(config), `"~/two"`) {
		t.Errorf("projects.toml:\n%s", config)
	}

	again := launch()
	again.wait("Demo › one")
	again.wait(" a.txt ")
	again.wait("second") // Its text, in the restored tab.
	again.key(tcell.KeyTab, 0)
	again.wait("Ln 2, Col 1")
}

func TestFirstRunInARealTerminal(t *testing.T) {
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

	one, two := filepath.Join(root, "one"), filepath.Join(root, "two")
	mkdirs(t, one, two)
	tab, err := term.Start(one, []string{bin}, 120, 30, func() {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tab.Close)
	d := &driver{t: t, tab: tab}

	d.wait("Welcome to LazyCode")
	d.wait("Language tools")
	d.typeText(" ")
	d.wait("[Light]")
	d.enter()
	d.wait("Trust one?")
	d.typeText("t")
	d.wait("one is trusted.")

	// A directory added later gets its own question (UC-09).
	d.typeText(":project new Demo")
	d.enter()
	d.wait("Demo › one")
	d.typeText(":dir add ../two")
	d.enter()
	d.wait("Trust two?")
	d.typeText("n")
	d.wait("Not trusted.")
}
