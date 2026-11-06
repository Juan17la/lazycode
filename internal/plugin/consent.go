package plugin

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Consents remembers the user's answers, per plugin and version (doc 09). A new version asks
// again, because the code changed.
type Consents struct {
	path string
	file consentFile
}

type consentFile struct {
	Accepted map[string]string `toml:"accepted"`
	Declined map[string]string `toml:"declined"`
}

// OpenConsents reads the file at path, which may not exist yet. If it can't be read nothing
// counts as accepted, and Set returns an error rather than replacing it.
func OpenConsents(path string) (*Consents, error) {
	c := &Consents{path: path}
	if _, err := toml.DecodeFile(path, &c.file); err != nil && !errors.Is(err, fs.ErrNotExist) {
		c.file = consentFile{}
		return c, err
	}
	return c, nil
}

// Accepted reports whether the user accepted this version.
func (c *Consents) Accepted(m Manifest) bool { return c.file.Accepted[m.Name] == m.Version }

// Declined reports whether the user declined this version.
func (c *Consents) Declined(m Manifest) bool { return c.file.Declined[m.Name] == m.Version }

// Set records an answer.
func (c *Consents) Set(m Manifest, accepted bool) error {
	if _, err := toml.DecodeFile(c.path, &consentFile{}); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if c.file.Accepted == nil {
		c.file.Accepted, c.file.Declined = map[string]string{}, map[string]string{}
	}
	delete(c.file.Accepted, m.Name)
	delete(c.file.Declined, m.Name)
	if accepted {
		c.file.Accepted[m.Name] = m.Version
	} else {
		c.file.Declined[m.Name] = m.Version
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(c.path), ".plugins-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name()) //nolint:errcheck // Already renamed, unless something failed.
	if err := toml.NewEncoder(temp).Encode(c.file); err != nil {
		temp.Close() //nolint:errcheck // The encode error is the one to report.
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), c.path)
}
