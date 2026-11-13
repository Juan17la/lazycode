package lsp

import (
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// This one runs the real gopls, on a module made for the test. It's skipped where gopls isn't installed.
func TestRealGopls(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls isn't installed")
	}
	if testing.Short() {
		t.Skip("starts gopls")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// The home and caches stay inside the test, so that nothing of the user's is touched. A Go
	// toolchain that's already installed is used as it is, and downloads nothing (no dependencies).
	home := t.TempDir()
	for _, name := range []string{"HOME", "XDG_CACHE_HOME", "XDG_CONFIG_HOME", "GOCACHE", "GOPATH", "LocalAppData", "AppData"} {
		t.Setenv(name, filepath.Join(home, name))
	}
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOPLS_NO_ANALYTICS", "1")
	src := "package main\n\nimport \"fmt\"\n\n// Greet says hi.\nfunc Greet(name string) string {\n\treturn \"héllo 😀 \" + name\n}\n\nfunc main() {\n\tmsg := Greet(\"x\")  \n\tfmt.Println(msg,   Greet(\"y\"))\n}\n"
	other := "package main\n\nfunc other() string { return Greet(\"z\") }\n"
	files := map[string]string{"go.mod": "module example.com/t\n\ngo 1.21\n", "main.go": src, "other.go": other}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	main := filepath.Join(dir, "main.go")
	c, err := Start(gopls, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Shutdown()
	if err := c.Sync(main, "go", []byte(src)); err != nil {
		t.Fatal(err)
	}
	doc := NewDoc([]byte(src))

	// Definition of Greet, from its use in main.
	use := strings.Index(src, "Greet(\"y\")")
	locs, err := c.Definition(main, doc.Position(use))
	if err != nil || len(locs) != 1 {
		t.Fatalf("definition: %v %v", locs, err)
	}
	def := doc.Offset(locs[0].Range.Start)
	if locs[0].Path != main || !strings.HasPrefix(src[def:], "Greet(name") {
		t.Errorf("definition at %s:%d", locs[0].Path, def)
	}

	// Rename it. The use in other.go is a second file, which isn't open.
	edits, err := c.Rename(main, doc.Position(def), "Hello")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if len(edits[main]) != 4 /* the doc comment too */ || len(edits[filepath.Join(dir, "other.go")]) != 1 {
		t.Errorf("rename edits: %+v", edits)
	}

	// Format: applying the edits must give what gofmt gives.
	fmtEdits, err := c.Format(main, 4, false)
	if err != nil || len(fmtEdits) == 0 {
		t.Fatalf("format: %v %v", fmtEdits, err)
	}
	got := []byte(src)
	slices.SortFunc(fmtEdits, func(a, b TextEdit) int { return doc.Offset(b.Range.Start) - doc.Offset(a.Range.Start) })
	for _, e := range fmtEdits { // Last first, so that earlier offsets stay right.
		start, end := doc.Offset(e.Range.Start), doc.Offset(e.Range.End)
		got = slices.Concat(got[:start], []byte(e.NewText), got[end:])
	}
	want, err := format.Source([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("formatted:\n%s\nwant:\n%s", got, want)
	}
}
