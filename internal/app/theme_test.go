package app

import (
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Juan17la/lazycode/internal/config"
	"github.com/Juan17la/lazycode/internal/git"
	"github.com/gdamore/tcell/v2"
)

func luminance(hex string) float64 {
	r, g, b := tcell.GetColor(hex).RGB()
	lin := func(v int32) float64 {
		c := float64(v) / 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

func TestEveryThemeIsCompleteAndReadable(t *testing.T) {
	if got := slices.Sorted(maps.Keys(palettes)); !slices.Equal(got, slices.Sorted(slices.Values(config.Themes))) {
		t.Fatalf("palettes %v, config lists %v", got, config.Themes)
	}
	for name, p := range palettes {
		for _, key := range config.ColorKeys {
			if p[key] == "" {
				t.Errorf("%s has no %s", name, key)
			}
		}
		if c := contrast(p["fg"], p["bg"]); c < 4.5 {
			t.Errorf("%s: text is only %.1f:1", name, c)
		}
		if c := contrast(p["fg"], p["selection"]); c < 3 {
			t.Errorf("%s: selected text is only %.1f:1", name, c)
		}
		for _, key := range []string{"muted", "comment", "accent", "red", "green", "yellow", "blue", "purple", "cyan", "orange"} {
			if c := contrast(p[key], p["bg"]); c < 3 {
				t.Errorf("%s: %s is only %.1f:1", name, key, c)
			}
		}
		th := themeFor(name)
		if attrs(th.keyword)&tcell.AttrBold == 0 || attrs(th.comment)&tcell.AttrItalic == 0 {
			t.Errorf("%s: meaning must survive without colour", name)
		}
	}
}

func TestCustomColourReplacesTheThemes(t *testing.T) {
	th := themeWith("dracula", map[string]string{"accent": "#123456"})
	if fg, _, _ := th.accent.Decompose(); fg != tcell.GetColor("#123456") {
		t.Fatalf("accent is %v", fg)
	}
}

func TestBackgroundIsPaintedByRegion(t *testing.T) {
	u, screen := launchStyled(t, map[string]string{"a.go": "package a\n", "b.go": "package b\n"})
	u.wait("b.go") // The first row is selected, which has its own background.
	want := tcell.GetColor(palettes[config.DefaultTheme]["panel"])
	screen.waitStyle(u, "b.go", func(st tcell.Style) bool {
		_, bg, _ := st.Decompose()
		return bg == want
	}, "the panel's background")
}

func TestThemeCommandPreviewsCancelsAndSaves(t *testing.T) {
	var a *app
	u := startTweaked(t, nil, func(x *app) { a = x })
	u.run("theme")
	u.wait("gruvbox-dark")
	u.key(tcell.KeyDown) // Preview only.
	time.Sleep(50 * time.Millisecond)
	u.key(tcell.KeyEsc)
	u.waitGone("gruvbox-dark")
	u.run("theme dracula")
	u.wait("Theme: dracula")
	path := a.settingsPath("config.toml")
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `theme = "dracula"`) {
		t.Fatalf("not saved: %q, %v", data, err)
	}
	if c, err := config.Load(path); err != nil || c.UI.Theme != "dracula" {
		t.Fatalf("doesn't load back: %v, %v", c.UI.Theme, err)
	}
	u.run("theme nope")
	u.wait("No theme nope")
}

func TestTreeShowsGitStates(t *testing.T) {
	u := startGit(t, map[string]string{"edited.txt": "one\n", "fresh.txt": "new\n", "stray.txt": "x\n"})
	u.git("init", "-q")
	u.git("add", "edited.txt")
	u.git("commit", "-qm", "first")
	u.write("edited.txt", "two\n")
	u.git("add", "fresh.txt")
	deadline := time.Now().Add(15 * time.Second)
	for {
		text := u.text()
		ok := true
		for name, letter := range map[string]string{"edited.txt": "M", "fresh.txt": "A", "stray.txt": "U"} {
			found := false
			for _, line := range strings.Split(text, "\n") {
				if _, rest, cut := strings.Cut(line, name); cut && strings.HasPrefix(strings.TrimLeft(rest, " "), letter) {
					found = true
				}
			}
			ok = ok && found
		}
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no letters:\n%s", text)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestBadgesCoverFoldersAndUntrackedContents(t *testing.T) {
	a := hlApp(t, nil)
	root := a.dirPath()
	a.git.dir = root
	a.git.st = &git.Status{Root: root, Files: []git.File{
		{Path: "new/", X: '?', Y: '?', Kind: git.Untracked},
		{Path: "d/e/f.txt", X: '.', Y: 'M', Kind: git.Unstaged},
		{Path: "d/e/g.txt", X: 'U', Y: 'U', Kind: git.Conflicted},
	}}
	b := a.badges()
	for path, want := range map[string]byte{"new": 'U', "new/inner/x.txt": 'U', "d/e/f.txt": 'M', "d/e/g.txt": '!', "d": '!', "d/e": '!', "other.txt": 0} {
		isDir := !strings.Contains(filepath.Base(path), ".")
		if got := b.of(filepath.Join(root, filepath.FromSlash(path)), isDir); got != want {
			t.Errorf("%s: got %q, want %q", path, got, want)
		}
	}
	var none *gitBadges
	if none.of("x", false) != 0 {
		t.Error("no status must mean no badge")
	}
}
