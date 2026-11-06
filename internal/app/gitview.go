package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Juan17la/lazycode/internal/buffer"
	"github.com/Juan17la/lazycode/internal/git"
	"github.com/Juan17la/lazycode/internal/github"
	"github.com/gdamore/tcell/v2"
)

const (
	logPage = 150 // History is loaded this many commits at a time, so a huge repository costs one page.
	// pollEvery is how often the Git view checks the repository while it's on screen. A slow repository
	// is checked less often, in proportion to how long each check takes.
	pollEvery = 3 * time.Second
)

const (
	rowHeader = iota
	rowFile
	rowCommit
	rowGraph // Graph lines between commits.
	rowNote
)

type gitRow struct {
	kind  int
	text  string
	file  git.File
	entry git.Entry
}

func (r gitRow) selectable() bool { return r.kind == rowFile || r.kind == rowCommit }
func (r gitRow) height() int {
	if r.kind == rowCommit {
		return 2 // The subject, then who and when.
	}
	return 1
}

// key identifies a row across refreshes, so the selection stays on it.
func (r gitRow) key() string {
	switch r.kind {
	case rowFile:
		return fmt.Sprintf("f%d%s", r.file.Kind, r.file.Path)
	case rowCommit:
		return "c" + r.entry.ID
	}
	return ""
}

// gitState is the Git view.
type gitState struct {
	dir     string // The folder this describes.
	st      *git.Status
	err     error // Not a repository, git missing, or git's own message.
	loaded  bool
	loading bool
	log     []git.Entry
	logDone bool // The whole history is loaded.
	logBusy bool
	logSig  string // What the loaded history was for. A change reloads it.
	commits int    // Entries in log that are commits.
	rows    []gitRow
	sel     int
	top     int
	busy    string // What's running, for the status line.
	last    string // How the last operation went, kept in the panel.
	next    time.Time
	afters  []func() // To run once the refresh in progress, and any asked for meanwhile, is in.
	again   bool     // Another refresh was asked for while one ran, so that one may be out of date.
}

func (g *gitState) selected() (gitRow, bool) {
	if g.sel >= 0 && g.sel < len(g.rows) && g.rows[g.sel].selectable() {
		return g.rows[g.sel], true
	}
	return gitRow{}, false
}

// rebuild lays out the lists, keeping the selection on the same file or commit if it's still there.
func (g *gitState) rebuild() {
	var keep string
	if r, ok := g.selected(); ok {
		keep = r.key()
	}
	rows := g.rows[:0]
	if st := g.st; st != nil {
		sections := []struct {
			kind  git.Kind
			title string
		}{{git.Conflicted, "Merge conflicts"}, {git.Staged, "Staged"}, {git.Unstaged, "Changes"}, {git.Untracked, "Untracked"}}
		files := 0
		for _, sec := range sections {
			var in []git.File
			for _, f := range st.Files {
				if f.Kind == sec.kind {
					in = append(in, f)
				}
			}
			if len(in) == 0 {
				continue
			}
			files += len(in)
			rows = append(rows, gitRow{kind: rowHeader, text: fmt.Sprintf("%s (%d)", sec.title, len(in))})
			for _, f := range in {
				rows = append(rows, gitRow{kind: rowFile, file: f})
			}
		}
		if files == 0 {
			rows = append(rows, gitRow{kind: rowNote, text: "Nothing changed"})
		}
		rows = append(rows, gitRow{kind: rowHeader, text: "History"})
		if !st.HasCommits {
			rows = append(rows, gitRow{kind: rowNote, text: "No commits yet"})
		}
		for _, e := range g.log {
			if e.ID == "" {
				rows = append(rows, gitRow{kind: rowGraph, entry: e})
			} else {
				rows = append(rows, gitRow{kind: rowCommit, entry: e})
			}
		}
		if !g.logDone && len(g.log) > 0 {
			rows = append(rows, gitRow{kind: rowNote, text: "Loading more..."})
		}
	}
	g.rows = rows
	g.sel = min(g.sel, max(0, len(rows)-1))
	if keep != "" {
		for i, r := range rows {
			if r.key() == keep {
				g.sel = i
				return
			}
		}
	}
	g.settle(1)
}

