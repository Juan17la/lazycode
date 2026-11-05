package lsp

import (
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

// FileURI returns the file: URI for an absolute path.
func FileURI(path string) string { return fileURI(path, runtime.GOOS == "windows") }

// PathOf returns the path a file: URI names.
func PathOf(uri string) (string, error) { return pathOf(uri, runtime.GOOS == "windows") }

// fileURI takes windows as an argument so that both forms can be tested anywhere.
func fileURI(path string, windows bool) string {
	if windows {
		path = strings.ReplaceAll(path, `\`, "/")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path // C:/x becomes /C:/x.
		}
	}
	// ponytail: a Windows network path (\\host\share) isn't given a host, so servers see a wrong path.
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func pathOf(uri string, windows bool) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("bad URI %q: %w", truncate(uri), err)
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("%q isn't a file", truncate(uri))
	}
	if u.Host != "" && u.Host != "localhost" {
		return "", fmt.Errorf("%q is on another machine", truncate(uri))
	}
	path := u.Path
	if windows {
		if len(path) >= 3 && path[0] == '/' && path[2] == ':' { // /c:/x, as gopls writes it.
			path = path[1:]
		}
		return strings.ReplaceAll(path, "/", `\`), nil
	}
	return filepath.Clean(path), nil
}
