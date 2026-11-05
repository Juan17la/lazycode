package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Juan17la/lazycode/internal/project"
	"github.com/gdamore/tcell/v2"
)

// launchWith is launch with chosen projects, and with no name in them to fall back on.
func launchWith(t *testing.T, dir string, projects []project.Project) *ui {
	t.Helper()
	u := &ui{t: t, screen: &recordingScreen{SimulationScreen: tcell.NewSimulationScreen("UTF-8")}, dir: dir, exited: make(chan error, 1)}
	go func() { u.exited <- Run(u.screen, projects, dir, "", "dev") }()
	u.wait("Files")
	t.Cleanup(func() {
		select {
		case <-u.exited:
		default:
			u.key(tcell.KeyEsc)
			u.key(tcell.KeyCtrlBackslash)
			u.run("q")
			select {
			case <-u.exited:
			case <-time.After(5 * time.Second):
			}
		}
	})
	return u
}

// sibling makes a directory beside the test's work directory.
func sibling(t *testing.T, u *ui, name string, files ...string) string {
	t.Helper()
	dir := filepath.Join(filepath.Dir(u.dir), name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func projectsFile(t *testing.T) string {
	t.Helper()
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "lazycode", "projects.toml")
}

func loadedProjects(t *testing.T) []project.Project {
	t.Helper()
	all, err := project.Load(projectsFile(t))
	if err != nil {
		t.Fatal(err)
	}
	return all
}

func (u *ui) quitNow() {
	u.t.Helper()
	u.key(tcell.KeyCtrlBackslash)
	u.run("q")
	u.quit()
}

func TestTabsCursorsSplitsAndFoldersComeBackAfterQuitting(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\ntwo\nthree\n", "b.txt": "bee\n"})
	if err := os.Mkdir(filepath.Join(u.dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(u.dir, "sub", "deep.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// Open the folder first, as it sorts first.
	u.key(tcell.KeyCtrlBackslash)
	u.press("r")
	u.wait("sub")
	u.key(tcell.KeyUp) // The selection stayed on a.txt, which moved down.
	u.key(tcell.KeyRight)
	u.wait("deep.txt")
	u.key(tcell.KeyDown)
	u.key(tcell.KeyDown)
	u.key(tcell.KeyEnter) // a.txt
	u.wait(" a.txt ")
	u.key(tcell.KeyDown)
	u.key(tcell.KeyDown)
	u.wait("Ln 3, Col 1")
	u.post(tcell.NewEventKey(tcell.KeyRune, 's', tcell.ModAlt)) // A second view, side by side.
	u.waitCount(" a.txt ", 3)                                   // The tree, and two tabs.
	u.quitNow()

	again := launch(t, u.dir)
	again.waitCount(" a.txt ", 3) // Without any key: the tree, both views, and the folder.
	again.wait("deep.txt")
	again.key(tcell.KeyTab)
	again.wait("Ln 3, Col 1")
	// The two views share one file, so an edit shows in both.
	again.press("X")
	again.waitCount("Xthree", 2)
}

func TestDeletedFileIsSkippedAndItsPaneGoes(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "a\n", "b.txt": "b\n"})
	u.open("a.txt")
	u.post(tcell.NewEventKey(tcell.KeyRune, 's', tcell.ModAlt))
	u.waitCount(" a.txt ", 3) // The tree, and two tabs.
	u.quitNow()
	if err := os.Remove(filepath.Join(u.dir, "a.txt")); err != nil {
		t.Fatal(err)
	}
	again := launch(t, u.dir)
	again.wait("couldn't be reopened")
	again.wait("No tabs here")
}

func TestBadStateFileDoesNotStopLazyCode(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "a\n"})
	u.quitNow()
	path, err := project.StatePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	again := launch(t, u.dir)
	again.wait("couldn't be read")
}

func TestOutOfRangeStateIsSafe(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "a\n"})
	u.quitNow()
	state := project.State{FilesPanel: 7, Directories: map[string]project.DirState{u.dir: {Layout: &project.Split{
		Cur:  99,
		Tabs: []project.Tab{{Path: filepath.Join(u.dir, "a.txt"), Line: -5, Col: 1 << 40, Scroll: 1 << 40}},
	}}}}
	path, err := project.StatePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := project.SaveState(path, state); err != nil {
		t.Fatal(err)
	}
	again := launch(t, u.dir)
	again.wait(" a.txt ")
	again.key(tcell.KeyTab)
	again.wait("Ln 1, Col ")
}