func (g *gitState) settle(dir int) {
	for _, d := range []int{dir, -dir} {
		for i := g.sel; i >= 0 && i < len(g.rows); i += d {
			if g.rows[i].selectable() {
				g.sel = i
				return
			}
		}
	}
}

func (g *gitState) move(delta int) {
	step := 1
	if delta < 0 {
		step = -1
	}
	for n := abs(delta); n > 0; n-- {
		next := g.sel + step
		for next >= 0 && next < len(g.rows) && !g.rows[next].selectable() {
			next += step
		}
		if next < 0 || next >= len(g.rows) {
			break
		}
		g.sel = next
	}
}

// Loading.

// gitVisible reports whether the Git view is showing in a panel that's on screen.
func (a *app) gitVisible() bool {
	return a.viewIn(0) == viewGit || (a.showSecond && a.viewIn(1) == viewGit)
}

// filesVisible reports whether the tree is showing, since it colours files by their Git state.
func (a *app) filesVisible() bool {
	return a.viewIn(0) == viewFiles || (a.showSecond && a.viewIn(1) == viewFiles)
}

// gitTick runs once a second. While the view is showing, it looks at the repository again now and then,
// which is how commits and edits made elsewhere appear (doc 06).
func (a *app) gitTick(now time.Time) {
	g := &a.git
	if !a.gitVisible() && !a.filesVisible() || g.loading || g.busy != "" {
		return
	}
	if g.dir != a.dirPath() {
		*g = gitState{dir: a.dirPath()} // A different directory: nothing here is about it.
	}
	if !g.loaded || now.After(g.next) {
		a.refreshGit(false, nil)
	}
}

// refreshGit reads the status, and the first page of history if it changed or force is set, off the UI
// goroutine. after runs when it's in.
func (a *app) refreshGit(force bool, after func()) {
	g := &a.git
	if after != nil {
		g.afters = append(g.afters, after)
	}
	if g.loading {
		g.again = g.again || force || after != nil // The one running may have read before the change.
		return
	}
	g.loading = true
	dir := a.dirPath()
	lastSig, hadLog := g.logSig, g.loaded && g.dir == dir
	g.dir = dir
	go func() {
		started := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		st, err := git.ReadStatus(ctx, dir)
		var log []git.Entry
		sig := ""
		reloaded := false
		if err == nil {
			sig = fmt.Sprintf("%s|%s|%d|%d|%s", st.Head, st.Branch, st.Ahead, st.Behind, st.Upstream)
			if force || !hadLog || sig != lastSig {
				log, err = git.Log(ctx, dir, 0, logPage)
				reloaded = true
			}
		}
		took := time.Since(started)
		a.post(func() {
			g.loading = false
			if g.dir != a.dirPath() {
				return // The directory changed while this ran, and gitTick starts over.
			}
			g.loaded = true
			g.next = time.Now().Add(max(pollEvery, 4*took))
			g.st, g.err = st, err
			if err == nil && reloaded {
				g.log, g.logSig = log, sig
				g.commits = countCommits(log)
				g.logDone = g.commits < logPage
			}
			if g.st == nil {
				g.log, g.logSig, g.commits = nil, "", 0
			}
			g.rebuild()
			a.requestRedraw()
			if g.again {
				g.again = false
				a.refreshGit(true, nil)
				return
			}
			afters := g.afters
			g.afters = nil
			for _, f := range afters {
				f()
			}
		})
	}()
}

func countCommits(log []git.Entry) int {
	n := 0
	for _, e := range log {
		if e.ID != "" {
			n++
		}
	}
	return n
}

