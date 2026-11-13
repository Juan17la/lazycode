// Package store reads the plugin store's index and downloads what it lists (doc 09). An entry pins
// a commit and a checksum, so a plugin can't change after review. Everything from the network is
// untrusted until it matches the entry, and nothing here runs a plugin.
package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/Juan17la/lazycode/internal/plugin"
)

const (
	indexURL   = "https://raw.githubusercontent.com/Juan17la/lazycode-store/main/index.toml"
	githubURL  = "https://github.com"
	maxIndex   = 1 << 20
	maxArchive = 20 << 20
	maxFiles   = 1000
	maxContent = 50 << 20 // What an archive may unpack to, so a small one can't fill the disk.
)

// Entry is one plugin in index.toml.
type Entry struct {
	Name        string `toml:"name"`
	Version     string `toml:"version"`
	Description string `toml:"description"`
	Repository  string `toml:"repository"`
	Commit      string `toml:"commit"`
	Checksum    string `toml:"checksum"`
}

var (
	repoPattern = regexp.MustCompile(`^https://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)$`)
	commitHash  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	sumPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Check reports what's wrong with an entry. Its fields end up in a file path and a link.
func (e Entry) Check() error {
	switch {
	case !plugin.ValidName(e.Name):
		return fmt.Errorf("name %q isn't valid", e.Name)
	case e.Version == "" || plugin.Clean(e.Version) != e.Version:
		return fmt.Errorf("%s: version %q isn't valid", e.Name, e.Version)
	case !repoPattern.MatchString(e.Repository):
		return fmt.Errorf("%s: repository must be https://github.com/owner/name", e.Name)
	case !commitHash.MatchString(e.Commit):
		return fmt.Errorf("%s: commit must be a full lowercase hash", e.Name)
	case !sumPattern.MatchString(e.Checksum):
		return fmt.Errorf("%s: checksum must be a lowercase SHA-256", e.Name)
	}
	return nil
}

// Client holds the addresses, so tests can use a local server.
type Client struct {
	Index  string
	GitHub string // Archives are at GitHub/owner/name/archive/commit.tar.gz.
	HTTP   *http.Client
}

// New returns a Client for the real store. Redirects must stay on HTTPS, as GitHub sends archives
// on to another host.
func New() *Client {
	return &Client{Index: indexURL, GitHub: githubURL, HTTP: &http.Client{
		Timeout: 2 * time.Minute,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if req.URL.Scheme != "https" {
				return errors.New("a download was sent to a link that isn't HTTPS")
			}
			return nil
		},
	}}
}

// get reads at most limit bytes. It sends nothing but a generic User-Agent (doc 11).
func (c *Client) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "lazycode")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // Read only.
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", req.URL.Host, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("the download is larger than allowed")
	}
	return data, nil
}

// Parse reads index.toml, with every entry's problems so the store's CI can report them.
func Parse(data []byte) ([]Entry, []error, error) {
	var index struct {
		Plugin []Entry `toml:"plugin"`
	}
	if err := toml.Unmarshal(data, &index); err != nil {
		return nil, nil, fmt.Errorf("index.toml: %w", err)
	}
	var good []Entry
	var problems []error
	for _, e := range index.Plugin {
		if err := e.Check(); err != nil {
			problems = append(problems, err)
			continue
		}
		e.Description = plugin.Clean(e.Description)
		good = append(good, e)
	}
	return good, problems, nil
}

// List returns the store's valid entries. One bad entry doesn't hide the rest.
func (c *Client) List(ctx context.Context) ([]Entry, error) {
	data, err := c.get(ctx, c.Index, maxIndex)
	if err != nil {
		return nil, err
	}
	entries, _, err := Parse(data)
	return entries, err
}

