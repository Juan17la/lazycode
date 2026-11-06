package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Juan17la/lazycode/internal/config"
	"github.com/Juan17la/lazycode/internal/keymap"
	"github.com/gdamore/tcell/v2"
)

// drag presses at one place, moves to another, and lets go.
func (u *ui) drag(x0, y0, x1, y1 int) {
	u.post(tcell.NewEventMouse(x0, y0, tcell.Button1, 0))
	u.post(tcell.NewEventMouse(x1, y1, tcell.Button1, 0))
	u.release(x1, y1)
}

// find returns where text is on screen.
func (u *ui) find(text string) (x, y int) {
	u.t.Helper()
	u.wait(text)
	for row, line := range strings.Split(u.text(), "\n") {
		if i := strings.Index(line, text); i >= 0 {
			return len([]rune(line[:i])), row
		}
	}
	u.t.Fatalf("no %q", text)
	return 0, 0
}

// dividerAt returns the column of the first vertical line on a row.
func (u *ui) dividerAt(row int) int {
	line := strings.Split(u.text(), "\n")[row]
	return len([]rune(line[:max(0, strings.Index(line, "│"))]))
}

// until waits for something that doesn't show as text.
func (u *ui) until(done func() bool) {
	u.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			u.t.Fatalf("never happened:\n%s", u.text())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func settingsFile(t *testing.T, name string) string {
	t.Helper()
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, name)
}

func TestTerminalsGoToTheDockAndCodeStaysAbove(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "file text\n"})
	u.open("a.txt")
	u.key(tcell.KeyEsc)
	u.press(":term")
	u.key(tcell.KeyEnter)
	_, codeRow := u.find("◫")
	u.wait("❯+") // The dock's own buttons.
	_, dockRow := u.find("❯+")
	if dockRow <= codeRow {
		t.Errorf("dock on row %d, code on row %d", dockRow, codeRow)
	}
	// Putting it away leaves the tool running, and the status line can bring it back.
	u.key(tcell.KeyCtrlBackslash)
	u.alt('t')
	u.waitGone("❯+")
	u.alt('t')
	u.wait("❯+")
}

func TestTabCloseButton(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\n"})
	u.open("a.txt")
	x, y := u.find("× ")
	u.click(x, y)
	u.waitGone("× ")
	u.wait("No tabs here")
}

func TestDragSelectsAndDoubleClickTakesAWord(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "alpha beta\n"})
	u.open("a.txt")
	x, y := u.find("alpha beta")
	u.drag(x, y, x+5, y)
	u.press("Z")
	u.wait("Z beta")
	x, y = u.find("beta")
	u.click(x+1, y)
	u.click(x+1, y)
	u.press("Q")
	u.wait("Z Q")
}

func TestDraggingThePanelEdgeResizesIt(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\n"})
	u.wait("a.txt")
	edge := u.dividerAt(5)
	u.drag(edge, 5, edge+7, 5)
	u.until(func() bool { return u.dividerAt(5) == edge+7 })
}

func TestWordWrapShowsTheWholeLine(t *testing.T) {
	long := strings.Repeat("word ", 40) + "theend"
	u := start(t, map[string]string{"a.txt": long + "\nnext\n"})
	u.open("a.txt")
	u.waitGone("theend")
	u.key(tcell.KeyEsc)
	u.press(":wrap")
	u.key(tcell.KeyEnter)
	u.wait("theend")
	u.wait("next")
}

func TestNewProjectAndAddDirectoryForms(t *testing.T) {
	other := t.TempDir()
	u := start(t, map[string]string{"a.txt": "one\n"})
	u.wait("a.txt")
	u.post(tcell.NewEventKey(tcell.KeyRune, 'P', tcell.ModAlt))
	u.wait("New project")
	u.press("demo")
	u.key(tcell.KeyEnter)
	u.wait("Created project demo")

	u.post(tcell.NewEventKey(tcell.KeyRune, 'F', tcell.ModAlt))
	u.wait("Add a directory")
	for range 200 {
		u.key(tcell.KeyBackspace2)
	}
	u.press(other)
	u.key(tcell.KeyEnter)
	u.wait("Added " + filepath.Base(other))
}

func TestSettingsAreSavedFromTheirMenus(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\n"})
	u.wait("a.txt")
	u.key(tcell.KeyEsc)
	u.press(":appearance")
	u.key(tcell.KeyEnter)
	u.wait("Appearance")
	u.key(tcell.KeyDown) // Past Themes, to Icons.
	u.key(tcell.KeyRight)
	u.wait("‹ nerd ›")
	u.key(tcell.KeyEsc)
	u.waitGone("Appearance")
	cfg, err := config.Load(settingsFile(t, "config.toml"))
	if err != nil || cfg.UI.Icons != "nerd" {
		t.Errorf("saved %q, %v", cfg.UI.Icons, err)
	}
}

func TestThemesMenuPreviewsAndGoesBack(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\n"})
	u.wait("a.txt")
	barColour := func() tcell.Color {
		_, style, _ := u.screen.Get(70, 0)
		_, bg, _ := style.Decompose()
		return bg
	}
	u.wait("a.txt")
	before := barColour()
	u.key(tcell.KeyEsc)
	u.press(":themes")
	u.key(tcell.KeyEnter)
	u.wait("monokai")
	u.key(tcell.KeyUp) // Round to the last one.
	u.until(func() bool { return barColour() != before })
	u.key(tcell.KeyEsc)
	u.until(func() bool { return barColour() == before })
}