// moreHistory loads the next page when the selection reaches the end of what's loaded.
func (a *app) moreHistory() {
	g := &a.git
	if g.logDone || g.logBusy || g.st == nil || g.sel < len(g.rows)-4 {
		return
	}
	g.logBusy = true
	dir, skip, sig := g.dir, g.commits, g.logSig
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		more, err := git.Log(ctx, dir, skip, logPage)
		a.post(func() {
			g.logBusy = false
			if g.logSig != sig || g.dir != dir {
				return // The history was reloaded meanwhile.
			}
			if err != nil {
				g.logDone = true
				g.last = "Can't load more history: " + shown(err.Error())
				return
			}
			g.log = append(g.log, more...)
			n := countCommits(more)
			g.commits += n
			g.logDone = n < logPage
			g.rebuild()
			a.requestRedraw()
		})
	}()
}

// Running things.

// background runs op off the UI goroutine, with its progress on the status line. Anything that can
// run the repository's own programs asks for trust first (doc 11). done runs on the UI goroutine.
func (a *app) background(label string, needTrust bool, op func(ctx context.Context) (string, error), done func(out string, err error)) {
	g := &a.git
	start := func() {
		if g.busy != "" {
			a.message = "Wait for " + g.busy
			return
		}
		g.busy = label + "..."
		go func() {
			out, err := op(context.Background())
			a.post(func() {
				g.busy = ""
				done(out, err)
			})
		}()
	}
	if needTrust {
		a.requireTrust(a.dirPath(), "git "+label, start)
		return
	}
	start()
}

// gitOp runs a Git operation and reports how it went, with the new ahead and behind counts.
func (a *app) gitOp(label string, needTrust bool, op func(ctx context.Context, root string) (string, error)) {
	root := a.gitRoot()
	if root == "" {
		a.message = "This isn't a Git repository yet. Alt+G shows how to make one."
		return
	}
	a.background(label, needTrust, func(ctx context.Context) (string, error) { return op(ctx, root) }, func(_ string, err error) {
		a.refreshGit(true, func() {
			a.message = a.gitOutcome(label, err)
			a.git.last = a.message
		})
	})
}

// gitRoot is the repository's folder as last read, or "" if there isn't one.
func (a *app) gitRoot() string {
	if a.git.st != nil && a.git.dir == a.dirPath() {
		return a.git.st.Root
	}
	return ""
}

func (a *app) gitOutcome(label string, err error) string {
	g := &a.git
	if err == nil {
		return label + " done. " + syncText(g.st)
	}
	text := shown(err.Error())
	if git.NeedsTerminal(err) {
		text = "It needs a password or key passphrase, which LazyCode can't ask for. Run git in a terminal tab (:term) once to sign in."
	}
	if g.st != nil && g.st.Merging && (label == "pull" || label == "merge") {
		n := 0
		for _, f := range g.st.Files {
			if f.Kind == git.Conflicted {
				n++
			}
		}
		if n > 0 {
			return fmt.Sprintf("%s stopped with %d conflicting files. Open each, fix it, then s marks it resolved.", label, n)
		}
	}
	return label + " failed: " + text
}

// syncText says where the branch stands against its upstream.
func syncText(st *git.Status) string {
	switch {
	case st == nil:
		return ""
	case st.Detached:
		return "HEAD is detached."
	case st.Upstream == "":
		return st.Branch + " has no upstream yet."
	case st.Ahead == 0 && st.Behind == 0:
		return st.Branch + " is up to date with " + shown(st.Upstream) + "."
	}
	return fmt.Sprintf("%s is %d ahead and %d behind %s.", st.Branch, st.Ahead, st.Behind, shown(st.Upstream))
}

