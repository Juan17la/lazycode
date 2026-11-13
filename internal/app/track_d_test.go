package app

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Juan17la/lazycode/internal/buffer"
	"github.com/Juan17la/lazycode/internal/project"
	"github.com/Juan17la/lazycode/internal/session"
	"github.com/Juan17la/lazycode/internal/term"
	"github.com/gdamore/tcell/v2"
)

// runFakeTool plays an AI tool: it shows its arguments, and how it's started says what it does.
func runFakeTool(mode string) {
	fmt.Println("tool args:", strings.Join(os.Args[1:], " "))
	fmt.Println("cwd:", filepath.Base(mustGetwd()))
	if os.Getenv("CLAUDECODE") != "" {
		fmt.Println("INHERITED CLAUDECODE")
	}
	switch mode {
	case "exit":
		os.Exit(9)
	case "failresume":
		for _, arg := range os.Args[1:] {
			if arg == "--resume" {
				fmt.Println("no such session")
				os.Exit(4)
			}
		}
	}
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		switch in.Text() {
		case "work":
			fmt.Println("working")
		case "bell":
			fmt.Print("\a")
		default:
			fmt.Println("got:", in.Text())
		}
	}
	os.Exit(0)
}

func mustGetwd() string {
	dir, _ := os.Getwd()
	return dir
}

