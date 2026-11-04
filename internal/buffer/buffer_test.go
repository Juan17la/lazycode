package buffer

import (
	"bytes"
	"math/rand/v2"
	"strings"
	"testing"
)

func text(s Snapshot) string { return string(s.Slice(0, s.Len())) }

func edit(t *testing.T, b *Buffer, edits ...Edit) {
	t.Helper()
	if err := b.Apply(edits...); err != nil {
		t.Fatal(err)
	}
}

// checkLines compares every line lookup with a plain scan of want.
func checkLines(t *testing.T, s Snapshot, want string) {
	t.Helper()
	starts := []int{0}
	for i := range len(want) {
		if want[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	if got := s.LineCount(); got != len(starts) {
		t.Fatalf("LineCount = %d, want %d, for %q", got, len(starts), want)
	}
	for line, start := range starts {
		if got := s.LineStart(line); got != start {
			t.Fatalf("LineStart(%d) = %d, want %d, for %q", line, got, start, want)
		}
		end := len(want)
		if line+1 < len(starts) {
			end = starts[line+1]
		}
		content := strings.TrimSuffix(strings.TrimSuffix(want[start:end], "\n"), "\r")
		if got := string(s.Line(line)); got != content {
			t.Fatalf("Line(%d) = %q, want %q", line, got, content)
		}
		if got := s.LineOf(start); got != line {
			t.Fatalf("LineOf(%d) = %d, want %d, for %q", start, got, line, want)
		}
		if end > start {
			if got := s.LineOf(end - 1); got != line {
				t.Fatalf("LineOf(%d) = %d, want %d, for %q", end-1, got, line, want)
			}
		}
	}
}

// checkTree verifies what the lookups rely on, and returns the height.
func checkTree(t *testing.T, s Snapshot, n *node) int {
	t.Helper()
	if n == nil {
		return 0
	}
	l, r := checkTree(t, s, n.left), checkTree(t, s, n.right)
	if n.length <= 0 || n.length > maxPiece {
		t.Fatalf("piece length %d", n.length)
	}
	if got := bytes.Count(s.text(n), []byte{'\n'}); got != n.breaks {
		t.Fatalf("piece has %d line feeds, recorded %d", got, n.breaks)
	}
	if n.totalLen != n.left.size()+n.length+n.right.size() {
		t.Fatalf("wrong totalLen %d", n.totalLen)
	}
	if n.totalBreaks != n.left.lineBreaks()+n.breaks+n.right.lineBreaks() {
		t.Fatalf("wrong totalBreaks %d", n.totalBreaks)
	}
	for _, child := range []*node{n.left, n.right} {
		if child != nil && child.priority > n.priority {
			t.Fatal("heap order broken")
		}
	}
	return 1 + max(l, r)
}

// runModel treats data as a script of operations, applying each to a Buffer and to
// plain strings, which must always agree. Both tests below feed it.
func runModel(t *testing.T, data []byte) {
	next := func(limit int) int {
		if len(data) == 0 || limit <= 0 {
			return 0
		}
		v := int(data[0])
		data = data[1:]
		if len(data) > 0 {
			v = v<<8 | int(data[0])
			data = data[1:]
		}
		return v % limit
	}
	pieces := []string{"", "a", "é", "\n", "\r\n", "\x00", "\xff", "line\n", "tab\there", "世界\n", "\r", "😀"}
	randomText := func() string {
		var sb strings.Builder
		for range next(5) {
			sb.WriteString(pieces[next(len(pieces))])
		}
		return sb.String()
	}

	original := randomText()
	b := New([]byte(original))
	history := []string{original} // Each undo step's text.
	at := 0                       // Where in history the buffer is.
	joinable := false

	check := func() {
		t.Helper()
		if got := text(b.Snapshot); got != history[at] {
			t.Fatalf("text = %q, want %q", got, history[at])
		}
		var written bytes.Buffer
		if _, err := b.WriteTo(&written); err != nil || written.String() != history[at] {
			t.Fatalf("WriteTo = %q, %v, want %q", written.String(), err, history[at])
		}
		checkLines(t, b.Snapshot, history[at])
		checkTree(t, b.Snapshot, b.root)
	}

	for len(data) > 0 {
		current := history[at]
		switch op := next(6); op {
		case 0, 1, 2: // One or more edits, as a new step or a joined one.
			var edits []Edit
			var result strings.Builder
			pos := 0
			for range 1 + next(3) {
				if pos > len(current) {
					break
				}
				start := pos + next(len(current)-pos+1)
				end := start + next(min(len(current)-start, 8)+1)
				insert := randomText()
				edits = append(edits, Edit{start, end, insert})
				result.WriteString(current[pos:start])
				result.WriteString(insert)
				pos = end
			}
			result.WriteString(current[pos:])
			changed := false
			for _, e := range edits {
				changed = changed || e.Start != e.End || e.Text != ""
			}

			var err error
			if op == 2 {
				err = b.Continue(edits...)
			} else {
				err = b.Apply(edits...)
			}
			if err != nil {
				t.Fatal(err)
			}
			if changed {
				if op == 2 && joinable {
					history = append(history[:at], result.String())
				} else {
					history = append(history[:at+1], result.String())
					at++
				}
				joinable = true
			}
		case 3:
			if got, want := b.Undo(), at > 0; got != want {
				t.Fatalf("Undo = %v, want %v", got, want)
			}
			if at > 0 {
				at--
				joinable = false
			}
		case 4:
			if got, want := b.Redo(), at < len(history)-1; got != want {
				t.Fatalf("Redo = %v, want %v", got, want)
			}
			if at < len(history)-1 {
				at++
				joinable = false
			}
		case 5:
			start, end := next(len(current)+2)-1, next(len(current)+2)-1
			want := ""
			if from, to := max(start, 0), min(end, len(current)); from < to {
				want = current[from:to]
			}
			if got := string(b.Slice(start, end)); got != want {
				t.Fatalf("Slice(%d, %d) = %q, want %q", start, end, got, want)
			}
		}
		check()
	}

	// Undoing everything returns the original text, and redoing returns the rest.
	for at > 0 {
		if !b.Undo() {
			t.Fatal("Undo ran out early")
		}
		at--
		check()
	}
	if b.Undo() {
		t.Fatal("Undo past the first step")
	}
	if text(b.Snapshot) != original {
		t.Fatalf("after undoing everything: %q, want %q", text(b.Snapshot), original)
	}
	for at < len(history)-1 {
		if !b.Redo() {
			t.Fatal("Redo ran out early")
		}
		at++
		check()
	}
	if b.Redo() {
		t.Fatal("Redo past the last step")
	}
}

func TestMatchesPlainStrings(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for range 300 {
		data := make([]byte, 2+rng.IntN(800))
		for i := range data {
			data[i] = byte(rng.Uint32())
		}
		runModel(t, data)
	}
}

func FuzzEdits(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("some edits, then undo and redo"))
	f.Add(bytes.Repeat([]byte{0, 3, 1, 7, 0, 4, 9, 2}, 40))
	f.Fuzz(runModel)
}

func TestLargeTextIsChunkedAndStaysBalanced(t *testing.T) {
	line := "some line of source code, about this long\r\n"
	original := strings.Repeat(line, 20_000)
	b := New([]byte(original))
	want := original
	paste := strings.Repeat("pasted\n", 10_000) // Bigger than one piece.
	edit(t, b, Edit{1000, 1000, paste})
	want = want[:1000] + paste + want[1000:]

	rng := rand.New(rand.NewPCG(3, 4))
	for range 3000 {
		at := rng.IntN(len(want))
		end := min(at+rng.IntN(20), len(want))
		edit(t, b, Edit{at, end, "x\n"})
		want = want[:at] + "x\n" + want[end:]
	}
	if text(b.Snapshot) != want {
		t.Fatal("text differs after many edits")
	}
	checkLines(t, b.Snapshot, want)
	if height := checkTree(t, b.Snapshot, b.root); height > 60 {
		t.Fatalf("tree height %d, so it isn't balanced", height)
	}
	for b.Undo() {
	}
	if text(b.Snapshot) != original {
		t.Fatal("undoing everything didn't restore the original")
	}
}

func TestBytesStayUnchangedUnlessEdited(t *testing.T) {
	for name, original := range map[string]string{
		"LF":          "one\ntwo\nthree\n",
		"CRLF":        "one\r\ntwo\r\nthree\r\n",
		"mixed":       "one\ntwo\r\nthree\rfour\n\r\n",
		"no newline":  "one\ntwo",
		"invalid":     "a\xff\xfe\x00b\xc3\n",
		"empty":       "",
		"only breaks": "\n\r\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			input := []byte(original)
			b := New(input)
			edit(t, b, Edit{0, 0, "new\n"})
			if got := text(b.Snapshot); got != "new\n"+original {
				t.Fatalf("got %q", got)
			}
			edit(t, b, Edit{0, 4, ""})
			if got := text(b.Snapshot); got != original {
				t.Fatalf("got %q, want %q", got, original)
			}
			if string(input) != original {
				t.Fatal("the original bytes were written to")
			}
		})
	}
}

