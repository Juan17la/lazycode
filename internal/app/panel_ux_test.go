package app

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestAltTabMovesFocusBetweenAreas(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "hello"})
	u.open("a.txt")
	u.wait("EDIT")
	u.mod(tcell.KeyTab, tcell.ModAlt) // From the file to the panel.
	u.wait("FILES")
	u.waitGone("EDIT")
	u.mod(tcell.KeyTab, tcell.ModAlt) // And back to the file.
	u.wait("EDIT")
}

func TestFocusedPanelHasAThickBorder(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "x"})
	u.wait("┃") // Files has the keys.
	u.open("a.txt")
	u.mod(tcell.KeyTab, tcell.ModAlt)
	u.wait("┃")
}

func TestSessionRowXDeletesIt(t *testing.T) {
	u := withTool(t, "stay", nil)
	u.startFake()
	u.key(tcell.KeyCtrlBackslash)
	u.alt('a')
	_, y := u.find("✻ Fake 1")
	u.click(u.dividerAt(y)-2, y) // The × at the end of the row.
	u.wait("No sessions yet")
}
