# LazyCode

A light, open-source code editor written in Go that runs in your terminal, between VS Code and Neovim. It's built around multi-directory projects, AI tool sessions, and watching AI changes live.

**Status: early prototype (M0).** There's no text editor yet. It has projects, a file tree, and terminal tabs that can run AI tools. Expect things to change. See the [docs](docs/README.md) and the [backlog](docs/backlog.html).

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
| Leave a terminal tab | Ctrl+\\ | Ctrl+\\ |
| Enter the current terminal tab | Tab | Tab |
| Move in the file tree, open and close folders | Arrow keys, Enter | Arrow keys, Enter |
| Reload the file tree | R | R |
| Close the current terminal tab (or `:close`) | Ctrl+W | Ctrl+W |

Inside a terminal tab, every key except Ctrl+\\ goes to the tool, so press Ctrl+\\ first.

## Commands

Press `:` outside a terminal tab.

| Command | Action |
|---|---|
| `:term claude` | Run a command in a new terminal tab, here an AI tool |
| `:term` | Open your shell in a new terminal tab |
| `:project`, `:dir` | Switch project or directory |
| `:close` | Close the current terminal tab |
| `:q` | Quit, stopping every tab's command |

## Development

```bash
go test -race ./...
```

To check that LazyCode's keys reach it in your terminal, run `go run ./cmd/keytest`, press each key it lists, then press q.

The `internal/syntax` package uses tree-sitter's C library, so testing it needs a C compiler. The `lazycode` binary doesn't link it yet.

## Licence

[MIT](LICENSE)
