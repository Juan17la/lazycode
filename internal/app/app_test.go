package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Juan17la/lazycode/internal/buffer"
	"github.com/Juan17la/lazycode/internal/config"
	"github.com/Juan17la/lazycode/internal/editor"
	"github.com/Juan17la/lazycode/internal/keymap"
	"github.com/Juan17la/lazycode/internal/project"
	"github.com/gdamore/tcell/v2"
)

// The test binary also plays a tool that exits as soon as it starts, and a LazyCode
// that dies with unsaved work.
func TestMain(m *testing.M) {
	askFirstQuestions = false
	if mode := os.Getenv("LAZYCODE_TEST_TOOL"); mode != "" {
		runFakeTool(mode)
	}
	if cfg := os.Getenv("LAZYCODE_TEST_LSP"); cfg != "" {
		runFakeLSP(cfg)
	}
	if path := os.Getenv("LAZYCODE_TEST_CRASH_PATH"); path != "" {
		r := editor.NewRecovery(os.Getenv("LAZYCODE_TEST_CRASH_DIR"), nil)
		r.Schedule(path, buffer.New([]byte(os.Getenv("LAZYCODE_TEST_CRASH_TEXT"))).Snapshot)
		r.Wait()
		os.Exit(2)
	}
	if code := os.Getenv("LAZYCODE_TEST_EXIT"); code != "" {
		var n int
		fmt.Sscan(code, &n) //nolint:errcheck // Tests pass a number.
		fmt.Println("tool ran")
		if os.Getenv("LAZYCODE_TEST_STAY") != "" {
			time.Sleep(time.Minute)
		}
		os.Exit(n)
	}
	os.Exit(m.Run())
}

// leaveRecovery runs a process that writes a recovery file, then dies.
func leaveRecovery(t *testing.T, dir, path, text string) {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "LAZYCODE_TEST_CRASH_PATH="+path, "LAZYCODE_TEST_CRASH_DIR="+dir, "LAZYCODE_TEST_CRASH_TEXT="+text)
	var exit *exec.ExitError
	if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != 2 {
		t.Fatalf("the crashing process: %v", err)
	}
}

// recordingScreen keeps a copy of each frame. The simulation's own GetContents returns
// its live cells, which can't be read while the UI goroutine draws.
type recordingScreen struct {
	tcell.SimulationScreen
	mu    sync.Mutex
	frame string
}

