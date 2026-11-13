//go:build unix

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Juan17la/lazycode/internal/term"
)

// If LazyCode is killed outright, tools in its tabs must not stay behind (doc 13). Killing it
// closes their terminals, and that hangs them up.
func TestToolsDontOutliveACrash(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "lazycode")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	answerFirstQuestions(t)
	pidFile := filepath.Join(root, "tool.pid")
	t.Setenv("LAZYCODE_E2E_TOOL", "1")
	t.Setenv("LAZYCODE_E2E_PIDFILE", pidFile)
	tool, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	tab, err := term.Start(root, []string{bin}, 120, 30, func() {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tab.Close)
	d := &driver{t: t, tab: tab}
	d.wait("Files")
	d.typeText(":term " + tool)
	d.enter()
	d.wait("tool ready")

	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("the tool isn't running: %v", err)
	}

	if err := syscall.Kill(tab.Pid(), syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			syscall.Kill(pid, syscall.SIGKILL) //nolint:errcheck // Cleaning up.
			t.Fatal("the tool outlived LazyCode")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
