// Package search finds file names and text in a directory (doc 06): plain text, in the background,
// stopping when its context is cancelled.
package search

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Juan17la/lazycode/internal/git"
)

const (
	// MaxResults bounds what a search returns, so a one-letter query can't fill the memory or the screen.
	MaxResults = 5000
	// maxFile is the largest file searched, as the editor opens bigger ones read-only (D-32).
	maxFile = 100 << 20
	// maxLine is the longest line searched. A file with longer ones is minified, and not worth the memory.
	maxLine     = 1 << 20
	binarySniff = 8 * 1024
	maxWorkers  = 8
)

// Match is one occurrence of the text.
type Match struct {
	Path  string // Relative to the searched directory, with the OS's separators.
	Line  int    // From 1.
	Col   int    // Byte offset in the line, from 0.
	Start int    // Byte offset in the file.
	Found string // The text as the file spells it, which differs from the query in case.
	Text  string // The line, cut if very long.
}

// Result is a finished search.
type Result struct {
	Names   []string // Paths whose name holds the query, best first.
	Matches []Match  // In path order, then line order.
	Files   int      // How many files were looked at.
	Skipped int      // Binary, too large, or unreadable.
	Capped  bool     // There were more than MaxResults of names or matches.
}

// Files lists the files in dir, relative to it and sorted. In a Git repository it asks Git, which
// follows every .gitignore, and includes files not yet added. Elsewhere it walks the tree.
// ponytail: outside Git nothing is ignored except .git, so a folder with node_modules searches it
// all. Reading a root .gitignore is the upgrade if that bites.
func Files(ctx context.Context, dir string) ([]string, error) {
	files, err := git.ListFiles(ctx, dir)
	if err == nil {
		for i, f := range files {
			files[i] = filepath.FromSlash(f)
		}
		sort.Strings(files)
		return files, nil
	}
	if !errors.Is(err, git.ErrNotRepo) && !errors.Is(err, git.ErrMissing) {
		return nil, err
	}
	var out []string
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		switch {
		case err != nil:
			return nil // Unreadable folders and files are left out.
		case d.IsDir() && d.Name() == ".git":
			return filepath.SkipDir
		case d.Type().IsRegular():
			if rel, err := filepath.Rel(dir, path); err == nil {
				out = append(out, rel)
			}
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// Run searches for query. Matching ignores case if the query is all lower case, for ASCII
// letters. An empty query, or one with a line break, finds nothing.
func Run(ctx context.Context, dir, query string) (Result, error) {
	var res Result
	if query == "" || strings.ContainsAny(query, "\r\n") {
		return res, nil
	}
	files, err := Files(ctx, dir)
	if err != nil {
		return res, err
	}
	res.Names = nameMatches(files, query)
	if len(res.Names) > MaxResults {
		res.Names, res.Capped = res.Names[:MaxResults], true
	}

	needle := []byte(query)
	fold := !hasUpper(needle)
	if fold {
		lowerASCII(needle, needle)
	}
	workers := min(maxWorkers, max(1, runtime.NumCPU()))
	type found struct {
		matches []Match
		skipped bool
	}
	results := make([]found, len(files))
	var total atomic.Int64
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			var lower []byte
			for i := range jobs {
				if ctx.Err() != nil {
					continue
				}
				m, ok := searchFile(ctx, dir, files[i], needle, fold, &lower)
				results[i] = found{m, !ok}
				total.Add(int64(len(m)))
			}
		})
	}
	// Files are handed out in order and stopped at the cap, so what's found is always the first
	// files in order, and not whichever a fast worker reached.
	done := 0
	for i := range files {
		if ctx.Err() != nil || total.Load() >= MaxResults {
			break
		}
		jobs <- i
		done = i + 1
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	res.Files = done
	for _, f := range results[:done] {
		if f.skipped {
			res.Skipped++
		}
		res.Matches = append(res.Matches, f.matches...)
	}
	if done < len(files) || len(res.Matches) > MaxResults {
		res.Capped = true
	}
	if len(res.Matches) > MaxResults {
		res.Matches = res.Matches[:MaxResults]
	}
	return res, nil
}

// nameMatches returns paths holding query, ignoring case. Those where the file's own name holds it
// come first, then shorter paths.
func nameMatches(files []string, query string) []string {
	q := strings.ToLower(query)
	var hits []string
	for _, f := range files {
		if strings.Contains(strings.ToLower(f), q) {
			hits = append(hits, f)
		}
	}
	inName := func(p string) bool { return strings.Contains(strings.ToLower(filepath.Base(p)), q) }
	slices.SortStableFunc(hits, func(a, b string) int {
		switch ia, ib := inName(a), inName(b); {
		case ia && !ib:
			return -1
		case ib && !ia:
			return 1
		}
		return len(a) - len(b)
	})
	return hits
}

func hasUpper(b []byte) bool {
	for _, c := range b {
		if c >= 'A' && c <= 'Z' {
			return true
		}
	}
	return false
}

// lowerASCII writes src into dst with A-Z lowered. Only ASCII, so the length never changes and
// offsets still fit the original.
func lowerASCII(dst, src []byte) {
	for i, c := range src {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		dst[i] = c
	}
}

// searchFile returns the matches in one file. ok is false if it was skipped.
func searchFile(ctx context.Context, dir, rel string, needle []byte, fold bool, lower *[]byte) (matches []Match, ok bool) {
	path := filepath.Join(dir, rel)
	// Lstat, so a link pointing outside the folder isn't followed, and a pipe isn't opened.
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() || info.Size() > maxFile {
		return nil, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	head := make([]byte, binarySniff)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, false
	}
	head = head[:n]
	if bytes.IndexByte(head, 0) >= 0 {
		return nil, false
	}
	sc := bufio.NewScanner(io.MultiReader(bytes.NewReader(head), f))
	sc.Buffer(make([]byte, 0, 64*1024), maxLine)
	sc.Split(splitLines)
	offset, line := 0, 0
	for sc.Scan() {
		line++
		text := sc.Bytes()
		if line%4096 == 0 && ctx.Err() != nil {
			return nil, true
		}
		hay := text
		if fold {
			if cap(*lower) < len(text) {
				*lower = make([]byte, len(text)*2)
			}
			hay = (*lower)[:len(text)]
			lowerASCII(hay, text)
		}
		for from := 0; ; {
			i := bytes.Index(hay[from:], needle)
			if i < 0 {
				break
			}
			col := from + i
			matches = append(matches, Match{
				Path: rel, Line: line, Col: col, Start: offset + col,
				Found: string(text[col : col+len(needle)]), Text: shortLine(text),
			})
			from = col + len(needle)
		}
		offset += len(text) + 1
	}
	// A line over maxLine stops the scan: what's before it still counts.
	return matches, true
}

// splitLines splits at \n only, so offsets add up. A trailing \r stays in the line.
func splitLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// shortLine is a line to show: cut at 300 bytes, on a character, and without the trailing \r.
func shortLine(text []byte) string {
	text = bytes.TrimSuffix(text, []byte{'\r'})
	if len(text) > 300 {
		cut := 300
		for cut > 0 && text[cut]&0xC0 == 0x80 {
			cut--
		}
		return string(text[:cut]) + "…"
	}
	return string(text)
}