func (s *recordingScreen) Show() {
	s.SimulationScreen.Show()
	cells, w, h := s.GetContents()
	var b strings.Builder
	for y := range h {
		for x := range w {
			if c := cells[y*w+x]; len(c.Runes) > 0 {
				b.WriteString(string(c.Runes))
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteByte('\n')
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frame = b.String()
}

// ui runs the real UI loop on a simulated screen.
type ui struct {
	t      *testing.T
	screen *recordingScreen
	dir    string
	exited chan error
}

// launchShows is what launch waits for. Onboarding tests change it.
var launchShows = "Files"

func start(t *testing.T, files map[string]string) *ui {
	return startTweaked(t, files, func(*app) {})
}

func startTweaked(t *testing.T, files map[string]string, tweak func(*app)) *ui {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Keep recovery files inside the test, on every OS.
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("AppData", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("LocalAppData", filepath.Join(root, "cache"))
	// No clipboard tool, so tests can't touch the real one.
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")

	dir := filepath.Join(root, "work")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return launchTweaked(t, dir, tweak)
}

func launch(t *testing.T, dir string) *ui {
	return launchTweaked(t, dir, func(*app) {})
}

// launchTweaked starts the app after tweak has changed it, such as to fake the updater.
func launchTweaked(t *testing.T, dir string, tweak func(*app)) *ui {
	t.Helper()
	u := &ui{t: t, screen: &recordingScreen{SimulationScreen: tcell.NewSimulationScreen("UTF-8")}, dir: dir, exited: make(chan error, 1)}
	projects := []project.Project{{Name: "test", Dirs: []string{dir}}}
	go func() { u.exited <- runApp(u.screen, projects, dir, "", tweak) }()
	u.wait(launchShows)
	t.Cleanup(func() {
		select {
		case <-u.exited:
		default:
			// Stop it, answering any question by not saving. Onboarding can have a question behind it.
			u.post(tcell.NewEventKey(tcell.KeyEsc, 0, 0))
			u.post(tcell.NewEventKey(tcell.KeyEsc, 0, 0))
			u.post(tcell.NewEventKey(tcell.KeyCtrlBackslash, 0, 0))
			u.post(tcell.NewEventKey(tcell.KeyRune, 'e', tcell.ModAlt)) // Search and Git would take : as text.
			u.run("q")
			for range 200 {
				select {
				case <-u.exited:
					return
				case <-time.After(50 * time.Millisecond):
					u.press("qd") // Quit despite busy sessions, and don't save.
				}
			}
			// Still running, it would write into the folders being deleted next.
			t.Errorf("didn't stop:\n%s", u.text())
		}
	})
	return u
}

func (u *ui) text() string {
	u.screen.mu.Lock()
	defer u.screen.mu.Unlock()
	return u.screen.frame
}

func (u *ui) wait(want string) {
	u.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(u.text(), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	u.t.Fatalf("screen never showed %q:\n%s", want, u.text())
}

func (u *ui) waitCount(text string, want int) {
	u.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Count(u.text(), text) == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	u.t.Fatalf("screen doesn't show %q %d times:\n%s", text, want, u.text())
}

func (u *ui) waitGone(text string) {
	u.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !strings.Contains(u.text(), text) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	u.t.Fatalf("screen still shows %q:\n%s", text, u.text())
}

// post retries, because PostEvent drops the event when the queue is full.
func (u *ui) post(ev tcell.Event) {
	for range 2000 { // Gives up once the app has stopped reading.
		if u.screen.PostEvent(ev) == nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

func (u *ui) key(k tcell.Key) { u.post(tcell.NewEventKey(k, 0, 0)) }

func (u *ui) mod(k tcell.Key, mods tcell.ModMask) { u.post(tcell.NewEventKey(k, 0, mods)) }

func (u *ui) press(text string) {
	for _, r := range text {
		u.post(tcell.NewEventKey(tcell.KeyRune, r, 0))
	}
}

// run types a command into the command bar, from outside a tab.
func (u *ui) run(command string) {
	u.press(":" + command)
	u.key(tcell.KeyEnter)
}

// open selects a file in the tree by name, and opens it.
func (u *ui) open(name string) {
	u.t.Helper()
	rows, err := os.ReadDir(u.dir)
	if err != nil {
		u.t.Fatal(err)
	}
	u.key(tcell.KeyCtrlBackslash)
	for range rows {
		u.key(tcell.KeyUp)
	}
	for _, row := range rows {
		if row.Name() == name {
			break
		}
		u.key(tcell.KeyDown)
	}
	u.key(tcell.KeyEnter)
	u.wait(" " + name + " ")
}

func (u *ui) quit() {
	u.t.Helper()
	select {
	case err := <-u.exited:
		if err != nil {
			u.t.Fatal(err)
		}
		u.exited <- nil
	case <-time.After(10 * time.Second):
		u.t.Fatalf("didn't quit:\n%s", u.text())
	}
}

func (u *ui) file(name string) string {
	u.t.Helper()
	data, err := os.ReadFile(filepath.Join(u.dir, name))
	if err != nil {
		u.t.Fatal(err)
	}
	return string(data)
}

func (u *ui) waitFile(name, want string) {
	u.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(filepath.Join(u.dir, name)); err == nil && string(data) == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	u.t.Fatalf("%s holds %q, want %q", name, u.file(name), want)
}

// The reported bug: once a tool exited, keys kept going to the dead tab, so : did nothing.
func TestToolExitingGivesTheKeysBack(t *testing.T) {
	u := start(t, nil)
	tool, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAZYCODE_TEST_EXIT", "3")
	u.run("term " + tool)
	u.wait("[exit 3]")
	u.wait("exited with code 3")

	u.press(":nope") // No Ctrl+\ first.
	u.key(tcell.KeyEnter)
	u.wait("Unknown command: nope")

	u.mod(tcell.KeyTab, 0) // Entering the finished tab just comes back out.
	u.wait("exited with code 3")
	u.press(":nope")
	u.key(tcell.KeyEnter)
	u.wait("Unknown command: nope")

	u.key(tcell.KeyCtrlW)
	u.wait("No tabs here.")
}

func TestOpenEditSave(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "hello\nworld\n"})
	u.key(tcell.KeyEnter)
	u.wait("hello")
	u.wait("Ln 1, Col 1")
	u.press("say ")
	u.wait("say hello")
	u.wait("a.txt*")
	u.wait("Ln 1, Col 5")
	if got := u.file("a.txt"); got != "hello\nworld\n" {
		t.Fatalf("written before saving: %q", got)
	}

	u.key(tcell.KeyCtrlS)
	u.wait("Saved a.txt")
	u.waitGone("a.txt*")
	if got := u.file("a.txt"); got != "say hello\nworld\n" {
		t.Fatalf("saved %q", got)
	}

	// Undo goes back, and marks it unsaved again.
	u.key(tcell.KeyCtrlZ)
	u.wait("a.txt*")
	u.key(tcell.KeyCtrlY)
	u.waitGone("a.txt*")

	u.key(tcell.KeyCtrlBackslash)
	u.run("q")
	u.quit()
}

func TestCommandBarFromAFile(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "text"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")

	u.press(":") // Plain : is just typing.
	u.wait(":text")
	u.key(tcell.KeyCtrlZ)

	u.key(tcell.KeyEsc) // Esc then : opens the bar (Q-22).
	u.wait("Press : for commands")
	u.press(":w")
	u.key(tcell.KeyEnter)
	if got := u.file("a.txt"); got != "text" {
		t.Fatalf("got %q", got)
	}

	u.key(tcell.KeyEsc)
	u.press("x") // Any other key after Esc types as normal.
	u.wait("xtext")
}

func TestBinaryAndMissingFilesGiveAMessage(t *testing.T) {
	u := start(t, map[string]string{"bin.dat": "a\x00b"})
	u.key(tcell.KeyEnter)
	u.wait("bin.dat is a binary file")
	u.wait("No tabs here.")
}

func TestQuitAsksAboutUnsavedWork(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one", "b.txt": "two"})
	u.open("a.txt")
	u.press("x")
	u.wait("a.txt*")
	u.open("b.txt")
	u.press("y")
	u.wait("b.txt*")

	u.key(tcell.KeyCtrlBackslash)
	u.waitGone("Ctrl+S save")
	u.run("q")
	u.wait("before quitting?")

	// Keys go to the question, and other keys do nothing.
	u.press("zzz")
	u.key(tcell.KeyDown)
	u.wait("before quitting?")

	// Cancelling loses nothing.
	u.key(tcell.KeyEsc)
	u.waitGone("before quitting?")
	u.wait("a.txt*")

	// Save one and don't save the other, in whichever order they're asked.
	u.run("q")
	u.wait("before quitting?")
	u.press("d")
	u.wait("before quitting?")
	u.press("s")
	u.quit()
	if a, b := u.file("a.txt"), u.file("b.txt"); !(a == "xone" && b == "two") && !(a == "one" && b == "ytwo") {
		t.Fatalf("a %q, b %q", a, b)
	}
}

func TestClosingAskFlows(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.press("x")
	u.wait("a.txt*")

	u.key(tcell.KeyCtrlW)
	u.wait("Save changes to a.txt?")
	u.press("c")
	u.waitGone("Save changes to a.txt?")
	u.wait("xone")

	u.key(tcell.KeyCtrlW)
	u.wait("Save changes to a.txt?")
	u.press("s")
	u.wait("No tabs here.")
	if got := u.file("a.txt"); got != "xone" {
		t.Fatalf("got %q", got)
	}

	u.key(tcell.KeyEnter)
	u.wait("xone")
	u.press("y")
	u.wait("a.txt*")
	u.key(tcell.KeyCtrlW)
	u.wait("Save changes to a.txt?")
	u.press("d")
	u.wait("No tabs here.")
	if got := u.file("a.txt"); got != "xone" {
		t.Fatalf("got %q", got)
	}
}

func writeDisk(t *testing.T, u *ui, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(u.dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSavingOverAToolsChangeAsks(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.press("x")
	u.wait("xone")
	writeDisk(t, u, "a.txt", "tool")

	u.key(tcell.KeyCtrlS)
	u.wait("a.txt changed on disk.")
	if got := u.file("a.txt"); got != "tool" {
		t.Fatalf("overwritten: %q", got)
	}
	u.press("c")
	u.wait("xone") // Still there.

	u.key(tcell.KeyCtrlS)
	u.wait("a.txt changed on disk.")
	u.press("r")
	u.wait("Reloaded a.txt")
	u.wait("tool")

	u.press("y")
	writeDisk(t, u, "a.txt", "again")
	u.key(tcell.KeyCtrlS)
	u.wait("a.txt changed on disk.")
	u.press("o")
	u.wait("Saved a.txt")
	if got := u.file("a.txt"); got != "tyool" { // Reloading keeps the cursor's place.
		t.Fatalf("got %q", got)
	}
}

func TestReturningToAFileNoticesChanges(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.key(tcell.KeyCtrlBackslash)

	// A clean file just follows the disk.
	writeDisk(t, u, "a.txt", "changed by a tool")
	u.key(tcell.KeyTab)
	u.wait("changed on disk, and was reloaded")
	u.wait("changed by a tool")

	// Edits are never replaced.
	u.press("mine ")
	u.wait("mine changed")
	u.key(tcell.KeyCtrlBackslash)
	writeDisk(t, u, "a.txt", "tool again")
	u.key(tcell.KeyTab)
	u.wait("Your edits are kept")
	u.wait("mine changed")
}

func TestPasting(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "a\r\nb"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.post(tcell.NewEventPaste(true))
	u.press("1")
	u.key(tcell.KeyEnter) // Pasted line breaks arrive as Enter.
	u.press("2")
	u.post(tcell.NewEventPaste(false))
	u.wait("a.txt*")
	u.key(tcell.KeyCtrlS)
	u.wait("Saved a.txt")
	if got := u.file("a.txt"); got != "1\r\n2a\r\nb" {
		t.Fatalf("got %q", got)
	}
}

func TestCopyAndPasteWithoutAClipboardTool(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "word\n"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.key(tcell.KeyCtrlC) // Copies the line.
	u.key(tcell.KeyCtrlV)
	u.wait("Ln 2, Col 1")
	u.wait("a.txt*")
	u.key(tcell.KeyCtrlS)
	u.wait("Saved")
	if got := u.file("a.txt"); got != "word\nword\n" {
		t.Fatalf("got %q", got)
	}
}

func TestSelectionAndWideTextDraw(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "世界 ok\n\x1b[31mred\n"})
	u.key(tcell.KeyEnter)
	u.wait("世")
	u.wait("?[31mred") // The escape code never reaches the terminal.
	u.mod(tcell.KeyRight, tcell.ModShift)
	u.wait("Ln 1, Col 3") // A wide character moves two columns.
}

func TestScrollingLongFiles(t *testing.T) {
	var text strings.Builder
	for i := range 200 {
		fmt.Fprintf(&text, "line number %d\n", i)
	}
	u := start(t, map[string]string{"a.txt": text.String()})
	u.key(tcell.KeyEnter)
	u.wait("line number 0")
	u.mod(tcell.KeyEnd, tcell.ModCtrl)
	u.wait("Ln 201, Col 1")
	u.wait("line number 199")
	u.waitGone("line number 0\n")
	u.mod(tcell.KeyHome, tcell.ModCtrl)
	u.wait("line number 0")
}

func TestSmallWindowsDontCrash(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "some text 世界\n"})
	u.key(tcell.KeyEnter)
	u.wait("some text")
	for _, size := range [][2]int{{10, 5}, {3, 3}, {1, 1}, {39, 4}, {80, 25}} {
		u.screen.SetSize(size[0], size[1])
		u.press("x")
	}
	u.wait("xxxxx")
}

// The whole story, through the real UI: type, crash, start again, restore.
func TestRecoveryThroughTheUI(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "saved\n"})
	dir, err := editor.RecoveryDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(u.dir, "a.txt")

	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.press("typed ")
	u.wait("typed saved")
	waitForFile(t, dir)

	// A running window's file isn't offered.
	second := launch(t, u.dir)
	if strings.Contains(second.text(), "Unsaved work") {
		t.Fatal("offered a running window's work")
	}
	second.run("q")
	second.quit()

	u.key(tcell.KeyCtrlS)
	u.wait("Saved a.txt")
	waitForEmpty(t, dir)

	// Saving and undoing back to the saved text leaves nothing to recover either.
	u.press("z")
	u.wait("a.txt*")
	u.key(tcell.KeyCtrlZ)
	u.waitGone("a.txt*")
	time.Sleep(1200 * time.Millisecond)
	waitForEmpty(t, dir)

	u.key(tcell.KeyCtrlBackslash)
	u.run("q")
	u.quit()

	// Left behind by a process that crashed.
	leaveRecovery(t, dir, path, "recovered text\n")
	writeDisk(t, u, "a.txt", "a tool's text\n")

	later := launch(t, u.dir)
	later.wait("Unsaved work from")
	later.wait("a.txt")
	later.press("r")
	later.wait("Restored a.txt")
	later.wait("recovered text")
	later.wait("a.txt*")
	if got := later.file("a.txt"); got != "a tool's text\n" {
		t.Fatalf("restoring wrote to the file: %q", got)
	}
	later.key(tcell.KeyCtrlZ)
	later.wait("a tool's text")
	later.key(tcell.KeyCtrlY)
	later.wait("recovered text")
	later.key(tcell.KeyCtrlS)
	later.wait("Saved a.txt")
	waitForEmpty(t, dir)
	later.waitFile("a.txt", "recovered text\n")
	later.key(tcell.KeyCtrlBackslash)
	later.run("q")
	later.quit()

	// Later keeps it for next time, and Delete removes it.
	leaveRecovery(t, dir, path, "again\n")
	next := launch(t, u.dir)
	next.wait("Unsaved work from")
	next.press("l")
	next.waitGone("Unsaved work from")
	next.run("q")
	next.quit()
	last := launch(t, u.dir)
	last.wait("Unsaved work from")
	last.press("d")
	last.waitGone("Unsaved work from")
	waitForEmpty(t, dir)
}

func TestRecoveryOfADeletedFile(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "saved\n"})
	dir, err := editor.RecoveryDir()
	if err != nil {
		t.Fatal(err)
	}
	u.run("q")
	u.quit()
	path := filepath.Join(u.dir, "a.txt")
	leaveRecovery(t, dir, path, "only copy\n")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	later := launch(t, u.dir)
	later.wait("Unsaved work from")
	later.press("r")
	later.wait("only copy")
	later.key(tcell.KeyCtrlS)
	later.wait("Saved a.txt")
	later.waitFile("a.txt", "only copy\n")
}

