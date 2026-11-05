package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Juan17la/lazycode/internal/buffer"
	"github.com/Juan17la/lazycode/internal/editor"
	"github.com/Juan17la/lazycode/internal/lsp"
)

// maxServerCrashes is how many times a server may stop or fail to start before it stays off.
const maxServerCrashes = 3

// lspClient is a running server and where it was started.
type lspClient struct {
	*lsp.Client
	key string
}

// language servers (E2). They start when the user first asks for definition, rename or format, and
// not before: a server can run the project's code, so an untrusted folder never gets one (doc 11).
type language struct {
	clients  map[string]*lspClient // By server and root.
	starting map[string]bool
	crashes  map[string]int
	wg       sync.WaitGroup // Starts and shutdowns in flight.
}

// withServer runs use with the server for a file, starting it first if need be. Every way it can't
// say so once, in the status line, and editing carries on (doc 07). use runs on the UI goroutine.
func (a *app) withServer(path string, use func(lc *lspClient, languageID string)) {
	srv, languageID, ok := lsp.ForPath(path)
	if !ok {
		a.message = "No language server for this kind of file."
		return
	}
	l := &a.lang
	dir := a.dirPath()
	root := lsp.Root(srv, path, dir)
	key := srv.Name + "\x00" + root
	switch {
	case l.clients[key] != nil:
		use(l.clients[key], languageID)
		return
	case l.starting[key]:
		a.message = srv.Name + " is still starting."
		return
	case l.crashes[key] >= maxServerCrashes:
		a.message = srv.Name + " keeps stopping here, so it stays off until LazyCode restarts."
		return
	}
	if _, err := exec.LookPath(srv.Command); err != nil {
		a.message = srv.Name + " isn't installed. Install: " + srv.Install
		return
	}
	// Starting one runs code from the folder, so it asks like :play does.
	a.requireTrust(dir, srv.Name, func() {
		a.startServer(key, srv, root, func(lc *lspClient) { use(lc, languageID) })
	})
}

func (a *app) startServer(key string, srv lsp.Server, root string, then func(*lspClient)) {
	l := &a.lang
	if l.clients == nil {
		l.clients, l.starting, l.crashes = map[string]*lspClient{}, map[string]bool{}, map[string]int{}
	}
	l.starting[key] = true
	a.message = "Starting " + srv.Name + "..."
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		c, err := lsp.Start(srv, root, a.done) // The app stopping ends the start.
		a.postOr(func() {
			delete(l.starting, key)
			if err != nil {
				if !errors.Is(err, lsp.ErrNotInstalled) {
					l.crashes[key]++
				}
				a.message = err.Error()
				return
			}
			lc := &lspClient{c, key}
			l.clients[key] = lc
			go func() {
				<-c.Dead()
				a.post(func() { a.serverDied(lc) })
			}()
			a.message = ""
			then(lc)
		}, func() {
			if c != nil {
				c.Shutdown()
			}
		})
	}()
}

// postOr runs f on the UI goroutine, or elseDo if the app stopped before it could.
func (a *app) postOr(f, elseDo func()) {
	select {
	case a.tasks <- f:
	case <-a.done:
		elseDo()
	}
}

// serverDied forgets a server that stopped by itself. The next request starts it again, up to a limit.
func (a *app) serverDied(lc *lspClient) {
	l := &a.lang
	if l.clients[lc.key] != lc {
		return // We stopped it.
	}
	delete(l.clients, lc.key)
	l.crashes[lc.key]++
	why := "stopped"
	if err := lc.Err(); err != nil {
		why = "stopped: " + err.Error()
	}
	a.message = lc.Server.Name + " " + why + "."
	if l.crashes[lc.key] < maxServerCrashes {
		a.message += " The next request starts it again."
	}
}

// stopServers shuts down the servers started in these directories, as a project closes. It doesn't
// make the UI wait.
func (a *app) stopServers(dirs []string) {
	l := &a.lang
	for key, lc := range l.clients {
		for _, dir := range dirs {
			if inDir(dir, lc.Root) {
				delete(l.clients, key)
				l.wg.Add(1)
				go func() { defer l.wg.Done(); lc.Shutdown() }()
				break
			}
		}
	}
}

// stopAllServers runs as the app stops, and returns once every server process is gone.
func (a *app) stopAllServers() {
	l := &a.lang
	for key, lc := range l.clients {
		delete(l.clients, key)
		l.wg.Add(1)
		go func() { defer l.wg.Done(); lc.Shutdown() }()
	}
	l.wg.Wait()
}

