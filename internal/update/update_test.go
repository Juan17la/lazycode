package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var bg = context.Background()

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		latest, current string
		want            bool
	}{
		{"v0.10.0", "v0.9.0", true},
		{"v0.9.0", "v0.10.0", false},
		{"v1.0.0", "v0.99.99", true},
		{"v0.1.1", "v0.1.0", true},
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.0", "dev", false},
		{"dev", "v0.1.0", false},
		{"v1.0", "v0.1.0", false},
		{"v1.0.0-rc1", "v0.1.0", false},
		{"1.0.0", "v0.1.0", false},
		{"v1.0.0\n", "v0.1.0", false},
		{"v99999999999999999999.0.0", "v0.1.0", false},
	} {
		if got := Newer(c.latest, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.latest, c.current, got)
		}
	}
}

// release serves a latest tag and, for that tag, the given binary and checksums.txt.
func release(t *testing.T, tag string, bin []byte, sums string) *Updater {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != "lazycode" || len(r.Header) > 3 {
			t.Errorf("unexpected request headers %v", r.Header)
		}
		fmt.Fprintf(w, `{"tag_name": %q}`, tag)
	})
	mux.HandleFunc("/dl/v1.2.3/"+Asset(), func(w http.ResponseWriter, r *http.Request) { w.Write(bin) })
	mux.HandleFunc("/dl/v1.2.3/checksums.txt", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, sums) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &Updater{Latest: srv.URL + "/latest", Download: srv.URL + "/dl", Client: srv.Client()}
}

func sumOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func target(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "lazycode")
	if err := os.WriteFile(p, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCheck(t *testing.T) {
	u := release(t, "v1.2.3", nil, "")
	if tag, err := u.Check(bg); err != nil || tag != "v1.2.3" {
		t.Errorf("Check = %q, %v", tag, err)
	}
	for _, bad := range []string{"../../evil", "v1.2", "", "v1.2.3/../x", "v1.2.3\n"} {
		if tag, err := release(t, bad, nil, "").Check(bg); err == nil {
			t.Errorf("tag %q should be refused, got %q", bad, tag)
		}
	}
}

func TestHostileTagBuildsNoLink(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()
	u := &Updater{Latest: srv.URL, Download: srv.URL, Client: srv.Client()}
	if err := u.Install(bg, "../../evil", target(t)); err == nil || hit {
		t.Errorf("a hostile tag must be refused before any request: err %v, requested %v", err, hit)
	}
}

func TestInstallReplacesAndKeepsExecutableBit(t *testing.T) {
	bin := []byte("new binary")
	u := release(t, "v1.2.3", bin, sumOf(bin)+"  "+Asset()+"\n"+"abc  other\n")
	path := target(t)
	if err := u.Install(bg, "v1.2.3", path); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, bin) {
		t.Errorf("target holds %q", got)
	}
	if st, _ := os.Stat(path); runtime.GOOS != "windows" && st.Mode()&0o111 == 0 {
		t.Errorf("mode %v lost the executable bit", st.Mode())
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".lazycode-update-*"))
	if len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
}

func TestBadChecksumLeavesTargetUntouched(t *testing.T) {
	bin := []byte("new binary")
	path := target(t)
	for name, sums := range map[string]string{
		"mismatch": sumOf([]byte("something else")) + "  " + Asset() + "\n",
		"missing":  sumOf(bin) + "  lazycode_plan9_arm\n",
		"empty":    "",
	} {
		u := release(t, "v1.2.3", bin, sums)
		if err := u.Install(bg, "v1.2.3", path); err == nil {
			t.Errorf("%s: install should fail", name)
		}
		if got, _ := os.ReadFile(path); string(got) != "old" {
			t.Errorf("%s: target changed to %q", name, got)
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("extra files next to the target: %v", entries)
	}
}

func TestDownloadSizeIsCapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, maxSmall+10))
	}))
	defer srv.Close()
	u := &Updater{Latest: srv.URL, Download: srv.URL, Client: srv.Client()}
	if _, err := u.Check(bg); err == nil {
		t.Error("an oversized answer should be refused")
	}
}

func TestReplaceAside(t *testing.T) {
	path := target(t)
	if err := os.WriteFile(path+".old", []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := replace(path, []byte("new"), true); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new" {
		t.Errorf("target holds %q", got)
	}
	if got, _ := os.ReadFile(path + ".old"); string(got) != "old" {
		t.Errorf(".old holds %q", got)
	}
}

func TestReplaceAsideFailureLeavesTarget(t *testing.T) {
	// A non-empty directory named target.old can't be removed or renamed over.
	path := target(t)
	if err := os.MkdirAll(filepath.Join(path+".old", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replace(path, []byte("new"), true); err == nil {
		t.Error("moving aside should fail")
	}
	if got, _ := os.ReadFile(path); string(got) != "old" {
		t.Errorf("target holds %q", got)
	}
}

func TestUnwritableDirectoryIsExplained(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions don't apply here")
	}
	path := target(t)
	dir := filepath.Dir(path)
	os.Chmod(dir, 0o555)
	defer os.Chmod(dir, 0o755)
	err := replace(path, []byte("new"), false)
	if err == nil || !strings.Contains(err.Error(), "can't write to "+dir) {
		t.Errorf("err = %v", err)
	}
}
