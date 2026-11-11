package main

import (
	"bufio"
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

// The test binary also plays the AI tool that runs inside LazyCode's terminal tab.
func TestMain(m *testing.M) {
	if file := os.Getenv("LAZYCODE_E2E_PIDFILE"); file != "" && os.Getenv("LAZYCODE_E2E_TOOL") != "" {
		os.WriteFile(file, fmt.Appendf(nil, "%d", os.Getpid()), 0o644) //nolint:errcheck // The test notices.
	}
	if os.Getenv("LAZYCODE_E2E_TOOL") == "stream" {
		streamLines()
	}
	if os.Getenv("LAZYCODE_E2E_TOOL") != "" {
		fmt.Println("tool ready")
		in := bufio.NewScanner(os.Stdin)
		for in.Scan() {
			fmt.Println("got: " + in.Text())
			if in.Text() == "bye" {
				os.Exit(7)
			}
		}
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// driver runs the real binary in a real pseudo-terminal, and reads its screen.
type driver struct {
	t   *testing.T
	tab *term.Tab
}

func (d *driver) wait(want string) {
	d.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(d.tab.Text(), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	d.t.Fatalf("screen never showed %q:\n%s", want, d.tab.Text())
}

func (d *driver) waitGone(text string) {
	d.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if !strings.Contains(d.tab.Text(), text) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	d.t.Fatalf("screen still shows %q:\n%s", text, d.tab.Text())
}

func (d *driver) key(k tcell.Key, mods tcell.ModMask) {
	d.tab.Key(tcell.NewEventKey(k, 0, mods))
}

func (d *driver) alt(r rune) {
	d.tab.Key(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModAlt))
}

func (d *driver) typeText(text string) {
	for _, r := range text {
		d.tab.Key(tcell.NewEventKey(tcell.KeyRune, r, 0))
		time.Sleep(time.Millisecond) // Keep ordering simple on slow CI consoles.
	}
}

func (d *driver) enter() { d.key(tcell.KeyEnter, 0) }

// answerFirstQuestions saves what onboarding would, for tests about other things.
func answerFirstQuestions(t *testing.T) {
	t.Helper()
	config, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	mkdirs(t, filepath.Join(config, "lazycode"))
	if err := os.WriteFile(filepath.Join(config, "lazycode", "config.toml"), []byte("version = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkdirs(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPrototypeEndToEnd(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Build first: the HOME below would make Go download modules into the temp folder.
	bin := filepath.Join(root, "lazycode")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	// Keep LazyCode's config inside the test, on every OS.
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("AppData", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("LocalAppData", filepath.Join(root, "cache"))

	one, two, three := filepath.Join(root, "one"), filepath.Join(root, "two"), filepath.Join(root, "three")
	mkdirs(t, filepath.Join(one, "sub"), two, three)
	answerFirstQuestions(t)
	if err := os.WriteFile(filepath.Join(one, "a.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	mkdirs(t, filepath.Join(config, "lazycode"))
	toml := fmt.Sprintf(`
[[project]]
name = "alpha"
[[project.directory]]
path = %q
[[project.directory]]
path = %q

[[project]]
name = "beta"
[[project.directory]]
path = %q
`, one, two, three)
	if err := os.WriteFile(filepath.Join(config, "lazycode", "projects.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	tool, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAZYCODE_E2E_TOOL", "1") // Inherited by the tool, through LazyCode.

	tab, err := term.Start(one, []string{bin}, 120, 30, func() {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tab.Close)
	d := &driver{t: t, tab: tab}

	// It opens on the project and directory it was started in, with the tree.
	d.wait("alpha › one")
	d.wait("sub")
	d.wait("a.txt")

	// Folders open in the tree. "sub" is first, because folders sort first.
	d.key(tcell.KeyRight, 0)
	d.wait("▾ sub")
	d.key(tcell.KeyLeft, 0)
	d.wait("▸ sub")

	// A file opens from the tree, takes typing, and saves. Unicode goes through the
	// real terminal on every OS.
	d.key(tcell.KeyDown, 0)
	d.enter()
	d.wait("Ln 1, Col 1")
	d.typeText("héllo 世界")
	d.wait("héllo 世界")
	d.wait("a.txt*")
	d.key(tcell.KeyCtrlS, tcell.ModCtrl)
	d.wait("Saved a.txt")
	if got, err := os.ReadFile(filepath.Join(one, "a.txt")); err != nil || string(got) != "héllo 世界" {
		t.Fatalf("saved %q, %v", got, err)
	}
	// ConPTY handles mouse input differently, so this one is for real terminals elsewhere.
	if runtime.GOOS != "windows" {
		d.key(tcell.KeyHome, 0)
		d.wait("Ln 1, Col 1")
		// 35 columns of tree and divider, then 3 of line numbers, so this is column 2.
		d.tab.Mouse(40, 2, false, false, true)
		d.tab.Mouse(40, 2, false, false, false)
		d.wait("Ln 1, Col 3")
	}
	d.key(tcell.KeyCtrlBackslash, tcell.ModCtrl)
	d.wait("Tab go to editor")

	// Alt+D switches directory.
	d.alt('d')
	d.wait("Switch folder")
	d.typeText("two")
	d.enter()
	d.wait("alpha › two")

	// Ctrl+K then P is the macOS form of Alt+P (Q-80), and switches project.
	d.key(tcell.KeyCtrlK, tcell.ModCtrl)
	d.wait("Ctrl+K: press")
	d.typeText("p")
	d.wait("Switch project")
	d.typeText("beta")
	d.enter()
	d.wait("beta › three")

	// Escape cancels a switcher, and a filter with no match says so.
	d.alt('p')
	d.typeText("nothing-matches")
	d.wait("No matches")
	d.key(tcell.KeyEsc, 0)
	d.waitGone("No matches")

	// A missing tool is a message, not a crash.
	d.typeText(":term lazycode-no-such-tool")
	d.enter()
	d.wait("lazycode-no-such-tool isn't installed")

	// A real child process runs in a terminal tab, and receives typed keys.
	d.typeText(":term " + tool)
	d.enter()
	d.wait("tool ready")
	d.typeText("hello")
	d.enter()
	d.wait("got: hello")

	// Ctrl+\ leaves the tab. The same keys now reach LazyCode, not the tool.
	d.wait(`Ctrl+Space leaves`)
	d.key(tcell.KeyCtrlBackslash, tcell.ModCtrl)
	d.wait("Tab go to editor")
	d.typeText(":")
	d.typeText("nope")
	d.enter()
	d.wait("Unknown command: nope")

	// Tabs belong to their directory, and keep running while hidden.
	d.alt('p')
	d.typeText("alpha")
	d.enter()
	d.wait("alpha › two")
	d.waitGone("got: hello")
	d.alt('p')
	d.typeText("beta")
	d.enter()
	d.wait("got: hello")

	// Alt+. focuses the tab again. The tool exits, and the tab stays with its code.
	d.alt('.')
	d.wait(`Ctrl+Space leaves`)
	d.typeText("bye")
	d.enter()
	d.wait("[exit 7]")

	// :close removes the finished tab.
	d.key(tcell.KeyCtrlBackslash, tcell.ModCtrl)
	d.wait("Tab go to editor")
	d.typeText(":close")
	d.enter()
	d.wait("No tabs here.")

	// A tab still running at quit is stopped with the app. :q exits cleanly.
	d.typeText(":term " + tool)
	d.enter()
	d.wait("tool ready")
	d.key(tcell.KeyCtrlBackslash, tcell.ModCtrl)
	d.wait("Tab go to editor")
	d.typeText(":q")
	d.enter()
	select {
	case <-tab.Done():
	case <-time.After(30 * time.Second):
		t.Fatalf("LazyCode didn't exit:\n%s", tab.Text())
	}
	if _, code := tab.Exited(); code != 0 {
		t.Fatalf("exit code %d:\n%s", code, tab.Text())
	}
}
