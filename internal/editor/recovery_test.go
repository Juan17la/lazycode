package editor

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Juan17la/lazycode/internal/buffer"
)

func newRecovery(t *testing.T) *Recovery {
	t.Helper()
	r := NewRecovery(filepath.Join(t.TempDir(), "recovery"), func(path string, err error) {
		t.Errorf("recovery of %s failed: %v", path, err)
	})
	r.delay = 10 * time.Millisecond
	return r
}

func snapshot(text string) buffer.Snapshot { return buffer.New([]byte(text)).Snapshot }

// deadPid returns the ID of a process that has exited.
func deadPid(t *testing.T) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.ProcessState.Pid()
}

// crash writes a recovery file as a process that has since died would have left it.
func crash(t *testing.T, r *Recovery, path, text string) {
	t.Helper()
	if err := r.write(path, snapshot(text), deadPid(t)); err != nil {
		t.Fatal(err)
	}
}

func onDisk(r *Recovery, path string) string {
	data, err := os.ReadFile(r.fileFor(path))
	if err != nil {
		return ""
	}
	_, body, _ := strings.Cut(string(data), "\n")
	return body
}

func TestScheduleWritesAfterTheDelay(t *testing.T) {
	r := newRecovery(t)
	r.delay = time.Hour
	r.Schedule("/a.txt", snapshot("text"))
	if onDisk(r, "/a.txt") != "" {
		t.Fatal("wrote before the delay")
	}
	r.Discard("/a.txt")
	r.Wait()

	r.delay = 10 * time.Millisecond
	r.Schedule("/a.txt", snapshot("text"))
	r.Wait()
	if got := onDisk(r, "/a.txt"); got != "text" {
		t.Fatalf("got %q", got)
	}
}

func TestOnlyTheLastEditIsWritten(t *testing.T) {
	r := newRecovery(t)
	r.delay = 50 * time.Millisecond
	for _, s := range []string{"a", "ab", "abc", "abcd"} {
		r.Schedule("/a.txt", snapshot(s))
	}
	r.Wait()
	if got := onDisk(r, "/a.txt"); got != "abcd" {
		t.Fatalf("got %q", got)
	}
}

func TestBytesSurviveExactly(t *testing.T) {
	r := newRecovery(t)
	content := "line\r\nbinary-ish \xff\xfe\x00 and\nmore\n"
	path := "/odd name/with\nnewline \"quotes\" é.txt"
	crash(t, r, path, content)
	found, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Path != path {
		t.Fatalf("found %+v", found)
	}
	got, err := found[0].Read()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("got %q", got)
	}
}

func TestVeryLongPath(t *testing.T) {
	r := newRecovery(t)
	path := "/" + strings.Repeat("long/", 2000) + "file.txt"
	crash(t, r, path, "text")
	found, err := r.List()
	if err != nil || len(found) != 1 || found[0].Path != path {
		t.Fatalf("found %d, %v", len(found), err)
	}
	if got, err := found[0].Read(); err != nil || string(got) != "text" {
		t.Fatalf("Read = %q, %v", got, err)
	}
}

func TestPrivateToTheUser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix permission bits: the user's profile folder protects it")
	}
	r := newRecovery(t)
	if err := os.MkdirAll(r.dir, 0o755); err != nil { // Already there, and too open.
		t.Fatal(err)
	}
	crash(t, r, "/secret.env", "PASSWORD=x")
	dir, err := os.Stat(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Stat(r.fileFor("/secret.env"))
	if err != nil {
		t.Fatal(err)
	}
	if dir.Mode().Perm() != 0o700 || file.Mode().Perm() != 0o600 {
		t.Fatalf("folder %o, file %o", dir.Mode().Perm(), file.Mode().Perm())
	}
}

func TestDiscard(t *testing.T) {
	r := newRecovery(t)
	r.Schedule("/a.txt", snapshot("text"))
	r.Wait()

	r.delay = time.Hour
	r.Schedule("/a.txt", snapshot("newer")) // Waiting when the discard arrives.
	r.Discard("/a.txt")
	r.Wait()
	if _, err := os.Stat(r.fileFor("/a.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("still there: %v", err)
	}
	r.Discard("/never-existed.txt")
	r.Wait()
}

// A slow write must not bring a discarded file back, and a discard must not remove
// what newer edits wrote.
func TestLatestRequestWins(t *testing.T) {
	r := newRecovery(t)
	r.delay = 0
	for range 50 {
		r.Schedule("/a.txt", snapshot("old"))
		r.Discard("/a.txt")
		r.Wait()
		if _, err := os.Stat(r.fileFor("/a.txt")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("a discarded file came back")
		}

		r.Discard("/a.txt")
		r.Schedule("/a.txt", snapshot("new"))
		r.Wait()
		if got := onDisk(r, "/a.txt"); got != "new" {
			t.Fatalf("newer edits were lost: %q", got)
		}
		r.Discard("/a.txt")
		r.Wait()
	}
}

func TestFilesOfRunningProcessesAreNotOffered(t *testing.T) {
	r := newRecovery(t)
	r.Schedule("/mine.txt", snapshot("mine")) // Written with this process's ID.
	r.Wait()
	crash(t, r, "/crashed.txt", "theirs")
	found, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Path != "/crashed.txt" {
		t.Fatalf("found %+v", found)
	}
}

func TestListWithNoFolder(t *testing.T) {
	found, err := newRecovery(t).List()
	if err != nil || found != nil {
		t.Fatalf("found %v, %v", found, err)
	}
}

func TestListIsOldestFirst(t *testing.T) {
	r := newRecovery(t)
	for _, name := range []string{"/1", "/2", "/3", "/4"} {
		crash(t, r, name, name)
		time.Sleep(2 * time.Millisecond)
	}
	found, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"/1", "/2", "/3", "/4"} {
		if found[i].Path != name {
			t.Fatalf("order: %+v", found)
		}
	}
}

