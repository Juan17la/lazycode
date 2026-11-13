package lsp

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

const maxTimeouts = 3 // In a row, and the server is taken for stuck and killed.

// Variables, so that tests needn't wait.
var (
	startTimeout   = 60 * time.Second // The first start of gopls can load a whole module.
	requestTimeout = 30 * time.Second
	exitWait       = time.Second // How long a server gets to leave after shutdown, before it's killed.
)

// ErrNotInstalled means the server's command isn't on the PATH.
var ErrNotInstalled = errors.New("not installed")

// Client talks to one running server. Its methods can be called from any goroutine.
type Client struct {
	Server Server
	Root   string

	cmd   *exec.Cmd
	out   chan []byte // Messages to write, in order, so didChange can't pass the request after it.
	dead  chan struct{}
	gone  chan struct{} // Closed when the process has exited.
	mu    sync.Mutex
	err   error // Why it died, once dead is closed.
	next  int64
	wait  map[int64]chan reply
	docs  map[string]docState
	stuck int
}

type docState struct {
	version int
	sum     [sha256.Size]byte
}

type reply struct {
	result json.RawMessage
	err    error
}

// message is any JSON-RPC message: a request has a method and an ID, a notification only a method,
// and a response an ID and a result or an error.
type message struct {
	ID     *json.RawMessage `json:"id,omitempty"`
	Method string           `json:"method,omitempty"`
	Params json.RawMessage  `json:"params,omitempty"`
	Result json.RawMessage  `json:"result,omitempty"`
	Error  *RPCError        `json:"error,omitempty"`
}

// RPCError is an error the server answered with.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("%s (code %d)", e.Message, e.Code) }

// Start runs the server in root and completes the handshake. It blocks until the server answers, so
// it must not run on the UI goroutine. Closing abort (which may be nil) stops the server and ends
// the wait, as the app does when it quits.
func Start(srv Server, root string, abort <-chan struct{}) (*Client, error) {
	path, err := exec.LookPath(srv.Command)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", srv.Command, ErrNotInstalled)
	}
	// The path and arguments are fixed, and there's no shell. Stderr goes to the null device, which
	// drains it so that a chatty server can't block, and it's never shown.
	cmd := exec.Command(path, srv.Args...)
	cmd.Dir = root
	prepare(cmd)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &Client{
		Server: srv, Root: root, cmd: cmd,
		out: make(chan []byte, 64), dead: make(chan struct{}), gone: make(chan struct{}),
		wait: map[int64]chan reply{}, docs: map[string]docState{},
	}
	go func() {
		cmd.Wait() //nolint:errcheck // Only that it ended matters.
		close(c.gone)
		c.fail(errors.New("the server exited"))
	}()
	go c.writeLoop(in)
	go c.readLoop(bufio.NewReader(stdout))
	go func() {
		select {
		case <-abort:
			c.fail(errors.New("LazyCode is closing"))
		case <-c.dead:
		}
	}()
	if err := c.handshake(); err != nil {
		c.kill()
		return nil, fmt.Errorf("%s didn't start: %w", srv.Name, err)
	}
	return c, nil
}

func (c *Client) handshake() error {
	uri := FileURI(c.Root)
	params := map[string]any{
		"processId":        os.Getpid(),
		"rootUri":          uri,
		"clientInfo":       map[string]string{"name": "LazyCode"},
		"workspaceFolders": []map[string]string{{"uri": uri, "name": baseName(c.Root)}},
		"capabilities": map[string]any{
			"general": map[string]any{"positionEncodings": []string{"utf-16"}},
			"textDocument": map[string]any{
				"synchronization": map[string]any{},
				"definition":      map[string]any{"linkSupport": true},
				"rename":          map[string]any{},
				"formatting":      map[string]any{},
			},
			// documentChanges, and no resource operations: renaming a file is refused, so none are asked for.
			"workspace": map[string]any{"workspaceEdit": map[string]any{"documentChanges": true}, "workspaceFolders": true, "configuration": true},
		},
	}
	if _, err := c.call("initialize", params, startTimeout); err != nil {
		return err
	}
	return c.notify("initialized", map[string]any{})
}

