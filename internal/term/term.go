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
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/aymanbagabas/go-pty"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/gdamore/tcell/v2"
)

const (
	scrollbackLines = 1000 // P-07: 36 MB per full tab, against 185 MB at 10,000.
	stopWait        = 2 * time.Second
	wheelLines      = 3 // How far one wheel notch scrolls back through the output.
	maxDigits       = 4 // In one number of a control sequence, so at most 9,999.
	maxSequence     = 4096
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

	lastOutput atomic.Int64 // When output last arrived, in Unix nanoseconds.
	bells      atomic.Int64 // How many times the command has rung the bell.
	mouseModes map[ansi.Mode]bool
	viewBack   int    // How many lines up the view is scrolled, from the live screen.
	unfinished []byte // A control sequence whose end hasn't arrived yet.
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
	cmd.Env = append(toolEnv(os.Environ()), "TERM=xterm-256color", "COLORTERM=truecolor")
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
	t.lastOutput.Store(time.Now().UnixNano())
	t.mouseModes = map[ansi.Mode]bool{}
	t.emu.SetCallbacks(vt.Callbacks{
		Bell:        func() { t.bells.Add(1) },
		EnableMode:  func(mode ansi.Mode) { t.setMouseMode(mode, true) },
		DisableMode: func(mode ansi.Mode) { t.setMouseMode(mode, false) },
	})

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

// toolEnv removes what would make a tool think it's running inside another copy of itself.
// With CLAUDE* variables set, as when LazyCode is started from inside Claude Code, Claude Code
// turns off saving its transcript, and then a session can't resume (M0-08).
func toolEnv(env []string) []string {
	return slices.DeleteFunc(slices.Clone(env), func(entry string) bool {
		return strings.HasPrefix(strings.ToUpper(entry), "CLAUDE")
	})
}

var mouseReportModes = []ansi.Mode{
	ansi.ModeMouseX10, ansi.ModeMouseNormal, ansi.ModeMouseHighlight, ansi.ModeMouseButtonEvent, ansi.ModeMouseAnyEvent,
}

// setMouseMode runs from the emulator, while t.mu is held by write.
func (t *Tab) setMouseMode(mode ansi.Mode, on bool) {
	if slices.Contains(mouseReportModes, mode) {
		t.mouseModes[mode] = on
	}
}

func (t *Tab) pumpOutput(onOutput func()) {
	buf := make([]byte, 32*1024)
	for {
		n, err := t.pty.Read(buf)
		if n > 0 {
			t.write(buf[:n])
			onOutput()
		}
		if err != nil {
			return
		}
	}
}

// survive keeps a panic in the emulator from taking the editor down, and unsaved work
// with it. vt v0.1.0 panics on some output, such as a scroll region bigger than the screen
// followed by an inserted line (FuzzEmulatorOutput found it), and any tool or printed file
// can send that. The tab keeps going, though the screen may look wrong until the tool redraws.
func survive() { _ = recover() } //nolint:errcheck // There's nothing useful to do with it.

// write gives output to the emulator a piece at a time, cut around escape sequences. The panics
// happen when a sequence is acted on, so that way one only loses that sequence, and not whatever
// else arrived with it. A sequence still arriving waits for the rest, so that its numbers can
// be checked as a whole.
func (t *Tab) write(data []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastOutput.Store(time.Now().UnixNano())
	if len(t.unfinished) > 0 {
		data = append(t.unfinished, data...)
		t.unfinished = nil
	}
	data, tail := splitUnfinishedRune(data)
	data = expandC1(data)
	for len(data) > 0 {
		n := nextPiece(data)
		piece := data[:n]
		if n == len(data) && unfinishedSequence(piece) {
			tail = append(bytes.Clone(piece), tail...)
			break
		}
		t.writePiece(capNumbers(piece))
		data = data[n:]
	}
	t.unfinished = bytes.Clone(tail)
}

// splitUnfinishedRune holds back a UTF-8 character whose end is in the next read.
func splitUnfinishedRune(data []byte) (whole, tail []byte) {
	for back := 1; back <= min(utf8.UTFMax-1, len(data)); back++ {
		at := len(data) - back
		if utf8.RuneStart(data[at]) {
			if data[at] >= utf8.RuneSelf && !utf8.FullRune(data[at:]) {
				return data[:at], data[at:]
			}
			break
		}
	}
	return data, nil
}

// expandC1 rewrites the one-byte form of a control sequence's start as ESC [, so that the rest of
// write sees every control sequence. 0x9b inside a UTF-8 character isn't one.
func expandC1(data []byte) []byte {
	if bytes.IndexByte(data, 0x9b) < 0 {
		return data
	}
	out := make([]byte, 0, len(data)+8)
	for i := 0; i < len(data); {
		if data[i] == 0x9b {
			out = append(out, 0x1b, '[')
			i++
			continue
		}
		_, size := utf8.DecodeRune(data[i:])
		out = append(out, data[i:i+size]...)
		i += size
	}
	return out
}

// unfinishedSequence reports whether piece is a control sequence with its end yet to come. A
// very long one isn't kept waiting: nothing real looks like that.
func unfinishedSequence(piece []byte) bool {
	if piece[0] != 0x1b || len(piece) > maxSequence {
		return false
	}
	if len(piece) == 1 {
		return true
	}
	last := piece[len(piece)-1]
	return piece[1] == '[' && (len(piece) == 2 || last < 0x40 || last > 0x7e)
}

// capNumbers limits the numbers in a control sequence. vt v0.1.0 loops on them while holding the
// lock that drawing needs, so printing "x" then ESC[200000000b would freeze the whole editor.
// Nothing real needs more: the cap is far past any screen.
func capNumbers(piece []byte) []byte {
	if len(piece) < 2 || piece[0] != 0x1b || piece[1] != '[' {
		return piece
	}
	out, run := piece[:0:0], 0
	for _, b := range piece {
		if b >= '0' && b <= '9' {
			if run++; run > maxDigits {
				continue
			}
		} else {
			run = 0
		}
		out = append(out, b)
	}
	return out
}

func (t *Tab) writePiece(piece []byte) {
	defer func() {
		if recover() != nil {
			// The parser stopped part way through the sequence, and would swallow what
			// comes next. CAN cancels a sequence, and shows nothing.
			defer survive()
			t.emu.Write([]byte{0x18}) //nolint:errcheck // As below.
		}
	}()
	t.emu.Write(piece) //nolint:errcheck // The emulator only fails once closed.
}

// nextPiece returns the length of data's first piece: plain text up to an escape sequence, or a
// control sequence with the byte that ends it. Other sequences run up to the next escape.
func nextPiece(data []byte) int {
	if data[0] != 0x1b {
		if i := bytes.IndexByte(data, 0x1b); i >= 0 {
			return i
		}
		return len(data)
	}
	if len(data) > 1 && data[1] == '[' {
		for i := 2; i < len(data); i++ {
			if data[i] == 0x1b { // A new sequence replaces this one.
				return i
			}
			if data[i] >= 0x40 && data[i] <= 0x7e {
				return i + 1
			}
		}
		return len(data)
	}
	if i := bytes.IndexByte(data[1:], 0x1b); i >= 0 {
		return i + 1
	}
	return len(data)
}

func (t *Tab) resizeScreen(width, height int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	defer survive()
	t.emu.Resize(width, height)
	t.viewBack = 0
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
	t.resizeScreen(width, height)
	t.pty.Resize(width, height) //nolint:errcheck // Fails only once the command is gone.
}

// SetScrollback sets how many lines the tab keeps. See scrollbackLines for what they cost.
func (t *Tab) SetScrollback(lines int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.emu.SetScrollbackSize(lines)
}

// Mouse sends a mouse event at a position in the tab if the command turned mouse reporting
// on. Otherwise the wheel scrolls back through what the tab has kept.
func (t *Tab) Mouse(x, y int, wheelUp, wheelDown, pressed bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	defer survive()
	wanted := false
	for _, on := range t.mouseModes {
		wanted = wanted || on
	}
	if !wanted {
		switch {
		case wheelUp:
			t.scroll(wheelLines)
		case wheelDown:
			t.scroll(-wheelLines)
		}
		return
	}
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

// scroll moves the view. A full-screen program's alternate screen has nothing to scroll to.
func (t *Tab) scroll(lines int) {
	if t.emu.IsAltScreen() {
		t.viewBack = 0
		return
	}
	t.viewBack = max(0, min(t.viewBack+lines, t.emu.ScrollbackLen()))
}

// ScrolledBack returns how many lines the view is above the live screen.
func (t *Tab) ScrolledBack() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.viewBack
}

// LastOutput returns when output last arrived, or when the tab started.
func (t *Tab) LastOutput() time.Time { return time.Unix(0, t.lastOutput.Load()) }

// Bells returns how many times the command has rung the terminal bell.
func (t *Tab) Bells() int64 { return t.bells.Load() }

// Paste sends text the way the tool asked for it, using bracketed paste if enabled.
func (t *Tab) Paste(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	defer survive()
	t.viewBack = 0
	t.emu.Paste(text)
}

// Key sends a key to the command.
func (t *Tab) Key(ev *tcell.EventKey) {
	t.mu.Lock()
	t.viewBack = 0 // Typing goes back to the live screen, as in any terminal.
	t.mu.Unlock()
	if seq := modifiedKeySeq(ev); seq != "" {
		io.WriteString(t.emu.InputPipe(), seq) //nolint:errcheck // Fails only once closed.
		return
	}
	if k, ok := toEmulatorKey(ev); ok {
		t.mu.Lock()
		defer t.mu.Unlock()
		defer survive()
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

// Cursor returns the cursor position, and whether the command wants it shown. It's hidden
// while looking back through earlier output.
func (t *Tab) Cursor() (x, y int, visible bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	defer survive()
	p := t.emu.CursorPosition()
	return p.X, p.Y, !t.emu.CursorHidden() && t.viewBack == 0
}

// cellAt returns what's in a row of the view, which may be scrolled back.
func (t *Tab) cellAt(x, y int) *uv.Cell {
	if t.viewBack == 0 {
		return t.emu.CellAt(x, y)
	}
	if y < t.viewBack {
		return t.emu.ScrollbackCellAt(x, t.emu.ScrollbackLen()-t.viewBack+y)
	}
	return t.emu.CellAt(x, y-t.viewBack)
}

// Draw paints the terminal's screen into the given area.
func (t *Tab) Draw(s tcell.Screen, x0, y0, width, height int) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	defer survive()
	for y := range height {
		for x := 0; x < width; x++ {
			c := t.cellAt(x, y)
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
	defer survive()
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
