package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMissingFileGivesDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "none.toml"))
	if err != nil || c != Default() {
		t.Fatalf("%+v, %v", c, err)
	}
}

func TestTemplateIsValidAndMatchesDefaults(t *testing.T) {
	c, err := Load(write(t, Template))
	if err != nil {
		t.Fatal(err)
	}
	if c != Default() {
		t.Fatalf("template gives %+v, defaults are %+v", c, Default())
	}
}

func TestValues(t *testing.T) {
	c, err := Load(write(t, `
[editor]
auto_save = true
tab_size = 2
[ui]
theme = "light"
sidebar = "right"
[terminal]
scrollback = 500
`))
	if err != nil {
		t.Fatal(err)
	}
	if !c.Editor.AutoSave || c.Editor.TabSize != 2 || c.UI.Theme != "light" || c.UI.Sidebar != "right" || c.Terminal.Scrollback != 500 {
		t.Fatalf("%+v", c)
	}
}

func TestPartialFileKeepsOtherDefaults(t *testing.T) {
	c, err := Load(write(t, "[editor]\ntab_size = 8\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Editor.TabSize != 8 || c.UI.Theme != "dark" || c.Terminal.Scrollback != 1000 {
		t.Fatalf("%+v", c)
	}
}

func TestSettingsForLaterFeaturesAreAccepted(t *testing.T) {
	_, err := Load(write(t, `
[sessions]
sleep_after_minutes = 30
[updates]
check = true
[[harness]]
name = "Claude Code"
command = "claude"
`))
	if err != nil {
		t.Fatal(err)
	}
}

func TestBadFiles(t *testing.T) {
	for name, tc := range map[string]struct{ content, want string }{
		"syntax":          {"version = 1\n[editor\n", ", line "},
		"wrong type":      {"[editor]\ntab_size = \"big\"\n", "tab_size"},
		"unknown":         {"[editor]\nfont = 1\n", "editor.font"},
		"version":         {"version = 2\n", "version 2"},
		"tab size":        {"[editor]\ntab_size = 0\n", "tab_size"},
		"big tab size":    {"[editor]\ntab_size = 99\n", "tab_size"},
		"theme":           {"[ui]\ntheme = \"pink\"\n", "theme"},
		"sidebar":         {"[ui]\nsidebar = \"top\"\n", "sidebar"},
		"zero scroll":     {"[terminal]\nscrollback = 0\n", "scrollback"},
		"huge scroll":     {"[terminal]\nscrollback = 100000\n", "scrollback"},
		"several at once": {"[ui]\ntheme = \"x\"\nsidebar = \"y\"\n", "sidebar"},
	} {
		t.Run(name, func(t *testing.T) {
			path := write(t, tc.content)
			c, err := Load(path)
			if err == nil {
				t.Fatalf("no error, got %+v", c)
			}
			if !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), filepath.Base(path)) {
				t.Fatalf("error %q should name %q and the file", err, tc.want)
			}
		})
	}
}

func TestEnsureFileNeverReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "dir", "file.toml")
	if err := EnsureFile(path, "first"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureFile(path, "second"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "first" {
		t.Fatalf("%q, %v", got, err)
	}
}