func TestStateFileHoldsNoTerminalsAndSkipsEmptyDirectories(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "a\n"})
	u.quitNow()
	path, err := project.StatePath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state project.State
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if _, found := state.Directories[u.dir]; found {
		t.Errorf("an untouched directory was saved: %s", data)
	}
	if state.Active["test"] != u.dir {
		t.Errorf("active directories: %v", state.Active)
	}
}

func TestRecoveredTextReplacesTheRestoredTab(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "disk\n"})
	u.open("a.txt")
	u.quitNow()
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	leaveRecovery(t, filepath.Join(dir, "lazycode", "recovery"), filepath.Join(u.dir, "a.txt"), "recovered\n")
	again := launch(t, u.dir)
	again.wait("Unsaved work")
	again.press("r")
	again.wait("recovered")
	again.waitCount(" a.txt* ", 1)
	again.waitCount(" a.txt ", 1) // Only the tree, with no second tab holding what's on disk.
	again.waitGone("disk")
}

func TestProjectCommandsSaveToTheFile(t *testing.T) {
	u := start(t, nil)
	other := sibling(t, u, "other", "o.txt")
	u.quitNow()

	// Started in a folder in no project.
	v := launchWith(t, u.dir, nil)
	v.wait("Current folder")
	v.run("dir add ../other")
	v.wait(":project new NAME")
	v.run("project new My Site")
	v.wait("My Site ›")
	v.run("project new My Site")
	v.wait("already a project")
	v.run("dir add ../nowhere")
	v.wait("isn't a directory")
	v.run("dir add ../other")
	v.wait("› other")
	v.wait("o.txt")
	want := []project.Project{{Name: "My Site", Dirs: []string{u.dir, other}}}
	if got := loadedProjects(t); !slices.EqualFunc(got, want, sameProject) {
		t.Fatalf("file holds %+v", got)
	}

	// Removing the current directory moves off it, and the last one can't go.
	v.run("dir remove")
	v.wait("› work")
	v.wait("The folder is untouched")
	v.run("dir remove")
	v.wait("only directory")
	if _, err := os.Stat(other); err != nil {
		t.Errorf("the folder was touched: %v", err)
	}
	if got := loadedProjects(t); len(got[0].Dirs) != 1 || got[0].Dirs[0] != u.dir {
		t.Fatalf("file holds %+v", got)
	}
}

func sameProject(a, b project.Project) bool {
	return a.Name == b.Name && slices.Equal(a.Dirs, b.Dirs)
}

func TestRemovingADirectoryWithOpenTabsIsRefused(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "a\n"})
	other := sibling(t, u, "other")
	all := []project.Project{{Name: "p", Dirs: []string{u.dir, other}}}
	if err := project.Save(projectsFile(t), all); err != nil {
		t.Fatal(err)
	}
	u.quitNow()
	v := launchWith(t, u.dir, all)
	v.open("a.txt")
	v.key(tcell.KeyCtrlBackslash)
	v.run("dir remove")
	v.wait("first.")
	if got := loadedProjects(t); len(got[0].Dirs) != 2 {
		t.Errorf("file holds %+v", got)
	}
}

