package term

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

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
