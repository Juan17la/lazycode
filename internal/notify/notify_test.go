//go:build linux

package notify

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSendPassesTextAsData(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "args")
	script := "#!/bin/sh\nfor arg in \"$@\"; do printf '%s\\n' \"$arg\"; done > '" + out + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "notify-send"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DISPLAY", ":0")
	if err := Send("-title; rm -rf /", "body\x1b[31m\nsecond $(touch x)"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	want := "--app-name=LazyCode\n--\n-title; rm -rf /\nbody [31m second $(touch x)\n"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestUnavailable(t *testing.T) {
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	if err := Send("a", "b"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no display: %v", err)
	}
	t.Setenv("DISPLAY", ":0")
	t.Setenv("PATH", t.TempDir())
	if err := Send("a", "b"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no tool: %v", err)
	}
}
