package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Juan17la/lazycode/internal/buffer"
	"github.com/Juan17la/lazycode/internal/editor"
	"github.com/Juan17la/lazycode/internal/project"
	"github.com/Juan17la/lazycode/internal/tree"
)

// The tree on screen.

// tree returns the tree being shown: the active directory's, or the combined one (Q-41).
// ponytail: the combined tree is rebuilt on changing project, so its open folders are
// forgotten. Keep one per project if that gets annoying.
func (a *app) tree() *tree.Tree {
	if !a.showAll {
		return a.dir().tree
	}
	dirs := a.projects[a.project].Dirs
	if a.combined == nil || !slices.Equal(a.combined.Roots, dirs) {
		a.combined = tree.NewCombined(dirs)
	}
	return a.combined
}

func (a *app) toggleCombinedTree() {
	a.showAll = !a.showAll
	a.tree().Refresh()
}

// syncWatcher keeps the watcher on the folders being shown. It runs after every event,
// which also covers every way of opening a folder or changing directory.
func (a *app) syncWatcher() {
	if a.watcher == nil {
		return
	}
	folders := a.tree().Folders()
	if slices.Equal(folders, a.watching) {
		return
	}
	a.watching = slices.Clone(folders)
	if err := a.watcher.Set(a.watching); err != nil {
		a.message = "Some folders won't update by themselves, and r refreshes them: " + err.Error()
	}
}

// Project and directory commands.

func (a *app) projectCommand(args []string) {
	switch {
	case len(args) == 0:
		a.openProjectSwitcher()
	case args[0] == "new" && len(args) == 1:
		a.openNewProject()
	case args[0] == "new":
		a.newProject(strings.Join(args[1:], " "))
	case args[0] == "close" && len(args) == 1:
		a.closeProject()
	default:
		a.message = "Try :project, :project new NAME, or :project close."
	}
}

func (a *app) dirCommand(args []string) {
	switch {
	case len(args) == 0:
		a.openDirSwitcher()
	case args[0] == "add" && len(args) == 1:
		a.openAddDir()
	case args[0] == "add":
		a.addDir(strings.Join(args[1:], " "))
	case args[0] == "remove" && len(args) == 1:
		a.removeDir(a.projects[a.project].Name, a.dirPath())
	default:
		a.message = "Try :dir, :dir add PATH, or :dir remove."
	}
}

// closeProject closes this project's tabs and moves to the next project (FR-6.5). Anything running is
// named first, and each file with unsaved work is asked about, as when quitting. Nothing saved is
// touched: projects.toml, the folders, and AI sessions, which sleep and can be resumed.
// ponytail: a directory that's in several projects (Q-66) loses its tabs in all of them, because
// tabs belong to directories. Per-project tabs would need a project key in dirState.
func (a *app) closeProject() {
	closing := a.projects[a.project]
	inScope := func(dir string) bool { return slices.Contains(closing.Dirs, dir) }
	var running []string
	for dir, st := range a.dirs {
		if !inScope(dir) {
			continue
		}
		for _, p := range st.panes() {
			for _, it := range p.tabs {
				if it.term != nil {
					if done, _ := it.term.Exited(); !done {
						running = append(running, it.name())
					}
				}
			}
		}
	}
	slices.Sort(running)
	proceed := func() {
		a.askUnsaved(inScope, "before closing "+closing.Name, map[*buffer.File]bool{}, func() { a.finishCloseProject(closing) })
	}
	if len(running) == 0 {
		proceed()
		return
	}
	a.ask(fmt.Sprintf("Closing %s stops: %s.", closing.Name, strings.Join(running, ", ")),
		choice{'c', "Close", proceed},
		choice{'k', "Keep open", func() {}})
}

// finishCloseProject closes every tab in the project and moves away from it, if there's anywhere to go.
func (a *app) finishCloseProject(closed project.Project) {
	at := slices.IndexFunc(a.projects, func(p project.Project) bool { return p.Name == closed.Name })
	if at < 0 {
		return // projects.toml changed while a question was open.
	}
	for _, dir := range closed.Dirs {
		if st := a.dirs[dir]; st != nil {
			for _, p := range st.panes() {
				for _, it := range slices.Clone(p.tabs) {
					a.removeTab(st, p, it)
				}
			}
			delete(a.dirs, dir) // Rebuilt, empty, if the project is opened again.
		}
	}
	a.tabFocused = false
	a.stopServers(closed.Dirs)
	if len(a.projects) == 1 {
		a.afterSwitch()
		a.message = "Closed the tabs of " + closed.Name + ". It's the only project, so it stays open."
		return
	}
	a.project = (at + 1) % len(a.projects)
	a.afterSwitch()
	a.message = "Closed " + closed.Name + ". It's still listed, and :project opens it again."
}

