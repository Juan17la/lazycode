// Package config loads config.toml (Q-26).
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds the settings the editor uses so far.
type Config struct {
	Version int `toml:"version"`
	Editor  struct {
		AutoSave bool `toml:"auto_save"`
		TabSize  int  `toml:"tab_size"`
	} `toml:"editor"`
	UI struct {
		Theme   string `toml:"theme"`
		Sidebar string `toml:"sidebar"`
	} `toml:"ui"`
	Terminal struct {
		Scrollback int `toml:"scrollback"`
	} `toml:"terminal"`
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
	c.UI.Theme = "dark"
	c.UI.Sidebar = "left"
	c.Terminal.Scrollback = 1000
	return c
}

// Template is what a new config.toml starts with.
const Template = `# LazyCode settings. Save this file to apply them.
version = 1

[editor]
auto_save = false   # Save a second after typing stops.
tab_size = 4

[ui]
theme = "dark"      # dark or light
sidebar = "left"    # left or right

[terminal]
scrollback = 1000   # Lines kept per terminal tab. Each 1,000 lines cost about 36 MB.
`

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
		// These sections are documented, and read by features that aren't built yet.
		if !slices.Contains([]string{"sessions", "updates", "harness"}, key[0]) {
			problems = append(problems, "unknown setting "+key.String())
		}
	}
	if c.Version != 1 {
		problems = append(problems, fmt.Sprintf("version %d isn't supported", c.Version))
	}
	if c.Editor.TabSize < 1 || c.Editor.TabSize > maxTabSize {
		problems = append(problems, fmt.Sprintf("tab_size must be from 1 to %d", maxTabSize))
	}
	if c.UI.Theme != "dark" && c.UI.Theme != "light" {
		problems = append(problems, `theme must be "dark" or "light"`)
	}
	if c.UI.Sidebar != "left" && c.UI.Sidebar != "right" {
		problems = append(problems, `sidebar must be "left" or "right"`)
	}
	if c.Terminal.Scrollback < 1 || c.Terminal.Scrollback > maxScrollback {
		problems = append(problems, fmt.Sprintf("scrollback must be from 1 to %d", maxScrollback))
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
