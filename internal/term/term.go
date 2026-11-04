// Package term runs a command in a pseudo-terminal and shows it as a tab.
// It is the only package that knows about the PTY and emulator libraries.
package term

import (
	"bytes"
	"fmt"
	"image/color"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/aymanbagabas/go-pty"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/gdamore/tcell/v2"
)

const (
	scrollbackLines = 1000 // P-07: 36 MB per full tab, against 185 MB at 10,000.
	stopWait        = 2 * time.Second
	// inputQueue sits between the emulator's unbuffered pipe and the PTY. Without
	// it a paste holds the emulator lock while the PTY is full, the output reader
	// can't take that lock, the tool blocks on output and stops reading: deadlock.
	// ponytail: 1,024 chunks of up to 32 KB, make it unbounded if pastes ever reach 32 MB.
	inputQueue = 1024
)

// Tab is a running command and its screen.
type Tab struct {
	Name string

	// mu guards emu. Not vt.SafeEmulator: its CellAt returns a pointer into the live
	// screen after releasing its lock, which races with output. Holding mu for a
	// whole frame also means a frame can't tear.
	mu     sync.RWMutex
	emu    *vt.Emulator
	pty    pty.Pty
	cmd    *pty.Cmd
	exited chan struct{}
	code   int // Set before exited closes.
	stop   sync.Once
}

