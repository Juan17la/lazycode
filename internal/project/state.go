package project

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// State is what LazyCode remembers between runs (Q-39, Q-73). It's one file rather than
// one per project, because tabs belong to directories, and a directory can be in several
// projects (P-23).
// ponytail: the last window to close wins when two are open. Per-window state if it matters.
type State struct {
	Active      map[string]string   `json:"active_directories,omitempty"` // Project name to path.
	Directories map[string]DirState `json:"directories,omitempty"`
	ShowSecond  bool                `json:"show_second,omitempty"`
	FilesPanel  int                 `json:"files_panel,omitempty"`
	ShowAll     bool                `json:"show_all,omitempty"`
}

// DirState is one directory's layout and open folders.
type DirState struct {
	Layout      *Split   `json:"layout,omitempty"`
	OpenFolders []string `json:"open_folders,omitempty"`
}

// Split is a pane with its tabs, or two children.
type Split struct {
	Tabs       []Tab  `json:"tabs,omitempty"`
	Cur        int    `json:"cur,omitempty"`
	Focus      bool   `json:"focus,omitempty"`
	SideBySide bool   `json:"side_by_side,omitempty"`
	First      *Split `json:"first,omitempty"`
	Second     *Split `json:"second,omitempty"`
}

// Tab is an open file and where its view was.
type Tab struct {
	Path   string `json:"path"`
	Line   int    `json:"line,omitempty"`
	Col    int    `json:"col,omitempty"`
	Scroll int    `json:"scroll,omitempty"`
}

// StatePath returns where the state lives. The OS can clear it, which only loses what
// was open (doc 08).
func StatePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lazycode", "state.json"), nil
}

// LoadState reads the state. A missing file means nothing saved yet.
func LoadState(path string) (State, error) {
	var s State
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, err
	}
	return s, nil
}

// SaveState writes the state in one step, readable only by the user since it lists paths.
func SaveState(path string, s State) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return writeAtomic(path, data, 0o600)
}
