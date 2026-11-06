// Package trust remembers which directories may run code (doc 11). Opening an untrusted folder
// is safe, but running its project or an AI tool in it isn't.
package trust

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/BurntSushi/toml"
)

// Store is the list of trusted directories, in one file.
type Store struct{ path string }

// New uses the file at path, which may not exist yet.
func New(path string) *Store { return &Store{path} }

func canonical(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	return dir
}

// file is trust.toml. Declined holds folders the user said no to, so they aren't asked about again.
type file struct {
	Directories []string `toml:"directories"`
	Declined    []string `toml:"declined,omitempty"`
}

func (s *Store) load() (file, error) {
	var f file
	if _, err := toml.DecodeFile(s.path, &f); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return file{}, nil
		}
		return file{}, fmt.Errorf("%s: %w", filepath.Base(s.path), err)
	}
	return f, nil
}

// Trusted reports whether dir was trusted. Only that exact directory counts, not those inside
// it. A file it can't read means no, which only costs a question.
func (s *Store) Trusted(dir string) bool {
	f, err := s.load()
	return err == nil && slices.Contains(f.Directories, canonical(dir))
}

// Undecided reports whether dir has never been answered for. A file it can't read means no: the
// answer couldn't be saved, so the question would come back on every start.
func (s *Store) Undecided(dir string) bool {
	f, err := s.load()
	dir = canonical(dir)
	return err == nil && !slices.Contains(f.Directories, dir) && !slices.Contains(f.Declined, dir)
}

// Add trusts dir.
func (s *Store) Add(dir string) error {
	f, err := s.load()
	if err != nil {
		return err // A broken file isn't replaced, in case it can be fixed.
	}
	dir = canonical(dir)
	f.Directories, f.Declined = with(f.Directories, dir), without(f.Declined, dir)
	return s.save(f)
}

// Remove stops trusting dir, and remembers that so opening it doesn't ask again.
func (s *Store) Remove(dir string) error {
	f, err := s.load()
	if err != nil {
		return err
	}
	dir = canonical(dir)
	f.Directories, f.Declined = without(f.Directories, dir), with(f.Declined, dir)
	return s.save(f)
}

func with(list []string, dir string) []string {
	if slices.Contains(list, dir) {
		return list
	}
	return append(list, dir)
}

func without(list []string, dir string) []string {
	return slices.DeleteFunc(list, func(entry string) bool { return entry == dir })
}

func (s *Store) save(f file) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(s.path), ".trust-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name()) //nolint:errcheck // Already renamed, unless something failed.
	if err := toml.NewEncoder(temp).Encode(f); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), s.path)
}
