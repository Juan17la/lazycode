# Components

**Status:** Proposed design, based on your decisions up to Q-79. Package names can change. Open points link to [03-open-questions.md](03-open-questions.md).

## Component map

```text
cmd/lazycode           starts everything
internal/
  app                  UI loop, messages, wiring
  ui                   layout, panels, tabs, splits, command bar, themes
  buffer               text storage, undo
  editor               cursors, selections, editing commands, recovery files
  keymap               default keys, rebinding
  command              the : commands and aliases
  syntax               highlighting
  lsp                  language server client
  tree                 file tree
  search               find and replace across files
  git                  wraps the git command
  github               wraps the gh command
  term                 pseudo-terminal, emulator, terminal tab
  session              AI sessions, sleep, resume, status
  marks                live file updates, modified-line markers
  notify               OS notifications
  run                  :play, detection
  project              projects, directories, switching, restore
  plugin               Lua runtime, API
  store                plugin store
  config               TOML loading, validation, paths
  trust                directory trust
  onboard              first-run setup
  update               release check, :update
```

**Dependency rules:**

- `buffer` depends on nothing else, so it can be tested alone.
- Feature packages don't import `ui`, and report through messages to `app`.
- Only `term` knows about the emulator library, so it can be replaced.
- Only `syntax` knows about the highlighting engine.

## Editor core and text buffer

**`buffer`:** A piece table: the original file stays unchanged, new text goes in an append-only slice, and a list of pieces describes the document.

| Requirement | How it's met |
|---|---|
| 100 MB files (Q-14) | The original file is read lazily or memory-mapped, not parsed into objects. The line index is built in the background. With about 2.5 million lines, it takes about 20 MB at 8 bytes per line. |
| Bigger files (Q-65) | Over 100 MB opens read-only without highlighting. Offsets are 64-bit, so the limit can rise later. |
| Binary files (Q-65) | A message only. Detected by looking for NUL bytes at the start of the file. |
| Undo | Saved piece changes. A multi-cursor edit counts as one step. |
| Safe saving | Write to a temporary file, flush it, then rename. Nothing replaces the file until the write succeeds. |
| Encoding | UTF-8. Other encodings: later. Line endings are preserved. |

Text width uses a Unicode width library, because emoji and CJK characters occupy two cells.

**`editor`:** Holds a sorted list of cursors and selections.

| Feature | Behaviour |
|---|---|
| Ctrl+D (Q-17) | Select the word, then add the next occurrence. |
| Ctrl+F2 (Q-17, Q-80) | Select all occurrences. Ctrl+K then A on macOS. |
| Edits | Applied from last cursor to first, so that offsets stay valid. |
| Find and replace (Q-17) | Plain text in the current file. |
| Auto-save (Q-16) | Off by default. |
| Crash recovery (Q-61) | Unsaved edits are written to a recovery file about a second after typing stops, and offered on the next start. |
| Closing unsaved work (Q-61) | LazyCode asks. |

## Layout, tabs and panels

**`ui`:**

- **Panels:** At most two (Q-18). Each can hold any set of views, and sit on either side.
- **Tabs:** **Each directory has its own tab bar** (Q-62). Types are file and terminal, and they look different (Q-25).
- **Splits:** A simple tree of editor areas (Q-15). Dragging can be added later, because mouse handling is routed through the same layout tree (Q-13).
- **Command bar:** One line for `:` commands and messages.

## File tree

**`tree`:** Loads folders only when expanded. Watches expanded folders with `fsnotify`, which is needed because AI tools edit files all the time. Shows the selected directory by default, with an option to show all (Q-41).

**Proposed:** In the combined view, opening a file from another directory also switches to that directory, because tabs belong to directories.

## Search

**`search`:** File names and contents in the active directory (Q-19), plain text only.

- Runs in the background, and new input cancels the old search.
- Skips `.gitignore` matches and binary files.
- **Replace** shows what will change first, and saves using the same safe writes as the editor.

## Git and GitHub

**`git`:** Runs the `git` command with fixed argument lists, never through a shell string.

- **History:** `git log --graph` data, drawn as a VS Code style view (Q-20).
- **Buttons:** Push, pull and fetch (Q-63).
- **Later versions:** Diffs, stage, commit, branches and merge conflicts (Q-63).
- Runs in the background, and refreshes when files change.

**`github`:** Runs `gh` for login status and repository creation (Q-21). Holds no tokens. Pull requests and Actions stay on github.com (Q-64).

## Commands and keymaps

**`command`:** Registers names and aliases such as `quit` and `q`. Plugins can add commands.

**`keymap`:** Resolves a key by context, loads defaults, then applies the user's TOML. It reports invalid or duplicate bindings. See [07](07-ui-ux.md).