func waitForFile(t *testing.T, dir string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if entries, _ := os.ReadDir(dir); len(entries) == 1 && strings.HasSuffix(entries[0].Name(), ".recovery") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no recovery file appeared")
}

func waitForEmpty(t *testing.T, dir string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if entries, _ := os.ReadDir(dir); len(entries) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the recovery file wasn't removed")
}

// CORE-05: multiple cursors, through the keys.

func TestMultipleCursorsThroughKeys(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "foo bar foo\nfoo\n"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.key(tcell.KeyCtrlD) // Selects the word.
	u.wait("Ln 1, Col 4")
	u.key(tcell.KeyCtrlD) // Adds the next one.
	u.wait("2 cursors")
	u.key(tcell.KeyCtrlD)
	u.wait("3 cursors")
	u.key(tcell.KeyCtrlD)
	u.wait("No more to select.")
	u.press("x")
	u.wait("x bar x")
	u.key(tcell.KeyCtrlZ) // One step (UC-15).
	u.wait("foo bar foo")
	u.waitGone("a.txt*")
	u.wait("3 cursors") // The cursors came back too.

	u.key(tcell.KeyEsc) // Back to one.
	u.waitGone("3 cursors")
}

func TestSelectAllOccurrencesEachWay(t *testing.T) {
	check := func(name string, trigger func(u *ui)) {
		t.Run(name, func(t *testing.T) {
			u := start(t, map[string]string{"a.txt": "dog cat dog\n"})
			u.key(tcell.KeyEnter)
			u.wait("Ln 1, Col 1")
			trigger(u)
			u.wait("2 selected")
			u.press("z")
			u.wait("z cat z")
		})
	}
	check("Ctrl+F2", func(u *ui) { u.mod(tcell.KeyF2, tcell.ModCtrl) })
	check("Ctrl+K then A", func(u *ui) { // macOS (Q-80).
		u.key(tcell.KeyCtrlK)
		u.wait("Ctrl+K: press")
		u.press("a")
	})
	check(":selectall", func(u *ui) {
		u.key(tcell.KeyEsc)
		u.press(":selectall")
		u.key(tcell.KeyEnter)
	})
}

