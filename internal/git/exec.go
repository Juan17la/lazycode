// Package git wraps the git command (doc 06). Arguments are always a fixed list, with "--" before
// any path, and never go through a shell.
//
// A repository can be hostile: its .git/config can name programs that git runs. Reading
// functions here are safe to run in the background: they turn off the fsmonitor hook, external diff
// and text conversion programs, and file filters, and take no locks. Functions that write run what
// the repository configures, such as hooks.
package git

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode"
)

var (
	// ErrMissing means git isn't installed.
	ErrMissing = errors.New("git isn't installed. Get it from https://git-scm.com/downloads")
	// ErrNotRepo means the folder isn't inside a repository.
	ErrNotRepo = errors.New("this folder isn't a Git repository")
)

const (
	readTimeout    = 20 * time.Second
	writeTimeout   = 60 * time.Second
	networkTimeout = 3 * time.Minute
	// maxOutput bounds what one command can hand back, so a huge diff can't use all the memory.
	maxOutput = 8 << 20
	// emptyTree is git's well-known empty tree. Reading file attributes from it means the
	// repository's .gitattributes can't pick a filter program to run while looking at status.
	emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
)

// Error is git's own message for a failure, with control characters removed.
type Error struct {
	Msg string
	err error
}

func (e *Error) Error() string { return e.Msg }
func (e *Error) Unwrap() error { return e.err }

// Clean removes characters that could change what a terminal shows: control characters, and
// the ones that reorder text. Line breaks become spaces.
func Clean(s string) string {
	s = strings.ToValidUTF8(s, "�")
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == ' ' || r == ' ':
			return -1
		}
		return r
	}, s)
}

var (
	versionOnce  sync.Once
	hasAttrSrc   bool
	gitInstalled bool
)

// probe finds out once whether git exists and knows --attr-source (2.40).
func probe() {
	versionOnce.Do(func() {
		out, err := exec.Command("git", "--version").Output()
		if err != nil {
			return
		}
		gitInstalled = true
		var major, minor int
		fields := strings.Fields(string(out)) // "git version 2.47.3"
		if len(fields) >= 3 {
			parts := strings.SplitN(fields[2], ".", 3)
			if len(parts) >= 2 {
				major, minor = atoi(parts[0]), atoi(parts[1])
			}
		}
		hasAttrSrc = major > 2 || (major == 2 && minor >= 40)
	})
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// Installed reports whether git can be run.
func Installed() bool { probe(); return gitInstalled }

// ResetForTest makes the next call look for git again, for tests that change PATH.
func ResetForTest() { versionOnce = sync.Once{}; gitInstalled, hasAttrSrc = false, false }

// run runs git in dir. write is true for the functions that may run the repository's programs.
// It returns stdout, which is cut at maxOutput (truncated says so).
func run(ctx context.Context, dir string, write bool, timeout time.Duration, args ...string) (out []byte, truncated bool, err error) {
	return runCap(ctx, dir, write, timeout, maxOutput, args...)
}

func runCap(ctx context.Context, dir string, write bool, timeout time.Duration, limit int, args ...string) (out []byte, truncated bool, err error) {
	probe()
	if !gitInstalled {
		return nil, false, ErrMissing
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	all := []string{"-C", dir, "--no-pager", "--literal-pathspecs"}
	if !write {
		// log.showSignature would have log and show run the repository's gpg.program on signed commits.
		all = append(all, "-c", "core.fsmonitor=false", "-c", "log.showSignature=false", "--no-optional-locks")
		if hasAttrSrc {
			all = append(all, "--attr-source="+emptyTree)
		}
	}
	cmd := exec.CommandContext(ctx, "git", append(all, args...)...)
	Detach(cmd)
	// LC_ALL=C so that git's messages can be recognised. GIT_TERMINAL_PROMPT=0 and the lack of a
	// terminal (detach) mean that a password question fails instead of hanging.
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_EDITOR=true", "GIT_PAGER=cat")
	cmd.Stdin = nil
	cmd.WaitDelay = 3 * time.Second // A grandchild holding the pipe open can't hang us.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &capped{buf: &stdout, cancel: cancel, truncated: &truncated, limit: limit}
	cmd.Stderr = &capped{buf: &stderr, cancel: cancel, truncated: new(bool), limit: maxOutput}
	runErr := cmd.Run()
	if truncated {
		return stdout.Bytes(), true, nil
	}
	if runErr == nil {
		return stdout.Bytes(), false, nil
	}
	if errors.Is(runErr, exec.ErrNotFound) {
		return nil, false, ErrMissing
	}
	if ctx.Err() == context.DeadlineExceeded {
		return nil, false, &Error{Msg: "git didn't finish in " + timeout.String() + " and was stopped", err: runErr}
	}
	msg := strings.TrimSpace(stderr.String())
	if msg == "" {
		msg = strings.TrimSpace(stdout.String()) // Merge reports conflicts there.
	}
	if msg == "" {
		msg = runErr.Error()
	}
	if strings.Contains(msg, "not a git repository") {
		return stdout.Bytes(), false, ErrNotRepo
	}
	return stdout.Bytes(), false, &Error{Msg: Clean(firstLines(msg, 3)), err: runErr}
}

// firstLines keeps the first n lines that say something: git's "To <remote>" and "hint:" lines are
// left out, and so is the "error: " and "fatal: " that begin the rest.
func firstLines(s string, n int) string {
	var keep []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(strings.TrimPrefix(line, "error: "), "fatal: ")
		if line == "" || strings.HasPrefix(line, "hint:") || strings.HasPrefix(line, "To ") {
			continue
		}
		keep = append(keep, line)
		if len(keep) == n {
			break
		}
	}
	return strings.Join(keep, " ")
}

// capped keeps up to maxOutput bytes, then stops the command.
type capped struct {
	buf       *bytes.Buffer
	cancel    context.CancelFunc
	truncated *bool
	limit     int
}

func (c *capped) Write(p []byte) (int, error) {
	if room := c.limit - c.buf.Len(); len(p) > room {
		c.buf.Write(p[:room])
		*c.truncated = true
		c.cancel()
		return len(p), nil
	}
	return c.buf.Write(p)
}

// NeedsTerminal reports whether a failure looks like git wanting a password or key passphrase, which
// it can't ask for here.
func NeedsTerminal(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, s := range []string{"terminal prompts disabled", "could not read Username", "could not read Password",
		"Permission denied (publickey", "Host key verification failed", "Authentication failed", "No such device or address"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}
