// Package buffer stores an open file's text as a piece table (P-05) and keeps its
// undo history. It imports nothing else from LazyCode, and it works in bytes: UTF-8
// and line endings are never interpreted or changed, so an edit touches only the
// bytes it names.
//
// The original file is never changed, new text goes in an append-only slice, and
// pieces pointing into the two describe the document. Pieces live in a balanced
// tree that is never modified in place: every edit makes a new root that shares
// most of its nodes with the old one. So an undo step is just an old root, and a
// Snapshot can be read by another goroutine while the UI goroutine keeps editing.
package buffer

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
)

// maxPiece bounds the bytes scanned to find a line inside a piece. It also replaces
// a separate line index: a 100 MB file needs about 6,400 nodes, under 1 MB, where
// 8 bytes per line would take about 20 MB.
const maxPiece = 16 * 1024

// node is one piece, and the root of a treap of pieces ordered by document position.
type node struct {
	left, right *node
	priority    uint32
	inAdded     bool // The piece points into the added text, not the original.
	start       int
	length      int
	breaks      int // Line feeds in this piece.
	totalLen    int // length, including both subtrees.
	totalBreaks int // breaks, including both subtrees.
}

func (n *node) size() int {
	if n == nil {
		return 0
	}
	return n.totalLen
}

func (n *node) lineBreaks() int {
	if n == nil {
		return 0
	}
	return n.totalBreaks
}

// with returns a copy of n with new children.
func (n *node) with(left, right *node) *node {
	c := *n
	c.left, c.right = left, right
	c.totalLen = left.size() + c.length + right.size()
	c.totalBreaks = left.lineBreaks() + c.breaks + right.lineBreaks()
	return &c
}

func newPiece(inAdded bool, start, length, breaks int) *node {
	n := node{inAdded: inAdded, start: start, length: length, breaks: breaks, priority: rand.Uint32()}
	return n.with(nil, nil)
}

func merge(a, b *node) *node {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case a.priority > b.priority:
		return a.with(a.left, merge(a.right, b))
	}
	return b.with(merge(a, b.left), b.right)
}

// Snapshot is the text at one moment. It never changes, and any goroutine can read it.
// Offsets are byte offsets. Read methods clamp bad offsets and lines to the valid
// range: a stale cursor must not crash the editor and lose unsaved work.
type Snapshot struct {
	original []byte
	added    []byte // Only ever appended to, so older bytes are safe to read during edits.
	root     *node
}

func (s Snapshot) text(n *node) []byte {
	if n.inAdded {
		return s.added[n.start : n.start+n.length]
	}
	return s.original[n.start : n.start+n.length]
}

// Same reports whether two snapshots hold the same version of the text. It's false for
// equal text reached by different edits.
func (s Snapshot) Same(other Snapshot) bool { return s.root == other.root }

// Len returns the size of the text in bytes.
func (s Snapshot) Len() int { return s.root.size() }

// LineCount returns the number of lines. Only "\n" ends a line, so a "\r\n" line keeps
// its "\r" until Line removes it. Text ending in "\n" has an empty last line.
func (s Snapshot) LineCount() int { return s.root.lineBreaks() + 1 }

// Slice returns a copy of the bytes from start up to end.
func (s Snapshot) Slice(start, end int) []byte {
	start = max(start, 0)
	end = min(end, s.Len())
	if start >= end {
		return []byte{}
	}
	return s.appendRange(make([]byte, 0, end-start), s.root, start, end)
}

func (s Snapshot) appendRange(dst []byte, n *node, start, end int) []byte {
	leftLen := n.left.size()
	if start < leftLen {
		dst = s.appendRange(dst, n.left, start, min(end, leftLen))
	}
	if from, to := max(start-leftLen, 0), min(end-leftLen, n.length); from < to {
		dst = append(dst, s.text(n)[from:to]...)
	}
	if after := leftLen + n.length; end > after {
		dst = s.appendRange(dst, n.right, max(start-after, 0), end-after)
	}
	return dst
}

// LineStart returns the offset of a line's first byte, counting lines from 0.
func (s Snapshot) LineStart(line int) int {
	line = min(line, s.LineCount()-1)
	if line <= 0 {
		return 0
	}
	base := 0
	for n := s.root; ; {
		leftBreaks := n.left.lineBreaks()
		switch {
		case line <= leftBreaks:
			n = n.left
		case line <= leftBreaks+n.breaks:
			return base + n.left.size() + indexOfBreak(s.text(n), line-leftBreaks) + 1
		default:
			line -= leftBreaks + n.breaks
			base += n.left.size() + n.length
			n = n.right
		}
	}
}

