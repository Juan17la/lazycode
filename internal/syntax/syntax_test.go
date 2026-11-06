package syntax

import (
	"math/rand"
	"slices"
	"strings"
	"testing"
)

func TestDefaultLanguagesParse(t *testing.T) {
	for _, tc := range []struct{ language, good, bad string }{
		{"go", "package a\nfunc b() {}\n", "package a\nfunc {{{\n"},
		{"javascript", "function a() { return 1; }\n", "function ((( {\n"},
		{"python", "def a():\n    return 1\n", "def (((:\n"},
		{"typescript", "function a(b: number): number { return b; }\n", "function a(b: : { \n"},
	} {
		if hasErrors, err := Check(tc.language, []byte(tc.good)); err != nil || hasErrors {
			t.Errorf("%s valid code: hasErrors=%v err=%v", tc.language, hasErrors, err)
		}
		if hasErrors, err := Check(tc.language, []byte(tc.bad)); err != nil || !hasErrors {
			t.Errorf("%s broken code: hasErrors=%v err=%v", tc.language, hasErrors, err)
		}
	}
}

func TestUnknownLanguageIsAnError(t *testing.T) {
	if _, err := Check("cobol", nil); err == nil {
		t.Error("want an error")
	}
}

func TestEveryQueryCompiles(t *testing.T) {
	for _, l := range All() {
		h, err := NewHighlighter(l)
		if err != nil {
			t.Errorf("%s: %v", l.Name, err)
			continue
		}
		h.Close()
		h.Close() // Twice is fine.
	}
}

func TestForPath(t *testing.T) {
	for path, want := range map[string]string{
		"a.go": "Go", "a.PY": "Python", "x/a.js": "JavaScript", "a.mjs": "JavaScript", "a.cjs": "JavaScript",
		"a.jsx": "JSX", "a.ts": "TypeScript", "a.tsx": "TSX", "a.txt": "", "Makefile": "", "go": "", ".go": "Go",
	} {
		got := ""
		if l := ForPath(path); l != nil {
			got = l.Name
		}
		if got != want {
			t.Errorf("%s: %q, want %q", path, got, want)
		}
	}
}

