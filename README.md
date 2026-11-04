# LazyCode

A light, open-source code editor written in Go that runs in your terminal, between VS Code and Neovim. It's built around multi-directory projects, AI tool sessions, and watching AI changes live.

**Status: early, and being built (M1).** It has projects, a file tree, terminal tabs that can run AI tools, and basic file editing with safe saves and crash recovery. Multi-cursor, find and replace, splits and the mouse aren't here yet. Expect things to change. See the [docs](docs/README.md) and the [backlog](docs/backlog.html).

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
| Previous and next terminal tab | Alt+, and Alt+. | Ctrl+K then , and . |
| Leave a tab for the file tree | Ctrl+\\ | Ctrl+\\ |
| Enter the current tab | Tab | Tab |
| Move in the file tree, open files and folders | Arrow keys, Enter | Arrow keys, Enter |
| Reload the file tree | R | R |
| Close the current tab (or `:close`) | Ctrl+W | Ctrl+W |

Inside a terminal tab, every key except Ctrl+\\ goes to the tool, so press Ctrl+\\ first.

In a file:

| Action | Key |
|---|---|
| Save (or `:save`) | Ctrl+S |
| Undo, redo | Ctrl+Z, Ctrl+Y |
| Copy, cut, paste. With no selection, copy and cut take the line. | Ctrl+C, Ctrl+X, Ctrl+V |
| Select | Shift with any movement key, or Ctrl+A for everything |
| Move by word, go to the start or end of the file | Ctrl+Left and Right, Ctrl+Home and End |
| Indent, or indent a selection. Remove an indent. | Tab, Shift+Tab |
| Open the command bar | Esc, then `:` |

## Commands

Press `:` in the file tree, or Esc then `:` in a file.

| Command | Action |
|---|---|
| `:save`, `:w` | Save the current file |
| `:term claude` | Run a command in a new terminal tab, here an AI tool |
| `:term` | Open your shell in a new terminal tab |
| `:project`, `:dir` | Switch project or directory |
| `:close` | Close the current tab |
| `:q` | Quit, stopping every tab's command, and asking about unsaved files |

## Development

```bash
go test -race ./...
```

To check that LazyCode's keys reach it in your terminal, run `go run ./cmd/keytest`, press each key it lists, then press q.

The `internal/syntax` package uses tree-sitter's C library, so testing it needs a C compiler. The `lazycode` binary doesn't link it yet.

## Licence

[MIT](LICENSE)