// Download unpacks an entry into a new hidden folder in parent, and returns it if it's exactly what
// the entry pinned. The caller moves it into place with Install, or removes it.
func (c *Client) Download(ctx context.Context, e Entry, parent string) (string, plugin.Manifest, error) {
	var m plugin.Manifest
	if err := e.Check(); err != nil {
		return "", m, err
	}
	url := strings.TrimSuffix(c.GitHub, "/") + strings.TrimPrefix(e.Repository, githubURL) + "/archive/" + e.Commit + ".tar.gz"
	data, err := c.get(ctx, url, maxArchive)
	if err != nil {
		return "", m, err
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", m, err
	}
	dir, err := os.MkdirTemp(parent, leftoverPrefix+"*")
	if err != nil {
		return "", m, err
	}
	fail := func(err error) (string, plugin.Manifest, error) {
		os.RemoveAll(dir) //nolint:errcheck // The first error is the one to report.
		return "", m, err
	}
	if err := unpack(data, dir); err != nil {
		return fail(fmt.Errorf("%s: %w", e.Name, err))
	}
	sum, err := Checksum(dir)
	if err != nil {
		return fail(err)
	}
	if sum != e.Checksum {
		return fail(fmt.Errorf("%s doesn't match its checksum, so nothing was installed", e.Name))
	}
	if m, err = plugin.ReadManifest(dir); err != nil {
		return fail(err)
	}
	if m.Name != e.Name || m.Version != e.Version {
		return fail(fmt.Errorf("the store lists %s %s, but the download is %s %s", e.Name, e.Version, m.Name, m.Version))
	}
	return dir, m, nil
}

// unpack takes a GitHub archive, whose files are all in one top folder. Only plain files and
// folders are allowed: a link could point a later file outside dir.
func unpack(data []byte, dir string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close() //nolint:errcheck // Read only.
	r := tar.NewReader(gz)
	total, files := int64(0), 0
	for {
		h, err := r.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		_, rel, _ := strings.Cut(path.Clean(h.Name), "/")
		if rel == "" {
			continue // The top folder.
		}
		if !filepath.IsLocal(filepath.FromSlash(rel)) {
			return fmt.Errorf("the archive has an unsafe path: %q", h.Name)
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			files++
			if total += h.Size; total > maxContent || files > maxFiles {
				return errors.New("the archive is larger than allowed")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, io.LimitReader(r, h.Size))
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("the archive has a link or special file: %q", h.Name)
		}
	}
}

// Checksum hashes a plugin's files by path, size and content, so it doesn't depend on how GitHub
// compresses an archive. A .git folder is skipped, so an author can run it on a checkout.
func Checksum(dir string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		switch {
		case d.IsDir() && d.Name() == ".git":
			return filepath.SkipDir
		case d.IsDir():
			return nil
		case !d.Type().IsRegular():
			return fmt.Errorf("%s is a link or special file, which plugins can't have", rel)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%s\x00%d\x00", filepath.ToSlash(rel), len(data))
		hash.Write(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

const leftoverPrefix = ".store-"

// Install moves a downloaded folder to target, keeping the old copy until the new one is there.
func Install(downloaded, target string) error {
	old := ""
	if _, err := os.Stat(target); err == nil {
		tmp, err := os.MkdirTemp(filepath.Dir(target), leftoverPrefix+"*")
		if err != nil {
			return err
		}
		old = filepath.Join(tmp, "old")
		if err := os.Rename(target, old); err != nil {
			os.RemoveAll(tmp) //nolint:errcheck // The rename error is the one to report.
			return err
		}
		defer os.RemoveAll(tmp) //nolint:errcheck // CleanLeftovers tries again next start.
	}
	if err := os.Rename(downloaded, target); err != nil {
		if old != "" {
			os.Rename(old, target) //nolint:errcheck // Best effort to put it back.
		}
		return err
	}
	return nil
}

// CleanLeftovers removes what an interrupted install left behind.
func CleanLeftovers(parent string) {
	entries, _ := os.ReadDir(parent) //nolint:errcheck // Nothing to clean, then.
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), leftoverPrefix) {
			os.RemoveAll(filepath.Join(parent, entry.Name())) //nolint:errcheck // Tried again next start.
		}
	}
}
