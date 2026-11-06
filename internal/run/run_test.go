package run

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func project(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestDetect(t *testing.T) {
	python := "python3"
	if runtime.GOOS == "windows" {
		python = "python"
	}
	for name, tc := range map[string]struct {
		files map[string]string
		want  string
	}{
		"empty":        {nil, ""},
		"rust":         {map[string]string{"Cargo.toml": ""}, ""},
		"npm dev":      {map[string]string{"package.json": `{"scripts":{"dev":"vite","start":"node ."}}`}, "npm run dev"},
		"npm start":    {map[string]string{"package.json": `{"scripts":{"start":"node ."}}`}, "npm run start"},
		"pnpm":         {map[string]string{"package.json": `{"scripts":{"dev":"x"}}`, "pnpm-lock.yaml": ""}, "pnpm run dev"},
		"yarn":         {map[string]string{"package.json": `{"scripts":{"dev":"x"}}`, "yarn.lock": ""}, "yarn run dev"},
		"bun":          {map[string]string{"package.json": `{"scripts":{"dev":"x"}}`, "bun.lock": ""}, "bun run dev"},
		"no scripts":   {map[string]string{"package.json": `{}`, "index.js": ""}, "node index.js"},
		"ts file":      {map[string]string{"index.ts": ""}, "npx tsx index.ts"},
		"js file":      {map[string]string{"index.js": ""}, "node index.js"},
		"nothing runs": {map[string]string{"package.json": `{}`}, ""},
		"broken json":  {map[string]string{"package.json": `{broken`, "index.js": ""}, ""},
		"go root":      {map[string]string{"go.mod": "", "main.go": ""}, "go run ."},
		"go one cmd":   {map[string]string{"go.mod": "", "cmd/tool/main.go": ""}, "go run ./cmd/tool"},
		"go two cmds":  {map[string]string{"go.mod": "", "cmd/a/main.go": "", "cmd/b/main.go": ""}, ""},
		"go library":   {map[string]string{"go.mod": "", "pkg/x.go": ""}, ""},
		"python main":  {map[string]string{"main.py": ""}, python + " main.py"},
		"python app":   {map[string]string{"app.py": ""}, python + " app.py"},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := Detect(project(t, tc.files))
			if got != tc.want || ok != (tc.want != "") {
				t.Fatalf("got %q, %v, want %q", got, ok, tc.want)
			}
		})
	}
}

func TestRememberedCommands(t *testing.T) {
	s := NewSaved(filepath.Join(t.TempDir(), "config", "directories.toml"))
	if _, ok, err := s.Command("/a"); ok || err != nil {
		t.Fatalf("before saving: %v, %v", ok, err)
	}
	for _, step := range [][2]string{{"/a", "cargo build"}, {"/b", `echo "quoted" && ls`}, {"/a", "cargo run"}} {
		if err := s.Remember(step[0], step[1]); err != nil {
			t.Fatal(err)
		}
	}
	reread := NewSaved(s.path)
	for dir, want := range map[string]string{"/a": "cargo run", "/b": `echo "quoted" && ls`} {
		if got, ok, err := reread.Command(dir); !ok || err != nil || got != want {
			t.Fatalf("%s: %q, %v, %v", dir, got, ok, err)
		}
	}
}

func TestBrokenFileIsntReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "directories.toml")
	if err := os.WriteFile(path, []byte("[[broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewSaved(path)
	if _, _, err := s.Command("/a"); err == nil {
		t.Fatal("no error reading")
	}
	if err := s.Remember("/a", "x"); err == nil {
		t.Fatal("no error writing")
	}
	if got, _ := os.ReadFile(path); string(got) != "[[broken" {
		t.Fatalf("replaced: %q", got)
	}
}
