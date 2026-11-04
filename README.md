# LazyCode

A light, open-source code editor written in Go that runs in your terminal, between VS Code and Neovim. It's built around multi-directory projects, AI tool sessions, and watching AI changes live.

**Status: early, and being built.** The core editor (M1) is in: projects, a file tree, tabs, splits, two panels, terminal tabs that can run AI tools, multi-cursor editing, find and replace, the mouse, themes, rebindable keys, safe saves and crash recovery. Not here yet: syntax highlighting, search across files, Git, AI session management and live file updates. Expect things to change. See the [docs](docs/README.md) and the [backlog](docs/backlog.html).

## Try it

You need Go 1.26 or newer.

```bash
go run ./cmd/lazycode
```

LazyCode opens on the current folder. To group folders into projects, create `projects.toml` in your config folder: `~/.config/lazycode/` on Linux, `~/Library/Application Support/lazycode/` on macOS, or `%AppData%\lazycode\` on Windows.

```toml
[[project]]
name = "Facebook"

[[project.directory]]
path = "~/code/frontend-facebook"

[[project.directory]]
path = "~/code/microservice-facebook-auth"
```

## Keys

| Action | Linux, Windows | macOS |
|---|---|---|
| Switch project (or `:project`) | Alt+O | Ctrl+K then O |
| Switch directory (or `:dir`) | Alt+D | Ctrl+K then D |
| Previous and next tab | Alt+, and Alt+. | Ctrl+K then , and . |
| Split the view (or `:split`, `:split down`) | Alt+S | Ctrl+K then S |
| Move to the next split | Alt+W | Ctrl+K then W |
| Open or close the second panel | Ctrl+B | Ctrl+B |
| Move the Files view to the other panel (or `:view`) | Alt+V, in the tree | Ctrl+K then V |
| Leave a tab for the file tree | Ctrl+\\ | Ctrl+\\ |
| Enter the current tab | Tab | Tab |
| Move in the file tree, open files and folders | Arrow keys, Enter | Arrow keys, Enter |
| Reload the file tree | R | R |
| Close the current tab (or `:close`) | Ctrl+W | Ctrl+W |

Inside a terminal tab, every key except Ctrl+\\ goes to the tool, so press Ctrl+\\ first.

The mouse works too: click files, tabs, cursor positions and question answers, and scroll with the wheel. LazyCode takes the mouse to do this, so hold Shift to select text the terminal's own way.

In a file:

| Action | Key |
|---|---|
| Save (or `:save`) | Ctrl+S |
| Undo, redo | Ctrl+Z, Ctrl+Y |
| Copy, cut, paste. With no selection, copy and cut take the line. | Ctrl+C, Ctrl+X, Ctrl+V |
| Select | Shift with any movement key, or Ctrl+A for everything |
| Move by word, go to the start or end of the file | Ctrl+Left and Right, Ctrl+Home and End |
| Indent, or indent a selection. Remove an indent. | Tab, Shift+Tab |
| Select the word, then add a cursor on its next match | Ctrl+D |
| Put a cursor on every match (or `:selectall`) | Ctrl+F2, or Ctrl+K then A on macOS |
| Go back to one cursor | Esc |
| Find (exact text and case). Arrow keys move between matches. | Ctrl+F |
| Replace. Enter replaces, and Alt+A replaces all. | Alt+H, or Ctrl+K then H on macOS |
| Open the command bar | Esc, then `:` |

## Commands

Press `:` in the file tree, or Esc then `:` in a file.

| Command | Action |
|---|---|
| `:save`, `:w` | Save the current file |
| `:term claude` | Run a command in a new terminal tab, here an AI tool |
| `:term` | Open your shell in a new terminal tab |
| `:project`, `:dir` | Switch project or directory |
| `:split`, `:split down` | Split the view beside or below |
| `:find`, `:replace` | Find or replace in the current file |
| `:selectall` | Put a cursor on every match |
| `:view` | Move the Files view to the other panel |
| `:config`, `:config keys` | Open the settings, or the key bindings |
| `:close` | Close the current tab |
| `:quit`, `:q` | Quit, stopping every tab's command, and asking about unsaved files |

## Settings

`:config` opens `config.toml`, and saving it applies it straight away. It's created with the defaults and their explanations the first time.

`:config keys` opens `keys.toml`, for changing keys. Give a key an action, or `"none"` to turn it off:

```toml
[keys]
"ctrl+shift+k" = "edit.cut"
"ctrl+d" = "none"
```

A setting or key that can't be used is reported, and the rest keep working. Ctrl+\\ can't be changed, because it's the way out of a terminal tab.

The actions:

| Group | Actions |
|---|---|
| Moving | `move.left`, `.right`, `.up`, `.down`, `.page_up`, `.page_down`, `.home`, `.end`, `.word_left`, `.word_right`, `.file_start`, `.file_end` |
| Selecting | `select.` and the same names as moving, plus `select.all`, `select.next_occurrence`, `select.all_occurrences` |
| Editing | `edit.escape`, `.newline`, `.indent`, `.outdent`, `.backspace`, `.delete`, `.copy`, `.cut`, `.paste`, `.undo`, `.redo` |
| Files and find | `file.save`, `find.open`, `find.replace` |
| Tabs and splits | `tab.previous`, `.next`, `.close`, `.enter`, `split.right`, `split.next` |
| Panels and tree | `panel.toggle_second`, `view.move`, `tree.up`, `.down`, `.open`, `.close`, `.select`, `.refresh` |
| Switching | `project.switch`, `directory.switch`, `command.open` |

## Development

```bash
go test -race ./...
```

To check that LazyCode's keys reach it in your terminal, run `go run ./cmd/keytest`, press each key it lists, then press q.

The `internal/syntax` package uses tree-sitter's C library, so testing it needs a C compiler. The `lazycode` binary doesn't link it yet.

## Licence

[MIT](LICENSE)
