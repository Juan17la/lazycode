package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Juan17la/lazycode/internal/buffer"
	"github.com/Juan17la/lazycode/internal/lsp"
	"github.com/Juan17la/lazycode/internal/plugin"
	"github.com/Juan17la/lazycode/internal/store"
	"github.com/Juan17la/lazycode/internal/syntax"
)

// pluginState holds the running plugins. They only ever come from plugins/ in the config
// folder, after the user accepted them, so opening a project can't run its code (Q-67).
type pluginState struct {
	loaded   []*plugin.Plugin
	consents *plugin.Consents
}

func (a *app) pluginsDir() string { return a.settingsPath("plugins") }

// loadPlugins starts accepted plugins and asks about new ones. A broken plugin only costs a message.
func (a *app) loadPlugins() {
	var problems []string
	consents, err := plugin.OpenConsents(a.settingsPath("plugins.toml"))
	if err != nil {
		problems = append(problems, "plugins.toml: "+err.Error())
	}
	a.plugins.consents = consents
	store.CleanLeftovers(a.pluginsDir())
	entries, err := os.ReadDir(a.pluginsDir())
	if err != nil && !os.IsNotExist(err) {
		problems = append(problems, "Plugins not loaded: "+err.Error())
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		dir := filepath.Join(a.pluginsDir(), entry.Name())
		m, err := readInstalled(dir)
		switch {
		case err != nil:
			problems = append(problems, err.Error())
		case consents.Accepted(m):
			if err := a.startPlugin(dir); err != nil {
				problems = append(problems, err.Error())
			}
		case !consents.Declined(m):
			a.askLocal(m, dir)
		}
	}
	if len(problems) > 0 {
		a.message = strings.TrimSpace(strings.Join(problems, "  ") + "  " + a.message)
	}
}

// readInstalled also checks the folder's name, because an answer is remembered by name.
func readInstalled(dir string) (plugin.Manifest, error) {
	m, err := plugin.ReadManifest(dir)
	if err == nil && m.Name != filepath.Base(dir) {
		err = fmt.Errorf("the plugin in the folder %s calls itself %s, and they must match", filepath.Base(dir), m.Name)
	}
	return m, err
}

func (a *app) startPlugin(dir string) error {
	p, err := plugin.Load(dir, plugin.Host{
		Message: func(text string) { a.message = text },
		Buffer: func() plugin.Buffer {
			if it := a.tab(); it != nil && it.ed != nil {
				return pluginBuffer{a, it}
			}
			return nil
		},
	})
	if err != nil {
		return err
	}
	a.plugins.loaded = append(a.plugins.loaded, p)
	a.syncLanguagePacks()
	return nil
}

func (a *app) syncLanguagePacks() {
	var servers []lsp.Pack
	var colours []syntax.Pack
	for _, p := range a.plugins.loaded {
		l := p.Lang
		if l == nil {
			continue
		}
		if l.Server.Command != "" {
			servers = append(servers, lsp.Pack{Extensions: l.Extensions, Server: lsp.Server{
				Name: l.Name, Command: l.Server.Command, Args: l.Server.Args, Markers: l.Server.Markers,
			}})
		}
		if l.Colours != nil {
			colours = append(colours, syntax.Pack{Name: l.Name, Extensions: l.Extensions, Rules: l.Colours})
		}
	}
	lsp.SetPacks(servers)
	syntax.SetPacks(colours)
}

// askPlugin is the consent prompt (Q-67). The description is the author's and can't be checked,
// so the warning is always part of the question. Esc declines. On accepting, prepare says which
// folder to run, and the plugin's older copy stops first.
func (a *app) askPlugin(m plugin.Manifest, prepare func() (string, error), declined func()) {
	accept := func() {
		a.stopPlugin(m.Name)
		dir, err := prepare()
		if err == nil {
			err = a.plugins.consents.Set(m, true) // It must not run if it would be forgotten.
		}
		if err == nil {
			err = a.startPlugin(dir)
		}
		if err != nil {
			a.message = "Couldn't add " + m.Name + ": " + err.Error()
			return
		}
		a.message = "Added " + m.Name + " " + m.Version + "."
	}
	a.ask(fmt.Sprintf("Add plugin %s %s? %s Plugins can read your files and run any command. Only accept plugins you trust.", m.Name, m.Version, m.Does),
		choice{'a', "Accept", accept},
		choice{'d', "Decline", declined})
}

