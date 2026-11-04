// Package clipboard talks to the system clipboard through the OS's own tools. They can
// block, so callers shouldn't use it on the UI goroutine.
//
// Windows has no entry here: Windows Terminal delivers Ctrl+V as a paste event by
// itself, and accepts clipboard writes from the terminal escape code that the UI falls
// back to.
package clipboard

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const timeout = 2 * time.Second

// ErrUnavailable means no clipboard tool was found.
var ErrUnavailable = errors.New("no clipboard tool found")

type tool struct{ copy, paste []string }

func tools() []tool {
	switch {
	case runtime.GOOS == "darwin":
		return []tool{{[]string{"pbcopy"}, []string{"pbpaste"}}}
	case runtime.GOOS == "windows":
		return nil
	}
	var found []tool
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		found = append(found, tool{[]string{"wl-copy"}, []string{"wl-paste", "--no-newline"}})
	}
	if os.Getenv("DISPLAY") != "" {
		found = append(found,
			tool{[]string{"xclip", "-selection", "clipboard", "-in"}, []string{"xclip", "-selection", "clipboard", "-out"}},
			tool{[]string{"xsel", "--clipboard", "--input"}, []string{"xsel", "--clipboard", "--output"}})
	}
	return found
}

// Write puts text on the clipboard.
func Write(text string) error {
	return run(func(t tool) []string { return t.copy }, func(cmd *exec.Cmd) error {
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run() // Output stays unconnected: xclip keeps running to serve the text.
	})
}

// Read returns the clipboard's text.
func Read() (string, error) {
	var text string
	err := run(func(t tool) []string { return t.paste }, func(cmd *exec.Cmd) error {
		out, err := cmd.Output()
		text = string(out)
		return err
	})
	return text, err
}

// run uses the first tool that's installed.
func run(args func(tool) []string, start func(*exec.Cmd) error) error {
	for _, t := range tools() {
		command := args(t)
		path, err := exec.LookPath(command[0])
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return start(exec.CommandContext(ctx, path, command[1:]...))
	}
	return ErrUnavailable
}
