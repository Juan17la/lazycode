# Coming from VS Code

Typing works the way you expect: arrow keys, Shift to select, Ctrl+C, X and V, Ctrl+Z and Ctrl+Y, Ctrl+S, Ctrl+F, Ctrl+A. Click to place the cursor. Most differences are about where things live, because everything runs inside a terminal.

## Same keys

| You know | LazyCode | Note |
|---|---|---|
| Ctrl+P, open by name | Ctrl+P | |
| Ctrl+D, next occurrence | Ctrl+D | Select the word, then add a cursor on its next match |
| Ctrl+Shift+L, all occurrences | Ctrl+F2 | `:selectall`. On macOS, Ctrl+K then A |
| Ctrl+Shift+Z, redo | Ctrl+Shift+Z or Ctrl+Y | |
| Ctrl+W, close tab | Ctrl+W | |
| F12, go to definition | F12 | Needs the language server |
| F2, rename | F2 | Needs the language server |
| Shift+Alt+F, format | Alt+Shift+F | Needs the language server |
| Ctrl+Left and Right, by word | Ctrl+Left and Right | |
| Ctrl+Home and End | Ctrl+Home and End | |
| Ctrl+C with nothing selected copies the line | The same, and so does Ctrl+X | |

## Different

| In VS Code | In LazyCode |
|---|---|
| Ctrl+Shift+P, command palette | Press `:` in the tree, or Esc then `:` in a file, and type a command such as `:save` |
| Ctrl+Shift+F, search in files | Alt+F, or `:search` |
| Ctrl+H, replace | Alt+H, or `:replace` (Enter replaces, Alt+A replaces all). Replacing across files, from Alt+F, shows every change before making it |
| Ctrl+B, show or hide the side bar | Ctrl+B opens or closes a second panel. Alt+E shows the files |
| Ctrl+\\ or Ctrl+Alt+Right, split | Alt+S, or `:split`. Alt+W moves between splits |
| Ctrl+Shift+`, new terminal | `:term` opens a shell in a tab. Ctrl+\\ leaves a terminal tab |
| Ctrl+Shift+G, source control | Alt+G, or `:git` |
| Workspaces with several folders | Projects: `:project new NAME`, `:dir add PATH`. Alt+O and Alt+D switch |
| Run button, tasks | Alt+P, or `:play`, runs the directory's project |
| Extensions | None yet. Plugins are planned |
| Settings UI, settings.json | `:config` opens `config.toml`. `:config keys` opens `keys.toml` |

Esc goes back to one cursor. Esc then `:` opens the command bar.

## Not there yet

No comment toggle, go to line, move line up or down, or Alt+click for extra cursors. Use Ctrl+D or Ctrl+F2 for multiple cursors. Settings are few ([Keys and config](keys-and-config.md)).

## What you gain

AI tools such as Claude Code run in tabs that sleep when idle and resume the same chat, and files they change update live with the changed lines marked. It uses little memory: the budget is under 300 MB.

## Tips

- Your terminal gets keys before LazyCode does. If a shortcut doesn't arrive, see [Troubleshooting](troubleshooting.md), and rebind it in `keys.toml`.
- LazyCode takes the mouse. Hold Shift to select text the terminal's way, for copying out of the terminal.
- On macOS, every Alt shortcut is Ctrl+K, then the key.
