package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Juan17la/lazycode/internal/git"
	"github.com/gdamore/tcell/v2"
)

// gitEnv keeps Git and gh away from the user's settings and from the network. Real git runs, but only in
// the test's own folders.
func gitEnv(t *testing.T) {
	t.Helper()
	if !git.Installed() {
		t.Skip("git isn't installed")
	}
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	text := "[init]\n\tdefaultBranch = main\n[user]\n\tname = T\n\temail = t@example.com\n[commit]\n\tgpgsign = false\n"
	if err := os.WriteFile(cfg, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir())
}

// startGit is start with Git isolated. setup runs before the app starts.
func startGit(t *testing.T, files map[string]string) *ui {
	t.Helper()
	gitEnv(t)
	u := start(t, files)
	u.screen.SetSize(100, 32)
	return u
}

// git runs git in the test's folder, as setup. Tests may write there.
func (u *ui) git(args ...string) string {
	u.t.Helper()
	return u.gitIn(u.dir, args...)
}

func (u *ui) gitIn(dir string, args ...string) string {
	u.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		u.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (u *ui) write(name, text string) {
	u.t.Helper()
	if err := os.WriteFile(filepath.Join(u.dir, name), []byte(text), 0o644); err != nil {
		u.t.Fatal(err)
	}
}

func TestNotARepoOffersToCreateOne(t *testing.T) {
	u := startGit(t, map[string]string{"a.txt": "x"})
	u.alt('g')
	u.wait("Not a Git repository")
	u.wait("[Create one: git init]")
	u.clickOn("[Create one: git init]")
	u.wait("Run git init in work?")
	u.press("c") // Cancel first: nothing is made.
	u.waitGone("Run git init")
	if _, err := os.Stat(filepath.Join(u.dir, ".git")); err == nil {
		t.Fatal("a repository was made without confirmation")
	}
	u.press("i")
	u.wait("Run git init in work?")
	u.press("y")
	u.wait("main")
	u.wait("Untracked (1)")
	if _, err := os.Stat(filepath.Join(u.dir, ".git")); err != nil {
		t.Fatal("no repository after confirming")
	}
}

func TestStageUnstageCommitThroughKeys(t *testing.T) {
	u := startGit(t, map[string]string{"a.txt": "one\n"})
	u.git("init", "-q")
	u.alt('g')
	u.wait("Untracked (1)")
	u.wait("No commits yet")
	u.press("c")
	u.wait("Type a commit message") // The box takes the keyboard.
	u.press("-first commit")
	u.key(tcell.KeyEnter)
	u.wait("Nothing is staged") // Offers to stage everything and commit.
	u.press("c")                // Cancel.
	u.key(tcell.KeyEsc)
	u.press("s")
	u.wait("Staged (1)")
	u.press("u")
	u.wait("Untracked (1)")
	u.press("s")
	u.wait("Staged (1)")
	u.press("c") // The message is still in the box.
	u.wait("commit done")
	u.wait("-first commit") // Starting with a dash is fine in a message.
	u.wait("Nothing changed")
	if out := u.git("log", "--format=%s"); strings.TrimSpace(out) != "-first commit" {
		t.Errorf("log: %q", out)
	}
}

func TestDiffOpensInAReadOnlyTab(t *testing.T) {
	u := startGit(t, map[string]string{"a.txt": "one\n"})
	u.git("init", "-q")
	u.git("add", "-A")
	u.git("commit", "-q", "-m", "first")
	u.write("a.txt", "two\n")
	u.alt('g')
	u.wait("Changes (1)")
	u.key(tcell.KeyEnter)
	u.wait("a.txt (diff)")
	u.wait("+two")
	u.wait("-one")
	u.press("x") // A read-only tab takes no typing.
	u.wait("read-only")
	u.waitGone("xdiff")
	// Closing it never asks about saving.
	u.mod(tcell.KeyCtrlW, 0)
	u.waitGone("a.txt (diff)")
}

func TestCommitDetailsListChangedFiles(t *testing.T) {
	u := startGit(t, map[string]string{"a.txt": "one\n", "b.txt": "b\n"})
	u.screen.SetSize(120, 32)
	u.git("init", "-q")
	u.git("add", "-A")
	u.git("commit", "-q", "-m", "add two files")
	u.alt('g')
	u.wait("add two files")
	u.wait("ago (HEAD -> main") // Relative date and refs, under the subject.
	u.key(tcell.KeyEnter)
	u.wait("(commit)")
	u.wait("a.txt")
	u.wait("2 files changed")
}

func TestPushPullFetchWithALocalRemote(t *testing.T) {
	u := startGit(t, map[string]string{"a.txt": "one\n"})
	remote := filepath.Join(t.TempDir(), "remote.git")
	u.git("init", "-q", "--bare", "-b", "main", remote)
	u.git("init", "-q")
	u.git("add", "-A")
	u.git("commit", "-q", "-m", "first")
	u.git("remote", "add", "origin", remote)
	u.alt('g')
	u.wait("no upstream")
	u.press("p")
	u.wait("has no upstream. Push it to origin and track it?")
	u.press("y")
	u.wait("push done. main is up to date with origin/main.")
	u.wait("↑0 ↓0")

	// Another clone pushes; fetch shows we're behind, and push is rejected.
	other := filepath.Join(t.TempDir(), "other")
	u.gitIn(filepath.Dir(other), "clone", "-q", remote, other)
	if err := os.WriteFile(filepath.Join(other, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	u.gitIn(other, "add", "-A")
	u.gitIn(other, "commit", "-q", "-m", "theirs")
	u.gitIn(other, "push", "-q")

	u.write("c.txt", "c")
	u.git("add", "-A")
	u.git("commit", "-q", "-m", "mine")
	u.press("f")
	u.wait("fetch done. main is 1 ahead and 1 behind origin/main.")
	u.press("p")
	u.wait("push failed:")
	u.wait("rejected")
	u.key(tcell.KeyCtrlBackslash) // Not needed; just keep the panel focused.
	u.alt('g')
	u.press("l")
	u.wait("pull done. main is 2 ahead and 0 behind origin/main.")
	// The command forms.
	u.key(tcell.KeyEsc)
	u.run("push")
	u.wait("push done. main is up to date with origin/main.")
}

func TestMergeConflictFlow(t *testing.T) {
	u := startGit(t, map[string]string{"a.txt": "base\n"})
	u.git("init", "-q")
	u.git("add", "-A")
	u.git("commit", "-q", "-m", "base")
	u.git("switch", "-q", "-c", "other")
	u.write("a.txt", "other\n")
	u.git("commit", "-q", "-am", "other")
	u.git("switch", "-q", "main")
	u.write("a.txt", "main\n")
	u.git("commit", "-q", "-am", "main")
	u.alt('g')
	u.wait("main")
	u.press("m")
	u.wait("Merge into the current branch")
	u.wait("other")
	u.key(tcell.KeyEnter)
	u.wait("Merge other into main?")
	u.press("m")
	u.wait("stopped with 1 conflicting files")
	u.wait("Merge conflicts (1)")
	u.wait("MERGING")
	u.wait("U a.txt")
	// Marking it resolved while markers remain is refused.
	u.key(tcell.KeyUp) // The selection stayed on the commit it was on.
	u.press("s")
	u.wait("still has conflict markers")
	// Enter opens the file to fix it.
	u.key(tcell.KeyEnter)
	u.wait("<<<<<<<")
	// Abort names what's lost, and waits for the answer.
	u.key(tcell.KeyCtrlBackslash)
	u.press("x")
	u.wait("Abort the merge? These 1 files go back to how they were before it")
	u.wait("a.txt")
	u.press("c") // Keep merging.
	u.waitGone("Abort the merge?")
	if _, err := os.Stat(filepath.Join(u.dir, ".git", "MERGE_HEAD")); err != nil {
		t.Fatal("keeping the merge ended it")
	}
	u.press("x")
	u.press("a")
	u.wait("abort merge done")
	u.waitGone("MERGING")
	if u.file("a.txt") != "main\n" {
		t.Errorf("abort left %q", u.file("a.txt"))
	}
}

func TestBranchSwitchRefusedShowsGitsOwnMessage(t *testing.T) {
	u := startGit(t, map[string]string{"a.txt": "one\n"})
	u.git("init", "-q")
	u.git("add", "-A")
	u.git("commit", "-q", "-m", "first")
	u.git("switch", "-q", "-c", "feature")
	u.write("a.txt", "feature\n")
	u.git("commit", "-q", "-am", "f")
	u.git("switch", "-q", "main")
	u.write("a.txt", "dirty\n")
	u.alt('g')
	u.wait("Changes (1)")
	u.press("b")
	u.wait("Switch branch")
	u.press("feat")
	u.key(tcell.KeyEnter)
	u.wait("switch to feature failed:")
	u.wait("overwritten")
	if u.file("a.txt") != "dirty\n" {
		t.Error("the refused switch touched the file")
	}
	// A new branch whose name looks like an option is refused before git sees it.
	u.press("n")
	u.press("--detach")
	u.key(tcell.KeyEnter)
	u.wait("can't start with -")
	u.press("n")
	u.press("topic")
	u.key(tcell.KeyEnter)
	u.wait("create branch done")
	u.wait("topic")
}

func TestRepoCreateWithAFakeGh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in gh is a shell script")
	}
	gitEnv(t)
	bin := t.TempDir()
	log := filepath.Join(bin, "log")
	script := "#!/bin/sh\necho \"$@\" >> '" + log + "'\ncase \"$1 $2\" in\n\"auth status\") [ -f '" + filepath.Join(bin, "loggedout") + "' ] && exit 1; exit 0;;\n\"repo create\") echo https://github.com/me/$3;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	u := start(t, map[string]string{"a.txt": "x"})
	u.screen.SetSize(140, 32)

	// Logged out: say what's missing and how to get it, and run nothing else.
	if err := os.WriteFile(filepath.Join(bin, "loggedout"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	u.run("repo create")
	u.wait("gh isn't logged in. Run gh auth login")
	os.Remove(filepath.Join(bin, "loggedout"))

	u.run("repo create")
	u.wait("isn't a Git repository. Run git init first?")
	u.press("y")
	u.wait("Repository name:")
	u.key(tcell.KeyBackspace2) // The folder is called "work": make it "wor".
	u.key(tcell.KeyEnter)
	u.wait("Who can see wor on GitHub?")
	u.wait("Private (default)")
	u.press("p")
	u.wait("gh repo create wor --private --source=. --remote=origin")
	if data, _ := os.ReadFile(log); strings.Contains(string(data), "repo create") {
		t.Fatal("gh repo create ran before it was confirmed")
	}
	u.press("r")
	u.wait("Created https://github.com/me/wor")
	data, _ := os.ReadFile(log)
	if !strings.Contains(string(data), "repo create wor --private --source=. --remote=origin") {
		t.Errorf("gh was run with %q", data)
	}

	// A name that would be read as an option never reaches gh.
	u.run("repo create")
	u.wait("Repository name:")
	for range 4 {
		u.key(tcell.KeyBackspace2)
	}
	u.press("--public")
	u.key(tcell.KeyEnter)
	u.wait("can't start with -")
}

func TestRepoCreateWithoutGh(t *testing.T) {
	gitEnv(t)
	t.Setenv("PATH", t.TempDir()) // No gh, and no git either, but the app only needs gh here.
	u := start(t, map[string]string{"a.txt": "x"})
	u.run("repo create")
	u.wait("gh isn't installed. Get it from https://cli.github.com")
}

func TestGitMissingIsExplained(t *testing.T) {
	gitEnv(t)
	t.Setenv("PATH", t.TempDir())
	git.ResetForTest()
	t.Cleanup(git.ResetForTest)
	u := start(t, map[string]string{"a.txt": "x"})
	u.screen.SetSize(100, 32)
	u.alt('g')
	u.wait("git isn't installed")
}

func TestGitPanelInSmallWindowsDoesntCrash(t *testing.T) {
	u := startGit(t, map[string]string{"a.txt": "x"})
	u.git("init", "-q")
	u.alt('g')
	u.wait("Untracked (1)")
	for _, size := range [][2]int{{40, 8}, {41, 3}, {80, 5}, {120, 40}, {60, 12}} {
		u.screen.SetSize(size[0], size[1])
		u.key(tcell.KeyDown)
		u.alt('f')
		u.press("a")
		u.alt('g')
	}
	u.screen.SetSize(100, 32)
	u.wait("Untracked (1)")
}

func TestHistoryLoadsInPages(t *testing.T) {
	u := startGit(t, map[string]string{"a.txt": "x"})
	u.git("init", "-q")
	// Many commits at once, without a process for each.
	var stream strings.Builder
	for i := range 400 {
		stream.WriteString("commit refs/heads/main\ncommitter T <t@e> 1700000000 +0000\ndata 6\n" + fmt.Sprintf("c%04d\n", i))
		if i == 0 {
			stream.WriteString("M 100644 inline a.txt\ndata 1\nx\n")
		}
		stream.WriteString("\n")
	}
	cmd := exec.Command("git", "-C", u.dir, "fast-import", "--quiet")
	cmd.Stdin = strings.NewReader(stream.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fast-import: %v %s", err, out)
	}
	u.git("reset", "-q", "--hard")
	u.alt('g')
	u.wait("c0399")
	u.waitGone("c0000")
	for range 200 { // Moving down to the end asks for the next page.
		u.key(tcell.KeyDown)
	}
	for range 400 {
		if strings.Contains(u.text(), "c0000") {
			return
		}
		u.key(tcell.KeyPgDn)
	}
	u.wait("c0000")
}

func TestButtonsAndRowsAreClickable(t *testing.T) {
	u := startGit(t, map[string]string{"a.txt": "one\n"})
	u.git("init", "-q")
	u.git("add", "-A")
	u.git("commit", "-q", "-m", "first")
	u.write("a.txt", "two\n")
	u.alt('g')
	u.wait("Fetch")
	u.clickOn("Fetch")
	u.clickOn("M a.txt")
	u.wait("a.txt (diff)")
	u.wait("+two")
}