// CORE-06: find and replace.

func TestFindBar(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one two\none three\n"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.key(tcell.KeyCtrlF)
	u.wait("Find: ")
	u.wait("0 matches")

	u.press("one")
	u.wait("Find: one")
	u.wait("2 matches")
	u.key(tcell.KeyEnter) // Next match.
	u.key(tcell.KeyEsc)
	u.waitGone("Find:")
	u.wait("Ln 2, Col 4")
	u.press("X") // The selection stays, so typing replaces it.
	u.wait("X three")

	u.key(tcell.KeyCtrlF)
	u.press("nothing here")
	u.wait("0 matches")
	u.key(tcell.KeyBackspace2)
	u.key(tcell.KeyEsc)
	if got := u.file("a.txt"); got != "one two\none three\n" {
		t.Fatalf("finding changed the file: %q", got)
	}
}

func TestFindArrowsAndPaste(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "ab\nab\nab\n"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.key(tcell.KeyCtrlF)
	u.post(tcell.NewEventPaste(true))
	u.press("ab")
	u.key(tcell.KeyEnter) // A line break in a paste is dropped.
	u.post(tcell.NewEventPaste(false))
	u.wait("Find: ab")
	u.wait("3 matches")
	u.key(tcell.KeyDown)
	u.key(tcell.KeyDown)
	u.key(tcell.KeyUp)
	u.key(tcell.KeyEsc)
	u.wait("Ln 2, Col 3")
}

