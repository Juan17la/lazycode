package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestPanelHeaderListsAllViewsAndClicksSwitch(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "x"})
	u.screen.SetSize(100, 30)
	u.wait("▤ Files  ⎇  ⌕  ✻")
	u.clickOn("⎇")
	u.wait("⎇ Git")
	u.wait("▤  ⎇ Git") // Files is now only its icon.
	u.alt('f')
	u.wait("⌕ Search")
	u.wait("Find:")
	u.alt('e')
	u.wait("▤ Files")
	// The second form of each shortcut (Q-80).
	u.key(tcell.KeyCtrlK)
	u.press("g")
	u.wait("⎇ Git")
	u.key(tcell.KeyCtrlK)
	u.press("f")
	u.wait("⌕ Search")
}

func TestSearchFindsNamesAndTextAndOpensAtTheMatch(t *testing.T) {
	u := start(t, map[string]string{
		"hello.txt": "first line\nsay hello there\nlast\n",
		"other.txt": "nothing\n",
	})
	u.screen.SetSize(100, 30)
	u.alt('f')
	u.wait("Type to search")
	u.press("hello")
	u.wait("Files (1)")
	u.wait("Text (1)")
	u.wait("2: say hello there")
	u.key(tcell.KeyDown) // From the name to the text match.
	u.key(tcell.KeyEnter)
	u.wait("say hello there")
	// The match is selected, so typing replaces it.
	u.press("X")
	u.wait("say X there")
}

func TestEmptySearchSaysSoAndNewInputReplacesTheOldSearch(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "abc\n"})
	u.screen.SetSize(100, 30)
	u.alt('f')
	u.press("zzz")
	u.wait("No results in work")
	u.press("\x7f") // Not a key; the real one is next.
	u.key(tcell.KeyBackspace2)
	u.key(tcell.KeyBackspace2)
	u.key(tcell.KeyBackspace2)
	u.key(tcell.KeyBackspace2)
	u.press("abc")
	u.wait("Text (1)")
	u.waitGone("No results")
}

func TestSearchSkipsIgnoredAndBinaryFiles(t *testing.T) {
	u := start(t, map[string]string{
		".gitignore":  "ignored.txt\n",
		"ok.txt":      "needle\n",
		"ignored.txt": "needle\n",
		"bin.dat":     "needle\x00\n",
	})
	u.git("init", "-q")
	u.screen.SetSize(100, 30)
	u.alt('f')
	u.press("needle")
	u.wait("Text (1)")
	u.waitGone("ignored.txt")
	u.waitGone("bin.dat")
}

func TestReplaceShowsChangesFirstAndAppliesOnConfirm(t *testing.T) {
	u := start(t, map[string]string{
		"a.txt": "red fish\nblue fish\n",
		"b.txt": "one fish\n",
	})
	u.screen.SetSize(100, 30)
	u.alt('f')
	u.press("fish")
	u.wait("Text (3)")
	u.key(tcell.KeyTab)
	u.press("cat")
	u.key(tcell.KeyEnter)
	u.wait("Replace \"fish\" with \"cat\": 3 changes in 2 files")
	u.wait("- ")
	u.wait("red fish")
	u.wait("red cat")
	if u.file("a.txt") != "red fish\nblue fish\n" {
		t.Fatal("something was written before confirming")
	}
	u.key(tcell.KeyEsc) // Cancel.
	u.waitGone("Enter applies")
	if u.file("a.txt") != "red fish\nblue fish\n" || u.file("b.txt") != "one fish\n" {
		t.Fatal("cancelling changed a file")
	}
	u.key(tcell.KeyEnter)
	u.wait("Enter applies")
	u.key(tcell.KeyEnter)
	u.waitFile("a.txt", "red cat\nblue cat\n")
	u.waitFile("b.txt", "one cat\n")
	u.wait("Replaced in 2 files")
}

func TestReplaceSkipsUnsavedAndReadOnlyFilesAndSaysSo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write to read-only files")
	}
	u := start(t, map[string]string{
		"open.txt":    "fish here\n",
		"locked.txt":  "fish there\n",
		"changed.txt": "fish again\n",
		"fine.txt":    "fish ok\n",
	})
	u.screen.SetSize(120, 30)
	if err := os.Chmod(filepath.Join(u.dir, "locked.txt"), 0o444); err != nil {
		t.Fatal(err)
	}
	u.open("open.txt")
	u.press("typed ") // Unsaved work, which must never be saved over.
	u.alt('f')
	u.press("fish")
	u.wait("Text (4)")
	u.key(tcell.KeyTab)
	u.press("cat")
	u.key(tcell.KeyEnter)
	u.wait("has unsaved edits")
	// A tool changes a file after the search, before the confirmation.
	if err := os.WriteFile(filepath.Join(u.dir, "changed.txt"), []byte("they rewrote it\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	u.wait("will be skipped")
	u.key(tcell.KeyEnter)
	u.waitFile("fine.txt", "cat ok\n")
	u.wait("were skipped")
	if got := u.file("open.txt"); got != "fish here\n" {
		t.Errorf("a file with unsaved edits was saved over: %q", got)
	}
	if got := u.file("locked.txt"); got != "fish there\n" {
		t.Errorf("read-only file changed: %q", got)
	}
	if got := u.file("changed.txt"); got != "they rewrote it\n" {
		t.Errorf("a file that changed after the search was overwritten: %q", got)
	}
	// The reasons are listed in the Search panel, per file.
	u.wait("locked.txt:")
	u.wait("open.txt:")
}

func TestReplaceInOpenFileCanBeUndone(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "one two one\n"})
	u.screen.SetSize(100, 30)
	u.open("a.txt")
	u.alt('f')
	u.press("one")
	u.wait("Text (2)")
	u.key(tcell.KeyTab)
	u.press("1")
	u.key(tcell.KeyEnter)
	u.wait("Enter applies")
	u.key(tcell.KeyEnter)
	u.waitFile("a.txt", "1 two 1\n")
	u.key(tcell.KeyEsc) // From the Search view into the file.
	u.mod(tcell.KeyCtrlZ, 0)
	u.wait("one two one")
}

func TestCtrlPOpensAFileByName(t *testing.T) {
	u := start(t, map[string]string{"alpha.txt": "AAA\n", "beta.txt": "BBB\n"})
	u.screen.SetSize(100, 30)
	if err := os.Mkdir(filepath.Join(u.dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	u.write("sub/gamma.txt", "GGG\n")
	u.key(tcell.KeyCtrlP)
	u.wait("Open file")
	u.wait(filepath.Join("sub", "gamma.txt"))
	u.press("gam")
	u.key(tcell.KeyEnter)
	u.wait("GGG")
}

func TestSearchInputPaste(t *testing.T) {
	u := start(t, map[string]string{"a.txt": "pasted words\n"})
	u.screen.SetSize(100, 30)
	u.alt('f')
	u.post(tcell.NewEventPaste(true))
	u.press("pasted")
	u.post(tcell.NewEventPaste(false))
	u.wait("Text (1)")
	if strings.Count(u.text(), "pasted") < 2 {
		t.Error("the pasted text should be in the field and the result")
	}
}
