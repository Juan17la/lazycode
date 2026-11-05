package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Juan17la/lazycode/internal/keymap"
	"github.com/gdamore/tcell/v2"
)

// fakeLSPConfig scripts the fake gopls: what it answers, and when it misbehaves.
type fakeLSPConfig struct {
	Log        string          `json:"log"`
	Definition json.RawMessage `json:"definition"`
	Rename     json.RawMessage `json:"rename"`
	Formatting json.RawMessage `json:"formatting"`
	DelayMS    int             `json:"delay_ms"`
	CrashOn    string          `json:"crash_on"` // A method: the server exits when asked it.
}

// runFakeLSP plays a language server for the tests, started by the gopls script in withFakeGopls.
func runFakeLSP(path string) {
	data, _ := os.ReadFile(path)
	var cfg fakeLSPConfig
	json.Unmarshal(data, &cfg) //nolint:errcheck // The test wrote it.
	log, _ := os.OpenFile(cfg.Log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	fmt.Fprintf(log, "start pid %d cwd %s\n", os.Getpid(), mustGetwd())
	in := bufio.NewReader(os.Stdin)
	send := func(v any) {
		body, _ := json.Marshal(v)
		fmt.Fprintf(os.Stdout, "Content-Length: %d\r\n\r\n%s", len(body), body)
	}
	for {
		length := -1
		for {
			line, err := in.ReadString('\n')
			if err != nil {
				os.Exit(0)
			}
			line = strings.TrimSpace(line)
			if line == "" {
				break
			}
			if v, ok := strings.CutPrefix(line, "Content-Length: "); ok {
				length, _ = strconv.Atoi(v)
			}
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(in, body); err != nil {
			os.Exit(0)
		}
		var m struct {
			ID     *json.RawMessage `json:"id"`
			Method string           `json:"method"`
			Params json.RawMessage  `json:"params"`
		}
		json.Unmarshal(body, &m) //nolint:errcheck // The client sends valid JSON.
		fmt.Fprintf(log, "%s %s\n", m.Method, m.Params)
		if m.Method == "exit" {
			os.Exit(0)
		}
		if m.ID == nil {
			continue
		}
		if strings.HasSuffix(m.Method, cfg.CrashOn) && cfg.CrashOn != "" {
			os.Exit(7)
		}
		var result json.RawMessage = []byte("null")
		switch m.Method {
		case "initialize":
			result = []byte(`{"capabilities":{}}`)
		case "textDocument/definition":
			time.Sleep(time.Duration(cfg.DelayMS) * time.Millisecond)
			result = cfg.Definition
		case "textDocument/rename":
			result = cfg.Rename
		case "textDocument/formatting":
			result = cfg.Formatting
		}
		if len(result) == 0 {
			result = []byte("null")
		}
		send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": result})
	}
}

// withFakeGopls puts a gopls on the PATH that runs the fake, and returns the log it writes.
func withFakeGopls(t *testing.T, cfg fakeLSPConfig) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake server is a shell script")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	cfg.Log = filepath.Join(bin, "log")
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(bin, "config.json")
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf("#!/bin/sh\nLAZYCODE_TEST_LSP=%q exec %q -test.run=^$\n", cfgPath, exe)
	if err := os.WriteFile(filepath.Join(bin, "gopls"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return cfg.Log
}

func readLog(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

func waitLog(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(readLog(path), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the server never saw %q:\n%s", want, readLog(path))
}

func pidIn(t *testing.T, log string) int {
	t.Helper()
	var pid int
	fmt.Sscanf(readLog(log)[strings.Index(readLog(log), "start pid"):], "start pid %d", &pid) //nolint:errcheck // Checked below.
	if pid == 0 {
		t.Fatalf("no pid in the log:\n%s", readLog(log))
	}
	return pid
}

func waitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if processGone(pid) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d is still running", pid)
}

func (u *ui) trustFolder() { u.run("trust") }

const lspMain = "package main\n\nfunc main() { Hello() }\n"

func lspUI(t *testing.T, cfg fakeLSPConfig, files map[string]string) (*ui, string) {
	t.Helper()
	log := withFakeGopls(t, cfg)
	if files == nil {
		files = map[string]string{"main.go": lspMain}
	}
	return start(t, files), log
}

func TestUntrustedFolderStartsNoServer(t *testing.T) {
	u, log := lspUI(t, fakeLSPConfig{}, nil)
	u.open("main.go")
	time.Sleep(200 * time.Millisecond)
	if _, err := os.Stat(log); err == nil {
		t.Fatal("opening a file started a server")
	}
	u.post(tcell.NewEventKey(tcell.KeyF12, 0, 0))
	u.wait("Run gopls in work?")
	u.press("c")
	time.Sleep(200 * time.Millisecond)
	if _, err := os.Stat(log); err == nil {
		t.Fatal("a server started after the question was refused")
	}
	// Format and rename ask too.
	u.key(tcell.KeyF2)
	u.wait("Rename to:")
	u.press("X")
	u.key(tcell.KeyEnter)
	u.wait("Run gopls in work?")
	u.press("c")
	u.post(tcell.NewEventKey(tcell.KeyRune, 'F', tcell.ModAlt))
	u.wait("Run gopls in work?")
	u.press("c")
	if _, err := os.Stat(log); err == nil {
		t.Fatal("a server started without trust")
	}
	// Once starts it, in the project's folder, and the answer comes back.
	u.post(tcell.NewEventKey(tcell.KeyF12, 0, 0))
	u.wait("Run gopls in work?")
	u.press("o")
	waitLog(t, log, "textDocument/definition")
	waitLog(t, log, "cwd "+u.dir)
	u.wait("No definition found")
}

func TestServerIsStartedByRequestsOnlyInTrustedFolders(t *testing.T) {
	u, log := lspUI(t, fakeLSPConfig{}, nil)
	u.trustFolder()
	u.open("main.go")
	time.Sleep(150 * time.Millisecond)
	if _, err := os.Stat(log); err == nil {
		t.Fatal("a server started on opening a file, even in a trusted folder")
	}
	u.post(tcell.NewEventKey(tcell.KeyF12, 0, 0))
	waitLog(t, log, "textDocument/didOpen")
}

func lspLocation(path string, line, char int) string {
	return fmt.Sprintf(`{"uri":%q,"range":{"start":{"line":%d,"character":%d},"end":{"line":%d,"character":%d}}}`,
		"file://"+path, line, char, line, char+1)
}

func TestDefinitionOpensTheFileAtTheRightColumn(t *testing.T) {
	u, _ := lspUI(t, fakeLSPConfig{}, map[string]string{"main.go": lspMain, "other.go": "package main\n\n😀é你X\n"})
	other := filepath.Join(u.dir, "other.go")
	// The server counts UTF-16: the emoji is 2 units, é 1, 你 1, so X is at 4. On screen it's column 6.
	withFakeGoplsResult(t, u, "definition", lspLocation(other, 2, 4))
	u.trustFolder()
	u.open("main.go")
	u.post(tcell.NewEventKey(tcell.KeyF12, 0, 0))
	u.wait("Ln 3, Col 6")
}

// withFakeGoplsResult rewrites the fake's config, for answers that name the test's own paths.
func withFakeGoplsResult(t *testing.T, u *ui, field, value string) {
	t.Helper()
	path, err := exec.LookPath("gopls")
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(filepath.Dir(path), "config.json")
	var cfg map[string]json.RawMessage
	data, _ := os.ReadFile(cfgPath)
	json.Unmarshal(data, &cfg) //nolint:errcheck // Written by withFakeGopls.
	cfg[field] = json.RawMessage(value)
	data, _ = json.Marshal(cfg)
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDefinitionOutsideTheDirectoryOpens(t *testing.T) {
	u, _ := lspUI(t, fakeLSPConfig{}, nil)
	outside := filepath.Join(filepath.Dir(u.dir), "elsewhere.go")
	if err := os.WriteFile(outside, []byte("package x\n\nfunc Hello() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withFakeGoplsResult(t, u, "definition", "["+lspLocation(outside, 2, 5)+"]")
	u.trustFolder()
	u.open("main.go")
	u.post(tcell.NewEventKey(tcell.KeyF12, 0, 0))
	u.wait("Ln 3, Col 6")
	u.wait("package x")
}

func TestFormatIsOneUndoStep(t *testing.T) {
	u, log := lspUI(t, fakeLSPConfig{}, map[string]string{"main.go": "package main\n\nfunc  main( ) {\n}\n"})
	withFakeGoplsResult(t, u, "formatting", `[
		{"range":{"start":{"line":2,"character":4},"end":{"line":2,"character":6}},"newText":" "},
		{"range":{"start":{"line":2,"character":11},"end":{"line":2,"character":12}},"newText":""}]`)
	u.trustFolder()
	u.open("main.go")
	u.post(tcell.NewEventKey(tcell.KeyRune, 'F', tcell.ModAlt))
	u.wait("func main() {")
	u.wait("Formatted with gopls")
	waitLog(t, log, "textDocument/formatting")
	u.key(tcell.KeyCtrlZ) // One step brings back both edits.
	u.wait("func  main( ) {")
	// The second form of the same key, for terminals where Alt types characters.
	u.post(tcell.NewEventKey(tcell.KeyCtrlK, 0, 0))
	u.press("F")
	u.wait("func main() {")
}

func TestRenameShowsTheChangeThenApplies(t *testing.T) {
	u, _ := lspUI(t, fakeLSPConfig{}, map[string]string{"main.go": "package main\n\nfunc main() { Hello() }\n", "other.go": "package main\n\nfunc Hello() {}\n"})
	main, other := filepath.Join(u.dir, "main.go"), filepath.Join(u.dir, "other.go")
	edit := func(line, char int) string {
		return fmt.Sprintf(`{"range":{"start":{"line":%d,"character":%d},"end":{"line":%d,"character":%d}},"newText":"Hi"}`, line, char, line, char+5)
	}
	withFakeGoplsResult(t, u, "rename", fmt.Sprintf(`{"documentChanges":[
		{"textDocument":{"uri":%q,"version":1},"edits":[%s]},
		{"textDocument":{"uri":%q,"version":null},"edits":[%s]}]}`, "file://"+main, edit(2, 14), "file://"+other, edit(2, 5)))
	u.trustFolder()
	u.open("main.go")
	u.press("") // Put the cursor on Hello.
	for range len("package main\n\nfunc main() { ") {
		u.key(tcell.KeyRight)
	}
	u.key(tcell.KeyF2)
	u.wait("Rename to: Hello")
	for range len("Hello") {
		u.key(tcell.KeyBackspace2)
	}
	u.press("Hi")
	u.key(tcell.KeyEnter)
	u.wait("Rename to Hi") // The review, before anything changes.
	if got := u.file("other.go"); !strings.Contains(got, "Hello") {
		t.Fatalf("changed before confirming: %q", got)
	}
	u.key(tcell.KeyEnter)
	u.waitFile("other.go", "package main\n\nfunc Hi() {}\n")
	u.waitFile("main.go", "package main\n\nfunc main() { Hi() }\n")
	u.wait("Renamed in 2 files")
}

func TestRenameWithFileOperationsAppliesNothing(t *testing.T) {
	u, _ := lspUI(t, fakeLSPConfig{}, map[string]string{"main.go": "package main\n\nfunc main() { Hello() }\n"})
	main := filepath.Join(u.dir, "main.go")
	withFakeGoplsResult(t, u, "rename", fmt.Sprintf(`{"documentChanges":[
		{"textDocument":{"uri":%q,"version":1},"edits":[{"range":{"start":{"line":2,"character":14},"end":{"line":2,"character":19}},"newText":"Hi"}]},
		{"kind":"rename","oldUri":%q,"newUri":%q}]}`, "file://"+main, "file://"+main, "file://"+main+".bak"))
	u.trustFolder()
	u.open("main.go")
	for range len("package main\n\nfunc main() { ") {
		u.key(tcell.KeyRight)
	}
	u.key(tcell.KeyF2)
	u.wait("Rename to: Hello")
	u.press("1")
	u.key(tcell.KeyEnter)
	u.wait("also wants to create, rename or delete files")
	if got := u.file("main.go"); !strings.Contains(got, "Hello()") {
		t.Errorf("part of it was applied: %q", got)
	}
}

func TestRenameOfAnEditedFileAsksToSaveFirst(t *testing.T) {
	u, log := lspUI(t, fakeLSPConfig{}, nil)
	u.trustFolder()
	u.open("main.go")
	u.press("x")
	u.key(tcell.KeyF2)
	u.wait("Save main.go first")
	if strings.Contains(readLog(log), "rename") {
		t.Error("a rename was asked of an unsaved file")
	}
}

func TestAnswerToAChangedBufferIsRefused(t *testing.T) {
	u, _ := lspUI(t, fakeLSPConfig{DelayMS: 500}, map[string]string{"main.go": lspMain, "other.go": "package main\n\nvar X = 1\n"})
	withFakeGoplsResult(t, u, "definition", lspLocation(filepath.Join(u.dir, "other.go"), 2, 4))
	u.trustFolder()
	u.open("main.go")
	u.post(tcell.NewEventKey(tcell.KeyF12, 0, 0))
	time.Sleep(100 * time.Millisecond)
	u.press("z") // Typing while the server thinks.
	u.wait("changed while the language server was answering")
	u.press("y")          // Clears the message.
	u.wait("Ln 1, Col 3") // Still in main.go, after the z and y.
}

func TestMissingServerSaysHowToGetItAndEditingGoesOn(t *testing.T) {
	u := start(t, map[string]string{"main.go": lspMain})
	t.Setenv("PATH", t.TempDir()) // No gopls, and nothing else either.
	u.open("main.go")
	u.post(tcell.NewEventKey(tcell.KeyF12, 0, 0))
	u.wait("gopls isn't installed")
	u.wait("go install golang.org/x/tools/gopls@latest")
	u.press("ok")
	u.wait("okpackage main")
}

func TestUnknownLanguageHasNoServer(t *testing.T) {
	u := start(t, map[string]string{"notes.txt": "x"})
	u.open("notes.txt")
	u.post(tcell.NewEventKey(tcell.KeyF12, 0, 0))
	u.wait("No language server for this kind of file")
}

func TestACrashingServerIsRestartedUpToALimit(t *testing.T) {
	u, log := lspUI(t, fakeLSPConfig{CrashOn: "definition"}, nil)
	u.trustFolder()
	u.open("main.go")
	for i := range maxServerCrashes {
		u.post(tcell.NewEventKey(tcell.KeyF12, 0, 0))
		u.wait("gopls stopped")
		if got := strings.Count(readLog(log), "start pid"); got != i+1 {
			t.Fatalf("after %d crashes the server has started %d times", i+1, got)
		}
		u.press("q") // Editing carries on.
		u.key(tcell.KeyBackspace2)
		u.waitGone("gopls stopped")
	}
	u.post(tcell.NewEventKey(tcell.KeyF12, 0, 0))
	u.wait("keeps stopping here")
	if got := strings.Count(readLog(log), "start pid"); got != maxServerCrashes {
		t.Errorf("it started %d times", got)
	}
}

func TestServersStopWithTheProjectAndTheApp(t *testing.T) {
	u, log := lspUI(t, fakeLSPConfig{}, nil)
	u.trustFolder()
	u.open("main.go")
	u.post(tcell.NewEventKey(tcell.KeyF12, 0, 0))
	waitLog(t, log, "textDocument/definition")
	pid := pidIn(t, log)

	u.key(tcell.KeyEsc)
	u.press(":project close")
	u.key(tcell.KeyEnter)
	waitGone(t, pid)
	if !strings.Contains(readLog(log), "shutdown") {
		t.Error("the server wasn't asked to shut down")
	}

	// Starts again, and the app quitting takes it too.
	u.open("main.go")
	u.post(tcell.NewEventKey(tcell.KeyF12, 0, 0))
	waitLog(t, log, "textDocument/didOpen")
	time.Sleep(100 * time.Millisecond)
	text := readLog(log)
	second := strings.LastIndex(text, "start pid")
	var pid2 int
	fmt.Sscanf(text[second:], "start pid %d", &pid2) //nolint:errcheck // Checked below.
	if pid2 == 0 || pid2 == pid {
		t.Fatalf("no second server: %q", text)
	}
	u.quitNow()
	waitGone(t, pid2)
}

func TestClosedFilesAreClosedOnTheServer(t *testing.T) {
	u, log := lspUI(t, fakeLSPConfig{}, nil)
	u.trustFolder()
	u.open("main.go")
	u.post(tcell.NewEventKey(tcell.KeyF12, 0, 0))
	waitLog(t, log, "textDocument/didOpen")
	u.key(tcell.KeyCtrlW)
	waitLog(t, log, "textDocument/didClose")
}

func TestEditsAreSentWholeBeforeARequest(t *testing.T) {
	u, log := lspUI(t, fakeLSPConfig{}, map[string]string{"main.go": "a\r\nb\r\n"})
	u.trustFolder()
	u.open("main.go")
	u.post(tcell.NewEventKey(tcell.KeyF12, 0, 0))
	waitLog(t, log, "didOpen")
	u.press("x")
	u.post(tcell.NewEventKey(tcell.KeyF12, 0, 0))
	waitLog(t, log, "didChange")
	if got := readLog(log); !strings.Contains(got, `"text":"xa\r\nb\r\n"`) {
		t.Errorf("the text with CRLF wasn't sent whole:\n%s", got)
	}
}

func TestDefaultKeysAndCommands(t *testing.T) {
	m := keymap.Default()
	for key, want := range map[string]string{"f12": "lsp.definition", "f2": "lsp.rename", "alt+shift+f": "lsp.format"} {
		if got := m.Lookup(keymap.Editor, key); got != want {
			t.Errorf("%s is %q, want %q", key, got, want)
		}
	}
	if got := m.LookupChord(keymap.Editor, "F"); got != "lsp.format" {
		t.Errorf("Ctrl+K then Shift+F is %q", got)
	}
	u := start(t, map[string]string{"notes.txt": "x"})
	u.open("notes.txt")
	for _, cmd := range []string{"definition", "rename", "format"} {
		u.key(tcell.KeyEsc)
		u.press(":" + cmd)
		u.key(tcell.KeyEnter)
		u.wait("No language server for this kind of file")
		u.press("x") // Clears the message, and types.
	}
}

// The real thing, if it's installed: F12, rename and format against gopls itself.
func TestRealGoplsThroughTheUI(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls isn't installed")
	}
	if testing.Short() {
		t.Skip("starts gopls")
	}
	home := t.TempDir()
	for _, name := range []string{"GOCACHE", "GOPATH"} {
		t.Setenv(name, filepath.Join(home, name))
	}
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOTOOLCHAIN", "local")
	src := "package main\n\nfunc Greet() string { return \"héllo 😀\" }\n\nfunc main() {\n\tx := Greet()  \n\t_ = x\n}\n"
	u := start(t, map[string]string{"go.mod": "module example.com/t\n\ngo 1.21\n", "main.go": src})
	u.trustFolder()
	u.open("main.go")
	for range 5 { // Down to the line of the call, then onto Greet.
		u.key(tcell.KeyDown)
	}
	for range len("\tx := ") {
		u.key(tcell.KeyRight)
	}
	u.key(tcell.KeyF12)
	u.wait("Ln 3, Col 6")

	u.key(tcell.KeyF2)
	u.wait("Rename to: Greet")
	u.press("Hello")
	u.key(tcell.KeyEnter)
	u.wait("Rename to GreetHello")
	u.key(tcell.KeyEnter)
	u.waitFile("main.go", strings.ReplaceAll(src, "Greet", "GreetHello"))

	u.post(tcell.NewEventKey(tcell.KeyRune, 'F', tcell.ModAlt))
	u.wait("Formatted with gopls")
	u.key(tcell.KeyCtrlS)
	u.waitFile("main.go", strings.ReplaceAll(strings.ReplaceAll(src, "Greet", "GreetHello"), "Greet()  \n", "Greet()\n"))
}
