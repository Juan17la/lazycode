// Package project loads projects and their directories from projects.toml.
package project

import (
	"bytes"
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
	Name    string
	Dirs    []string
	Unsaved bool // Made up for a folder that's in no project, and not in the file.
}

// Template is what a new projects.toml starts with.
const Template = `# LazyCode projects. Save this file to apply it. A directory can be in several projects.
# LazyCode rewrites this file for :project new, :dir add and :dir remove, without comments.
#
# [[project]]
# name = "Facebook"
#
# [[project.directory]]
# path = "~/code/frontend-facebook"
`

// file is the layout of projects.toml.
type file struct {
	Project []projectEntry `toml:"project"`
}

type projectEntry struct {
	Name      string           `toml:"name"`
	Directory []directoryEntry `toml:"directory"`
}

type directoryEntry struct {
	Path string `toml:"path"`
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
	var saved file
	if _, err := toml.DecodeFile(path, &saved); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Project
	for i, p := range saved.Project {
		if p.Name == "" {
			return nil, fmt.Errorf("%s: project %d has no name", path, i+1)
		}
		if len(p.Directory) == 0 {
			return nil, fmt.Errorf("%s: project %q has no directories", path, p.Name)
		}
		project := Project{Name: p.Name}
		for _, d := range p.Directory {
			project.Dirs = append(project.Dirs, Expand(d.Path))
		}
		out = append(out, project)
	}
	return out, nil
}

// Save writes projects to path in one step, leaving out any that are Unsaved. Paths under
// the home folder keep their ~, so that the file can move between machines.
func Save(path string, projects []Project) error {
	var saved file
	for _, p := range projects {
		if p.Unsaved {
			continue
		}
		entry := projectEntry{Name: p.Name}
		for _, d := range p.Dirs {
			entry.Directory = append(entry.Directory, directoryEntry{collapse(d)})
		}
		saved.Project = append(saved.Project, entry)
	}
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(saved); err != nil {
		return err
	}
	return writeAtomic(path, b.Bytes(), 0o644)
}

func collapse(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + filepath.ToSlash(rest)
	}
	return path
}

// writeAtomic replaces path in one step, so a crash leaves the old file or the new one.
// A symlink stays a symlink, as people keep config files in a dotfiles folder.
func writeAtomic(path string, data []byte, perm fs.FileMode) error {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name()) //nolint:errcheck // Already renamed, unless this failed.
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(f.Name(), perm); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Expand turns a leading ~ into the home folder, and cleans the path.
func Expand(path string) string {
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
	return append(projects, Project{Name: "Current folder", Dirs: []string{dir}, Unsaved: true}), len(projects), 0
}
