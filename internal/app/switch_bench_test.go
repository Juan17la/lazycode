package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Juan17la/lazycode/internal/project"
	"github.com/gdamore/tcell/v2"
)

// One switch between two projects of 1,000 files, including the next frame. Switching
// directory runs the same afterSwitch. The target is 50 ms (doc 12).
func BenchmarkSwitchProject(b *testing.B) {
	b.Setenv("XDG_CONFIG_HOME", b.TempDir())
	var all []project.Project
	for _, name := range []string{"one", "two"} {
		dir := b.TempDir()
		for i := range 1000 {
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%04d.txt", i)), nil, 0o644); err != nil {
				b.Fatal(err)
			}
		}
		all = append(all, project.Project{Name: name, Dirs: []string{dir}})
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		b.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(120, 50)
	a := newApp(screen, all, all[0].Dirs[0], "")
	a.statePath = filepath.Join(b.TempDir(), "state.toml") // The real save is part of a switch.
	for b.Loop() {
		a.project = 1 - a.project
		a.afterSwitch()
		a.draw()
	}
}
