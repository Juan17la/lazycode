package project

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestSaveThenLoadKeepsProjectsAndHomePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := filepath.Join(home, "config", "projects.toml")
	want := []Project{{Name: "a b", Dirs: []string{filepath.Join(home, "code"), filepath.Join(filepath.Dir(home), "x y")}}, {Name: "c", Dirs: []string{filepath.Join(home, "code")}}}
	all := append(want[:2:2], Project{Name: "Current folder", Dirs: []string{"/tmp"}, Unsaved: true})
	if err := Save(path, all); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"~/code"`) || strings.Contains(string(data), "Current folder") {
		t.Errorf("file:\n%s", data)
	}
	got, err := Load(path)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, %v, want %+v", got, err, want)
	}
}

func TestSaveWritesThroughASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs extra rights on Windows")
	}
	dir := t.TempDir()
	real, link := filepath.Join(dir, "dotfiles.toml"), filepath.Join(dir, "projects.toml")
	if err := os.WriteFile(real, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := Save(link, []Project{{Name: "x", Dirs: []string{"/a"}}}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link was replaced: %v, %v", info, err)
	}
	if got, err := Load(real); err != nil || len(got) != 1 {
		t.Errorf("target: %+v, %v", got, err)
	}
}

func TestStateRoundTripAndBadFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "state.json")
	if s, err := LoadState(path); err != nil || s.Directories != nil {
		t.Errorf("missing file: %+v, %v", s, err)
	}
	want := State{
		Active:     map[string]string{"p": "/a"},
		ShowSecond: true,
		Directories: map[string]DirState{"/a": {
			OpenFolders: []string{"/a/sub"},
			Layout:      &Split{SideBySide: true, First: &Split{Tabs: []Tab{{Path: "/a/x", Line: 3, Col: 2, Scroll: 1}}, Focus: true}, Second: &Split{Cur: 1, Tabs: []Tab{{Path: "/a/y"}, {Path: "/a/z"}}}},
		}},
	}
	if err := SaveState(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(path)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, %v", got, err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(path); err == nil {
		t.Error("a corrupt file gave no error")
	}
}
