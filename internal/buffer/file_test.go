package buffer

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func writeFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t testing.TB, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func openNew(t *testing.T, content string) *File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "file.txt")
	writeFile(t, path, content)
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func change(t *testing.T, f *File, s string) {
	t.Helper()
	edit(t, f.Buffer, Edit{0, 0, s})
}

// onlyFile fails if a save left anything else in the folder.
func onlyFile(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("folder holds %v, want only %s", entries, filepath.Base(path))
	}
}

func TestSaveKeepsEveryOtherByte(t *testing.T) {
	original := "one\r\ntwo\nthree\rfour\xff\xfe\n\n"
	f := openNew(t, original)
	if f.Modified() {
		t.Fatal("a newly opened file counts as modified")
	}
	change(t, f, "new ")
	if !f.Modified() {
		t.Fatal("an edit doesn't count as modified")
	}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, f.Path); got != "new "+original {
		t.Fatalf("saved %q", got)
	}
	if f.Modified() {
		t.Fatal("still modified after saving")
	}
	onlyFile(t, f.Path)

	// Text saved earlier can be edited and saved again.
	change(t, f, "x")
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, f.Path); got != "xnew "+original {
		t.Fatalf("second save: %q", got)
	}
}

func TestUndoBackToSavedIsNotModified(t *testing.T) {
	f := openNew(t, "text")
	change(t, f, "a")
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	change(t, f, "b")
	f.Undo()
	if f.Modified() {
		t.Fatal("undoing to the saved text still counts as modified")
	}
	f.Undo()
	if !f.Modified() {
		t.Fatal("undoing past the saved text doesn't count as modified")
	}
}

func TestSaveWithoutEditsLeavesTheFileAlone(t *testing.T) {
	f := openNew(t, "text")
	before, err := os.Stat(f.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(f.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("the file was replaced with nothing to save")
	}
}

func TestExternalChangesAreNeverOverwritten(t *testing.T) {
	for name, toolWrites := range map[string]string{
		"different size": "the tool wrote this",
		"same size":      "TEXT",
		"emptied":        "",
	} {
		t.Run(name, func(t *testing.T) {
			f := openNew(t, "text")
			change(t, f, "mine ")
			writeFile(t, f.Path, toolWrites)

			if err := f.Save(); !errors.Is(err, ErrChangedOnDisk) {
				t.Fatalf("Save = %v", err)
			}
			if got := readFile(t, f.Path); got != toolWrites {
				t.Fatalf("the tool's text was replaced by %q", got)
			}
			if got := text(f.Snapshot); got != "mine text" || !f.Modified() {
				t.Fatalf("edits lost: %q, modified %v", got, f.Modified())
			}
			onlyFile(t, f.Path)

			if err := f.Overwrite(); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, f.Path); got != "mine text" {
				t.Fatalf("Overwrite wrote %q", got)
			}
			if err := f.Save(); err != nil {
				t.Fatalf("Save after Overwrite: %v", err)
			}
		})
	}
}

func TestToolReplacingTheFileIsDetected(t *testing.T) {
	f := openNew(t, "text")
	change(t, f, "mine ")
	replacement := f.Path + ".new" // How most tools save: write a new file, then rename.
	writeFile(t, replacement, "tool")
	if err := os.Rename(replacement, f.Path); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(); !errors.Is(err, ErrChangedOnDisk) {
		t.Fatalf("Save = %v", err)
	}
}

