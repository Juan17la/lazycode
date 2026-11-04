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
	"github.com/Juan17la/lazycode/internal/editor"
	"github.com/Juan17la/lazycode/internal/project"
	"github.com/gdamore/tcell/v2"
)

// The test binary also plays a tool that exits as soon as it starts, and a LazyCode
// that dies with unsaved work.
func TestMain(m *testing.M) {
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

func start(t *testing.T, files map[string]string) *ui {
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
	return launch(t, dir)
}

func launch(t *testing.T, dir string) *ui {
	t.Helper()
	u := &ui{t: t, screen: &recordingScreen{SimulationScreen: tcell.NewSimulationScreen("UTF-8")}, dir: dir, exited: make(chan error, 1)}
	projects := []project.Project{{Name: "test", Dirs: []string{dir}}}
	go func() { u.exited <- Run(u.screen, projects, dir, "") }()
	u.wait("Files")
	t.Cleanup(func() {
		select {
		case <-u.exited:
		default:
			// Stop it, answering any question by not saving.
			u.post(tcell.NewEventKey(tcell.KeyEsc, 0, 0))
			u.post(tcell.NewEventKey(tcell.KeyCtrlBackslash, 0, 0))
			u.run("q")
			for range 20 {
				select {
				case <-u.exited:
					return
				case <-time.After(50 * time.Millisecond):
					u.press("d")
				}
			}
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
	for u.screen.PostEvent(ev) != nil {
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

func TestOpenTheSameFileTwice(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one"})
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	u.key(tcell.KeyCtrlBackslash)
	u.waitGone("Ln 1, Col 1")
	u.key(tcell.KeyEnter)
	u.wait("Ln 1, Col 1")
	if strings.Count(u.text(), " a.txt ") > 2 { // In the tree, and once as a tab.
		t.Fatalf("opened twice:\n%s", u.text())
	}
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
