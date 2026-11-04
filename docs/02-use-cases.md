# Use Cases

**Status:** Updated from all answers up to Q-80. Undecided steps are marked `TBD` and link to [03-open-questions.md](03-open-questions.md).

## Actors

| Actor | Role |
|---|---|
| Developer | Any level. Main user. |
| Plugin author | Writes Lua plugins or language support. |
| Store maintainer | Reviews submissions in the store repository. |
| AI harness | Claude Code, Codex, OpenCode, or any tool the user adds. |
| Git, `gh` | Provide repository operations and GitHub access. |
| Language server | Provides go-to-definition, rename and format. |

## Overview

| ID | Use case | Feature |
|---|---|---|
| UC-01 | Browse the tree and open a file | F1, F2 |
| UC-02 | Work with tabs and splits | F2 |
| UC-03 | Search and replace in a directory | F1 |
| UC-04 | Use git history and sync | F1 |
| UC-05 | Create a GitHub repository | F1 |
| UC-06 | Run a command from the command bar | F3 |
| UC-07 | Edit code with language support | F4 |
| UC-08 | Add a local plugin | F4, F8 |
| UC-09 | Create a project | F6 |
| UC-10 | Start an AI session | F5, F6 |
| UC-11 | Return to a session, get notified, and wake it | F5 |
| UC-12 | Switch directory | F7 |
| UC-13 | Switch and close projects | F9, F6 |
| UC-14 | Change a shortcut | F10 |
| UC-15 | Edit with multiple cursors | F3 |
| UC-16 | Publish and install from the store | F8 |
| UC-17 | Run code with `:play` | F10 |
| UC-18 | Install, launch and onboard | F11 |
| UC-19 | Watch AI changes | F12 |

## Editing and navigation

### UC-01: Browse the tree and open a file

1. The Developer opens the tree with a click or a shortcut.
2. LazyCode shows the selected directory. An option shows all of the project's directories.
3. The Developer selects a file.
4. LazyCode opens it in a tab.

- **Already open:** LazyCode goes to the existing tab and shows a message in the command bar.
- **Binary file:** A message is shown, and it doesn't open.
- **Over 100 MB:** It opens read-only, without highlighting.

### UC-02: Work with tabs and splits

1. The Developer moves between tabs with a shortcut or a click.
2. The Developer splits the view.
3. The Developer closes a tab.

- **Unsaved changes:** LazyCode asks before closing.
- **Dragging:** Later.

### UC-03: Search and replace in a directory

1. The Developer opens the search panel and types plain text.
2. LazyCode lists matching file names and contents in the active directory.
3. The Developer selects a result, and it opens in a tab.
4. The Developer may enter replacement text and apply it.

### UC-06: Run a command from the command bar

1. In a file tab, the Developer presses Escape, then `:`.
2. The Developer types a command, such as `:save` or `:q`, and presses Enter.
3. LazyCode runs it, and shows the result in the command bar.

- **Terminal tab focused:** Escape belongs to the tool inside. Press Ctrl+\ first.
- **Unknown command:** Message in the command bar.

### UC-15: Edit with multiple cursors

1. The Developer places the cursor on a word and presses Ctrl+D.
2. LazyCode selects the next occurrence and adds a cursor. Repeating adds more.
3. Alternatively, Ctrl+F2 selects all occurrences (Ctrl+K then A on macOS).
4. The Developer types, and every cursor edits at once.
5. Undo reverses it as one step.

### UC-07: Edit code with language support

1. The Developer opens a TypeScript, JavaScript, Python or Go file.
2. LazyCode highlights it.
3. In a later version, if the language server is installed, go-to-definition, rename and format are available.

- **File over about 1 MB:** Highlighting is off (see [12](12-performance-budget.md)).
- **Server missing:** LazyCode says so, and offers the install command.
- **Code not trusted:** Language servers don't start.

### UC-08: Add a local plugin

1. The Plugin author writes a Lua plugin in the local plugin folder.
2. LazyCode shows what the plugin does, and asks the Developer to accept it.
3. Once accepted, the plugin loads, and it can do anything.

- **Declined:** The plugin isn't loaded.

## Git and GitHub

### UC-04: Use git history and sync

1. The Developer opens the git panel for the active directory.
2. LazyCode shows the VS Code style history.
3. The Developer presses push, pull or fetch.
4. LazyCode runs the operation and shows the result.

- **Commit, stage, diffs, branches:** Later versions. Until then, use a shell tab or an AI tool.
- **Not a repository:** LazyCode offers to create one (UC-05).
- **Operation fails:** LazyCode shows Git's error.

### UC-05: Create a GitHub repository

1. The Developer chooses to create a repository.
2. LazyCode checks that `gh` is installed and logged in.
3. If it isn't logged in, LazyCode asks the Developer to complete `gh` login.
4. The Developer enters the name and visibility.
5. LazyCode creates it through `gh`.

