// LazyCode is a light code editor that runs in your terminal.
package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/Juan17la/lazycode/internal/app"
	"github.com/Juan17la/lazycode/internal/project"
	"github.com/gdamore/tcell/v2"
)

// version is "dev" unless a release sets it: -ldflags "-X main.version=v0.1.0".
var version = "dev"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(versionText())
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "lazycode:", err)
		os.Exit(1)
	}
}

// versionText adds the commit to a development build, so that two builds can be told apart. The
// version itself stays "dev", which is what turns off the update check.
func versionText() string {
	info, ok := debug.ReadBuildInfo()
	if version != "dev" || !ok {
		return version
	}
	commit, changed := "", ""
	for _, s := range info.Settings {
		switch {
		case s.Key == "vcs.revision" && len(s.Value) >= 7:
			commit = s.Value[:7]
		case s.Key == "vcs.modified" && s.Value == "true":
			changed = ", with uncommitted changes"
		}
	}
	if commit == "" {
		return version
	}
	return version + " (" + commit + changed + ")"
}

// memoryLimit keeps Go's heap well under the 300 MB limit (D-36), leaving room for
// what Go doesn't manage. Measured: 10 full terminal tabs go from 362 MB to 242 MB.
// ponytail: fixed soft limit, past about 15 full tabs the garbage collector works
// harder rather than the limit breaking; scale it or sleep tabs if that shows up.
const memoryLimit = 250 << 20

func run() error {
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(memoryLimit)
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	var projects []project.Project
	startup := ""
	path, err := project.ConfigPath()
	if err == nil {
		projects, err = project.Load(path)
	}
	if err != nil {
		startup = "Projects not loaded: " + err.Error() // Keep running on this folder.
	}
	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	return app.Run(screen, projects, dir, startup, version)
}
