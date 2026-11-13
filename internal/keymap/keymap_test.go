package keymap

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestNames(t *testing.T) {
	for _, tc := range []struct {
		key  tcell.Key
		r    rune
		mods tcell.ModMask
		want string
	}{
		{tcell.KeyCtrlS, 0, tcell.ModCtrl, "ctrl+s"},
		{tcell.KeyCtrlS, 0, 0, "ctrl+s"},
		{tcell.KeyRune, 's', tcell.ModCtrl, "ctrl+s"}, // Windows.
		{tcell.KeyRune, 's', tcell.ModCtrl | tcell.ModShift, "ctrl+shift+s"},
		{tcell.KeyRune, 'd', tcell.ModAlt, "alt+d"},
		{tcell.KeyRune, 'F', tcell.ModAlt, "alt+shift+f"},
		{tcell.KeyRune, 'f', tcell.ModAlt | tcell.ModShift, "alt+shift+f"}, // Kitty's protocol.
		{tcell.KeyRune, ',', tcell.ModAlt, "alt+,"},
		{tcell.KeyRune, 'x', 0, "x"},
		{tcell.KeyRune, 'X', tcell.ModShift, "X"},
		{tcell.KeyRune, ':', tcell.ModShift, ":"},
		{tcell.KeyLeft, 0, 0, "left"},
		{tcell.KeyLeft, 0, tcell.ModShift, "shift+left"},
		{tcell.KeyLeft, 0, tcell.ModCtrl | tcell.ModShift, "ctrl+shift+left"},
		{tcell.KeyHome, 0, tcell.ModCtrl, "ctrl+home"},
		{tcell.KeyF2, 0, tcell.ModCtrl, "ctrl+f2"},
		{tcell.KeyBacktab, 0, 0, "shift+tab"},
		{tcell.KeyTab, 0, tcell.ModShift, "shift+tab"},
		{tcell.KeyEnter, 0, 0, "enter"},
		{tcell.KeyEsc, 0, 0, "esc"},
		{tcell.KeyBackspace, 0, 0, "backspace"},
		{tcell.KeyBackspace2, 0, 0, "backspace"},
		{tcell.KeyDelete, 0, 0, "delete"},
		{tcell.KeyCtrlBackslash, 0, tcell.ModCtrl, Reserved},
		{tcell.KeyRune, '\\', tcell.ModCtrl, Reserved},
		{tcell.KeyRune, '|', tcell.ModCtrl, Reserved},
		{tcell.KeyCtrlSpace, 0, tcell.ModCtrl, Toggle},
		{tcell.KeyRune, ' ', tcell.ModCtrl, Toggle},
		{tcell.Key(' '), 0, tcell.ModCtrl, Toggle},
		{tcell.KeyRune, 'p', tcell.ModAlt, "alt+p"},
	} {
		if got := Name(tcell.NewEventKey(tc.key, tc.r, tc.mods)); got != tc.want {
			t.Errorf("%v %q %v: got %q, want %q", tc.key, tc.r, tc.mods, got, tc.want)
		}
	}
}

