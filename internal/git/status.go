package git

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
)

// File is one changed path. Path is relative to the repository root, and is raw bytes: names
// don't have to be valid UTF-8, so show it with Clean and use it as is.
type File struct {
	Path string
	Orig string // The old name, for a rename.
	X, Y byte   // Staged and unstaged state, as in git status: M A D R C U, or '.'.
	Kind Kind
}

// Kind says which list of the panel a file is in. A file changed both ways is two Files.
type Kind int

const (
	Conflicted Kind = iota
	Staged
	Unstaged
	Untracked
)

// Status is the state of the repository at dir.
type Status struct {
	Root       string
	Branch     string // Empty when detached.
	Detached   bool
	Head       string // The commit id, or "" before the first commit.
	HasCommits bool
	Upstream   string
	Ahead      int
	Behind     int
	Merging    bool
	Files      []File
}

// Root returns the top folder of the repository holding dir, or ErrNotRepo.
func Root(ctx context.Context, dir string) (string, error) {
	out, _, err := run(ctx, dir, false, readTimeout, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return filepath.FromSlash(strings.TrimRight(string(out), "\r\n")), nil // Git writes / on Windows too.
}

// ReadStatus gets the status. Safe for the background: see the package comment.
func ReadStatus(ctx context.Context, dir string) (*Status, error) {
	root, err := Root(ctx, dir)
	if err != nil {
		return nil, err
	}
	// Untracked folders are shown as one entry, so a tree with no .gitignore doesn't list a million files.
	out, _, err := run(ctx, root, false, readTimeout, "status", "--porcelain=v2", "-z", "--branch", "--untracked-files=normal")
	if err != nil {
		return nil, err
	}
	st := parseStatus(out)
	st.Root = root
	if _, _, err := run(ctx, root, false, readTimeout, "rev-parse", "-q", "--verify", "MERGE_HEAD"); err == nil {
		st.Merging = true
	}
	return st, nil
}

func parseStatus(out []byte) *Status {
	st := &Status{}
	records := strings.Split(string(out), "\x00")
	for i := 0; i < len(records); i++ {
		r := records[i]
		switch {
		case strings.HasPrefix(r, "# branch.oid "):
			if oid := strings.TrimPrefix(r, "# branch.oid "); oid != "(initial)" {
				st.Head, st.HasCommits = oid, true
			}
		case strings.HasPrefix(r, "# branch.head "):
			if name := strings.TrimPrefix(r, "# branch.head "); name == "(detached)" {
				st.Detached = true
			} else {
				st.Branch = name
			}
		case strings.HasPrefix(r, "# branch.upstream "):
			st.Upstream = strings.TrimPrefix(r, "# branch.upstream ")
		case strings.HasPrefix(r, "# branch.ab "):
			f := strings.Fields(r)
			if len(f) == 4 {
				st.Ahead, _ = strconv.Atoi(strings.TrimPrefix(f[2], "+"))
				st.Behind, _ = strconv.Atoi(strings.TrimPrefix(f[3], "-"))
			}
		case strings.HasPrefix(r, "1 "): // 1 XY sub mH mI mW hH hI path
			if f := strings.SplitN(r, " ", 9); len(f) == 9 {
				st.add(f[1], f[8], "")
			}
		case strings.HasPrefix(r, "2 "): // 2 XY sub mH mI mW hH hI Xscore path, then the old path
			if f := strings.SplitN(r, " ", 10); len(f) == 10 {
				orig := ""
				if i+1 < len(records) {
					i++
					orig = records[i]
				}
				st.add(f[1], f[9], orig)
			}
		case strings.HasPrefix(r, "u "): // u XY sub m1 m2 m3 mW h1 h2 h3 path
			if f := strings.SplitN(r, " ", 11); len(f) == 11 {
				st.Files = append(st.Files, File{Path: f[10], X: f[1][0], Y: f[1][1], Kind: Conflicted})
			}
		case strings.HasPrefix(r, "? "):
			st.Files = append(st.Files, File{Path: r[2:], X: '?', Y: '?', Kind: Untracked})
		}
	}
	return st
}

func (st *Status) add(xy, path, orig string) {
	if len(xy) != 2 {
		return
	}
	if xy[0] != '.' {
		st.Files = append(st.Files, File{Path: path, Orig: orig, X: xy[0], Y: xy[1], Kind: Staged})
	}
	if xy[1] != '.' {
		st.Files = append(st.Files, File{Path: path, Orig: orig, X: xy[0], Y: xy[1], Kind: Unstaged})
	}
}

// Marker is the letter shown beside a file: its state in the list it's in.
func (f File) Marker() byte {
	switch f.Kind {
	case Staged:
		return f.X
	case Unstaged:
		return f.Y
	case Untracked:
		return '?'
	}
	return 'U'
}
