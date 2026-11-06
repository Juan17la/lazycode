package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Juan17la/lazycode/internal/config"
	"github.com/Juan17la/lazycode/internal/editor"
	"github.com/Juan17la/lazycode/internal/project"
	"github.com/Juan17la/lazycode/internal/syntax"
	"github.com/gdamore/tcell/v2"
)

// styledScreen also keeps each cell's style from the last frame.
type styledScreen struct {
	*recordingScreen
	mu    sync.Mutex
	cells []tcell.SimCell
	width int
}

func (s *styledScreen) Show() {
	s.recordingScreen.Show()
	cells, w, _ := s.GetContents()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cells, s.width = append(s.cells[:0], cells...), w
}

// styleOf returns the style of the first cell of the first place text appears on screen.
func (s *styledScreen) styleOf(u *ui, text string) (tcell.Style, bool) {
	frame := strings.Split(u.text(), "\n")
	s.mu.Lock()
	defer s.mu.Unlock()
	for y, line := range frame {
		// The frame has one rune per cell, so a rune index is a column.
		if x := strings.Index(string([]rune(line)), text); x >= 0 {
			col := len([]rune(line[:x]))
			if i := y*s.width + col; i < len(s.cells) {
				return s.cells[i].Style, true
			}
		}
	}
	return tcell.StyleDefault, false
}

func (s *styledScreen) waitStyle(u *ui, text string, want func(tcell.Style) bool, what string) {
	u.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if st, ok := s.styleOf(u, text); ok && want(st) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	st, ok := s.styleOf(u, text)
	u.t.Fatalf("%q never became %s (found=%v, style %+v):\n%s", text, what, ok, st, u.text())
}

// launchStyled starts the real UI, with a screen that remembers styles.
func launchStyled(t *testing.T, files map[string]string) (*ui, *styledScreen) {
	t.Helper()
	first := start(t, files) // Sets the folders up, and is stopped again.
	first.quitNow()
	rec := &recordingScreen{SimulationScreen: tcell.NewSimulationScreen("UTF-8")}
	screen := &styledScreen{recordingScreen: rec}
	u := &ui{t: t, screen: rec, dir: first.dir, exited: make(chan error, 1)}
	projects := []project.Project{{Name: "test", Dirs: []string{u.dir}}}
	go func() { u.exited <- Run(screen, projects, u.dir, "", "dev") }()
	u.wait("Files")
	t.Cleanup(func() {
		select {
		case <-u.exited:
		default:
			u.key(tcell.KeyEsc)
			u.key(tcell.KeyCtrlBackslash)
			u.run("q")
			for range 20 {
				select {
				case <-u.exited:
					return
				case <-time.After(50 * time.Millisecond):
					u.press("d")
				}
			}
		}
	})
	return u, screen
}

func attrs(st tcell.Style) tcell.AttrMask { _, _, a := st.Decompose(); return a }

func isKeyword(st tcell.Style) bool {
	fg, _, a := st.Decompose()
	return fg == tcell.GetColor(palettes[config.DefaultTheme]["purple"]) && a&tcell.AttrBold != 0 && a&tcell.AttrReverse == 0
}

func isComment(st tcell.Style) bool { return attrs(st)&tcell.AttrItalic != 0 }

func plain(st tcell.Style) bool {
	fg, _, a := st.Decompose()
	return fg == tcell.GetColor(palettes[config.DefaultTheme]["fg"]) && a == 0
}

const goFile = "package main\n\n// note\nfunc main() {\n\tprintln(\"hi\", 42)\n}\n"

func TestGoFileIsColoured(t *testing.T) {
	u, screen := launchStyled(t, map[string]string{"main.go": goFile, "notes.txt": "package main\n"})
	u.open("main.go")
	screen.waitStyle(u, "package", isKeyword, "a keyword")
	screen.waitStyle(u, "// note", isComment, "a comment")
	screen.waitStyle(u, "42", func(st tcell.Style) bool {
		fg, _, a := st.Decompose()
		return fg == tcell.GetColor(palettes[config.DefaultTheme]["orange"]) && a&tcell.AttrBold == 0
	}, "a number")
	screen.waitStyle(u, "main() {", func(st tcell.Style) bool {
		fg, _, _ := st.Decompose()
		return fg == tcell.GetColor(palettes[config.DefaultTheme]["blue"])
	}, "a function")
	u.wait("Go  Ln 1") // The status bar names the language.

	// A language without highlighting gets plain text.
	u.open("notes.txt")
	screen.waitStyle(u, "package main", plain, "plain")
	u.waitGone("Go  Ln")
}

func TestTypingAndUndoRecolour(t *testing.T) {
	u, screen := launchStyled(t, map[string]string{"main.go": goFile})
	u.open("main.go")
	screen.waitStyle(u, "package", isKeyword, "a keyword")
	u.press("// ") // At the start of the file: package main is a comment now.
	screen.waitStyle(u, "package main", isComment, "a comment")
	u.key(tcell.KeyCtrlZ)
	screen.waitStyle(u, "package", isKeyword, "a keyword again")
}

