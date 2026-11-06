// Package keymap names keys, holds the default bindings and applies the user's (Q-24).
package keymap

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/BurntSushi/toml"
	"github.com/gdamore/tcell/v2"
)

// Context says where a key was pressed. Terminal tabs have no context: they get every
// key except the reserved one.
type Context int

const (
	// Global bindings work in files and panels.
	Global Context = iota
	// Editor bindings work in a file.
	Editor
	// Panel bindings work in the file tree.
	Panel
)

// Reserved leaves a terminal tab, so it can't be rebound (Q-74).
const Reserved = `ctrl+\`

// Template is what a new keys.toml starts with.
const Template = `# Change a key by giving it an action, or turn one off with "none". For example:
#
# [keys]
# "ctrl+b" = "panel.toggle_second"
# "ctrl+shift+k" = "edit.cut"
# "ctrl+d" = "none"
#
# F1 lists the shortcuts, and can change them for you. The actions are in the README.
[keys]
`

// defaults lists every action a key can have, per context. The keys follow one rule per family, so
// that they can be guessed (doc 07):
//
//	Ctrl+letter        this file and tab: save, find, replace, new file, close
//	Alt+letter         show something: Explorer, Git, Find, AI, Terminal, Projects, Directories, Run
//	Alt+Shift+letter   make something: Project, Directory, Terminal, AI session, Split
//	Alt+, Alt+. Alt+W  move: previous and next tab (the < and > keys), next window (split)
//	F-key              ask the language: F2 rename, F3 format, F12 definition. F1 and F10 are help and menu
//
// Alt+arrows would be the obvious moving keys, and Alt+Shift+D the obvious directory key, but Windows
// Terminal keeps those for its own panes (doc 07).
var defaults = map[Context]map[string]string{
	Global: {
		"alt+e": "view.files", "alt+g": "view.git", "alt+f": "view.search", "alt+a": "view.sessions", "alt+t": "view.terminal",
		"alt+p": "project.switch", "alt+d": "directory.switch", "alt+r": "run.play",
		"alt+m": "app.menu", "alt+s": "app.settings", "alt+k": "app.shortcuts", "alt+x": "command.open", "alt+z": "view.wrap",
		"alt+shift+p": "project.new", "alt+shift+f": "directory.add", "alt+shift+t": "terminal.new", "alt+shift+a": "session.new",
		"alt+shift+s": "split.right", "alt+shift+b": "split.down",
		"alt+,": "tab.previous", "alt+.": "tab.next", "alt+w": "split.next",
		"ctrl+b": "panel.toggle_second", "ctrl+w": "tab.close", "ctrl+p": "file.open_by_name", "ctrl+n": "file.new", "ctrl+q": "app.quit",
		"f1": "app.shortcuts", "f10": "app.menu",
	},
	Editor: {
		"left": "move.left", "right": "move.right", "up": "move.up", "down": "move.down",
		"pgup": "move.page_up", "pgdn": "move.page_down", "home": "move.home", "end": "move.end",
		"ctrl+left": "move.word_left", "ctrl+right": "move.word_right",
		"ctrl+home": "move.file_start", "ctrl+end": "move.file_end",
		"shift+left": "select.left", "shift+right": "select.right", "shift+up": "select.up", "shift+down": "select.down",
		"shift+pgup": "select.page_up", "shift+pgdn": "select.page_down", "shift+home": "select.home", "shift+end": "select.end",
		"ctrl+shift+left": "select.word_left", "ctrl+shift+right": "select.word_right",
		"ctrl+shift+home": "select.file_start", "ctrl+shift+end": "select.file_end",
		"ctrl+a": "select.all", "ctrl+d": "select.next_occurrence", "ctrl+f2": "select.all_occurrences",
		"esc": "edit.escape", "enter": "edit.newline", "tab": "edit.indent", "shift+tab": "edit.outdent",
		"backspace": "edit.backspace", "delete": "edit.delete",
		"ctrl+c": "edit.copy", "ctrl+x": "edit.cut", "ctrl+v": "edit.paste",
		"ctrl+z": "edit.undo", "ctrl+y": "edit.redo", "ctrl+shift+z": "edit.redo",
		"ctrl+s": "file.save", "ctrl+f": "find.open", "ctrl+r": "find.replace",
		"f12": "lsp.definition", "f2": "lsp.rename", "f3": "lsp.format",
	},
	Panel: {
		"up": "tree.up", "down": "tree.down", "left": "tree.close", "right": "tree.open",
		"enter": "tree.select", "tab": "tab.enter", "r": "tree.refresh", ":": "command.open",
		"alt+v": "view.move", "n": "session.new", "delete": "session.hide",
	},
}

// Entry is one row of the shortcuts menu.
type Entry struct{ Family, Action, Label string }

// Catalog is what the shortcuts menu lists, by family. The cursor and typing keys stay out of it.
var Catalog = []Entry{
	{"Show (Alt+letter)", "view.files", "Explorer"},
	{"Show (Alt+letter)", "view.git", "Git"},
	{"Show (Alt+letter)", "view.search", "Find in this directory"},
	{"Show (Alt+letter)", "view.sessions", "AI sessions"},
	{"Show (Alt+letter)", "view.terminal", "Terminal dock, show or hide"},
	{"Show (Alt+letter)", "project.switch", "Projects"},
	{"Show (Alt+letter)", "directory.switch", "Directories"},
	{"Show (Alt+letter)", "run.play", "Run this directory"},
	{"Show (Alt+letter)", "view.wrap", "Word wrap, on or off"},
	{"Show (Alt+letter)", "app.menu", "Menu bar"},
	{"Show (Alt+letter)", "app.settings", "Settings"},
	{"Show (Alt+letter)", "app.shortcuts", "These shortcuts"},
	{"Show (Alt+letter)", "command.open", "Command bar"},
	{"Make (Alt+Shift+letter)", "project.new", "New project"},
	{"Make (Alt+Shift+letter)", "directory.add", "Add a directory"},
	{"Make (Alt+Shift+letter)", "terminal.new", "New terminal"},
	{"Make (Alt+Shift+letter)", "session.new", "New AI session"},
	{"Make (Alt+Shift+letter)", "split.right", "Split right"},
	{"Make (Alt+Shift+letter)", "split.down", "Split below"},
	{"Move (Alt+, Alt+. Alt+W)", "tab.previous", "Previous tab"},
	{"Move (Alt+, Alt+. Alt+W)", "tab.next", "Next tab"},
	{"Move (Alt+, Alt+. Alt+W)", "split.next", "Next split, and the dock"},
	{"This file (Ctrl+letter)", "file.new", "New file"},
	{"This file (Ctrl+letter)", "file.open_by_name", "Open a file by name"},
	{"This file (Ctrl+letter)", "file.save", "Save"},
	{"This file (Ctrl+letter)", "tab.close", "Close the tab"},
	{"This file (Ctrl+letter)", "find.open", "Find"},
	{"This file (Ctrl+letter)", "find.replace", "Replace"},
	{"This file (Ctrl+letter)", "edit.undo", "Undo"},
	{"This file (Ctrl+letter)", "edit.redo", "Redo"},
	{"This file (Ctrl+letter)", "edit.copy", "Copy"},
	{"This file (Ctrl+letter)", "edit.cut", "Cut"},
	{"This file (Ctrl+letter)", "edit.paste", "Paste"},
	{"This file (Ctrl+letter)", "select.all", "Select all"},
	{"This file (Ctrl+letter)", "select.next_occurrence", "Add the next match"},
	{"This file (Ctrl+letter)", "panel.toggle_second", "Second panel, open or close"},
	{"This file (Ctrl+letter)", "app.quit", "Quit"},
	{"Language (F-key)", "lsp.rename", "Rename the symbol"},
	{"Language (F-key)", "lsp.format", "Format the file"},
	{"Language (F-key)", "lsp.definition", "Go to definition"},
}

// KeysFor returns the keys bound to an action, shortest first.
func (m *Map) KeysFor(action string) []string {
	seen := map[string]bool{}
	for _, keys := range m.bindings {
		for key, bound := range keys {
			if bound == action {
				seen[key] = true
			}
		}
	}
	return slices.SortedFunc(maps.Keys(seen), func(a, b string) int {
		if len(a) != len(b) {
			return len(a) - len(b)
		}
		return strings.Compare(a, b)
	})
}

// Bind writes key = action into a keys.toml, and turns off the action's other keys, so that the menu
// shows one key and means it. Every other line is left as it is.
func (m *Map) Bind(path, key, action string) error {
	if key == Reserved {
		return errors.New(Reserved + " is reserved for leaving terminal tabs")
	}
	if len([]rune(key)) == 1 {
		return errors.New("a plain character would stop being typed: add Ctrl or Alt")
	}
	if len(contextsOf(action)) == 0 {
		return fmt.Errorf("%q isn't an action", action)
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		data, err = []byte(Template), nil
	}
	if err != nil {
		return err
	}
	changes := map[string]string{key: action}
	for _, old := range m.KeysFor(action) {
		if old != key && len([]rune(old)) > 1 {
			changes[old] = "none"
		}
	}
	var kept []string
	for _, line := range strings.Split(strings.TrimRight(string(data), "\r\n"), "\n") {
		name, _, found := strings.Cut(line, "=")
		if _, changing := changes[strings.Trim(strings.TrimSpace(name), `"`)]; !found || !changing {
			kept = append(kept, line)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(changes)) {
		kept = append(kept, fmt.Sprintf("%q = %q", name, changes[name]))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0o644)
}