**Escape then `:` in a non-modal editor:** Escape first clears any selection or extra cursors. The next key `:` opens the command bar. Any other key behaves normally. A plain `:` without Escape types a colon.

## Language support

**`syntax`:** Uses the official tree-sitter C binding (chosen under Q-69, see [04](04-feasibility.md#syntax-parsing)).

- Files over about 1 MB don't get highlighting, because memory scales with file size.
- Syntax trees are freed for hidden tabs, and rebuilt when the tab is shown.
- Grammars for the four default languages are built into the binary.

**`lsp`:** A later version. Small client for go-to-definition, rename and format (Q-28, Q-51). Starts one server per language and directory, on first need. Rename shows what will change before applying it. See [09](09-language-and-plugin-api.md).

## Embedded terminal

**`term`:** A pseudo-terminal plus an emulator, shown as a tab. Used by `session`, `run` and plain shells.

- Scrollback defaults to 1,000 lines, and can be changed (see [12](12-performance-budget.md)).
- Screens are only redrawn when the tab is visible.
- Handles resize, colours, mouse and pasted text.
- Keys pass through, except Ctrl+\ (Q-74).
- The emulator handles the tool's escape sequences. They never reach the user's terminal directly (see [11](11-security-and-privacy.md)).

**Library findings from the prototype (`charmbracelet/x/vt` v0.1.0):**

- `SafeEmulator.CellAt` returns a pointer to the live screen after releasing its lock, which the race detector caught. `term` uses the plain emulator with its own lock instead.
- `SendKey` silently drops special keys with modifiers, such as Shift+Tab and Ctrl+Left, which AI tools use. `term` sends those sequences itself.
- Its input pipe is unbuffered, so a large paste can deadlock with a tool that echoes it. `term` queues input, and a test covers this.
- Some tools leave processes that can't be stopped with their tab: `opencode` started `opencode serve --service`, which moved itself under systemd and kept running. Sleep and memory work should expect this (`M0-08`, `M0-09`).

## Harness sessions

**`session`:** Starts, lists, renames, hides, resumes, sleeps and tracks status. See [10](10-harness-integration.md).

**`notify`:** OS notifications (Q-72). A small package per OS, using each system's standard notification method.

## Watching AI changes

**`marks`:** From Q-53 and Q-76. It only shows which lines were modified, and has no review screen.

```text
File changes (fsnotify)
      │  combined, then read in the background
      ▼
  Open with no unsaved edits? ─ yes ─► reload the buffer ─► update line markers
      │
      └─ Unsaved edits? ─► keep them, and show a message
```

**What lines are compared against (Proposed):** The last Git commit, as VS Code's line markers do. In a non-Git folder, the file as it was when opened. You didn't specify this in Q-76, so it's easy to change.

**Limits:** Only open files show markers. Your own unsaved edits count as modified too. LazyCode only sees files, so two sessions in one directory can't be told apart. Very frequent writes are combined, so that screen updates stay smooth.

## Projects and workspace

**`project`:** Owns projects, the active directory and switching (Q-62).

| Action | Tabs, tree, search, git | Processes |
|---|---|---|
| Switch directory | Change to that directory | Keep running |
| Switch project | Change to its active directory | Keep running (Q-34) |
| Close project | Saved, then cleared | Stop, and sessions show as sleeping next time |

**Proposed:** Selecting a session in another directory switches to that directory first.

Missing directories are marked, and the user decides (Q-66).

## Run commands and custom shortcuts

**`run`:** Runs a directory's command in a terminal tab (Q-25).

**Order (Q-56):** A saved command first, then detection, then asking the user. The answer is saved for that directory.

**Detection (Proposed):**

| Found | Command |
|---|---|
| `package.json` with a `dev` script, otherwise `start` | `npm run dev`, or the same with `pnpm`, `yarn` or `bun` if its lock file is present |
| `go.mod` | `go run .` |
| `main.py` | `python main.py` |

Pressing again goes to its tab (Q-57). The command is only run when the directory is trusted.

## Plugins and store

**`plugin`:** One Lua state per plugin, started on first need. Before any plugin is added, the user sees what it does and accepts it (Q-67). See [09](09-language-and-plugin-api.md).

**`store`:** Reads the index from the store's Git repository, then downloads and checks plugins. Comes later (Q-06, Q-49).

## Configuration and others

**`config`:** Loads and validates TOML, and finds OS-specific paths with Go's standard library. See [08](08-data-and-config.md).

**`trust`:** Remembers which directories are trusted. See [11](11-security-and-privacy.md).

**`onboard`:** First-run setup. For language tools, it shows the command and runs it after confirmation (Q-68). See [07](07-ui-ux.md).

**`update`:** Checks GitHub's latest release at most once a day, and shows a message. `:update` installs it (Q-71, Q-79). See [15](15-release-and-governance.md).