func TestDefaults(t *testing.T) {
	m := Default()
	if got := m.Lookup(Editor, "ctrl+s"); got != "file.save" {
		t.Errorf("ctrl+s: %q", got)
	}
	if got := m.Lookup(Panel, "ctrl+s"); got != "" {
		t.Errorf("saving has no meaning in the tree: %q", got)
	}
	if got := m.Lookup(Editor, "alt+p"); got != "project.switch" {
		t.Errorf("global keys work in files: %q", got)
	}
	if got := m.Lookup(Panel, "alt+p"); got != "project.switch" {
		t.Errorf("global keys work in the tree: %q", got)
	}
	if got := m.Lookup(Panel, "up"); got != "tree.up" {
		t.Errorf("up in the tree: %q", got)
	}
	if got := m.Lookup(Editor, "up"); got != "move.up" {
		t.Errorf("up in a file: %q", got)
	}
	if got := m.Lookup(Editor, "x"); got != "" {
		t.Errorf("a plain letter in a file types: %q", got)
	}
	if got := m.Lookup(Panel, ":"); got != "command.open" {
		t.Errorf(": in the tree: %q", got)
	}
	if !contains(Actions(), "file.save") {
		t.Error("actions are missing")
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func TestChordsMirrorAltKeys(t *testing.T) {
	m := Default()
	if got := m.LookupChord(Editor, "d"); got != "directory.switch" {
		t.Errorf("d: %q", got)
	}
	if got := m.LookupChord(Panel, ","); got != "tab.previous" {
		t.Errorf(",: %q", got)
	}
	if got := m.LookupChord(Editor, "t"); got != "view.terminal" {
		t.Errorf("h: %q", got)
	}
	if got := m.LookupChord(Editor, "o"); got != "select.all_occurrences" {
		t.Errorf("o is the macOS Ctrl+F2 (Q-80): %q", got)
	}
	if got := m.LookupChord(Editor, "y"); got != "" {
		t.Errorf("y: %q", got)
	}
}

func load(t *testing.T, keys string) (*Map, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "keys.toml")
	if err := os.WriteFile(path, []byte(keys), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestRebinding(t *testing.T) {
	m, err := load(t, `
[keys]
"Ctrl+Shift+K" = "edit.cut"
"ctrl+d" = "none"
"alt+x" = "tab.close"
"ctrl+x" = "file.save"
"f5" = "move.file_start"
`)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Lookup(Editor, "ctrl+shift+k"); got != "edit.cut" {
		t.Errorf("new key: %q", got)
	}
	if got := m.Lookup(Editor, "ctrl+d"); got != "" {
		t.Errorf("turned off: %q", got)
	}
	if got := m.Lookup(Editor, "alt+x"); got != "tab.close" {
		t.Errorf("global action: %q", got)
	}
	if got := m.Lookup(Panel, "alt+x"); got != "tab.close" {
		t.Errorf("global action in the tree: %q", got)
	}
	if got := m.Lookup(Editor, "ctrl+x"); got != "file.save" {
		t.Errorf("replaced: %q", got)
	}
	if got := m.Lookup(Editor, "ctrl+s"); got != "file.save" {
		t.Errorf("old key stays: %q", got)
	}
	if got := m.Lookup(Editor, "f5"); got != "move.file_start" {
		t.Errorf("function key: %q", got)
	}
	if got := m.LookupChord(Editor, "x"); got != "tab.close" {
		t.Errorf("chord follows the Alt binding: %q", got)
	}
	// The defaults themselves aren't changed.
	if got := Default().Lookup(Editor, "ctrl+d"); got != "select.next_occurrence" {
		t.Errorf("defaults changed: %q", got)
	}
}

func TestBadBindingsAreReportedAndTheRestApply(t *testing.T) {
	m, err := load(t, `
[keys]
"ctrl+\\" = "file.save"
"ctrl+nothing" = "file.save"
"ctrl+q" = "no.such_action"
"ctrl+g" = "edit.undo"
"CTRL+J" = "edit.cut"
"ctrl+j" = "edit.copy"
`)
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{"reserved", "ctrl+nothing", "no.such_action", "same key"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q doesn't mention %q", err, want)
		}
	}
	if got := m.Lookup(Editor, "ctrl+g"); got != "edit.undo" {
		t.Errorf("a good binding didn't apply: %q", got)
	}
	if got := m.Lookup(Editor, Reserved); got != "" {
		t.Errorf("reserved key bound: %q", got)
	}
}

func TestBrokenFile(t *testing.T) {
	m, err := load(t, "[keys\n")
	if err == nil || !strings.Contains(err.Error(), ", line ") {
		t.Fatalf("error %v", err)
	}
	if got := m.Lookup(Editor, "ctrl+s"); got != "file.save" {
		t.Errorf("defaults should still work: %q", got)
	}
	m, err = Load(filepath.Join(t.TempDir(), "missing"))
	if err != nil || m.Lookup(Editor, "ctrl+s") != "file.save" {
		t.Fatalf("missing file: %v", err)
	}
}

func TestEveryDefaultKeyIsWrittenCanonically(t *testing.T) {
	for context, keys := range defaults {
		for key := range keys {
			got, err := normalize(key)
			if err != nil || got != key {
				t.Errorf("context %d: %q normalizes to %q, %v", context, key, got, err)
			}
		}
	}
}

func TestLanguageServerKeys(t *testing.T) {
	m := Default()
	for key, want := range map[string]string{"f12": "lsp.definition", "f2": "lsp.rename", "f3": "lsp.format"} {
		if got := m.Lookup(Editor, key); got != want {
			t.Errorf("%s is %q, want %q", key, got, want)
		}
	}
	// The second form of Alt+Shift+S: Ctrl+K, then a capital S.
	if got := m.LookupChord(Editor, "S"); got != "split.right" {
		t.Errorf("Ctrl+K then S is %q", got)
	}
	if got := m.LookupChord(Editor, "G"); got != "" {
		t.Errorf("an unbound capital is %q", got)
	}
	// Alt+Shift+F arrives as a capital with Alt held, and has to name the same key.
	ev := tcell.NewEventKey(tcell.KeyRune, 'F', tcell.ModAlt)
	if got := Name(ev); got != "alt+shift+f" {
		t.Errorf("named %q", got)
	}
}

func TestBindWritesTheKeyAndTurnsOffTheOld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.toml")
	m := Default()
	if err := m.Bind(path, "alt+j", "view.git"); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Lookup(Panel, "alt+j"); got != "view.git" {
		t.Errorf("new key: %q", got)
	}
	if got := loaded.Lookup(Panel, "alt+g"); got != "" {
		t.Errorf("old key still works: %q", got)
	}
	// Binding the same key again must not write a duplicate, which TOML refuses.
	if err := loaded.Bind(path, "alt+j", "view.files"); err != nil {
		t.Fatal(err)
	}
	if loaded, err = Load(path); err != nil {
		t.Fatal(err)
	}
	if got := loaded.Lookup(Panel, "alt+j"); got != "view.files" {
		t.Errorf("rebound key: %q", got)
	}
	for _, key := range []string{Reserved, Toggle, "x"} {
		if err := m.Bind(path, key, "view.git"); err == nil {
			t.Errorf("%s was accepted", key)
		}
	}
}

func TestCatalogHoldsOnlyRealActions(t *testing.T) {
	for _, entry := range Catalog {
		if len(Default().KeysFor(entry.Action)) == 0 {
			t.Errorf("%s has no key", entry.Action)
		}
	}
}

func TestNewEditorKeysAreBound(t *testing.T) {
	if got := Name(tcell.NewEventKey(tcell.KeyCtrlUnderscore, 0, tcell.ModCtrl)); got != "ctrl+/" {
		t.Fatalf("ctrl+/ is named %q", got)
	}
	if got := Name(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModAlt)); got != "alt+up" {
		t.Fatalf("alt+up is named %q", got)
	}
	for key, want := range map[string]string{"ctrl+/": "edit.toggle_comment", "alt+up": "cursor.add_above", "alt+down": "cursor.add_below", "ctrl+shift+up": "line.move_up"} {
		if got := defaults[Editor][key]; got != want {
			t.Errorf("%s does %q, want %q", key, got, want)
		}
	}
}

func TestSwitchingWorksFromTerminalsAndNothingElseIsLost(t *testing.T) {
	m := Default()
	for _, action := range FromTerminal {
		if len(m.KeysFor(action)) == 0 {
			t.Errorf("%s has no key", action)
		}
	}
	for key, want := range map[string]string{"alt+w": "focus.next", "alt+tab": "focus.next", "alt+v": "view.move"} {
		if got := m.Lookup(Editor, key); got != want {
			t.Errorf("%s is %q, want %q", key, got, want)
		}
	}
	if slices.Contains(Actions(), "split.next") {
		t.Error("split.next came back")
	}
}

func TestLoadRefusesTheToggleKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.toml")
	if err := os.WriteFile(path, []byte("[keys]\n\"ctrl+space\" = \"file.save\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Errorf("got %v", err)
	}
	if got := m.Lookup(Editor, Toggle); got != "" {
		t.Errorf("bound to %q", got)
	}
}
