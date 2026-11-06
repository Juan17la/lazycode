// Package github wraps the gh command for login status and creating a repository (Q-21). LazyCode
// holds no tokens: gh keeps the login, and this never reads it.
package github

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Juan17la/lazycode/internal/git"
)

var (
	// ErrMissing means gh isn't installed.
	ErrMissing = errors.New("gh isn't installed. Get it from https://cli.github.com")
	// ErrLoggedOut means gh has no login.
	ErrLoggedOut = errors.New("gh isn't logged in. Run gh auth login in a terminal tab (:term), then try again")
)

const timeout = 2 * time.Minute

// run runs gh in dir, with no questions asked and no terminal to ask on.
func run(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	git.Detach(cmd)
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1", "NO_COLOR=1", "LC_ALL=C")
	cmd.WaitDelay = 3 * time.Second
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	text := strings.TrimSpace(out.String())
	if err == nil {
		return text, nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return "", ErrMissing
	}
	msg := strings.TrimSpace(errOut.String())
	if msg == "" {
		msg = text
	}
	if msg == "" {
		msg = err.Error()
	}
	if ctx.Err() == context.DeadlineExceeded {
		msg = "gh didn't finish in " + timeout.String() + " and was stopped"
	}
	return "", errors.New(git.Clean(firstLines(msg, 3)))
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	return strings.Join(lines[:min(n, len(lines))], " ")
}

// Check reports whether gh is installed and logged in, as ErrMissing or ErrLoggedOut.
func Check(ctx context.Context) error {
	if _, err := exec.LookPath("gh"); err != nil {
		return ErrMissing
	}
	if _, err := run(ctx, "", "auth", "status"); err != nil {
		if errors.Is(err, ErrMissing) {
			return err
		}
		return ErrLoggedOut
	}
	return nil
}

// ValidName checks a repository name. The errors are phrases, so the caller makes a sentence of one.
// GitHub allows letters, digits, dots, dashes and underscores,
// and a name starting with "-" would be read as an option.
func ValidName(name string) error {
	switch {
	case name == "":
		return errors.New("a name is needed")
	case len(name) > 100:
		return errors.New("a repository name can be at most 100 characters")
	case name == "." || name == "..":
		return errors.New("that isn't a usable name")
	case strings.HasPrefix(name, "-"):
		return errors.New("a repository name can't start with -")
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_') {
			return fmt.Errorf("a repository name can have letters, digits, . - and _, but not %q", string(r))
		}
	}
	return nil
}

// CreateArgs is exactly what CreateRepo runs after "gh", so it can be shown before it does.
func CreateArgs(name string, private bool) []string {
	visibility := "--private"
	if !private {
		visibility = "--public"
	}
	return []string{"repo", "create", name, visibility, "--source=.", "--remote=origin"}
}

// CreateRepo makes the repository on GitHub from the one in dir, and adds it as origin. It doesn't push.
func CreateRepo(ctx context.Context, dir, name string, private bool) (string, error) {
	if err := ValidName(name); err != nil {
		return "", err
	}
	return run(ctx, dir, CreateArgs(name, private)...)
}