func (a *app) gitPush() {
	st := a.git.st
	switch {
	case a.gitRoot() == "":
		a.message = "This isn't a Git repository yet. Alt+G shows how to make one."
	case st.Detached:
		a.message = "HEAD is detached, so there's no branch to push. Switch to one first (b)."
	case st.Upstream == "":
		branch := st.Branch
		a.ask(fmt.Sprintf("%s has no upstream. Push it to origin and track it?", shown(branch)),
			choice{'y', "Push", func() {
				a.gitOp("push", true, func(ctx context.Context, root string) (string, error) {
					if remotes, err := git.Remotes(ctx, root); err == nil && len(remotes) == 0 {
						return "", errors.New("there's no remote yet. :repo create makes one on GitHub")
					}
					return git.Push(ctx, root, branch, true)
				})
			}},
			choice{'c', "Cancel", func() {}})
	default:
		a.gitOp("push", true, func(ctx context.Context, root string) (string, error) { return git.Push(ctx, root, "", false) })
	}
}

func (a *app) gitPull() {
	a.gitOp("pull", true, func(ctx context.Context, root string) (string, error) { return git.Pull(ctx, root) })
}

func (a *app) gitFetch() {
	a.gitOp("fetch", true, func(ctx context.Context, root string) (string, error) { return git.Fetch(ctx, root) })
}

func (a *app) gitCommit() {
	st := a.git.st
	if a.gitRoot() == "" {
		a.message = "This isn't a Git repository yet. Alt+G shows how to make one."
		return
	}
	staged := 0
	for _, f := range st.Files {
		if f.Kind == git.Staged {
			staged++
		}
	}
	if staged == 0 && !st.Merging {
		a.message = "Nothing is staged. s stages the selected file."
		return
	}
	a.ask1Line(fmt.Sprintf("Commit message (%d files):", staged), "", func(msg string) {
		a.gitOp("commit", true, func(ctx context.Context, root string) (string, error) { return git.Commit(ctx, root, msg) })
	})
}

// stageSelected stages the selected file, or marks a conflicted one resolved.
func (a *app) stageSelected() {
	r, ok := a.git.selected()
	if !ok || r.kind != rowFile {
		a.message = "Select a file first."
		return
	}
	f := r.file
	if f.Kind == git.Staged {
		a.message = "That's staged already. u unstages it."
		return
	}
	a.gitOp("stage", true, func(ctx context.Context, root string) (string, error) {
		if f.Kind == git.Conflicted && hasConflictMarkers(filepath.Join(root, f.Path)) {
			return "", fmt.Errorf("%s still has conflict markers (<<<<<<<). Fix them first", git.Clean(f.Path))
		}
		return "", git.Stage(ctx, root, f.Path)
	})
}

// hasConflictMarkers looks for a leftover marker at the start of a line. A file it can't read or
// that's huge isn't held back.
func hasConflictMarkers(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.Size() > 4<<20 {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "<<<<<<< ") || strings.HasPrefix(line, ">>>>>>> ") || line == "=======" {
			return true
		}
	}
	return false
}

func (a *app) unstageSelected() {
	r, ok := a.git.selected()
	if !ok || r.kind != rowFile || r.file.Kind != git.Staged {
		a.message = "Select a staged file first."
		return
	}
	f, hasCommits := r.file, a.git.st.HasCommits
	a.gitOp("unstage", true, func(ctx context.Context, root string) (string, error) {
		paths := []string{f.Path}
		if f.Orig != "" {
			paths = append(paths, f.Orig) // A rename is two paths.
		}
		return "", git.Unstage(ctx, root, hasCommits, paths...)
	})
}

// Branches.