// editProjects applies a change to what's in projects.toml now, so that a hand edit made
// since it was read isn't overwritten. A broken file is left alone. change returns a
// problem to show, if it refuses.
func (a *app) editProjects(change func([]project.Project) ([]project.Project, string)) bool {
	path := a.settingsPath("projects.toml")
	loaded, err := project.Load(path)
	if err != nil {
		a.message = "Nothing changed, because projects.toml has a problem: " + err.Error()
		return false
	}
	changed, problem := change(loaded)
	if problem != "" {
		a.message = problem
		return false
	}
	if err := project.Save(path, changed); err != nil {
		a.message = "Can't save projects.toml: " + err.Error()
		return false
	}
	a.setProjects(changed)
	return true
}

// setProjects replaces the list, staying on the same directory and remembering where
// each project was.
func (a *app) setProjects(loaded []project.Project) {
	name, dir, active := a.projects[a.project].Name, a.dirPath(), a.activeDirs()
	all, p, d := project.Locate(loaded, dir)
	for i, candidate := range all { // A directory can be in several projects (Q-66).
		if at := slices.Index(candidate.Dirs, dir); candidate.Name == name && at >= 0 {
			p, d = i, at
		}
	}
	a.projects, a.project, a.dirOf = all, p, make([]int, len(all))
	a.setActiveDirs(active)
	a.dirOf[p] = d
}

// reloadProjects applies projects.toml after it's saved. A broken file changes nothing.
func (a *app) reloadProjects() {
	loaded, err := project.Load(a.settingsPath("projects.toml"))
	if err != nil {
		a.message = "Projects not loaded: " + err.Error()
		return
	}
	a.setProjects(loaded) // Stays on this directory, so focus can stay in the file being edited.
}

func (a *app) activeDirs() map[string]string {
	active := map[string]string{}
	for i, p := range a.projects {
		active[p.Name] = p.Dirs[a.dirOf[i]]
	}
	return active
}

func (a *app) setActiveDirs(active map[string]string) {
	for i, p := range a.projects {
		if at := slices.Index(p.Dirs, active[p.Name]); at >= 0 {
			a.dirOf[i] = at
		}
	}
}

// goTo switches to a directory of a named project.
func (a *app) goTo(name, dir string) {
	p := slices.IndexFunc(a.projects, func(p project.Project) bool { return p.Name == name })
	if p < 0 {
		return
	}
	if at := slices.Index(a.projects[p].Dirs, dir); at >= 0 {
		a.project, a.dirOf[p] = p, at
		a.afterSwitch()
	}
}

// newProject makes a project holding the current directory, and moves to it.
func (a *app) newProject(name string) { a.newProjectAt(name, a.dirPath()) }

// newProjectAt makes a project holding one directory, and moves to it.
func (a *app) newProjectAt(name, dir string) {
	if a.editProjects(func(all []project.Project) ([]project.Project, string) {
		if slices.ContainsFunc(all, func(p project.Project) bool { return p.Name == name }) {
			return nil, "There's already a project called " + name + "."
		}
		return append(all, project.Project{Name: name, Dirs: []string{dir}}), ""
	}) {
		a.goTo(name, dir)
		a.message = "Created project " + name + ". :dir add PATH adds another directory."
	}
}

// addDir adds a directory to this project, and moves to it. It needn't be a Git repository,
// and can be in other projects too (Q-66). A relative path starts from the current directory.
func (a *app) addDir(path string) {
	current := a.projects[a.project]
	if current.Unsaved {
		a.message = "This folder isn't in a project yet. :project new NAME makes one."
		return
	}
	path = project.Expand(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(a.dirPath(), path)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		a.message = filepath.Base(path) + " isn't a directory." // Short, so that it fits the line.
		return
	}
	if a.editProjects(func(all []project.Project) ([]project.Project, string) {
		i := slices.IndexFunc(all, func(p project.Project) bool { return p.Name == current.Name })
		if i < 0 {
			return nil, current.Name + " is no longer in projects.toml."
		}
		if !slices.Contains(all[i].Dirs, path) {
			all[i].Dirs = append(all[i].Dirs, path)
		}
		return all, ""
	}) {
		a.goTo(current.Name, path)
		a.message = "Added " + filepath.Base(path) + " to " + current.Name + "."
	}
}