func baseName(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}

// Dead is closed once the server has stopped or been given up on. Err says why.
func (c *Client) Dead() <-chan struct{} { return c.dead }

func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// fail marks the server dead, once, and stops it. Requests waiting for it return the error.
func (c *Client) fail(err error) {
	c.mu.Lock()
	select {
	case <-c.dead:
		c.mu.Unlock()
		return
	default:
	}
	c.err = err
	close(c.dead)
	c.mu.Unlock()
	c.kill()
}

// kill stops the server and what it started, unless it's already been reaped: its process group ID could
// have been given to something else by now.
func (c *Client) kill() {
	select {
	case <-c.gone:
	default:
		killTree(c.cmd)
	}
}

func (c *Client) writeLoop(in io.WriteCloser) {
	defer in.Close()
	for {
		select {
		case body := <-c.out:
			if err := writeFrame(in, body); err != nil {
				c.fail(fmt.Errorf("can't write to the server: %w", err))
				return
			}
		case <-c.dead:
			return
		}
	}
}

func (c *Client) readLoop(r *bufio.Reader) {
	for {
		body, err := readFrame(r)
		if err != nil {
			c.fail(fmt.Errorf("the server's output broke: %w", err))
			return
		}
		var m message
		if err := json.Unmarshal(body, &m); err != nil {
			c.fail(fmt.Errorf("the server sent bad JSON: %w", err))
			return
		}
		switch {
		case m.Method != "" && m.ID != nil:
			go c.answerRequest(m) // Writing can wait for the server to read, and reading mustn't.
		case m.Method != "":
			// Notifications such as diagnostics and logs aren't used yet.
		case m.ID != nil:
			var id int64
			if json.Unmarshal(*m.ID, &id) != nil {
				continue // Not an ID of ours.
			}
			c.mu.Lock()
			ch := c.wait[id]
			delete(c.wait, id)
			c.mu.Unlock()
			if ch != nil {
				r := reply{result: m.Result}
				if m.Error != nil {
					r.err = m.Error
				}
				ch <- r
			}
		}
	}
}

// answerRequest replies to what the server asks of the client, so that it doesn't wait for us. Most
// get an empty answer; anything unknown is "method not found".
func (c *Client) answerRequest(m message) {
	var result any
	switch m.Method {
	case "workspace/configuration":
		var p struct{ Items []json.RawMessage }
		json.Unmarshal(m.Params, &p) //nolint:errcheck // No items gives an empty list.
		result = make([]any, len(p.Items))
	case "workspace/workspaceFolders":
		result = []map[string]string{{"uri": FileURI(c.Root), "name": baseName(c.Root)}}
	case "workspace/applyEdit":
		result = map[string]bool{"applied": false} // Edits come only from requests LazyCode made.
	case "client/registerCapability", "client/unregisterCapability", "window/workDoneProgress/create",
		"window/showMessageRequest", "window/showDocument", "workspace/semanticTokens/refresh",
		"workspace/inlayHint/refresh", "workspace/codeLens/refresh", "workspace/diagnostic/refresh":
		result = nil
	default:
		c.send(response{ID: m.ID, Error: &RPCError{Code: -32601, Message: "method not found"}})
		return
	}
	c.send(response{ID: m.ID, Result: result})
}

type response struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id"`
	Result  any              `json:"result"`
	Error   *RPCError        `json:"error,omitempty"`
}

func (c *Client) send(v any) error {
	var body []byte
	var err error
	if r, ok := v.(response); ok {
		r.JSONRPC = "2.0"
		body, err = json.Marshal(r)
	} else {
		body, err = json.Marshal(v)
	}
	if err != nil {
		return err
	}
	select {
	case <-c.dead:
		return c.Err() // Checked first: with room in the queue, both cases below could be ready.
	default:
	}
	select {
	case c.out <- body:
		return nil
	case <-c.dead:
		return c.Err()
	case <-time.After(requestTimeout):
		return errors.New("the server isn't reading")
	}
}