func TestShortcutsMenuChangesAKey(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\n"})
	u.wait("a.txt")
	u.key(tcell.KeyF1)
	u.wait("Shortcuts")
	u.press("Explorer")
	u.key(tcell.KeyEnter)
	u.wait("Press the new key")
	u.alt('j')
	u.wait("Explorer is now Alt+J")
	if _, err := os.Stat(settingsFile(t, "keys.toml")); err != nil {
		t.Error(err)
	}
}

func TestMenuBarRunsAnItem(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\n"})
	u.wait("a.txt")
	x, y := u.find("Help")
	u.click(x, y)
	u.wait("Welcome Screen")
	x, y = u.find("Welcome Screen")
	u.click(x, y)
	u.wait("Welcome to LazyCode")
	u.key(tcell.KeyEsc) // Coming back to it mustn't change anything.
	u.waitGone("Welcome to LazyCode")
}

func TestDraggedSizesComeBack(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\n"})
	u.wait("a.txt")
	edge := u.dividerAt(5)
	u.drag(edge, 5, edge+7, 5)
	u.until(func() bool { return u.dividerAt(5) == edge+7 })
	u.run("q")
	u.quit()
	later := launch(t, u.dir)
	later.wait("a.txt")
	later.until(func() bool { return later.dividerAt(5) == edge+7 })
}

func TestDragTabToAnotherSplit(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "aaa\n", "b.txt": "bbb\n"})
	u.open("a.txt")
	u.post(tcell.NewEventKey(tcell.KeyRune, 'S', tcell.ModAlt)) // Both show a, and focus is on the right.
	u.waitCount("aaa", 2)
	u.key(tcell.KeyCtrlBackslash)
	u.key(tcell.KeyDown)
	u.key(tcell.KeyEnter) // b opens on the right, beside its a.
	u.wait("bbb")
	x, y := u.find("b.txt ×")
	leftBar, _ := u.find("a.txt ×")
	u.drag(x, y, leftBar, y)
	u.until(func() bool {
		at, _ := u.find("b.txt ×")
		return at < x
	})
	u.waitCount("aaa", 1) // The right split is back to a, and the left now shows b.
	u.waitCount("bbb", 1)
}

func TestTreeMenuRenamesAndDeletes(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\n"})
	x, y := u.find("a.txt")
	u.post(tcell.NewEventMouse(x, y, tcell.Button2, 0))
	u.release(x, y)
	u.wait("Rename")
	mx, my := u.find("Rename")
	u.click(mx, my)
	u.wait("✎ Rename")
	u.press("x")
	u.key(tcell.KeyEnter)
	u.wait("a.txtx")
	if _, err := os.Stat(filepath.Join(u.dir, "a.txtx")); err != nil {
		t.Error(err)
	}
	x, y = u.find("a.txtx")
	u.post(tcell.NewEventMouse(x, y, tcell.Button2, 0))
	u.release(x, y)
	mx, my = u.find("Delete")
	u.click(mx, my)
	u.wait("for good?")
	u.press("c") // Cancel keeps it.
	u.waitGone("for good?")
	if _, err := os.Stat(filepath.Join(u.dir, "a.txtx")); err != nil {
		t.Error(err)
	}
	u.post(tcell.NewEventMouse(x, y, tcell.Button2, 0))
	u.release(x, y)
	mx, my = u.find("Delete")
	u.click(mx, my)
	u.press("d")
	u.waitGone("a.txtx")
	if _, err := os.Stat(filepath.Join(u.dir, "a.txtx")); err == nil {
		t.Error("still there")
	}
}

func TestEveryMenuItemDoesSomethingKnown(t *testing.T) {
	known := keymap.Actions()
	known = append(known, "app.appearance", "app.themes", "app.plugins", "app.store", "app.onboarding")
	for _, group := range menuBar {
		for _, item := range group.items {
			if item.label == "" {
				continue
			}
			if item.action == "" && item.command == "" && item.do == nil {
				t.Errorf("%s has nothing to do", item.label)
			}
			if item.action != "" && !slices.Contains(known, item.action) {
				t.Errorf("%s runs %q", item.label, item.action)
			}
		}
	}
}

func TestMenusOpenAndScrollInASmallWindow(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\n"})
	u.screen.SetSize(80, 10)
	u.post(tcell.NewEventResize(80, 10))
	u.wait("a.txt")
	x, y := u.find("View")
	u.click(x, y)
	u.wait("Command Bar")
	u.key(tcell.KeyUp) // Round to the last item, which starts off screen.
	u.wait("Themes")
	u.key(tcell.KeyEnter)
	u.wait("◉ Themes") // The window is too short for the list, so the first ones show.
}

func TestGoToLineAndAbout(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one\ntwo\nthree\n"})
	u.open("a.txt")
	x, y := u.find("Go ")
	u.click(x, y)
	mx, my := u.find("Go to Line")
	u.click(mx, my)
	u.wait("1 to 4")
	u.press("3")
	u.key(tcell.KeyEnter)
	u.wait("Ln 3, Col 1")
	x, y = u.find("Help")
	u.click(x, y)
	mx, my = u.find("About LazyCode")
	u.click(mx, my)
	u.wait("Version")
}
