package term

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/gdamore/tcell/v2"
)

// The test binary doubles as the tool running inside the terminal.
func TestMain(m *testing.M) {
	switch os.Getenv("LAZYCODE_TERM_HELPER") {
	case "exit":
		fmt.Println("helper ready")
		os.Exit(3)
	case "sleep":
		fmt.Println("helper ready")
		time.Sleep(time.Hour)
	case "mouse":
		fmt.Print("\x1b[?1000h\x1b[?1006h") // Asks for mouse reports, in the SGR format.
		fmt.Println("helper ready")
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			fmt.Printf("GOT:%q\n", sc.Text())
		}
		os.Exit(1)
	case "lines":
		for i := range 200 {
			fmt.Printf("line %d\n", i)
		}
		fmt.Println("helper ready")
		time.Sleep(time.Hour)
	case "bell":
		fmt.Println("helper ready")
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			fmt.Print("\a")
		}
		os.Exit(1)
	case "breakit": // The output that makes the emulator panic.
		fmt.Print("\x1b[1;200r\x1b[L")
		fmt.Println("helper ready")
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			fmt.Printf("GOT:%q\n", sc.Text())
		}
		os.Exit(1)
	case "nomouse":
		fmt.Println("helper ready")
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			fmt.Printf("GOT:%q\n", sc.Text())
		}
		os.Exit(1)
	case "count":
		// Echoes every line, like a tool in a terminal does, and counts them.
		fmt.Println("helper ready")
		sc := bufio.NewScanner(os.Stdin)
		n := 0
		for sc.Scan() {
			if sc.Text() == "END" {
				fmt.Printf("COUNT:%d\n", n)
				time.Sleep(time.Hour)
			}
			n++
			fmt.Println(sc.Text())
		}
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func startHelper(t *testing.T, mode string) *Tab {
	t.Helper()
	t.Setenv("LAZYCODE_TERM_HELPER", mode)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tab, err := Start(t.TempDir(), []string{exe}, 80, 24, func() {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tab.Close)
	waitFor(t, tab, "helper ready")
	return tab
}

func waitFor(t *testing.T, tab *Tab, want string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(tab.Text(), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("screen never showed %q:\n%s", want, tab.Text())
}

func TestExitCodeIsReported(t *testing.T) {
	tab := startHelper(t, "exit")
	<-tab.Done()
	if done, code := tab.Exited(); !done || code != 3 {
		t.Fatalf("Exited() = %v, %d, want true, 3", done, code)
	}
}

func TestCloseStopsTheCommand(t *testing.T) {
	tab := startHelper(t, "sleep")
	tab.Close()
	select {
	case <-tab.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("command still running after Close")
	}
}

func TestMissingCommandIsAClearError(t *testing.T) {
	_, err := Start(t.TempDir(), []string{"lazycode-no-such-tool"}, 80, 24, func() {})
	if err == nil || !strings.Contains(err.Error(), "lazycode-no-such-tool") {
		t.Fatalf("err = %v, want it to name the command", err)
	}
}

// A large paste into a tool that echoes used to be able to deadlock. See inputQueue.
func TestLargePasteDoesNotDeadlock(t *testing.T) {
	tab := startHelper(t, "count")
	const lines = 4000
	line := strings.Repeat("x", 60) + "\r"
	tab.Paste(strings.Repeat(line, lines) + "END\r")
	waitFor(t, tab, fmt.Sprintf("COUNT:%d", lines))
}

func TestModifiedKeySeq(t *testing.T) {
	for _, tc := range []struct {
		key  tcell.Key
		mods tcell.ModMask
		want string
	}{
		{tcell.KeyBacktab, 0, "\x1b[Z"},
		{tcell.KeyTab, tcell.ModShift, "\x1b[Z"},
		{tcell.KeyLeft, tcell.ModCtrl, "\x1b[1;5D"},
		{tcell.KeyUp, tcell.ModShift, "\x1b[1;2A"},
		{tcell.KeyDelete, tcell.ModAlt, "\x1b[3;3~"},
		{tcell.KeyF5, tcell.ModCtrl | tcell.ModShift, "\x1b[15;6~"},
		{tcell.KeyLeft, 0, ""}, // Plain keys go through the emulator, for app cursor mode.
		{tcell.KeyEnter, tcell.ModShift, ""},
	} {
		got := modifiedKeySeq(tcell.NewEventKey(tc.key, 0, tc.mods))
		if got != tc.want {
			t.Errorf("key %v mods %v: got %q, want %q", tc.key, tc.mods, got, tc.want)
		}
	}
}

// Ctrl+\ is the one key that must always work, and each OS delivers it differently.
func TestIsLeaveKey(t *testing.T) {
	for name, ev := range map[string]*tcell.EventKey{
		"unix":         tcell.NewEventKey(tcell.KeyCtrlBackslash, 0, tcell.ModCtrl),
		"control char": tcell.NewEventKey(tcell.KeyRune, 0x1c, tcell.ModNone),
		"windows":      tcell.NewEventKey(tcell.KeyRune, '|', tcell.ModCtrl),
		"kitty":        tcell.NewEventKey(tcell.KeyRune, '\\', tcell.ModCtrl),
	} {
		if !IsLeaveKey(ev) {
			t.Errorf("%s: %v not recognised", name, ev.Name())
		}
	}
	for name, ev := range map[string]*tcell.EventKey{
		"plain pipe":      tcell.NewEventKey(tcell.KeyRune, '|', 0),
		"plain backslash": tcell.NewEventKey(tcell.KeyRune, '\\', 0),
		"ctrl+k":          tcell.NewEventKey(tcell.KeyCtrlK, 0, tcell.ModCtrl),
	} {
		if IsLeaveKey(ev) {
			t.Errorf("%s must not leave the terminal", name)
		}
	}
}

func TestWindowsCtrlPunctuationIsForwardedCorrectly(t *testing.T) {
	for in, want := range map[rune]rune{'}': ']', '~': '^', '{': '['} {
		got, ok := toEmulatorKey(tcell.NewEventKey(tcell.KeyRune, in, tcell.ModCtrl))
		if !ok || got.Code != want {
			t.Errorf("Ctrl+%c: got %q, want %q", in, got.Code, want)
		}
	}
}

func TestToEmulatorKey(t *testing.T) {
	ctrlA, _ := toEmulatorKey(tcell.NewEventKey(tcell.KeyCtrlA, 0, tcell.ModCtrl))
	if ctrlA.Code != 'a' || ctrlA.Text != "" {
		t.Errorf("Ctrl+A = %+v", ctrlA)
	}
	bang, _ := toEmulatorKey(tcell.NewEventKey(tcell.KeyRune, '!', 0))
	if bang.Text != "!" {
		t.Errorf("! = %+v", bang)
	}
	if _, ok := toEmulatorKey(tcell.NewEventKey(tcell.KeyRune, 0, 0)); ok {
		t.Error("an empty rune event must not be sent")
	}
}

func TestMouseReachesToolsThatAskForIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ConPTY delivers input to a console program differently, and this reads lines")
	}
	tab := startHelper(t, "mouse")
	tab.Mouse(2, 1, false, false, true)
	tab.Mouse(2, 1, false, false, false)
	tab.Mouse(2, 1, false, true, false)
	tab.Key(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	waitFor(t, tab, "<0;3;2M") // Column and row count from 1.
	waitFor(t, tab, "<0;3;2m")
	waitFor(t, tab, "<65;3;2M")
}

func TestMouseDoesntReachToolsThatDidntAsk(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ConPTY delivers input to a console program differently, and this reads lines")
	}
	tab := startHelper(t, "nomouse")
	tab.Mouse(2, 1, false, false, true)
	tab.Mouse(2, 1, false, true, false)
	for _, r := range "x" {
		tab.Key(tcell.NewEventKey(tcell.KeyRune, r, 0))
	}
	tab.Key(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	waitFor(t, tab, `GOT:"x"`)
}

func TestWheelScrollsBackWhenTheToolDidntAskForTheMouse(t *testing.T) {
	tab := startHelper(t, "lines")
	waitFor(t, tab, "line 199")
	if tab.ScrolledBack() != 0 {
		t.Fatal("started scrolled back")
	}
	if _, _, visible := tab.Cursor(); !visible {
		t.Fatal("the cursor should show on the live screen")
	}
	for range 20 {
		tab.Mouse(0, 0, true, false, false)
	}
	if tab.ScrolledBack() != 60 {
		t.Fatalf("scrolled %d", tab.ScrolledBack())
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)
	tab.Draw(screen, 0, 0, 80, 24)
	screen.Show()
	cells, w, _ := screen.GetContents()
	var first strings.Builder
	for x := range w {
		first.WriteString(string(cells[x].Runes))
	}
	if strings.Contains(first.String(), "line 199") || !strings.Contains(first.String(), "line ") {
		t.Fatalf("the top row shows %q", first.String())
	}
	if _, _, visible := tab.Cursor(); visible {
		t.Fatal("the cursor should hide while scrolled back")
	}
	for range 40 {
		tab.Mouse(0, 0, false, true, false)
	}
	if tab.ScrolledBack() != 0 {
		t.Fatalf("scrolled %d after coming back", tab.ScrolledBack())
	}
	tab.Mouse(0, 0, true, false, false)
	tab.Key(tcell.NewEventKey(tcell.KeyRune, 'x', 0)) // Typing goes back to live.
	if tab.ScrolledBack() != 0 {
		t.Fatal("typing should return to the live screen")
	}
	for range 5000 {
		tab.Mouse(0, 0, true, false, false)
	}
	if tab.ScrolledBack() > 200 {
		t.Fatalf("scrolled past what's kept: %d", tab.ScrolledBack())
	}
}

func TestOutputAndBellAreTracked(t *testing.T) {
	tab := startHelper(t, "bell")
	if time.Since(tab.LastOutput()) > time.Minute {
		t.Fatal("output time wasn't recorded")
	}
	before := tab.LastOutput()
	if tab.Bells() != 0 {
		t.Fatalf("%d bells before any", tab.Bells())
	}
	time.Sleep(5 * time.Millisecond)
	tab.Key(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	deadline := time.Now().Add(30 * time.Second)
	for tab.Bells() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if tab.Bells() != 1 {
		t.Fatalf("%d bells", tab.Bells())
	}
	if !tab.LastOutput().After(before) {
		t.Fatal("output time didn't move")
	}
}

// A regression test: this output crashed the whole editor, through vt's panic.
func TestOutputThatPanicsTheEmulatorIsSurvived(t *testing.T) {
	tab := startHelper(t, "breakit")
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	tab.Draw(screen, 0, 0, 80, 24)
	tab.Key(tcell.NewEventKey(tcell.KeyRune, 'x', 0))
	tab.Key(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	waitFor(t, tab, `GOT:"x"`) // Still running, and still showing output.
}

func TestToolsDontInheritClaudeVariables(t *testing.T) {
	got := toolEnv([]string{"PATH=/bin", "CLAUDECODE=1", "CLAUDE_CODE_ENTRYPOINT=cli", "HOME=/h", "MYCLAUDE=kept"})
	want := []string{"PATH=/bin", "HOME=/h", "MYCLAUDE=kept"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v", got)
	}
}

func TestNextPiece(t *testing.T) {
	for _, tc := range []struct {
		data string
		want []string
	}{
		{"plain", []string{"plain"}},
		{"a\x1b[Lb", []string{"a", "\x1b[L", "b"}},
		{"\x1b[1;200r\x1b[Lhello", []string{"\x1b[1;200r", "\x1b[L", "hello"}},
		{"\x1b[12", []string{"\x1b[12"}}, // Ends in the next chunk.
		{"\x1b]0;title\x07text\x1b[m", []string{"\x1b]0;title\x07text", "\x1b[m"}},
		{"\x1b", []string{"\x1b"}},
		{"e\u0301世", []string{"e\u0301世"}}, // Characters stay whole.
	} {
		var got []string
		data := []byte(tc.data)
		for len(data) > 0 {
			n := nextPiece(data)
			got = append(got, string(data[:n]))
			data = data[n:]
		}
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%q: got %q", tc.data, got)
		}
	}
}

// A regression test: these froze the editor, as the emulator loops on the number while holding
// the lock that drawing needs.
func TestHugeNumbersDontStallTheEmulator(t *testing.T) {
	for name, chunks := range map[string][]string{
		"repeat":     {"x\x1b[200000000b"},
		"tabs":       {"\x1b[2000000000Z"},
		"split":      {"x\x1b[2000", "00", "0000b"},
		"split esc":  {"x\x1b", "[2000000000b"},
		"with marks": {"x\x1b[?200000000;200000000b"},
		"restarted":  {"x\x1b[200\x1b[2000000000Z\x1b[200000000X"},
		"8-bit":      {"x\x9b200000000b"},
		"8-bit tabs": {"\x9b2000000000Z"},
	} {
		t.Run(name, func(t *testing.T) {
			emu := vt.NewEmulator(120, 40)
			tab := &Tab{emu: emu, mouseModes: map[ansi.Mode]bool{}}
			done := make(chan struct{})
			go func() {
				defer close(done)
				for _, chunk := range chunks {
					tab.write([]byte(chunk))
				}
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("the emulator is still looping")
			}
		})
	}
}

func TestCapNumbers(t *testing.T) {
	for in, want := range map[string]string{
		"\x1b[200000000b":      "\x1b[2000b",
		"\x1b[1;200r":          "\x1b[1;200r",
		"\x1b[38;2;255;0;10m":  "\x1b[38;2;255;0;10m",
		"\x1b[99999;12345H":    "\x1b[9999;1234H",
		"plain 123456789":      "plain 123456789", // Text keeps its numbers.
		"\x1b]0;123456789\x07": "\x1b]0;123456789\x07",
	} {
		if got := string(capNumbers([]byte(in))); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestSequencesSplitAcrossReadsStillWork(t *testing.T) {
	emu := vt.NewEmulator(40, 10)
	tab := &Tab{emu: emu, mouseModes: map[ansi.Mode]bool{}}
	for _, chunk := range []string{"ab\x1b", "[3", "1mcd\x1b[", "0m"} {
		tab.write([]byte(chunk))
	}
	if got := strings.TrimSpace(tab.Text()); got != "abcd" {
		t.Fatalf("got %q", got)
	}
	if len(tab.unfinished) != 0 {
		t.Fatalf("still waiting on %q", tab.unfinished)
	}
	// A sequence that never ends isn't kept forever.
	tab.write([]byte("\x1b[" + strings.Repeat("1;", maxSequence)))
	if len(tab.unfinished) != 0 {
		t.Fatalf("kept %d bytes", len(tab.unfinished))
	}
}

func TestCharactersHoldingTheC1ByteStayWhole(t *testing.T) {
	const text = "仛a仛" // 仛 is e4 bb 9b.
	whole := &Tab{emu: vt.NewEmulator(40, 10), mouseModes: map[ansi.Mode]bool{}}
	whole.write([]byte(text))
	if got := strings.TrimSpace(whole.Text()); got != text {
		t.Fatalf("whole: %q", got)
	}
	for cut := 1; cut < len(text); cut++ { // Split at every byte, which reads can do.
		tab := &Tab{emu: vt.NewEmulator(40, 10), mouseModes: map[ansi.Mode]bool{}}
		tab.write([]byte(text[:cut]))
		tab.write([]byte(text[cut:]))
		if got := strings.TrimSpace(tab.Text()); got != text {
			t.Fatalf("split at %d: %q", cut, got)
		}
		if len(tab.unfinished) != 0 {
			t.Fatalf("split at %d: still holding %q", cut, tab.unfinished)
		}
	}
}