func (a *app) gitBranches(merge bool) {
	root := a.gitRoot()
	if root == "" {
		a.message = "This isn't a Git repository yet. Alt+G shows how to make one."
		return
	}
	go func() {
		branches, err := git.Branches(context.Background(), root)
		a.post(func() {
			if err != nil {
				a.message = "Can't list branches: " + shown(err.Error())
				return
			}
			s := &switcher{title: "Switch branch"}
			if merge {
				s.title = "Merge into the current branch"
			}
			var names []string
			for _, b := range branches {
				if merge && b.Current {
					continue
				}
				label := shown(b.Name)
				if b.Current {
					label += "  (current)"
				}
				s.labels = append(s.labels, label)
				names = append(names, b.Name)
			}
			if len(names) == 0 {
				a.message = "There's no other branch."
				return
			}
			s.pick = func(i int) {
				name := names[i]
				if merge {
					cur := ""
					if a.git.st != nil {
						cur = a.git.st.Branch
					}
					a.ask(fmt.Sprintf("Merge %s into %s?", shown(name), shown(cur)),
						choice{'m', "Merge", func() {
							a.gitOp("merge", true, func(ctx context.Context, root string) (string, error) { return git.Merge(ctx, root, name) })
						}},
						choice{'c', "Cancel", func() {}})
					return
				}
				a.gitOp("switch to "+shown(name), true, func(ctx context.Context, root string) (string, error) { return "", git.Switch(ctx, root, name) })
			}
			a.switcher = s
		})
	}()
}

func (a *app) gitNewBranch() {
	if a.gitRoot() == "" {
		a.message = "This isn't a Git repository yet. Alt+G shows how to make one."
		return
	}
	a.ask1Line("New branch name:", "", func(name string) {
		a.gitOp("create branch", true, func(ctx context.Context, root string) (string, error) { return "", git.CreateBranch(ctx, root, name) })
	})
}

// abortMerge asks first, naming what goes back to how it was.
func (a *app) abortMerge() {
	st := a.git.st
	if a.gitRoot() == "" || !st.Merging {
		a.message = "There's no merge in progress."
		return
	}
	var names []string
	for _, f := range st.Files {
		if f.Kind != git.Untracked && !slices.Contains(names, shown(f.Path)) {
			names = append(names, shown(f.Path))
		}
	}
	list := strings.Join(names[:min(len(names), 8)], ", ")
	if len(names) > 8 {
		list += fmt.Sprintf(" and %d more", len(names)-8)
	}
	a.ask(fmt.Sprintf("Abort the merge? These %d files go back to how they were before it, and what you did to them in it is lost: %s", len(names), list),
		choice{'a', "Abort merge", func() {
			a.gitOp("abort merge", true, func(ctx context.Context, root string) (string, error) { return "", git.MergeAbort(ctx, root) })
		}},
		choice{'c', "Keep merging", func() {}})
}

func (a *app) gitInit() {
	dir := a.dirPath()
	a.ask(fmt.Sprintf("Run git init in %s? It makes a .git folder there.", filepath.Base(dir)),
		choice{'y', "Create repository", func() {
			a.background("init", false, func(ctx context.Context) (string, error) { return "", git.Init(ctx, dir) }, func(_ string, err error) {
				a.refreshGit(true, func() {
					a.message = a.gitOutcome("git init", err)
					a.git.last = a.message
				})
			})
		}},
		choice{'c', "Cancel", func() {}})
}

// Opening things.

func (a *app) openGitRow() {
	r, ok := a.git.selected()
	if !ok {
		return
	}
	root := a.gitRoot()
	switch r.kind {
	case rowCommit:
		id, subject := r.entry.ID, r.entry.Subject
		go func() {
			text, err := git.Show(context.Background(), root, id)
			a.post(func() {
				if err != nil {
					a.message = "Can't show that commit: " + shown(err.Error())
					return
				}
				a.openText(root, id[:min(7, len(id))]+" "+subject+" (commit)", text)
			})
		}()
	case rowFile:
		f := r.file
		if f.Kind == git.Conflicted {
			a.openFile(a.dir(), filepath.Join(root, f.Path))
			return
		}
		go func() {
			text, cut, err := git.Diff(context.Background(), root, f)
			a.post(func() {
				if err != nil {
					a.message = "Can't show the changes: " + shown(err.Error())
					return
				}
				if cut {
					text += "\n[The diff is longer than LazyCode shows.]\n"
				}
				kind := "diff"
				if f.Kind == git.Staged {
					kind = "staged"
				}
				a.openText(root, filepath.Base(f.Path)+" ("+kind+")", text)
			})
		}()
	}
}

