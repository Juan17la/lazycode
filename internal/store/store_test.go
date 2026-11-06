package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const hash40 = "0123456789abcdef0123456789abcdef01234567"

func archive(t *testing.T, files map[string]string, extra ...*tar.Header) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	w := tar.NewWriter(gz)
	w.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": hash40}}) //nolint:errcheck // Checked by Close.
	for _, h := range extra {
		if err := w.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range files {
		w.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "repo-abc/" + name, Mode: 0o644, Size: int64(len(content))}) //nolint:errcheck // Checked by Close.
		w.Write([]byte(content))                                                                                            //nolint:errcheck // Checked by Close.
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	gz.Close() //nolint:errcheck // Only memory.
	return buf.Bytes()
}

var goodFiles = map[string]string{
	"plugin.toml": "name = \"demo\"\nversion = \"1.0.0\"\napi_version = 1\ndoes = \"Demo\"\n",
	"main.lua":    "-- nothing",
	"lib/a.lua":   "return 1",
}

func checksumOf(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755) //nolint:errcheck // WriteFile fails next.
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sum, err := Checksum(dir)
	if err != nil {
		t.Fatal(err)
	}
	return sum
}

func serve(t *testing.T, data []byte) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/owner/repo/archive/"+hash40+".tar.gz") {
			http.NotFound(w, r)
			return
		}
		w.Write(data) //nolint:errcheck // A test.
	}))
	t.Cleanup(srv.Close)
	return &Client{GitHub: srv.URL, HTTP: srv.Client()}
}

func entry(sum string) Entry {
	return Entry{Name: "demo", Version: "1.0.0", Repository: "https://github.com/owner/repo", Commit: hash40, Checksum: sum}
}

func TestDownloadChecksThePin(t *testing.T) {
	c := serve(t, archive(t, goodFiles))
	parent := t.TempDir()
	dir, m, err := c.Download(context.Background(), entry(checksumOf(t, goodFiles)), parent)
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "demo" {
		t.Fatalf("got %+v", m)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "lib", "a.lua")); string(data) != "return 1" {
		t.Fatalf("top folder not removed: %q", data)
	}
	target := filepath.Join(parent, "demo")
	if err := Install(dir, target); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "stale"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	dir, _, err = c.Download(context.Background(), entry(checksumOf(t, goodFiles)), parent)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "stale")); err == nil {
		t.Fatal("the old copy wasn't replaced")
	}
	CleanLeftovers(parent)
	if entries, _ := os.ReadDir(parent); len(entries) != 1 {
		t.Fatalf("left behind: %v", entries)
	}
}

func TestDownloadRefusesAndLeavesNothing(t *testing.T) {
	sum := checksumOf(t, goodFiles)
	for name, tc := range map[string]struct {
		data []byte
		e    Entry
	}{
		"changed after review": {archive(t, map[string]string{"plugin.toml": goodFiles["plugin.toml"], "main.lua": "os.execute('bad')"}), entry(sum)},
		"other name":           {archive(t, goodFiles), func() Entry { e := entry(sum); e.Name = "other"; return e }()},
		"other version":        {archive(t, goodFiles), func() Entry { e := entry(sum); e.Version = "9"; return e }()},
		"escaping path":        {archive(t, goodFiles, &tar.Header{Typeflag: tar.TypeReg, Name: "repo/../../evil"}), entry(sum)},
		"link":                 {archive(t, goodFiles, &tar.Header{Typeflag: tar.TypeSymlink, Name: "repo/x", Linkname: "/etc"}), entry(sum)},
		"bad repository":       {nil, func() Entry { e := entry(sum); e.Repository = "http://evil.example/a/b"; return e }()},
		"short commit":         {nil, func() Entry { e := entry(sum); e.Commit = "abc"; return e }()},
		"name leaves folder":   {nil, func() Entry { e := entry(sum); e.Name = "../x"; return e }()},
	} {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			if _, _, err := serve(t, tc.data).Download(context.Background(), tc.e, parent); err == nil {
				t.Fatal("accepted")
			}
			if entries, _ := os.ReadDir(parent); len(entries) != 0 {
				t.Fatalf("left behind: %v", entries)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(parent), "evil")); err == nil {
				t.Fatal("wrote outside the folder")
			}
		})
	}
}

func TestParseKeepsGoodEntries(t *testing.T) {
	sum := strings.Repeat("a", 64)
	index := `[[plugin]]
name = "good"
version = "1"
description = "fine"
repository = "https://github.com/a/b"
commit = "` + hash40 + `"
checksum = "` + sum + `"
[[plugin]]
name = "../bad"
`
	entries, problems, err := Parse([]byte(index))
	if err != nil || len(entries) != 1 || len(problems) != 1 {
		t.Fatalf("%v %v %v", entries, problems, err)
	}
	if _, _, err := Parse([]byte("nope =")); err == nil {
		t.Fatal("a broken index must be an error")
	}
}
