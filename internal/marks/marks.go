// Package marks works out which lines of a file were modified (Q-53, Q-76). That's all it
// does: it shows lines to look at, and has no review screen.
package marks

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"time"
)

// maxDifference bounds the work of comparing very different texts. Past it, every line
// between the matching start and end counts as modified, which is still true enough to look at.
const maxDifference = 1000

// Changed returns the lines of text, counting from 0, that are new or different from base.
// Removed lines leave no mark, as they're not there to mark. Line endings don't count, so a
// checkout that turned LF into CRLF doesn't mark the whole file.
func Changed(base, text []byte) []int {
	a, b := lines(base), lines(text)
	start := 0
	for start < len(a) && start < len(b) && bytes.Equal(a[start], b[start]) {
		start++
	}
	endA, endB := len(a), len(b)
	for endA > start && endB > start && bytes.Equal(a[endA-1], b[endB-1]) {
		endA--
		endB--
	}
	if endB == start {
		return nil
	}
	if endA == start {
		return span(start, endB)
	}
	ids := map[string]int{}
	intern := func(ls [][]byte) []int {
		out := make([]int, len(ls))
		for i, l := range ls {
			id, ok := ids[string(l)]
			if !ok {
				id = len(ids)
				ids[string(l)] = id
			}
			out[i] = id
		}
		return out
	}
	inserted, ok := diff(intern(a[start:endA]), intern(b[start:endB]))
	if !ok {
		return span(start, endB)
	}
	out := make([]int, len(inserted))
	for i, line := range inserted {
		out[i] = line + start
	}
	return out
}

func span(from, to int) []int {
	out := make([]int, 0, to-from)
	for i := from; i < to; i++ {
		out = append(out, i)
	}
	return out
}

func lines(text []byte) [][]byte {
	if len(text) == 0 {
		return nil
	}
	out := bytes.Split(text, []byte{'\n'})
	if len(out[len(out)-1]) == 0 {
		out = out[:len(out)-1] // A final line break doesn't start a line to compare.
	}
	for i, line := range out {
		out[i] = bytes.TrimSuffix(line, []byte{'\r'})
	}
	return out
}

// diff is Myers' algorithm, returning the indexes in b that aren't matched in a, in order.
// It reports false if a and b differ by more than maxDifference lines.
func diff(a, b []int) ([]int, bool) {
	n, m := len(a), len(b)
	limit := min(n+m, maxDifference)
	v := make([]int, 2*limit+2)
	var trace [][]int
	for d := 0; d <= limit; d++ {
		trace = append(trace, append([]int(nil), v...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[k-1+limit] < v[k+1+limit]) {
				x = v[k+1+limit] // A line from b.
			} else {
				x = v[k-1+limit] + 1 // A line from a.
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[k+limit] = x
			if x >= n && y >= m {
				return backtrack(trace, limit, n, m), true
			}
		}
	}
	return nil, false
}

func backtrack(trace [][]int, limit, x, y int) []int {
	var inserted []int
	for d := len(trace) - 1; d > 0; d-- {
		v := trace[d]
		k := x - y
		var prev int
		if k == -d || (k != d && v[k-1+limit] < v[k+1+limit]) {
			prev = k + 1
		} else {
			prev = k - 1
		}
		prevX := v[prev+limit]
		prevY := prevX - prev
		for x > prevX && y > prevY {
			x--
			y--
		}
		if prev == k+1 {
			inserted = append(inserted, prevY)
		}
		x, y = prevX, prevY
	}
	for i, j := 0, len(inserted)-1; i < j; i, j = i+1, j-1 {
		inserted[i], inserted[j] = inserted[j], inserted[i]
	}
	return inserted
}

// Base returns what to compare a file with: its text in the last Git commit. A file Git
// doesn't have yet compares with nothing, so all of it is new. Outside Git it returns false,
// and the caller uses the file as it was when opened.
func Base(path string) ([]byte, bool) {
	dir, name := filepath.Dir(path), "./"+filepath.ToSlash(filepath.Base(path))
	// Read-only. These settings stop a repository's own config running anything (doc 11).
	git := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		all := append([]string{"-C", dir, "--no-pager", "-c", "core.fsmonitor=false"}, args...)
		return exec.CommandContext(ctx, "git", all...).Output()
	}
	if _, err := git("rev-parse", "--is-inside-work-tree"); err != nil {
		return nil, false
	}
	text, err := git("show", "--no-textconv", "--no-ext-diff", "HEAD:"+name)
	if err != nil {
		return nil, true
	}
	return text, true
}
