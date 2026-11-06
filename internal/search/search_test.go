package search

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var bg = context.Background()

func isolate(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	// The temp folder must not be inside some repository of the machine running the test.
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(home))
}

func write(t testing.TB, dir, name, text string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git: %v %s", err, out)
	}
}

func TestContentSearchOffsetsAndCase(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	write(t, dir, "a.txt", "Hello world\nhello again, HELLO hello\r\nlast hello")
	write(t, dir, "sub/b.go", "no match here\n")
	res, err := Run(bg, dir, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Matches) != 5 {
		t.Fatalf("want 5 matches (case folded), got %+v", res.Matches)
	}
	content, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	for _, m := range res.Matches {
		if got := string(content[m.Start : m.Start+len("hello")]); got != m.Found {
			t.Errorf("offset %d holds %q, not %q", m.Start, got, m.Found)
		}
	}
	if m := res.Matches[2]; m.Line != 2 || m.Col != 13 || m.Found != "HELLO" || strings.Contains(m.Text, "\r") {
		t.Errorf("third match: %+v", m)
	}
	res, _ = Run(bg, dir, "Hello") // A capital makes it exact.
	if len(res.Matches) != 1 || res.Matches[0].Line != 1 {
		t.Errorf("exact search: %+v", res.Matches)
	}
	if res, _ := Run(bg, dir, ""); len(res.Matches) != 0 || len(res.Names) != 0 {
		t.Error("empty query should find nothing")
	}
	if res, _ := Run(bg, dir, "a\nb"); len(res.Matches) != 0 {
		t.Error("a query with a line break should find nothing")
	}
}

func TestSkipsBinaryIgnoredLinksAndHugeFiles(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	gitInit(t, dir)
	write(t, dir, ".gitignore", "ignored.txt\nbuild/\n")
	write(t, dir, "ok.txt", "needle")
	write(t, dir, "ignored.txt", "needle")
	write(t, dir, "build/x.txt", "needle")
	write(t, dir, "bin.dat", "needle\x00needle")
	write(t, dir, "late.dat", strings.Repeat("a", 9000)+"\x00needle") // A NUL after the 8 KB is read as text.
	outside := t.TempDir()
	write(t, outside, "secret.txt", "needle")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "link.txt")); err != nil && runtime.GOOS != "windows" {
		t.Fatal(err)
	}
	big, err := os.Create(filepath.Join(dir, "big.txt"))
	if err != nil {
		t.Fatal(err)
	}
	big.Truncate(101 << 20)
	big.Close()

	res, err := Run(bg, dir, "needle")
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, m := range res.Matches {
		paths = append(paths, m.Path)
	}
	got := strings.Join(paths, ",")
	if got != "late.dat,ok.txt" {
		t.Errorf("matched in %q, want only late.dat and ok.txt", got)
	}
	if res.Skipped < 3 { // bin.dat, big.txt, link.txt
		t.Errorf("skipped = %d", res.Skipped)
	}
}

func TestOutsideGitWalksAndSkipsDotGit(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	write(t, dir, "a.txt", "findme")
	write(t, dir, ".git/config", "findme")
	write(t, dir, "node_modules/x.js", "findme") // No .gitignore handling outside Git.
	res, err := Run(bg, dir, "findme")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Matches) != 2 {
		t.Errorf("matches: %+v", res.Matches)
	}
	for _, m := range res.Matches {
		if strings.HasPrefix(m.Path, ".git") {
			t.Errorf("searched inside .git: %s", m.Path)
		}
	}
}

func TestNameMatches(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	for _, n := range []string{"src/deep/config_loader.go", "config.go", "docs/Config.md", "other.txt", "a/b/c/d/e/f/g/config.toml"} {
		write(t, dir, n, "x")
	}
	res, _ := Run(bg, dir, "CONFIG")
	if len(res.Names) != 4 || res.Names[0] != "config.go" {
		t.Errorf("names = %v", res.Names)
	}
	res, _ = Run(bg, dir, "deep/conf") // Matches the path, not just the name.
	if len(res.Names) != 1 {
		t.Errorf("path match: %v", res.Names)
	}
}

