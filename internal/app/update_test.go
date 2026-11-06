package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Juan17la/lazycode/internal/update"
	"github.com/gdamore/tcell/v2"
)

// fakeRelease serves tag as the latest release, with bin as its binary, and counts the questions.
func fakeRelease(t *testing.T, tag string, bin []byte) (*update.Updater, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	sum := sha256.Sum256(bin)
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		fmt.Fprintf(w, `{"tag_name": %q}`, tag)
	})
	mux.HandleFunc("/dl/"+tag+"/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), update.Asset())
	})
	mux.HandleFunc("/dl/"+tag+"/"+update.Asset(), func(w http.ResponseWriter, r *http.Request) { w.Write(bin) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &update.Updater{Latest: srv.URL + "/latest", Download: srv.URL + "/dl", Client: srv.Client()}, &hits
}

func TestUpdateOnDevBuildSaysItCant(t *testing.T) {
	u := start(t, nil)
	u.run("update")
	u.wait("development build")
}

func TestAvailableMessageAppearsOnce(t *testing.T) {
	up, hits := fakeRelease(t, "v0.2.0", nil)
	stamp := filepath.Join(t.TempDir(), "stamp")
	u := startTweaked(t, nil, func(a *app) {
		a.upd.version, a.upd.updater, a.upd.stamp = "v0.1.0", up, stamp
	})
	u.wait("LazyCode v0.2.0 is available. :update installs it.")
	if hits.Load() != 1 {
		t.Errorf("asked %d times", hits.Load())
	}
	if _, err := os.Stat(stamp); err != nil {
		t.Errorf("the time of the check wasn't kept: %v", err)
	}
	u.press("x") // Any key clears it, and it doesn't come back.
	time.Sleep(1500 * time.Millisecond)
	if got := u.text(); strings.Contains(got, "is available") {
		t.Error("the message came back")
	}
}

func TestUpdateCommandInstalls(t *testing.T) {
	bin := []byte("new program")
	up, hits := fakeRelease(t, "v0.2.0", bin)
	target := filepath.Join(t.TempDir(), "lazycode")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	u := startTweaked(t, nil, func(a *app) {
		// The check is off, and :update goes ahead anyway because the user asked.
		a.upd.version, a.upd.updater, a.upd.target = "v0.1.0", up, target
		a.upd.stamp = filepath.Join(t.TempDir(), "stamp")
		path := a.settingsPath("config.toml")
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte("[updates]\ncheck = false\n"), 0o644)
	})
	u.run("update")
	u.wait("Updated to v0.2.0. Restart LazyCode to use it.")
	if got, _ := os.ReadFile(target); string(got) != string(bin) {
		t.Errorf("target holds %q", got)
	}
	if hits.Load() != 1 {
		t.Errorf("asked %d times", hits.Load())
	}
}

func TestUpdateCommandUpToDate(t *testing.T) {
	up, _ := fakeRelease(t, "v0.1.0", nil)
	u := startTweaked(t, nil, func(a *app) { a.upd.version, a.upd.updater = "v0.1.0", up })
	u.run("update")
	u.wait("up to date")
}

func TestUpdateRunningTwiceIsRefused(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		fmt.Fprint(w, `{"tag_name": "v0.1.0"}`)
	}))
	defer srv.Close()
	defer close(release)
	up := &update.Updater{Latest: srv.URL, Download: srv.URL, Client: srv.Client()}
	u := startTweaked(t, nil, func(a *app) { a.upd.version, a.upd.updater = "v0.1.0", up })
	u.run("update")
	u.run("update")
	u.wait("already running")
	// Quitting while the request hangs must not hang the app: cleanup waits for it to exit.
	u.post(tcell.NewEventKey(tcell.KeyEsc, 0, 0))
}

// The automatic check, driven directly so the gates can be told apart.
func TestAutomaticCheckGates(t *testing.T) {
	cases := []struct {
		name    string
		version string
		on      bool
		prior   string // Content of the stamp file, if any.
		want    int32
	}{
		{"first time", "v0.1.0", true, "", 1},
		{"setting off", "v0.1.0", false, "", 0},
		{"dev build", "dev", true, "", 0},
		{"checked an hour ago", "v0.1.0", true, time.Now().Add(-time.Hour).Format(time.RFC3339), 0},
		{"checked two days ago", "v0.1.0", true, time.Now().Add(-48 * time.Hour).Format(time.RFC3339), 1},
		{"unreadable stamp", "v0.1.0", true, "garbage", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			up, hits := fakeRelease(t, "v0.1.0", nil) // Not newer, so nothing is posted.
			a := newApp(nil, nil, t.TempDir(), "")
			a.upd.version, a.upd.updater, a.upd.stamp = c.version, up, filepath.Join(t.TempDir(), "sub", "stamp")
			if c.prior != "" {
				os.MkdirAll(filepath.Dir(a.upd.stamp), 0o755)
				os.WriteFile(a.upd.stamp, []byte(c.prior), 0o644)
			}
			a.checkForUpdate(c.on)
			if hits.Load() != c.want {
				t.Errorf("asked %d times, want %d", hits.Load(), c.want)
			}
			if c.want == 1 {
				a.checkForUpdate(c.on) // The second start of the day.
				if hits.Load() != 1 {
					t.Errorf("asked again on the same day")
				}
			}
		})
	}
}

func TestFailedCheckStillRecordsTime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", 500) }))
	defer srv.Close()
	a := newApp(nil, nil, t.TempDir(), "")
	a.upd.version, a.upd.updater = "v0.1.0", &update.Updater{Latest: srv.URL, Client: srv.Client()}
	a.upd.stamp = filepath.Join(t.TempDir(), "stamp")
	a.checkForUpdate(true)
	if _, err := os.Stat(a.upd.stamp); err != nil {
		t.Error("a failed check should still be remembered")
	}
	if a.message != "" || a.upd.note != "" {
		t.Error("a failed automatic check should say nothing")
	}
}

func TestUpdateNoteWaitsForPrompt(t *testing.T) {
	a := newApp(nil, nil, t.TempDir(), "")
	a.upd.note = "LazyCode v9.9.9 is available. :update installs it."
	a.ask("Question?", choice{'y', "Yes", func() {}})
	a.showUpdateNote()
	if a.message != "" || a.upd.note == "" {
		t.Fatal("the message must wait while a question is showing")
	}
	a.prompts = nil
	a.showUpdateNote()
	if a.message == "" || a.upd.note != "" {
		t.Error("the message should show once nothing is asking")
	}
}