// removeDir takes a directory out of a project, which never touches the folder. Open tabs
// would become unreachable, so they have to be closed first.
func (a *app) removeDir(name, dir string) {
	if st := a.dirs[dir]; st != nil {
		for _, p := range st.panes() {
			if len(p.tabs) > 0 {
				a.message = "Close the tabs in " + filepath.Base(dir) + " first."
				return
			}
		}
	}
	if a.projects[a.project].Unsaved {
		a.message = "This folder isn't in a project."
		return
	}
	wasCurrent := a.projects[a.project].Name == name && a.dirPath() == dir
	if a.editProjects(func(all []project.Project) ([]project.Project, string) {
		i := slices.IndexFunc(all, func(p project.Project) bool { return p.Name == name })
		if i < 0 || !slices.Contains(all[i].Dirs, dir) {
			return nil, filepath.Base(dir) + " isn't in " + name + "."
		}
		if len(all[i].Dirs) == 1 {
			return nil, filepath.Base(dir) + " is the only directory in " + name + ", so it stays."
		}
		all[i].Dirs = slices.DeleteFunc(all[i].Dirs, func(d string) bool { return d == dir })
		if wasCurrent { // Move away first, or setProjects would follow it out of the project.
			next := slices.IndexFunc(a.projects[a.project].Dirs, func(d string) bool { return d != dir })
			a.dirOf[a.project] = max(0, next)
		}
		return all, ""
	}) {
		a.afterSwitch()
		a.message = "Removed " + filepath.Base(dir) + " from " + name + ". The folder is untouched."
	}
}

// askAboutMissingDir marks a missing directory and lets the user decide. An unplugged drive
// looks the same as a deleted folder, so it's never removed without asking (Q-66).
func (a *app) askAboutMissingDir() {
	current, dir := a.projects[a.project], a.dirPath()
	if _, err := os.Stat(dir); err == nil || current.Unsaved {
		return
	}
	a.ask(filepath.Base(dir)+" is missing.",
		choice{'r', "Remove from project", func() { a.removeDir(current.Name, dir) }},
		choice{'k', "Keep", func() {}})
}

// Saving and restoring (Q-39, Q-73).

func (a *app) loadState() {
	path, err := project.StatePath()
	if err != nil {
		return // No cache folder: nothing is remembered, and nothing else changes.
	}
	a.statePath = path
	a.state, err = project.LoadState(path)
	if err != nil {
		a.message = strings.TrimSpace("Saved tabs couldn't be read, so they weren't restored. " + a.message)
		return
	}
	a.showSecond, a.showAll = a.state.ShowSecond, a.state.ShowAll
	a.panelW, a.dockH, a.dockHidden = a.state.PanelWidth, a.state.DockHeight, a.state.DockHidden
	a.setFilesPanel(max(0, min(a.state.FilesPanel, 1)))
	if a.panelOf[viewFiles] == 1 {
		a.showSecond = true
	}
	// The folder it was started in comes first, and the other projects return to where they were.
	started := a.dirOf[a.project]
	a.setActiveDirs(a.state.Active)
	a.dirOf[a.project] = started
}

// saveState is best effort: losing it only loses what was open, so no failure is reported.
func (a *app) saveState() {
	if a.statePath == "" {
		return
	}
	if a.state.Active == nil {
		a.state.Active = map[string]string{}
	}
	if a.state.Directories == nil {
		a.state.Directories = map[string]project.DirState{}
	}
	for name, dir := range a.activeDirs() {
		a.state.Active[name] = dir
	}
	for path, st := range a.dirs {
		saved := project.DirState{Layout: saveSplit(st, st.root), OpenFolders: st.tree.OpenFolders()}
		if len(saved.OpenFolders) == 0 && layoutEmpty(saved.Layout) {
			delete(a.state.Directories, path) // So that the file doesn't grow forever.
			continue
		}
		a.state.Directories[path] = saved
	}
	a.state.ShowSecond, a.state.FilesPanel, a.state.ShowAll = a.showSecond, a.panelOf[viewFiles], a.showAll
	a.state.PanelWidth, a.state.DockHeight, a.state.DockHidden = a.panelW, a.dockH, a.dockHidden
	project.SaveState(a.statePath, a.state) //nolint:errcheck // See above.
}

