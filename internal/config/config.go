// Package config loads config.toml (Q-26).
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/Juan17la/lazycode/internal/session"
)

// Config holds the settings the editor uses so far.
type Config struct {
	Version int `toml:"version"`
	Editor  struct {
		AutoSave bool `toml:"auto_save"`
		TabSize  int  `toml:"tab_size"`
	} `toml:"editor"`
	UI struct {
		Theme   string            `toml:"theme"`
		Icons   string            `toml:"icons"`
		Colors  map[string]string `toml:"colors"`
		Sidebar string            `toml:"sidebar"`
	} `toml:"ui"`
	Terminal struct {
		Scrollback int `toml:"scrollback"`
	} `toml:"terminal"`
	Sessions struct {
		SleepAfterMinutes int `toml:"sleep_after_minutes"`
	} `toml:"sessions"`
	Updates struct {
		Check bool `toml:"check"` // Q-79.
	} `toml:"updates"`
	Harness []session.Harness `toml:"harness"`
}

// Limits. A scrollback of 0 means the emulator's large default, not none (doc 04), so
// it's refused. 10,000 lines cost 185 MB a tab, which is already over the budget's share.
const (
	maxTabSize    = 16
	maxScrollback = 10_000
)

// Default returns the settings used with no config file.
func Default() Config {
	var c Config
	c.Version = 1
	c.Editor.TabSize = 4
	c.UI.Theme = DefaultTheme
	c.UI.Icons = "basic"
	c.UI.Sidebar = "left"
	c.Terminal.Scrollback = 1000
	c.Sessions.SleepAfterMinutes = 30 // Q-77.
	c.Updates.Check = true
	return c
}

// Template is what a new config.toml starts with.
const Template = `# LazyCode settings. Save this file to apply them.
version = 1

[editor]
auto_save = false   # Save a second after typing stops.
tab_size = 4

[ui]
theme = "dark"      # Or use :theme. dark, light, gruvbox-dark, gruvbox-light, dracula, one-dark,
                    # nord, tokyo-night, catppuccin-mocha, solarized-dark, solarized-light, monokai
icons = "basic"     # basic works in any font. nerd needs a Nerd Font: nerdfonts.com
sidebar = "left"    # left or right

# Change any colour of the theme: bg, panel, bar, selection, fg, muted, comment, accent,
# red, green, yellow, blue, purple, cyan, orange.
# [ui.colors]
# accent = "#ff8800"

[terminal]
scrollback = 1000   # Lines kept per terminal tab. Each 1,000 lines cost about 36 MB.

[sessions]
sleep_after_minutes = 30   # An idle AI session stops to free its memory. 0 turns this off.

[updates]
check = true   # Look on GitHub for a new version, at most once a day.

# Add an AI tool, or change a default one by using its name. The resume fields are
# optional. Without them, selecting a stopped session starts a new one. {id} is a session
# ID, and {id_hex} is the same without dashes.
#
# [[harness]]
# name = "Claude Code"
# command = "claude"
# start_args = ["--session-id", "{id}"]
# resume_args = ["--resume", "{id}"]
`

// DefaultTheme is used until the user chooses.
const DefaultTheme = "dark"

// Themes lists the built-in themes, in the order :theme shows them.
var Themes = []string{"dark", "light", "gruvbox-dark", "gruvbox-light", "dracula", "one-dark", "nord", "tokyo-night", "catppuccin-mocha", "solarized-dark", "solarized-light", "monokai"}

// ColorKeys are what [ui.colors] can change.
var ColorKeys = []string{"bg", "panel", "bar", "selection", "fg", "muted", "comment", "accent", "red", "green", "yellow", "blue", "purple", "cyan", "orange"}

var (
	hexColor  = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	themeLine = regexp.MustCompile(`(?m)^theme[ \t]*=[^\r\n#]*?([ \t]*(#[^\r\n]*)?)$`)
	uiTable   = regexp.MustCompile(`(?m)^\[ui\][ \t]*\r?$`)
)

