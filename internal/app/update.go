package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Juan17la/lazycode/internal/update"
)

// updates is the state for finding and installing new versions (Q-71, Q-79).
type updates struct {
	version string // "dev" for a build that wasn't released. It never checks or updates.
	updater *update.Updater
	stamp   string // Where the time of the last check is kept. Empty means the cache folder.
	target  string // The file :update replaces. Empty means the running program.
	running bool   // An :update is in progress.
	note    string // The "available" message, waiting for a quiet moment to show.
	ctx     context.Context
	cancel  context.CancelFunc // Stops a check or download when the app quits.
}

func newUpdates() updates {
	u := updates{version: "dev", updater: update.New()}
	u.ctx, u.cancel = context.WithCancel(context.Background())
	return u
}

// stampPath is in the cache folder, which the OS can clear, and then LazyCode checks sooner (doc 08).
func (a *app) stampPath() (string, error) {
	if a.upd.stamp != "" {
		return a.upd.stamp, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lazycode", "last-update-check"), nil
}

// checkDue says whether a day has passed since the last check, and notes the time now. It does so even
// if the check then fails, so a bad connection doesn't mean a try at every start.
func (a *app) checkDue(now time.Time) bool {
	path, err := a.stampPath()
	if err != nil {
		return false // Nowhere to remember it, and asking at every start would be worse than never.
	}
	if data, err := os.ReadFile(path); err == nil {
		if last, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data))); err == nil {
			if d := now.Sub(last); d >= 0 && d < 24*time.Hour {
				return false
			}
		}
	}
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil || os.WriteFile(path, []byte(now.Format(time.RFC3339)), 0o644) != nil {
		return false
	}
	return true
}

// checkForUpdate is the automatic check, run in the background. A failure says nothing (Q-79).
// The setting is passed in because only the UI goroutine may read a.cfg (P-01).
func (a *app) checkForUpdate(on bool) {
	if a.upd.version == "dev" || !on || !a.checkDue(time.Now()) {
		return
	}
	tag, err := a.upd.updater.Check(a.upd.ctx)
	if err != nil || !update.Newer(tag, a.upd.version) {
		return
	}
	a.post(func() {
		a.upd.note = fmt.Sprintf("LazyCode %s is available. :update installs it.", tag)
		a.showUpdateNote()
	})
}

// showUpdateNote shows the message once nothing else is using the command bar. It's only a
// message, never a question, and what comes first isn't pushed aside for it.
func (a *app) showUpdateNote() {
	if a.upd.note == "" || a.message != "" || len(a.prompts) > 0 || a.onboard != nil || a.command != nil || a.find != nil || a.input != nil {
		return
	}
	a.message, a.upd.note = a.upd.note, ""
}

// updateCommand is :update. The user asked, so it ignores the setting and the daily limit.
func (a *app) updateCommand() {
	switch {
	case a.upd.version == "dev":
		a.message = "This is a development build, so it can't update itself."
	case a.upd.running:
		a.message = "An update is already running."
	default:
		a.upd.running = true
		a.message = "Checking for updates..."
		go func() {
			msg := a.install()
			a.post(func() {
				a.upd.running = false
				a.message = msg
			})
		}()
	}
}

// install runs off the UI goroutine, and says what happened.
func (a *app) install() string {
	tag, err := a.upd.updater.Check(a.upd.ctx)
	if err != nil {
		return "Couldn't check for updates: " + err.Error()
	}
	if !update.Newer(tag, a.upd.version) {
		return "LazyCode is up to date (" + a.upd.version + ")."
	}
	a.post(func() { a.message = "Downloading LazyCode " + tag + "..." })
	target := a.upd.target
	if target == "" {
		if target, err = update.Executable(); err != nil {
			return "Couldn't find LazyCode's own file: " + err.Error()
		}
	}
	if err := a.upd.updater.Install(a.upd.ctx, tag, target); err != nil {
		return "Update failed: " + err.Error()
	}
	return "Updated to " + tag + ". Restart LazyCode to use it."
}
