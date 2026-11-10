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