// openSelectedFile opens the selected file itself in the editor.
func (a *app) openSelectedFile() {
	if r, ok := a.git.selected(); ok && r.kind == rowFile {
		a.openFile(a.dir(), filepath.Join(a.gitRoot(), r.file.Path))
	}
}

// openText shows text in a read-only editor tab. It has no file, so it can't be saved over anything, and
// a newer version of the same view replaces the old one.
func (a *app) openText(root, name, text string) {
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r < ' ' {
			return '∕'
		}
		return r
	}, name)
	path := filepath.Join(root, ".git", "lazycode-views", name)
	st := a.dir()
	for _, p := range st.panes() {
		for _, it := range slices.Clone(p.tabs) {
			if it.ed != nil && it.ed.File.Path == path {
				a.removeTab(st, p, it)
			}
		}
	}
	a.addEditor(st, buffer.NewReadOnly(path, []byte(text)))
}

// GitHub.

func (a *app) repoCommand(args []string) {
	if len(args) == 1 && args[0] == "create" {
		a.repoCreate()
		return
	}
	a.message = "Try :repo create"
}

// repoCreate creates the GitHub repository with gh: it checks gh first, asks for a name and visibility,
// shows the exact command, and runs it only when told to.
func (a *app) repoCreate() {
	dir := a.dirPath()
	a.background("checking gh", false, func(ctx context.Context) (string, error) {
		if err := github.Check(ctx); err != nil {
			return "", err
		}
		if _, err := git.Root(ctx, dir); err != nil {
			return "norepo", nil
		}
		return "", nil
	}, func(out string, err error) {
		switch {
		case err != nil:
			a.message = shown(err.Error())
		case out == "norepo":
			a.ask(fmt.Sprintf("%s isn't a Git repository. Run git init first?", filepath.Base(dir)),
				choice{'y', "git init", func() {
					a.background("init", false, func(ctx context.Context) (string, error) { return "", git.Init(ctx, dir) }, func(_ string, err error) {
						if err != nil {
							a.message = "git init failed: " + shown(err.Error())
							return
						}
						a.askRepoName()
					})
				}},
				choice{'c', "Cancel", func() {}})
		default:
			a.askRepoName()
		}
	})
}

func (a *app) askRepoName() {
	dir := a.dirPath()
	a.ask1Line("Repository name:", filepath.Base(dir), func(name string) {
		if err := github.ValidName(name); err != nil {
			msg := err.Error()
			a.message = strings.ToUpper(msg[:1]) + msg[1:] + "."

			return
		}
		a.ask(fmt.Sprintf("Who can see %s on GitHub?", name),
			choice{'p', "Private (default)", func() { a.confirmRepo(name, true) }},
			choice{'u', "Public", func() { a.confirmRepo(name, false) }},
			choice{'c', "Cancel", func() {}})
	})
}

func (a *app) confirmRepo(name string, private bool) {
	dir := a.dirPath()
	command := "gh " + strings.Join(github.CreateArgs(name, private), " ")
	a.ask(fmt.Sprintf("This will run, in %s: %s", filepath.Base(dir), command),
		choice{'r', "Run it", func() {
			a.background("creating repository", false, func(ctx context.Context) (string, error) {
				return github.CreateRepo(ctx, dir, name, private)
			}, func(out string, err error) {
				if err != nil {
					a.message = "gh failed: " + shown(err.Error())
				} else {
					a.message = "Created " + shown(out) + ". :push sends your commits."
				}
				a.git.last = a.message
				a.refreshGit(true, nil)
			})
		}},
		choice{'c', "Cancel", func() {}})
}

