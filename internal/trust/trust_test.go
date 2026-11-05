package trust

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrust(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, `odd "name"`)
	inner := filepath.Join(dir, "inner")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	s := New(filepath.Join(root, "config", "trust.toml"))
	if s.Trusted(dir) {
		t.Fatal("trusted by default")
	}
	if err := s.Add(dir); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(dir); err != nil {
		t.Fatal(err)
	}
	if !s.Trusted(dir) || !s.Trusted(dir+string(filepath.Separator)) {
		t.Fatal("not trusted after adding")
	}
	if s.Trusted(inner) || s.Trusted(root) {
		t.Fatal("trust spread to another folder")
	}
	if !New(s.path).Trusted(dir) {
		t.Fatal("not saved")
	}
	if err := s.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if s.Trusted(dir) {
		t.Fatal("still trusted")
	}
	if err := s.Remove(dir); err != nil {
		t.Fatalf("removing twice: %v", err)
	}
}

func TestSymlinkMeansItsTarget(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "real")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Skip("no symlinks here:", err)
	}
	s := New(filepath.Join(root, "trust.toml"))
	if err := s.Add(link); err != nil {
		t.Fatal(err)
	}
	if !s.Trusted(dir) {
		t.Fatal("the target isn't trusted")
	}
}

func TestBrokenFileMeansUntrustedAndIsKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trust.toml")
	if err := os.WriteFile(path, []byte("directories = [broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(path)
	if s.Trusted(t.TempDir()) {
		t.Fatal("trusted through a broken file")
	}
	if err := s.Add(t.TempDir()); err == nil {
		t.Fatal("no error")
	}
	if got, _ := os.ReadFile(path); string(got) != "directories = [broken" {
		t.Fatalf("replaced: %q", got)
	}
}

func TestAnswersAreRemembered(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(t.TempDir(), "trust.toml"))
	if !s.Undecided(dir) {
		t.Fatal("a new folder should be undecided")
	}
	if err := s.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if s.Undecided(dir) || s.Trusted(dir) {
		t.Fatal("no should be remembered, and shouldn't trust")
	}
	if err := s.Add(dir); err != nil {
		t.Fatal(err)
	}
	if s.Undecided(dir) || !s.Trusted(dir) {
		t.Fatal("yes should replace no")
	}
	if err := s.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if s.Undecided(dir) || s.Trusted(dir) {
		t.Fatal(":untrust shouldn't bring the question back")
	}
}

func TestBrokenFileAsksNothingOnOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trust.toml")
	if err := os.WriteFile(path, []byte("not toml ["), 0o644); err != nil {
		t.Fatal(err)
	}
	if New(path).Undecided(t.TempDir()) {
		t.Fatal("an answer couldn't be saved, so it shouldn't ask")
	}
}