// askLocal asks about a plugin already in the plugins folder, and remembers a no.
func (a *app) askLocal(m plugin.Manifest, dir string) {
	a.askPlugin(m, func() (string, error) { return dir, nil }, func() {
		if err := a.plugins.consents.Set(m, false); err != nil {
			a.message = "Couldn't remember the answer: " + err.Error()
		}
	})
}

func (a *app) closePlugins() {
	for _, p := range a.plugins.loaded {
		p.Close()
	}
	a.plugins.loaded = nil
}

func (a *app) stopPlugin(name string) {
	a.plugins.loaded = slices.DeleteFunc(a.plugins.loaded, func(p *plugin.Plugin) bool {
		if p.Name != name {
			return false
		}
		p.Close()
		return true
	})
	a.syncLanguagePacks()
}

// runPlugin runs a plugin's command, and reports whether one has it.
func (a *app) runPlugin(command string) bool {
	for _, p := range a.plugins.loaded {
		if slices.Contains(p.Commands(), command) {
			if p.Err != nil {
				a.message = "Plugin " + p.Name + " " + p.Err.Error()
				return true
			}
			p.Run(command)
			return true
		}
	}
	return false
}

// pluginKey finds the command a plugin bound to a key. The user's own bindings come first.
func (a *app) pluginKey(name string) string {
	for _, p := range a.plugins.loaded {
		if command := p.Keys()[name]; command != "" && p.Err == nil {
			return command
		}
	}
	return ""
}

func (a *app) emitPlugins(event string, it *tabItem) {
	if it == nil || it.ed == nil || it.ed.File.ReadOnly() {
		return
	}
	for _, p := range a.plugins.loaded {
		p.Emit(event, pluginBuffer{a, it})
	}
}

func (a *app) tabOf(f *buffer.File) *tabItem {
	for _, st := range a.dirs {
		for _, p := range st.root.panes() {
			for _, it := range p.tabs {
				if it.ed != nil && it.ed.File == f {
					return it
				}
			}
		}
	}
	return nil
}

// pluginCommand is :plugin, :plugin on NAME and :plugin off NAME.
func (a *app) pluginCommand(args []string) {
	switch {
	case len(args) == 0:
		a.message = a.pluginSummary()
	case len(args) == 2 && args[0] == "off":
		a.pluginOff(args[1])
	case len(args) == 2 && args[0] == "on":
		a.pluginOn(args[1])
	default:
		a.message = "Use :plugin, :plugin on NAME, or :plugin off NAME."
	}
}

func (a *app) pluginSummary() string {
	if len(a.plugins.loaded) == 0 {
		return "No plugins are running. They go in " + a.pluginsDir()
	}
	var names []string
	for _, p := range a.plugins.loaded {
		name := p.Name + " " + p.Version
		if p.Err != nil {
			name += " (disabled)"
		}
		names = append(names, name)
	}
	return "Plugins: " + strings.Join(names, ", ")
}

// installed reads a plugin by the name the user typed, which must not lead out of the folder.
func (a *app) installed(name string) (plugin.Manifest, string, error) {
	dir := filepath.Join(a.pluginsDir(), filepath.Base(name))
	m, err := readInstalled(dir)
	return m, dir, err
}

func (a *app) pluginOff(name string) {
	m, _, err := a.installed(name)
	if err != nil {
		a.message = "Can't turn off " + name + ": " + err.Error()
		return
	}
	a.stopPlugin(m.Name)
	if err := a.plugins.consents.Set(m, false); err != nil {
		a.message = "Stopped " + m.Name + ", but it will come back: " + err.Error()
		return
	}
	a.message = "Turned off " + m.Name + "."
}

func (a *app) pluginOn(name string) {
	m, dir, err := a.installed(name)
	if err != nil {
		a.message = "Can't turn on " + name + ": " + err.Error()
		return
	}
	a.askLocal(m, dir)
}

// pluginBuffer gives a plugin one tab's file.
type pluginBuffer struct {
	a  *app
	it *tabItem
}

func (b pluginBuffer) Path() string { return b.it.ed.File.Path }
func (b pluginBuffer) Len() int     { return b.it.ed.File.Len() }

func (b pluginBuffer) Text(start, end int) (string, error) {
	f := b.it.ed.File
	if start < 0 || end < start || end > f.Len() {
		return "", fmt.Errorf("%d to %d is outside the file, which has %d bytes", start, end, f.Len())
	}
	return string(f.Slice(start, end)), nil
}

func (b pluginBuffer) Replace(start, end int, text string) error {
	if err := b.it.ed.ReplaceRange(start, end, text); err != nil {
		return err
	}
	b.a.edited(b.it)
	return nil
}