// Keys and drawing.

// gitKey gives the Git view its keys, and leaves the rest to the keymap.
func (a *app) gitKey(ev *tcell.EventKey) bool {
	g := &a.git
	switch ev.Key() {
	case tcell.KeyUp:
		g.move(-1)
	case tcell.KeyDown:
		g.move(1)
		a.moreHistory()
	case tcell.KeyPgUp:
		g.move(-8)
	case tcell.KeyPgDn:
		g.move(8)
		a.moreHistory()
	case tcell.KeyEnter:
		a.openGitRow()
	case tcell.KeyRune:
		if ev.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) != 0 {
			return false
		}
		switch ev.Rune() {
		case 'p':
			a.gitPush()
		case 'l':
			a.gitPull()
		case 'f':
			a.gitFetch()
		case 'c':
			a.gitCommit()
		case 's':
			a.stageSelected()
		case 'u':
			a.unstageSelected()
		case 'b':
			a.gitBranches(false)
		case 'n':
			a.gitNewBranch()
		case 'm':
			a.gitBranches(true)
		case 'x':
			a.abortMerge()
		case 'o':
			a.openSelectedFile()
		case 'i':
			if g.err != nil && errors.Is(g.err, git.ErrNotRepo) {
				a.gitInit()
			} else {
				a.message = "There's a repository here already."
			}
		case 'r':
			a.refreshGit(true, nil)
		default:
			return false
		}
	default:
		return false
	}
	return true
}

// graphText draws the graph characters, and the matching ones for the line under a commit.
var graphLook = strings.NewReplacer("*", "●", "|", "│")

func graphBelow(graph string) string {
	return graphLook.Replace(strings.Map(func(r rune) rune {
		switch r {
		case '*':
			return '|'
		case '|':
			return '|'
		}
		return ' '
	}, graph))
}

