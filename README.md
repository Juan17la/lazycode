# LazyCode

A light, open-source code editor written in Go that runs in your terminal, between VS Code and Neovim. It's built around multi-directory projects, AI tool sessions, and watching AI changes live.

**Status: early, and being built.** The core editor (M1) is in: projects, a file tree, tabs, splits, two panels, terminal tabs, AI sessions that sleep and resume, `:play`, live file updates with modified-line markers, multi-cursor editing, find and replace, search across a directory, Git and GitHub, syntax highlighting, language servers, the mouse, themes, rebindable keys, safe saves, crash recovery and first-run onboarding. The first release is being prepared. Expect things to change. See the [docs](docs/README.md) and the [backlog](docs/backlog.html).

## Install

Linux (any 64-bit distro) and macOS (12 or newer, Intel or Apple silicon):

```bash
curl -fsSL https://raw.githubusercontent.com/Juan17la/lazycode/main/install.sh | sh
```

Windows 10 and 11 (Intel, AMD or ARM), in cmd or PowerShell:

```powershell
curl.exe -fsSL https://raw.githubusercontent.com/Juan17la/lazycode/main/install.ps1 | powershell -NoProfile -Command -
```

Releases are **not signed**. The scripts check each download's SHA-256 against the release's `checksums.txt`, which catches a corrupted download but not a compromised release. You can read the scripts before running them: [install.sh](install.sh) and [install.ps1](install.ps1). They install to a folder in your home (no `sudo`, no administrator), add it to your `PATH` if needed, and create an app icon. More in the [install guide](docs/guide/install.md).

Or build from source. You need Go 1.26 or newer, and a C compiler for syntax highlighting:

```bash
go run ./cmd/lazycode
```

**Guides:** [install](docs/guide/install.md), [getting started](docs/guide/getting-started.md), [coming from VS Code](docs/guide/coming-from-vscode.md), [coming from Neovim](docs/guide/coming-from-neovim.md), [keys and configuration](docs/guide/keys-and-config.md), [troubleshooting](docs/guide/troubleshooting.md). See also the [changelog](CHANGELOG.md), [contributing](CONTRIBUTING.md) and [security policy](SECURITY.md).

This is a free-time project. There is no guaranteed response time for issues or questions.

The first run asks for a theme and auto-save, and offers to install language tools, showing each command before it runs. All of this can be skipped.

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
| Menus (or `:menu`) | Alt+M or F10 | Ctrl+K then M, or F10 |
| Settings (or `:settings`) | Alt+S | Ctrl+K then S |
| Shortcuts, which can also change them (or `:shortcuts`) | Alt+K or F1 | Ctrl+K then K, or F1 |
| Switch project (or `:project`). Starts on the last one, so Alt+P then Enter goes back. 1–9 pick. | Alt+P, also inside a terminal | Ctrl+K then P |
| Switch directory (or `:dir`), the same way | Alt+D, also inside a terminal | Ctrl+K then D |
| New project (or `:project new`) | Alt+Shift+P | Ctrl+K then Shift+P |
| Add a directory (or `:dir add`) | Alt+Shift+F | Ctrl+K then Shift+F |
| New file (or `:new`) | Ctrl+N | Ctrl+N |
| Previous and next tab | Alt+, and Alt+. | Ctrl+K then , and . |
| Split right, split below (or `:split`, `:split down`) | Alt+Shift+S, Alt+Shift+B | Ctrl+K then Shift+S, Shift+B |
| Move to the next area: panels, splits and the dock | Alt+W or Alt+Tab, also inside a terminal | Ctrl+K then W |
| Open or close the second panel | Ctrl+B | Ctrl+B |
| Show the files (or `:sessions` for AI sessions in the panel) | Alt+E | Ctrl+K then E |
| AI mode, in and out (or `:ai`): every session by project and folder, beside the tool you used last | Alt+A, also inside a terminal | Ctrl+K then A |
| Show or hide the terminal dock (or `:dock`) | Alt+T | Ctrl+K then T |
| New terminal, new AI session | Alt+Shift+T, Alt+Shift+A | Ctrl+K then Shift+T, Shift+A |
| Search this directory (or `:search`) | Alt+F | Ctrl+K then F |
| Show Git (or `:git`) | Alt+G | Ctrl+K then G |
| Open a file by name | Ctrl+P | Ctrl+P |
| Run this directory's project (or `:play`) | Alt+R | Ctrl+K then R |
| Word wrap (or `:wrap`) | Alt+Z | Ctrl+K then Z |
| Move the views to the other panel, or a terminal between the editor area and the dock (or `:view`, `:move`) | Alt+V, also inside a terminal | Ctrl+K then V |
| Leave a tab for the file tree, and go back in | Ctrl+Space (Ctrl+\\ only leaves) | Ctrl+Space |
| Enter the current tab | Tab | Tab |
| Move in the file tree, open files and folders | Arrow keys, Enter | Arrow keys, Enter |
| Reload the file tree | R | R |
| Close the current tab (or `:close`) | Ctrl+W | Ctrl+W |