func TestLines(t *testing.T) {
	s := New([]byte("one\r\n\ntwo\rstill two\nlast")).Snapshot
	for line, want := range []string{"one", "", "two\rstill two", "last"} {
		if got := string(s.Line(line)); got != want {
			t.Errorf("Line(%d) = %q, want %q", line, got, want)
		}
	}
	if got := New([]byte("a\n")).LineCount(); got != 2 {
		t.Errorf("a trailing line feed gives %d lines, want 2", got)
	}
	if got := New(nil).LineCount(); got != 1 {
		t.Errorf("empty text has %d lines, want 1", got)
	}
}

func TestReadsClampBadPositions(t *testing.T) {
	s := New([]byte("ab\ncd")).Snapshot
	if got := s.LineStart(-1); got != 0 {
		t.Errorf("LineStart(-1) = %d", got)
	}
	if got := s.LineStart(99); got != 3 {
		t.Errorf("LineStart(99) = %d", got)
	}
	if got := s.LineOf(-5); got != 0 {
		t.Errorf("LineOf(-5) = %d", got)
	}
	if got := s.LineOf(99); got != 1 {
		t.Errorf("LineOf(99) = %d", got)
	}
	if got := string(s.Line(-1)); got != "ab" {
		t.Errorf("Line(-1) = %q", got)
	}
	if got := string(s.Line(99)); got != "cd" {
		t.Errorf("Line(99) = %q", got)
	}
	if got := string(s.Slice(-3, 99)); got != "ab\ncd" {
		t.Errorf("Slice(-3, 99) = %q", got)
	}
	if got := s.Slice(4, 2); got == nil || len(got) != 0 {
		t.Errorf("Slice(4, 2) = %v", got)
	}
}