// indexOfBreak returns the index of the nth line feed in text, counting from 1.
func indexOfBreak(text []byte, nth int) int {
	at := -1
	for ; nth > 0; nth-- {
		at += 1 + bytes.IndexByte(text[at+1:], '\n')
	}
	return at
}

// LineOf returns the line holding offset. A line feed belongs to the line it ends.
func (s Snapshot) LineOf(offset int) int {
	offset = min(offset, s.Len())
	line := 0
	for n := s.root; n != nil && offset > 0; {
		leftLen := n.left.size()
		if offset <= leftLen {
			n = n.left
			continue
		}
		line += n.left.lineBreaks()
		if offset <= leftLen+n.length {
			return line + bytes.Count(s.text(n)[:offset-leftLen], []byte{'\n'})
		}
		line += n.breaks
		offset -= leftLen + n.length
		n = n.right
	}
	return line
}

// Line returns a copy of a line without its "\n" or "\r\n".
func (s Snapshot) Line(line int) []byte {
	line = max(0, min(line, s.LineCount()-1))
	end := s.Len()
	if line+1 < s.LineCount() {
		end = s.LineStart(line + 1)
	}
	text := s.Slice(s.LineStart(line), end)
	return bytes.TrimSuffix(bytes.TrimSuffix(text, []byte{'\n'}), []byte{'\r'})
}

// split divides the tree at a document offset, cutting a piece in two if needed.
func (s Snapshot) split(n *node, offset int) (left, right *node) {
	if n == nil {
		return nil, nil
	}
	leftLen := n.left.size()
	// merge, not with: a piece cut further down has a new priority that can outrank n.
	switch {
	case offset <= leftLen:
		l, r := s.split(n.left, offset)
		return l, merge(r, n.with(nil, n.right))
	case offset >= leftLen+n.length:
		l, r := s.split(n.right, offset-leftLen-n.length)
		return merge(n.with(n.left, nil), l), r
	}
	cut := offset - leftLen
	text := s.text(n)
	var headBreaks int // Scanning only the shorter side keeps this cheap.
	if cut <= n.length/2 {
		headBreaks = bytes.Count(text[:cut], []byte{'\n'})
	} else {
		headBreaks = n.breaks - bytes.Count(text[cut:], []byte{'\n'})
	}
	head := newPiece(n.inAdded, n.start, cut, headBreaks)
	tail := newPiece(n.inAdded, n.start+cut, n.length-cut, n.breaks-headBreaks)
	return merge(n.left, head), merge(tail, n.right)
}

// pieces builds a tree over a stored range of the original or added text.
func (s Snapshot) pieces(inAdded bool, start, length int) *node {
	source := s.original
	if inAdded {
		source = s.added
	}
	var root *node
	for pos := start; pos < start+length; pos += maxPiece {
		size := min(maxPiece, start+length-pos)
		breaks := bytes.Count(source[pos:pos+size], []byte{'\n'})
		root = merge(root, newPiece(inAdded, pos, size, breaks))
	}
	return root
}

// extendLast grows n's last piece by the given added bytes, if they directly follow
// it and it stays within maxPiece. This stops typing from making one piece per key.
func extendLast(n *node, addedStart, length, breaks int) (*node, bool) {
	switch {
	case n == nil:
		return nil, false
	case n.right != nil:
		right, ok := extendLast(n.right, addedStart, length, breaks)
		if !ok {
			return nil, false
		}
		return n.with(n.left, right), true
	case !n.inAdded || n.start+n.length != addedStart || n.length+length > maxPiece:
		return nil, false
	}
	c := *n
	c.length += length
	c.breaks += breaks
	return c.with(c.left, nil), true
}

// WriteTo writes the whole text to w, without first copying it into one slice.
func (s Snapshot) WriteTo(w io.Writer) (int64, error) { return s.writeNode(w, s.root) }

func (s Snapshot) writeNode(w io.Writer, n *node) (int64, error) {
	if n == nil {
		return 0, nil
	}
	total, err := s.writeNode(w, n.left)
	if err != nil {
		return total, err
	}
	written, err := w.Write(s.text(n))
	total += int64(written)
	if err != nil {
		return total, err
	}
	written64, err := s.writeNode(w, n.right)
	return total + written64, err
}

