// Package update finds a newer release on GitHub and installs it (Q-71, Q-79). Releases are
// unsigned, so the only protection is HTTPS from github.com and the SHA-256 in checksums.txt (doc 11).
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Where releases live. Tests point these at a local server.
const (
	latestURL   = "https://api.github.com/repos/Juan17la/lazycode/releases/latest"
	downloadURL = "https://github.com/Juan17la/lazycode/releases/download"
)

const (
	maxBinary = 200 << 20 // A release binary is about 10 MB.
	maxSmall  = 1 << 20
)

// Updater holds the addresses, so tests can use a local server.
type Updater struct {
	Latest   string // The API address for the latest release.
	Download string // Assets are at Download/<tag>/<name>.
	Client   *http.Client
}

// New returns an Updater for the real releases. Redirects must stay on HTTPS, as GitHub sends
// downloads on to its storage host.
func New() *Updater {
	return &Updater{Latest: latestURL, Download: downloadURL, Client: &http.Client{
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if req.URL.Scheme != "https" {
				return errors.New("a download was sent to a link that isn't HTTPS")
			}
			return nil
		},
	}}
}

var tagPattern = regexp.MustCompile(`^v(\d{1,9})\.(\d{1,9})\.(\d{1,9})$`)

func parse(tag string) ([3]int, bool) {
	m := tagPattern.FindStringSubmatch(tag)
	if m == nil {
		return [3]int{}, false
	}
	var v [3]int
	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return v, true
}

// Newer reports whether latest is a later version than current. Anything that isn't vX.Y.Z,
// such as "dev", is never newer and never older.
func Newer(latest, current string) bool {
	l, ok1 := parse(latest)
	c, ok2 := parse(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// get reads at most limit bytes. It sends nothing but a generic User-Agent (doc 11).
func (u *Updater) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "lazycode")
	resp, err := u.Client.Do(req)
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
		return nil, errors.New("the download is larger than expected")
	}
	return data, nil
}

// Check returns the tag of the latest release. The tag is remote input, so it has to be
// a plain vX.Y.Z before anything builds a link from it.
func (u *Updater) Check(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	data, err := u.get(ctx, u.Latest, maxSmall)
	if err != nil {
		return "", err
	}
	var rel struct {
		Tag string `json:"tag_name"`
	}
	if err := json.Unmarshal(data, &rel); err != nil {
		return "", err
	}
	if _, ok := parse(rel.Tag); !ok {
		return "", fmt.Errorf("the latest release has a version I can't read: %q", rel.Tag)
	}
	return rel.Tag, nil
}

// Asset is the file name for this system in every release.
func Asset() string {
	name := "lazycode_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// Executable is the file that's running, with symlinks resolved so the real file is replaced.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// Install downloads tag's binary and replaces target with it. The checksum is checked first, and
// any failure leaves target as it was.
func (u *Updater) Install(ctx context.Context, tag, target string) error {
	if _, ok := parse(tag); !ok {
		return fmt.Errorf("refusing the version %q", tag)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	name := Asset()
	sums, err := u.get(ctx, u.Download+"/"+tag+"/checksums.txt", maxSmall)
	if err != nil {
		return err
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			want = strings.ToLower(f[0])
		}
	}
	if want == "" {
		return fmt.Errorf("checksums.txt has no entry for %s, so nothing was installed", name)
	}
	bin, err := u.get(ctx, u.Download+"/"+tag+"/"+name, maxBinary)
	if err != nil {
		return err
	}
	if sum := sha256.Sum256(bin); hex.EncodeToString(sum[:]) != want {
		return errors.New("the download doesn't match its checksum, so nothing was installed")
	}
	return replace(target, bin, runtime.GOOS == "windows")
}

// replace writes data next to target and renames it over target, so a crash never leaves half a file.
// A running Windows exe can be renamed but not overwritten, so with aside the old file moves to
// target.old first, and comes back if the new one can't take its place.
func replace(target string, data []byte, aside bool) error {
	dir := filepath.Dir(target)
	mode := os.FileMode(0o755)
	if st, err := os.Stat(target); err == nil {
		mode = st.Mode().Perm() | 0o111
	}
	tmp, err := os.CreateTemp(dir, ".lazycode-update-*")
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("can't write to %s, so LazyCode can't update itself there. Reinstall it, or run the install script again", dir)
		}
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // Already gone after a rename.
	if _, err := tmp.Write(data); err != nil {
		tmp.Close() //nolint:errcheck // The write error is the one to report.
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	if !aside {
		return os.Rename(tmp.Name(), target)
	}
	old := target + ".old"
	os.Remove(old) //nolint:errcheck // A leftover from the last update, if it's there.
	if err := os.Rename(target, old); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		os.Rename(old, target) //nolint:errcheck // Best effort to put it back.
		return err
	}
	return nil
}

// CleanOld removes the file a Windows update moved aside, once the old program has exited.
func CleanOld() {
	if runtime.GOOS != "windows" {
		return
	}
	if exe, err := Executable(); err == nil {
		os.Remove(exe + ".old") //nolint:errcheck // Missing is the usual case.
	}
}