// Start runs args in dir. onOutput runs on another goroutine whenever the screen
// changes or the command exits, so it must not block.
func Start(dir string, args []string, width, height int, onOutput func()) (*Tab, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("no command given")
	}
	// Resolve here: go-pty on Windows would look the name up relative to dir.
	path, err := exec.LookPath(args[0])
	if err != nil {
		return nil, fmt.Errorf("%s isn't installed or isn't on PATH", args[0])
	}
	p, err := pty.New()
	if err != nil {
		return nil, fmt.Errorf("open terminal: %w", err)
	}
	if err := p.Resize(width, height); err != nil {
		p.Close()
		return nil, fmt.Errorf("size terminal: %w", err)
	}
	cmd := p.Command(path, args[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	if err := cmd.Start(); err != nil {
		p.Close()
		return nil, fmt.Errorf("start %s: %w", args[0], err)
	}

	t := &Tab{
		Name:   strings.TrimSuffix(filepath.Base(args[0]), filepath.Ext(args[0])),
		emu:    vt.NewEmulator(width, height),
		pty:    p,
		cmd:    cmd,
		exited: make(chan struct{}),
	}
	t.emu.SetScrollbackSize(scrollbackLines)

	go t.pumpOutput(onOutput)
	go t.pumpInput()
	go func() {
		cmd.Wait() //nolint:errcheck // The exit code below carries the result.
		if cmd.ProcessState != nil {
			t.code = cmd.ProcessState.ExitCode()
		}
		close(t.exited)
		onOutput()
	}()
	return t, nil
}

func (t *Tab) pumpOutput(onOutput func()) {
	buf := make([]byte, 32*1024)
	for {
		n, err := t.pty.Read(buf)
		if n > 0 {
			t.mu.Lock()
			t.emu.Write(buf[:n]) //nolint:errcheck // The emulator only fails once closed.
			t.mu.Unlock()
			onOutput()
		}
		if err != nil {
			return
		}
	}
}

func (t *Tab) pumpInput() {
	queue := make(chan []byte, inputQueue)
	go func() {
		for b := range queue {
			if _, err := t.pty.Write(b); err != nil {
				for range queue { // Keep draining so the emulator never blocks.
				}
				return
			}
		}
	}()
	defer close(queue)
	buf := make([]byte, 32*1024)
	for {
		n, err := t.emu.Read(buf)
		if n > 0 {
			queue <- bytes.Clone(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// Exited reports whether the command has ended, and its exit code.
func (t *Tab) Exited() (bool, int) {
	select {
	case <-t.exited:
		return true, t.code
	default:
		return false, 0
	}
}

// Pid returns the command's process ID.
func (t *Tab) Pid() int { return t.cmd.Process.Pid }

// Done is closed when the command has ended.
func (t *Tab) Done() <-chan struct{} { return t.exited }

// Close asks the command to stop by hanging up its terminal, and forces it after
// a short wait.
func (t *Tab) Close() {
	t.stop.Do(func() {
		t.pty.Close()
		select {
		case <-t.exited:
		case <-time.After(stopWait):
			if t.cmd.Process != nil {
				t.cmd.Process.Kill() //nolint:errcheck // It may have just exited.
			}
		}
		// Close the pipe, not the emulator: Emulator.Close races with its Read.
		if c, ok := t.emu.InputPipe().(io.Closer); ok {
			c.Close()
		}
	})
}

// Resize changes the size of the screen and tells the command.
func (t *Tab) Resize(width, height int) {
	t.mu.Lock()
	t.emu.Resize(width, height)
	t.mu.Unlock()
	t.pty.Resize(width, height) //nolint:errcheck // Fails only once the command is gone.
}

// SetScrollback sets how many lines the tab keeps. See scrollbackLines for what they cost.
func (t *Tab) SetScrollback(lines int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.emu.SetScrollbackSize(lines)
}

// Mouse sends a mouse event at a position in the tab. The emulator only passes it on if the
// command turned mouse reporting on.
func (t *Tab) Mouse(x, y int, wheelUp, wheelDown, pressed bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch {
	case wheelUp:
		t.emu.SendMouse(vt.MouseWheel{X: x, Y: y, Button: vt.MouseWheelUp})
	case wheelDown:
		t.emu.SendMouse(vt.MouseWheel{X: x, Y: y, Button: vt.MouseWheelDown})
	case pressed:
		t.emu.SendMouse(vt.MouseClick{X: x, Y: y, Button: vt.MouseLeft})
	default:
		t.emu.SendMouse(vt.MouseRelease{X: x, Y: y, Button: vt.MouseLeft})
	}
}

// Paste sends text the way the tool asked for it, using bracketed paste if enabled.
func (t *Tab) Paste(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.emu.Paste(text)
}

// Key sends a key to the command.
func (t *Tab) Key(ev *tcell.EventKey) {
	if seq := modifiedKeySeq(ev); seq != "" {
		io.WriteString(t.emu.InputPipe(), seq) //nolint:errcheck // Fails only once closed.
		return
	}
	if k, ok := toEmulatorKey(ev); ok {
		t.mu.Lock()
		defer t.mu.Unlock()
		t.emu.SendKey(k)
	}
}

var emulatorKeys = map[tcell.Key]rune{
	tcell.KeyEnter: vt.KeyEnter, tcell.KeyTab: vt.KeyTab, tcell.KeyEsc: vt.KeyEscape,
	tcell.KeyBackspace: vt.KeyBackspace, tcell.KeyBackspace2: vt.KeyBackspace,
	tcell.KeyUp: vt.KeyUp, tcell.KeyDown: vt.KeyDown, tcell.KeyLeft: vt.KeyLeft, tcell.KeyRight: vt.KeyRight,
	tcell.KeyHome: vt.KeyHome, tcell.KeyEnd: vt.KeyEnd, tcell.KeyPgUp: vt.KeyPgUp, tcell.KeyPgDn: vt.KeyPgDown,
	tcell.KeyInsert: vt.KeyInsert, tcell.KeyDelete: vt.KeyDelete,
	tcell.KeyF1: uv.KeyF1, tcell.KeyF2: uv.KeyF2, tcell.KeyF3: uv.KeyF3, tcell.KeyF4: uv.KeyF4,
	tcell.KeyF5: uv.KeyF5, tcell.KeyF6: uv.KeyF6, tcell.KeyF7: uv.KeyF7, tcell.KeyF8: uv.KeyF8,
	tcell.KeyF9: uv.KeyF9, tcell.KeyF10: uv.KeyF10, tcell.KeyF11: uv.KeyF11, tcell.KeyF12: uv.KeyF12,
}

var ctrlPunctuation = map[tcell.Key]rune{
	tcell.KeyCtrlSpace: vt.KeySpace, tcell.KeyCtrlLeftSq: '[', tcell.KeyCtrlBackslash: '\\',
	tcell.KeyCtrlRightSq: ']', tcell.KeyCtrlCarat: '^', tcell.KeyCtrlUnderscore: '_',
	tcell.KeyFS: '\\', tcell.KeyGS: ']', tcell.KeyRS: '^', tcell.KeyUS: '_',
}

// windowsCtrlRunes undoes a tcell quirk on Windows: it adds 0x60 to control
// characters, which suits letters but turns Ctrl+\ into Ctrl+|, and so on.
var windowsCtrlRunes = map[rune]rune{'{': '[', '|': '\\', '}': ']', '~': '^'}

// IsLeaveKey reports whether ev is Ctrl+\, however this OS's terminal delivers it.
func IsLeaveKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyCtrlBackslash, tcell.KeyFS:
		return true
	case tcell.KeyRune:
		r := ev.Rune()
		return ev.Modifiers()&tcell.ModCtrl != 0 && (r == '\\' || r == '|')
	}
	return false
}

func toEmulatorKey(ev *tcell.EventKey) (vt.KeyPressEvent, bool) {
	var mod vt.KeyMod
	if ev.Modifiers()&tcell.ModAlt != 0 {
		mod |= vt.ModAlt
	}
	key := ev.Key()
	switch {
	case key == tcell.KeyRune:
		r := ev.Rune()
		if ev.Modifiers()&tcell.ModCtrl != 0 {
			if base, ok := windowsCtrlRunes[r]; ok {
				r = base
			}
			return vt.KeyPressEvent{Code: unicode.ToLower(r), Mod: mod | vt.ModCtrl}, true
		}
		return vt.KeyPressEvent{Code: r, Text: string(r), Mod: mod}, r != 0
	case key >= tcell.KeyCtrlA && key <= tcell.KeyCtrlZ:
		return vt.KeyPressEvent{Code: 'a' + rune(key-tcell.KeyCtrlA), Mod: mod | vt.ModCtrl}, true
	}
	if code, ok := ctrlPunctuation[key]; ok {
		return vt.KeyPressEvent{Code: code, Mod: mod | vt.ModCtrl}, true
	}
	code, ok := emulatorKeys[key]
	return vt.KeyPressEvent{Code: code, Mod: mod}, ok
}

// Letter and number endings of the xterm sequences for modified keys.
var (
	modifiedLetter = map[tcell.Key]string{
		tcell.KeyUp: "A", tcell.KeyDown: "B", tcell.KeyRight: "C", tcell.KeyLeft: "D",
		tcell.KeyHome: "H", tcell.KeyEnd: "F",
		tcell.KeyF1: "P", tcell.KeyF2: "Q", tcell.KeyF3: "R", tcell.KeyF4: "S",
	}
	modifiedNumber = map[tcell.Key]int{
		tcell.KeyInsert: 2, tcell.KeyDelete: 3, tcell.KeyPgUp: 5, tcell.KeyPgDn: 6,
		tcell.KeyF5: 15, tcell.KeyF6: 17, tcell.KeyF7: 18, tcell.KeyF8: 19,
		tcell.KeyF9: 20, tcell.KeyF10: 21, tcell.KeyF11: 23, tcell.KeyF12: 24,
	}
)

// modifiedKeySeq encodes what vt.SendKey silently drops: special keys with a
// modifier, such as Shift+Tab or Ctrl+Left. AI tools rely on these.
func modifiedKeySeq(ev *tcell.EventKey) string {
	key, mods := ev.Key(), ev.Modifiers()
	if key == tcell.KeyBacktab || (key == tcell.KeyTab && mods&tcell.ModShift != 0) {
		return "\x1b[Z"
	}
	n := 1
	if mods&tcell.ModShift != 0 {
		n++
	}
	if mods&tcell.ModAlt != 0 {
		n += 2
	}
	if mods&tcell.ModCtrl != 0 {
		n += 4
	}
	if n == 1 {
		return ""
	}
	if s, ok := modifiedLetter[key]; ok {
		return fmt.Sprintf("\x1b[1;%d%s", n, s)
	}
	if num, ok := modifiedNumber[key]; ok {
		return fmt.Sprintf("\x1b[%d;%d~", num, n)
	}
	return ""
}

// Cursor returns the cursor position, and whether the command wants it shown.
func (t *Tab) Cursor() (x, y int, visible bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	p := t.emu.CursorPosition()
	return p.X, p.Y, !t.emu.CursorHidden()
}

// Draw paints the terminal's screen into the given area.
func (t *Tab) Draw(s tcell.Screen, x0, y0, width, height int) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for y := range height {
		for x := 0; x < width; x++ {
			c := t.emu.CellAt(x, y)
			switch {
			case c == nil || c.Content == "":
				if c == nil || c.Width != 0 { // Width 0 continues a wide character.
					s.SetContent(x0+x, y0+y, ' ', nil, tcell.StyleDefault)
				}
			default:
				r := []rune(c.Content)
				s.SetContent(x0+x, y0+y, r[0], r[1:], toStyle(c.Style))
			}
		}
	}
}

// Text returns the screen as plain text, one line per row.
func (t *Tab) Text() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var b strings.Builder
	for y := range t.emu.Height() {
		var line strings.Builder
		for x := range t.emu.Width() {
			if c := t.emu.CellAt(x, y); c != nil {
				line.WriteString(c.Content)
			}
		}
		b.WriteString(strings.TrimRight(line.String(), " "))
		b.WriteByte('\n')
	}
	return b.String()
}

func toStyle(st uv.Style) tcell.Style {
	style := tcell.StyleDefault.
		Foreground(toColor(st.Fg)).
		Background(toColor(st.Bg)).
		Bold(st.Attrs&uv.AttrBold != 0).
		Dim(st.Attrs&uv.AttrFaint != 0).
		Italic(st.Attrs&uv.AttrItalic != 0).
		Blink(st.Attrs&uv.AttrBlink != 0).
		Reverse(st.Attrs&uv.AttrReverse != 0).
		StrikeThrough(st.Attrs&uv.AttrStrikethrough != 0)
	if st.Underline != 0 {
		style = style.Underline(true)
	}
	return style
}

// toColor keeps palette colours as palette colours, so the user's terminal theme
// still applies inside a terminal tab.
func toColor(c color.Color) tcell.Color {
	switch v := c.(type) {
	case nil:
		return tcell.ColorDefault
	case ansi.BasicColor:
		return tcell.PaletteColor(int(v))
	case ansi.IndexedColor:
		return tcell.PaletteColor(int(v))
	default:
		return tcell.FromImageColor(c)
	}
}