// ErrReadOnly is returned when changing a text that can only be read.
var ErrReadOnly = errors.New("this file is read-only")

// Buffer is an editable text with undo. Only one goroutine may edit it. Others can
// read a copy of its embedded Snapshot.
type Buffer struct {
	Snapshot
	// Mark belongs to the caller: whatever goes with the current text, such as cursors.
	// An undo step keeps the Mark from just before its edit, and Undo and Redo set it
	// back, so the cursors can't drift out of step with the text.
	Mark       any
	undo, redo []step
	joinable   bool // The last change was an edit, so Continue may join its undo step.
	readOnly   bool
}

// step is a state undo can return to.
type step struct {
	root *node
	mark any
}

// ReadOnly reports whether edits are refused.
func (b *Buffer) ReadOnly() bool { return b.readOnly }

// New returns a buffer holding original. It's kept and never written to, and the
// caller must not change it either.
func New(original []byte) *Buffer {
	b := &Buffer{Snapshot: Snapshot{original: original}}
	b.root = b.pieces(false, 0, len(original))
	return b
}

// Edit replaces the bytes from Start up to End with Text.
type Edit struct {
	Start, End int
	Text       string
}

// Apply makes the edits as one new undo step, as a multi-cursor edit needs. Offsets
// all refer to the text before the call, and edits must be in order without
// overlapping. If they aren't, it returns an error and changes nothing.
func (b *Buffer) Apply(edits ...Edit) error { return b.apply(false, edits) }

// Continue is Apply, but joins the previous undo step when it can, as typing a word
// should. After Undo or Redo it starts a new step, since the step it would join is
// no longer simply "the last thing done".
func (b *Buffer) Continue(edits ...Edit) error { return b.apply(b.joinable, edits) }

func (b *Buffer) apply(join bool, edits []Edit) error {
	if b.readOnly {
		return ErrReadOnly
	}
	previousEnd := 0
	for _, e := range edits {
		if e.Start < previousEnd || e.End < e.Start || e.End > b.Len() {
			return fmt.Errorf("bad edit %d..%d: text is %d bytes, and the previous edit ended at %d",
				e.Start, e.End, b.Len(), previousEnd)
		}
		previousEnd = e.End
	}

	root := b.root
	for i := len(edits) - 1; i >= 0; i-- { // Last first, so earlier offsets stay valid.
		e := edits[i]
		if e.Start == e.End && e.Text == "" {
			continue
		}
		left, rest := b.split(root, e.Start)
		_, right := b.split(rest, e.End-e.Start)
		root = merge(b.insert(left, e.Text), right)
	}
	if root == b.root {
		return nil // Nothing changed, so there's nothing to undo.
	}

	if !join {
		b.undo = append(b.undo, step{b.root, b.Mark})
	}
	b.redo = nil
	b.root = root
	b.joinable = true
	return nil
}

// insert stores text and adds it after the end of left.
func (b *Buffer) insert(left *node, text string) *node {
	if text == "" {
		return left
	}
	start := len(b.added)
	b.added = append(b.added, text...)
	breaks := bytes.Count(b.added[start:], []byte{'\n'})
	if extended, ok := extendLast(left, start, len(text), breaks); ok {
		return extended
	}
	return merge(left, b.pieces(true, start, len(text)))
}

// Undo reverses the last step, and reports whether there was one.
func (b *Buffer) Undo() bool {
	if len(b.undo) == 0 {
		return false
	}
	b.redo = append(b.redo, step{b.root, b.Mark})
	last := b.undo[len(b.undo)-1]
	b.root, b.Mark = last.root, last.mark
	b.undo = b.undo[:len(b.undo)-1]
	b.joinable = false
	return true
}

// Redo restores the last undone step, and reports whether there was one. Any edit
// after an undo clears what could be redone.
func (b *Buffer) Redo() bool {
	if len(b.redo) == 0 {
		return false
	}
	b.undo = append(b.undo, step{b.root, b.Mark})
	last := b.redo[len(b.redo)-1]
	b.root, b.Mark = last.root, last.mark
	b.redo = b.redo[:len(b.redo)-1]
	b.joinable = false
	return true
}
