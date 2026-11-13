package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Juan17la/lazycode/internal/lsp"
	"github.com/Juan17la/lazycode/internal/store"
	"github.com/Juan17la/lazycode/internal/syntax"
	"github.com/gdamore/tcell/v2"
)

func writePlugin(t *testing.T, root, name, version, main string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "name = \"" + name + "\"\nversion = \"" + version + "\"\napi_version = 1\ndoes = \"Does a thing\"\n"
	if err := os.WriteFile(filepath.Join(dir, "plugin.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.lua"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
}

const shoutPlugin = `
local lc = require("lazycode")
lc.command("shout", function() lc.message("HEY " .. lc.current_buffer():len()) end)
lc.key("ctrl+alt+y", "shout")
lc.on("buffer_save", function(buf)
  local text = buf:text()
  if text:sub(-1) ~= "!" then buf:replace(#text + 1, #text, "!") end
end)
`

func withPlugins(t *testing.T, files map[string]string, add func(root string)) *ui {
	return startTweaked(t, files, func(a *app) { add(a.pluginsDir()) })
}

func TestPluginAcceptedRunsAndSaveEventChangesWhatIsSaved(t *testing.T) {
	u := withPlugins(t, map[string]string{"a.txt": "abc"}, func(root string) { writePlugin(t, root, "shout", "1.0.0", shoutPlugin) })
	u.wait("Only accept plugins you trust")
	u.wait("Add plugin shout 1.0.0? Does a thing")
	u.press("a")
	u.wait("Added shout")
	u.open("a.txt")
	u.key(tcell.KeyCtrlBackslash)
	u.run("shout")
	u.wait("HEY 3")
	u.run("w")
	u.waitFile("a.txt", "abc!")
}

func TestPluginDeclinedDoesNotRunAndIsNotAskedAgain(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	var dir string
	u := withPlugins(t, nil, func(root string) {
		dir = root
		writePlugin(t, root, "bad", "1.0.0", fmt.Sprintf(`io.open(%q, "w"):close()`, marker))
	})
	u.wait("Add plugin bad")
	u.key(tcell.KeyEsc)
	u.waitGone("Add plugin bad")
	u.run("bad")
	u.wait("Unknown command")
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a declined plugin ran")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(dir), "plugins.toml"))
	if err != nil || !strings.Contains(string(data), "declined") {
		t.Fatalf("the answer wasn't kept: %q, %v", data, err)
	}
	u.run("q")
	u.quit()
	u2 := launch(t, u.dir)
	time.Sleep(100 * time.Millisecond)
	if strings.Contains(u2.text(), "Add plugin") {
		t.Fatal("asked again after declining")
	}
	u2.run("q")
	u2.quit()
	writePlugin(t, dir, "bad", "2.0.0", "")
	u3 := launch(t, u.dir)
	u3.wait("Add plugin bad 2.0.0") // A new version is new code.
}

func TestPluginFailureDisablesItAndEditorKeepsRunning(t *testing.T) {
	u := withPlugins(t, map[string]string{"a.txt": "abc"}, func(root string) {
		writePlugin(t, root, "boom", "1", `require("lazycode").command("boom", function() error("oops") end)`)
	})
	u.press("a")
	u.wait("Added boom")
	u.run("boom")
	u.wait("disabled")
	u.run("plugin")
	u.wait("boom 1 (disabled)")
	u.open("a.txt")
	u.press("x")
	u.wait("xabc")
}

func TestPluginFolderMustMatchName(t *testing.T) {
	u := withPlugins(t, nil, func(root string) {
		writePlugin(t, root, "real", "1", "")
		if err := os.Rename(filepath.Join(root, "real"), filepath.Join(root, "other")); err != nil {
			t.Fatal(err)
		}
	})
	u.wait("they must match")
}

func TestPluginOffAndOn(t *testing.T) {
	u := withPlugins(t, map[string]string{"a.txt": "abc"}, func(root string) { writePlugin(t, root, "shout", "1", shoutPlugin) })
	u.press("a")
	u.wait("Added shout")
	u.run("plugin off shout")
	u.wait("Turned off shout")
	u.run("shout")
	u.wait("Unknown command")
	u.run("plugin on ../shout") // filepath.Base keeps it in the plugins folder.
	u.wait("Add plugin shout")
	u.press("a")
	u.wait("Added shout")
}

func TestPluginKeyRunsItsCommandAndCursorFollowsItsEdit(t *testing.T) {
	u := withPlugins(t, map[string]string{"a.txt": "abc"}, func(root string) {
		writePlugin(t, root, "front", "1", `
local lc = require("lazycode")
lc.command("front", function() lc.current_buffer():replace(1, 0, "12") end)
lc.key("ctrl+alt+y", "front")`)
	})
	u.press("a")
	u.wait("Added front")
	u.open("a.txt")
	u.post(tcell.NewEventKey(tcell.KeyRune, 'y', tcell.ModCtrl|tcell.ModAlt))
	u.wait("12abc")
	u.press("x") // The cursor was at the start, where the text went in, so it stays before it.
	u.wait("x12abc")
}

func TestLanguagePackAddsAServerAndStopsWithIt(t *testing.T) {
	u := withPlugins(t, nil, func(root string) {
		writePlugin(t, root, "rust", "1", "")
		lang := "name = \"rust\"\nextensions = [\"rs\"]\n[server]\ncommand = \"rust-analyzer\"\n"
		if err := os.WriteFile(filepath.Join(root, "rust", "language.toml"), []byte(lang), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	u.press("a")
	u.wait("Added rust")
	if srv, id, ok := lsp.ForPath("a.RS"); !ok || srv.Command != "rust-analyzer" || id != "rust" {
		t.Fatalf("got %+v %q %v", srv, id, ok)
	}
	u.run("plugin off rust")
	u.wait("Turned off rust")
	if _, _, ok := lsp.ForPath("a.rs"); ok {
		t.Fatal("still there after turning it off")
	}
}

func storeServer(t *testing.T, name, version, lua string) string {
	t.Helper()
	files := map[string]string{
		"plugin.toml": "name = \"" + name + "\"\nversion = \"" + version + "\"\napi_version = 1\ndoes = \"Does store things\"\n",
		"main.lua":    lua,
	}
	dir := t.TempDir()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	w := tar.NewWriter(gz)
	for n, content := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "top/" + n, Mode: 0o644, Size: int64(len(content))}) //nolint:errcheck // Checked by Close.
		w.Write([]byte(content))                                                                                    //nolint:errcheck // Checked by Close.
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	gz.Close() //nolint:errcheck // Only memory.
	sum, err := store.Checksum(dir)
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("a", 40)
	index := fmt.Sprintf("[[plugin]]\nname=%q\nversion=%q\ndescription=\"From the store\"\nrepository=\"https://github.com/o/r\"\ncommit=%q\nchecksum=%q\n", name, version, commit, sum)
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.toml" {
			rw.Write([]byte(index)) //nolint:errcheck // A test.
			return
		}
		rw.Write(buf.Bytes()) //nolint:errcheck // A test.
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestStoreBrowseAndInstall(t *testing.T) {
	url := storeServer(t, "fromstore", "1.0.0", `require("lazycode").command("hi", function() require("lazycode").message("hi from store") end)`)
	var dir string
	u := startTweaked(t, nil, func(a *app) {
		dir = a.pluginsDir()
		a.storeApp.client = &store.Client{Index: url + "/index.toml", GitHub: url, HTTP: http.DefaultClient}
	})
	u.run("store")
	u.wait("fromstore 1.0.0  From the store")
	u.key(tcell.KeyEnter)
	u.wait("Add plugin fromstore 1.0.0? Does store things")
	u.wait("Only accept plugins you trust")
	if _, err := os.Stat(filepath.Join(dir, "fromstore")); err == nil {
		t.Fatal("in place before the user accepted")
	}
	u.press("a")
	u.wait("Added fromstore")
	u.run("hi")
	u.wait("hi from store")
	u.run("store")
	u.wait("(installed)")
}

func TestStoreDeclineInstallsNothing(t *testing.T) {
	url := storeServer(t, "fromstore", "1.0.0", "")
	var dir string
	u := startTweaked(t, nil, func(a *app) {
		dir = a.pluginsDir()
		a.storeApp.client = &store.Client{Index: url + "/index.toml", GitHub: url, HTTP: http.DefaultClient}
	})
	u.run("store")
	u.wait("fromstore 1.0.0")
	u.key(tcell.KeyEnter)
	u.wait("Add plugin fromstore")
	u.key(tcell.KeyEsc)
	u.waitGone("Add plugin fromstore")
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("left behind: %v", entries)
	}
}

func TestStoreUnreachableSaysSo(t *testing.T) {
	u := startTweaked(t, nil, func(a *app) {
		a.storeApp.client = &store.Client{Index: "http://127.0.0.1:1/index.toml", HTTP: http.DefaultClient}
	})
	u.run("store")
	u.wait("Couldn't open the store")
}

func TestColourPackColoursItsFiles(t *testing.T) {
	u, _ := launchStyled(t, map[string]string{"q.sql": "select 1\n"})
	u.quitNow()
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "lazycode", "plugins")
	writePlugin(t, root, "sql", "1", "")
	lang := "name = \"SQL\"\nextensions = [\"sql\"]\n[[highlight]]\nclass = \"keyword\"\npattern = 'select'\n"
	if err := os.WriteFile(filepath.Join(root, "sql", "language.toml"), []byte(lang), 0o644); err != nil {
		t.Fatal(err)
	}
	v := launch(t, u.dir)
	v.wait("Add plugin sql")
	v.press("a")
	v.wait("Added sql")
	v.open("q.sql")
	v.wait("SQL  Ln 1") // The status line names what's colouring it.
	if syntax.ForPath("q.sql") == nil {
		t.Fatal("the pack isn't registered")
	}
	v.run("q")
}