func TestChangeWithTheSameSizeAndTimeIsDetected(t *testing.T) {
	f := openNew(t, "text")
	info, err := os.Stat(f.Path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, f.Path, "TEXT")
	if err := os.Chtimes(f.Path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if changed, err := f.ChangedOnDisk(); err != nil || !changed {
		t.Fatalf("ChangedOnDisk = %v, %v", changed, err)
	}
}

func TestATouchIsNotAChange(t *testing.T) {
	f := openNew(t, "text")
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(f.Path, later, later); err != nil {
		t.Fatal(err)
	}
	if changed, err := f.ChangedOnDisk(); err != nil || changed {
		t.Fatalf("ChangedOnDisk = %v, %v", changed, err)
	}
}

func TestSavingAfterTheFileWasDeleted(t *testing.T) {
	f := openNew(t, "text")
	change(t, f, "mine ")
	if err := os.Remove(f.Path); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(); !errors.Is(err, ErrChangedOnDisk) {
		t.Fatalf("Save = %v", err)
	}
	if err := f.Overwrite(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, f.Path); got != "mine text" {
		t.Fatalf("got %q", got)
	}
}

func TestReload(t *testing.T) {
	f := openNew(t, "text")
	change(t, f, "mine ")
	writeFile(t, f.Path, "from the tool\n")
	if err := f.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := text(f.Snapshot); got != "from the tool\n" {
		t.Fatalf("got %q", got)
	}
	if f.Modified() || f.Undo() {
		t.Fatal("reloading should leave a clean file with no undo history")
	}
	if changed, err := f.ChangedOnDisk(); err != nil || changed {
		t.Fatalf("ChangedOnDisk = %v, %v", changed, err)
	}

	writeFile(t, f.Path, "now\x00binary")
	change(t, f, "kept ")
	if err := f.Reload(); !errors.Is(err, ErrBinary) {
		t.Fatalf("Reload = %v", err)
	}
	if got := text(f.Snapshot); got != "kept from the tool\n" || !f.Modified() {
		t.Fatalf("a failed reload changed the text to %q", got)
	}
}

func TestCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.txt")
	f, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := f.ChangedOnDisk(); err != nil || changed {
		t.Fatalf("ChangedOnDisk = %v, %v", changed, err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("Create wrote the file")
	}
	if err := f.Save(); err != nil { // An empty new file can still be saved.
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "" {
		t.Fatalf("got %q", got)
	}
	change(t, f, "hello")
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "hello" {
		t.Fatalf("got %q", got)
	}

	other, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	change(t, other, "clash")
	if err := other.Save(); !errors.Is(err, ErrChangedOnDisk) {
		t.Fatalf("Create didn't protect an existing file: %v", err)
	}
}

func TestBinaryDetection(t *testing.T) {
	dir := t.TempDir()
	for name, tc := range map[string]struct {
		content string
		binary  bool
	}{
		"NUL at start":      {"\x00abc", true},
		"NUL in first 8 KB": {strings.Repeat("a", binarySniff-1) + "\x00", true},
		"NUL after 8 KB":    {strings.Repeat("a", binarySniff) + "\x00", false},
		"UTF-16":            {"\xff\xfeh\x00i\x00", true},
		"invalid UTF-8":     {"\xff\xfe\xc3", false},
		"empty":             {"", false},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, "file")
			writeFile(t, path, tc.content)
			f, err := Open(path)
			if tc.binary {
				if !errors.Is(err, ErrBinary) {
					t.Fatalf("Open = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := text(f.Snapshot); got != tc.content {
				t.Fatalf("got %q", got)
			}
		})
	}
}

func TestOpenRefusals(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(dir); !errors.Is(err, ErrNotRegular) {
		t.Errorf("a directory: %v", err)
	}
	if _, err := Open(filepath.Join(dir, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a missing file: %v", err)
	}
}

func sparseFile(t *testing.T, size int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "big.log")
	// Enough real text that the NUL bytes of the sparse part aren't taken for binary.
	writeFile(t, path, "first line\nsecond line\n"+strings.Repeat("text\n", binarySniff/5+1))
	if err := os.Truncate(path, size); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFilesOverTheLimitAreReadOnly(t *testing.T) {
	path := sparseFile(t, readOnlyAbove+1)
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if !f.ReadOnly() {
		t.Fatal("not read-only")
	}
	if f.Len() != readOnlyAbove+1 || string(f.Line(1)) != "second line" {
		t.Fatalf("Len %d, Line(1) %q", f.Len(), f.Line(1))
	}
	if err := f.Apply(Edit{0, 0, "x"}); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Apply = %v", err)
	}
	if err := f.Save(); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Save = %v", err)
	}
	if err := f.Overwrite(); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Overwrite = %v", err)
	}
	if f.Modified() {
		t.Fatal("counts as modified")
	}
	if changed, err := f.ChangedOnDisk(); err != nil || changed {
		t.Fatalf("ChangedOnDisk = %v, %v", changed, err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if f.Len() != 0 || !f.ReadOnly() || f.Modified() {
		t.Fatal("a closed file should read as empty")
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing twice: %v", err)
	}
}

func TestFileAtTheLimitIsEditable(t *testing.T) {
	f, err := Open(sparseFile(t, readOnlyAbove))
	if err != nil {
		t.Fatal(err)
	}
	if f.ReadOnly() {
		t.Fatal("read-only at exactly the limit")
	}
}

func TestPermissionsAreKept(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix permission bits")
	}
	for _, perm := range []os.FileMode{0o755, 0o640, 0o600} {
		f := openNew(t, "text")
		if err := os.Chmod(f.Path, perm); err != nil {
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
		if info.Mode().Perm() != perm {
			t.Errorf("mode %o became %o", perm, info.Mode().Perm())
		}
	}
}

func TestSymlinkStaysASymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")
	writeFile(t, target, "text")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks aren't available here:", err)
	}
	f, err := Open(link)
	if err != nil {
		t.Fatal(err)
	}
	change(t, f, "x")
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink was replaced by a file")
	}
	if got := readFile(t, target); got != "xtext" {
		t.Fatalf("the target holds %q", got)
	}
}

func TestReadOnlyFileIsNotReplaced(t *testing.T) {
	f := openNew(t, "text")
	if err := os.Chmod(f.Path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(f.Path, 0o644) }) //nolint:errcheck // Lets Windows delete it.
	if isRoot() {
		t.Skip("root can write to anything")
	}
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
	onlyFile(t, f.Path)
}

func TestSnapshotWriteTo(t *testing.T) {
	b := New([]byte("hello world\n"))
	edit(t, b, Edit{0, 5, "goodbye"}, Edit{6, 6, "big "})
	var out bytes.Buffer
	n, err := b.WriteTo(&out)
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != text(b.Snapshot) || n != int64(b.Len()) {
		t.Fatalf("wrote %q, %d", out.String(), n)
	}
}

func hundredMBFile(b *testing.B) string {
	b.Helper()
	path := filepath.Join(b.TempDir(), "big.txt")
	if err := os.WriteFile(path, largeFile(), 0o644); err != nil {
		b.Fatal(err)
	}
	return path
}

// bytes/op is the file itself, plus what it costs to hold.
func BenchmarkOpenFile100MB(b *testing.B) {
	path := hundredMBFile(b)
	b.ResetTimer()
	for b.Loop() {
		if _, err := Open(path); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSaveFile100MB(b *testing.B) {
	f, err := Open(hundredMBFile(b))
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if err := f.Apply(Edit{0, 0, "x"}); err != nil {
			b.Fatal(err)
		}
		if err := f.Save(); err != nil {
			b.Fatal(err)
		}
	}
}