func TestReplaceBar(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "cat cat cat cat\n"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.post(tcell.NewEventKey(tcell.KeyRune, 'h', tcell.ModAlt))
	u.wait("Replace:")
	u.press("dog") // The replacement has focus first.
	u.key(tcell.KeyTab)
	u.press("cat")
	u.wait("4 matches")
	u.key(tcell.KeyTab)
	u.key(tcell.KeyEnter) // Replaces the selected match.
	u.wait("dog cat cat cat")
	u.wait("3 matches")
	u.key(tcell.KeyEnter)
	u.wait("dog dog cat cat")
	u.post(tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModAlt))
	u.wait("Replaced 2.")
	u.wait("dog dog dog dog")
	u.key(tcell.KeyEsc)
	u.key(tcell.KeyCtrlZ) // Replacing all is one step.
	u.wait("dog dog cat cat")
	u.wait("a.txt*")
}

func TestReplaceInAReadOnlyFile(t *testing.T) {
	u := start(t, map[string]string{"big.log": strings.Repeat("line\n", 2000)})
	if err := os.Truncate(filepath.Join(u.dir, "big.log"), 100<<20+1); err != nil {
		t.Fatal(err)
	}
	u.key(tcell.KeyEnter)
	u.wait("over 100 MB")
	u.press("x")
	u.wait("big.log is read-only.")
	u.post(tcell.NewEventKey(tcell.KeyRune, 'h', tcell.ModAlt))
	u.press("y")
	u.key(tcell.KeyTab)
	u.press("line")
	u.post(tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModAlt))
	u.wait("big.log is read-only.")
	u.wait("line")
}

// CORE-07: splits.

func TestSplitsShowOneFileTwice(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "shared text\n"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.post(tcell.NewEventKey(tcell.KeyRune, 's', tcell.ModAlt))
	u.waitCount("shared text", 2)
	u.press("new ") // Edits show in both views.
	u.waitCount("new shared text", 2)
	u.waitCount("a.txt*", 2)

	// Closing one view loses nothing, and doesn't ask.
	u.key(tcell.KeyCtrlW)
	u.waitCount("new shared text", 1)
	u.wait("a.txt*")
	u.key(tcell.KeyCtrlS)
	u.wait("Saved a.txt")
	if got := u.file("a.txt"); got != "new shared text\n" {
		t.Fatalf("got %q", got)
	}
}

func TestClosingTheLastViewAsks(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "text\n"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.post(tcell.NewEventKey(tcell.KeyRune, 's', tcell.ModAlt))
	u.waitCount("text", 2)
	u.press("x")
	u.waitCount("xtext", 2)
	u.key(tcell.KeyCtrlW)
	u.waitCount("xtext", 1)
	u.key(tcell.KeyCtrlW)
	u.wait("Save changes to a.txt?")
	u.press("c")
	u.wait("xtext")
}

func TestQuitAsksOnceForAFileInTwoViews(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "text\n"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.post(tcell.NewEventKey(tcell.KeyRune, 's', tcell.ModAlt))
	u.press("x")
	u.waitCount("xtext", 2)
	u.key(tcell.KeyCtrlBackslash)
	u.run("q")
	u.wait("before quitting?")
	u.press("d")
	u.quit()
}

func TestFileAndTerminalSideBySide(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "file text\n"})
	tool, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAZYCODE_TEST_EXIT", "0")
	t.Setenv("LAZYCODE_TEST_STAY", "1") // The tool stays running.
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")

	u.key(tcell.KeyEsc)
	u.press(":split")
	u.key(tcell.KeyEnter)
	u.waitCount("file text", 2)
	u.key(tcell.KeyCtrlW) // Leaves an empty split, the other view having closed.
	u.waitCount("file text", 1)

	u.post(tcell.NewEventKey(tcell.KeyRune, 's', tcell.ModAlt))
	u.waitCount("file text", 2)
	u.key(tcell.KeyEsc)
	u.press(":term " + tool)
	u.key(tcell.KeyEnter)
	u.wait("tool ran")
	u.waitCount("file text", 1) // That pane now shows the tool.

	// Alt+W isn't seen by LazyCode inside a terminal, so leave first.
	u.key(tcell.KeyCtrlBackslash)
	u.post(tcell.NewEventKey(tcell.KeyRune, 'w', tcell.ModAlt))
	u.wait("Ln 1, Col 1")
}

func TestEmptySplitTakesFilesAndCloses(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\n", "b.txt": "two\n"})
	u.post(tcell.NewEventKey(tcell.KeyRune, 's', tcell.ModAlt)) // Nothing open, so it's empty.
	u.wait("Empty split")
	u.waitCount("No tabs here.", 2)
	u.key(tcell.KeyEnter)
	u.wait("one")
	u.waitCount("No tabs here.", 1)
	u.key(tcell.KeyCtrlBackslash)
	u.post(tcell.NewEventKey(tcell.KeyRune, 'w', tcell.ModAlt)) // To the empty one.
	u.wait("Empty split")
	u.key(tcell.KeyDown)
	u.key(tcell.KeyEnter)
	u.wait("two")
	u.waitGone("No tabs here.")
	u.key(tcell.KeyCtrlW)
	u.waitGone("two") // That pane goes with its last tab.
	u.wait("one")
}

func TestSplitDownStacksThem(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "text\n"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.key(tcell.KeyEsc)
	u.press(":split down")
	u.key(tcell.KeyEnter)
	u.waitCount("text", 2)
	u.wait("─")
}

func TestAlreadyOpenSaysSo(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.key(tcell.KeyCtrlBackslash)
	u.key(tcell.KeyEnter)
	u.wait("a.txt is already open.") // Q-15.
}

// CORE-08: panels, mouse and themes.