func TestLiveReloadRecolours(t *testing.T) {
	u, screen := launchStyled(t, map[string]string{"main.go": "package main\n"})
	u.open("main.go")
	screen.waitStyle(u, "package", isKeyword, "a keyword")
	writeDisk(t, u, "main.go", "// package main\nfunc f() {}\n")
	screen.waitStyle(u, "// package", isComment, "a comment")
	screen.waitStyle(u, "func", isKeyword, "a keyword")
}

func TestBigFilesAreNotColoured(t *testing.T) {
	big := strings.Repeat("package main // x\n", maxHighlighted/16+10)
	u, screen := launchStyled(t, map[string]string{"big.go": big})
	u.open("big.go")
	time.Sleep(300 * time.Millisecond)
	if st, ok := screen.styleOf(u, "package"); !ok || !plain(st) {
		t.Fatalf("a file over 1 MiB was coloured: %+v", st)
	}
	u.wait("Go  Ln 1") // The language still shows.
}

func TestHugeFileSaysItHasNoColours(t *testing.T) {
	dir := workDir(t, nil)
	path := filepath.Join(dir, "huge.go")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Repeat("package main // x ", 4) + "\n"
	for range (101 << 20) / len(line) {
		f.WriteString(line) //nolint:errcheck // A short write fails the test below.
	}
	f.Close()
	u := prepared(t, dir, nil)
	u.open("huge.go")
	u.wait("over 100 MB, so it's read-only, and has no syntax colours")
}

// Unit tests of the machinery, on an app with no UI loop. pump plays that loop.

func hlApp(t *testing.T, files map[string]string) *app {
	t.Helper()
	dir := workDir(t, files)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(100, 30)
	t.Cleanup(screen.Fini)
	a := newApp(screen, []project.Project{{Name: "p", Dirs: []string{dir}}}, dir, "")
	t.Cleanup(func() { close(a.done); a.closeAllTabs() })
	return a
}

func (a *app) pump(t *testing.T, until func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		a.syncHighlight()
		if until() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("never happened")
		}
		select {
		case f := <-a.tasks:
			f()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func (a *app) coloured(path string) bool {
	for f, s := range a.hl.files {
		if f.Path == path {
			return s.valid && len(s.spans) > 0
		}
	}
	return false
}

func TestTwoViewsShareOneParse(t *testing.T) {
	a := hlApp(t, map[string]string{"a.go": goFile})
	path := filepath.Join(a.dirPath(), "a.go")
	a.openFile(a.dir(), path)
	a.splitPane(true)
	st := a.dir()
	if len(st.root.panes()) != 2 || st.focus.current() == nil {
		t.Fatal("splitting should show the file in both panes")
	}
	a.pump(t, func() bool { return a.coloured(path) })
	if len(a.hl.files) != 1 || len(a.hl.all) != 1 {
		t.Fatalf("%d states for one file", len(a.hl.files))
	}
	// Closing one view keeps it, and the last view frees it.
	a.removeTab(st, st.focus, st.focus.tabs[0])
	a.syncHighlight()
	if len(a.hl.files) != 1 {
		t.Fatal("closing one of two views dropped the colours")
	}
	p0 := st.root.panes()[0]
	a.removeTab(st, p0, p0.tabs[0])
	a.pump(t, func() bool { return len(a.hl.all) == 0 })
}

func TestClosingATabWhileItParses(t *testing.T) {
	a := hlApp(t, map[string]string{"a.go": strings.Repeat("func f() { println(1) }\n", 30000)})
	path := filepath.Join(a.dirPath(), "a.go")
	for range 5 {
		a.openFile(a.dir(), path)
		a.syncHighlight() // Starts the parse.
		st := a.dir()
		a.removeTab(st, st.focus, st.focus.current())
		a.pump(t, func() bool { return len(a.hl.all) == 0 && len(a.hl.files) == 0 })
	}
}

func TestHiddenTabsFreeTheirTreesAndShownOnesRebuild(t *testing.T) {
	a := hlApp(t, map[string]string{"a.go": goFile, "b.go": goFile})
	dir := a.dirPath()
	a.openFile(a.dir(), filepath.Join(dir, "a.go"))
	a.openFile(a.dir(), filepath.Join(dir, "b.go"))
	st := a.dir()
	st.focus.cur = 0
	a.pump(t, func() bool { return a.coloured(filepath.Join(dir, "a.go")) })
	st.focus.cur = 1 // b.go is showing now, and a.go is hidden.
	a.pump(t, func() bool { return a.coloured(filepath.Join(dir, "b.go")) && !a.coloured(filepath.Join(dir, "a.go")) })
	for f, s := range a.hl.files {
		if f.Path == filepath.Join(dir, "a.go") && (s.spans != nil || s.valid) {
			t.Fatal("a hidden tab kept its spans")
		}
	}
	st.focus.cur = 0
	a.pump(t, func() bool { return a.coloured(filepath.Join(dir, "a.go")) })
}

func TestStaleSpansAndDeletedFilesDontPanic(t *testing.T) {
	a := hlApp(t, map[string]string{"a.go": goFile})
	path := filepath.Join(a.dirPath(), "a.go")
	a.openFile(a.dir(), path)
	a.pump(t, func() bool { return a.coloured(path) })
	for _, s := range a.hl.files {
		s.spans = append(s.spans, syntax.Span{Start: 1 << 30, End: 1<<30 + 5, Class: syntax.Keyword}, syntax.Span{Start: 3, End: 1 << 31, Class: syntax.Function})
	}
	a.draw()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	a.filesChanged(map[string]bool{path: true})
	a.draw()
	it := a.tab()
	it.ed.DocEnd(false)
	it.ed.Type("\n// more\n") //nolint:errcheck // The text is still there.
	a.pump(t, func() bool { return !a.coloured("") && a.hl.files[it.ed.File].seen.Same(it.ed.File.Snapshot) })
}

func TestSelectionsAndMatchesBeatSyntaxColour(t *testing.T) {
	a := hlApp(t, nil)
	for _, class := range []syntax.Class{syntax.None, syntax.Keyword, syntax.String, syntax.Function, syntax.Comment} {
		if a.cellStyle(editor.Cell{Selected: true}, class) != a.th.active ||
			a.cellStyle(editor.Cell{Match: true}, class) != a.th.match ||
			a.cellStyle(editor.Cell{Cursor: true}, class) != a.th.otherCursor ||
			a.cellStyle(editor.Cell{Special: true}, class) != a.th.muted {
			t.Errorf("class %d won over an overlay", class)
		}
	}
	if a.cellStyle(editor.Cell{}, syntax.Keyword) != a.th.keyword || a.cellStyle(editor.Cell{}, syntax.None) != a.th.primary {
		t.Error("plain cells lost their syntax colour")
	}
}

func rssMB(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) > 1 && f[0] == "VmRSS:" {
			kb, _ := strconv.Atoi(f[1])
			return kb / 1024
		}
	}
	return 0
}

