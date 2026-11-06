// Package plugin runs Lua plugins (doc 09). Each plugin has its own Lua state, and a
// plugin that fails is disabled without touching the editor. There is no sandbox: Load
// runs the plugin's code, so the caller must have the user's consent first (Q-67).
package plugin

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/BurntSushi/toml"
	lua "github.com/yuin/gopher-lua"
)

// APIVersion is the version of the API below. Doc 09: the current and the previous
// version are supported, and there is no previous one yet.
const APIVersion = 1

const (
	callLimit = 100 * time.Millisecond // A call runs on the UI goroutine, so it holds up typing.
	loadLimit = time.Second
	maxSlow   = 3 // Calls stopped for time before the plugin is disabled.
)

// Manifest is plugin.toml.
type Manifest struct {
	Name        string `toml:"name"`
	Version     string `toml:"version"`
	APIVersion  int    `toml:"api_version"`
	Description string `toml:"description"`
	Does        string `toml:"does"`
}

// Buffer is the file a plugin works on. Positions here are byte offsets from 0, with end
// excluded.
type Buffer interface {
	Path() string
	Len() int
	Text(start, end int) (string, error)
	Replace(start, end int, text string) error
}

// Host is what a plugin can reach in the editor.
type Host struct {
	Message func(string)
	Buffer  func() Buffer // Nil result: the current tab isn't a file.
}

// Plugin is one loaded plugin.
type Plugin struct {
	Manifest
	Err error // Why it was disabled. A disabled plugin never runs again.

	host     Host
	l        *lua.LState
	limit    time.Duration
	slow     int
	commands map[string]*lua.LFunction
	handlers map[string][]*lua.LFunction
	keys     map[string]string
}

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// maxText keeps a description short enough that the warning after it stays on screen.
const maxText = 300

// clean removes control characters, which could redraw the screen, and limits the length.
func clean(text string, limit int) string {
	r := []rune(strings.Join(strings.FieldsFunc(text, unicode.IsControl), " "))
	return strings.TrimSpace(string(r[:min(len(r), limit)]))
}

// ReadManifest reads and checks plugin.toml without running anything, so that the consent
// prompt can show it.
func ReadManifest(dir string) (Manifest, error) {
	var m Manifest
	if _, err := toml.DecodeFile(filepath.Join(dir, "plugin.toml"), &m); err != nil {
		return m, fmt.Errorf("plugin.toml: %w", err)
	}
	// All of this is the author's text, and ends up on screen in the consent prompt.
	m.Version, m.Description, m.Does = clean(m.Version, 40), clean(m.Description, maxText), clean(m.Does, maxText)
	switch {
	case !namePattern.MatchString(m.Name): // It becomes a folder name when installed.
		return m, fmt.Errorf("plugin.toml: name %q isn't one word of letters, digits, dots, dashes or underscores", m.Name)
	case m.Version == "":
		return m, fmt.Errorf("plugin.toml: %s has no version", m.Name)
	case m.Does == "":
		return m, fmt.Errorf("plugin.toml: %s doesn't say what it does", m.Name)
	case m.APIVersion != APIVersion:
		return m, fmt.Errorf("%s needs plugin API %d, and this LazyCode has %d", m.Name, m.APIVersion, APIVersion)
	}
	return m, nil
}

// Load runs the plugin's main.lua. The caller must have the user's consent.
func Load(dir string, host Host) (*Plugin, error) {
	m, err := ReadManifest(dir)
	if err != nil {
		return nil, err
	}
	p := &Plugin{
		Manifest: m,
		host:     host,
		l:        lua.NewState(),
		limit:    callLimit,
		commands: map[string]*lua.LFunction{},
		handlers: map[string][]*lua.LFunction{},
		keys:     map[string]string{},
	}
	p.l.PreloadModule("lazycode", p.module)
	registerBuffer(p.l)
	fn, err := p.l.LoadFile(filepath.Join(dir, "main.lua"))
	if err == nil {
		err = p.protected(loadLimit, fn)
	}
	if err != nil {
		p.l.Close()
		return nil, fmt.Errorf("%s: %w", m.Name, err)
	}
	return p, nil
}

// Close frees the Lua state.
func (p *Plugin) Close() { p.l.Close() }

// Commands lists the plugin's command names.
func (p *Plugin) Commands() []string {
	out := make([]string, 0, len(p.commands))
	for name := range p.commands {
		out = append(out, name)
	}
	return out
}

// Keys returns the plugin's key bindings, from key name to command.
func (p *Plugin) Keys() map[string]string { return p.keys }

