package app

import (
	"testing"

	"github.com/Juan17la/lazycode/internal/project"
	"github.com/gdamore/tcell/v2"
)

func (u *ui) ctrlSpace() { u.mod(tcell.KeyCtrlSpace, tcell.ModCtrl) }

// alphaAndBeta starts in alpha, with beta beside it.
func alphaAndBeta(t *testing.T) *ui {
	t.Helper()
	u := start(t, nil)
	other := sibling(t, u, "other")
	all := []project.Project{{Name: "alpha", Dirs: []string{u.dir}}, {Name: "beta", Dirs: []string{other}}}
	if err := project.Save(projectsFile(t), all); err != nil {
		t.Fatal(err)
	}
	u.quitNow()
	v := launchWith(t, u.dir, all)
	v.wait("alpha ›")
	return v
}

func TestSwitchKeysReachLazyCodeFromATerminal(t *testing.T) {
	u := start(t, nil)
	u.run("term")
	u.wait("❯+")
	u.alt('p')
	u.wait("Switch project")
	u.key(tcell.KeyEsc)
	u.waitGone("Switch project")
	u.alt('d')
	u.wait("Switch folder")
}

func TestCtrlSpaceLeavesAndGoesBackIn(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\n"})
	u.open("a.txt")
	u.wait("Ln 1, Col 1")
	u.ctrlSpace()
	u.wait("Tab go to editor")
	u.ctrlSpace()
	u.wait("Ln 1, Col 1")
}

func TestAltPThenEnterGoesBackToTheLastProject(t *testing.T) {
	u := alphaAndBeta(t)
	u.alt('p')
	u.wait("Switch project")
	u.press("2") // A digit picks, while nothing is typed.
	u.wait("beta ›")
	u.alt('p')
	u.wait("Switch project")
	u.key(tcell.KeyEnter) // It starts on the project before this one.
	u.wait("alpha ›")
	u.alt('p')
	u.wait("Switch project")
	u.key(tcell.KeyEnter)
	u.wait("beta ›")
}

func TestTheSameKeyAgainMovesDownTheList(t *testing.T) {
	u := alphaAndBeta(t)
	u.alt('p')
	u.wait("Switch project")
	u.alt('p')
	u.key(tcell.KeyEnter)
	u.wait("beta ›")
}

func TestNInFilesMakesAFile(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\n"})
	u.wait("a.txt")
	u.press("n")
	u.wait("New file in")
}

func TestAltVMovesATerminalBetweenAreas(t *testing.T) {
	u := start(t, nil)
	u.run("term")
	u.wait("❯+")
	u.alt('v')
	u.wait("editor area")
	u.alt('v')
	u.wait("bottom panel")
}

func TestF2RenamesTheSelectedFile(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\n"})
	u.wait("a.txt")
	u.key(tcell.KeyDown) // Past the folder's own row, if there is one.
	u.key(tcell.KeyF2)
	u.wait("Rename")
}