// syncServers tells servers about files that closed. It runs after every event, like syncLive.
func (a *app) syncServers() {
	if len(a.lang.clients) == 0 {
		return
	}
	var open map[string]bool
	for _, lc := range a.lang.clients {
		for _, path := range lc.OpenDocs() {
			if open == nil {
				open = map[string]bool{}
				for f := range a.openFiles() {
					open[f.Path] = true
				}
			}
			if !open[path] {
				go lc.CloseDoc(path)
			}
		}
	}
}

// failure turns what went wrong into a sentence. A server that died is reported once, by serverDied.
func (lc *lspClient) failure(err error) string {
	select {
	case <-lc.Dead():
		return ""
	default:
	}
	var rpc *lsp.RPCError
	if errors.As(err, &rpc) {
		return lc.Server.Name + ": " + rpc.Message
	}
	return lc.Server.Name + ": " + err.Error()
}

// request is what one action needs from the file it was asked in.
type request struct {
	it   *tabItem
	file *buffer.File
	snap buffer.Snapshot // The text sent. Answers for any other text are refused.
	text []byte
	off  int // Cursor, as a byte offset in text.
}

// current is the open file the action was asked in, or nil with a message.
func (a *app) currentFile() *tabItem {
	it := a.tab()
	if it == nil || it.ed == nil {
		a.message = "That needs a file."
		return nil
	}
	return it
}

// unchanged reports whether the file is still open with the text the request was made from.
func (a *app) unchanged(r request) bool {
	if a.isOpen(r.file) && r.file.Snapshot.Same(r.snap) {
		return true
	}
	a.message = r.it.name() + " changed while the language server was answering, so nothing was done. Try again."
	return false
}

// serverCall runs work on a goroutine, with the server told about the file's text first, then hands what it
// returns to done on the UI goroutine. The UI never waits for a server.
func (a *app) serverCall(lc *lspClient, languageID string, r request, work func(c *lsp.Client, doc *lsp.Doc) (any, error), done func(result any)) {
	go func() {
		doc := lsp.NewDoc(r.text)
		err := lc.Sync(r.file.Path, languageID, r.text)
		var result any
		if err == nil {
			result, err = work(lc.Client, doc)
		}
		a.post(func() {
			if err != nil {
				if msg := lc.failure(err); msg != "" {
					a.message = msg
				}
			} else if a.unchanged(r) {
				done(result)
			}
		})
	}()
}

// definition is F12: go to where the name under the cursor is defined.
func (a *app) definition() {
	it := a.currentFile()
	if it == nil {
		return
	}
	a.withServer(it.ed.File.Path, func(lc *lspClient, languageID string) {
		r := a.newRequest(it)
		a.serverCall(lc, languageID, r, func(c *lsp.Client, doc *lsp.Doc) (any, error) {
			return c.Definition(r.file.Path, doc.Position(r.off))
		}, func(result any) {
			locs := result.([]lsp.Location)
			if len(locs) == 0 {
				a.message = "No definition found."
				return
			}
			a.goToLocation(locs[0])
			if len(locs) > 1 { // ponytail: the first of several. A list to pick from is the upgrade.
				a.message = fmt.Sprintf("%d definitions. This is the first.", len(locs))
			}
		})
	})
}

func (a *app) newRequest(it *tabItem) request {
	snap := it.ed.File.Snapshot
	return request{it: it, file: it.ed.File, snap: snap, text: snap.Slice(0, snap.Len()), off: it.ed.CursorOffset()}
}

// goToLocation opens the file, even from outside the directory, and puts the cursor there.
func (a *app) goToLocation(loc lsp.Location) {
	if cur := a.tab(); cur == nil || cur.ed == nil || cur.ed.File.Path != loc.Path {
		a.openFile(a.dir(), loc.Path)
		if strings.HasSuffix(a.message, " is already open.") {
			a.message = "" // Going there is what was asked, so it isn't news.
		}
	}
	it := a.tab()
	if it == nil || it.ed == nil || it.ed.File.Path != loc.Path {
		return // openFile said why.
	}
	s := it.ed.File.Snapshot
	// ponytail: positions are for the text the server has, which is the file on disk unless it was
	// opened through here. An open file with unsaved edits may land a few columns off.
	at := s.Len()
	if loc.Range.Start.Line < s.LineCount() {
		at = s.LineStart(loc.Range.Start.Line) + lsp.ColumnOffset(s.Line(loc.Range.Start.Line), loc.Range.Start.Character)
	}
	it.ed.GoToOffset(at)
	a.focusTab()
}

// renameCommand is F2: ask for the new name, then show what would change.
func (a *app) renameCommand() {
	it := a.currentFile()
	if it == nil {
		return
	}
	f := it.ed.File
	start, end := it.ed.WordAtCursor()
	word := string(f.Slice(start, end))
	switch {
	case word == "":
		a.message = "Put the cursor on a name to rename it."
	case f.Modified():
		a.message = "Save " + it.name() + " first (Ctrl+S): a rename changes saved files."
	default:
		a.ask1Line("Rename to:", word, func(name string) {
			if name == word {
				a.message = "That's the same name."
				return
			}
			a.rename(it, name)
		})
	}
}

