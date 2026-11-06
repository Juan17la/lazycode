package config

import (
	"os"
	"path/filepath"
	"reflect"
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
	if err != nil || !reflect.DeepEqual(c, Default()) {
		t.Fatalf("%+v, %v", c, err)
	}
}

func TestTemplateIsValidAndMatchesDefaults(t *testing.T) {
	c, err := Load(write(t, Template))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c, Default()) {
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

func TestUpdateCheckSetting(t *testing.T) {
	if !Default().Updates.Check {
		t.Error("the check should be on by default")
	}
	c, err := Load(write(t, "[updates]\ncheck = false\n"))
	if err != nil || c.Updates.Check {
		t.Errorf("check = %v, %v", c.Updates.Check, err)
	}
	if _, err := Load(write(t, "[updates]\ncheck = true\nother = 1\n")); err == nil {
		t.Error("an unknown key under [updates] should be reported now that the section is read")
	}
}

func TestSessionSettings(t *testing.T) {
	c, err := Load(write(t, `
[sessions]
sleep_after_minutes = 0
[[harness]]
name = "Aider"
command = "aider"
start_args = ["--yes"]
resume_args = ["--restore-chat-history"]
`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Sessions.SleepAfterMinutes != 0 || len(c.Harness) != 1 || c.Harness[0].Command != "aider" || !c.Harness[0].CanResume() {
		t.Fatalf("%+v", c)
	}
	if _, err := Load(write(t, "[[harness]]\nname = \"no command\"\n")); err == nil {
		t.Fatal("a tool with no command was accepted")
	}
	if _, err := Load(write(t, "[sessions]\nsleep_after_minutes = -1\n")); err == nil {
		t.Fatal("a negative time was accepted")
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

func TestFirstConfigHoldsTheChoices(t *testing.T) {
	c, err := Load(write(t, FirstConfig("light", true)))
	if err != nil {
		t.Fatal(err)
	}
	if c.UI.Theme != "light" || !c.Editor.AutoSave {
		t.Fatalf("theme %q, auto-save %v", c.UI.Theme, c.Editor.AutoSave)
	}
	if FirstConfig("dark", false) != Template {
		t.Fatal("the defaults should be the template")
	}
}

func TestSetThemeKeepsTheRestOfTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := SetTheme(path, "nord"); err != nil { // No file yet: starts from the template.
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil || c.UI.Theme != "nord" {
		t.Fatalf("%q, %v", c.UI.Theme, err)
	}
	custom := "version = 1\n\n[ui]\nsidebar = \"right\"\n"
	if err := os.WriteFile(path, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetTheme(path, "dracula"); err != nil {
		t.Fatal(err)
	}
	if c, err = Load(path); err != nil || c.UI.Theme != "dracula" || c.UI.Sidebar != "right" {
		t.Fatalf("%+v, %v", c.UI, err)
	}
	if err := SetTheme(path, "monokai"); err != nil { // Now it replaces the line, keeping its comment.
		t.Fatal(err)
	}
	if c, err = Load(path); err != nil || c.UI.Theme != "monokai" {
		t.Fatalf("%+v, %v", c.UI, err)
	}
}

func TestColourAndIconSettingsAreChecked(t *testing.T) {
	for _, text := range []string{"[ui]\nicons = \"x\"\n", "[ui.colors]\nnope = \"#000000\"\n", "[ui.colors]\naccent = \"red\"\n"} {
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte("version = 1\n"+text), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
}