// virtualTab is a read-only view of something that isn't a file, such as a diff. Its path names
// nothing on disk, so it couldn't be reopened. A big read-only file does exist, and is kept.
func virtualTab(it *tabItem) bool {
	if !it.ed.File.ReadOnly() {
		return false
	}
	_, err := os.Stat(it.ed.File.Path)
	return err != nil
}

func layoutEmpty(s *project.Split) bool {
	return s.First == nil && len(s.Tabs) == 0
}

// saveSplit records files and leaves out terminals, whose contents aren't saved. Sessions
// come back through D3 and D4 (doc 08).
func saveSplit(st *dirState, s *split) *project.Split {
	if s.pane == nil {
		return &project.Split{SideBySide: s.sideBySide, Ratio: s.ratio, First: saveSplit(st, s.first), Second: saveSplit(st, s.second)}
	}
	out := &project.Split{Focus: s.pane == st.focus}
	for i, it := range s.pane.tabs {
		if it.ed == nil || virtualTab(it) {
			continue
		}
		if i == s.pane.cur {
			out.Cur = len(out.Tabs)
		}
		line, col := it.ed.Position()
		out.Tabs = append(out.Tabs, project.Tab{Path: it.ed.File.Path, Line: line, Col: col, Scroll: it.ed.ScrollLine})
	}
	return out
}

// newDirState makes a directory's state, with whatever was saved for it. It runs the first
// time a directory is shown, so a project's other directories cost nothing until then.
func (a *app) newDirState(path string) *dirState {
	st := &dirState{tree: tree.New(path), dock: &pane{}}
	saved := a.state.Directories[path]
	if len(saved.OpenFolders) > 0 {
		st.tree.Reopen(saved.OpenFolders)
	}
	r := restorer{a: a, st: st, files: map[string]*buffer.File{}}
	if saved.Layout != nil {
		st.root = r.split(saved.Layout)
	}
	if st.root == nil {
		st.root = &split{pane: &pane{}}
	}
	if st.focus == nil {
		st.focus = st.panes()[0]
	}
	if r.failed > 0 {
		a.message = fmt.Sprintf("%d saved tabs in %s couldn't be reopened.", r.failed, filepath.Base(path))
	}
	return st
}

type restorer struct {
	a      *app
	st     *dirState
	files  map[string]*buffer.File // So that two views of a file share it.
	failed int
}

// split rebuilds a saved node. A pane whose files have all gone is dropped, with its sibling
// taking the space, and nil means nothing is left. Numbers come from a file, so they're
// kept in range here and by the editor.
func (r *restorer) split(s *project.Split) *split {
	if s.First != nil || s.Second != nil {
		var first, second *split
		if s.First != nil {
			first = r.split(s.First)
		}
		if s.Second != nil {
			second = r.split(s.Second)
		}
		switch {
		case first == nil:
			return second
		case second == nil:
			return first
		}
		return &split{sideBySide: s.SideBySide, ratio: s.Ratio, first: first, second: second}
	}
	p := &pane{}
	for i, tab := range s.Tabs {
		f := r.files[tab.Path]
		if f == nil {
			opened, err := buffer.Open(tab.Path)
			if err != nil {
				r.failed++
				continue
			}
			f, r.files[tab.Path] = opened, opened
		}
		e := editor.New(f, r.a.cfg.Editor.TabSize)
		e.GoTo(tab.Line, tab.Col, false)
		e.ScrollLine = max(0, tab.Scroll)
		if i == s.Cur {
			p.cur = len(p.tabs)
		}
		p.tabs = append(p.tabs, &tabItem{ed: e})
	}
	if len(p.tabs) == 0 {
		return nil
	}
	if s.Focus {
		r.st.focus = p
	}
	return &split{pane: p}
}

// dropRestoredTabs closes the views that were restored for a path, so that recovered text
// can't end up beside a second copy of the file from disk. Nothing can have been typed in
// them, because the recovery question comes before any other key.
func (a *app) dropRestoredTabs(st *dirState, path string) {
	for _, p := range st.panes() {
		for _, it := range slices.Clone(p.tabs) {
			if it.ed != nil && it.ed.File.Path == path {
				a.removeTab(st, p, it)
			}
		}
	}
}