// extraChords add to Ctrl+K then key, which is the second form of every Alt binding on
// macOS, where Option types special characters by default (Q-80). This one has no Alt form,
// because Ctrl+F2 opens the menu bar there.
var extraChords = map[string]string{"o": "select.all_occurrences"}

// Map resolves keys to actions.
type Map struct {
	bindings map[Context]map[string]string
}

// Default returns the default bindings.
func Default() *Map {
	m := &Map{bindings: map[Context]map[string]string{}}
	for context, keys := range defaults {
		m.bindings[context] = maps.Clone(keys)
	}
	return m
}

func contextsOf(action string) []Context {
	var found []Context
	for _, context := range []Context{Global, Editor, Panel} {
		for _, known := range defaults[context] {
			if known == action {
				found = append(found, context)
				break
			}
		}
	}
	return found
}

// Actions returns every action name, in order.
func Actions() []string {
	seen := map[string]bool{}
	for _, keys := range defaults {
		for _, action := range keys {
			seen[action] = true
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

// Lookup returns the action for a named key, checking the context before Global.
func (m *Map) Lookup(context Context, key string) string {
	if action := m.bindings[context][key]; action != "" {
		return action
	}
	return m.bindings[Global][key]
}

// LookupChord returns the action for Ctrl+K then key.
func (m *Map) LookupChord(context Context, key string) string {
	if action := m.Lookup(context, "alt+"+key); action != "" {
		return action
	}
	// Shift+Alt+F is the chord's capital F: a capital letter has no shift of its own in a name.
	if r := []rune(key); len(r) == 1 && unicode.IsUpper(r[0]) {
		if action := m.Lookup(context, "alt+shift+"+strings.ToLower(key)); action != "" {
			return action
		}
	}
	return extraChords[key]
}

// Load applies a keys.toml over the defaults. A missing file changes nothing. Bindings
// it can't use are reported together in the error, and the good ones still apply.
func Load(path string) (*Map, error) {
	m := Default()
	name := filepath.Base(path) // Short, so that a one-line message reaches the problem.
	var file struct {
		Keys map[string]string `toml:"keys"`
	}
	if _, err := toml.DecodeFile(path, &file); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return m, nil
		}
		var parse toml.ParseError
		if errors.As(err, &parse) {
			return m, fmt.Errorf("%s, line %d: %s", name, parse.Position.Line, parse.Message)
		}
		return m, fmt.Errorf("%s: %w", name, err)
	}
	var problems []string
	seen := map[string]string{}
	for _, raw := range slices.Sorted(maps.Keys(file.Keys)) {
		action := file.Keys[raw]
		key, err := normalize(raw)
		switch {
		case err != nil:
			problems = append(problems, err.Error())
		case key == Reserved:
			problems = append(problems, Reserved+" is reserved for leaving terminal tabs")
		case seen[key] != "":
			problems = append(problems, fmt.Sprintf("%q and %q are the same key", seen[key], raw))
		case action == "none":
			seen[key] = raw
			for _, context := range []Context{Global, Editor, Panel} {
				delete(m.bindings[context], key)
			}
		default:
			contexts := contextsOf(action)
			if len(contexts) == 0 {
				problems = append(problems, fmt.Sprintf("%q isn't an action", action))
				continue
			}
			seen[key] = raw
			// A key means one thing wherever the action works, so a global binding
			// also has to replace what the key did in a file or the tree.
			if slices.Contains(contexts, Global) {
				delete(m.bindings[Editor], key)
				delete(m.bindings[Panel], key)
			}
			for _, context := range contexts {
				m.bindings[context][key] = action
			}
		}
	}
	if len(problems) > 0 {
		return m, fmt.Errorf("%s: %s", name, strings.Join(problems, "; "))
	}
	return m, nil
}

var specialKeys = map[tcell.Key]string{
	tcell.KeyEnter: "enter", tcell.KeyTab: "tab", tcell.KeyEsc: "esc", tcell.KeyBackspace: "backspace",
	tcell.KeyBackspace2: "backspace", tcell.KeyDelete: "delete", tcell.KeyInsert: "insert",
	tcell.KeyUp: "up", tcell.KeyDown: "down", tcell.KeyLeft: "left", tcell.KeyRight: "right",
	tcell.KeyHome: "home", tcell.KeyEnd: "end", tcell.KeyPgUp: "pgup", tcell.KeyPgDn: "pgdn",
	tcell.KeyF1: "f1", tcell.KeyF2: "f2", tcell.KeyF3: "f3", tcell.KeyF4: "f4", tcell.KeyF5: "f5", tcell.KeyF6: "f6",
	tcell.KeyF7: "f7", tcell.KeyF8: "f8", tcell.KeyF9: "f9", tcell.KeyF10: "f10", tcell.KeyF11: "f11", tcell.KeyF12: "f12",
	tcell.KeyCtrlBackslash: `ctrl+\`,
}

var knownNames = func() map[string]bool {
	names := map[string]bool{}
	for _, name := range specialKeys {
		names[name] = true
	}
	return names
}()

// Name returns a key press's canonical name, such as "ctrl+shift+left" or "alt+,". A
// plain character is itself, and is only special if something binds it.
func Name(ev *tcell.EventKey) string {
	mods := ev.Modifiers()
	key := ev.Key()
	base := ""
	switch {
	case key == tcell.KeyBacktab:
		base, mods = "tab", mods|tcell.ModShift
	case specialKeys[key] != "":
		base = specialKeys[key]
		if base == Reserved {
			return Reserved
		}
	case key == tcell.KeyRune:
		r := ev.Rune()
		if mods&tcell.ModCtrl != 0 && (r == '\\' || r == '|') {
			return Reserved // Windows delivers it this way.
		}
		if unicode.IsUpper(r) && mods&(tcell.ModAlt|tcell.ModCtrl) != 0 {
			mods |= tcell.ModShift
		}
		if mods&(tcell.ModAlt|tcell.ModCtrl) != 0 {
			r = unicode.ToLower(r)
		} else {
			mods &^= tcell.ModShift // The character already says so.
		}
		base = string(r)
	case key >= tcell.KeyCtrlA && key <= tcell.KeyCtrlZ:
		base, mods = string('a'+rune(key-tcell.KeyCtrlA)), mods|tcell.ModCtrl
	default:
		return ""
	}
	return prefix(mods&tcell.ModCtrl != 0, mods&tcell.ModAlt != 0, mods&tcell.ModShift != 0) + base
}

func prefix(ctrl, alt, shift bool) string {
	out := ""
	if ctrl {
		out += "ctrl+"
	}
	if alt {
		out += "alt+"
	}
	if shift {
		out += "shift+"
	}
	return out
}

// normalize turns a key written in a config file into its canonical name.
func normalize(raw string) (string, error) {
	text := strings.ToLower(strings.TrimSpace(raw))
	bad := fmt.Errorf("%q isn't a key", raw)
	var ctrl, alt, shift bool
	for {
		switch {
		case strings.HasPrefix(text, "ctrl+") && !ctrl:
			ctrl, text = true, text[5:]
			continue
		case strings.HasPrefix(text, "alt+") && !alt:
			alt, text = true, text[4:]
			continue
		case strings.HasPrefix(text, "shift+") && !shift:
			shift, text = true, text[6:]
			continue
		}
		break
	}
	switch {
	case text == "":
		return "", bad
	case knownNames[text]:
	case len([]rune(text)) != 1:
		return "", bad
	}
	return prefix(ctrl, alt, shift) + text, nil
}
