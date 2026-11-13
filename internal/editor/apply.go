package editor

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/Juan17la/lazycode/internal/buffer"
)

// Edit replaces the bytes Start up to End, which must hold Old, with Text. Old is how a
// change made from outside, such as a rename or a replace, finds out that the text moved
// on since it was worked out.
type Edit struct {
	Start, End int
	Old, Text  string
}

// FileChange is every edit to one file.
type FileChange struct {
	Path  string
	Edits []Edit
}

// plan checks the edits against the file and returns them in order, ready to apply.
func plan(f *buffer.File, edits []Edit) ([]buffer.Edit, error) {
	sorted := slices.Clone(edits)
	slices.SortStableFunc(sorted, func(a, b Edit) int { return a.Start - b.Start })
	changes := make([]buffer.Edit, len(sorted))
	end := 0
	for i, ed := range sorted {
		switch {
		case ed.Start < 0 || ed.End < ed.Start || ed.End > f.Len():
			return nil, fmt.Errorf("edit %d..%d is outside the text", ed.Start, ed.End)
		case ed.Start < end:
			return nil, fmt.Errorf("edits overlap at byte %d", ed.Start)
		case !bytes.Equal(f.Slice(ed.Start, ed.End), []byte(ed.Old)):
			return nil, fmt.Errorf("the text at byte %d changed", ed.Start)
		}
		end = ed.End
		changes[i] = buffer.Edit{Start: ed.Start, End: ed.End, Text: ed.Text}
	}
	return changes, nil
}

// ApplyToFile is ApplyEdits for a file nobody has open. It doesn't save.
func ApplyToFile(f *buffer.File, edits []Edit) error {
	changes, err := plan(f, edits)
	if err != nil || len(changes) == 0 {
		return err
	}
	return f.Apply(changes...)
}

// ApplyEdits makes all the edits as one undo step. Offsets refer to the text before the call, and
// edits can come in any order. If one doesn't match its Old text, or two overlap, nothing changes.
func (e *Editor) ApplyEdits(edits []Edit) error {
	changes, err := plan(e.File, edits)
	if err != nil || len(changes) == 0 {
		return err
	}
	e.saveMark()
	if err := e.File.Apply(changes...); err != nil {
		return err
	}
	for i, c := range e.cursors {
		e.cursors[i] = cursor{pos: mapOffset(c.pos, changes), anchor: mapOffset(c.anchor, changes), goal: -1}
	}
	e.edited()
	return nil
}

// SelectRange selects length bytes from start, as a search result does.
func (e *Editor) SelectRange(start, length int) {
	start = e.clamp(start)
	e.selectMatch(start, min(length, e.File.Len()-start))
}
