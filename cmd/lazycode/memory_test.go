package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Juan17la/lazycode/internal/term"
	"github.com/gdamore/tcell/v2"
)

const memoryLimitMB = 300 // D-36

// streamLines fills a tab's whole scrollback, in a 120-column tab with colours,
// the worst normal case for a terminal tab.
func streamLines() {
	line := "\x1b[32m" + strings.Repeat("x", 100) + "\x1b[0m"
	for i := range 50000 {
		fmt.Printf("%6d %s\n", i, line)
	}
	fmt.Println("stream done")
	time.Sleep(time.Hour)
}

// procMemoryMB returns current and peak resident memory.
func procMemoryMB(t *testing.T, pid int) (rss, peak int) {
	t.Helper()
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		kb, _ := strconv.Atoi(fields[1])
		switch fields[0] {
		case "VmRSS:":
			rss = kb / 1024
		case "VmHWM:":
			peak = kb / 1024
		}
	}
	return rss, peak
}

// TestMemoryStaysUnderLimit is M0-09, and guards the limit from then on. It runs
// LazyCode with full terminal tabs and reads its memory from the OS. The tools'
// own memory isn't counted (Q-70), and the prototype has no editor yet.
func TestMemoryStaysUnderLimit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc")
	}
	if testing.Short() {
		t.Skip("takes about a minute")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "lazycode")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	tool, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", root) // No projects file.
	t.Setenv("LAZYCODE_E2E_TOOL", "stream")

	// 153 columns leaves the terminal area 120 wide, beside the 32-column tree.
	tab, err := term.Start(root, []string{bin}, 153, 43, func() {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tab.Close)
	d := &driver{t: t, tab: tab}
	d.wait("Current folder")
	time.Sleep(time.Second)

	report := func(label string) int {
		rss, peak := procMemoryMB(t, tab.Pid())
		t.Logf("%-18s %4d MB now, %4d MB peak", label, rss, peak)
		return peak
	}
	report("idle, no tabs")

	var peak int
	tabs := 0
	for _, target := range []int{1, 5, 10} {
		for tabs < target {
			d.typeText(":term " + tool)
			d.enter()
			d.wait("stream done")
			d.key(tcell.KeyCtrlBackslash, tcell.ModCtrl)
			d.wait("Tab terminal")
			tabs++
		}
		time.Sleep(3 * time.Second) // Let the Go runtime settle.
		peak = report(fmt.Sprintf("%d full tabs", tabs))
		if peak >= memoryLimitMB {
			t.Errorf("%d MB peak with %d full tabs, over the %d MB limit", peak, tabs, memoryLimitMB)
		}
	}

	// Closing tabs must give memory back to the OS: sleeping sessions rely on it (D-28).
	for ; tabs > 0; tabs-- {
		d.key(tcell.KeyCtrlW, tcell.ModCtrl)
		time.Sleep(50 * time.Millisecond)
	}
	d.wait("No terminal tabs here.")
	time.Sleep(5 * time.Second)
	report("all tabs closed")
	if rss, _ := procMemoryMB(t, tab.Pid()); rss > 100 {
		t.Errorf("%d MB still held after closing every tab, so memory leaks or isn't returned", rss)
	}
}
