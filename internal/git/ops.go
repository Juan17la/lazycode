package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ValidName checks a branch name typed by the user. A name starting with "-" would be read as an
// option by git, so it's refused before git sees it.
func ValidName(ctx context.Context, dir, name string) error {
	switch {
	case name == "":
		return &Error{Msg: "A name is needed."}
	case strings.HasPrefix(name, "-"):
		return &Error{Msg: "A branch name can't start with -."}
	case strings.IndexFunc(name, func(r rune) bool { return r < ' ' || r == 0x7f }) >= 0:
		return &Error{Msg: "A branch name can't hold control characters."}
	}
	if _, _, err := run(ctx, dir, false, readTimeout, "check-ref-format", "refs/heads/"+name); err != nil {
		if _, ok := err.(*Error); ok {
			return &Error{Msg: "Git doesn't allow that as a branch name."}
		}
		return err
	}
	return nil
}

// Diff returns the changes to one file as text. An untracked file is shown as all new. It runs
// no external diff or conversion program, so it's safe for the background.
func Diff(ctx context.Context, root string, f File) (text string, truncated bool, err error) {
	if f.Kind == Untracked {
		return untrackedDiff(root, f.Path), false, nil
	}
	args := []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color"}
	if f.Kind == Staged {
		args = append(args, "--cached")
	}
	out, cut, err := run(ctx, root, false, readTimeout, append(args, "--", f.Path)...)
	if err != nil {
		return "", false, err
	}
	text = sanitizeText(string(out))
	if strings.TrimSpace(text) == "" {
		text = "No changes to show for " + Clean(f.Path) + ".\n"
	}
	return text, cut, nil
}

func untrackedDiff(root, path string) string {
	full := filepath.Join(root, path)
	info, err := os.Lstat(full)
	switch {
	case err != nil:
		return "Can't read " + Clean(path) + ": " + Clean(err.Error()) + "\n"
	case info.IsDir():
		return Clean(path) + " is a new folder. Stage it to add everything in it.\n"
	case !info.Mode().IsRegular():
		return Clean(path) + " is a link or special file, so it isn't shown.\n"
	case info.Size() > 1<<20:
		return Clean(path) + " is new, and too large to show here.\n"
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "Can't read " + Clean(path) + ": " + Clean(err.Error()) + "\n"
	}
	if slices.Contains(data[:min(len(data), 8192)], 0) {
		return "New binary file " + Clean(path) + "\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "new file: %s\n", Clean(path))
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		b.WriteString("+" + line + "\n")
	}
	return sanitizeText(b.String())
}

// The functions below can run what the repository configures: hooks, clean filters.

func write(ctx context.Context, root string, args ...string) (string, error) {
	out, _, err := run(ctx, root, true, writeTimeout, args...)
	return strings.TrimSpace(Clean(string(out))), err
}

// Init makes a repository in dir.
func Init(ctx context.Context, dir string) error {
	_, err := write(ctx, dir, "init")
	return err
}

// Stage adds files, or their removal, to the next commit. paths are as in File.Path.
func Stage(ctx context.Context, root string, paths ...string) error {
	_, err := write(ctx, root, append([]string{"add", "--"}, paths...)...)
	return err
}

// Unstage takes files out of the next commit and leaves the changes in them.
func Unstage(ctx context.Context, root string, hasCommits bool, paths ...string) error {
	args := []string{"restore", "--staged", "--"}
	if !hasCommits {
		args = []string{"rm", "--cached", "-q", "--"} // There's no commit to restore from yet.
	}
	_, err := write(ctx, root, append(args, paths...)...)
	return err
}

// Commit makes a commit of what's staged. The message goes in as one argument, so it can start with "-".
func Commit(ctx context.Context, root, message string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", &Error{Msg: "A commit needs a message."}
	}
	return write(ctx, root, "commit", "-m", message)
}

func network(ctx context.Context, root string, args ...string) (string, error) {
	out, _, err := run(ctx, root, true, networkTimeout, args...)
	return strings.TrimSpace(Clean(string(out))), err
}

// Fetch gets what's new from the remotes without changing any branch.
func Fetch(ctx context.Context, root string) (string, error) { return network(ctx, root, "fetch") }

// Pull merges what's new into the branch. It never rebases and never forces, and a merge that
// conflicts stops for the user to resolve.
func Pull(ctx context.Context, root string) (string, error) {
	return network(ctx, root, "pull", "--no-rebase", "--no-edit")
}

// Push sends the branch. With setUpstream, it also makes origin its upstream, for a branch that has none.
func Push(ctx context.Context, root, branch string, setUpstream bool) (string, error) {
	if setUpstream {
		if err := ValidName(ctx, root, branch); err != nil {
			return "", err
		}
		return network(ctx, root, "push", "--set-upstream", "origin", branch)
	}
	return network(ctx, root, "push")
}

// Remotes lists the names of the remotes.
func Remotes(ctx context.Context, root string) ([]string, error) {
	out, _, err := run(ctx, root, false, readTimeout, "remote")
	return strings.Fields(string(out)), err
}

// Branch is a local branch.
type Branch struct {
	Name     string
	Current  bool
	Upstream string
}

// Branches lists the local branches. A name git would read as an option is left out, because a
// repository can contain one that only looks like a branch.
func Branches(ctx context.Context, root string) ([]Branch, error) {
	out, _, err := run(ctx, root, false, readTimeout, "for-each-ref", "--format=%(refname:short)"+sep+"%(HEAD)"+sep+"%(upstream:short)", "refs/heads/")
	if err != nil {
		return nil, err
	}
	var list []Branch
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Split(line, sep)
		if len(f) != 3 || f[0] == "" || strings.HasPrefix(f[0], "-") {
			continue
		}
		list = append(list, Branch{Name: f[0], Current: f[1] == "*", Upstream: Clean(f[2])})
	}
	return list, nil
}

// Switch changes to an existing branch. Git refuses if that would lose uncommitted work, and its message says so.
func Switch(ctx context.Context, root, name string) error {
	if err := ValidName(ctx, root, name); err != nil {
		return err
	}
	_, err := write(ctx, root, "switch", name)
	return err
}

// CreateBranch makes a branch at the current commit and switches to it.
func CreateBranch(ctx context.Context, root, name string) error {
	if err := ValidName(ctx, root, name); err != nil {
		return err
	}
	_, err := write(ctx, root, "switch", "-c", name)
	return err
}

// Merge merges a branch into the current one. A conflict leaves the merge open: see Status.Merging.
func Merge(ctx context.Context, root, name string) (string, error) {
	if err := ValidName(ctx, root, name); err != nil {
		return "", err
	}
	return write(ctx, root, "merge", "--no-edit", name)
}

// MergeAbort gives up an unfinished merge, which puts back what the files held before it.
func MergeAbort(ctx context.Context, root string) error {
	_, err := write(ctx, root, "merge", "--abort")
	return err
}

// ListFiles returns every file in dir that isn't ignored, tracked or not, relative to dir. In a
// folder that isn't a repository it returns ErrNotRepo. Safe for the background.
func ListFiles(ctx context.Context, dir string) ([]string, error) {
	out, _, err := runCap(ctx, dir, false, readTimeout*3, 512<<20, "ls-files", "-co", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			files = append(files, p)
		}
	}
	return files, nil
}