func TestAwkwardNames(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	gitInit(t, dir)
	names := []string{"sp ace.txt", "new\nline.txt", "-dash.txt", "ünï.txt"}
	if runtime.GOOS == "linux" {
		names = append(names, "bad\xffname.txt")
	}
	var made []string
	for _, n := range names {
		if os.WriteFile(filepath.Join(dir, n), []byte("needle"), 0o644) == nil {
			made = append(made, n)
		}
	}
	res, err := Run(bg, dir, "needle")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Matches) != len(made) {
		t.Errorf("found %d of %d files: %+v", len(res.Matches), len(made), res.Matches)
	}
}

func TestResultsAreCappedInOrder(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	line := strings.Repeat("x\n", 2000)
	for i := range 5 {
		write(t, dir, fmt.Sprintf("f%d.txt", i), line)
	}
	res, err := Run(bg, dir, "x")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Capped || len(res.Matches) != MaxResults {
		t.Fatalf("capped=%v len=%d", res.Capped, len(res.Matches))
	}
	if res.Matches[0].Path != "f0.txt" || res.Matches[MaxResults-1].Path != "f2.txt" {
		t.Errorf("the cap should keep the first files in order: %s .. %s", res.Matches[0].Path, res.Matches[MaxResults-1].Path)
	}
}

func TestCancelStopsTheSearch(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	for i := range 300 {
		write(t, dir, fmt.Sprintf("d%d/f.txt", i%10+i), strings.Repeat("abc\n", 20000))
	}
	ctx, cancel := context.WithCancel(bg)
	cancel()
	if _, err := Run(ctx, dir, "zzz"); err == nil {
		t.Error("a cancelled search should say so")
	}
	ctx, cancel = context.WithCancel(bg)
	errc := make(chan error, 1)
	go func() { _, err := Run(ctx, dir, "zzz"); errc <- err }()
	cancel()
	select {
	case <-errc:
	case <-time.After(5 * time.Second):
		t.Fatal("the search kept running after it was cancelled")
	}
}

func TestUnreadableFileIsSkipped(t *testing.T) {
	isolate(t)
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permissions can't be taken away here")
	}
	dir := t.TempDir()
	write(t, dir, "ok.txt", "needle")
	write(t, dir, "locked.txt", "needle")
	os.Chmod(filepath.Join(dir, "locked.txt"), 0)
	res, err := Run(bg, dir, "needle")
	if err != nil || len(res.Matches) != 1 || res.Skipped != 1 {
		t.Errorf("%+v %v", res, err)
	}
}

// BenchmarkSearch50000Files is the budget's "search a 50,000-file directory" (doc 12).
func BenchmarkSearch50000Files(b *testing.B) {
	dir := b.TempDir()
	home := b.TempDir()
	b.Setenv("HOME", home)
	b.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	b.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	b.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(home))
	body := strings.Repeat("func example() { return value + 1 }\n", 60) // About 2 KB.
	for i := range 50000 {
		sub := filepath.Join(dir, fmt.Sprintf("pkg%03d", i%500), fmt.Sprintf("sub%d", i%7))
		if i < 500 {
			os.MkdirAll(sub, 0o755)
		}
		text := body
		if i%1000 == 0 {
			text += "// needle here\n"
		}
		write(b, sub, fmt.Sprintf("file%d.go", i), text)
	}
	for _, mode := range []string{"walk", "git"} {
		if mode == "git" {
			if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
				b.Skipf("git: %v %s", err, out)
			}
		}
		b.Run(mode, func(b *testing.B) {
			for b.Loop() {
				res, err := Run(bg, dir, "needle")
				if err != nil || len(res.Matches) != 50 || res.Files != 50000 {
					b.Fatalf("%v: %d matches in %d files", err, len(res.Matches), res.Files)
				}
			}
		})
	}
}
