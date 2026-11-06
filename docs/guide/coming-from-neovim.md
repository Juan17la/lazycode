# Coming from Neovim

LazyCode is not modal and doesn't have Vim keys. You are always typing: arrow keys move, Shift selects, Ctrl+C, X and V copy, cut and paste. There's no Normal mode, no `hjkl`, no operators, no registers, and no macros. If Vim motions are what you love, LazyCode isn't a replacement. If you want a small, fast, terminal editor with ordinary keys that also holds your AI sessions and several folders, read on.

## What is familiar

- **`:` commands.** Press `:` in the file tree, or Esc, then `:` in a file. Commands have short forms: `:w` saves, `:q` quits, `:p` runs the project, `:split` splits.
- **Splits and tabs.** `:split` and `:split down`, Alt+W to move between them. Alt+, and Alt+. for tabs.
- **Plain-text config.** `config.toml` and `keys.toml`, applied when you save. `:config` and `:config keys` open them.
- **Language servers.** `gopls`, `pyright` and `typescript-language-server`: F12 definition, F2 rename, F3 format. Install them yourself or from the first-run screen, which shows each command before it runs.
- **Treesitter highlighting** for Go, Python, JavaScript and TypeScript.
- **Small and quick.** One file, no runtime to install.

## Mapping

| In Neovim | In LazyCode |
|---|---|
| `:w`, `:q` | `:w` or `:save`, and `:q` or `:quit` (asks about unsaved files) |
| `:e file`, Telescope find files | Ctrl+P, or Enter on a file in the tree |
| `/` search, `:%s` replace | Ctrl+F in a file, Ctrl+R to replace |
| `:grep`, live grep | Alt+F, or `:search` |
| `:vsplit`, `:split` | `:split`, `:split down` |
| `:terminal` | `:term` (a shell), `:term lazygit` or any command |
| `:bnext`, `:bprev` | Alt+. and Alt+, |
| `*`, then `cgn` | Ctrl+D to add the next match as another cursor, then type |
| `gd`, `:lua vim.lsp.buf.rename()` | F12, F2 |
| lazygit | Alt+G, or `:git` |
| `init.lua` | `config.toml` and `keys.toml`: settings and key bindings only |
| Plugins | None yet. Plugins are planned, in Lua |

## Different things to know

- Keys are rebound one at a time in `keys.toml`, to a named action such as `"edit.cut"`. There's no mapping language. The list is in [Keys and config](keys-and-config.md).
- Inside a terminal tab every key goes to the program there. Ctrl+\\ is how you leave, and it can't be rebound.
- Esc in a file returns to one cursor. Esc then `:` opens the command bar.
- On macOS, Alt shortcuts are Ctrl+K, then the key.
- LazyCode takes the mouse. Hold Shift to select text the terminal's way.
- Running inside tmux: see [Troubleshooting](troubleshooting.md) for keys that don't arrive.