func TestSecondPanel(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "text\n"})
	u.screen.SetSize(120, 30)
	u.wait("Files")
	u.key(tcell.KeyCtrlB)
	u.wait("No views here")
	u.key(tcell.KeyCtrlB)
	u.waitGone("No views here")

	// :view moves Files over, and opens the panel for it. The AI view stays in the first.
	u.run("view")
	u.wait("No sessions yet")
	u.wait("a.txt")
	u.waitGone("No views here")
	u.run("view")
	u.wait("a.txt")
	u.waitGone("No sessions yet")

	// Closing the panel holding Files brings the view back.
	u.run("view")
	u.wait("No sessions yet")
	u.key(tcell.KeyCtrlB)
	u.waitGone("No sessions yet")
	u.wait("Files")
	u.wait("a.txt")
}

func TestPanelsHideWhenNarrow(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "file body\n"})
	u.screen.SetSize(120, 30)
	u.key(tcell.KeyEnter)
	u.wait("file body")
	u.key(tcell.KeyCtrlBackslash)
	u.key(tcell.KeyCtrlB)
	u.wait("No views here")
	u.screen.SetSize(79, 30) // The second goes first (doc 07).
	u.post(tcell.NewEventResize(79, 30))
	u.waitGone("No views here")
	u.wait("Files")
	u.screen.SetSize(39, 30) // Then the first.
	u.post(tcell.NewEventResize(39, 30))
	u.waitGone("Files")
	u.wait("file body")
}

func (u *ui) click(x, y int) { u.post(tcell.NewEventMouse(x, y, tcell.Button1, 0)); u.release(x, y) }

func (u *ui) release(x, y int) { u.post(tcell.NewEventMouse(x, y, tcell.ButtonNone, 0)) }

func (u *ui) wheel(x, y int, button tcell.ButtonMask) {
	u.post(tcell.NewEventMouse(x, y, button, 0))
}

// clickOn clicks the first place text appears on screen.
func (u *ui) clickOn(text string) {
	u.t.Helper()
	u.wait(text)
	for y, line := range strings.Split(u.text(), "\n") {
		if i := strings.Index(line, text); i >= 0 {
			u.click(len([]rune(line[:i])), y)
			return
		}
	}
	u.t.Fatalf("%q isn't on screen", text)
}

func TestMouseOpensFilesAndMovesTheCursor(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "alpha\nbeta\ngamma\n", "b.txt": "second\n"})
	u.clickOn("a.txt")
	u.wait("Ln 1, Col 1")
	u.clickOn("gamma") // Line 3, at its start.
	u.wait("Ln 3, Col 1")
	u.clickOn("b.txt")
	u.wait("second")
	u.wait("Ln 1, Col 1")

	// A click on a tab goes there.
	u.clickOn(" a.txt ") // The tree's row has two spaces before it.
	u.wait("gamma")

	// A click past the end of a line goes to its end.
	w, _ := u.screen.Size()
	lines := strings.Split(u.text(), "\n")
	for y, line := range lines {
		if strings.Contains(line, "beta") {
			u.click(w-1, y)
			u.wait("Ln 2, Col 5")
			break
		}
	}
}

func TestShiftClickSelects(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "alpha\nbeta\n"})
	u.key(tcell.KeyEnter)
	u.wait("alpha")
	for y, line := range strings.Split(u.text(), "\n") {
		if strings.Contains(line, "beta") {
			u.post(tcell.NewEventMouse(len([]rune(line[:strings.Index(line, "beta")])), y, tcell.Button1, tcell.ModShift))
			break
		}
	}
	u.wait("Ln 2, Col 1")
	u.key(tcell.KeyDelete) // Removes what's selected.
	u.wait("beta")
	u.waitGone("alpha")
}

func TestMouseWheelScrollsWithoutMovingTheCursor(t *testing.T) {
	var text strings.Builder
	for i := range 100 {
		fmt.Fprintf(&text, "row-%d\n", i)
	}
	u := start(t, map[string]string{"a.txt": text.String()})
	u.key(tcell.KeyEnter)
	u.wait("row-0")
	w, h := u.screen.Size()
	u.wheel(w-1, h/2, tcell.WheelDown)
	u.wheel(w-1, h/2, tcell.WheelDown)
	u.waitGone("row-0\n")
	u.wait("row-6")
	u.wait("Ln 1, Col 1")
	u.wheel(w-1, h/2, tcell.WheelUp)
	u.wheel(w-1, h/2, tcell.WheelUp)
	u.wait("row-0")
	u.press("x") // Typing comes back to the cursor.
	u.wheel(w-1, h/2, tcell.WheelDown)
	u.waitGone("xrow-0")
	u.press("y")
	u.wait("xyrow-0")
}

