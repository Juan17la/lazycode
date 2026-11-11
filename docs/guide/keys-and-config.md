# Keys and configuration

Everything here comes from what's built. If you find a mismatch, it's a bug in this page: please open an issue.

The keys come in families, so that most can be guessed: **Ctrl+Space** goes in and out of a tab, **Alt+letter** goes somewhere or shows something, **Alt+Shift+letter** makes something, **Ctrl+letter** acts on this file, and **F-keys** act on the selected thing (rename, format, definition).

**On macOS, every Alt shortcut is Ctrl+K, then the key.** Alt+D is Ctrl+K, then D. Ctrl+F2 (all occurrences) is Ctrl+K, then O. An Alt+Shift shortcut is Ctrl+K, then the capital. Inside a terminal tab every key goes to the program there, except Ctrl+Space (or Ctrl+\\) to leave, Alt+Q to close the tab, and the switching keys Alt+P, Alt+D, Alt+W and Alt+V.

## Keys everywhere

| Action | Key | Command |
|---|---|---|
| Menus | Alt+M or F10 | `:menu` |
| Settings | Alt+S | `:settings` |
| Shortcuts, which can also change them | Alt+K or F1 | `:shortcuts` |
| Command bar | Alt+X | `:` in a panel |
| Switch project | Alt+P | `:project` |
| Switch directory | Alt+D | `:dir` |
| New project, in a form | Alt+Shift+P | `:project new`, or `:project new NAME` |
| Add a directory, in a form | Alt+Shift+F | `:dir add`, or `:dir add PATH` |
| New file | Ctrl+N | `:new` |
| Previous, next tab | Alt+, and Alt+. | |
| Close the tab | Ctrl+W | `:close` |
| Split right, split below | Alt+Shift+S, Alt+Shift+B | `:split`, `:split down` |
| Next area: panels, splits, the dock | Alt+W, Alt+Tab | |
| Open or close the second panel | Ctrl+B | |
| Show the files | Alt+E | |
| Show AI sessions | Alt+A | `:session` |
| Show or hide the terminal dock | Alt+T | `:dock` |
| New terminal, new AI session | Alt+Shift+T, Alt+Shift+A | `:term`, `:session` |
| Search this directory | Alt+F | `:search` |
| Show Git | Alt+G | `:git` |
| Open a file by name | Ctrl+P | |
| Run the directory's project | Alt+R | `:play`, `:p` |
| Word wrap | Alt+Z | `:wrap` |
| Quit | Ctrl+Q | `:q` |
| Leave a tab for the file tree, and go back in | Ctrl+Space | |

## In the file tree

| Action | Key |
|---|---|
| Move, open or close a folder | Arrow keys |
| Open a file or folder | Enter |
| Enter the current tab | Tab |
| Reload the tree | R |
| Command bar | `:` |
| Move the Files view to the other panel | Alt+V (`:view`) |
| New AI session | N |
| Take a session off the list | Delete |

## In a file

| Action | Key |
|---|---|
| Save | Ctrl+S |
| Undo, redo | Ctrl+Z, Ctrl+Y or Ctrl+Shift+Z |
| Copy, cut, paste (no selection takes the line) | Ctrl+C, Ctrl+X, Ctrl+V |
| Select | Shift with a movement key, Ctrl+A for all |
| By word, start or end of file | Ctrl+Left and Right, Ctrl+Home and End (add Shift to select) |
| Indent, outdent | Tab, Shift+Tab |
| Next occurrence as a new cursor | Ctrl+D |
| Cursor on every match | Ctrl+F2 (`:selectall`) |
| Back to one cursor | Esc |
| Find | Ctrl+F (`:find`) |
| Replace | Ctrl+R (`:replace`). Enter replaces, Alt+A replaces all |
| Definition, rename, format | F12, F2, F3 (`:definition`, `:rename`, `:format`) |
| Command bar | Esc, then `:` |

Other views: in AI sessions, Enter resumes, N starts, R renames. In Git, S stages, U unstages, C commits, P pushes, L pulls, F fetches, B switches branch, N makes a branch, M merges, X aborts a merge, O opens the file, Enter opens a diff or commit. In Search, Tab moves between Find and With, Enter opens a result, and Enter in With shows every change before making it.

## Commands

| Command | Does |
|---|---|
| `:save`, `:w` | Save |
| `:quit`, `:q` | Quit, stopping every tab's command, asking about unsaved files |
| `:close` | Close the tab |
| `:project`, `:project new NAME`, `:project close` | Switch, make, or close a project |
| `:dir`, `:dir open`, `:dir add PATH`, `:dir remove` | Switch, browse to any folder, add to the project, or remove from it |
| `:term`, `:term COMMAND` | Open a shell, or run a command, in a new tab |
| `:session`, `:session hidden` | Start an AI session or shell, or bring back a hidden one |
| `:play`, `:p`, `:play COMMAND` | Run the project. With a command, set and remember it |
| `:split`, `:split down` | Split beside or below |
| `:find`, `:replace`, `:selectall` | Find, replace, a cursor on every match |
| `:search`, `:git` | Show those views |
| `:push`, `:pull`, `:fetch` | Git, as named |
| `:repo create` | Create a GitHub repository through `gh`, after showing the command |
| `:definition`, `:rename`, `:format` | Language server actions |
| `:view`, `:tree` | Move the Files view to the other panel, or toggle the combined tree |
| `:config`, `:config keys`, `:config projects` | Open `config.toml`, `keys.toml` or `projects.toml` |
| `:update` | Install a newer release, if there is one |

