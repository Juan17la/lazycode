package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

// TestMain lets the test binary play a language server: the tests start it again with LSP_FAKE set.
func TestMain(m *testing.M) {
	if mode := os.Getenv("LSP_FAKE"); mode != "" {
		fakeServer(mode)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeServer answers from the environment: LSP_FAKE_<METHOD> holds the JSON result to give. It writes
// what it receives to LSP_FAKE_LOG, one line each, so that tests can look at it.
func fakeServer(mode string) {
	log, _ := os.OpenFile(os.Getenv("LSP_FAKE_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	logf := func(format string, args ...any) {
		if log != nil {
			fmt.Fprintf(log, format+"\n", args...)
		}
	}
	in := bufio.NewReader(os.Stdin)
	out := os.Stdout
	send := func(v any) {
		body, _ := json.Marshal(v)
		writeFrame(out, body) //nolint:errcheck // The test notices.
	}
	if mode == "chatty" { // Far more than a pipe holds.
		os.Stderr.WriteString(strings.Repeat("noise\n", 1<<18)) //nolint:errcheck // Nobody may be reading.
	}
	if mode == "stubborn" {
		signalIgnore()
	}
	for {
		body, err := readFrame(in)
		if err != nil {
			return
		}
		var m message
		json.Unmarshal(body, &m) //nolint:errcheck // The test sends valid JSON.
		if m.ID == nil {
			var p struct {
				TextDocument struct {
					URI     string `json:"uri"`
					Version int    `json:"version"`
					Text    string `json:"text"`
				} `json:"textDocument"`
				ContentChanges []struct{ Text string } `json:"contentChanges"`
			}
			json.Unmarshal(m.Params, &p) //nolint:errcheck // Not every notification has these.
			text := p.TextDocument.Text
			if len(p.ContentChanges) > 0 {
				text = p.ContentChanges[0].Text
			}
			logf("%s v%d %q", m.Method, p.TextDocument.Version, text)
			if m.Method == "exit" && mode != "stubborn" {
				return
			}
			continue
		}
		logf("request %s %s", m.Method, m.Params)
		reply := func(result any) { send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": result}) }
		switch m.Method {
		case "initialize":
			switch mode {
			case "crash":
				os.Exit(3)
			case "garbage":
				out.WriteString("this is not a frame\r\n\r\n") //nolint:errcheck // The client hangs up.
				time.Sleep(time.Minute)
			case "badjson":
				writeFrame(out, []byte("{not json")) //nolint:errcheck // As above.
				time.Sleep(time.Minute)
			case "huge":
				out.WriteString("Content-Length: 99999999999\r\n\r\n") //nolint:errcheck // As above.
				time.Sleep(time.Minute)
			case "silent":
				time.Sleep(time.Minute)
			case "asks":
				// Server to client requests come before the answer, and the server waits for each.
				send(map[string]any{"jsonrpc": "2.0", "id": 100, "method": "workspace/configuration", "params": map[string]any{"items": []any{map[string]any{}, map[string]any{}}}})
				send(map[string]any{"jsonrpc": "2.0", "id": "progress-1", "method": "window/workDoneProgress/create", "params": map[string]any{"token": "t"}})
				send(map[string]any{"jsonrpc": "2.0", "id": 102, "method": "something/unknown", "params": nil})
				for range 3 {
					got, err := readFrame(in)
					if err != nil {
						return
					}
					logf("client replied %s", got)
				}
			}
			reply(map[string]any{"capabilities": map[string]any{}})
		case "shutdown":
			if mode != "stubborn" {
				reply(nil)
			}
		case "textDocument/definition", "textDocument/rename", "textDocument/formatting":
			if mode == "crashlater" {
				os.Exit(4)
			}
			if mode == "slow" {
				time.Sleep(2 * time.Second)
			}
			result := os.Getenv("LSP_FAKE_RESULT")
			if result == "" {
				result = "null"
			}
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": json.RawMessage(result)})
		default:
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{"code": -32601, "message": "no"}})
		}
	}
}

var signalIgnore = func() {} // Set on unix, in another file.

func fakeFor(t *testing.T, mode string, result string) (*Client, string, error) {
	t.Helper()
	log := filepath.Join(t.TempDir(), "log")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GORACE", "atexit_sleep_ms=0") // The race detector would otherwise hold the fake for a second at exit.
	t.Setenv("LSP_FAKE", mode)
	t.Setenv("LSP_FAKE_LOG", log)
	t.Setenv("LSP_FAKE_RESULT", result)
	root := t.TempDir()
	c, err := Start(Server{Name: "fake", Command: exe, Args: []string{"-test.run=^$"}}, root, nil)
	if c != nil {
		t.Cleanup(c.Shutdown)
	}
	return c, log, err
}

func logOf(t *testing.T, path string) string {
	t.Helper()
	data, _ := os.ReadFile(path)
	return string(data)
}

func fast(t *testing.T) {
	t.Helper()
	oldStart, oldReq, oldExit := startTimeout, requestTimeout, exitWait
	startTimeout, requestTimeout, exitWait = 2*time.Second, 400*time.Millisecond, 300*time.Millisecond
	t.Cleanup(func() { startTimeout, requestTimeout, exitWait = oldStart, oldReq, oldExit })
}

func TestDefinitionAnswers(t *testing.T) {
	loc := func(path string) string {
		return fmt.Sprintf(`{"uri":%q,"range":{"start":{"line":1,"character":2},"end":{"line":1,"character":5}}}`, FileURI(path))
	}
	a, b := filepath.FromSlash("/tmp/a b.go"), filepath.FromSlash("/tmp/b.go")
	want := Range{Position{1, 2}, Position{1, 5}}
	for name, tc := range map[string]struct {
		answer string
		want   []Location
	}{
		"Location":       {loc(a), []Location{{a, want}}},
		"Location[]":     {"[" + loc(a) + "," + loc(b) + "]", []Location{{a, want}, {b, want}}},
		"LocationLink[]": {fmt.Sprintf(`[{"targetUri":%q,"targetRange":{"start":{"line":0,"character":0},"end":{"line":9,"character":0}},"targetSelectionRange":{"start":{"line":1,"character":2},"end":{"line":1,"character":5}}}]`, FileURI(a)), []Location{{a, want}}},
		"null":           {"null", nil},
		"empty list":     {"[]", nil},
		"not a file":     {`[{"uri":"jar:file:///x.jar!/A.class","range":{"start":{"line":0,"character":0},"end":{"line":0,"character":0}}}]`, nil},
	} {
		t.Run(name, func(t *testing.T) {
			c, log, err := fakeFor(t, "ok", tc.answer)
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.Definition("/tmp/x.go", Position{3, 4})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
			if !strings.Contains(logOf(t, log), `"position":{"line":3,"character":4}`) {
				t.Errorf("the request didn't carry the position:\n%s", logOf(t, log))
			}
		})
	}
}

func TestRenameAnswers(t *testing.T) {
	uriA, uriB := FileURI("/tmp/a.go"), FileURI("/tmp/b b.go")
	edit := `{"range":{"start":{"line":0,"character":1},"end":{"line":0,"character":3}},"newText":"x"}`
	for name, tc := range map[string]struct {
		answer string
		files  int
		err    error
	}{
		"changes":         {fmt.Sprintf(`{"changes":{%q:[%s],%q:[%s,%s]}}`, uriA, edit, uriB, edit, edit), 2, nil},
		"documentChanges": {fmt.Sprintf(`{"documentChanges":[{"textDocument":{"uri":%q,"version":3},"edits":[%s]},{"textDocument":{"uri":%q,"version":null},"edits":[%s]}]}`, uriA, edit, uriB, edit), 2, nil},
		"annotated edit":  {fmt.Sprintf(`{"documentChanges":[{"textDocument":{"uri":%q},"edits":[{"range":{"start":{"line":0,"character":1},"end":{"line":0,"character":3}},"newText":"x","annotationId":"a"}]}]}`, uriA), 1, nil},
		"null":            {"null", 0, nil},
		"create a file":   {fmt.Sprintf(`{"documentChanges":[{"textDocument":{"uri":%q},"edits":[%s]},{"kind":"create","uri":%q}]}`, uriA, edit, uriB), 0, ErrFileOperation},
		"rename a file":   {fmt.Sprintf(`{"documentChanges":[{"kind":"rename","oldUri":%q,"newUri":%q}]}`, uriA, uriB), 0, ErrFileOperation},
		"delete a file":   {fmt.Sprintf(`{"documentChanges":[{"kind":"delete","uri":%q}]}`, uriA), 0, ErrFileOperation},
	} {
		t.Run(name, func(t *testing.T) {
			c, _, err := fakeFor(t, "ok", tc.answer)
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.Rename("/tmp/a.go", Position{0, 1}, "x")
			if !errors.Is(err, tc.err) {
				t.Fatalf("error %v, want %v", err, tc.err)
			}
			if len(got) != tc.files {
				t.Errorf("%d files, want %d: %v", len(got), tc.files, got)
			}
		})
	}
}

func TestFormatAnswer(t *testing.T) {
	c, log, err := fakeFor(t, "ok", `[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":2}},"newText":"  "}]`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Format("/tmp/a.go", 4, false)
	if err != nil || len(got) != 1 || got[0].NewText != "  " {
		t.Fatalf("%v %v", got, err)
	}
	if !strings.Contains(logOf(t, log), `"insertSpaces":false`) || !strings.Contains(logOf(t, log), `"tabSize":4`) {
		t.Error(logOf(t, log))
	}
}

func TestSyncOpensChangesOnceAndCloses(t *testing.T) {
	c, log, err := fakeFor(t, "ok", "null")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"one", "one", "two"} {
		if err := c.Sync("/tmp/a.go", "go", []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	c.CloseDoc("/tmp/a.go")
	c.CloseDoc("/tmp/a.go")
	if _, err := c.Definition("/tmp/a.go", Position{}); err != nil { // Everything before it has been read.
		t.Fatal(err)
	}
	got := logOf(t, log)
	for _, want := range []string{`textDocument/didOpen v1 "one"`, `textDocument/didChange v2 "two"`, "textDocument/didClose"} {
		if !strings.Contains(got, want) {
			t.Errorf("log lacks %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "didOpen") != 1 || strings.Count(got, "didChange") != 1 || strings.Count(got, "didClose") != 1 {
		t.Errorf("unchanged text or a second close was sent:\n%s", got)
	}
	if strings.Index(got, "didChange") > strings.Index(got, "request textDocument/definition") {
		t.Error("the change came after the request")
	}
}

func TestServerRequestsAreAnswered(t *testing.T) {
	fast(t)
	c, log, err := fakeFor(t, "asks", "null")
	if err != nil {
		t.Fatal(err)
	}
	_ = c
	got := logOf(t, log)
	for _, want := range []string{`"id":100,"result":[null,null]`, `"id":"progress-1","result":null`, `"id":102`, `"code":-32601`} {
		if !strings.Contains(got, want) {
			t.Errorf("reply lacks %s:\n%s", want, got)
		}
	}
}

func TestBrokenServers(t *testing.T) {
	fast(t)
	for _, mode := range []string{"crash", "garbage", "badjson", "huge", "silent"} {
		t.Run(mode, func(t *testing.T) {
			start := time.Now()
			c, _, err := fakeFor(t, mode, "")
			if err == nil {
				t.Fatalf("%s started", mode)
			}
			if c != nil {
				t.Error("a client came with the error")
			}
			if time.Since(start) > 10*time.Second {
				t.Errorf("took %v", time.Since(start))
			}
		})
	}
}

func TestACrashLaterEndsRequestsAndMarksItDead(t *testing.T) {
	fast(t)
	c, _, err := fakeFor(t, "crashlater", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Definition("/tmp/a.go", Position{}); err == nil {
		t.Fatal("no error from a server that died")
	}
	select {
	case <-c.Dead():
	case <-time.After(2 * time.Second):
		t.Fatal("not marked dead")
	}
	if _, err := c.Definition("/tmp/a.go", Position{}); err == nil {
		t.Error("a dead server answered")
	}
	if err := c.Sync("/tmp/a.go", "go", []byte("x")); err == nil {
		t.Error("a dead server took a change")
	}
}

func TestSlowRepliesTimeOutAndRepeatedOnesKillIt(t *testing.T) {
	fast(t)
	c, _, err := fakeFor(t, "slow", "null")
	if err != nil {
		t.Fatal(err)
	}
	for i := range maxTimeouts {
		start := time.Now()
		if _, err := c.Definition("/tmp/a.go", Position{}); err == nil || !strings.Contains(err.Error(), "in time") && i < maxTimeouts-1 {
			t.Fatalf("attempt %d: %v", i, err)
		}
		if time.Since(start) > 2*time.Second {
			t.Fatalf("attempt %d took %v", i, time.Since(start))
		}
	}
	select {
	case <-c.Dead():
	case <-time.After(2 * time.Second):
		t.Fatal("a server that never answers wasn't given up on")
	}
}

func TestChattyStderrDoesntBlock(t *testing.T) {
	fast(t)
	c, _, err := fakeFor(t, "chatty", "null")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Definition("/tmp/a.go", Position{}); err != nil {
		t.Fatal(err)
	}
}

func TestMissingServer(t *testing.T) {
	_, err := Start(Server{Name: "none", Command: "lazycode-no-such-server"}, t.TempDir(), nil)
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("%v", err)
	}
}

func TestShutdownLeavesNoProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signals")
	}
	fast(t)
	for _, mode := range []string{"ok", "stubborn"} {
		c, _, err := fakeFor(t, mode, "null")
		if err != nil {
			t.Fatal(err)
		}
		pid := c.cmd.Process.Pid
		start := time.Now()
		c.Shutdown()
		if time.Since(start) > 5*time.Second {
			t.Errorf("%s: shutdown took %v", mode, time.Since(start))
		}
		if alive(pid) {
			t.Errorf("%s: process %d is still there", mode, pid)
		}
		c.Shutdown() // Twice is fine.
	}
}

func TestFrames(t *testing.T) {
	read := func(s string) ([]byte, error) { return readFrame(bufio.NewReader(strings.NewReader(s))) }
	if body, err := read("Content-Length: 2\r\n\r\n{}"); err != nil || string(body) != "{}" {
		t.Errorf("%q %v", body, err)
	}
	if body, err := read("content-length: 2\ncontent-type: x\n\n{}"); err != nil || string(body) != "{}" {
		t.Errorf("bare newlines and a second header: %q %v", body, err)
	}
	if _, err := read(""); err != io.EOF {
		t.Errorf("an empty stream is a clean end, got %v", err)
	}
	for name, input := range map[string]string{
		"no length":         "X: 1\r\n\r\n{}",
		"negative":          "Content-Length: -1\r\n\r\n",
		"not a number":      "Content-Length: abc\r\n\r\n",
		"too big":           fmt.Sprintf("Content-Length: %d\r\n\r\n", maxMessage+1),
		"huge number":       "Content-Length: 99999999999999999999\r\n\r\n",
		"short body":        "Content-Length: 10\r\n\r\n{}",
		"no colon":          "garbage\r\n\r\n",
		"long header":       strings.Repeat("A", maxHeaderLine+10) + "\r\n\r\n",
		"header never ends": "Content-Length: 2",
	} {
		if body, err := read(input); err == nil || err == io.EOF {
			t.Errorf("%s: %q, %v", name, body, err)
		}
	}
	var buf bytes.Buffer
	writeFrame(&buf, []byte(`{"a":1}`)) //nolint:errcheck // A buffer.
	if buf.String() != "Content-Length: 7\r\n\r\n{\"a\":1}" {
		t.Errorf("%q", buf.String())
	}
}

func FuzzReadFrame(f *testing.F) {
	f.Add([]byte("Content-Length: 2\r\n\r\n{}"))
	f.Add([]byte("Content-Length: 5\r\nContent-Type: x\r\n\r\nabc"))
	f.Add([]byte("\r\n\r\n"))
	f.Add([]byte("Content-Length: 99999999999\r\n\r\n"))
	f.Add([]byte(strings.Repeat("A", 9000)))
	f.Fuzz(func(t *testing.T, data []byte) {
		r := bufio.NewReader(bytes.NewReader(data))
		for range 5 { // A stream of frames, to hit what follows one.
			body, err := readFrame(r)
			if err != nil {
				return
			}
			if len(body) > maxMessage || len(body) > len(data) {
				t.Fatalf("a %d byte body from %d bytes", len(body), len(data))
			}
		}
	})
}

func TestURIs(t *testing.T) {
	for _, tc := range []struct {
		path, uri string
		windows   bool
	}{
		{"/home/u/a b/x.go", "file:///home/u/a%20b/x.go", false},
		{"/tmp/100%/#1?.go", "file:///tmp/100%25/%231%3F.go", false},
		{"/tmp/日本/é.go", "file:///tmp/%E6%97%A5%E6%9C%AC/%C3%A9.go", false},
		{`C:\Users\a b\x.go`, "file:///C:/Users/a%20b/x.go", true},
		{`D:\x.go`, "file:///D:/x.go", true},
	} {
		if got := fileURI(tc.path, tc.windows); got != tc.uri {
			t.Errorf("%s: %s, want %s", tc.path, got, tc.uri)
		}
		if back, err := pathOf(tc.uri, tc.windows); err != nil || back != tc.path {
			t.Errorf("%s: back to %q, %v", tc.uri, back, err)
		}
	}
	// What servers write: a lowercase drive with its colon escaped, and localhost.
	if p, err := pathOf("file:///c%3A/Users/a%20b/x.go", true); err != nil || p != `c:\Users\a b\x.go` {
		t.Errorf("%q %v", p, err)
	}
	if p, err := pathOf("file://localhost/tmp/x.go", false); err != nil || p != "/tmp/x.go" {
		t.Errorf("%q %v", p, err)
	}
	for _, bad := range []string{"http://x/y", "file://otherhost/x", "%zz", "jar:file:///x.jar!/A", ""} {
		if p, err := pathOf(bad, false); err == nil {
			t.Errorf("%q gave %q", bad, p)
		}
	}
}

func TestPositionsInUTF16(t *testing.T) {
	// Each line: text, and the byte offset and UTF-16 character of the character "X" on it.
	for _, tc := range []struct {
		text         string
		byteAt, unit int
	}{
		{"abcX", 3, 3},
		{"😀X", 4, 2},   // An emoji is two units.
		{"你好X", 6, 2},  // CJK is one unit and three bytes.
		{"é😀你X", 9, 4}, // 2 + 4 + 3 bytes, 1 + 2 + 1 units.
		{"\tX", 1, 1},
		{"👨‍👩‍👧X", 18, 8}, // A joined family: seven code points, 8 units, 18 bytes.
	} {
		doc := NewDoc([]byte("first\n" + tc.text + "\nlast"))
		line := 1
		offset := len("first\n") + tc.byteAt
		if got := doc.Position(offset); got != (Position{line, tc.unit}) {
			t.Errorf("%q: position %+v, want %d,%d", tc.text, got, line, tc.unit)
		}
		if got := doc.Offset(Position{line, tc.unit}); got != offset {
			t.Errorf("%q: offset %d, want %d", tc.text, got, offset)
		}
		// Against an independent count of the units before the X.
		if n := len(utf16.Encode([]rune(tc.text[:tc.byteAt]))); n != tc.unit {
			t.Errorf("%q: test data wrong, %d units", tc.text, n)
		}
	}
}

func TestPositionEdges(t *testing.T) {
	doc := NewDoc([]byte("ab\r\ncd\n\nlast"))
	for pos, want := range map[Position]int{
		{0, 0}: 0, {0, 2}: 2,
		{0, 99}: 2, // Past the end of a line is its end, before the \r.
		{1, 1}:  5, {1, 99}: 6,
		{2, 0}: 7, {3, 4}: 12, {3, 99}: 12,
		{9, 0}:  12, // Past the last line is the end of the text.
		{-1, 0}: 0,
	} {
		if got := doc.Offset(pos); got != want {
			t.Errorf("offset of %+v is %d, want %d", pos, got, want)
		}
	}
	if got := doc.Position(3); got != (Position{0, 2}) { // Between \r and \n.
		t.Errorf("%+v", got)
	}
	if got := doc.Position(4); got != (Position{1, 0}) {
		t.Errorf("%+v", got)
	}
	for _, off := range []int{-5, 0, 12, 400} {
		doc.Position(off) // Out of range must not panic.
	}
	// Inside a character counts as its start, and bad bytes count one unit each.
	d := NewDoc([]byte("é\xff\xfeX"))
	if got := d.Position(1); got.Character != 0 {
		t.Errorf("%+v", got)
	}
	if got := d.Position(4); got.Character != 3 || d.Offset(Position{0, 3}) != 4 {
		t.Errorf("%+v", got)
	}
	empty := NewDoc(nil)
	if empty.Offset(Position{3, 3}) != 0 || empty.Position(5) != (Position{}) {
		t.Error("an empty text")
	}
}

func TestRoot(t *testing.T) {
	boundary := t.TempDir()
	sub := filepath.Join(boundary, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(boundary, "a", "go.mod"), nil, 0o644) //nolint:errcheck // Checked by the result.
	file := filepath.Join(sub, "x.go")
	if got := Root(gopls, file, boundary); got != filepath.Join(boundary, "a") {
		t.Errorf("the nearest marker: %s", got)
	}
	if got := Root(pyright, file, boundary); got != boundary {
		t.Errorf("no marker: %s", got)
	}
	elsewhere := filepath.Join(t.TempDir(), "y.go")
	if got := Root(gopls, elsewhere, boundary); got != boundary {
		t.Errorf("a file outside the directory: %s", got)
	}
}

func TestForPath(t *testing.T) {
	for path, want := range map[string]string{"a.go": "go", "a.py": "python", "a.ts": "typescript", "a.tsx": "typescriptreact", "a.js": "javascript", "a.mjs": "javascript", "a.jsx": "javascriptreact", "a.rs": "", "a": ""} {
		if _, id, _ := ForPath(path); id != want {
			t.Errorf("%s: %q, want %q", path, id, want)
		}
	}
	srv, _, _ := ForPath("a.ts")
	if srv.Command != "typescript-language-server" || !slices.Equal(srv.Args, []string{"--stdio"}) {
		t.Errorf("%+v", srv)
	}
	srv, _, _ = ForPath("a.py")
	if srv.Command != "pyright-langserver" || !slices.Equal(srv.Args, []string{"--stdio"}) {
		t.Errorf("%+v", srv)
	}
}
