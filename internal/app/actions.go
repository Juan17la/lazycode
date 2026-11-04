package app

import (
	"errors"
	"fmt"

	"github.com/Juan17la/lazycode/internal/buffer"
	"github.com/Juan17la/lazycode/internal/editor"
)

// do runs an action from the keymap. Every name here is listed in keymap's defaults, and
// a test checks that each one is handled.
func (a *app) do(action string) bool {
	switch action {
	case "project.switch":
		a.openProjectSwitcher()
	case "directory.switch":
		a.openDirSwitcher()
	case "tab.previous":
		a.moveTab(-1)
	case "tab.next":
		a.moveTab(1)
	case "tab.close":
		a.closeTab()
	case "tab.enter":
		if a.tab() != nil {
			a.focusTab()
		} else {
			a.message = "No tabs here. Enter opens a file, and :term starts a tool."
		}
	case "split.right":
		a.splitPane(true)
	case "split.next":
		a.nextPane()
	case "panel.toggle_second":
		a.toggleSecondPanel()
	case "view.move":
		a.moveFilesView()
	case "command.open":
		a.openCommand()
	case "tree.up":
		a.dir().tree.Move(-1)
	case "tree.down":
		a.dir().tree.Move(1)
	case "tree.close":
		a.dir().tree.SetOpen(false)
	case "tree.open":
		a.dir().tree.SetOpen(true)
	case "tree.select":
		a.openSelected(a.dir())
	case "tree.refresh":
		a.dir().tree.Refresh()
	default:
		return a.doInEditor(action)
	}
	return true
}

// doInEditor runs an action that needs a file. Keys only reach here from a file, but a
// command can ask for one anywhere.
func (a *app) doInEditor(action string) bool {
	it := a.tab()
	if it == nil || it.ed == nil {
		a.message = "That needs a file."
		return false
	}
	e := it.ed
	if run := editorMoves[action]; run != nil {
		run(e)
		return true
	}
	switch action {
	case "select.all":
		e.SelectAll()
	case "select.next_occurrence":
		if !e.AddNextOccurrence() {
			a.message = "No more to select."
		}
	case "select.all_occurrences":
		switch n := e.SelectAllOccurrences(); n {
		case 0:
			a.message = "No word here to select."
		default:
			a.message = fmt.Sprintf("%d selected", n)
		}
	case "edit.escape":
		e.ClearSelection()
		a.escaped = true
		a.message = "Press : for commands"
	case "edit.newline":
		a.editWith(it, e.Newline)
	case "edit.indent":
		a.editWith(it, e.Tab)
	case "edit.outdent":
		a.editWith(it, e.Outdent)
	case "edit.backspace":
		a.editWith(it, e.Backspace)
	case "edit.delete":
		a.editWith(it, e.Delete)
	case "edit.copy":
		a.copyText(e.Copy())
	case "edit.cut":
		a.editWith(it, func() error {
			text, err := e.Cut()
			if err == nil {
				a.copyText(text)
			}
			return err
		})
	case "edit.paste":
		a.pasteFromClipboard(it)
	case "edit.undo":
		a.editWith(it, func() error { e.Undo(); return nil })
	case "edit.redo":
		a.editWith(it, func() error { e.Redo(); return nil })
	case "file.save":
		a.save(it, func() {})
	case "find.open":
		a.openFind(it, false)
	case "find.replace":
		a.openFind(it, true)
	default:
		return false
	}
	return true
}

// editorMoves are the actions that only move cursors.
var editorMoves = map[string]func(*editor.Editor){
	"move.left":       func(e *editor.Editor) { e.Left(false) },
	"move.right":      func(e *editor.Editor) { e.Right(false) },
	"move.up":         func(e *editor.Editor) { e.Up(false) },
	"move.down":       func(e *editor.Editor) { e.Down(false) },
	"move.page_up":    func(e *editor.Editor) { e.PageUp(false) },
	"move.page_down":  func(e *editor.Editor) { e.PageDown(false) },
	"move.home":       func(e *editor.Editor) { e.Home(false) },
	"move.end":        func(e *editor.Editor) { e.End(false) },
	"move.word_left":  func(e *editor.Editor) { e.WordLeft(false) },
	"move.word_right": func(e *editor.Editor) { e.WordRight(false) },
	"move.file_start": func(e *editor.Editor) { e.DocStart(false) },
	"move.file_end":   func(e *editor.Editor) { e.DocEnd(false) },

	"select.left":       func(e *editor.Editor) { e.Left(true) },
	"select.right":      func(e *editor.Editor) { e.Right(true) },
	"select.up":         func(e *editor.Editor) { e.Up(true) },
	"select.down":       func(e *editor.Editor) { e.Down(true) },
	"select.page_up":    func(e *editor.Editor) { e.PageUp(true) },
	"select.page_down":  func(e *editor.Editor) { e.PageDown(true) },
	"select.home":       func(e *editor.Editor) { e.Home(true) },
	"select.end":        func(e *editor.Editor) { e.End(true) },
	"select.word_left":  func(e *editor.Editor) { e.WordLeft(true) },
	"select.word_right": func(e *editor.Editor) { e.WordRight(true) },
	"select.file_start": func(e *editor.Editor) { e.DocStart(true) },
	"select.file_end":   func(e *editor.Editor) { e.DocEnd(true) },
}

// editWith runs an edit, reports any failure, and keeps recovery in step with the text.
func (a *app) editWith(it *tabItem, edit func() error) {
	before := it.ed.File.Snapshot
	if err := edit(); errors.Is(err, buffer.ErrReadOnly) {
		a.message = it.name() + " is read-only."
	} else if err != nil {
		a.message = err.Error()
	}
	if !before.Same(it.ed.File.Snapshot) {
		a.edited(it)
	}
}
