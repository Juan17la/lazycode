package project

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "projects.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDocumentedFormat(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	t.Setenv("USERPROFILE", "/home/test")
	got, err := Load(write(t, `
[[project]]
name = "Facebook"

[[project.directory]]
path = "~/code/frontend"

[[project.directory]]
path = "/srv/auth"
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Facebook" || len(got[0].Dirs) != 2 {
		t.Fatalf("got %+v", got)
	}
	if want := filepath.Clean("/home/test/code/frontend"); got[0].Dirs[0] != want {
		t.Errorf("~ not expanded: got %q, want %q", got[0].Dirs[0], want)
	}
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "none.toml"))
	if err != nil || got != nil {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestLoadRejectsBadFiles(t *testing.T) {
	for name, content := range map[string]string{
		"syntax":  "[[project",
		"no name": "[[project]]\n[[project.directory]]\npath = \"/a\"\n",
		"no dirs": "[[project]]\nname = \"x\"\n",
	} {
		if _, err := Load(write(t, content)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestLocate(t *testing.T) {
	projects := []Project{{Name: "a", Dirs: []string{"/x", "/y"}}}
	if _, p, d := Locate(projects, "/y"); p != 0 || d != 1 {
		t.Errorf("listed dir: got %d, %d", p, d)
	}
	all, p, d := Locate(projects, "/other")
	if len(all) != 2 || p != 1 || d != 0 || all[1].Dirs[0] != "/other" {
		t.Errorf("unlisted dir: got %+v, %d, %d", all, p, d)
	}
}