func TestABrokenProjectsFileIsNeverRewritten(t *testing.T) {
	u := start(t, nil)
	path := projectsFile(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[[project"), 0o644); err != nil {
		t.Fatal(err)
	}
	u.run("project new x")
	u.wait("Nothing changed")
	if data, _ := os.ReadFile(path); string(data) != "[[project" {
		t.Errorf("file holds %q", data)
	}
}

func TestAHandEditIsKeptByTheNextChange(t *testing.T) {
	u := start(t, nil)
	other := sibling(t, u, "other")
	if err := project.Save(projectsFile(t), []project.Project{{Name: "test", Dirs: []string{u.dir}}, {Name: "by hand", Dirs: []string{other}}}); err != nil {
		t.Fatal(err)
	}
	u.run("project new added")
	u.wait("Created project")
	got := loadedProjects(t)
	if len(got) != 3 || got[1].Name != "by hand" {
		t.Errorf("file holds %+v", got)
	}
}

func TestEditingProjectsFileAppliesOnSave(t *testing.T) {
	u := start(t, nil)
	other := sibling(t, u, "other")
	if err := project.Save(projectsFile(t), []project.Project{{Name: "test", Dirs: []string{u.dir, other}}}); err != nil {
		t.Fatal(err)
	}
	u.run("config projects")
	u.wait(" projects.toml ")
	u.press("#")
	u.key(tcell.KeyEnter)
	u.key(tcell.KeyCtrlS)
	u.wait("Saved")
	u.press("x") // Focus stayed in the file.
	u.wait(" projects.toml* ")
	u.key(tcell.KeyCtrlBackslash)
	u.run("dir")
	u.wait("Switch directory")
	u.wait("other")
}

func TestMissingDirectoryIsMarkedAndTheUserDecides(t *testing.T) {
	u := start(t, nil)
	gone := sibling(t, u, "gone")
	third := sibling(t, u, "third")
	all := []project.Project{{Name: "p", Dirs: []string{u.dir, gone, third}}}
	if err := project.Save(projectsFile(t), all); err != nil {
		t.Fatal(err)
	}
	u.quitNow()
	v := launchWith(t, u.dir, all)
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}

	pick := func() {
		v.run("dir")
		v.wait("(missing)")
		v.press("gone")
		v.key(tcell.KeyEnter)
		v.wait("gone is missing")
	}
	// Keeping it changes nothing, including with Esc.
	pick()
	v.key(tcell.KeyEsc)
	v.waitGone("gone is missing")
	v.wait("Can't read this directory")
	if got := loadedProjects(t); len(got[0].Dirs) != 3 {
		t.Fatalf("file holds %+v", got)
	}
	// Going back to it asks again, and removing it takes it out of the file.
	v.run("dir")
	v.press("work")
	v.key(tcell.KeyEnter)
	v.wait("› work")
	pick()
	v.press("r")
	v.wait("Removed gone")
	if got := loadedProjects(t); slices.Contains(got[0].Dirs, gone) || len(got[0].Dirs) != 2 {
		t.Errorf("file holds %+v", got)
	}
}

func TestCombinedTreeOpensAFileInItsOwnDirectory(t *testing.T) {
	u := start(t, map[string]string{"here.txt": "here\n"})
	other := sibling(t, u, "other", "there.txt")
	all := []project.Project{{Name: "p", Dirs: []string{u.dir, other}}}
	u.quitNow()
	v := launchWith(t, u.dir, all)
	v.run("tree")
	v.wait("Files, all directories")
	v.wait("there.txt")
	v.wait("here.txt")
	// work, here.txt, other, there.txt.
	v.key(tcell.KeyDown)
	v.key(tcell.KeyDown)
	v.key(tcell.KeyDown)
	v.key(tcell.KeyEnter)
	v.wait(" there.txt ")
	v.wait("› other")
	v.key(tcell.KeyCtrlBackslash)
	v.wait("here.txt") // Still the combined view.
	v.quitNow()

	// The tab belongs to the other directory, so it isn't here. The view choice is remembered.
	w := launchWith(t, u.dir, all)
	w.wait("Files, all directories")
	if text := w.text(); strings.Contains(text, " there.txt  ") && strings.Contains(strings.SplitN(text, "\n", 3)[1], "there.txt") {
		t.Errorf("another directory's tab is showing:\n%s", text)
	}
	w.run("tree")
	w.waitGone("all directories")
	w.waitGone("there.txt")
}

func TestTreeNoticesNewFilesByItself(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "a\n"})
	if err := os.WriteFile(filepath.Join(u.dir, "appeared.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	u.wait("appeared.txt")
	if err := os.Remove(filepath.Join(u.dir, "appeared.txt")); err != nil {
		t.Fatal(err)
	}
	u.waitGone("appeared.txt")
}

func TestProjectsReturnToTheirOwnDirectory(t *testing.T) {
	u := start(t, nil)
	two := sibling(t, u, "two")
	three := sibling(t, u, "three")
	all := []project.Project{{Name: "first", Dirs: []string{u.dir, two}}, {Name: "second", Dirs: []string{three}}}
	u.quitNow()
	v := launchWith(t, u.dir, all)
	v.run("dir")
	v.press("two")
	v.key(tcell.KeyEnter)
	v.wait("› two")
	v.run("project")
	v.press("second")
	v.key(tcell.KeyEnter)
	v.wait("› three")
	v.quitNow()

	// Started in "three": "first" remembers it was on "two".
	w := launchWith(t, three, all)
	w.wait("› three")
	w.run("project")
	w.press("first")
	w.key(tcell.KeyEnter)
	w.wait("› two")
}