func (a *app) rename(it *tabItem, name string) {
	a.withServer(it.ed.File.Path, func(lc *lspClient, languageID string) {
		if it.ed.File.Modified() {
			a.message = "Save " + it.name() + " first (Ctrl+S): a rename changes saved files."
			return
		}
		r := a.newRequest(it)
		a.serverCall(lc, languageID, r, func(c *lsp.Client, doc *lsp.Doc) (any, error) {
			edits, err := c.Rename(r.file.Path, doc.Position(r.off), name)
			if err != nil {
				if errors.Is(err, lsp.ErrFileOperation) {
					return nil, fmt.Errorf("%s also wants to create, rename or delete files, so nothing changed", lc.Server.Name)
				}
				return nil, err
			}
			return changesFrom(r, edits)
		}, func(result any) {
			changes := result.([]editor.FileChange)
			if len(changes) == 0 {
				a.message = "Nothing to rename there."
				return
			}
			a.reviewChanges("Rename to "+name, changes, func(applied int, failed []string) {
				switch {
				case applied == 0 && len(failed) == 0:
					a.message = "Nothing was renamed."
				case len(failed) == 0:
					a.message = fmt.Sprintf("Renamed in %d files.", applied)
				default:
					a.message = fmt.Sprintf("Renamed in %d files. Not changed: %s.", applied, strings.Join(failed, ", "))
				}
			})
		})
	})
}

// changesFrom works out byte offsets for a server's edits, against the text each file had when the
// server read it: the text sent for the file asked about, and the file on disk for the rest. A file
// that can't be read stops it all, so a rename is never half done.
func changesFrom(r request, edits map[string][]lsp.TextEdit) ([]editor.FileChange, error) {
	var paths []string
	for path := range edits {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var changes []editor.FileChange
	for _, path := range paths {
		text := r.text
		if path != r.file.Path {
			info, err := os.Stat(path)
			if err == nil && info.Size() > maxChangeFile {
				err = errors.New("too large to change here")
			}
			if err == nil {
				text, err = os.ReadFile(path)
			}
			if err != nil {
				return nil, fmt.Errorf("can't read %s, so nothing was renamed: %w", filepath.Base(path), err)
			}
		}
		es, err := editsFor(text, edits[path])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		changes = append(changes, editor.FileChange{Path: path, Edits: es})
	}
	return changes, nil
}

// editsFor converts a server's edits to byte offsets in text, with the text each replaces.
func editsFor(text []byte, edits []lsp.TextEdit) ([]editor.Edit, error) {
	doc := lsp.NewDoc(text)
	crlf := firstBreakIsCRLF(text)
	out := make([]editor.Edit, 0, len(edits))
	for _, e := range edits {
		start, end := doc.Offset(e.Range.Start), doc.Offset(e.Range.End)
		if end < start {
			return nil, errors.New("the server sent a backwards range")
		}
		out = append(out, editor.Edit{Start: start, End: end, Old: string(text[start:end]), Text: lineEndings(e.NewText, crlf)})
	}
	return out, nil
}

// firstBreakIsCRLF says which line ending the file uses, by its first.
func firstBreakIsCRLF(text []byte) bool {
	i := bytes.IndexByte(text, '\n')
	return i > 0 && text[i-1] == '\r'
}

// lineEndings gives inserted text the file's own line endings, as servers write \n.
func lineEndings(text string, crlf bool) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if crlf {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}

// format is Shift+Alt+F: format the whole file, as one undo step.
func (a *app) format() {
	it := a.currentFile()
	if it == nil {
		return
	}
	a.withServer(it.ed.File.Path, func(lc *lspClient, languageID string) {
		r := a.newRequest(it)
		a.serverCall(lc, languageID, r, func(c *lsp.Client, doc *lsp.Doc) (any, error) {
			edits, err := c.Format(r.file.Path, a.cfg.Editor.TabSize, true)
			if err != nil {
				return nil, err
			}
			return editsFor(r.text, edits)
		}, func(result any) {
			edits := result.([]editor.Edit)
			if len(edits) == 0 {
				a.message = it.name() + " is already formatted."
				return
			}
			before := it.ed.File.Snapshot
			a.editWith(it, func() error { return it.ed.ApplyEdits(edits) })
			if it.ed.File.Snapshot.Same(before) && a.message == "" {
				a.message = "Formatting changed nothing."
			} else if !it.ed.File.Snapshot.Same(before) {
				a.message = "Formatted with " + lc.Server.Name + ". Ctrl+Z undoes it."
			}
		})
	})
}
