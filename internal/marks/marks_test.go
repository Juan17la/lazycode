package marks

import (
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestChanged(t *testing.T) {
	for name, tc := range map[string]struct {
		base, text string
		want       []int
	}{
		"same":            {"a\nb\n", "a\nb\n", nil},
		"both empty":      {"", "", nil},
		"new file":        {"", "a\nb\n", []int{0, 1}},
		"emptied":         {"a\nb\n", "", nil},
		"edited":          {"a\nb\nc\n", "a\nX\nc\n", []int{1}},
		"added at end":    {"a\n", "a\nb\n", []int{1}},
		"added at start":  {"b\n", "a\nb\n", []int{0}},
		"added in middle": {"a\nc\n", "a\nb\nc\n", []int{1}},
		"removed":         {"a\nb\nc\n", "a\nc\n", nil},
		"several":         {"a\nb\nc\nd\ne\n", "a\nX\nc\nY\nZ\ne\n", []int{1, 3, 4}},
		"moved":           {"a\nb\nc\n", "c\na\nb\n", []int{0}},
		"line endings":    {"a\nb\n", "a\r\nb\r\n", nil},
		"no last break":   {"a\nb", "a\nb\n", nil},
		"repeats":         {"x\nx\nx\n", "x\nx\nx\nx\n", []int{3}},
		"all different":   {"a\nb\n", "c\nd\n", []int{0, 1}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := Changed([]byte(tc.base), []byte(tc.text)); !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// Removing the marked lines must leave a subsequence of base, and the marks must be as few as
// a plain longest-common-subsequence says they can be.
func TestChangedAgainstThePlainAlgorithm(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for range 2000 {
		random := func() ([]string, string) {
			n := rng.IntN(15)
			ls := make([]string, n)
			for i := range ls {
				ls[i] = string(rune('a' + rng.IntN(4)))
			}
			if n == 0 {
				return ls, ""
			}
			return ls, strings.Join(ls, "\n") + "\n"
		}
		a, baseText := random()
		b, text := random()
		marked := Changed([]byte(baseText), []byte(text))
		var kept []string
		for i, line := range b {
			if !slices.Contains(marked, i) {
				kept = append(kept, line)
			}
		}
		if !isSubsequence(kept, a) {
			t.Fatalf("base %v, text %v: marks %v leave %v", a, b, marked, kept)
		}
		if want := len(b) - lcs(a, b); len(marked) != want {
			t.Fatalf("base %v, text %v: %d marks %v, the minimum is %d", a, b, len(marked), marked, want)
		}
	}
}

func isSubsequence(sub, of []string) bool {
	i := 0
	for _, s := range of {
		if i < len(sub) && sub[i] == s {
			i++
		}
	}
	return i == len(sub)
}

func lcs(a, b []string) int {
	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = 1 + dp[i+1][j+1]
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	return dp[0][0]
}

func TestVeryDifferentTextsStillGetAnAnswer(t *testing.T) {
	var a, b strings.Builder
	for i := range 5000 {
		fmt.Fprintf(&a, "a%d\n", i)
		fmt.Fprintf(&b, "b%d\n", i)
	}
	if got := Changed([]byte("same\n"+a.String()+"end\n"), []byte("same\n"+b.String()+"end\n")); len(got) != 5000 || got[0] != 1 {
		t.Fatalf("%d marks, first %v", len(got), got[:1])
	}
}

func FuzzChanged(f *testing.F) {
	f.Add("a\nb\n", "a\nc\n")
	f.Fuzz(func(t *testing.T, base, text string) {
		count := len(lines([]byte(text)))
		last := -1
		for _, line := range Changed([]byte(base), []byte(text)) {
			if line <= last || line >= count {
				t.Fatalf("bad mark %d of %d lines", line, count)
			}
			last = line
		}
	})
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestBase(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git isn't installed")
	}
	outside := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, inGit := Base(outside); inGit {
		t.Fatal("found Git in a plain folder")
	}

	dir := t.TempDir()
	git(t, dir, "init", "-q")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "sub", "tracked file.txt")
	if err := os.WriteFile(path, []byte("committed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if text, inGit := Base(path); !inGit || len(text) != 0 {
		t.Fatalf("before any commit: %q, %v", text, inGit)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "first")
	if err := os.WriteFile(path, []byte("edited since\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if text, inGit := Base(path); !inGit || string(text) != "committed\n" {
		t.Fatalf("got %q, %v", text, inGit)
	}
	fresh := filepath.Join(dir, "new.txt")
	if err := os.WriteFile(fresh, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if text, inGit := Base(fresh); !inGit || len(text) != 0 {
		t.Fatalf("untracked: %q, %v", text, inGit)
	}
}