// Parsers, trees and queries are C memory the garbage collector can't see, so opening and closing
// a coloured megabyte over and over must not make the process grow.
func TestOpeningAndClosingColouredFilesDoesNotLeak(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc")
	}
	var src strings.Builder
	for src.Len() < maxHighlighted-100 {
		fmt.Fprintf(&src, "func f%d(a int) string { return \"x\" + fmt.Sprint(a, 1.5) } // c\n", src.Len())
	}
	a := hlApp(t, map[string]string{"a.go": src.String()})
	path := filepath.Join(a.dirPath(), "a.go")
	cycle := func() {
		a.openFile(a.dir(), path)
		a.pump(t, func() bool { return a.coloured(path) })
		st := a.dir()
		a.removeTab(st, st.focus, st.focus.current())
		a.pump(t, func() bool { return len(a.hl.all) == 0 })
	}
	settle := func() {
		debug.FreeOSMemory()
		time.Sleep(300 * time.Millisecond) // Releasing the parser's memory runs on its own goroutine.
	}
	for range 2 {
		cycle() // Warm up: allocators settle.
	}
	settle()
	before := rssMB(t)
	for range 15 {
		cycle()
	}
	settle()
	after := rssMB(t)
	t.Logf("RSS %d MB before, %d MB after 15 more cycles", before, after)
	if after > before+40 {
		t.Fatalf("memory grew from %d MB to %d MB", before, after)
	}
}

func BenchmarkFrameOfAHighlightedMiB(b *testing.B) {
	var src strings.Builder
	for src.Len() < maxHighlighted-100 {
		fmt.Fprintf(&src, "func f%d(a int) string { return \"x\" + fmt.Sprint(a, 1.5) } // c\n", src.Len())
	}
	dir := b.TempDir()
	path := filepath.Join(dir, "a.go")
	if err := os.WriteFile(path, []byte(src.String()), 0o644); err != nil {
		b.Fatal(err)
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		b.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(120, 40)
	a := newApp(screen, []project.Project{{Name: "p", Dirs: []string{dir}}}, dir, "")
	defer func() { close(a.done); a.closeAllTabs() }()
	a.openFile(a.dir(), path)
	for !a.coloured(path) {
		a.syncHighlight()
		select {
		case f := <-a.tasks:
			f()
		case <-time.After(time.Millisecond):
		}
	}
	a.tab().ed.GoTo(5000, 0, false)
	b.ResetTimer()
	for range b.N {
		a.draw()
	}
}