func (a *app) drawGit(r rect) {
	g := &a.git
	maxX := r.x + r.w
	if g.dir != a.dirPath() {
		a.put(r.x+1, r.y+1, maxX, "Loading...", a.th.muted)
		return
	}
	switch {
	case !g.loaded:
		a.put(r.x+1, r.y+1, maxX, "Loading...", a.th.muted)
		return
	case errors.Is(g.err, git.ErrNotRepo):
		a.put(r.x+1, r.y+1, maxX, "Not a Git repository", a.th.primary)
		button := "[Create one: git init]"
		a.put(r.x+1, r.y+3, maxX, button, a.th.accent)
		a.hits = append(a.hits, hit{area: rect{r.x + 1, r.y + 3, len(button), 1}, click: func(int, int, tcell.ModMask) { a.gitInit() }})
		a.put(r.x+1, r.y+5, maxX, "Or press i.", a.th.muted)
		a.put(r.x+1, r.y+6, maxX, "On GitHub: :repo create", a.th.muted)
		return
	case g.err != nil:
		lines := wrapText(shown(g.err.Error()), max(1, r.w-2))
		for i, line := range lines[:min(len(lines), max(0, r.h-2))] {
			a.put(r.x+1, r.y+1+i, maxX, line, a.th.primary)
		}
		return
	}
	st := g.st
	if st == nil {
		a.put(r.x+1, r.y+1, maxX, "Loading...", a.th.muted)
		return
	}
	branch := st.Branch
	switch {
	case st.Detached:
		branch = "(detached) " + st.Head[:min(7, len(st.Head))]
	case !st.HasCommits:
		branch += " (no commits yet)"
	}
	line := shown(branch)
	if st.Upstream != "" {
		line += fmt.Sprintf("  ↑%d ↓%d", st.Ahead, st.Behind)
	} else if st.HasCommits && !st.Detached {
		line += "  no upstream"
	}
	if st.Merging {
		line += "  MERGING"
	}
	a.put(r.x+1, r.y+1, maxX, line, a.th.accent)

	x := r.x + 1
	for _, b := range []struct {
		label string
		do    func()
	}{{"[Push]", a.gitPush}, {"[Pull]", a.gitPull}, {"[Fetch]", a.gitFetch}} {
		start, do := x, b.do
		x = a.put(x, r.y+2, maxX, b.label, a.th.primary) + 1
		a.hits = append(a.hits, hit{area: rect{start, r.y + 2, x - start - 1, 1}, click: func(int, int, tcell.ModMask) { do() }})
	}
	x = r.x + 1
	for _, b := range []struct {
		label string
		do    func()
	}{{"[Commit]", a.gitCommit}, {"[Branches]", func() { a.gitBranches(false) }}} {
		start, do := x, b.do
		x = a.put(x, r.y+3, maxX, b.label, a.th.primary) + 1
		a.hits = append(a.hits, hit{area: rect{start, r.y + 3, x - start - 1, 1}, click: func(int, int, tcell.ModMask) { do() }})
	}

	y0, rows := r.y+4, r.h-5
	footer := g.last
	if g.busy != "" {
		footer = g.busy
	} else if footer == "" {
		footer = "p l f push pull fetch, c commit, s u stage"
	}
	a.put(r.x+1, r.y+r.h-1, maxX, footer, a.th.muted)
	if rows <= 0 || len(g.rows) == 0 {
		return
	}
	// Keep the selection on screen. Commits take two rows.
	g.top = max(0, min(g.top, g.sel))
	for g.top < g.sel && heightBetween(g.rows, g.top, g.sel+1) > rows {
		g.top++
	}
	focused := !a.tabFocused && a.focusedView() == viewGit
	y := y0
	type seen struct{ y, h, i int }
	var drawn []seen
	for i := g.top; i < len(g.rows) && y < y0+rows; i++ {
		row := g.rows[i]
		style := a.th.primary
		if i == g.sel && focused && row.selectable() {
			style = a.th.active
		}
		switch row.kind {
		case rowHeader:
			a.put(r.x+1, y, maxX, row.text, a.th.accent)
		case rowNote:
			a.put(r.x+1, y, maxX, row.text, a.th.muted)
		case rowGraph:
			a.put(r.x+1, y, maxX, graphLook.Replace(row.entry.Graph), a.th.muted)
		case rowFile:
			f := row.file
			text := fmt.Sprintf("%c %s", f.Marker(), shown(f.Path))
			if f.Orig != "" {
				text = fmt.Sprintf("%c %s ← %s", f.Marker(), shown(f.Path), shown(f.Orig))
			}
			a.put(r.x+1, y, maxX, text, style)
		case rowCommit:
			e := row.entry
			graph := graphLook.Replace(e.Graph)
			a.put(r.x+1, y, maxX, graph+" "+shown(e.Subject), style)
			if y+1 < y0+rows {
				// Date and refs before the author, so a narrow panel cuts the least useful part.
				detail := graphBelow(e.Graph) + " " + shown(e.When)
				if e.Refs != "" {
					detail += " (" + shown(e.Refs) + ")"
				}
				detail += " " + shown(e.Author)
				a.put(r.x+1, y+1, maxX, detail, a.th.muted)
			}
		}
		drawn = append(drawn, seen{y, row.height(), i})
		y += row.height()
	}
	a.hits = append(a.hits, hit{
		area: rect{r.x, y0, r.w, rows},
		click: func(_, cy int, _ tcell.ModMask) {
			for _, d := range drawn {
				if cy >= d.y && cy < d.y+d.h && g.rows[d.i].selectable() {
					g.sel = d.i
					a.tabFocused = false
					a.openGitRow()
				}
			}
		},
		wheel: func(delta int) { g.move(delta * 2); a.moreHistory() },
	})
}

func heightBetween(rows []gitRow, from, to int) int {
	h := 0
	for _, r := range rows[from:to] {
		h += r.height()
	}
	return h
}
