// Package session holds what's known about AI tools and their sessions (doc 10): how to start and
// resume one, what's saved, and when one can sleep. The tabs themselves belong to the app.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Harness is how to run one AI tool (Q-35, Q-59). Defaults are data, so a user can fix a changed
// flag without a new release.
type Harness struct {
	Name       string   `toml:"name"`
	Command    string   `toml:"command"`
	StartArgs  []string `toml:"start_args"`
	ResumeArgs []string `toml:"resume_args"`
}

// CanResume reports whether stopping this tool can be undone. Without it, a click starts a new
// session (Q-59), and it never sleeps automatically (Q-77).
func (h Harness) CanResume() bool { return len(h.ResumeArgs) > 0 }

// Defaults returns the tools that open instantly. Claude Code and OpenCode's flags were tested
// (M0-08). Codex's resume flags weren't, because it wasn't installed, so it has none.
func Defaults() []Harness {
	return []Harness{
		{Name: "Claude Code", Command: "claude", StartArgs: []string{"--session-id", "{id}"}, ResumeArgs: []string{"--resume", "{id}"}},
		{Name: "OpenCode", Command: "opencode", StartArgs: []string{"--standalone", "--session", "ses_{id_hex}"}, ResumeArgs: []string{"--standalone", "--session", "ses_{id_hex}"}},
		{Name: "Codex", Command: "codex"},
	}
}

// Merge adds the user's tools to the defaults. One with the same name replaces the default.
func Merge(user []Harness) []Harness {
	all := Defaults()
	for _, h := range user {
		if i := slices.IndexFunc(all, func(d Harness) bool { return d.Name == h.Name }); i >= 0 {
			all[i] = h
		} else {
			all = append(all, h)
		}
	}
	return all
}

// Args returns the command line for this tool. A tool that takes no ID starts fresh each time.
func (h Harness) Args(id string, resume bool) []string {
	args := h.StartArgs
	if resume && h.CanResume() {
		args = h.ResumeArgs
	}
	hexID := strings.ReplaceAll(id, "-", "")
	out := []string{h.Command}
	for _, arg := range args {
		out = append(out, strings.NewReplacer("{id_hex}", hexID, "{id}", id).Replace(arg))
	}
	return out
}

// NewID returns a random UUID, which the tool is then told to use. So LazyCode knows which session
// is which without reading the tool's files (M0-08).
func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

// Session is what's saved about one. Removing a session only hides it (Q-60).
type Session struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Harness string    `json:"harness"`
	Dir     string    `json:"directory"`
	Hidden  bool      `json:"hidden,omitempty"`
	Created time.Time `json:"created,omitzero"` // Orders the AI tree, newest first.
	Used    time.Time `json:"used,omitzero"`    // AI mode comes back to the last one used.
}

// Status is what a session is doing (doc 10).
type Status int

// The states of a session.
const (
	Sleeping  Status = iota // No process, and a click resumes it.
	Running                 // The tool is alive.
	Attention               // It looks finished, or it's asking for something.
	Exited                  // It ended by itself, and its tab is showing that.
)

// StatePath returns where sessions are saved. It's in the cache folder, as losing it only loses the
// list (doc 08).
func StatePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lazycode", "state", "sessions.json"), nil
}

// Load reads the saved sessions. A missing file means none. A broken one returns an error, and none.
func Load(path string) ([]Session, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var sessions []Session
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, fmt.Errorf("saved sessions can't be read: %w", err)
	}
	return sessions, nil
}

// Save writes the sessions in one step, so a crash keeps the last good list.
func Save(path string, sessions []Session) error {
	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".sessions-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name()) //nolint:errcheck // Already renamed, unless something failed.
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}

// SleepCheck is what's known when deciding whether a session can sleep.
type SleepCheck struct {
	After      time.Duration // How long without output. Zero turns sleeping off.
	SinceOut   time.Duration
	Focused    bool
	CanResume  bool
	CPUSamples [2]time.Duration // Total CPU of the tool and everything it started, a while apart.
	SampleOK   bool
}

// busyCPU is how much CPU between two samples means it's still working on something.
const busyCPU = time.Second

// ShouldSleep applies Q-77's rules, which all have to hold. Stopping a tool that's working costs far
// more than keeping an idle one, so anything unknown means no.
//
// The rule says nothing the tool started is busy. That means using CPU, not merely alive: AI tools keep
// idle helpers such as language and MCP servers for as long as they run.
func ShouldSleep(c SleepCheck) bool {
	return c.After > 0 && c.SinceOut >= c.After && !c.Focused && c.CanResume &&
		c.SampleOK && c.CPUSamples[1]-c.CPUSamples[0] < busyCPU
}
