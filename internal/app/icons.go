package app

import (
	"path/filepath"
	"strings"

	"github.com/Juan17la/lazycode/internal/git"
	"github.com/gdamore/tcell/v2"
)

// Icons come in two sets. A terminal app can't bring a font, so "basic" sticks to symbols most
// fonts have, and "nerd" needs a Nerd Font and shows empty boxes without one.
var (
	basicViews = map[string]string{viewFiles: "▤", viewGit: "⎇", viewSearch: "⌕", viewAI: "✻"}
	nerdViews  = map[string]string{viewFiles: "", viewGit: "", viewSearch: "", viewAI: ""}
)

// viewColour gives each view a colour of its own, and it's the same wherever the view is named.
var viewColour = map[string]string{viewFiles: "blue", viewGit: "orange", viewSearch: "yellow", viewAI: "purple"}

type fileIcon struct{ glyph, colour string }

var nerdFiles = map[string]fileIcon{
	".go": {"", "cyan"}, ".py": {"", "yellow"}, ".js": {"", "yellow"}, ".mjs": {"", "yellow"}, ".cjs": {"", "yellow"},
	".jsx": {"", "cyan"}, ".ts": {"", "blue"}, ".tsx": {"", "blue"}, ".json": {"", "yellow"},
	".md": {"", "blue"}, ".html": {"", "orange"}, ".css": {"", "blue"}, ".rs": {"", "orange"},
	".lua": {"", "blue"}, ".sh": {"", "green"}, ".bash": {"", "green"}, ".ps1": {"", "blue"},
	".toml": {"", "muted"}, ".yaml": {"", "purple"}, ".yml": {"", "purple"}, ".c": {"", "blue"},
	".h": {"", "purple"}, ".cpp": {"", "blue"}, ".java": {"", "red"}, ".png": {"", "purple"},
	".jpg": {"", "purple"}, ".svg": {"", "orange"}, ".lock": {"", "muted"}, ".sum": {"", "muted"}, ".mod": {"", "purple"},
}

// viewIcon returns a panel view's icon.
func (a *app) viewIcon(view string) string {
	if a.cfg.UI.Icons == "nerd" {
		return nerdViews[view]
	}
	return basicViews[view]
}

// iconFor returns a file's or folder's icon and its colour.
func (a *app) iconFor(name string, isDir, open bool) (string, tcell.Style) {
	nerd := a.cfg.UI.Icons == "nerd"
	switch {
	case isDir && !nerd:
		return "", a.th.primary // The arrow already says it's a folder.
	case isDir && open:
		return "", a.th.icon["blue"]
	case isDir:
		return "", a.th.icon["blue"]
	}
	lower := strings.ToLower(name)
	icon, known := nerdFiles[filepath.Ext(lower)]
	if strings.HasPrefix(lower, ".git") {
		icon, known = fileIcon{"", "orange"}, true
	}
	switch {
	case nerd && known:
		return icon.glyph, a.th.icon[icon.colour]
	case nerd:
		return "", a.th.icon["muted"]
	case known:
		return "•", a.th.icon[icon.colour]
	}
	return "•", a.th.icon["muted"]
}

// gitBadges is the tree's view of git status, rebuilt when a new status arrives.
type gitBadges struct {
	from      *git.Status
	files     map[string]byte // Full path to letter: A, M, D, R, U for untracked, ! for a conflict.
	folders   map[string]byte // Full path to the strongest state inside.
	untracked map[string]bool // Folders git lists as one untracked entry.
}

// badgeRank orders states, so a file or folder shows the one that matters most.
func badgeRank(b byte) int { return strings.IndexByte("URDAM!", b) }

func (a *app) badges() *gitBadges {
	g := a.git.st
	if g == nil || a.git.err != nil || a.git.dir != a.dirPath() {
		return nil
	}
	if a.gitMarks.from == g {
		return &a.gitMarks
	}
	b := gitBadges{from: g, files: map[string]byte{}, folders: map[string]byte{}, untracked: map[string]bool{}}
	stronger := func(m map[string]byte, key string, letter byte) {
		if old, ok := m[key]; !ok || badgeRank(letter) > badgeRank(old) {
			m[key] = letter
		}
	}
	for _, f := range g.Files {
		letter := f.Marker()
		switch letter {
		case '?':
			letter = 'U'
		case 'C':
			letter = 'A'
		}
		if f.Kind == git.Conflicted {
			letter = '!'
		}
		rel := filepath.FromSlash(f.Path)
		path := filepath.Join(g.Root, rel)
		if strings.HasSuffix(f.Path, "/") {
			b.untracked[path] = true
			stronger(b.folders, path, 'U')
		} else {
			stronger(b.files, path, letter)
		}
		for dir := filepath.Dir(path); len(dir) > len(g.Root); dir = filepath.Dir(dir) {
			stronger(b.folders, dir, letter)
		}
	}
	a.gitMarks = b
	return &a.gitMarks
}

// of returns the letter for a tree row, and 0 if it has none. A file in an untracked folder isn't
// listed by git, so its folders are looked at too. ponytail: only the active directory has status,
// so the combined tree colours just that one. Read a status per directory if that's missed.
func (b *gitBadges) of(path string, isDir bool) byte {
	if b == nil {
		return 0
	}
	if isDir {
		if letter, ok := b.folders[path]; ok {
			return letter
		}
	} else if letter, ok := b.files[path]; ok {
		return letter
	}
	for dir := filepath.Dir(path); len(dir) > len(b.from.Root); dir = filepath.Dir(dir) {
		if b.untracked[dir] {
			return 'U'
		}
	}
	return 0
}

func (a *app) badgeStyle(letter byte) tcell.Style {
	switch letter {
	case 'A', 'R':
		return a.th.gitAdded
	case 'M':
		return a.th.gitModified
	case 'U':
		return a.th.gitUntracked
	case '!', 'D':
		return a.th.gitConflict
	}
	return a.th.primary
}
