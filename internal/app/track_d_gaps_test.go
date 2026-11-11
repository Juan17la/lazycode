package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Juan17la/lazycode/internal/buffer"
	"github.com/Juan17la/lazycode/internal/editor"
	"github.com/Juan17la/lazycode/internal/project"
	"github.com/gdamore/tcell/v2"
)

func TestDeletedSessionsStayDeleted(t *testing.T) {
	u := withTool(t, "stay", nil)
	u.run("session hidden")
	u.wait("No hidden sessions in this project")

	u.startFake()
	u.key(tcell.KeyCtrlBackslash)
	u.alt('a')
	u.wait("Fake 1")
	u.key(tcell.KeyDelete)
	u.wait("No sessions yet")

	u.run("session hidden") // Delete removes it for good, so there's nothing to bring back.
	u.wait("No hidden sessions in this project")
}

// twoProjects starts with "test" and a second project whose tabs aren't touched.
func twoProjects(t *testing.T) (*ui, string) {
	t.Helper()
	u := start(t, map[string]string{"a.txt": "alpha\n"})
	other := sibling(t, u, "other", "o.txt")
	u.quitNow()
	v := launchWith(t, u.dir, []project.Project{{Name: "test", Dirs: []string{u.dir}}, {Name: "second", Dirs: []string{other}}})
	return v, other
}

func TestProjectCloseAsksAboutUnsavedWorkAndMovesOn(t *testing.T) {
	u, _ := twoProjects(t)
	u.open("a.txt")
	u.press("X")
	u.key(tcell.KeyEsc) // Esc then : opens the command bar from a file.
	u.press(":project close")
	u.key(tcell.KeyEnter)
	u.wait("Save changes to a.txt before closing test?")
	u.press("c")
	u.wait(" a.txt* ") // Cancelling closes nothing.
	u.key(tcell.KeyEsc)
	u.press(":project close")
	u.key(tcell.KeyEnter)
	u.wait("before closing test?")
	u.press("s")
	u.waitFile("a.txt", "Xalpha\n")
	u.wait("Closed test. It's still listed")
	u.wait("second ›")
	u.waitGone("a.txt")
}

func TestProjectCloseDiscardsOnlyWhenTold(t *testing.T) {
	u, _ := twoProjects(t)
	u.open("a.txt")
	u.press("X")
	u.key(tcell.KeyEsc)
	u.press(":project close")
	u.key(tcell.KeyEnter)
	u.wait("before closing test?")
	u.press("d")
	u.wait("second ›")
	if got := u.file("a.txt"); got != "alpha\n" {
		t.Errorf("file holds %q", got)
	}
}

func TestProjectCloseNamesWhatStops(t *testing.T) {
	u, _ := twoProjects(t)
	tool, _ := os.Executable()
	t.Setenv("LAZYCODE_TEST_TOOL", "stay")
	u.run("term " + tool)
	u.wait("tool args:")
	u.key(tcell.KeyCtrlBackslash)
	u.run("project close")
	u.wait("Closing test stops: " + strings.TrimSuffix(filepath.Base(tool), filepath.Ext(tool)))
	u.press("k")
	u.wait("test ›")
	u.run("project close")
	u.wait("Closing test stops")
	u.press("c")
	u.wait("second ›")
	u.run("project")
	u.wait("Switch project")
	u.key(tcell.KeyUp)
	u.key(tcell.KeyEnter)
	u.wait("test ›")
	u.waitGone("tool args:") // Its tab is gone, not hidden.
}

func TestClosingTheOnlyProjectKeepsIt(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "alpha\n"})
	u.open("a.txt")
	u.key(tcell.KeyCtrlBackslash)
	u.run("project close")
	u.wait("It's the only project, so it stays open")
	u.wait("test ›")
	u.wait("No tabs here")
}

// A diff tab has a made-up path. Saving it would make the next start count it as a lost tab.
func TestVirtualReadOnlyTabsAreNotSaved(t *testing.T) {
	a := hlApp(t, map[string]string{"a.txt": "x\n"})
	st := a.dir()
	a.openFile(st, filepath.Join(a.dirPath(), "a.txt"))
	virtual := filepath.Join(a.dirPath(), ".git", "lazycode-views", "a (diff)")
	st.focus.tabs = append(st.focus.tabs, &tabItem{ed: editor.New(buffer.NewReadOnly(virtual, []byte("+x\n")), 4)})
	saved := saveSplit(st, st.root)
	if len(saved.Tabs) != 1 || saved.Tabs[0].Path != filepath.Join(a.dirPath(), "a.txt") {
		t.Fatalf("saved %+v", saved.Tabs)
	}
}
