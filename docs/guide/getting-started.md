# Getting started

LazyCode is a code editor that runs in your terminal. You type in files the normal way, with no modes to learn. It groups several folders into a project, and keeps AI tool sessions (such as Claude Code) in tabs beside your code.

Start it in a folder:

```bash
cd ~/code/my-app
lazycode
```

## The first run

A first-run screen asks for a theme and whether to auto-save, and offers to install language tools (`gopls`, `pyright`, `typescript-language-server`). It shows the exact command before anything runs, and you can skip all of it. It also tells you about the daily update check, which you can turn off.

The first time you open a folder, LazyCode asks whether to trust it. Trusted means it may run things there (language servers, your run command) without asking each time. Otherwise it asks before running anything. `:trust` and `:untrust` change your mind later.

## Moving around

The screen has a file tree on the left and tabs on the right. Everything has two routes: a key, and a command typed after `:`. Press `:` in the file tree, or Esc then `:` in a file.

| Do this | Key | Command |
|---|---|---|
| Move in the tree, open a file or folder | Arrow keys, Enter | |
| Go into the current tab, or back to the tree | Tab, and Ctrl+\\ back | |
| Open a file by name | Ctrl+P | |
| Save | Ctrl+S | `:save` or `:w` |
| Previous or next tab | Alt+, and Alt+. | |
| Close the tab | Ctrl+W | `:close` |
| Split the view | Alt+S | `:split` or `:split down` |
| Find in a file | Ctrl+F | `:find` |
| Search the folder | Alt+F | `:search` |
| Quit | | `:quit` or `:q` |

**On macOS, read every Alt shortcut as Ctrl+K, then the key.** Alt+S is Ctrl+K, then S. Option types special characters on a Mac, so Alt doesn't work by default there.

The mouse works: click files, tabs and cursor positions, and scroll. LazyCode takes the mouse, so hold Shift to select text the terminal's way.

## Several folders in one project

LazyCode opens on the current folder. To group folders, type `:project new Shop`, then `:dir add ~/code/shop-web`, and again for each folder. Switch with Alt+O (projects) and Alt+D (directories), or `:project` and `:dir`. It's saved in `projects.toml` ([Keys and config](keys-and-config.md)).

## Terminals and AI tools

`:term` opens your shell in a tab, and `:term claude` runs a command, here an AI tool. Inside a terminal tab every key goes to the tool, except Ctrl+\\, which takes you back out.

Alt+I shows AI sessions: Enter goes to one or resumes it, N starts a new one, R renames it. An idle session stops after 30 minutes to free memory, and resumes the same chat when you select it. Alt+E goes back to the files. When the tool changes a file you have open, it updates on screen, and changed lines are marked. If you have unsaved edits in it, LazyCode never replaces them.

## Running your project

Alt+P (or `:play`) runs the project in the current directory. `:play cargo build` sets a different command and remembers it.

## Git and language tools

Alt+G shows Git: S stages, U unstages, C commits, P pushes, L pulls, F fetches, B switches branch. Anything that writes asks first in a folder you haven't trusted. In a Go, Python, JavaScript or TypeScript file, F12 goes to a definition, F2 renames, and Alt+Shift+F formats, if the language's server is installed.

## Where next

[Keys and config](keys-and-config.md) lists every key and setting. Used VS Code? [Coming from VS Code](coming-from-vscode.md). Used Neovim? [Coming from Neovim](coming-from-neovim.md). Something wrong? [Troubleshooting](troubleshooting.md).
