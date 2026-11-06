package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Juan17la/lazycode/internal/config"
	"github.com/gdamore/tcell/v2"
)

// firstRun starts LazyCode as if for the first time.
func firstRun(t *testing.T) *ui {
	t.Helper()
	askFirstQuestions, launchShows = true, "Welcome to LazyCode"
	// Runs after the app has stopped, because this cleanup was registered before it started.
	t.Cleanup(func() { askFirstQuestions, launchShows = false, "Files" })
	return start(t, nil)
}

func TestFirstRunSavesChoicesAndDoesntComeBack(t *testing.T) {
	u := firstRun(t)
	u.press(" ")
	u.wait("[Light]")
	u.key(tcell.KeyDown)
	u.press(" ")
	u.wait("[On]")
	u.wait("new version once a day") // Q-79: disclosed before it happens.
	u.key(tcell.KeyEnter)
	u.waitGone("Welcome to LazyCode")

	u.wait("Trust work?")
	u.press("t")
	u.wait("work is trusted.")
	u.quitNow()

	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if c.UI.Theme != "light" || !c.Editor.AutoSave {
		t.Fatalf("saved theme %q, auto-save %v", c.UI.Theme, c.Editor.AutoSave)
	}

	launchShows = "Files"
	again := launch(t, u.dir)
	if text := again.text(); strings.Contains(text, "Welcome") || strings.Contains(text, "Trust work?") {
		t.Fatalf("asked again:\n%s", text)
	}
}

func TestSkippingSavesDefaultsAndNoStillAsksBeforeRunning(t *testing.T) {
	u := firstRun(t)
	u.press(" ") // A choice made and then skipped doesn't count.
	u.wait("[Light]")
	u.key(tcell.KeyEsc)
	u.wait("Trust work?")
	u.key(tcell.KeyEsc) // The safe answer.
	u.wait("Not trusted.")
	if got := readConfig(t, "config.toml"); got != config.Template {
		t.Fatalf("skipping should save the defaults: %q", got)
	}

	u.run("play true")
	u.wait("Run true in work?")
	u.press("c")
	u.quitNow()

	launchShows = "Files"
	again := launch(t, u.dir)
	if text := again.text(); strings.Contains(text, "Trust work?") {
		t.Fatalf("no wasn't remembered:\n%s", text)
	}
}

func TestContinueAndARowCanBeClicked(t *testing.T) {
	u := firstRun(t)
	u.clickOn("Theme")
	u.wait("[Light]")
	u.clickOn("Continue")
	u.wait("Trust work?")
	if got := readConfig(t, "config.toml"); !strings.Contains(got, `theme = "light"`) {
		t.Fatalf("config: %q", got)
	}
}

// fakeInstaller makes the PATH hold only sh and an npm that records where and how it ran.
func fakeInstaller(t *testing.T) (log string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as npm")
	}
	bin := t.TempDir()
	log = filepath.Join(bin, "npm.log")
	script := "#!/bin/sh\necho \"$PWD $*\" > " + log + "\n"
	if err := os.WriteFile(filepath.Join(bin, "npm"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sh, filepath.Join(bin, "sh")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return log
}

func TestInstallShowsCommandAndRunsOutsideTheProjectAfterConfirming(t *testing.T) {
	log := fakeInstaller(t)
	u := firstRun(t)
	u.wait("needs go")
	u.wait("missing")

	u.key(tcell.KeyDown)
	u.key(tcell.KeyDown)
	u.wait("go isn't installed") // Go's row says what's in the way, and can't be picked.
	u.press(" ")
	u.wait("[ ] Go")

	u.key(tcell.KeyDown)
	u.wait("npm install -g pyright") // Q-68: the exact command, as soon as it's on the row.
	u.press(" ")
	u.wait("[x] Python")
	u.key(tcell.KeyEnter)

	u.wait("by running: npm install -g pyright")
	if _, err := os.Stat(log); err == nil {
		t.Fatal("ran before confirming")
	}
	u.press("r")

	deadline := time.Now().Add(10 * time.Second)
	var got []byte
	for time.Now().Before(deadline) && len(got) == 0 {
		got, _ = os.ReadFile(log)
		time.Sleep(5 * time.Millisecond)
	}
	if want := filepath.Dir(u.dir) + " install -g pyright\n"; string(got) != want {
		t.Fatalf("ran as %q, want %q", got, want)
	}
	u.wait("Trust work?") // The install didn't use up the folder's question.
}

func TestDecliningAnInstallRunsNothing(t *testing.T) {
	log := fakeInstaller(t)
	u := firstRun(t)
	u.key(tcell.KeyDown)
	u.key(tcell.KeyDown)
	u.key(tcell.KeyDown)
	u.press(" ")
	u.wait("[x] Python")
	u.key(tcell.KeyEnter)
	u.wait("by running: npm install -g pyright")
	u.key(tcell.KeyEsc)
	u.wait("Trust work?")
	if _, err := os.Stat(log); err == nil {
		t.Fatal("ran after declining")
	}
}

func TestOnlyMacOSExplainsCtrlK(t *testing.T) {
	has := func(goos string) bool { return strings.Contains(strings.Join(onboardNotes(goos), " "), "Ctrl+K") }
	if !has("darwin") || has("linux") || has("windows") {
		t.Fatal("wrong note for an OS")
	}
}
