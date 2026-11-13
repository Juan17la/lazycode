//go:build linux

package clipboard

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// fakeTool stands in for xclip: "-in" stores stdin in a file, and "-out" prints it.
func fakeTool(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	store := filepath.Join(dir, "store")
	script := "#!/bin/sh\ncase \"$3\" in\n-in) cat > '" + store + "' ;;\n-out) cat '" + store + "' ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "xclip"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestWriteThenRead(t *testing.T) {
	t.Setenv("PATH", fakeTool(t)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":0")
	text := "line one\r\nüñí 世界 😀\n\ttabbed"
	if err := Write(text); err != nil {
		t.Fatal(err)
	}
	got, err := Read()
	if err != nil {
		t.Fatal(err)
	}
	if got != text {
		t.Fatalf("got %q", got)
	}
}

func TestNoDisplayMeansUnavailable(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
	if err := Write("x"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Write = %v", err)
	}
	if _, err := Read(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Read = %v", err)
	}
}

func TestDisplayWithoutToolsMeansUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("DISPLAY", ":0")
	if err := Write("x"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Write = %v", err)
	}
}