// Run runs a command, and reports whether the plugin has it.
func (p *Plugin) Run(command string) bool {
	fn, ok := p.commands[command]
	if ok {
		p.call(fn)
	}
	return ok
}

// Emit tells the plugin about an event.
func (p *Plugin) Emit(event string, buf Buffer) {
	for _, fn := range p.handlers[event] {
		p.call(fn, newBuffer(p.l, buf))
	}
}

func (p *Plugin) call(fn *lua.LFunction, args ...lua.LValue) {
	if p.Err != nil {
		return
	}
	err := p.protected(p.limit, fn, args...)
	switch {
	case err == nil:
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), context.DeadlineExceeded.Error()):
		p.slow++
		if p.slow >= maxSlow {
			p.Err = fmt.Errorf("disabled: it ran over the time limit %d times", p.slow)
		}
	default:
		p.Err = fmt.Errorf("disabled: %w", err)
	}
	if p.Err != nil && p.host.Message != nil {
		p.host.Message("Plugin " + p.Name + " " + p.Err.Error())
	}
}

// protected runs fn with a time limit. Lua errors and panics come back as errors.
// ponytail: the limit is checked between Lua instructions, so os.execute("sleep 9") still
// blocks the editor. Running plugins on their own goroutine is the upgrade if that bites.
func (p *Plugin) protected(limit time.Duration, fn *lua.LFunction, args ...lua.LValue) error {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	p.l.SetContext(ctx)
	defer p.l.RemoveContext()
	defer p.l.SetTop(0)
	return p.l.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, args...)
}

var events = map[string]bool{"buffer_save": true, "buffer_open": true}

func (p *Plugin) module(l *lua.LState) int {
	l.Push(l.SetFuncs(l.NewTable(), map[string]lua.LGFunction{
		"command": func(l *lua.LState) int {
			name := l.CheckString(1)
			if name == "" || strings.ContainsAny(name, " \t\r\n") {
				l.ArgError(1, "a command name is one word")
			}
			p.commands[name] = l.CheckFunction(2)
			return 0
		},
		"on": func(l *lua.LState) int {
			event := l.CheckString(1)
			if !events[event] {
				l.ArgError(1, "unknown event "+event)
			}
			p.handlers[event] = append(p.handlers[event], l.CheckFunction(2))
			return 0
		},
		"key": func(l *lua.LState) int {
			p.keys[l.CheckString(1)] = l.CheckString(2)
			return 0
		},
		"message": func(l *lua.LState) int {
			if p.host.Message != nil {
				p.host.Message(l.CheckString(1))
			}
			return 0
		},
		"current_buffer": func(l *lua.LState) int {
			if p.host.Buffer != nil {
				if buf := p.host.Buffer(); buf != nil {
					l.Push(newBuffer(l, buf))
					return 1
				}
			}
			l.Push(lua.LNil)
			return 1
		},
	}))
	return 1
}

const bufferType = "lazycode.buffer"

func registerBuffer(l *lua.LState) {
	mt := l.NewTypeMetatable(bufferType)
	l.SetField(mt, "__index", l.SetFuncs(l.NewTable(), map[string]lua.LGFunction{
		"path": func(l *lua.LState) int {
			l.Push(lua.LString(checkBuffer(l).Path()))
			return 1
		},
		"len": func(l *lua.LState) int {
			l.Push(lua.LNumber(checkBuffer(l).Len()))
			return 1
		},
		// Like string.sub: from 1, end included, and the whole file by default.
		"text": func(l *lua.LState) int {
			buf := checkBuffer(l)
			text, err := buf.Text(l.OptInt(2, 1)-1, l.OptInt(3, buf.Len()))
			if err != nil {
				l.RaiseError("%s", err.Error())
			}
			l.Push(lua.LString(text))
			return 1
		},
		// replace(5, 4, "x") inserts before byte 5.
		"replace": func(l *lua.LState) int {
			if err := checkBuffer(l).Replace(l.CheckInt(2)-1, l.CheckInt(3), l.CheckString(4)); err != nil {
				l.RaiseError("%s", err.Error())
			}
			return 0
		},
	}))
}

func newBuffer(l *lua.LState, buf Buffer) *lua.LUserData {
	ud := l.NewUserData()
	ud.Value = buf
	l.SetMetatable(ud, l.GetTypeMetatable(bufferType))
	return ud
}

func checkBuffer(l *lua.LState) Buffer {
	if buf, ok := l.CheckUserData(1).Value.(Buffer); ok {
		return buf
	}
	l.ArgError(1, "not a buffer")
	return nil
}