func (c *Client) notify(method string, params any) error {
	return c.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// call sends a request and waits for its answer.
func (c *Client) call(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	ch := make(chan reply, 1)
	c.mu.Lock()
	select {
	case <-c.dead:
		c.mu.Unlock()
		return nil, c.Err()
	default:
	}
	c.next++
	id := c.next
	c.wait[id] = ch
	c.mu.Unlock()
	forget := func() {
		c.mu.Lock()
		delete(c.wait, id)
		c.mu.Unlock()
	}
	if err := c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		forget()
		return nil, err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r := <-ch:
		c.mu.Lock()
		c.stuck = 0
		c.mu.Unlock()
		return r.result, r.err
	case <-c.dead:
		return nil, c.Err()
	case <-timer.C:
		forget()
		c.notify("$/cancelRequest", map[string]int64{"id": id}) //nolint:errcheck // Best effort.
		c.mu.Lock()
		c.stuck++
		giveUp := c.stuck >= maxTimeouts
		c.mu.Unlock()
		if giveUp {
			c.fail(errors.New("the server stopped answering"))
		}
		return nil, fmt.Errorf("%s didn't answer %s in time", c.Server.Name, method)
	}
}

// Sync tells the server the current text of a file: it opens it the first time, and after that
// sends the whole text again if it changed (full sync, no change tracking). It runs before each
// request, so the server answers for the same text the editor sent.
func (c *Client) Sync(path, languageID string, text []byte) error {
	uri := FileURI(path)
	sum := sha256.Sum256(text)
	c.mu.Lock()
	d, open := c.docs[uri]
	if open && d.sum == sum {
		c.mu.Unlock()
		return nil
	}
	d.version++
	d.sum = sum
	c.docs[uri] = d
	c.mu.Unlock()
	// A text that isn't UTF-8 is sent with U+FFFD for the bad bytes, as JSON does.
	if !open {
		return c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{
			"uri": uri, "languageId": languageID, "version": d.version, "text": string(text)}})
	}
	return c.notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": d.version},
		"contentChanges": []map[string]string{{"text": string(text)}},
	})
}

// CloseDoc tells the server a file is no longer open.
func (c *Client) CloseDoc(path string) {
	uri := FileURI(path)
	c.mu.Lock()
	_, open := c.docs[uri]
	delete(c.docs, uri)
	c.mu.Unlock()
	if open {
		c.notify("textDocument/didClose", map[string]any{"textDocument": map[string]string{"uri": uri}}) //nolint:errcheck // Best effort.
	}
}

// OpenDocs returns the paths of the files the server has been told about.
func (c *Client) OpenDocs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var paths []string
	for uri := range c.docs {
		if p, err := PathOf(uri); err == nil {
			paths = append(paths, p)
		}
	}
	return paths
}

// Shutdown asks the server to stop, and kills it, and anything it started, if it doesn't leave soon.
// It returns once the process is gone.
func (c *Client) Shutdown() {
	select {
	case <-c.dead:
	default:
		c.call("shutdown", nil, exitWait) //nolint:errcheck // Killed below if it doesn't go.
		c.notify("exit", nil)             //nolint:errcheck // As above.
	}
	select {
	case <-c.gone:
	case <-time.After(exitWait):
	}
	c.fail(errors.New("shut down")) // Kills it, and anything it started, if it's still there.
	<-c.gone
}

// Location is a place in a file, with the path decoded.
type Location struct {
	Path  string
	Range Range
}

// Definition returns where the symbol at pos is defined. The server may answer with a Location, a
// list of them, a list of LocationLinks, or null: all become a list, empty for null.
func (c *Client) Definition(path string, pos Position) ([]Location, error) {
	raw, err := c.call("textDocument/definition", map[string]any{
		"textDocument": map[string]string{"uri": FileURI(path)}, "position": pos}, requestTimeout)
	if err != nil {
		return nil, err
	}
	return parseLocations(raw)
}

