package git

import (
	"context"
	"strconv"
	"strings"
)

// Entry is one row of the history. A row with no ID is graph lines only, which join the rows
// around it.
type Entry struct {
	Graph   string
	ID      string
	Author  string
	When    string // Relative, as git words it: "2 days ago".
	Refs    string
	Subject string
}

const sep = "\x1f"

// Log returns up to n commits after skipping skip, with the graph. Safe for the background.
// ponytail: each page asks git to walk past the earlier ones, so very deep pages of a huge history
// get slower. A cursor on the last commit id is the upgrade.
func Log(ctx context.Context, dir string, skip, n int) ([]Entry, error) {
	out, _, err := run(ctx, dir, false, readTimeout, "log", "--graph", "--branches", "--remotes", "--tags", "HEAD",
		"--max-count="+strconv.Itoa(n), "--skip="+strconv.Itoa(skip),
		"--format="+sep+"%H"+sep+"%an"+sep+"%ar"+sep+"%D"+sep+"%s", "--")
	if err != nil {
		if e, ok := err.(*Error); ok && (strings.Contains(e.Msg, "does not have any commits") || strings.Contains(e.Msg, "bad revision 'HEAD'") || strings.Contains(e.Msg, "ambiguous argument 'HEAD'")) {
			return nil, nil // A new repository has no history yet.
		}
		return nil, err
	}
	var commits []Entry
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		graph, rest, ok := strings.Cut(line, sep)
		if !ok {
			commits = append(commits, Entry{Graph: Clean(strings.TrimRight(graph, " "))})
			continue
		}
		f := strings.SplitN(rest, sep, 5)
		if len(f) != 5 {
			continue
		}
		commits = append(commits, Entry{Graph: Clean(graph), ID: f[0], Author: Clean(f[1]), When: Clean(f[2]), Refs: Clean(f[3]), Subject: Clean(f[4])})
	}
	return commits, nil
}

// Show returns a commit's details and the files it changed, as text. Safe for the background.
func Show(ctx context.Context, dir, id string) (string, error) {
	if !validID(id) {
		return "", &Error{Msg: "that isn't a commit id"}
	}
	out, _, err := run(ctx, dir, false, readTimeout, "show", "--no-ext-diff", "--no-textconv", "--no-color", "--stat",
		"--format=commit %H%nAuthor: %an <%ae>%nDate:   %ad%n%n%w(0,4,4)%B", id, "--")
	return sanitizeText(string(out)), err
}

func validID(id string) bool {
	if len(id) < 4 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// sanitizeText is Clean for many lines: it keeps line breaks and tabs.
func sanitizeText(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r", ""), "\n")
	for i, l := range lines {
		lines[i] = strings.ReplaceAll(Clean(l), "�", "?")
	}
	return strings.Join(lines, "\n")
}
