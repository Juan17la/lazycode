//go:build unix

package buffer

import (
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func isRoot() bool { return os.Geteuid() == 0 }

func TestPipeIsRefusedWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skip("no pipes here:", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := Open(path)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrNotRegular) {
			t.Fatalf("Open = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Open blocked on a pipe")
	}
}

func TestUnwritableFolderLeavesTheFileAlone(t *testing.T) {
	if isRoot() {
		t.Skip("root can write anywhere")
	}
	f := openNew(t, "text")
	dir := filepath.Dir(f.Path)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) }) //nolint:errcheck // Lets the test clean up.
	change(t, f, "x")
	if err := f.Save(); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Save = %v", err)
	}
	if got := readFile(t, f.Path); got != "text" {
		t.Fatalf("file holds %q", got)
	}
	if !f.Modified() {
		t.Fatal("a failed save cleared Modified")
	}
}

// A real failed write from the OS, as a full disk gives: the file size limit stops
// the write part way through.
func TestFailedWriteLeavesTheFileAlone(t *testing.T) {
	original := strings.Repeat("original\n", 100)
	f := openNew(t, original)
	change(t, f, strings.Repeat("new text\n", 100_000))

	signal.Ignore(syscall.SIGXFSZ)
	defer signal.Reset(syscall.SIGXFSZ)
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(rlimitFileSize, &limit); err != nil {
		t.Fatal(err)
	}
	small := limit
	small.Cur = 4096
	if err := syscall.Setrlimit(rlimitFileSize, &small); err != nil {
		t.Skip("can't limit file size:", err)
	}
	err := f.Save()
	if restore := syscall.Setrlimit(rlimitFileSize, &limit); restore != nil {
		t.Fatal(restore)
	}

	if err == nil {
		t.Fatal("Save succeeded past the limit")
	}
	if got := readFile(t, f.Path); got != original {
		t.Fatalf("the original was damaged: %d bytes", len(got))
	}
	if !f.Modified() {
		t.Fatal("a failed save cleared Modified")
	}
	onlyFile(t, f.Path)

	if err := f.Save(); err != nil {
		t.Fatalf("saving once there's room: %v", err)
	}
	if !strings.HasPrefix(readFile(t, f.Path), "new text\n") {
		t.Fatal("the retried save didn't write the edits")
	}
}

const rlimitFileSize = 1 // RLIMIT_FSIZE on Linux and macOS.

func TestNewFileFollowsTheUmask(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	f, err := Create(filepath.Join(t.TempDir(), "secret.txt"))
	if err != nil {
		t.Fatal(err)
	}
	change(t, f, "x")
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(f.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o, want 600", info.Mode().Perm())
	}
}