// SetTheme changes the theme in a config file and leaves the rest of it alone. The file is replaced
// in one step, so a crash can't leave half of it.
func SetTheme(path, name string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		data, err = []byte(Template), nil
	}
	if err != nil {
		return err
	}
	text := string(data)
	line := fmt.Sprintf("theme = %q", name)
	switch loc := themeLine.FindStringSubmatchIndex(text); {
	case loc != nil:
		text = text[:loc[0]] + line + text[loc[2]:loc[3]] + text[loc[1]:]
	case uiTable.MatchString(text):
		loc := uiTable.FindStringIndex(text)
		text = text[:loc[1]] + "\n" + line + text[loc[1]:]
	default:
		text = strings.TrimRight(text, "\r\n") + "\n\n[ui]\n" + line + "\n"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name()) //nolint:errcheck // Already renamed, unless something failed.
	if _, err := temp.WriteString(text); err != nil {
		temp.Close() //nolint:errcheck // The write error is the one to report.
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}

// FirstConfig is the Template with the choices made in onboarding.
func FirstConfig(theme string, autoSave bool) string {
	return strings.NewReplacer(
		`theme = "dark"`, fmt.Sprintf("theme = %q", theme),
		"auto_save = false", fmt.Sprintf("auto_save = %t", autoSave),
	).Replace(Template)
}

// Dir returns the folder holding LazyCode's config files.
func Dir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lazycode"), nil
}

// Load reads a config file. A missing file means the defaults. A broken one returns an
// error naming the file and line, and the defaults, so the caller can keep what it has.
func Load(path string) (Config, error) {
	c := Default()
	name := filepath.Base(path) // Short, so that a one-line message reaches the problem.
	meta, err := toml.DecodeFile(path, &c)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		var parse toml.ParseError
		if errors.As(err, &parse) {
			return Default(), fmt.Errorf("%s, line %d: %s", name, parse.Position.Line, parse.Message)
		}
		return Default(), fmt.Errorf("%s: %w", name, err)
	}
	var problems []string
	for _, key := range meta.Undecoded() {
		problems = append(problems, "unknown setting "+key.String())
	}
	if c.Version != 1 {
		problems = append(problems, fmt.Sprintf("version %d isn't supported", c.Version))
	}
	if c.Editor.TabSize < 1 || c.Editor.TabSize > maxTabSize {
		problems = append(problems, fmt.Sprintf("tab_size must be from 1 to %d", maxTabSize))
	}
	if !slices.Contains(Themes, c.UI.Theme) {
		problems = append(problems, "theme must be one of "+strings.Join(Themes, ", "))
	}
	if c.UI.Icons != "basic" && c.UI.Icons != "nerd" {
		problems = append(problems, `icons must be "basic" or "nerd"`)
	}
	for _, key := range slices.Sorted(maps.Keys(c.UI.Colors)) {
		if !slices.Contains(ColorKeys, key) {
			problems = append(problems, "unknown colour "+key)
		} else if !hexColor.MatchString(c.UI.Colors[key]) {
			problems = append(problems, "colour "+key+` must look like "#ff8800"`)
		}
	}
	if c.UI.Sidebar != "left" && c.UI.Sidebar != "right" {
		problems = append(problems, `sidebar must be "left" or "right"`)
	}
	if c.Terminal.Scrollback < 1 || c.Terminal.Scrollback > maxScrollback {
		problems = append(problems, fmt.Sprintf("scrollback must be from 1 to %d", maxScrollback))
	}
	if c.Sessions.SleepAfterMinutes < 0 {
		problems = append(problems, "sleep_after_minutes can't be negative")
	}
	for i, h := range c.Harness {
		if h.Name == "" || h.Command == "" {
			problems = append(problems, fmt.Sprintf("harness %d needs a name and a command", i+1))
		}
	}
	if len(problems) > 0 {
		return Default(), fmt.Errorf("%s: %s", name, strings.Join(problems, "; "))
	}
	return c, nil
}

// EnsureFile creates a file with this content if it doesn't exist, and never replaces one.
func EnsureFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
