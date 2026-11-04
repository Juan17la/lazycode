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

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "lazycode:", err)
		os.Exit(1)
	}
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
	return app.Run(screen, projects, dir, startup)
}