// withTool starts LazyCode with the test binary set up as an AI tool that can resume.
func withTool(t *testing.T, mode string, files map[string]string) *ui {
	t.Helper()
	tool, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAZYCODE_TEST_TOOL", mode)
	dir := workDir(t, files)
	root := filepath.Dir(dir)
	t.Setenv("LocalAppData", filepath.Join(root, "cache"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	return prepared(t, dir, map[string]string{"config.toml": fmt.Sprintf(`
[[harness]]
name = "Fake"
command = %q
start_args = ["--session-id", "{id}"]
resume_args = ["--resume", "{id}"]

[[harness]]
name = "Plain"
command = %q
`, tool, tool)})
}

// startFake picks the fake tool in the session list.
func (u *ui) startFake() {
	u.t.Helper()
	u.key(tcell.KeyCtrlBackslash)
	u.run("sessions")
	u.press("n")
	u.wait("Start a session")
	u.press("Fake")
	u.key(tcell.KeyEnter)
	u.wait("tool args: --session-id")
}

func (u *ui) alt(r rune) { u.post(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModAlt)) }

func (u *ui) sendLine(text string) {
	u.press(text)
	u.key(tcell.KeyEnter)
}

func TestStartingASession(t *testing.T) {
	u := withTool(t, "stay", nil)
	u.run("sessions")
	u.wait("No sessions yet")
	u.press("n")
	u.wait("Start a session")
	u.wait("Claude Code") // The defaults are listed too (FR-5.2).
	u.wait("Shell")       // And plain shells (FR-5.12).
	u.press("Fake")
	u.key(tcell.KeyEnter)

	u.wait("tool args: --session-id")
	u.wait("cwd: work")
	u.wait("Fake 1")
	u.sendLine("hello")
	u.wait("got: hello")

	// Several of one tool are fine (FR-5.5).
	u.key(tcell.KeyCtrlBackslash)
	u.run("sessions")
	u.press("n")
	u.press("Fake")
	u.key(tcell.KeyEnter)
	u.wait("Fake 2")
}

func TestToolsDontInheritClaudeVariablesInTabs(t *testing.T) {
	t.Setenv("CLAUDECODE", "1")
	u := withTool(t, "stay", nil)
	u.startFake()
	u.waitGone("INHERITED")
	if strings.Contains(u.text(), "INHERITED") {
		t.Fatalf("the tool saw CLAUDECODE:\n%s", u.text())
	}
}

func TestMissingToolIsMarkedAndSaysSo(t *testing.T) {
	dir := workDir(t, nil)
	root := filepath.Dir(dir)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("LocalAppData", filepath.Join(root, "cache"))
	u := prepared(t, dir, map[string]string{"config.toml": "[[harness]]\nname = \"Ghost\"\ncommand = \"lazycode-no-such-tool\"\n"})
	u.run("sessions")
	u.press("n")
	u.wait("Ghost  (not installed)")
	u.press("Ghost")
	u.key(tcell.KeyEnter)
	u.wait("can't find lazycode-no-such-tool")
	u.wait("No sessions yet") // It isn't added to the list.
}

func TestRenameAndHide(t *testing.T) {
	u := withTool(t, "stay", nil)
	u.startFake()
	u.key(tcell.KeyCtrlBackslash)
	u.run("sessions")
	u.wait("Fake 1")
	u.key(tcell.KeyF2)
	u.wait("Rename to: Fake 1")
	for range len("Fake 1") {
		u.key(tcell.KeyBackspace2)
	}
	u.press("auth rewrite")
	u.key(tcell.KeyEnter)
	u.waitCount("auth rewrite", 2) // In the list and on its tab.

	u.key(tcell.KeyDelete)
	u.wait("deleted from the list. The tool still has its chat")
	u.wait("No sessions yet")
	u.waitCount("auth rewrite", 1) // Only in the message: its tab stops with it.
}

func TestClosingAndResumingASession(t *testing.T) {
	u := withTool(t, "stay", nil)
	u.startFake()
	first := u.text()
	id := first[strings.Index(first, "--session-id ")+len("--session-id "):]
	id = strings.Fields(id)[0]
	u.key(tcell.KeyCtrlBackslash)
	u.key(tcell.KeyCtrlW) // Stops the tool, and the entry stays.
	u.waitGone("tool args")
	u.run("sessions")
	u.wait("○ Fake 1")
	u.key(tcell.KeyEnter) // The same chat comes back (FR-5.8).
	u.wait("tool args: --resume " + id)
	u.wait("✻ Fake 1")
}

func TestSessionsComeBackAfterQuitting(t *testing.T) {
	u := withTool(t, "stay", nil)
	u.startFake()
	first := u.text()
	id := strings.Fields(first[strings.Index(first, "--session-id ")+len("--session-id "):])[0]
	u.key(tcell.KeyCtrlBackslash)
	time.Sleep(quietPeriod + 100*time.Millisecond) // So that it doesn't look busy.
	u.run("q")
	u.quit()

	later := launch(t, u.dir)
	later.run("sessions")
	later.wait("○ Fake 1") // Sleeping, as the doc says.
	later.key(tcell.KeyEnter)
	later.wait("tool args: --resume " + id)
}

func TestQuitWarnsAboutBusySessions(t *testing.T) {
	u := withTool(t, "stay", nil)
	u.startFake()
	u.sendLine("work")
	u.wait("working")
	u.key(tcell.KeyCtrlBackslash)
	u.run("q")
	u.wait("look busy")
	u.press("c")
	u.waitGone("look busy")
	u.wait("working") // Still there.
	u.run("q")
	u.wait("look busy")
	u.press("q")
	u.quit()
}

func TestToolWithoutResumeStartsAfresh(t *testing.T) {
	u := withTool(t, "stay", nil)
	u.run("sessions")
	u.press("n")
	u.press("Plain")
	u.key(tcell.KeyEnter)
	u.wait("tool args:")
	u.key(tcell.KeyCtrlBackslash)
	u.key(tcell.KeyCtrlW)
	u.run("sessions")
	u.wait("○ Plain 1")
	u.key(tcell.KeyEnter) // A new session (FR-5.13).
	u.wait("tool args:")
	u.wait("✻ Plain 1")
}

func TestFailedResumeSaysSo(t *testing.T) {
	u := withTool(t, "failresume", nil)
	u.startFake()
	u.key(tcell.KeyCtrlBackslash)
	u.key(tcell.KeyCtrlW)
	u.run("sessions")
	u.wait("○ Fake 1")
	u.key(tcell.KeyEnter)
	u.wait("no such session") // The tool's error stays on screen.
	u.wait("[exit 4]")
	u.wait("✗ Fake 1")
}

func TestExitedSessionKeepsItsTabThenSleeps(t *testing.T) {
	u := withTool(t, "exit", nil)
	u.startFake()
	u.wait("[exit 9]")
	u.wait("exited with code 9")
	u.run("sessions")
	u.wait("✗ Fake 1")
	u.alt('e')
	u.key(tcell.KeyCtrlW)
	u.wait("No tabs here.")
	u.run("sessions")
	u.wait("○ Fake 1")
}

func TestShellsAreListedAndGoWithTheirTab(t *testing.T) {
	u := withTool(t, "stay", nil)
	tool, _ := os.Executable()
	u.run("term " + tool)
	u.wait("tool args:")
	name := strings.TrimSuffix(filepath.Base(tool), filepath.Ext(tool))
	u.key(tcell.KeyCtrlBackslash)
	u.run("sessions")
	u.wait("❯ " + name)
	u.alt('e')
	u.key(tcell.KeyCtrlW)
	u.run("sessions")
	u.wait("No sessions yet")
}

func TestViewsInPanels(t *testing.T) {
	u := withTool(t, "stay", map[string]string{"a.txt": "x"})
	u.wait("a.txt")
	u.run("sessions")
	u.wait("No sessions yet")
	u.waitGone("a.txt")
	u.clickOn("▤") // A click on an icon brings the view forward.
	u.wait("a.txt")
	u.run("sessions")
	u.wait("No sessions yet")
	u.alt('e')
	u.wait("a.txt")

	// With a window wide enough, the AI view can have its own panel, beside Files.
	u.screen.SetSize(120, 30)
	u.post(tcell.NewEventResize(120, 30))
	u.run("sessions")
	u.run("view")
	u.wait("a.txt")
	u.wait("No sessions yet")
	u.key(tcell.KeyCtrlB) // Closing the panel can't lose it.
	u.wait("Files")
	u.run("sessions")
	u.wait("No sessions yet")
}

// Status and sleep, with the clock passed in, so that tests don't wait half an hour.

type sent struct {
	mu   sync.Mutex
	list []string
}

func (s *sent) add(title, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.list = append(s.list, title+": "+body)
}

func (s *sent) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.list)
}

