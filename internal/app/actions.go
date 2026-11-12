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
	a.aiSync()
	if a.aiMode && a.doInAIMode(action) {
		return true
	}
	if !a.tabFocused && a.focusedView() == viewAI && a.doInSessionsView(action) {
		return true
	}
	switch action {
	case "run.play":
		a.playCommand("")
	case "ai.mode":
		a.toggleAIMode()
	case "view.sessions":
		a.showView(viewAI)
	case "view.files":
		a.showView(viewFiles)
	case "view.git":
		a.showView(viewGit)
		if !a.git.loading {
			a.refreshGit(false, nil)
		}
	case "view.search":
		a.showView(viewSearch)
	case "file.open_by_name":
		a.openFilePicker()
	case "session.new":
		a.openSessionPicker()
	case "panel.new": // n in a panel: new in this view.
		if a.focusedView() == viewAI {
			a.openSessionPicker()
		} else {
			a.openNewFile()
		}
	case "session.hide": // Delete, which means something else in each view.
		if a.focusedView() == viewFiles {
			a.deleteSelected()
			return true
		}
		a.message = a.keyFor("view.sessions") + " shows the sessions, where this removes one from the list."
	case "project.switch":
		a.openProjectHub(projectItems)
	case "directory.switch":
		a.openDirHub(folderItems)
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
	case "split.down":
		a.splitPane(false)
	case "tab.move":
		a.moveCurrentTab()
	case "focus.next":
		a.focusNext()
	case "view.terminal":
		a.toggleDock()
	case "view.wrap":
		a.toggleWrap()
	case "terminal.new":
		a.startTab(nil)
	case "file.new":
		a.openNewFile()
	case "project.new":
		a.openNewProject()
	case "directory.add":
		a.openAddDir()
	case "app.menu":
		a.openMenuBar(0)
	case "app.settings":
		a.openSettingsMenu()
	case "app.appearance":
		a.openAppearance()
	case "app.themes":
		a.openThemes()
	case "app.plugins":
		a.openPlugins()
	case "app.store":
		a.storeCommand()
	case "app.shortcuts":
		a.openShortcuts()
	case "app.onboarding":
		a.openOnboarding()
	case "app.quit":
		a.quitCommand()
	case "panel.toggle_second":
		a.toggleSecondPanel()
	case "view.move": // A terminal goes between the editor area and the dock; anything else moves the views.
		if it := a.tab(); a.tabFocused && it != nil && it.term != nil {
			a.moveCurrentTab()
		} else {
			a.moveFilesView()
		}
	case "lsp.rename":
		if !a.tabFocused && a.renameInPanel() {
			return true
		}
		return a.doInEditor(action)
	case "command.open":
		a.openCommand()
	case "tree.up":
		a.tree().Move(-1)
	case "tree.down":
		a.tree().Move(1)
	case "tree.close":
		a.tree().SetOpen(false)
	case "tree.open":
		a.tree().SetOpen(true)
	case "tree.select":
		a.openSelected()
	case "tree.refresh":
		a.tree().Refresh()
	case "tree.undo_delete":
		a.undoDelete()
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
	case "edit.delete_word_left":
		a.editWith(it, e.DeleteWordLeft)
	case "edit.delete_word_right":
		a.editWith(it, e.DeleteWordRight)
	case "edit.toggle_comment":
		a.editWith(it, e.ToggleComment)
	case "line.move_up":
		a.editWith(it, func() error { return e.MoveLines(-1) })
	case "line.move_down":
		a.editWith(it, func() error { return e.MoveLines(1) })
	case "line.copy_up":
		a.editWith(it, func() error { return e.CopyLines(-1) })
	case "line.copy_down":
		a.editWith(it, func() error { return e.CopyLines(1) })
	case "line.delete":
		a.editWith(it, e.DeleteLines)
	case "line.insert_below":
		a.editWith(it, func() error { return e.InsertLine(true) })
	case "line.insert_above":
		a.editWith(it, func() error { return e.InsertLine(false) })
	case "file.go_to_line":
		a.openGoToLine()
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
	case "lsp.definition":
		a.definition()
	case "lsp.rename":
		a.renameCommand()
	case "lsp.format":
		a.format()
	default:
		return false
	}
	return true
}

// editorMoves are the actions that only move cursors.
var editorMoves = map[string]func(*editor.Editor){
	"cursor.add_above": func(e *editor.Editor) { e.AddCursorLine(-1) },
	"cursor.add_below": func(e *editor.Editor) { e.AddCursorLine(1) },
	"select.line":      (*editor.Editor).SelectLines,

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