// classes returns the class of each word in src, for the spans of one language.
func classesOf(t *testing.T, path, src string) map[string]Class {
	t.Helper()
	h, err := NewHighlighter(ForPath(path))
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	spans, err := h.Update([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Class{}
	for _, s := range spans {
		out[src[s.Start:s.End]] = s.Class
	}
	return out
}

func TestSpansPerLanguage(t *testing.T) {
	for _, tc := range []struct {
		path, src string
		want      map[string]Class
	}{
		{"a.go", "package a\n// hi\nfunc Add(x int) string { return \"s\" + 1 }\n", map[string]Class{"package": Keyword, "// hi": Comment, "func": Keyword, "Add": Name, "int": Name, "\"s\"": Literal, "1": Literal, "return": Keyword}},
		{"a.py", "def f(x):\n    # c\n    return 'a'\n", map[string]Class{"def": Keyword, "f": Name, "# c": Comment, "return": Keyword, "'a'": Literal}},
		{"a.js", "function f() { return 42 } // c\n", map[string]Class{"function": Keyword, "f": Name, "42": Literal, "// c": Comment}},
		{"a.ts", "interface A { b: string }\nconst x: number = 1\n", map[string]Class{"interface": Keyword, "const": Keyword, "1": Literal}},
		{"a.tsx", "const a = <div className=\"x\">hi</div>\n", map[string]Class{"const": Keyword, "\"x\"": Literal}},
		{"a.jsx", "const a = <b id=\"x\">hi</b>\n", map[string]Class{"const": Keyword, "\"x\"": Literal}},
	} {
		got := classesOf(t, tc.path, tc.src)
		for word, class := range tc.want {
			if got[word] != class {
				t.Errorf("%s: %q is class %d, want %d (all: %v)", tc.path, word, got[word], class, got)
			}
		}
	}
}

func TestNarrowerAndLaterCapturesWin(t *testing.T) {
	// "len" is a call (function) and a builtin, which is a later pattern.
	got := classesOf(t, "a.go", "package a\nvar n = len(\"x\")\n")
	if got["len"] != Name {
		t.Errorf("len is %d", got["len"])
	}
}

func TestEmptyInvalidAndBrokenText(t *testing.T) {
	for _, l := range All() {
		h, err := NewHighlighter(l)
		if err != nil {
			t.Fatal(err)
		}
		for _, src := range []string{"", "\xff\xfe\x00 é\xc3", "func (((( {{ \"unterminated", strings.Repeat("(", 5000), "\r\n\r\n"} {
			if _, err := h.Update([]byte(src)); err != nil {
				t.Errorf("%s %q: %v", l.Name, src, err)
			}
		}
		h.Drop()
		if _, err := h.Update([]byte("x")); err != nil { // After Drop, it starts over.
			t.Error(err)
		}
		h.Close()
		if _, err := h.Update(nil); err == nil {
			t.Error("a closed highlighter parsed")
		}
	}
}

// code is valid Go, so that each edit in the test starts from sensible text.
const code = `package main

import "fmt"

// Greeting is shown first. héllo 你好 🙂
type Greeting struct {
	Text string
	N    int
}

func (g Greeting) Say(n int) string {
	if n > 3 {
		return "many 🙂 " + g.Text
	}
	return fmt.Sprintf("%s %d", g.Text, n) // done
}

func main() {
	g := Greeting{Text: "hi 你好", N: 2}
	for i := 0; i < 3; i++ {
		fmt.Println(g.Say(i))
	}
}
`

// Reparsing an edited tree has to colour the text as a fresh parse does, however the text changed.
func TestIncrementalMatchesFreshParse(t *testing.T) {
	inserts := []string{"x", "\n", "\t", "é", "你", "🙂", "// c\n", "\"s\"", "func f() {}\n", "{", "}", "(", ")", "/*", "*/", "`", "\r\n", ""}
	rng := rand.New(rand.NewSource(1))
	inc, _ := NewHighlighter(ForPath("a.go"))
	defer inc.Close()
	text := []byte(code)
	if _, err := inc.Update(text); err != nil {
		t.Fatal(err)
	}
	mismatched := 0
	for i := range 400 {
		a := rng.Intn(len(text) + 1)
		b := min(len(text), a+rng.Intn(6))
		next := slices.Concat(text[:a], []byte(inserts[rng.Intn(len(inserts))]), text[b:])
		if i%50 == 49 {
			next = []byte(code) // Back to valid text now and then, so that edits aren't all on broken code.
		}
		got, err := inc.Update(next)
		if err != nil {
			t.Fatal(err)
		}
		fresh, _ := NewHighlighter(ForPath("a.go"))
		want, _ := fresh.Update(next)
		fresh.Close()
		if !slices.Equal(got, want) {
			mismatched++
			if !hasError(t, next) { // On valid code the two must agree.
				t.Fatalf("edit %d changed %q to %q: incremental and fresh spans differ", i, text, next)
			}
		}
		text = next
	}
	t.Logf("%d of 400 edits on broken code coloured differently from a fresh parse", mismatched)
}

func hasError(t *testing.T, src []byte) bool {
	t.Helper()
	bad, err := Check("go", src)
	if err != nil {
		t.Fatal(err)
	}
	return bad
}

// Edits that keep the code valid must agree exactly, including after multi-byte text.
func TestIncrementalOnValidEdits(t *testing.T) {
	steps := []struct{ old, new string }{
		{"Greeting is shown", "Greeting 你好🙂 is shown"},
		{"many 🙂", "many"},
		{"n > 3", "n > 30"},
		{"// done", "// done é\n\t// more"},
		{"Say(n int)", "Say(n, m int)"},
		{"\"hi 你好\"", "\"\""},
	}
	inc, _ := NewHighlighter(ForPath("a.go"))
	defer inc.Close()
	text := code
	inc.Update([]byte(text)) //nolint:errcheck // Checked below.
	for _, s := range steps {
		if !strings.Contains(text, s.old) {
			t.Fatalf("%q isn't in the text", s.old)
		}
		text = strings.Replace(text, s.old, s.new, 1)
		got, err := inc.Update([]byte(text))
		if err != nil {
			t.Fatal(err)
		}
		fresh, _ := NewHighlighter(ForPath("a.go"))
		want, _ := fresh.Update([]byte(text))
		fresh.Close()
		if !slices.Equal(got, want) {
			t.Fatalf("after %q -> %q the spans differ", s.old, s.new)
		}
	}
}

func TestDiffPoints(t *testing.T) {
	e := diff([]byte("ab\ncd\nef"), []byte("ab\ncXd\nef"))
	if e.StartByte != 4 || e.OldEndByte != 4 || e.NewEndByte != 5 || e.StartPosition.Row != 1 || e.StartPosition.Column != 1 || e.NewEndPosition.Column != 2 {
		t.Errorf("%+v", e)
	}
	e = diff([]byte("a\nb"), []byte("a"))
	if e.StartByte != 1 || e.OldEndByte != 3 || e.NewEndByte != 1 || e.OldEndPosition.Row != 1 || e.OldEndPosition.Column != 1 {
		t.Errorf("%+v", e)
	}
	diff(nil, nil)
}

func TestCursor(t *testing.T) {
	spans := []Span{{2, 4, Keyword}, {6, 7, Comment}, {10, 12, Name}}
	c := NewCursor(spans)
	for off, want := range map[int]Class{0: None, 2: Keyword, 3: Keyword, 4: None, 6: Comment, 8: None, 11: Name, 50: None} {
		if got := c.At(off); got != want {
			t.Errorf("At(%d) = %d, want %d", off, got, want)
		}
	}
	if NewCursor(nil).At(3) != None {
		t.Error("no spans")
	}
}

func BenchmarkUpdateOneMiB(b *testing.B) {
	src := []byte(strings.Repeat(code[strings.Index(code, "type"):], 1<<20/len(code)))
	h, _ := NewHighlighter(ForPath("a.go"))
	defer h.Close()
	b.Run("first parse", func(b *testing.B) {
		for range b.N {
			h.Drop()
			h.Update(src) //nolint:errcheck // Benchmark.
		}
	})
	b.Run("one keystroke", func(b *testing.B) {
		h.Update(src) //nolint:errcheck // Benchmark.
		edited := slices.Concat(src[:len(src)/2], []byte("x"), src[len(src)/2:])
		for i := range b.N {
			if i%2 == 0 {
				h.Update(edited) //nolint:errcheck // Benchmark.
			} else {
				h.Update(src) //nolint:errcheck // Benchmark.
			}
		}
	})
}