// sessionApp builds an app with one real running fake tool in a tab.
func sessionApp(t *testing.T) (*app, *sessionEntry, *sent) {
	t.Helper()
	tool, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAZYCODE_TEST_TOOL", "stay")
	dir := workDir(t, nil)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(screen.Fini)
	a := newApp(screen, []project.Project{{Name: "p", Dirs: []string{dir}}}, dir, "")
	t.Cleanup(func() { close(a.done); a.closeAllTabs() })
	a.cfg.Harness = []session.Harness{{Name: "Fake", Command: tool, ResumeArgs: []string{"--resume", "{id}"}}}
	notices := &sent{}
	a.sessions.send = notices.add
	a.sessions.cpuOf = func(int) (time.Duration, error) { return 0, nil }
	e := &sessionEntry{Session: session.Session{ID: "11111111-2222-4333-8444-555555555555", Name: "Fake 1", Harness: "Fake", Dir: dir}}
	a.sessions.entries = append(a.sessions.entries, e)
	h, _ := a.harness("Fake")
	if !a.launch(e, h, false) {
		t.Fatal(a.message)
	}
	waitText(t, e.tab.term, "tool args")
	return a, e, notices
}

func waitText(t *testing.T, tab *term.Tab, want string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for !strings.Contains(tab.Text(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("never showed %q:\n%s", want, tab.Text())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func typeInto(tab *term.Tab, text string) {
	for _, r := range text {
		tab.Key(tcell.NewEventKey(tcell.KeyRune, r, 0))
	}
	tab.Key(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
}

func TestAttentionAfterWorkThenQuiet(t *testing.T) {
	a, e, notices := sessionApp(t)
	a.tabFocused = false // The user is elsewhere.
	a.housekeeping(time.Now())
	e.seenOutput = time.Now()
	time.Sleep(2 * time.Millisecond)
	typeInto(e.tab.term, "work")
	waitText(t, e.tab.term, "working")
	last := e.tab.term.LastOutput()

	a.housekeeping(last.Add(quietPeriod - time.Second))
	if e.status != session.Running || notices.count() != 0 {
		t.Fatalf("too soon: %v, %d notices", e.status, notices.count())
	}
	a.housekeeping(last.Add(quietPeriod + time.Second))
	if e.status != session.Attention || notices.count() != 1 {
		t.Fatalf("after going quiet: %v, %d notices", e.status, notices.count())
	}
	a.housekeeping(last.Add(quietPeriod + 2*time.Second))
	if notices.count() != 1 {
		t.Fatalf("notified again: %d", notices.count())
	}

	// Working again clears the detail, and finishing again notifies again.
	e.seenOutput = time.Now()
	time.Sleep(2 * time.Millisecond)
	typeInto(e.tab.term, "work")
	waitForOutputAfter(t, e)
	a.housekeeping(time.Now())
	if e.status != session.Running {
		t.Fatalf("working again: %v", e.status)
	}

	// Looking at it clears the detail (doc 10).
	a.housekeeping(e.tab.term.LastOutput().Add(quietPeriod + time.Second))
	if e.status != session.Attention {
		t.Fatalf("%v", e.status)
	}
	a.tabFocused = true
	a.housekeeping(time.Now())
	if e.status != session.Running {
		t.Fatalf("looked at: %v", e.status)
	}
}

func waitForOutputAfter(t *testing.T, e *sessionEntry) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for !e.tab.term.LastOutput().After(e.seenOutput) {
		if time.Now().After(deadline) {
			t.Fatal("no new output")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestBellMeansAttentionAtOnce(t *testing.T) {
	a, e, notices := sessionApp(t)
	a.tabFocused = false
	a.housekeeping(time.Now())
	typeInto(e.tab.term, "bell")
	deadline := time.Now().Add(30 * time.Second)
	for e.tab.term.Bells() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no bell")
		}
		time.Sleep(5 * time.Millisecond)
	}
	a.housekeeping(time.Now())
	if e.status != session.Attention || notices.count() != 1 {
		t.Fatalf("%v, %d notices", e.status, notices.count())
	}
}

func TestViewedTabIsNotNotified(t *testing.T) {
	a, e, notices := sessionApp(t)
	a.tabFocused = true
	typeInto(e.tab.term, "work")
	waitText(t, e.tab.term, "working")
	a.housekeeping(e.tab.term.LastOutput().Add(time.Hour))
	if e.status != session.Running || notices.count() != 0 {
		t.Fatalf("%v, %d notices", e.status, notices.count())
	}
}

func TestResizeIsntTakenForWork(t *testing.T) {
	a, e, notices := sessionApp(t)
	a.tabFocused = false
	a.housekeeping(time.Now())
	e.ignoreUntil = time.Now().Add(time.Hour)
	typeInto(e.tab.term, "work")
	waitText(t, e.tab.term, "working")
	a.housekeeping(e.tab.term.LastOutput().Add(quietPeriod + time.Second))
	if e.status != session.Running || notices.count() != 0 {
		t.Fatalf("%v, %d notices", e.status, notices.count())
	}
}

// hideTab makes sure the session's tab isn't on screen, which a sleeping session needs.
func hideTab(a *app, e *sessionEntry) {
	st, p := a.findTab(e.tab)
	st.focus = p
	p.cur = len(p.tabs) // No current tab, so it's hidden.
}

func TestSleepingFollowsTheRules(t *testing.T) {
	a, e, _ := sessionApp(t)
	tab := e.tab.term
	late := tab.LastOutput().Add(31 * time.Minute)

	// On screen, it stays awake.
	a.tabFocused = false
	a.housekeeping(late)
	a.housekeeping(late.Add(sleepSample))
	if e.tab == nil {
		t.Fatal("slept while visible")
	}

	hideTab(a, e)
	if a.isVisible(e.tab) {
		t.Fatal("still visible")
	}

	// Too soon.
	a.housekeeping(tab.LastOutput().Add(29 * time.Minute))
	if e.tab == nil || !e.cpuAt.IsZero() {
		t.Fatal("measured or slept before the 30 minutes were up")
	}

	// CPU in use keeps it awake, with a sample needed a while apart.
	burn := time.Duration(0)
	a.sessions.cpuOf = func(int) (time.Duration, error) { burn += 10 * time.Second; return burn, nil }
	a.housekeeping(late)
	if e.tab == nil {
		t.Fatal("slept on the first sample")
	}
	a.housekeeping(late.Add(time.Second))
	if burn != 10*time.Second {
		t.Fatal("sampled again too soon")
	}
	a.housekeeping(late.Add(sleepSample))
	if e.tab == nil {
		t.Fatal("slept while using CPU")
	}

	// CPU that can't be measured keeps it awake.
	a.sessions.cpuOf = func(int) (time.Duration, error) { return 0, fmt.Errorf("no") }
	a.housekeeping(late.Add(2 * sleepSample))
	if e.tab == nil {
		t.Fatal("slept without knowing")
	}

	// Turned off keeps it awake.
	a.cfg.Sessions.SleepAfterMinutes = 0
	a.sessions.cpuOf = func(int) (time.Duration, error) { return 0, nil }
	a.housekeeping(late.Add(3 * sleepSample))
	a.housekeeping(late.Add(4 * sleepSample))
	if e.tab == nil {
		t.Fatal("slept while turned off")
	}

	// Idle at last: it stops, and the entry stays to resume from.
	a.cfg.Sessions.SleepAfterMinutes = 30
	a.housekeeping(late.Add(5 * sleepSample))
	a.housekeeping(late.Add(6 * sleepSample))
	if e.tab != nil || e.status != session.Sleeping {
		t.Fatalf("still awake: %v", e.status)
	}
	select {
	case <-tab.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("the tool is still running")
	}
}

func TestToolWithoutResumeNeverSleeps(t *testing.T) {
	a, e, _ := sessionApp(t)
	a.cfg.Harness[0].ResumeArgs = nil
	hideTab(a, e)
	late := e.tab.term.LastOutput().Add(time.Hour)
	a.housekeeping(late)
	a.housekeeping(late.Add(sleepSample))
	if e.tab == nil {
		t.Fatal("slept with no way back")
	}
}

// D2: :play.

func prePlay(t *testing.T, files map[string]string) *ui {
	t.Helper()
	dir := workDir(t, files)
	root := filepath.Dir(dir)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("LocalAppData", filepath.Join(root, "cache"))
	return prepared(t, dir, nil)
}

// echoCommand is a shell command that prints and stays, on each OS.
func echoCommand(text string) string {
	if os.PathSeparator == '\\' {
		return "echo " + text + " && pause"
	}
	return "echo " + text + "; sleep 60"
}

func TestPlayRunsThenRemembers(t *testing.T) {
	u := prePlay(t, nil)
	u.alt('r')
	u.wait("Command to run here:")
	u.sendLine(echoCommand("first-run"))
	u.wait("first-run")
	u.wait("⏵") // A terminal tab, and shows activity.

	// Pressing it again goes to its tab, and doesn't start another (Q-57).
	u.key(tcell.KeyCtrlBackslash)
	u.waitGone(`Ctrl+Space leaves`)
	u.alt('r')
	u.wait(`Ctrl+Space leaves`)
	if got := strings.Count(u.text(), "first-run"); got > 3 {
		t.Fatalf("started twice:\n%s", u.text())
	}
	u.key(tcell.KeyCtrlBackslash)
	u.run("q")
	u.quit()

	// Remembered for next time.
	later := launch(t, u.dir)
	later.run("play")
	later.wait("first-run")
}

func TestWrapText(t *testing.T) {
	if got := wrapText("abcdef", 4); len(got) != 2 || got[0] != "abcd" || got[1] != "ef" {
		t.Fatalf("got %q", got)
	}
	if got := wrapText("世界世", 4); len(got) != 2 || got[0] != "世界" || got[1] != "世" {
		t.Fatalf("wide: %q", got)
	}
}

func TestPlayDetects(t *testing.T) {
	u := prePlay(t, map[string]string{"go.mod": "module x\n", "main.go": "package main\n"})
	u.alt('r')
	u.wait("go run .")
}

func TestPlayWithACommandReplacesTheOldOne(t *testing.T) {
	u := prePlay(t, nil)
	u.run("play " + echoCommand("one"))
	u.wait("one")
	u.key(tcell.KeyCtrlBackslash)
	u.waitGone(`Ctrl+Space leaves`)
	u.alt('r') // Still running, so it goes there.
	u.wait(`Ctrl+Space leaves`)
}

func TestPlayRunsAgainOnceFinished(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("the command here is for sh")
	}
	u := prePlay(t, nil)
	u.run("play echo ran-it")
	u.wait("[exit 0]")
	u.alt('r')
	u.wait("[exit 0]")
	u.waitCount("[exit 0]", 1) // The finished tab was replaced.
}

func TestCommandBarInputCancels(t *testing.T) {
	u := prePlay(t, nil)
	u.alt('r')
	u.wait("Command to run here:")
	u.press("nope")
	u.key(tcell.KeyEsc)
	u.waitGone("Command to run here:")
	u.alt('r') // Not remembered.
	u.wait("Command to run here:")
}

// D5: live updates and markers.

func TestOpenFilesUpdateLive(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\ntwo\n"})
	u.key(tcell.KeyEnter)
	u.wait("one")
	writeDisk(t, u, "a.txt", "changed by a tool\n") // Without touching any key.
	u.wait("changed by a tool")
	u.waitGone("a.txt*")

	// As most tools save: a new file renamed over it.
	temp := filepath.Join(u.dir, ".tmp")
	if err := os.WriteFile(temp, []byte("renamed into place\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temp, filepath.Join(u.dir, "a.txt")); err != nil {
		t.Fatal(err)
	}
	u.wait("renamed into place")
}

func TestLiveUpdatesKeepUnsavedEdits(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\n"})
	u.key(tcell.KeyEnter)
	u.wait("one")
	u.press("mine ")
	u.wait("mine one")
	writeDisk(t, u, "a.txt", "tool\n")
	u.wait("Your edits are kept")
	u.wait("mine one")
	u.key(tcell.KeyCtrlS)
	u.wait("a.txt changed on disk.") // Saving still asks.
}

func TestOwnSaveDoesntReload(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\n"})
	u.key(tcell.KeyEnter)
	u.wait("one")
	u.press("x")
	u.key(tcell.KeyCtrlS)
	u.wait("Saved a.txt")
	u.key(tcell.KeyCtrlZ) // The undo history is still there, so nothing reloaded.
	u.wait("a.txt*")
	u.wait(" one")
}

func TestGrowingLargeFileOnlySaysSo(t *testing.T) {
	u := start(t, map[string]string{"big.log": strings.Repeat("line\n", 2000)})
	path := filepath.Join(u.dir, "big.log")
	if err := os.Truncate(path, 100<<20+1); err != nil {
		t.Fatal(err)
	}
	u.key(tcell.KeyEnter)
	u.wait("over 100 MB")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("more\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	u.wait("Close and reopen")
}

func TestMarkersOutsideGit(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\ntwo\nthree\n"})
	u.key(tcell.KeyEnter)
	u.wait("three")
	time.Sleep(2 * marksDelay)
	if strings.Contains(u.text(), "▌") {
		t.Fatalf("marks before any change:\n%s", u.text())
	}
	writeDisk(t, u, "a.txt", "one\nTWO\nthree\nfour\n")
	u.wait("TWO")
	u.waitCount("▌", 2) // Compared with the file as it was when opened.

	// Unsaved edits count too (doc 06).
	u.key(tcell.KeyDown)
	u.key(tcell.KeyDown)
	u.press("x")
	u.waitCount("▌", 3)
	u.key(tcell.KeyCtrlZ)
	u.waitCount("▌", 2)
}

func TestMarkersFollowGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git isn't installed")
	}
	dir := workDir(t, map[string]string{"a.txt": "one\ntwo\nthree\n"})
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("add", ".")
	git("commit", "-qm", "first")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\nedited before opening\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	u := prepared(t, dir, nil)
	u.key(tcell.KeyEnter)
	u.wait("edited before opening")
	u.waitCount("▌", 1) // Compared with the commit, not with what was opened.

	// A tool commits the change, and the marker goes.
	git("add", ".")
	git("commit", "-qm", "second")
	writeDisk(t, u, "a.txt", "one\nedited before opening\nthree\nnew line\n")
	u.wait("new line")
	u.waitCount("▌", 1)
	if got := strings.Split(u.text(), "\n"); !strings.Contains(strings.Join(got, "\n"), "▌") {
		t.Fatal("no marker")
	}
}

func TestLargeFilesHaveNoMarkers(t *testing.T) {
	a := &app{marks: map[*buffer.File]*fileMarks{}}
	path := filepath.Join(t.TempDir(), "big.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x\n", maxMarkedFile/2+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := buffer.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := a.marksFor(f); got != nil || len(a.marks) != 0 {
		t.Fatalf("%v, %d", got, len(a.marks))
	}
}
