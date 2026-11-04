// Package keymap names keys, holds the default bindings and applies the user's (Q-24).
package keymap

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
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
# The actions are listed in the README.
[keys]
`

// defaults lists every action a key can have, per context.
var defaults = map[Context]map[string]string{
	Global: {
		"alt+o": "project.switch", "alt+d": "directory.switch",
		"alt+,": "tab.previous", "alt+.": "tab.next",
		"alt+s": "split.right", "alt+w": "split.next",
		"ctrl+b": "panel.toggle_second", "ctrl+w": "tab.close",
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
		"ctrl+s": "file.save", "ctrl+f": "find.open", "alt+h": "find.replace",
	},
	Panel: {
		"up": "tree.up", "down": "tree.down", "left": "tree.close", "right": "tree.open",
		"enter": "tree.select", "tab": "tab.enter", "r": "tree.refresh", ":": "command.open",
		"alt+v": "view.move",
	},
}

// extraChords add to Ctrl+K then key, which is the second form of every Alt binding on
// macOS, where Option types special characters by default (Q-80). This one has no Alt form,
// because Ctrl+F2 opens the menu bar there.
var extraChords = map[string]string{"a": "select.all_occurrences"}

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
