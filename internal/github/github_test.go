package github

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var bg = context.Background()

// fakeGH puts a stand-in gh first on PATH. It records its arguments in args, and acts on $STATE:
// "out" fails "auth status", "fail" fails "repo create".
func fakeGH(t *testing.T, state string) (argsFile string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in gh is a shell script")
	}
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")
	script := `#!/bin/sh
echo "$@" >> '` + argsFile + `'
case "$1 $2" in
  "auth status") [ "$STATE" = out ] && { echo "You are not logged in" >&2; exit 1; }; exit 0;;
  "repo create") [ "$STATE" = fail ] && { echo "name already exists on this account" >&2; exit 1; }; echo "https://github.com/me/$3"; exit 0;;
esac
exit 2
`
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("STATE", state)
	return argsFile
}

func TestCheck(t *testing.T) {
	fakeGH(t, "in")
	if err := Check(bg); err != nil {
		t.Errorf("logged in: %v", err)
	}
	t.Setenv("STATE", "out")
	if err := Check(bg); !errors.Is(err, ErrLoggedOut) {
		t.Errorf("want ErrLoggedOut, got %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	if err := Check(bg); !errors.Is(err, ErrMissing) {
		t.Errorf("want ErrMissing, got %v", err)
	}
}

func TestCreateRepoRunsExactlyWhatIsShown(t *testing.T) {
	args := fakeGH(t, "in")
	dir := t.TempDir()
	out, err := CreateRepo(bg, dir, "my-repo", true)
	if err != nil || !strings.Contains(out, "github.com/me/my-repo") {
		t.Fatalf("%q %v", out, err)
	}
	got, _ := os.ReadFile(args)
	if strings.TrimSpace(string(got)) != strings.Join(CreateArgs("my-repo", true), " ") {
		t.Errorf("ran %q", got)
	}
	if want := "repo create my-repo --private --source=. --remote=origin"; strings.Join(CreateArgs("my-repo", true), " ") != want {
		t.Errorf("private args wrong")
	}
	if !strings.Contains(strings.Join(CreateArgs("x", false), " "), "--public") {
		t.Error("public should say --public")
	}
	fakeGH(t, "fail")
	if _, err := CreateRepo(bg, dir, "taken", true); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("gh's message should come through, got %v", err)
	}
}

func TestNamesStartingWithDashAreRefused(t *testing.T) {
	args := fakeGH(t, "in")
	for _, bad := range []string{"", "-x", "--public", "a b", "a/b", "x;rm", ".", "..", "name\n", strings.Repeat("a", 101), "é"} {
		if err := ValidName(bad); err == nil {
			t.Errorf("%q should be refused", bad)
		}
		if _, err := CreateRepo(bg, t.TempDir(), bad, true); err == nil {
			t.Errorf("CreateRepo(%q) should be refused", bad)
		}
	}
	if _, err := os.Stat(args); err == nil {
		t.Error("gh must not have run for a refused name")
	}
	for _, good := range []string{"a", "my.repo_1-x", "A"} {
		if err := ValidName(good); err != nil {
			t.Errorf("%q: %v", good, err)
		}
	}
}
