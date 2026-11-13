// Package run works out what :play runs (Q-56): a command remembered for the directory, or one
// detected from the project's files.
package run

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/BurntSushi/toml"
)

// Detect proposes a command for a TypeScript, JavaScript, Python or Go project (FR-10.2).
func Detect(dir string) (string, bool) {
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}
	if command, ok := detectNode(dir, exists); ok {
		return command, true
	}
	if exists("go.mod") {
		return detectGo(dir)
	}
	python := "python3"
	if runtime.GOOS == "windows" {
		python = "python"
	}
	for _, name := range []string{"main.py", "app.py"} {
		if exists(name) {
			return python + " " + name, true
		}
	}
	return "", false
}

func detectNode(dir string, exists func(string) bool) (string, bool) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		switch {
		case exists("index.js"):
			return "node index.js", true
		case exists("index.ts"):
			return "npx tsx index.ts", true
		}
		return "", false
	}
	manager := "npm"
	for lock, name := range map[string]string{"pnpm-lock.yaml": "pnpm", "yarn.lock": "yarn", "bun.lock": "bun", "bun.lockb": "bun"} {
		if exists(lock) {
			manager = name
		}
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
		Main    string            `json:"main"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", false // Asking beats guessing from a broken file.
	}
	for _, script := range []string{"dev", "start"} {
		if pkg.Scripts[script] != "" {
			return manager + " run " + script, true
		}
	}
	switch {
	case exists("index.js"):
		return "node index.js", true
	case exists("index.ts"):
		return "npx tsx index.ts", true
	}
	return "", false
}

// detectGo finds the main package: this folder's, or the only one under cmd.
func detectGo(dir string) (string, bool) {
	if matches, _ := filepath.Glob(filepath.Join(dir, "*.go")); len(matches) > 0 {
		return "go run .", true
	}
	entries, err := os.ReadDir(filepath.Join(dir, "cmd"))
	if err != nil {
		return "", false
	}
	var commands []string
	for _, entry := range entries {
		if entry.IsDir() {
			commands = append(commands, entry.Name())
		}
	}
	if len(commands) == 1 {
		return "go run ./cmd/" + commands[0], true
	}
	return "", false // Several commands, and any guess could be wrong.
}

// Shell returns how to run a command line that the user wrote or confirmed.
func Shell(command string) []string {
	if runtime.GOOS == "windows" {
		shell := os.Getenv("COMSPEC")
		if shell == "" {
			shell = "cmd.exe"
		}
		return []string{shell, "/C", command}
	}
	return []string{"sh", "-c", command}
}

// Saved holds the directories.toml file, which LazyCode writes (doc 08).
type Saved struct{ path string }

// NewSaved uses the file at path, which may not exist yet.
func NewSaved(path string) *Saved { return &Saved{path} }

type savedFile struct {
	Directory []savedDirectory `toml:"directory"`
}

type savedDirectory struct {
	Path string `toml:"path"`
	Play string `toml:"play"`
}

func (s *Saved) load() (savedFile, error) {
	var file savedFile
	if _, err := toml.DecodeFile(s.path, &file); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return file, fmt.Errorf("%s: %w", filepath.Base(s.path), err)
	}
	return file, nil
}

// Command returns the command remembered for dir.
func (s *Saved) Command(dir string) (string, bool, error) {
	file, err := s.load()
	if err != nil {
		return "", false, err
	}
	i := slices.IndexFunc(file.Directory, func(d savedDirectory) bool { return d.Path == dir })
	if i < 0 || file.Directory[i].Play == "" {
		return "", false, nil
	}
	return file.Directory[i].Play, true, nil
}

// Remember saves the command for dir. A broken file is reported and not replaced.
func (s *Saved) Remember(dir, command string) error {
	file, err := s.load()
	if err != nil {
		return err
	}
	if i := slices.IndexFunc(file.Directory, func(d savedDirectory) bool { return d.Path == dir }); i >= 0 {
		file.Directory[i].Play = command
	} else {
		file.Directory = append(file.Directory, savedDirectory{dir, command})
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(s.path), ".directories-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name()) //nolint:errcheck // Already renamed, unless something failed.
	if err := toml.NewEncoder(temp).Encode(file); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), s.path)
}
