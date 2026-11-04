// LazyCode is a light code editor that runs in your terminal. This is the M0
// prototype: projects, a file tree and terminal tabs, with no editor yet.
package main

import (
	"fmt"
	"os"

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

func run() error {
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
