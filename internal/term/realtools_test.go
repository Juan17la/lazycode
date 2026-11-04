package term

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

// TestRealToolsResumeTheirChat checks M0-08 with the real tools, so it needs their
// logins and spends a little of their quota. It never runs in CI:
//
//	LAZYCODE_REAL_TOOLS=1 go test -v -count=1 -timeout 10m -run RealTools ./internal/term
//
// Each session it creates stays in that tool's history, in this repository's folder.
func TestRealToolsResumeTheirChat(t *testing.T) {
	if os.Getenv("LAZYCODE_REAL_TOOLS") == "" {
		t.Skip("set LAZYCODE_REAL_TOOLS=1 to run against real AI tools")
	}
	// A tool started from inside another one inherits its markers, and Claude Code
	// then turns off transcript saving, which is what resuming depends on.
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "CLAUDE") {
			os.Unsetenv(name)
		}
	}
	dir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []struct {
		name          string
		start, resume func(id string) []string
		id            func() string
		ready         string
	}{
		{
			name:   "claude",
			start:  func(id string) []string { return []string{"claude", "--session-id", id} },
			resume: func(id string) []string { return []string{"claude", "--resume", id} },
			id:     func() string { return uuid(t) },
			ready:  "❯",
		},
		{
			name:   "opencode",
			start:  func(id string) []string { return []string{"opencode", "--session", id} },
			resume: func(id string) []string { return []string{"opencode", "--session", id} },
			id:     func() string { return "ses_" + strings.ReplaceAll(uuid(t), "-", "") },
			ready:  "┃",
		},
	} {
		t.Run(tool.name, func(t *testing.T) {
			if _, err := exec.LookPath(tool.name); err != nil {
				t.Skip(tool.name + " isn't installed")
			}
			id := tool.id()
			code := "ZEBRA" + strings.ToUpper(strings.ReplaceAll(uuid(t), "-", "")[:6])
			t.Logf("session id %s, code %s", id, code)

			tab := startTool(t, dir, tool.start(id), tool.ready)
			send(tab, "The code is "+code+". Remember it. Reply with only the word STORED in lowercase letters.")
			waitReply(t, tab, "stored")

			started := time.Now()
			tab.Close()
			select {
			case <-tab.Done():
				t.Logf("stopped after hangup in %v", time.Since(started).Round(time.Millisecond))
			case <-time.After(10 * time.Second):
				t.Fatal("still running after Close")
			}

			tab = startTool(t, dir, tool.resume(id), tool.ready)
			defer tab.Close()
			time.Sleep(5 * time.Second)
			t.Logf("chat visible on resume, before typing: %v\n%s", strings.Contains(tab.Text(), code), tab.Text())

			// The lowercase code was never typed, so only the old chat can supply it.
			send(tab, "What code did I give you? Reply with only that code in lowercase letters.")
			waitReply(t, tab, strings.ToLower(code))
			t.Logf("resumed the same chat:\n%s", tab.Text())
		})
	}
}

func startTool(t *testing.T, dir string, args []string, ready string) *Tab {
	t.Helper()
	tab, err := Start(dir, args, 120, 40, func() {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tab.Close)
	waitReply(t, tab, ready)
	time.Sleep(3 * time.Second) // Let the tool finish drawing and take input.
	return tab
}

func send(tab *Tab, text string) {
	for _, r := range text {
		tab.Key(tcell.NewEventKey(tcell.KeyRune, r, 0))
		time.Sleep(3 * time.Millisecond)
	}
	// Enter right after fast typing can read as a pasted line break.
	time.Sleep(700 * time.Millisecond)
	tab.Key(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
}

func waitReply(t *testing.T, tab *Tab, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		if strings.Contains(tab.Text(), want) {
			return
		}
		if done, code := tab.Exited(); done {
			t.Fatalf("exited with code %d, without showing %q:\n%s", code, want, tab.Text())
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("screen never showed %q:\n%s", want, tab.Text())
}

func uuid(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
