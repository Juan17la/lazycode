package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Juan17la/lazycode/internal/editor"
	"github.com/Juan17la/lazycode/internal/keymap"
	"github.com/Juan17la/lazycode/internal/search"
	"github.com/gdamore/tcell/v2"
)

// searchDelay waits for typing to pause before a search starts, so each key doesn't cost one.
const searchDelay = 200 * time.Millisecond

const (
	itemHeading = iota
	itemName
	itemFile // The file a run of text matches belongs to.
	itemMatch
	itemNote
)

type searchItem struct {
	kind  int
	text  string
	path  string // Relative to the directory searched.
	match search.Match
}

func (it searchItem) selectable() bool { return it.kind == itemName || it.kind == itemMatch }

// searchState is the Search view.
type searchState struct {
	query, replace string
	onReplace      bool // The Replace field has the keys.
	res            search.Result
	items          []searchItem
	notes          []string // What a replace skipped.
	sel, top       int
	running        bool
	searched       bool // res is for query and dir.
	gen            int  // Bumped by each change, so older results are dropped.
	cancel         context.CancelFunc
	timer          *time.Timer
	dir            string
	err            error
}

func (s *searchState) field() *string {
	if s.onReplace {
		return &s.replace
	}
	return &s.query
}

// scheduleSearch drops the search in progress and starts a new one once typing pauses.
func (a *app) scheduleSearch() {
	s := &a.srch
	s.gen++
	if s.cancel != nil {
		s.cancel()
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	s.running, s.searched, s.err = false, false, nil
	s.res, s.notes = search.Result{}, nil
	s.rebuild()
	if s.query == "" {
		return
	}
	gen := s.gen
	s.timer = time.AfterFunc(searchDelay, func() { a.post(func() { a.runSearch(gen) }) })
}

func (a *app) runSearch(gen int) {
	s := &a.srch
	if gen != s.gen {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.running, s.dir = cancel, true, a.dirPath()
	dir, query := s.dir, s.query
	go func() {
		res, err := search.Run(ctx, dir, query)
		a.post(func() {
			if gen != s.gen {
				return // Typing went on, or the directory changed, since this started.
			}
			s.running, s.searched, s.res, s.err = false, true, res, err
			s.sel, s.top = 0, 0
			s.rebuild()
			a.requestRedraw()
		})
	}()
}

// searchDirChanged runs the search again in the new directory.
func (a *app) searchDirChanged() {
	if a.srch.query != "" && a.srch.dir != a.dirPath() {
		a.scheduleSearch()
	}
}

// rebuild lays the results out as rows: names first, then the text matches grouped by file.
func (s *searchState) rebuild() {
	s.items = s.items[:0]
	for _, n := range s.notes {
		s.items = append(s.items, searchItem{kind: itemNote, text: n})
	}
	if len(s.res.Names) > 0 {
		s.items = append(s.items, searchItem{kind: itemHeading, text: fmt.Sprintf("Files (%d)", len(s.res.Names))})
		for _, n := range s.res.Names {
			s.items = append(s.items, searchItem{kind: itemName, text: shown(n), path: n})
		}
	}
	if len(s.res.Matches) > 0 {
		heading := fmt.Sprintf("Text (%d)", len(s.res.Matches))
		if s.res.Capped {
			heading = fmt.Sprintf("Text (first %d, more not shown)", len(s.res.Matches))
		}
		s.items = append(s.items, searchItem{kind: itemHeading, text: heading})
		last := ""
		for _, m := range s.res.Matches {
			if m.Path != last {
				s.items = append(s.items, searchItem{kind: itemFile, text: shown(m.Path)})
				last = m.Path
			}
			s.items = append(s.items, searchItem{kind: itemMatch, path: m.Path, match: m,
				text: fmt.Sprintf("%d: %s", m.Line, strings.TrimSpace(shown(m.Text)))})
		}
	}
	s.sel = min(s.sel, max(0, len(s.items)-1))
	s.settle(1)
}

// settle moves sel to a row that can be picked, going the way dir says first.
func (s *searchState) settle(dir int) {
	for _, d := range []int{dir, -dir} {
		for i := s.sel; i >= 0 && i < len(s.items); i += d {
			if s.items[i].selectable() {
				s.sel = i
				return
			}
		}
	}
}

func (s *searchState) move(delta int) {
	step := 1
	if delta < 0 {
		step = -1
	}
	for n := abs(delta); n > 0; n-- {
		next := s.sel + step
		for next >= 0 && next < len(s.items) && !s.items[next].selectable() {
			next += step
		}
		if next < 0 || next >= len(s.items) {
			return
		}
		s.sel = next
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// searchKey gives the Search view its keys. A shortcut that works everywhere still does.
func (a *app) searchKey(ev *tcell.EventKey) bool {
	s := &a.srch
	if a.keys.Lookup(keymap.Global, keymap.Name(ev)) != "" || keymap.Name(ev) == "ctrl+k" {
		return false
	}
	field := s.field()
	switch ev.Key() {
	case tcell.KeyEsc:
		if a.tab() != nil {
			a.focusTab()
		}
	case tcell.KeyTab, tcell.KeyBacktab:
		s.onReplace = !s.onReplace
	case tcell.KeyUp:
		s.move(-1)
	case tcell.KeyDown:
		s.move(1)
	case tcell.KeyPgUp:
		s.move(-10)
	case tcell.KeyPgDn:
		s.move(10)
	case tcell.KeyEnter:
		if s.onReplace {
			a.searchReplace()
		} else {
			a.openSearchItem(s.sel)
		}
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if *field != "" {
			_, size := utf8.DecodeLastRuneInString(*field)
			*field = (*field)[:len(*field)-size]
			a.fieldChanged()
		}
	case tcell.KeyRune:
		if mods := ev.Modifiers(); mods&(tcell.ModCtrl|tcell.ModAlt) != 0 {
			return false
		}
		if ev.Rune() == ':' && *field == "" && s.query == "" {
			return false // So the command bar still opens from here.
		}
		*field += string(ev.Rune())
		a.fieldChanged()
	default:
		return false
	}
	return true
}

func (a *app) fieldChanged() {
	if !a.srch.onReplace {
		a.scheduleSearch()
	}
}

func (a *app) searchPaste(text string) {
	field := a.srch.field()
	*field += strings.NewReplacer("\r", "", "\n", "").Replace(text)
	a.fieldChanged()
}

// openSearchItem opens the file of a result, with the match selected.
func (a *app) openSearchItem(i int) {
	s := &a.srch
	if i < 0 || i >= len(s.items) || !s.items[i].selectable() {
		return
	}
	item := s.items[i]
	st := a.dir()
	path := filepath.Join(s.dirOrCurrent(a), item.path)
	a.openFile(st, path)
	if item.kind != itemMatch {
		return
	}
	if it := a.tab(); it != nil && it.ed != nil && it.ed.File.Path == path {
		m := item.match
		e := it.ed
		if m.Line-1 >= e.File.LineCount() {
			return
		}
		start := min(e.File.LineStart(m.Line-1)+m.Col, e.File.Len())
		if string(e.File.Slice(start, min(start+len(m.Found), e.File.Len()))) == m.Found {
			e.SelectRange(start, len(m.Found))
		} else {
			e.GoTo(m.Line-1, 0, false) // The file changed since; the line is the best guide.
		}
	}
}

func (s *searchState) dirOrCurrent(a *app) string {
	if s.dir != "" {
		return s.dir
	}
	return a.dirPath()
}

// searchReplace turns the matches into changes, and shows them before anything is written.
func (a *app) searchReplace() {
	s := &a.srch
	switch {
	case s.query == "":
		a.message = "Type what to find first."
		return
	case s.running || !s.searched:
		a.message = "Wait for the search to finish."
		return
	case len(s.res.Matches) == 0:
		a.message = "There's no text to replace."
		return
	}
	dir := s.dir
	byPath := map[string]*editor.FileChange{}
	var changes []editor.FileChange
	for _, m := range s.res.Matches {
		c := byPath[m.Path]
		if c == nil {
			changes = append(changes, editor.FileChange{Path: filepath.Join(dir, m.Path)})
			c = &changes[len(changes)-1]
			byPath[m.Path] = c
		}
		c.Edits = append(c.Edits, editor.Edit{Start: m.Start, End: m.Start + len(m.Found), Old: m.Found, Text: s.replace})
	}
	with := "nothing"
	if s.replace != "" {
		with = fmt.Sprintf("%q", shown(s.replace))
	}
	title := fmt.Sprintf("Replace %q with %s: %d changes in %d files", shown(s.query), with, len(s.res.Matches), len(changes))
	if s.res.Capped {
		title += " (only the first results are listed, and only those change)"
	}
	a.reviewChanges(title, changes, func(applied int, failed []string) {
		switch {
		case applied == 0 && len(failed) == 0:
			a.message = "Nothing was replaced."
			return
		case len(failed) == 0:
			a.message = fmt.Sprintf("Replaced in %d files.", applied)
		default:
			a.message = fmt.Sprintf("Replaced in %d files, and %d were skipped, listed in Search.", applied, len(failed))
		}
		a.scheduleSearch()
		s.notes = failed // After scheduleSearch, which clears them.
		s.rebuild()
	})
}

// Drawing.

func (a *app) drawSearch(r rect) {
	s := &a.srch
	maxX := r.x + r.w
	focused := !a.tabFocused && a.focusedView() == viewSearch && a.overlayFree()
	label := func(y int, name, text string, active bool) {
		style := a.th.primary
		if active && focused {
			style = a.th.accent
		}
		x := a.put(r.x+1, y, maxX, name, a.th.muted)
		end := a.put(x, y, maxX, text, style)
		if active && focused {
			a.screen.ShowCursor(min(end, maxX-1), y)
		}
	}
	label(r.y+1, "Find: ", s.query, !s.onReplace)
	label(r.y+2, "With: ", s.replace, s.onReplace)
	a.hits = append(a.hits,
		hit{area: rect{r.x, r.y + 1, r.w, 1}, click: func(int, int, tcell.ModMask) { s.onReplace, a.tabFocused = false, false }},
		hit{area: rect{r.x, r.y + 2, r.w, 1}, click: func(int, int, tcell.ModMask) { s.onReplace, a.tabFocused = true, false }})

	status := "Type to search names and text"
	switch {
	case s.query == "":
	case s.running || !s.searched:
		status = "Searching..."
	case s.err != nil:
		status = "Search failed: " + shown(s.err.Error())
	case len(s.res.Names) == 0 && len(s.res.Matches) == 0:
		status = "No results in " + filepath.Base(a.dirPath())
	default:
		status = fmt.Sprintf("%d in names, %d in text", len(s.res.Names), len(s.res.Matches))
	}
	button := "[Replace all]"
	x := a.put(r.x+1, r.y+3, maxX, button, a.th.accent)
	a.hits = append(a.hits, hit{area: rect{r.x + 1, r.y + 3, x - r.x - 1, 1}, click: func(int, int, tcell.ModMask) { a.searchReplace() }})
	a.put(x+1, r.y+3, maxX, "Tab: field", a.th.muted)
	a.put(r.x+1, r.y+4, maxX, status, a.th.muted)

	y0, rows := r.y+5, r.h-5
	if rows <= 0 {
		return
	}
	if s.sel < s.top {
		s.top = s.sel
	} else if s.sel >= s.top+rows {
		s.top = s.sel - rows + 1
	}
	for i := s.top; i < len(s.items) && i < s.top+rows; i++ {
		item := s.items[i]
		style, prefix := a.th.primary, "  "
		switch item.kind {
		case itemHeading:
			style, prefix = a.th.accent, ""
		case itemFile, itemNote:
			style, prefix = a.th.muted, ""
		case itemMatch:
			prefix = "   "
		}
		if i == s.sel && !a.tabFocused && a.focusedView() == viewSearch {
			style, prefix = a.th.active, "› "
		}
		a.put(r.x+1, y0+i-s.top, maxX, prefix+item.text, style)
	}
	top := s.top
	a.hits = append(a.hits, hit{
		area: rect{r.x, y0, r.w, rows},
		click: func(_, y int, _ tcell.ModMask) {
			if i := top + y - y0; i >= 0 && i < len(s.items) && s.items[i].selectable() {
				s.sel = i
				a.tabFocused = false
				a.openSearchItem(i)
			}
		},
		wheel: func(delta int) { s.move(delta * wheelLines) },
	})
}

// File picker (Ctrl+P).

// openFilePicker lists the directory's files in the switcher as soon as git or the walk has them.
func (a *app) openFilePicker() {
	dir := a.dirPath()
	s := &switcher{title: "Open file (loading...)"}
	var paths []string // The real names, which the labels have cleaned for the screen.
	s.pick = func(i int) { a.openFile(a.dir(), filepath.Join(dir, paths[i])) }
	a.switcher = s
	go func() {
		files, err := search.Files(context.Background(), dir)
		a.post(func() {
			if a.switcher != s {
				return
			}
			s.title = "Open file"
			if err != nil {
				s.title = "Open file: " + shown(err.Error())
			}
			for _, f := range files {
				s.labels = append(s.labels, shown(f))
				paths = append(paths, f)
			}
		})
	}()
}