- **`gh` missing:** LazyCode shows how to install it.
- **Pull requests and Actions:** On github.com.

## Projects

### UC-09: Create a project

1. The Developer creates a project, for example `Facebook`.
2. The Developer adds directories such as `frontend-facebook` and `microservice-facebook-auth`. They can be non-Git folders, or belong to other projects too.
3. LazyCode asks whether to trust each directory.
4. LazyCode saves the project to local configuration.

### UC-12: Switch directory

1. The Developer is viewing `frontend-facebook`.
2. The Developer presses Alt+D (Ctrl+K then D on macOS), or runs `:dir`.
3. The Developer selects `microservice-facebook-auth`.
4. Every view changes to that directory: tabs, tree, search and git.
5. Sessions and commands in `frontend-facebook` keep running.
6. Switching back restores its tabs, cursor positions and layout.

### UC-13: Switch and close projects

1. The Developer presses Alt+O (Ctrl+K then O on macOS), or runs `:project`, and selects a project.
2. LazyCode shows that project, at its active directory.
3. Sessions and commands in the previous project keep running.
4. When the Developer returns, they're as they were, unless an idle session went to sleep.
5. When the Developer closes a project, its sessions and commands stop.

- **Unsaved files on closing:** LazyCode asks.
- **A directory is missing:** It's marked, and LazyCode asks what to do. It might be an unplugged drive.

## AI sessions

### UC-10: Start an AI session

1. The Developer chooses to start a session.
2. The Developer selects a tool, such as Claude Code.
3. The Developer selects one of the project's directories.
4. LazyCode opens a terminal tab in that directory and starts the tool.
5. The session appears in the sessions panel, and the Developer can rename it.

- **Tool not installed:** LazyCode says so, and doesn't open a broken tab.
- **Add a tool:** The Developer adds a name, a command and optional resume fields to configuration.
- **Plain shell:** Works the same way, and appears in the same list.

### UC-11: Return to a session, get notified, and wake it

1. A session finishes a task, or asks for something, while another tab is in use.
2. LazyCode shows a green detail and an OS notification.
3. The Developer selects the session, and LazyCode shows its tab. If it's in another directory, LazyCode switches there first (Proposed).

- **Idle for a long time:** The tool is stopped to free its RAM, and shows as sleeping. This happens after 30 minutes without output, if the tab isn't focused and nothing the tool started is busy.
- **After closing and reopening the project:** Sessions show as sleeping.
- **Clicking a sleeping session:** LazyCode resumes that exact chat.
- **Tool has no resume fields:** A new session starts.
- **Removing a session:** It's hidden from the list, and the tool's data is untouched.

## Watching AI changes

### UC-19: Watch AI changes

1. An AI session is working in a directory.
2. The Developer has one of its files open, in a tab or in a split next to the session.
3. The file updates live, and modified lines are marked.
4. The Developer looks at those lines, then returns to the session or keeps working.

- **Files that aren't open:** No marker appears until they're opened.
- **Unsaved edits in that file:** LazyCode doesn't overwrite them. It shows a message, and lets the Developer choose.
- **Several sessions in one directory:** Modified lines can't be separated by session.

## Commands and shortcuts

### UC-14: Change a shortcut

1. The Developer runs `:config`.
2. The Developer changes a key binding in the TOML file.
3. LazyCode applies it. Invalid config shows a clear error, and the last valid config stays in use.

### UC-17: Run code with `:play`

1. The Developer runs `:play` or presses its shortcut.
2. LazyCode looks for this directory's saved command. If there is none, it detects a TypeScript, JavaScript, Python or Go project.
3. If nothing is detected, LazyCode asks for a command, such as `cargo build`, and remembers it for that directory.
4. LazyCode opens a terminal tab and runs it. The tab looks different from file tabs and shows activity.
5. The command runs until it ends, or the project or window closes.

- **Pressed again while running:** LazyCode goes to its tab.
- **Directory not trusted:** LazyCode asks first.

## Install, onboarding and store

### UC-18: Install, launch and onboard

1. The Developer installs LazyCode with a curl command, or a PowerShell one-liner on Windows.
2. The Developer runs `lazycode` in a terminal, or clicks the app icon, which opens the default terminal running it.
3. On first run, LazyCode offers theme, auto-save (off by default) and language tools, and says it checks GitHub for updates.
4. For each tool, LazyCode shows the install command, and runs it once the Developer confirms.
5. The Developer can skip anything. LazyCode saves the choices.

### UC-16: Publish and install from the store

1. The Plugin author submits the plugin through the store's Git repository.
2. The Store maintainer reviews and accepts it.
3. The Developer opens the store in LazyCode. Only accepted plugins appear.
4. LazyCode shows what the plugin does, and the Developer accepts it.
5. LazyCode downloads and loads it.