func parseLocations(raw json.RawMessage) ([]Location, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if raw[0] == '{' {
		raw = append(append([]byte{'['}, raw...), ']')
	}
	// A Location has uri and range. A LocationLink has targetUri and targetSelectionRange.
	var items []struct {
		URI                  string `json:"uri"`
		Range                Range  `json:"range"`
		TargetURI            string `json:"targetUri"`
		TargetSelectionRange Range  `json:"targetSelectionRange"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("unexpected definition answer: %w", err)
	}
	var out []Location
	for _, it := range items {
		uri, r := it.URI, it.Range
		if it.TargetURI != "" {
			uri, r = it.TargetURI, it.TargetSelectionRange
		}
		path, err := PathOf(uri)
		if err != nil {
			continue // Not a file we can open, such as a jar: URI.
		}
		out = append(out, Location{path, r})
	}
	return out, nil
}

// TextEdit replaces a range with text.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// ErrFileOperation means a rename also wants to create, rename or delete files, which LazyCode
// doesn't do, and it applies nothing rather than part of it.
var ErrFileOperation = errors.New("it wants to create, rename or delete files too")

// Rename returns the edits that rename the symbol at pos, by path.
func (c *Client) Rename(path string, pos Position, name string) (map[string][]TextEdit, error) {
	raw, err := c.call("textDocument/rename", map[string]any{
		"textDocument": map[string]string{"uri": FileURI(path)}, "position": pos, "newName": name}, requestTimeout)
	if err != nil {
		return nil, err
	}
	return parseWorkspaceEdit(raw)
}

func parseWorkspaceEdit(raw json.RawMessage) (map[string][]TextEdit, error) {
	if s := bytes.TrimSpace(raw); len(s) == 0 || string(s) == "null" {
		return nil, nil
	}
	var we struct {
		Changes         map[string][]TextEdit `json:"changes"`
		DocumentChanges []json.RawMessage     `json:"documentChanges"`
	}
	if err := json.Unmarshal(raw, &we); err != nil {
		return nil, fmt.Errorf("unexpected rename answer: %w", err)
	}
	edits := map[string][]TextEdit{}
	add := func(uri string, list []TextEdit) error {
		path, err := PathOf(uri)
		if err != nil {
			return err
		}
		edits[path] = append(edits[path], list...)
		return nil
	}
	for uri, list := range we.Changes {
		if err := add(uri, list); err != nil {
			return nil, err
		}
	}
	for _, change := range we.DocumentChanges {
		var c struct {
			Kind         string `json:"kind"`
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			Edits []TextEdit `json:"edits"`
		}
		if err := json.Unmarshal(change, &c); err != nil {
			return nil, fmt.Errorf("unexpected rename answer: %w", err)
		}
		if c.Kind != "" { // create, rename and delete name a kind. An edit to a document doesn't.
			return nil, ErrFileOperation
		}
		if err := add(c.TextDocument.URI, c.Edits); err != nil {
			return nil, err
		}
	}
	return edits, nil
}

// Format returns the edits that format a whole file.
func (c *Client) Format(path string, tabSize int, spaces bool) ([]TextEdit, error) {
	raw, err := c.call("textDocument/formatting", map[string]any{
		"textDocument": map[string]string{"uri": FileURI(path)},
		"options":      map[string]any{"tabSize": tabSize, "insertSpaces": spaces},
	}, requestTimeout)
	if err != nil {
		return nil, err
	}
	if s := bytes.TrimSpace(raw); len(s) == 0 || string(s) == "null" {
		return nil, nil
	}
	var edits []TextEdit
	if err := json.Unmarshal(raw, &edits); err != nil {
		return nil, fmt.Errorf("unexpected format answer: %w", err)
	}
	return edits, nil
}
