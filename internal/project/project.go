// Package project loads projects and their directories from projects.toml.
package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Project is a named group of directories.
type Project struct {
	Name string
	Dirs []string
}

// ConfigPath returns where projects.toml lives on this OS.
func ConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lazycode", "projects.toml"), nil
}

// Load reads projects from path. A missing file means no projects yet. A broken
// file returns an error, and is never rewritten.
func Load(path string) ([]Project, error) {
	var file struct {
		Project []struct {
			Name      string
			Directory []struct{ Path string }
		}
	}
	if _, err := toml.DecodeFile(path, &file); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Project
	for i, p := range file.Project {
		if p.Name == "" {
			return nil, fmt.Errorf("%s: project %d has no name", path, i+1)
		}
		if len(p.Directory) == 0 {
			return nil, fmt.Errorf("%s: project %q has no directories", path, p.Name)
		}
		project := Project{Name: p.Name}
		for _, d := range p.Directory {
			project.Dirs = append(project.Dirs, expand(d.Path))
		}
		out = append(out, project)
	}
	return out, nil
}

func expand(path string) string {
	if rest, ok := strings.CutPrefix(path, "~"); ok && (rest == "" || rest[0] == '/' || rest[0] == filepath.Separator) {
		if home, err := os.UserHomeDir(); err == nil {
			path = home + rest
		}
	}
	return filepath.Clean(path)
}

// Locate finds the project holding dir. If none does, it adds one for dir, so that
// LazyCode always opens on the folder it was started in.
func Locate(projects []Project, dir string) (all []Project, projectIdx, dirIdx int) {
	for p, project := range projects {
		for d, path := range project.Dirs {
			if path == dir {
				return projects, p, d
			}
		}
	}
	return append(projects, Project{Name: "Current folder", Dirs: []string{dir}}), len(projects), 0
}
