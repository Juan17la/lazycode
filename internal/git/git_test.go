package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var bg = context.Background()

// isolate keeps every test away from the user's own Git settings and from the network.
func isolate(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "T")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "T")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	ResetForTest()
	if !Installed() {
		t.Skip("git isn't installed")
	}
}

// sh runs git for setup, which is the one place tests may write.
func sh(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func put(t *testing.T, dir, name, text string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRepo makes a repository on branch main.
func newRepo(t *testing.T) string {
	t.Helper()
	isolate(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sh(t, dir, "init", "-q", "-b", "main")
	return dir
}

func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	sh(t, dir, "add", "-A")
	sh(t, dir, "commit", "-q", "-m", msg)
}

func find(st *Status, path string, kind Kind) *File {
	for i, f := range st.Files {
		if f.Path == path && f.Kind == kind {
			return &st.Files[i]
		}
	}
	return nil
}

func TestStatusKinds(t *testing.T) {
	dir := newRepo(t)
	put(t, dir, "a.txt", "one\n")
	put(t, dir, "b.txt", "two\n")
	put(t, dir, "gone.txt", "x\n")
	commitAll(t, dir, "first")
	put(t, dir, "a.txt", "one changed\n") // Unstaged.
	put(t, dir, "b.txt", "two changed\n") // Staged, then changed again.
	sh(t, dir, "add", "b.txt")
	put(t, dir, "b.txt", "two changed twice\n")
	put(t, dir, "new.txt", "n\n")            // Untracked.
	sh(t, dir, "rm", "-q", "gone.txt")       // Staged deletion.
	sh(t, dir, "mv", "a.txt", "renamed.txt") // Staged rename, with an unstaged change.

	st, err := ReadStatus(bg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Branch != "main" || st.Detached || !st.HasCommits || st.Root != dir {
		t.Fatalf("branch info wrong: %+v", st)
	}
	if f := find(st, "new.txt", Untracked); f == nil || f.Marker() != '?' {
		t.Error("new.txt should be untracked")
	}
	if f := find(st, "gone.txt", Staged); f == nil || f.Marker() != 'D' {
		t.Error("gone.txt should be a staged deletion")
	}
	if f := find(st, "renamed.txt", Staged); f == nil || f.Orig != "a.txt" || f.Marker() != 'R' {
		t.Errorf("rename not found: %+v", st.Files)
	}
	if find(st, "b.txt", Staged) == nil || find(st, "b.txt", Unstaged) == nil {
		t.Error("b.txt changed both ways should be in both lists")
	}
}

func TestStatusEmptyRepoAndDetached(t *testing.T) {
	dir := newRepo(t)
	put(t, dir, "x", "1")
	st, err := ReadStatus(bg, dir)
	if err != nil || st.HasCommits || st.Head != "" || st.Branch != "main" {
		t.Fatalf("empty repo: %+v, %v", st, err)
	}
	if find(st, "x", Untracked) == nil {
		t.Error("x should be untracked")
	}
	if log, err := Log(bg, dir, 0, 10); err != nil || len(log) != 0 {
		t.Fatalf("log of an empty repo should be empty, got %v, %v", log, err)
	}
	commitAll(t, dir, "one")
	sh(t, dir, "checkout", "-q", "--detach")
	st, err = ReadStatus(bg, dir)
	if err != nil || !st.Detached || st.Branch != "" || !st.HasCommits {
		t.Fatalf("detached: %+v, %v", st, err)
	}
}

func TestStatusAwkwardNames(t *testing.T) {
	dir := newRepo(t)
	names := []string{"with space.txt", "-dash.txt", "new\nline.txt", ":(top)magic.txt", "*.glob", "ünï.txt"}
	if runtime.GOOS != "windows" {
		names = append(names, "bad\xffname.txt")
	}
	var made []string
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err == nil {
			made = append(made, n)
		}
	}
	st, err := ReadStatus(bg, dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range made {
		if find(st, n, Untracked) == nil {
			t.Errorf("%q missing from %v", n, st.Files)
		}
	}
	// Stage only one of them: --literal-pathspecs must stop "*.glob" and ":(top)" meaning more.
	if err := Stage(bg, dir, "*.glob"); err != nil {
		t.Fatal(err)
	}
	st, _ = ReadStatus(bg, dir)
	for _, f := range st.Files {
		if f.Kind == Staged && f.Path != "*.glob" {
			t.Errorf("%q was staged too", f.Path)
		}
	}
	if find(st, "*.glob", Staged) == nil {
		t.Error("*.glob not staged")
	}
	if err := Unstage(bg, dir, false, "*.glob"); err != nil {
		t.Fatal(err)
	}
	if st, _ = ReadStatus(bg, dir); find(st, "*.glob", Staged) != nil {
		t.Error("unstage on a repo with no commits didn't work")
	}
	// Names are shown cleaned.
	if got := Clean("new\nline\x1b[31m.txt\u202e"); got != "new line[31m.txt" {
		t.Errorf("Clean = %q", got)
	}
}

func TestLogGraphPagesAndRefs(t *testing.T) {
	dir := newRepo(t)
	for _, m := range []string{"one", "two", "three"} {
		put(t, dir, m, m)
		commitAll(t, dir, "commit "+m)
	}
	sh(t, dir, "switch", "-q", "-c", "side")
	put(t, dir, "s", "s")
	commitAll(t, dir, "on side")
	sh(t, dir, "switch", "-q", "main")
	put(t, dir, "m", "m")
	commitAll(t, dir, "on main")
	sh(t, dir, "merge", "-q", "--no-edit", "side")
	sh(t, dir, "tag", "v1")

	all, err := Log(bg, dir, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	var commits []Entry
	graphOnly := 0
	for _, e := range all {
		if e.ID == "" {
			graphOnly++
		} else {
			commits = append(commits, e)
		}
	}
	if len(commits) != 6 {
		t.Fatalf("want 6 commits, got %d: %+v", len(commits), all)
	}
	top := commits[0]
	if !strings.HasPrefix(top.Subject, "Merge branch") || top.Author != "T" || top.When == "" ||
		!strings.Contains(top.Refs, "HEAD -> main") || !strings.Contains(top.Refs, "tag: v1") || !strings.Contains(top.Graph, "*") {
		t.Errorf("top commit wrong: %+v", top)
	}
	if graphOnly == 0 {
		t.Error("a merge should draw connector rows")
	}
	page1, _ := Log(bg, dir, 0, 2)
	page2, _ := Log(bg, dir, 2, 2)
	ids := map[string]bool{}
	for _, e := range append(page1, page2...) {
		if e.ID != "" {
			if ids[e.ID] {
				t.Errorf("commit %s on two pages", e.ID)
			}
			ids[e.ID] = true
		}
	}
	detail, err := Show(bg, dir, top.ID)
	if err != nil || !strings.Contains(detail, "Merge branch") {
		t.Errorf("show: %q, %v", detail, err)
	}
	if _, err := Show(bg, dir, "--output=/tmp/x"); err == nil {
		t.Error("an option must not pass as a commit id")
	}
	detail, _ = Show(bg, dir, commits[1].ID)
	if !strings.Contains(detail, "|") { // The --stat line for the file the commit changed.
		t.Errorf("show should list changed files: %q", detail)
	}
}

func TestDiffKinds(t *testing.T) {
	dir := newRepo(t)
	put(t, dir, "a.txt", "one\n")
	commitAll(t, dir, "first")
	put(t, dir, "a.txt", "two\n")
	put(t, dir, "u.txt", "new\n")
	put(t, dir, "bin.dat", "a\x00b")
	st, _ := ReadStatus(bg, dir)
	d, _, err := Diff(bg, dir, *find(st, "a.txt", Unstaged))
	if err != nil || !strings.Contains(d, "-one") || !strings.Contains(d, "+two") {
		t.Errorf("unstaged diff: %q, %v", d, err)
	}
	d, _, _ = Diff(bg, dir, *find(st, "u.txt", Untracked))
	if !strings.Contains(d, "+new") {
		t.Errorf("untracked diff: %q", d)
	}
	d, _, _ = Diff(bg, dir, *find(st, "bin.dat", Untracked))
	if !strings.Contains(d, "binary") {
		t.Errorf("binary diff: %q", d)
	}
	sh(t, dir, "add", "a.txt")
	st, _ = ReadStatus(bg, dir)
	d, _, _ = Diff(bg, dir, *find(st, "a.txt", Staged))
	if !strings.Contains(d, "+two") {
		t.Errorf("staged diff: %q", d)
	}
}

func TestStageCommitBranchesAndRefusedSwitch(t *testing.T) {
	dir := newRepo(t)
	put(t, dir, "a.txt", "one\n")
	if err := Stage(bg, dir, "a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := Commit(bg, dir, "-m starts with a dash"); err != nil {
		t.Fatal(err)
	}
	if _, err := Commit(bg, dir, "  "); err == nil {
		t.Error("an empty message should be refused")
	}
	if _, err := Commit(bg, dir, "nothing staged"); err == nil || !strings.Contains(err.Error(), "nothing") {
		t.Errorf("commit with nothing staged should say so, got %v", err)
	}
	log, _ := Log(bg, dir, 0, 5)
	if len(log) != 1 || log[0].Subject != "-m starts with a dash" {
		t.Fatalf("log: %+v", log)
	}

	for _, bad := range []string{"-x", "--detach", "", "a b", "a..b", "x\x01y", "ends.lock", "/lead", "q~1"} {
		if err := CreateBranch(bg, dir, bad); err == nil {
			t.Errorf("branch name %q should be refused", bad)
		}
		if err := Switch(bg, dir, bad); err == nil {
			t.Errorf("switching to %q should be refused", bad)
		}
		if _, err := Merge(bg, dir, bad); err == nil {
			t.Errorf("merging %q should be refused", bad)
		}
	}
	// A branch that only looks like an option, as a hostile repository could hold.
	sh(t, dir, "update-ref", "refs/heads/-evil", "HEAD")
	if err := CreateBranch(bg, dir, "feature"); err != nil {
		t.Fatal(err)
	}
	put(t, dir, "a.txt", "feature\n")
	commitAll(t, dir, "feature change")
	if err := Switch(bg, dir, "main"); err != nil {
		t.Fatal(err)
	}
	branches, err := Branches(bg, dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, b := range branches {
		names = append(names, b.Name)
		if b.Name == "main" && !b.Current {
			t.Error("main should be current")
		}
	}
	if strings.Join(names, ",") != "feature,main" {
		t.Errorf("branches = %v (the one starting with - must be left out)", names)
	}
	// Dirty work that the other branch would overwrite: git refuses, and says why.
	put(t, dir, "a.txt", "dirty\n")
	err = Switch(bg, dir, "feature")
	if err == nil || !strings.Contains(err.Error(), "overwritten") {
		t.Errorf("switch should be refused with git's message, got %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(got) != "dirty\n" {
		t.Error("the refused switch changed the file")
	}
}

func TestMergeConflictAndAbort(t *testing.T) {
	dir := newRepo(t)
	put(t, dir, "a.txt", "base\n")
	commitAll(t, dir, "base")
	sh(t, dir, "switch", "-q", "-c", "other")
	put(t, dir, "a.txt", "other\n")
	commitAll(t, dir, "other")
	sh(t, dir, "switch", "-q", "main")
	put(t, dir, "a.txt", "main\n")
	commitAll(t, dir, "main")

	_, err := Merge(bg, dir, "other")
	if err == nil || !strings.Contains(err.Error(), "CONFLICT") {
		t.Fatalf("merge should report the conflict: %v", err)
	}
	st, err := ReadStatus(bg, dir)
	if err != nil || !st.Merging {
		t.Fatalf("status should show a merge: %+v %v", st, err)
	}
	f := find(st, "a.txt", Conflicted)
	if f == nil || f.Marker() != 'U' {
		t.Fatalf("conflict not listed: %+v", st.Files)
	}
	if err := MergeAbort(bg, dir); err != nil {
		t.Fatal(err)
	}
	st, _ = ReadStatus(bg, dir)
	if st.Merging || len(st.Files) != 0 {
		t.Errorf("abort should leave a clean tree: %+v", st)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(got) != "main\n" {
		t.Errorf("abort should restore the file, got %q", got)
	}
	// Resolve by hand and mark resolved with Stage.
	Merge(bg, dir, "other") //nolint:errcheck // Conflicts again, on purpose.
	put(t, dir, "a.txt", "resolved\n")
	if err := Stage(bg, dir, "a.txt"); err != nil {
		t.Fatal(err)
	}
	if st, _ = ReadStatus(bg, dir); find(st, "a.txt", Conflicted) != nil || !st.Merging {
		t.Errorf("resolved file should leave the conflict list: %+v", st)
	}
}

func TestPushRejectedPullFetchAndAheadBehind(t *testing.T) {
	dir := newRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	sh(t, dir, "init", "-q", "--bare", "-b", "main", remote)
	put(t, dir, "a.txt", "1\n")
	commitAll(t, dir, "one")
	sh(t, dir, "remote", "add", "origin", remote)
	if remotes, _ := Remotes(bg, dir); len(remotes) != 1 || remotes[0] != "origin" {
		t.Fatalf("remotes = %v", remotes)
	}
	if _, err := Push(bg, dir, "main", false); err == nil || !strings.Contains(err.Error(), "no upstream") {
		t.Errorf("push without upstream should say so, got %v", err)
	}
	if _, err := Push(bg, dir, "-x", true); err == nil {
		t.Error("a dash name must be refused for push too")
	}
	if _, err := Push(bg, dir, "main", true); err != nil {
		t.Fatal(err)
	}
	st, _ := ReadStatus(bg, dir)
	if st.Upstream != "origin/main" || st.Ahead != 0 || st.Behind != 0 {
		t.Fatalf("after push: %+v", st)
	}

	// Someone else pushes first.
	other := filepath.Join(t.TempDir(), "other")
	sh(t, filepath.Dir(other), "clone", "-q", remote, other)
	put(t, other, "b.txt", "theirs\n")
	commitAll(t, other, "theirs")
	sh(t, other, "push", "-q")

	put(t, dir, "c.txt", "mine\n")
	commitAll(t, dir, "mine")
	if _, err := Push(bg, dir, "main", false); err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Errorf("push should be rejected, got %v", err)
	}
	if _, err := Fetch(bg, dir); err != nil {
		t.Fatal(err)
	}
	st, _ = ReadStatus(bg, dir)
	if st.Ahead != 1 || st.Behind != 1 {
		t.Errorf("want ahead 1 behind 1, got %d %d", st.Ahead, st.Behind)
	}
	if _, err := Pull(bg, dir); err != nil {
		t.Fatalf("pull should merge: %v", err)
	}
	if _, err := Push(bg, dir, "main", false); err != nil {
		t.Fatalf("push after pull: %v", err)
	}
	st, _ = ReadStatus(bg, dir)
	if st.Ahead != 0 || st.Behind != 0 {
		t.Errorf("want in sync, got %d %d", st.Ahead, st.Behind)
	}
}

func TestNotARepoAndGitMissing(t *testing.T) {
	isolate(t)
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(t.TempDir()))
	dir := t.TempDir()
	if _, err := ReadStatus(bg, dir); !errors.Is(err, ErrNotRepo) {
		t.Errorf("want ErrNotRepo, got %v", err)
	}
	if _, err := ListFiles(bg, dir); !errors.Is(err, ErrNotRepo) {
		t.Errorf("ListFiles: want ErrNotRepo, got %v", err)
	}
	if err := Init(bg, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadStatus(bg, dir); err != nil {
		t.Errorf("after init: %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	ResetForTest()
	if _, err := ReadStatus(bg, dir); !errors.Is(err, ErrMissing) {
		t.Errorf("want ErrMissing, got %v", err)
	}
	if Installed() {
		t.Error("git shouldn't be found")
	}
}

func TestListFilesHonoursGitignore(t *testing.T) {
	dir := newRepo(t)
	put(t, dir, ".gitignore", "*.log\nbuild/\n")
	put(t, dir, "keep.go", "x")
	put(t, dir, "sub/deep.go", "x")
	put(t, dir, "debug.log", "x")
	put(t, dir, "build/out.o", "x")
	put(t, dir, "sp ace\nnl.txt", "x")
	files, err := ListFiles(bg, filepath.Join(dir, "sub"))
	if err != nil || len(files) != 1 || files[0] != "deep.go" {
		t.Fatalf("subdir listing: %v %v", files, err)
	}
	files, err = ListFiles(bg, dir)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(files, "|")
	for _, want := range []string{".gitignore", "keep.go", "sub/deep.go", "sp ace\nnl.txt"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q missing from %q", want, got)
		}
	}
	for _, not := range []string{"debug.log", "build/out.o"} {
		if strings.Contains(got, not) {
			t.Errorf("%q should be ignored", not)
		}
	}
}

// A repository's own config must not get a program run by anything that runs on its own.
func TestBackgroundReadsRunNoRepoPrograms(t *testing.T) {
	dir := newRepo(t)
	put(t, dir, "a.txt", "one\n")
	put(t, dir, ".gitattributes", "*.txt filter=evil diff=evil\n")
	commitAll(t, dir, "first")
	marker := filepath.Join(t.TempDir(), "ran")
	script := filepath.Join(t.TempDir(), "evil.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho ran >> '"+marker+"'\ncat\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in program is a shell script")
	}
	sh(t, dir, "config", "core.fsmonitor", script)
	sh(t, dir, "config", "filter.evil.clean", script)
	sh(t, dir, "config", "diff.evil.textconv", script)
	sh(t, dir, "config", "diff.external", script)
	put(t, dir, "a.txt", "two, which changes size and time\n")

	// Precondition: plain git status would have run them, or this test proves nothing.
	if err := exec.Command("git", "-C", dir, "status").Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Skip("this git doesn't run the programs for a plain status, so there's nothing to guard")
	}
	os.Remove(marker)

	if _, err := ReadStatus(bg, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := Log(bg, dir, 0, 10); err != nil {
		t.Fatal(err)
	}
	st, _ := ReadStatus(bg, dir)
	if f := find(st, "a.txt", Unstaged); f != nil {
		if _, _, err := Diff(bg, dir, *f); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ListFiles(bg, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a program named in the repository's config ran during a background read")
	}
}

func TestNeedsTerminal(t *testing.T) {
	if !NeedsTerminal(&Error{Msg: "fatal: could not read Username for 'https://github.com': terminal prompts disabled"}) {
		t.Error("a password question should be recognised")
	}
	if NeedsTerminal(&Error{Msg: "rejected"}) || NeedsTerminal(nil) {
		t.Error("other failures aren't")
	}
}