Inside a terminal tab, every key goes to the tool except Ctrl+Space and Ctrl+\\ (leave), and the Alt keys for closing the tab, switching project or folder, moving between areas, moving the tab and AI mode (Alt+Q, Alt+P, Alt+D, Alt+W, Alt+V, Alt+A).

The mouse works too: click files, tabs, cursor positions and question answers, and scroll with the wheel. LazyCode takes the mouse to do this, so hold Shift to select text the terminal's own way.

In the AI sessions view: Enter goes to a session or resumes it, N starts one, F2 renames it, and Delete takes it off the list without touching the tool's data.

In the Git view: P pushes, L pulls, F fetches, S stages, U unstages, C commits, B switches branch, N makes a branch, M merges, X aborts a merge, O opens the file, and Enter opens a diff or commit.

In the Search view: Tab moves between Find and With, Enter opens a result, and Enter in With shows every change before making it.

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
| Put a cursor on every match (or `:selectall`) | Ctrl+F2, or Ctrl+K then O on macOS |
| Go back to one cursor | Esc |
| Find (exact text and case). Arrow keys move between matches. | Ctrl+F |
| Replace. Enter replaces, and Alt+A replaces all. | Ctrl+R |
| Go to definition, rename, format. These need the language's server: `gopls`, `pyright` or `typescript-language-server`. | F12, F2, F3 |
| Open the command bar | Esc, then `:` |

## Commands

Press `:` in the file tree, or Esc then `:` in a file.

| Command | Action |
|---|---|
| `:save`, `:w` | Save the current file |
| `:session`, `:session hidden` | Start an AI session or shell, or bring back a hidden one |
| `:search`, `:git` | Show the Search or Git view |
| `:push`, `:pull`, `:fetch` | Run that Git operation |
| `:repo create` | Create a GitHub repository through `gh`, after showing the command |
| `:definition`, `:rename`, `:format` | Language server actions |
| `:project close` | Close this project's tabs and move to the next project |
| `:play`, `:p` | Run this directory's project, or go to its tab if it's running. `:play cargo build` sets and remembers the command. |
| `:term claude` | Run a command in a new terminal tab, here an AI tool |
| `:term` | Open your shell in a new terminal tab |
| `:project`, `:dir` | Switch project or directory |
| `:split`, `:split down` | Split the view beside or below |
| `:find`, `:replace` | Find or replace in the current file |
| `:selectall` | Put a cursor on every match |
| `:view` | Move the Files view to the other panel |
| `:config`, `:config keys` | Open the settings, or the key bindings |
| `:update` | Install a newer release, if there is one |
| `:close` | Close the current tab |
| `:quit`, `:q` | Quit, stopping every tab's command, and asking about unsaved files |

## Settings

LazyCode has no telemetry. Its one automatic network connection is a check of GitHub's latest release at most once a day, and `check = false` under `[updates]` turns it off.

`:config` opens `config.toml`, and saving it applies it straight away. It's created with the defaults and their explanations the first time.

`:config keys` opens `keys.toml`, for changing keys. Give a key an action, or `"none"` to turn it off:

```toml
[keys]
"ctrl+shift+k" = "edit.cut"
"ctrl+d" = "none"
```

A setting or key that can't be used is reported, and the rest keep working. Ctrl+Space and Ctrl+\\ can't be changed, because they're the way out of a terminal tab.

The actions:

| Group | Actions |
|---|---|
| Moving | `move.left`, `.right`, `.up`, `.down`, `.page_up`, `.page_down`, `.home`, `.end`, `.word_left`, `.word_right`, `.file_start`, `.file_end` |
| Selecting | `select.` and the same names as moving, plus `select.all`, `select.next_occurrence`, `select.all_occurrences` |
| Editing | `edit.escape`, `.newline`, `.indent`, `.outdent`, `.backspace`, `.delete`, `.copy`, `.cut`, `.paste`, `.undo`, `.redo` |
| Files and find | `file.save`, `file.open_by_name`, `find.open`, `find.replace` |
| Language servers | `lsp.definition`, `lsp.rename`, `lsp.format` |
| Tabs and splits | `tab.previous`, `.next`, `.close`, `.enter`, `split.right`, `split.next` |
| Running and sessions | `run.play`, `ai.mode`, `view.sessions`, `view.files`, `view.search`, `view.git`, `session.new`, `session.hide` |
| Panels and tree | `panel.toggle_second`, `view.move`, `tree.up`, `.down`, `.open`, `.close`, `.select`, `.refresh` |
| Switching | `project.switch`, `directory.switch`, `command.open` |

## Development

```bash
go test -race ./...
```

To check that LazyCode's keys reach it in your terminal, run `go run ./cmd/keytest`, press each key it lists, then press q.

The `internal/syntax` package uses tree-sitter's C library, so building LazyCode needs a C compiler.

## Licence

[MIT](LICENSE). Third-party licences are in [THIRD_PARTY_LICENSES](THIRD_PARTY_LICENSES).
