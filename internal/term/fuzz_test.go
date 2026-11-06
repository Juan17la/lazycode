package term

import (
	"bytes"
	"io"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/gdamore/tcell/v2"
)

// Whatever a tool prints must not crash the editor, where it would take unsaved work
// with it (doc 13).
func FuzzEmulatorOutput(f *testing.F) {
	for _, seed := range []string{
		"plain text\r\n",
		"\x1b[31mred\x1b[0m \x1b[2J\x1b[H",
		"\x1b[?1049h\x1b[?1000h\x1b[?1006h\x1b[?2004h",
		"\x1b]0;title\x07\x1b]52;c;aGk=\x07\x1b]9;note\x07",
		"\x1b[999;999H\x1b[99999A\x1b[r\x1b[0;0r",
		"世界😀é\x1b[1;200r\x1b[L\x1b[M",
		"\x1bP+q\x1b\\\x1b_x\x1b\\\x1b^y\x1b\\",
		"\x1b[>4;2m\x1b[=1;1u\x1b[6n\x1b[c",
		"x\x1b[200000000b\x1b[2000000000Z\x1b[200000000X",
	} {
		f.Add([]byte(seed), 80, 24)
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, data []byte, w, h int) {
		w, h = 1+abs(w)%200, 1+abs(h)%60
		emu := vt.NewEmulator(w, h)
		emu.SetScrollbackSize(100)
		done := make(chan struct{})
		go func() { // Replies to queries go here, as they would to the tool.
			defer close(done)
			buf := make([]byte, 4096)
			for {
				if _, err := emu.Read(buf); err != nil {
					return
				}
			}
		}()
		tab := &Tab{emu: emu, mouseModes: map[ansi.Mode]bool{}}
		emu.SetCallbacks(vt.Callbacks{EnableMode: func(mode ansi.Mode) { tab.setMouseMode(mode, true) }})
		for chunk := range bytes.Lines(data) {
			tab.write(chunk)
		}
		tab.resizeScreen(1+w/2, 1+h/2)
		tab.write(data)
		tab.Draw(screen, 0, 0, w, h)
		tab.Text()
		tab.Cursor()
		tab.Mouse(1, 1, true, false, false)
		tab.Draw(screen, 0, 0, w, h)
		emu.InputPipe().(io.Closer).Close() // As Tab.Close does: Emulator.Close races with Read.
		<-done
	})
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
