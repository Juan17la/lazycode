// Package syntax parses source code with the official tree-sitter C binding (D-40) and turns the
// tree into coloured spans. It is the only package that knows about the engine.
//
// Parsers, trees and queries are C memory: Close frees them, and a Highlighter must only be used by
// one goroutine at a time (E1).
package syntax

import (
	"bytes"
	"embed"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
	javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// The highlight queries are copied from each grammar's own module, which go:embed can't reach. Each
// grammar's MIT licence sits beside them.
//
//go:embed queries
var queryFiles embed.FS

// Lang is a language LazyCode can highlight.
type Lang struct {
	Name    string // Shown in the status bar.
	key     string
	grammar func() *ts.Language
	queries []string // Files in queries/. TypeScript's extend JavaScript's, so it lists both.
}

var langs = []*Lang{
	{"Go", "go", func() *ts.Language { return ts.NewLanguage(golang.Language()) }, []string{"go"}},
	{"Python", "python", func() *ts.Language { return ts.NewLanguage(python.Language()) }, []string{"python"}},
	{"JavaScript", "javascript", func() *ts.Language { return ts.NewLanguage(javascript.Language()) }, []string{"javascript"}},
	{"JSX", "jsx", func() *ts.Language { return ts.NewLanguage(javascript.Language()) }, []string{"javascript", "javascript-jsx"}},
	{"TypeScript", "typescript", func() *ts.Language { return ts.NewLanguage(typescript.LanguageTypescript()) }, []string{"javascript", "typescript"}},
	{"TSX", "tsx", func() *ts.Language { return ts.NewLanguage(typescript.LanguageTSX()) }, []string{"javascript", "typescript", "javascript-jsx"}},
}

var extensions = map[string]string{
	".go": "go", ".py": "python", ".js": "javascript", ".mjs": "javascript", ".cjs": "javascript",
	".jsx": "jsx", ".ts": "typescript", ".tsx": "tsx",
}

// All returns every language, for tests.
func All() []*Lang { return langs }

func byKey(key string) *Lang {
	for _, l := range langs {
		if l.key == key {
			return l
		}
	}
	return nil
}

// ForPath returns the language of a file by its extension, or nil: other files just aren't coloured.
func ForPath(path string) *Lang {
	return byKey(extensions[strings.ToLower(filepath.Ext(path))])
}

// Check parses src and reports whether the parser found syntax errors.
func Check(language string, src []byte) (hasErrors bool, err error) {
	l := byKey(language)
	if l == nil {
		return false, fmt.Errorf("no grammar for %q", language)
	}
	parser := ts.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(l.grammar()); err != nil {
		return false, err
	}
	tree := parser.Parse(src, nil)
	if tree == nil {
		return false, fmt.Errorf("parsing %s returned no tree", language)
	}
	defer tree.Close()
	return tree.RootNode().HasError(), nil
}

// Class is how a span is coloured. There are few on purpose (Q-48): the theme has two colours.
type Class uint8

const (
	None    Class = iota
	Keyword       // Accent.
	Literal       // Strings, numbers and constants: accent, without bold.
	Name          // Functions and types: primary, bold.
	Comment       // Muted, italic.
)

// classOf maps a capture name from the queries, such as function.method, to a class.
func classOf(capture string) Class {
	base, _, _ := strings.Cut(capture, ".")
	switch base {
	case "keyword":
		return Keyword
	case "string", "number", "constant", "escape", "boolean":
		return Literal
	case "function", "type", "constructor":
		return Name
	case "comment":
		return Comment
	}
	return None
}

// Span colours the bytes Start up to End. Spans are sorted and never overlap.
type Span struct {
	Start, End uint32
	Class      Class
}

// Highlighter holds the parse of one file, so that a change reparses only what it touched.
type Highlighter struct {
	parser  *ts.Parser
	tree    *ts.Tree
	query   *ts.Query
	cursor  *ts.QueryCursor
	classes []Class // Of each capture in the query.
	src     []byte  // What tree describes, for working out the next edit.
}

// NewHighlighter makes one for a language. It parses nothing yet.
func NewHighlighter(l *Lang) (*Highlighter, error) {
	var source strings.Builder
	for _, name := range l.queries {
		data, err := queryFiles.ReadFile("queries/" + name + ".scm")
		if err != nil {
			return nil, err
		}
		source.Write(data)
		source.WriteByte('\n')
	}
	grammar := l.grammar()
	h := &Highlighter{parser: ts.NewParser()}
	if err := h.parser.SetLanguage(grammar); err != nil {
		h.Close()
		return nil, err
	}
	query, qerr := ts.NewQuery(grammar, source.String())
	if qerr != nil {
		h.Close()
		return nil, fmt.Errorf("%s highlights: %w", l.Name, qerr)
	}
	h.query, h.cursor = query, ts.NewQueryCursor()
	for _, name := range query.CaptureNames() {
		h.classes = append(h.classes, classOf(name))
	}
	return h, nil
}

// Close frees the C memory. Calling it again does nothing.
func (h *Highlighter) Close() {
	if h == nil {
		return
	}
	h.Drop()
	if h.cursor != nil {
		h.cursor.Close()
		h.cursor = nil
	}
	if h.query != nil {
		h.query.Close()
		h.query = nil
	}
	if h.parser != nil {
		h.parser.Close()
		h.parser = nil
	}
}

// Drop frees the tree, as for a tab that's hidden. The next Update parses everything again.
func (h *Highlighter) Drop() {
	if h.tree != nil {
		h.tree.Close()
		h.tree = nil
	}
	h.src = nil
}

// Update parses src, reusing the last tree where the text still matches, and returns the spans.
func (h *Highlighter) Update(src []byte) ([]Span, error) {
	if h.parser == nil {
		return nil, fmt.Errorf("highlighter is closed")
	}
	if h.tree != nil {
		h.tree.Edit(diff(h.src, src))
	}
	tree := h.parser.Parse(src, h.tree)
	if tree == nil {
		return nil, fmt.Errorf("parsing returned no tree")
	}
	if h.tree != nil {
		h.tree.Close()
	}
	h.tree = tree
	h.src = append(h.src[:0], src...) // The caller may reuse src.
	return h.spans(src), nil
}

// diff describes the change from old to new as one edit: whatever lies between the text they
// share at the start and at the end. That takes well under a millisecond for a megabyte, and it's
// how a tree is kept without following every keystroke.
func diff(old, new []byte) *ts.InputEdit {
	n := min(len(old), len(new))
	start := 0
	for start < n && old[start] == new[start] {
		start++
	}
	tail := 0
	for tail < n-start && old[len(old)-1-tail] == new[len(new)-1-tail] {
		tail++
	}
	from := pointAt(old, 0, ts.Point{}, start)
	return &ts.InputEdit{
		StartByte:      uint(start),
		OldEndByte:     uint(len(old) - tail),
		NewEndByte:     uint(len(new) - tail),
		StartPosition:  from,
		OldEndPosition: pointAt(old, start, from, len(old)-tail),
		NewEndPosition: pointAt(new, start, from, len(new)-tail),
	}
}

// pointAt returns the row and column (in bytes) of offset, counting on from an earlier offset.
func pointAt(text []byte, from int, at ts.Point, offset int) ts.Point {
	region := text[from:offset]
	if rows := bytes.Count(region, []byte{'\n'}); rows > 0 {
		return ts.Point{Row: at.Row + uint(rows), Column: uint(len(region) - bytes.LastIndexByte(region, '\n') - 1)}
	}
	return ts.Point{Row: at.Row, Column: at.Column + uint(len(region))}
}

type capture struct {
	start, end uint32
	pattern    uint32
	class      Class
}

// spans runs the query over the whole tree. A narrower capture beats a wider one over it, and of
// two on the same bytes the later pattern wins, so a Python constant beats a constructor.
func (h *Highlighter) spans(src []byte) []Span {
	var found []capture
	matches := h.cursor.Matches(h.query, h.tree.RootNode(), src)
	for m := matches.Next(); m != nil; m = matches.Next() {
		for _, c := range m.Captures {
			if h.classes[c.Index] == None {
				continue // Plain variables and the like. Queries list them first or last, so they'd only fight with real colours.
			}
			found = append(found, capture{uint32(c.Node.StartByte()), uint32(c.Node.EndByte()), uint32(m.PatternIndex), h.classes[c.Index]})
		}
	}
	slices.SortFunc(found, func(a, b capture) int {
		if wa, wb := a.end-a.start, b.end-b.start; wa != wb {
			return int(wb) - int(wa)
		}
		return int(a.pattern) - int(b.pattern)
	})
	// ponytail: painting a byte array costs the size of the file per parse, a megabyte at most
	// here. An interval structure would be the upgrade for much larger files.
	paint := make([]Class, len(src))
	for _, c := range found {
		if c.end > uint32(len(paint)) {
			continue
		}
		for i := c.start; i < c.end; i++ {
			paint[i] = c.class
		}
	}
	var spans []Span
	for i := 0; i < len(paint); {
		c, j := paint[i], i+1
		for j < len(paint) && paint[j] == c {
			j++
		}
		if c != None {
			spans = append(spans, Span{uint32(i), uint32(j), c})
		}
		i = j
	}
	return spans
}

// Cursor finds the class at a run of increasing offsets, as a row is drawn.
type Cursor struct {
	spans []Span
	i     int
}

// NewCursor reads spans, which may be stale: offsets past the text just find nothing.
func NewCursor(spans []Span) *Cursor { return &Cursor{spans: spans, i: -1} }

// At returns the class of the byte at offset. Offsets usually grow, and going back is still correct.
func (c *Cursor) At(offset int) Class {
	if len(c.spans) == 0 {
		return None
	}
	if c.i < 0 || (c.i > 0 && int(c.spans[c.i-1].End) > offset) {
		c.i, _ = slices.BinarySearchFunc(c.spans, offset, func(s Span, at int) int {
			if int(s.End) <= at {
				return -1
			}
			return 1
		})
	}
	for c.i < len(c.spans) && int(c.spans[c.i].End) <= offset {
		c.i++
	}
	if c.i < len(c.spans) && int(c.spans[c.i].Start) <= offset {
		return c.spans[c.i].Class
	}
	return None
}