func TestBadEditsChangeNothing(t *testing.T) {
	for name, edits := range map[string][]Edit{
		"negative start":  {{-1, 2, "x"}},
		"past the end":    {{0, 99, "x"}},
		"end before":      {{3, 2, "x"}},
		"overlapping":     {{0, 3, "x"}, {2, 4, "y"}},
		"out of order":    {{3, 4, "x"}, {0, 1, "y"}},
		"second is wrong": {{0, 1, "x"}, {2, 99, "y"}},
	} {
		t.Run(name, func(t *testing.T) {
			b := New([]byte("hello"))
			if err := b.Apply(edits...); err == nil {
				t.Fatal("no error")
			}
			if got := text(b.Snapshot); got != "hello" {
				t.Fatalf("text = %q", got)
			}
			if b.Undo() {
				t.Fatal("a rejected edit made an undo step")
			}
		})
	}
}

func TestEmptyEditMakesNoUndoStep(t *testing.T) {
	b := New([]byte("hello"))
	edit(t, b)
	edit(t, b, Edit{2, 2, ""})
	if b.Undo() {
		t.Fatal("an empty edit made an undo step")
	}
	edit(t, b, Edit{0, 0, "x"})
	edit(t, b, Edit{1, 1, ""})
	if !b.Undo() || text(b.Snapshot) != "hello" {
		t.Fatalf("text = %q", text(b.Snapshot))
	}
	if !b.Redo() {
		t.Fatal("an empty edit cleared redo")
	}
}

func TestMultiCursorEditIsOneUndoStep(t *testing.T) {
	b := New([]byte("foo bar foo baz foo"))
	edit(t, b, Edit{0, 3, "qux!"}, Edit{8, 11, "qux!"}, Edit{16, 19, "qux!"})
	if got, want := text(b.Snapshot), "qux! bar qux! baz qux!"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if !b.Undo() || text(b.Snapshot) != "foo bar foo baz foo" {
		t.Fatalf("after one undo: %q", text(b.Snapshot))
	}
	if b.Undo() {
		t.Fatal("it took more than one step")
	}
}

func TestInsertsAtTheSameOffsetKeepTheirOrder(t *testing.T) {
	b := New([]byte("ab"))
	edit(t, b, Edit{1, 1, "1"}, Edit{1, 1, "2"})
	if got := text(b.Snapshot); got != "a12b" {
		t.Fatalf("got %q", got)
	}
}

