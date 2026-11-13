package lsp

import (
	"os"
	"path/filepath"
	"strings"
)

// Server says how to run a language server. LazyCode doesn't bundle any, and never runs through a
// shell: the command and its arguments are fixed here.
type Server struct {
	Name    string
	Command string
	Args    []string
	Install string   // What gets it. Only onboarding runs this, after the user confirms (Q-68).
	Markers []string // Files that mark the root of a project for this server.
}

var (
	gopls    = Server{"gopls", "gopls", nil, "go install golang.org/x/tools/gopls@latest", []string{"go.work", "go.mod"}}
	pyright  = Server{"pyright", "pyright-langserver", []string{"--stdio"}, "npm install -g pyright", []string{"pyrightconfig.json", "pyproject.toml", "setup.py", "setup.cfg", "requirements.txt"}}
	tsserver = Server{"typescript-language-server", "typescript-language-server", []string{"--stdio"}, "npm install -g typescript-language-server typescript", []string{"tsconfig.json", "jsconfig.json", "package.json"}}
)

// Language is a language with a server configured by default (FR-4.1).
type Language struct {
	Name   string
	Server Server
}

// Languages is what onboarding offers to install.
var Languages = []Language{{"Go", gopls}, {"Python", pyright}, {"TypeScript, JS", tsserver}}

type added struct {
	srv Server
	id  string
}

// packs holds servers from language packs, by extension. Only the UI goroutine uses it.
var packs = map[string]added{}

// SetPacks replaces the language pack servers. Built-in languages can't be replaced.
func SetPacks(languages []Pack) {
	packs = map[string]added{}
	for _, l := range languages {
		for _, ext := range l.Extensions {
			packs["."+strings.ToLower(strings.TrimPrefix(ext, "."))] = added{l.Server, strings.ToLower(l.Server.Name)}
		}
	}
}

// Pack is a language added by a language pack.
type Pack struct {
	Extensions []string
	Server     Server // Its Name is also the language ID.
}

// ForPath returns the server for a file and the language ID it expects, by extension.
func ForPath(path string) (srv Server, languageID string, ok bool) {
	ext := strings.ToLower(filepath.Ext(path))
	if srv, id, ok := builtIn(ext); ok {
		return srv, id, true
	}
	if p, ok := packs[ext]; ok {
		return p.srv, p.id, true
	}
	return Server{}, "", false
}

func builtIn(ext string) (srv Server, languageID string, ok bool) {
	switch ext {
	case ".go":
		return gopls, "go", true
	case ".py":
		return pyright, "python", true
	case ".ts":
		return tsserver, "typescript", true
	case ".tsx":
		return tsserver, "typescriptreact", true
	case ".js", ".mjs", ".cjs":
		return tsserver, "javascript", true
	case ".jsx":
		return tsserver, "javascriptreact", true
	}
	return Server{}, "", false
}

// Root returns the folder to start the server in: the nearest one above file, no higher than
// boundary, holding one of the server's marker files. Without one, it's boundary, or the file's own
// folder if the file lies outside it. One server runs for each server and root.
func Root(srv Server, file, boundary string) string {
	dir := filepath.Dir(file)
	rel, err := filepath.Rel(boundary, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return boundary
	}
	for {
		for _, marker := range srv.Markers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}
		if dir == boundary || filepath.Dir(dir) == dir {
			return boundary
		}
		dir = filepath.Dir(dir)
	}
}
