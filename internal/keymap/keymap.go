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

// Reserved leaves a terminal tab, so it can't be rebound (Q-74). Toggle does the same from one hand
// (Ctrl+\ is AltGr+º on a Spanish keyboard), and goes back in when pressed again.
const (
	Reserved = `ctrl+\`
	Toggle   = "ctrl+space"
)

// IsReserved says whether a key is one of the two that can't be bound.
func IsReserved(key string) bool { return key == Reserved || key == Toggle }

// FromTerminal are the actions that work inside a terminal or AI tab, because switching should never
// need a key to leave first. Everything else goes to the tool there.
var FromTerminal = []string{"tab.close", "focus.next", "project.switch", "directory.switch", "view.move"}

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
//	Ctrl+Space         in and out: leave a terminal or file for the panel, and back in (Ctrl+\ only leaves)
//	Alt+letter         go somewhere or show something: Explorer, Git, Find, AI, Terminal, Projects, Folders, Run
//	Alt+Shift+letter   make something: Project, Folder, Terminal, AI session, Split
//	Alt+, Alt+. Alt+W  move: previous and next tab (the < and > keys), next area (panels, splits, dock)
//	Ctrl+letter        this file and tab: save, find, replace, new file, close
//	F-key              the selected thing: F2 rename, F3 format, F12 definition. F1 and F10 are help and menu
//
// Switching project (Alt+P), folder (Alt+D), area (Alt+W) and the tab's place (Alt+V) work inside a
// terminal tab too (FromTerminal). Alt+P is also Claude Code's model picker and Alt+D bash's delete-word.

// Alt+arrows would be the obvious moving keys, and Alt+Shift+D the obvious directory key, but Windows
// Terminal keeps those for its own panes (doc 07).
var defaults = map[Context]map[string]string{
	Global: {
		"alt+e": "view.files", "alt+g": "view.git", "alt+f": "view.search", "alt+a": "view.sessions", "alt+t": "view.terminal",
		"alt+p": "project.switch", "alt+d": "directory.switch", "alt+r": "run.play",
		"alt+m": "app.menu", "alt+s": "app.settings", "alt+k": "app.shortcuts", "alt+x": "command.open", "alt+z": "view.wrap",
		"alt+shift+p": "project.new", "alt+shift+f": "directory.add", "alt+shift+t": "terminal.new", "alt+shift+a": "session.new",
		"alt+shift+s": "split.right", "alt+shift+b": "split.down",
		"alt+,": "tab.previous", "alt+.": "tab.next", "alt+w": "focus.next", "alt+tab": "focus.next", "alt+v": "view.move",
		"ctrl+b": "panel.toggle_second", "ctrl+w": "tab.close", "alt+q": "tab.close", "ctrl+p": "file.open_by_name", "ctrl+n": "file.new", "ctrl+q": "app.quit",
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
		"alt+up": "cursor.add_above", "alt+down": "cursor.add_below", "ctrl+alt+up": "cursor.add_above", "ctrl+alt+down": "cursor.add_below",
		"ctrl+shift+up": "line.move_up", "ctrl+shift+down": "line.move_down", "alt+shift+up": "line.copy_up", "alt+shift+down": "line.copy_down",
		"ctrl+shift+k": "line.delete", "ctrl+enter": "line.insert_below", "ctrl+j": "line.insert_below", "ctrl+shift+enter": "line.insert_above",
		"ctrl+l": "select.line", "ctrl+shift+l": "select.all_occurrences", "ctrl+/": "edit.toggle_comment", "ctrl+g": "file.go_to_line",
		"ctrl+backspace": "edit.delete_word_left", "alt+backspace": "edit.delete_word_left", "ctrl+delete": "edit.delete_word_right",
	},
	Panel: {
		"up": "tree.up", "down": "tree.down", "left": "tree.close", "right": "tree.open",
		"enter": "tree.select", "tab": "tab.enter", "r": "tree.refresh", ":": "command.open",
		"n": "panel.new", "f2": "lsp.rename", "delete": "session.hide", "ctrl+z": "tree.undo_delete",
	},
}

// Entry is one row of the shortcuts menu.
type Entry struct{ Family, Action, Label string }

// Catalog is what the shortcuts menu lists, by task. The cursor and typing keys stay out of it.
var Catalog = []Entry{
	{"Move around", "project.switch", "Switch project"},
	{"Move around", "directory.switch", "Switch folder"},
	{"Move around", "file.open_by_name", "Go to a file by name"},
	{"Move around", "file.go_to_line", "Go to line"},
	{"Move around", "focus.next", "Next area: panels, splits, dock"},
	{"Move around", "tab.previous", "Previous tab"},
	{"Move around", "tab.next", "Next tab"},
	{"Move around", "tab.close", "Close the tab"},
	{"Move around", "view.move", "Move to the other panel, or a terminal to the other area"},
	{"Move around", "command.open", "Command bar"},
	{"Move around", "app.menu", "Menu bar"},
	{"Show", "view.files", "Files"},
	{"Show", "view.git", "Git"},
	{"Show", "view.search", "Find in this folder"},
	{"Show", "view.sessions", "AI sessions"},
	{"Show", "view.terminal", "Terminal dock, show or hide"},
	{"Show", "panel.toggle_second", "Second panel, open or close"},
	{"Show", "view.wrap", "Word wrap, on or off"},
	{"Make and run", "project.new", "New project"},
	{"Make and run", "directory.add", "Add a folder"},
	{"Make and run", "file.new", "New file"},
	{"Make and run", "terminal.new", "New terminal"},
	{"Make and run", "session.new", "New AI session"},
	{"Make and run", "split.right", "Split right"},
	{"Make and run", "split.down", "Split below"},
	{"Make and run", "run.play", "Run this folder"},
	{"This file", "file.save", "Save"},
	{"This file", "find.open", "Find"},
	{"This file", "find.replace", "Replace"},
	{"This file", "edit.undo", "Undo"},
	{"This file", "edit.redo", "Redo"},
	{"This file", "edit.copy", "Copy"},
	{"This file", "edit.cut", "Cut"},
	{"This file", "edit.paste", "Paste"},
	{"This file", "select.all", "Select all"},
	{"This file", "select.next_occurrence", "Add the next match"},
	{"Lines and cursors", "cursor.add_above", "Add a cursor above"},
	{"Lines and cursors", "cursor.add_below", "Add a cursor below"},
	{"Lines and cursors", "line.move_up", "Move the line up"},
	{"Lines and cursors", "line.move_down", "Move the line down"},
	{"Lines and cursors", "line.copy_up", "Copy the line up"},
	{"Lines and cursors", "line.copy_down", "Copy the line down"},
	{"Lines and cursors", "line.delete", "Delete the line"},
	{"Lines and cursors", "line.insert_below", "New line below"},
	{"Lines and cursors", "line.insert_above", "New line above"},
	{"Lines and cursors", "select.line", "Select the line"},
	{"Lines and cursors", "edit.toggle_comment", "Comment or uncomment"},
	{"Lines and cursors", "edit.delete_word_left", "Delete the word before"},
	{"Lines and cursors", "edit.delete_word_right", "Delete the word after"},
	{"Language", "lsp.rename", "Rename the symbol"},
	{"Language", "lsp.format", "Format the file"},
	{"Language", "lsp.definition", "Go to definition"},
	{"App", "app.settings", "Settings"},
	{"App", "app.shortcuts", "These shortcuts"},
	{"App", "app.quit", "Quit"},
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
	if IsReserved(key) {
		return errors.New(key + " is reserved for leaving terminal tabs")
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
		case IsReserved(key):
			problems = append(problems, key+" is reserved for leaving terminal tabs")
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
	tcell.KeyCtrlBackslash: `ctrl+\`, tcell.KeyCtrlSpace: Toggle,
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
		if IsReserved(base) {
			return base
		}
	case key == tcell.KeyRune:
		r := ev.Rune()
		if mods&tcell.ModCtrl != 0 && (r == '\\' || r == '|') {
			return Reserved // Windows delivers it this way.
		}
		if mods&tcell.ModCtrl != 0 && r == ' ' {
			return Toggle
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
	case key == tcell.KeyCtrlUnderscore: // What terminals send for Ctrl+/.
		base, mods = "/", mods|tcell.ModCtrl
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
	case text == "space" && ctrl && !alt && !shift:
		return Toggle, nil
	case knownNames[text]:
	case len([]rune(text)) != 1:
		return "", bad
	}
	return prefix(ctrl, alt, shift) + text, nil
}