func TestContinueJoinsTheLastStep(t *testing.T) {
	typeAt := func(b *Buffer, at int, s string) {
		if err := b.Continue(Edit{at, at, s}); err != nil {
			t.Fatal(err)
		}
	}

	b := New(nil)
	typeAt(b, 0, "h") // With nothing to join, it starts a step.
	typeAt(b, 1, "i")
	edit(t, b, Edit{2, 2, " there"})
	typeAt(b, 8, "!")
	if !b.Undo() || text(b.Snapshot) != "hi" {
		t.Fatalf("first undo: %q", text(b.Snapshot))
	}
	if !b.Undo() || text(b.Snapshot) != "" {
		t.Fatalf("second undo: %q", text(b.Snapshot))
	}

	// After an undo, typing must not swallow the step before it.
	if !b.Redo() || text(b.Snapshot) != "hi" {
		t.Fatalf("redo: %q", text(b.Snapshot))
	}
	if !b.Redo() || !b.Undo() {
		t.Fatal("redo and undo")
	}
	typeAt(b, 2, "?")
	if b.Redo() {
		t.Fatal("an edit didn't clear redo")
	}
	if !b.Undo() || text(b.Snapshot) != "hi" {
		t.Fatalf("Continue joined across an undo: %q", text(b.Snapshot))
	}
}

func TestMarksFollowUndoAndRedo(t *testing.T) {
	b := New(nil)
	b.Mark = "before first"
	edit(t, b, Edit{0, 0, "a"})
	b.Mark = "after first"
	edit(t, b, Edit{1, 1, "b"})
	b.Mark = "after second"

	b.Undo()
	if b.Mark != "after first" {
		t.Fatalf("after undo: %v", b.Mark)
	}
	b.Undo()
	if b.Mark != "before first" {
		t.Fatalf("after second undo: %v", b.Mark)
	}
	b.Redo()
	b.Redo()
	if b.Mark != "after second" {
		t.Fatalf("after redo: %v", b.Mark)
	}
}

func TestTypingDoesNotMakeOnePiecePerKey(t *testing.T) {
	b := New(nil)
	for i := range 1000 {
		if err := b.Continue(Edit{i, i, "x"}); err != nil {
			t.Fatal(err)
		}
	}
	if b.root.left != nil || b.root.right != nil {
		t.Fatal("typing in one place made more than one piece")
	}
}

// A snapshot must stay readable and unchanged on another goroutine during edits.
// Run with -race, this also proves reading needs no lock.
func TestSnapshotIgnoresLaterEdits(t *testing.T) {
	original := strings.Repeat("line of text\n", 5000)
	b := New([]byte(original))
	snap := b.Snapshot
	done := make(chan string)
	go func() {
		got := ""
		for range 20 {
			got = text(snap)
			snap.LineStart(snap.LineCount() / 2)
		}
		done <- got
	}()
	for i := range 2000 {
		edit(t, b, Edit{i, i + 1, "changed\n"})
	}
	if got := <-done; got != original {
		t.Fatal("the snapshot changed")
	}
}

const hundredMB = 100 * 1024 * 1024

func largeFile() []byte {
	return bytes.Repeat([]byte("the quick brown fox jumps over the lazy dog, line\n"), hundredMB/50)
}

// bytes/op is the extra memory for a 100 MB file (budget: under 100 MB).
func BenchmarkOpen100MB(b *testing.B) {
	data := largeFile()
	b.ResetTimer()
	for b.Loop() {
		New(data)
	}
}

// bytes/op is an upper limit on what one undo step keeps alive, in a 100 MB file
// with 20,000 scattered edits behind it.
func BenchmarkScatteredEdit100MB(b *testing.B) {
	buf := New(largeFile())
	rng := rand.New(rand.NewPCG(5, 6))
	for range 20_000 {
		at := rng.IntN(buf.Len())
		buf.Apply(Edit{at, at, "x"}) //nolint:errcheck // The offset is in range.
	}
	b.ResetTimer()
	for b.Loop() {
		at := rng.IntN(buf.Len())
		if err := buf.Apply(Edit{at, at, "x"}); err != nil {
			b.Fatal(err)
		}
	}
}

// One screen's worth of lines, which the UI needs every frame.
func BenchmarkDraw60Lines100MB(b *testing.B) {
	buf := New(largeFile())
	rng := rand.New(rand.NewPCG(7, 8))
	for range 20_000 {
		at := rng.IntN(buf.Len())
		buf.Apply(Edit{at, at, "x"}) //nolint:errcheck // The offset is in range.
	}
	b.ResetTimer()
	for b.Loop() {
		first := rng.IntN(buf.LineCount() - 60)
		for line := first; line < first+60; line++ {
			buf.Line(line)
		}
	}
}

// Replace-all across a large file, as one undo step.
func BenchmarkReplace100kPlaces(b *testing.B) {
	data := largeFile()
	edits := make([]Edit, 100_000)
	for i := range edits {
		start := i * 500
		edits[i] = Edit{start, start + 5, "slow"}
	}
	b.ResetTimer()
	for b.Loop() {
		buf := New(data)
		if err := buf.Apply(edits...); err != nil {
			b.Fatal(err)
		}
	}
}