## Where the files are

| System | Folder |
|---|---|
| Linux | `~/.config/lazycode/` |
| macOS | `~/Library/Application Support/lazycode/` |
| Windows | `%AppData%\lazycode\` |

`:config` creates `config.toml` with the defaults and their explanations the first time, and saving applies it straight away. A setting that can't be used is reported and the rest keep working.

## config.toml

```toml
version = 1

[editor]
auto_save = false   # Save a second after typing stops.
tab_size = 4        # 1 to 16

[ui]
theme = "dark"      # Or use :theme, which previews them. dark, light, gruvbox-dark, gruvbox-light,
                    # dracula, one-dark, nord, tokyo-night, catppuccin-mocha, solarized-dark,
                    # solarized-light, monokai
icons = "basic"     # basic works in any font. nerd adds file-type icons, and needs a Nerd Font.
sidebar = "left"    # left or right

# Change any colour of the theme: bg, panel, bar, selection, fg, muted, comment, accent,
# red, green, yellow, blue, purple, cyan, orange.
# [ui.colors]
# accent = "#ff8800"

[terminal]
scrollback = 1000   # Lines per terminal tab, 1 to 10000. About 36 MB per 1,000 lines.

[sessions]
sleep_after_minutes = 30   # An idle AI session stops to free memory. 0 turns this off.

[updates]
check = true   # Look on GitHub for a new version, at most once a day.
```

To add an AI tool, or change a default one (Claude Code, OpenCode and Codex are built in), use a `[[harness]]` table. `{id}` is the session ID and `{id_hex}` is the same without dashes. The resume fields are optional; without them, selecting a stopped session starts a new one.

```toml
[[harness]]
name = "Claude Code"
command = "claude"
start_args = ["--session-id", "{id}"]
resume_args = ["--resume", "{id}"]
```

## keys.toml

`:config keys` opens it. Give a key an action, or `"none"` to turn the key off:

```toml
[keys]
"ctrl+shift+k" = "edit.cut"
"ctrl+d" = "none"
```

Key names are lower case, joined with `+`: `ctrl+`, `alt+`, `shift+`, then the key (`left`, `pgup`, `f2`, `enter`, a letter or a symbol such as `,`). Ctrl+Space and Ctrl+\\ can't be changed, because they're the way out of a terminal tab.

| Group | Actions |
|---|---|
| Moving | `move.left`, `.right`, `.up`, `.down`, `.page_up`, `.page_down`, `.home`, `.end`, `.word_left`, `.word_right`, `.file_start`, `.file_end` |
| Selecting | `select.` and the same names as moving, plus `select.all`, `select.next_occurrence`, `select.all_occurrences` |
| Editing | `edit.escape`, `.newline`, `.indent`, `.outdent`, `.backspace`, `.delete`, `.copy`, `.cut`, `.paste`, `.undo`, `.redo` |
| Files and find | `file.save`, `file.open_by_name`, `find.open`, `find.replace` |
| Language servers | `lsp.definition`, `lsp.rename`, `lsp.format` |
| Tabs and splits | `tab.previous`, `.next`, `.close`, `.enter`, `split.right`, `split.next` |
| Running and sessions | `run.play`, `view.sessions`, `view.files`, `view.search`, `view.git`, `session.new`, `session.hide` |
| Panels and tree | `panel.toggle_second`, `view.move`, `tree.up`, `.down`, `.open`, `.close`, `.select`, `.refresh` |
| Switching | `project.switch`, `directory.switch`, `command.open` |

## projects.toml

Normally written by `:project new` and `:dir add`. You can edit it by hand:

```toml
[[project]]
name = "Shop"

[[project.directory]]
path = "~/code/shop-web"

[[project.directory]]
path = "~/code/shop-api"
```

## Editing like VS Code

Typing `(`, `[`, `{`, `'`, `"` or `` ` `` adds its closer, surrounds any selection, and typing the closer steps over it. Backspace removes an empty pair, and Enter between brackets opens an indented block. `<` closes in markup files such as HTML, and in code only straight after a word, so `a < b` stays as typed.

| Key | Does |
|---|---|
| Ctrl+Shift+Left, Right | Select by word |
| Alt+Up, Alt+Down (or Ctrl+Alt+Up, Down) | Add a cursor above or below |
| Alt+click | Add or remove a cursor |
| Ctrl+D, Ctrl+Shift+L | Add the next match, select all matches |
| Ctrl+Shift+Up, Down | Move the line |
| Alt+Shift+Up, Down | Copy the line |
| Ctrl+Shift+K | Delete the line |
| Ctrl+Enter, Ctrl+Shift+Enter | New line below, above (Ctrl+J also goes below) |
| Ctrl+L | Select the line, and the next each time |
| Ctrl+/ | Comment or uncomment |
| Ctrl+Backspace or Alt+Backspace, Ctrl+Delete | Delete a word back, forward |
| Ctrl+G | Go to line |

Some terminals can't send every combination, or keep them for themselves. Any of these can be changed in `keys.toml`.
