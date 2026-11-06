package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Juan17la/lazycode/internal/term"
)

// TestStartupTime times the real binary from launch to its first full screen, in a real
// pseudo-terminal, with a folder of 1,000 files. The target is 100 ms (doc 12). It only
// reports: CI machines vary too much for a timing check to fail on.
func TestStartupTime(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "lazycode")
	if runtime.GOOS == "windows" {
		bin += ".exe" // Starting it fails without this.
	}
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", root)
	t.Setenv("LocalAppData", root)
	dir := filepath.Join(root, "project")
	mkdirs(t, dir)
	for i := range 1000 {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%04d.txt", i)), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	answerFirstQuestions(t, dir)

	const runs = 7
	times := make([]time.Duration, 0, runs)
	for range runs {
		start := time.Now()
		tab, err := term.Start(dir, []string{bin}, 120, 40, func() {})
		if err != nil {
			t.Fatal(err)
		}
		for !strings.Contains(tab.Text(), "Current folder") {
			if time.Since(start) > 30*time.Second {
				tab.Close()
				t.Fatalf("no first screen:\n%s", tab.Text())
			}
			time.Sleep(time.Millisecond)
		}
		times = append(times, time.Since(start))
		tab.Close()
	}
	slices.Sort(times)
	t.Logf("startup to first screen: median %.1f ms, worst %.1f ms, target 100 ms",
		float64(times[runs/2].Microseconds())/1000, float64(times[runs-1].Microseconds())/1000)
}
