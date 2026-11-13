package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestArgs(t *testing.T) {
	id := "11111111-2222-4333-8444-555555555555"
	all := Defaults()
	claude, opencode, codex := all[0], all[1], all[2]
	if got := claude.Args(id, false); !slices.Equal(got, []string{"claude", "--session-id", id}) {
		t.Errorf("claude start: %v", got)
	}
	if got := claude.Args(id, true); !slices.Equal(got, []string{"claude", "--resume", id}) {
		t.Errorf("claude resume: %v", got)
	}
	want := []string{"opencode", "--standalone", "--session", "ses_11111111222243338444555555555555"}
	if got := opencode.Args(id, true); !slices.Equal(got, want) {
		t.Errorf("opencode: %v", got)
	}
	if got := codex.Args(id, true); !slices.Equal(got, []string{"codex"}) {
		t.Errorf("codex: %v", got)
	}
	if codex.CanResume() || !claude.CanResume() {
		t.Error("CanResume is wrong")
	}
}

func TestMerge(t *testing.T) {
	all := Merge([]Harness{{Name: "Claude Code", Command: "my-claude"}, {Name: "Aider", Command: "aider"}})
	if len(all) != 4 || all[0].Command != "my-claude" || all[3].Name != "Aider" {
		t.Fatalf("%+v", all)
	}
	if Defaults()[0].Command != "claude" {
		t.Fatal("the defaults changed")
	}
}

func TestNewID(t *testing.T) {
	uuid := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	first, err := NewID()
	if err != nil || !uuid.MatchString(first) {
		t.Fatalf("%q, %v", first, err)
	}
	if second, _ := NewID(); second == first {
		t.Fatal("the same twice")
	}
}

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "sessions.json")
	if got, err := Load(path); got != nil || err != nil {
		t.Fatalf("no file: %v, %v", got, err)
	}
	want := []Session{{ID: "a", Name: "auth rewrite", Harness: "Claude Code", Dir: "/x"}, {ID: "b", Name: `odd "name"`, Hidden: true}}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("%+v, %v", got, err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := Load(path); got != nil || err == nil {
		t.Fatalf("broken file: %v, %v", got, err)
	}
}

func TestShouldSleep(t *testing.T) {
	ok := SleepCheck{After: time.Minute, SinceOut: time.Hour, CanResume: true, SampleOK: true, CPUSamples: [2]time.Duration{time.Second, time.Second + time.Millisecond}}
	if !ShouldSleep(ok) {
		t.Fatal("every rule holds, and it stayed awake")
	}
	for name, change := range map[string]func(*SleepCheck){
		"turned off":   func(c *SleepCheck) { c.After = 0 },
		"recent":       func(c *SleepCheck) { c.SinceOut = time.Second },
		"focused":      func(c *SleepCheck) { c.Focused = true },
		"can't resume": func(c *SleepCheck) { c.CanResume = false },
		"busy":         func(c *SleepCheck) { c.CPUSamples[1] = c.CPUSamples[0] + 5*time.Second },
		"unknown CPU":  func(c *SleepCheck) { c.SampleOK = false },
	} {
		c := ok
		change(&c)
		if ShouldSleep(c) {
			t.Errorf("%s: it slept", name)
		}
	}
}

func TestTreeCPUSeesThisProcess(t *testing.T) {
	if _, err := exec.LookPath("ps"); err != nil && os.PathSeparator == '/' {
		t.Skip("ps isn't installed")
	}
	before, err := TreeCPU(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		for range 1 << 24 {
			_ = strings.ToUpper("spin")
		}
		after, err := TreeCPU(os.Getpid())
		if err != nil {
			t.Fatal(err)
		}
		if after > before {
			return
		}
	}
	t.Fatal("CPU use never went up")
}
