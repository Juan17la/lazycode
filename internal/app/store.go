package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Juan17la/lazycode/internal/plugin"
	"github.com/Juan17la/lazycode/internal/store"
)

// storeState is the plugin store (doc 09). It only connects when the user opens it (doc 11).
type storeState struct {
	client *store.Client
	busy   bool
}

// storeCommand is :store: it fetches the index, then shows it in the switcher.
func (a *app) storeCommand() {
	if a.storeApp.busy {
		a.message = "The store is still working."
		return
	}
	a.storeApp.busy = true
	a.message = "Opening the store..."
	client, ctx := a.storeApp.client, a.upd.ctx
	go func() {
		entries, err := client.List(ctx)
		a.post(func() {
			a.storeApp.busy = false
			if err != nil {
				a.message = "Couldn't open the store: " + err.Error()
				return
			}
			a.showStore(entries)
		})
	}()
}

func (a *app) showStore(entries []store.Entry) {
	if len(entries) == 0 {
		a.message = "The store has no plugins yet."
		return
	}
	s := &switcher{title: "Plugin store"}
	for _, e := range entries {
		label := e.Name + " " + e.Version + "  " + e.Description
		if m, _, err := a.installed(e.Name); err == nil {
			if m.Version == e.Version {
				label += "  (installed)"
			} else {
				label += "  (update from " + m.Version + ")"
			}
		}
		s.labels = append(s.labels, label)
	}
	s.pick = func(i int) { a.storeInstall(entries[i]) }
	a.message = ""
	a.switcher = s
}

// storeInstall downloads first, so that the prompt shows what's really in the plugin. Nothing runs
// or moves into place until the user accepts.
func (a *app) storeInstall(e store.Entry) {
	if m, _, err := a.installed(e.Name); err == nil && m.Version == e.Version && a.plugins.consents.Accepted(m) {
		a.message = e.Name + " " + e.Version + " is already installed."
		return
	}
	a.storeApp.busy = true
	a.message = "Downloading " + e.Name + "..."
	client, ctx, parent := a.storeApp.client, a.upd.ctx, a.pluginsDir()
	go func() {
		dir, m, err := client.Download(ctx, e, parent)
		a.post(func() {
			a.storeApp.busy = false
			if err != nil {
				a.message = "Couldn't install " + e.Name + ": " + err.Error()
				return
			}
			a.askStorePlugin(m, dir)
		})
	}()
}

// askStorePlugin leaves any older copy running if the user declines.
func (a *app) askStorePlugin(m plugin.Manifest, downloaded string) {
	a.askPlugin(m, func() (string, error) {
		target := filepath.Join(a.pluginsDir(), m.Name)
		if err := store.Install(downloaded, target); err != nil {
			return "", fmt.Errorf("can't move it into place: %w", err)
		}
		return target, nil
	}, func() {
		os.RemoveAll(downloaded) //nolint:errcheck // Cleaned at the next start otherwise.
	})
}