func TestBrokenFilesAreReportedAndKept(t *testing.T) {
	r := newRecovery(t)
	crash(t, r, "/good.txt", "good")
	for name, content := range map[string]string{
		"noheader" + recoveryExt: "just text with no line break",
		"badjson" + recoveryExt:  "{not json\nbody",
		"future" + recoveryExt:   `{"Version":99,"Path":"/x"}` + "\nbody",
		"empty" + recoveryExt:    "",
	} {
		if err := os.WriteFile(filepath.Join(r.dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	found, err := r.List()
	if err == nil {
		t.Fatal("no error for broken files")
	}
	if len(found) != 1 || found[0].Path != "/good.txt" {
		t.Fatalf("found %+v", found)
	}
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatalf("%d files left, want all 5", len(entries))
	}
}

func TestOldLeftoverWritesAreCleanedUp(t *testing.T) {
	r := newRecovery(t)
	crash(t, r, "/a.txt", "text")
	old := filepath.Join(r.dir, "old.tmp")
	recent := filepath.Join(r.dir, "recent.tmp") // Possibly another instance writing.
	for _, path := range []string{old, recent} {
		if err := os.WriteFile(path, []byte("partial"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	long := time.Now().Add(-2 * staleTemp)
	if err := os.Chtimes(old, long, long); err != nil {
		t.Fatal(err)
	}
	if _, err := r.List(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the old leftover is still there")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatal("a recent write was removed")
	}
}

func TestFailedWriteIsReported(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file-not-folder")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var reported string
	r := NewRecovery(blocked, func(path string, err error) {
		mu.Lock()
		defer mu.Unlock()
		reported = path
	})
	r.delay = 0
	r.Schedule("/a.txt", snapshot("text"))
	r.Wait()
	mu.Lock()
	defer mu.Unlock()
	if reported != "/a.txt" {
		t.Fatalf("reported %q", reported)
	}
}

// The whole story: edit, crash, restart, restore, undo, save.
func TestRecoveringAfterACrash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "code.txt")
	if err := os.WriteFile(path, []byte("line one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := buffer.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Apply(buffer.Edit{Start: 9, End: 9, Text: "unsaved work\n"}); err != nil {
		t.Fatal(err)
	}
	r := newRecovery(t)
	crash(t, r, f.Path, string(f.Slice(0, f.Len())))
	if err := os.WriteFile(path, []byte("a tool changed this\n"), 0o644); err != nil { // During the outage.
		t.Fatal(err)
	}

	after := NewRecovery(r.dir, nil)
	found, err := after.List()
	if err != nil || len(found) != 1 {
		t.Fatalf("found %+v, %v", found, err)
	}
	saved, err := found[0].Read()
	if err != nil {
		t.Fatal(err)
	}
	f, err = buffer.Open(found[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Apply(buffer.Edit{Start: 0, End: f.Len(), Text: string(saved)}); err != nil {
		t.Fatal(err)
	}
	if got := string(f.Slice(0, f.Len())); got != "line one\nunsaved work\n" || !f.Modified() {
		t.Fatalf("restored %q, modified %v", got, f.Modified())
	}
	if got, _ := os.ReadFile(path); string(got) != "a tool changed this\n" {
		t.Fatalf("restoring wrote to the file: %q", got)
	}
	if !f.Undo() || string(f.Slice(0, f.Len())) != "a tool changed this\n" {
		t.Fatal("undo should go back to what's on disk")
	}
	if !f.Redo() {
		t.Fatal("redo")
	}

	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	after.Discard(f.Path)
	after.Wait()
	if found, err := after.List(); err != nil || len(found) != 0 {
		t.Fatalf("after saving: %+v, %v", found, err)
	}
}

func BenchmarkWrite100MB(b *testing.B) {
	r := NewRecovery(b.TempDir(), nil)
	text := buffer.New([]byte(strings.Repeat("the quick brown fox jumps over the lazy dog, line\n", 100<<20/50))).Snapshot
	b.ResetTimer()
	for b.Loop() {
		if err := r.write("/big.txt", text, os.Getpid()); err != nil {
			b.Fatal(err)
		}
	}
}