func TestMouseInTheTree(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one"})
	if err := os.Mkdir(filepath.Join(u.dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	u.key(tcell.KeyCtrlBackslash)
	u.run("project") // Refreshes the tree when switched back to.
	u.key(tcell.KeyEnter)
	u.wait("▸ sub")
	u.clickOn("▸ sub")
	u.wait("▾ sub")
	w, h := u.screen.Size()
	_ = w
	u.wheel(2, h/2, tcell.WheelDown) // Moves the selection.
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
}

func TestMouseAnswersQuestions(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.press("x")
	u.wait("xone")
	u.key(tcell.KeyCtrlW)
	u.wait("Save changes")
	u.clickOn("xone") // A question takes everything until it's answered.
	u.wait("Save changes")
	u.clickOn("[c] Cancel")
	u.waitGone("Save changes")
	u.wait("xone")
}

func TestMouseInASwitcher(t *testing.T) {
	root := t.TempDir()
	one, two := filepath.Join(root, "one"), filepath.Join(root, "two")
	for _, d := range []string{one, two} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("AppData", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("LocalAppData", filepath.Join(root, "cache"))
	t.Setenv("HOME", root)
	screen := &recordingScreen{SimulationScreen: tcell.NewSimulationScreen("UTF-8")}
	u := &ui{t: t, screen: screen, dir: one, exited: make(chan error, 1)}
	go func() {
		u.exited <- Run(screen, []project.Project{{Name: "p", Dirs: []string{one, two}}}, one, "", "dev")
	}()
	t.Cleanup(func() {
		u.key(tcell.KeyCtrlBackslash)
		u.run("q")
		<-u.exited
	})
	u.wait("p › one")
	u.post(tcell.NewEventKey(tcell.KeyRune, 'd', tcell.ModAlt))
	u.wait("Switch directory")
	u.click(0, 0) // Outside cancels.
	u.waitGone("Switch directory")
	u.post(tcell.NewEventKey(tcell.KeyRune, 'd', tcell.ModAlt))
	u.wait("Switch directory")
	for y, line := range strings.Split(u.text(), "\n") {
		if strings.Contains(line, "  two") {
			u.click(len([]rune(line[:strings.Index(line, "two")])), y)
			break
		}
	}
	u.wait("p › two")
}

func TestThemeColoursApply(t *testing.T) {
	dark, light := themeFor("dark"), themeFor("light")
	if dark.accent == light.accent || dark.muted == light.muted {
		t.Fatal("the themes look the same")
	}
	// Nothing depends on colour alone (Q-48).
	for name, th := range map[string]theme{"dark": dark, "light": light} {
		if fg, _, attrs := th.accent.Decompose(); fg == tcell.ColorDefault || attrs&tcell.AttrBold == 0 {
			t.Errorf("%s: accent should be coloured and bold", name)
		}
		if _, bg, _ := th.active.Decompose(); bg == tcell.ColorDefault {
			t.Errorf("%s: selections should have a background", name)
		}
		if _, _, attrs := th.match.Decompose(); attrs&tcell.AttrUnderline == 0 {
			t.Errorf("%s: matches should be underlined", name)
		}
	}
}

func TestLowColourTerminal(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "dog dog\n"})
	u.screen.SetSize(80, 25)
	u.key(tcell.KeyEnter)
	u.wait("dog dog")
	u.key(tcell.KeyCtrlF)
	u.press("dog")
	u.wait("2 matches") // Draws without trouble, whatever the colours.
}

// CORE-09: config, rebinding and commands.

func writeSettings(t *testing.T, name, content string) {
	t.Helper()
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func prepared(t *testing.T, dir string, settings map[string]string) *ui {
	t.Helper()
	root := filepath.Dir(dir)
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("AppData", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("LocalAppData", filepath.Join(root, "cache"))
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
	for name, content := range settings {
		writeSettings(t, name, content)
	}
	return launch(t, dir)
}

func workDir(t *testing.T, files map[string]string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "work")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestConfigApplies(t *testing.T) {
	dir := workDir(t, map[string]string{"a.txt": "\tx\n"})
	u := prepared(t, dir, map[string]string{"config.toml": `
[editor]
tab_size = 8
[ui]
sidebar = "right"
`})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.key(tcell.KeyRight)
	u.wait("Ln 1, Col 9") // The tab is 8 wide.

	// The sidebar is on the right: its divider sits past the text.
	line := strings.Split(u.text(), "\n")[2]
	if !strings.HasSuffix(strings.TrimRight(line, " "), "│") && !strings.Contains(line, "│") {
		t.Fatalf("no divider: %q", line)
	}
	if strings.Index(line, "│") < 20 {
		t.Fatalf("the panel should be on the right: %q", line)
	}
}

func TestBrokenConfigSaysWhere(t *testing.T) {
	dir := workDir(t, nil)
	u := prepared(t, dir, map[string]string{"config.toml": "[ui]\ntheme = \"pink\"\n"})
	u.wait("theme must be")
	u.wait("Files") // Still running, on the defaults.
}

func TestConfigCommandOpensAndAppliesOnSave(t *testing.T) {
	dir := workDir(t, map[string]string{"a.txt": "\tx\n"})
	u := prepared(t, dir, nil)
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.key(tcell.KeyRight)
	u.wait("Ln 1, Col 5")

	u.key(tcell.KeyEsc)
	u.press(":config")
	u.key(tcell.KeyEnter)
	u.wait("LazyCode settings") // A new file starts from the template.
	if got := readConfig(t, "config.toml"); got != config.Template {
		t.Fatalf("template: %q", got)
	}

	// A bad value is reported on save, and the last good settings stay.
	u.mod(tcell.KeyHome, tcell.ModCtrl)
	u.press("[bad")
	u.key(tcell.KeyCtrlS)
	u.wait("config.toml, line")
	u.key(tcell.KeyCtrlZ)
	u.key(tcell.KeyCtrlZ)
	u.waitGone("[bad")

	// Replace the file's text with a good value, and it applies at once.
	u.key(tcell.KeyCtrlA)
	u.key(tcell.KeyBackspace2)
	u.post(tcell.NewEventPaste(true))
	u.press("[editor]")
	u.key(tcell.KeyEnter)
	u.press("tab_size = 2")
	u.post(tcell.NewEventPaste(false))
	u.key(tcell.KeyCtrlS)
	u.wait("Saved config.toml")
	u.post(tcell.NewEventKey(tcell.KeyRune, ',', tcell.ModAlt))
	u.wait("Ln 1, Col 3") // The tab is now 2 wide.
}

func readConfig(t *testing.T, name string) string {
	t.Helper()
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestKeysCanBeRebound(t *testing.T) {
	dir := workDir(t, map[string]string{"a.txt": "text\n"})
	u := prepared(t, dir, map[string]string{"keys.toml": `
[keys]
"f5" = "file.save"
"ctrl+s" = "none"
"alt+x" = "tab.close"
`})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.press("x")
	u.wait("a.txt*")
	u.key(tcell.KeyCtrlS) // Turned off.
	u.key(tcell.KeyLeft)
	u.wait("Ln 1, Col 1")
	if got := u.file("a.txt"); got != "text\n" {
		t.Fatalf("Ctrl+S still saved: %q", got)
	}
	u.key(tcell.KeyF5)
	u.wait("Saved a.txt")

	// The macOS form of an Alt key follows it.
	u.key(tcell.KeyCtrlK)
	u.wait("Ctrl+K: press")
	u.press("x")
	u.wait("No tabs here.")
}

func TestBadKeysAreReported(t *testing.T) {
	dir := workDir(t, nil)
	u := prepared(t, dir, map[string]string{"keys.toml": "[keys]\n\"ctrl+q\" = \"no.such\"\n"})
	u.wait("no.such")
	u.wait("Files")
}

func TestLeaveKeyCantBeRebound(t *testing.T) {
	dir := workDir(t, map[string]string{"a.txt": "text\n"})
	u := prepared(t, dir, map[string]string{"keys.toml": "[keys]\n\"ctrl+\\\\\" = \"file.save\"\n"})
	u.wait("reserved")
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.press("x")
	u.key(tcell.KeyCtrlBackslash)
	u.waitGone("Ctrl+S save")
	if got := u.file("a.txt"); got != "text\n" {
		t.Fatalf("the reserved key saved: %q", got)
	}
}

func TestAutoSave(t *testing.T) {
	dir := workDir(t, map[string]string{"a.txt": "text\n", "b.txt": "other\n"})
	u := prepared(t, dir, map[string]string{"config.toml": "[editor]\nauto_save = true\n"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.press("x")
	u.waitFile("a.txt", "xtext\n")
	u.waitGone("a.txt*")

	// A change on disk is never overwritten, and edits survive.
	u.press("y")
	u.wait("a.txt*")
	writeDisk(t, u, "a.txt", "tool\n")
	u.wait("Not auto-saved")
	if got := u.file("a.txt"); got != "tool\n" {
		t.Fatalf("overwritten: %q", got)
	}
	u.wait("a.txt*")
}

// A regression test: the timer used to save what "Don't save" had just thrown away.
func TestAutoSaveRespectsDontSave(t *testing.T) {
	dir := workDir(t, map[string]string{"a.txt": "text\n"})
	u := prepared(t, dir, map[string]string{"config.toml": "[editor]\nauto_save = true\n"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.press("x")
	u.wait("a.txt*")
	u.key(tcell.KeyCtrlW)
	u.wait("Save changes to a.txt?")
	u.press("d")
	u.wait("No tabs here.")
	time.Sleep(autoSaveDelay + 300*time.Millisecond)
	if got := u.file("a.txt"); got != "text\n" {
		t.Fatalf("saved after choosing not to: %q", got)
	}
}

func TestAutoSaveIsOffByDefault(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "text\n"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.press("x")
	u.wait("a.txt*")
	time.Sleep(autoSaveDelay + 300*time.Millisecond)
	if got := u.file("a.txt"); got != "text\n" {
		t.Fatalf("saved without asking: %q", got)
	}
}

func TestEveryKeymapActionIsHandled(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	dir := workDir(t, map[string]string{"a.txt": "text\n"})
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
	a := newApp(screen, []project.Project{{Name: "p", Dirs: []string{dir}}}, dir, "")
	recoveryDir := t.TempDir()
	a.recovery = editor.NewRecovery(recoveryDir, nil)
	defer close(a.done)
	go func() {
		for {
			select {
			case <-a.clipboardJobs:
			case <-a.done:
				return
			}
		}
	}()
	f, err := buffer.Open(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range keymap.Actions() {
		st := a.dir()
		if st.focus.current() == nil {
			a.addEditor(st, f)
		}
		a.switcher, a.command, a.prompts = nil, nil, nil
		a.closeFind()
		if !a.do(action) {
			t.Errorf("%s isn't handled", action)
		}
		a.draw()
	}
	a.recovery.Wait()
}

func TestSmallWindowsWithSplitsDontCrash(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "some text 世界\n"})
	u.key(tcell.KeyEnter)
	u.wait("some text")
	u.post(tcell.NewEventKey(tcell.KeyRune, 's', tcell.ModAlt))
	u.key(tcell.KeyCtrlB)
	u.post(tcell.NewEventKey(tcell.KeyRune, 's', tcell.ModAlt))
	for _, size := range [][2]int{{10, 5}, {3, 3}, {1, 1}, {39, 4}, {2, 40}, {200, 2}, {80, 25}} {
		u.screen.SetSize(size[0], size[1])
		u.post(tcell.NewEventResize(size[0], size[1]))
		u.click(0, 0)
		u.press("x")
	}
	u.wait("Files")
}

// One whole frame of a 100 MiB file, on a simulated screen. The budget for a key press
// is 16 ms.
func BenchmarkDrawFrame100MB(b *testing.B) {
	path := filepath.Join(b.TempDir(), "big.txt")
	content := strings.Repeat("the quick brown fox jumps over the lazy dog, line\n", 100<<20/50)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		b.Fatal(err)
	}
	f, err := buffer.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		b.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(120, 50)
	all := []project.Project{{Name: "test", Dirs: []string{b.TempDir()}}}
	a := &app{screen: screen, projects: all, dirOf: []int{0}, dirs: map[string]*dirState{}}
	it := a.addEditor(a.dir(), f)
	for b.Loop() {
		it.ed.PageDown(false)
		if it.ed.ScrollLine > 2_000_000 {
			it.ed.DocStart(false)
		}
		a.draw()
	}
}
