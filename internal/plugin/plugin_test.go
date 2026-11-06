package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeBuffer struct{ text string }

func (b *fakeBuffer) Path() string { return "/x/a.txt" }
func (b *fakeBuffer) Len() int     { return len(b.text) }
func (b *fakeBuffer) Text(s, e int) (string, error) {
	if s < 0 || e < s || e > len(b.text) {
		return "", fmt.Errorf("%d..%d is outside the text", s, e)
	}
	return b.text[s:e], nil
}
func (b *fakeBuffer) Replace(s, e int, text string) error {
	if _, err := b.Text(s, e); err != nil {
		return err
	}
	b.text = b.text[:s] + text + b.text[e:]
	return nil
}

func write(t *testing.T, main string, apiVersion int) string {
	t.Helper()
	dir := t.TempDir()
	manifest := fmt.Sprintf("name = \"demo\"\nversion = \"1.0.0\"\napi_version = %d\ndoes = \"Demo\"\n", apiVersion)
	if err := os.WriteFile(filepath.Join(dir, "plugin.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.lua"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func load(t *testing.T, main string, host Host) *Plugin {
	t.Helper()
	p, err := Load(write(t, main, APIVersion), host)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

// The example from doc 09: a command, an event, a key, and an edit.
func TestTrimPlugin(t *testing.T) {
	buf := &fakeBuffer{text: "one   \ntwo\t\n"}
	var messages []string
	p := load(t, `
local lc = require("lazycode")
local function trim(b)
  local text = b:text()
  local clean = text:gsub("[ \t]+\n", "\n")
  if clean ~= text then b:replace(1, b:len(), clean) end
end
lc.command("trim", function() trim(lc.current_buffer()); lc.message("Trimmed " .. lc.current_buffer():path()) end)
lc.on("buffer_save", trim)
lc.key("ctrl+alt+t", "trim")
`, Host{Message: func(s string) { messages = append(messages, s) }, Buffer: func() Buffer { return buf }})

	if !p.Run("trim") || buf.text != "one\ntwo\n" || p.Err != nil {
		t.Fatalf("after :trim: %q, %v", buf.text, p.Err)
	}
	if len(messages) != 1 || messages[0] != "Trimmed /x/a.txt" {
		t.Errorf("messages = %q", messages)
	}
	if p.Run("other") {
		t.Error("ran a command it doesn't have")
	}
	if p.Keys()["ctrl+alt+t"] != "trim" {
		t.Errorf("keys = %v", p.Keys())
	}
	other := &fakeBuffer{text: "x \n"}
	p.Emit("buffer_save", other)
	if other.text != "x\n" {
		t.Errorf("after the event: %q", other.text)
	}
}

func TestPositionsWorkLikeStringSub(t *testing.T) {
	buf := &fakeBuffer{text: "abcdef"}
	p := load(t, `
local lc = require("lazycode")
lc.command("go", function()
  local b = lc.current_buffer()
  assert(b:text(2, 3) == "bc")
  b:replace(1, 0, "X")   -- Inserts.
  b:replace(2, 2, "")    -- Removes the a.
end)`, Host{Buffer: func() Buffer { return buf }})
	p.Run("go")
	if p.Err != nil || buf.text != "Xbcdef" {
		t.Errorf("%q, %v", buf.text, p.Err)
	}
}

func TestAnErrorDisablesOnlyThePlugin(t *testing.T) {
	var message string
	buf := &fakeBuffer{text: "abc"}
	p := load(t, `
local lc = require("lazycode")
local runs = 0
lc.command("boom", function() runs = runs + 1; lc.message("run " .. runs); error("broken") end)
lc.command("outside", function() lc.current_buffer():replace(1, 99, "x") end)
`, Host{Message: func(s string) { message = s }, Buffer: func() Buffer { return buf }})
	p.Run("boom")
	if p.Err == nil || !strings.Contains(message, "broken") {
		t.Fatalf("err %v, message %q", p.Err, message)
	}
	p.Run("boom")
	if strings.Contains(message, "run 2") {
		t.Error("a disabled plugin ran again")
	}
	if buf.text != "abc" {
		t.Errorf("text changed: %q", buf.text)
	}
}

func TestABadEditIsAnError(t *testing.T) {
	buf := &fakeBuffer{text: "abc"}
	p := load(t, `
local lc = require("lazycode")
lc.command("outside", function() lc.current_buffer():replace(1, 99, "x") end)
`, Host{Buffer: func() Buffer { return buf }})
	p.Run("outside")
	if p.Err == nil || buf.text != "abc" {
		t.Errorf("%q, %v", buf.text, p.Err)
	}
}

func TestSlowCallsAreStoppedThenDisabled(t *testing.T) {
	p := load(t, `
local lc = require("lazycode")
lc.command("spin", function() while true do end end)
`, Host{})
	p.limit = 20 * time.Millisecond
	for i := range maxSlow {
		if p.Err != nil {
			t.Fatalf("disabled after %d calls", i)
		}
		start := time.Now()
		p.Run("spin")
		if d := time.Since(start); d > 2*time.Second {
			t.Fatalf("not stopped: %v", d)
		}
	}
	if p.Err == nil {
		t.Error("still enabled")
	}
}

func TestLoadRefuses(t *testing.T) {
	if _, err := Load(write(t, "", 99), Host{}); err == nil || !strings.Contains(err.Error(), "API 99") {
		t.Errorf("wrong API version: %v", err)
	}
	for name, main := range map[string]string{
		"syntax":  "this is not lua",
		"error":   `error("nope")`,
		"event":   `require("lazycode").on("nothing", function() end)`,
		"command": `require("lazycode").command("two words", function() end)`,
		"spin":    `while true do end`,
	} {
		if _, err := Load(write(t, main, APIVersion), Host{}); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
	dir := t.TempDir()
	if _, err := Load(dir, Host{}); err == nil {
		t.Error("no manifest: loaded")
	}
}

// Reading the manifest for the consent prompt must never run the plugin's code.
func TestReadManifestRunsNothing(t *testing.T) {
	mark := filepath.Join(t.TempDir(), "ran")
	dir := write(t, fmt.Sprintf(`io.open(%q, "w"):close()`, mark), APIVersion)
	m, err := ReadManifest(dir)
	if err != nil || m.Does != "Demo" {
		t.Fatalf("%+v, %v", m, err)
	}
	if _, err := os.Stat(mark); err == nil {
		t.Error("ReadManifest ran main.lua")
	}
}

// The examples are the proof that useful plugins need no editor changes (M5).
func TestExamples(t *testing.T) {
	run := func(name, command, text string) (*fakeBuffer, string) {
		t.Helper()
		buf, said := &fakeBuffer{text: text}, ""
		p, err := Load(filepath.Join("..", "..", "examples", "plugins", name), Host{
			Message: func(s string) { said = s },
			Buffer:  func() Buffer { return buf },
		})
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		if command == "" {
			p.Emit("buffer_save", buf)
		} else if !p.Run(command) {
			t.Fatalf("%s has no command %s", name, command)
		}
		if p.Err != nil {
			t.Fatal(p.Err)
		}
		return buf, said
	}
	if buf, _ := run("trim-on-save", "", "a  \r\nb\t\nc  "); buf.text != "a\r\nb\nc" {
		t.Errorf("trimmed to %q", buf.text)
	}
	if _, said := run("word-count", "wc", "one two\nthree\n"); said != "2 lines, 3 words, 14 bytes" {
		t.Errorf("wc said %q", said)
	}
	if _, said := run("todo-count", "todos", "TODO a TODO FIXME"); said != "2 TODO, 1 FIXME" {
		t.Errorf("todos said %q", said)
	}
}

func TestManifestChecks(t *testing.T) {
	for _, bad := range []string{
		`name = ".."` + "\nversion=\"1\"\napi_version=1\ndoes=\"x\"",
		`name = "a/b"` + "\nversion=\"1\"\napi_version=1\ndoes=\"x\"",
		`name = "a"` + "\nversion=\"1\"\napi_version=1\ndoes=\"\"",
		`name = "a"` + "\napi_version=1\ndoes=\"x\"",
	} {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "plugin.toml"), []byte(bad), 0o644) //nolint:errcheck // Read just below.
		if _, err := ReadManifest(dir); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "plugin.toml"), []byte("name=\"a\"\nversion=\"1\"\napi_version=1\ndoes=\"x\\u001b[2Jy\"\n"), 0o644) //nolint:errcheck // Read just below.
	if m, err := ReadManifest(dir); err != nil || strings.ContainsRune(m.Does, 0x1b) {
		t.Errorf("escape kept: %q, %v", m.Does, err)
	}
}

func TestConsentsAreByVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.toml")
	c, err := OpenConsents(path)
	if err != nil {
		t.Fatal(err)
	}
	m := Manifest{Name: "a", Version: "1"}
	if c.Accepted(m) || c.Declined(m) {
		t.Fatal("answered before being asked")
	}
	if err := c.Set(m, true); err != nil {
		t.Fatal(err)
	}
	c, _ = OpenConsents(path)
	if !c.Accepted(m) || c.Accepted(Manifest{Name: "a", Version: "2"}) {
		t.Fatal("acceptance must be for this version only")
	}
	os.WriteFile(path, []byte("broken ="), 0o644) //nolint:errcheck // Read just below.
	if c, err = OpenConsents(path); err == nil || c.Accepted(m) {
		t.Fatal("a broken file must accept nothing")
	}
	if c.Set(m, true) == nil {
		t.Fatal("a broken file must not be replaced")
	}
}
