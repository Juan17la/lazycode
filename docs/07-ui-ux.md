# UI, UX and Keymap

**Status:** Draft, updated up to Q-80. **Decided** comes from your answers, and **Proposed** is for you to review. Mockups are proposals.

## Design principles

| Principle | Source |
|---|---|
| Everything in one window, without feeling overwhelmed. | Original brief |
| The usual VS Code keys, so that any developer can start immediately. | Q-01, Q-22 |
| Commands for speed, but never required. Mouse clicks work too. | Q-13, Q-22 |
| One primary colour and one accent colour, in a simple style similar to Claude Code's. | Q-48 |
| Show only one sidebar by default. | Q-18 |

## Onboarding

Shown on first run, and any step can be skipped (Q-16, Q-29).

```text
┌─ Welcome to LazyCode ─────────────────────────────┐
│                                                   │
│  Theme          [Dark]  Light                     │
│  Auto-save      [Off]   On                        │
│                                                   │
│  Language tools                                   │
│    [x] Go                 gopls       installed   │
│    [ ] Python             pyright     missing     │
│    [ ] TypeScript, JS     ts server   missing     │
│                                                   │
│                       [Skip]   [Continue]         │
└───────────────────────────────────────────────────┘
```

**Decided (Q-68):** Selecting a missing tool shows its exact install command, and LazyCode runs it after the user confirms.

The screen also says LazyCode checks GitHub for updates, with a setting to turn that off (Q-79). On macOS, it explains the Ctrl+K shortcuts (Q-80).

**Built (P-53, P-54):** Up and Down move, Space changes a row, Enter continues, and Esc skips. It shows while `config.toml` doesn't exist, and either way out creates that file. Each install is confirmed once more, then runs in a terminal tab started in the home folder, so its output can be read.

## Startup screen

**Proposed:** Recent projects and three actions. No panels until a project is open.

```text
 LazyCode

 Recent projects
   Facebook               4 directories
   Personal site          1 directory

 Ctrl+N  New project     Ctrl+O  Open folder     Esc :  Commands
```

## Main layout mockups

**Default:** One left sidebar (Q-18).

```text
 Facebook › frontend-facebook                                    ● 2 sessions
┌──────────────────────┬────────────────────────────────────────────────────┐
│ Files  Git  Search AI│  app.ts   login.ts  │ ▶ npm run dev │ ✦ Claude     │
├──────────────────────┼────────────────────────────────────────────────────┤
│ ▾ src                │  1  export function login(user: User) {            │
│   ▸ components       │  2    return api.post("/login", user);             │
│     app.ts           │  3  }                                              │
│     login.ts         │                                                    │
│   package.json       │                                                    │
├──────────────────────┴────────────────────────────────────────────────────┤
│ login.ts is already open                               main  Ln 2  TypeScript│
└───────────────────────────────────────────────────────────────────────────┘
```

File tabs show only a name. Terminal tabs have a marker: `▶` for run commands and `✦` for AI, and the marker changes while output is active (Q-25). Each directory has its own tabs, so this bar changes when switching directory (Q-62).

**Watching AI changes (Q-53, Q-76):** A split with the AI session beside a file. The file updates live, and `▌` marks modified lines. There's nothing more to it.

```text
 Facebook › frontend-facebook
┌──────────────────┬───────────────────────────┬───────────────────────────┐
│ Files Git Search │  login.ts                 │  ✦ Claude: auth           │
│ AI               ├───────────────────────────┼───────────────────────────┤
├──────────────────┤   1  export function login│  > Add rate limiting      │
│ ▾ src            │ ▌ 2    await limit(user); │                           │
│     login.ts     │ ▌ 3    return api.post(   │  Editing login.ts...      │
│     limit.ts     │   4  }                    │                           │
└──────────────────┴───────────────────────────┴───────────────────────────┘
```

**Two panels:** Ctrl+B opens the second (Q-22). Views can be moved between them, and each can sit on either side (Q-18).

```text
┌────────────────┬────────────────────────────────────┬──────────────────┐
│ Files Git Search│  app.ts   login.ts                 │ AI sessions      │
├────────────────┼────────────────────────────────────┼──────────────────┤
│ ▾ src          │  1  export function login() {      │ ● Claude: auth   │
│     app.ts     │  2    ...                          │ ◐ Codex: tests   │
│     login.ts   │                                    │ ○ OpenCode: api  │
└────────────────┴────────────────────────────────────┴──────────────────┘
```

**Session markers** (Q-36, Q-72): The green detail is decided, and an OS notification goes with it. The symbols are proposed.

| Marker | Meaning |
|---|---|
| `●` green | Finished, or asking for something |
| `◐` | Running |
| `○` | Sleeping: not using RAM. Click to resume the same chat. |

Colour is never the only signal, because each state has its own shape.

**Switcher:** Same for directories and projects.

```text
┌─ Switch directory ───────────────┐
│ auth                             │
│ › microservice-facebook-auth     │
│   frontend-facebook              │
│   microservice-facebook-post     │
└──────────────────────────────────┘
```

## Focus model

**Proposed:**

- Exactly one area has focus, and its border uses the accent colour.
- A click moves focus (Q-13).
- Editor areas, panels and the command bar use the keymap.
- **Terminal tabs get all keys except Ctrl+\** (Q-74). The command bar shows that key while a terminal tab has focus.
- When a terminal tab's command exits, focus returns to the file tree, so keys don't go to a tool that's gone.
- Ctrl+\ also moves focus from a file to the file tree, and Tab moves it back.

## Default keymap

All of these can be changed (Q-24). **Decided** rows come from your answers. The rest follows your "typical VS Code" rule, and is **Proposed**.

LazyCode runs in the user's terminal, which receives keys first (Q-50). The defaults follow your choices in Q-74 and Q-80: Linux and Windows share one set, and **on macOS, every Alt shortcut below is Ctrl+K then the same key**, such as Ctrl+K then D for Alt+D. Option types special characters on macOS by default, so Alt also works there only if the terminal's Option setting is on. The prototype tests all of them in real terminals.

### Commands

| Command | Short form | Action | Status |
|---|---|---|---|
| `:settings`, `:appearance`, `:themes`, `:plugins`, `:shortcuts`, `:onboard`, `:menu` | | Open that menu | Decided (D-44) |
| `:new`, `:wrap`, `:dock` | | New file, word wrap, show or hide the dock | Decided (D-44) |
| `:project new`, `:dir add` | | With no name or path, open the form | Decided (D-44) |
| `:play` | `:p` | Run the directory's command | Decided, short form proposed |
| `:save` | `:w` | Save | Decided, short form proposed |
| `:quit` | `:q` | Quit | Decided |
| `:config` | | Open configuration | Decided |
| `:project` | | Switch project. A backup for its shortcut. | Decided (Q-74) |
| `:dir` | | Switch directory. A backup for its shortcut. | Decided (Q-74) |
| `:project new NAME` | | Make a project holding the current directory | Proposed |
| `:dir add PATH` | | Add a directory to this project. A relative path starts from the current directory. | Proposed |
| `:dir remove` | | Take this directory out of the project. The folder is untouched. | Proposed |
| `:tree` | | Show all of the project's directories in the tree, or go back to one (Q-41) | Proposed |
| `:config projects` | | Open `projects.toml`. Saving it applies it. | Proposed |
| `:session` | | Start an AI session | Proposed |
| `:update` | | Install the latest release | Decided (Q-79) |
| `:split` | | Split the view. `:split down` splits below. | Proposed |
| `:selectall` | | Select all occurrences | Decided (Q-80) |
| `:find`, `:replace` | | Find or replace in the file | Proposed |
| `:view` | | Move the Files view to the other panel | Proposed |
| `:search`, `:git` | | Show the Search or Git view | Proposed |
| `:push`, `:pull`, `:fetch` | | The Git buttons | Proposed |
| `:repo create` | | Create a GitHub repository through `gh` | Proposed |
| `:definition`, `:rename`, `:format` | | Language server actions | Proposed |
| `:session hidden` | | List hidden sessions, and bring one back | Proposed (P-42) |
| `:project close` | | Close this project's tabs, and move to the next project | Proposed (P-43) |

**Git view keys (P-32):** `p` push, `l` pull, `f` fetch, `c` commit, `s` stage or mark resolved, `u` unstage, `b` switch branch, `n` new branch, `m` merge, `x` abort a merge, `o` open the file, `i` create a repository, `r` refresh, Enter opens a diff or commit.

**Search view keys:** Tab moves between Find and With, Enter opens a result or starts a replace from With, Esc goes back to the tab, and `:` opens the command bar when the field is empty.

### Shortcuts

| Key | Action | Status |
|---|---|---|
| Esc, then `:` | Open the command bar | Decided |
| Alt+, and Alt+. | Previous and next tab | Decided (Q-80) |
| Alt+E, G, F, A, T | Show Explorer, Git, Find in directory, AI sessions, the terminal dock | Decided (D-44) |
| Alt+P, Alt+D, Alt+R | Projects, directories, run | Decided (D-44), replacing Alt+O and Alt+P for run |
| Alt+M or F10, Alt+S, Alt+K or F1, Alt+X, Alt+Z | Menus, settings, shortcuts, command bar, word wrap | Decided (D-44) |
| Alt+Shift+P, F, T, A, S, B | New project, add a folder, new terminal, new AI session, split right, split below | Decided (D-44) |
| Ctrl+N, Ctrl+R, Ctrl+Q | New file, replace in file, quit | Decided (D-44) |
| F3 | Format | Decided (D-44), replacing Shift+Alt+F |
| Ctrl+B | Toggle second panel | Decided |
| Ctrl+D | Add next occurrence | Decided |
| Ctrl+F2 | Select all occurrences. On macOS, Ctrl+K then O. | Decided (Q-17, Q-80, D-44) |
| Ctrl+S | Save | Proposed |
| Ctrl+Z, Ctrl+Y | Undo, redo | Proposed |
| Ctrl+X, Ctrl+C, Ctrl+V | Cut, copy, paste | Proposed |
| Ctrl+F | Find in file | Proposed |
| Alt+F | Search panel | Proposed, replacing Ctrl+Shift+F |
| Alt+G | Git panel | Proposed |
| Ctrl+P | Open a file by name | Proposed |
| Ctrl+W | Close tab | Proposed |
| Alt+W | Next split | Proposed (P-21) |
| Alt+V | Move a view to the other panel, from the tree | Proposed (P-21) |
| Ctrl+A | Select everything | Proposed |
| F12, F2 | Go to definition, rename | Proposed |
| Alt+D | Switch directory | Decided (Q-74) |
| Ctrl+\\ | Leave a terminal tab | Decided (Q-74) |

Inside a terminal tab, every key except Ctrl+\\ and Alt+Q (close the tab) goes to the tool. Ctrl+C interrupts rather than copies, and Alt+D goes to the tool, so press Ctrl+\\ first.

### Known key clashes

Terminal and OS defaults, which users can usually change in settings. Priority terminals come first (Q-75).

| Key | Clash | Source | Status |
|---|---|---|---|
| Alt+Left, Alt+Right | Windows Terminal: move between panes. | Microsoft docs | Avoided (Q-80) |
| Ctrl+PageUp, Ctrl+PageDown | GNOME Terminal: switch tabs. | Forum answer, official list not confirmed | Avoided (Q-80) |
| Ctrl+F2 | macOS: focus the menu bar. | Apple docs | Avoided on macOS (Q-80) |
| Alt+F2 | Linux desktops: commonly opens a run dialog. | General knowledge, not checked | Avoided (Q-80) |
| Alt+key | macOS: types special characters unless the Option setting is on. | Apple's Terminal list, secondary source | Avoided on macOS (Q-80) |
| Ctrl+H | Most terminals: sends the same byte as Backspace. | Tested (`M0-07`) | Avoided |
| Alt+\\ | Most terminals: sends `ESC \`, a control sequence that's dropped. | Tested (`M0-07`) | Avoided |
| Ctrl+V | Windows Terminal: pastes. It arrives as a paste, which does the same job. | Tested (`M0-07`) | Accepted |
| Alt+Shift+D | Windows Terminal: duplicate pane. | General knowledge, not checked | Avoided (D-44) |
| F1, F10 | GNOME Terminal: help and menu bar. | General knowledge, not checked | Each has an Alt form too (D-44) |
| Ctrl+Tab | Windows Terminal and Kitty: next tab. | Microsoft docs, Kitty source | Avoided (Q-74) |
| Ctrl+Shift+D | Windows Terminal: duplicate tab. | Microsoft docs | Avoided (Q-74) |
| Ctrl+Shift+Space | Windows Terminal: new tab dropdown. | Microsoft docs | Avoided (Q-74) |
| Ctrl+Shift+P | Kitty: prefix for hints. | Kitty source | Avoided (Q-74) |

**No default found** in Windows Terminal's docs, Kitty's docs or Apple's lists for: Ctrl+\\, Alt+D, Alt+O, Alt+P, Alt+, and Alt+. on Linux and Windows, or Ctrl+K on macOS, where Apple lists it as reaching the program. **Tested on 2026-11-04 (`M0-07`).** `cmd/keytest` ran in real Windows Terminal, macOS Terminal and Kitty on CI, with simulated key presses, and every listed key arrived. Kitty also passed with a Spanish layout, including Ctrl+\\. The presses were simulated, so a physical keyboard and other layouts are still worth a try: run `go run ./cmd/keytest` in the terminal, press each key, then press q.

## Mouse behaviour

**Added by D-44.** Clicking works on everything, including tab close buttons and the buttons at the end of each tab bar. Dragging selects text, moves a tab along its bar or to another split, moves the panel edges, the dock's top and split lines, and moves the scroll bar. A double click takes a word and a triple click a line. The middle button closes a tab, and the right button opens a menu on a tab, in a file or in the file tree. In a terminal tab dragging selects what's on screen and copies it, unless the tool asked for the mouse.


- **Decided (Q-13):** Clicking and scrolling. Dragging later.
- **Proposed:** Click tabs, tree items, buttons, cursor positions and the panels' view titles. A terminal tab receives mouse input if its tool asks for it.

## Themes

**Decided (Q-48, widened on 2026-11-05):** Twelve built-in themes, chosen with `:theme`, which previews each one and saves the choice: `dark`, `light`, `gruvbox-dark`, `gruvbox-light`, `dracula`, `one-dark`, `nord`, `tokyo-night`, `catppuccin-mocha`, `solarized-dark`, `solarized-light` and `monokai`. `[ui.colors]` changes any colour. Syntax has its own colour per kind of code. Terminals with fewer than 256 colours keep their own background and use reversed selections.

The area with the keys has a tinted background, and its title is a solid bar. Panel headers show icons, with the name beside the view on top. `icons = "basic"` works in any font, and `icons = "nerd"` adds file-type icons but needs a Nerd Font.

**Tree:** names are coloured by Git state, with a letter too so that colour isn't the only sign: `A` added, `M` modified, `U` untracked, `R` renamed, `!` conflict. A folder gets a dot in the colour of the strongest state inside it.

| Token | Use |
|---|---|
| `primary` | Main text, active items |
| `accent` | Focus, cursor, selections |
| `muted` | Borders, secondary text |
| `success` | Session attention marker |
| `error` | Errors |
| `syntax.*` | Code colours |

Colour values: **TBD**. Low-colour terminals use the closest available, along with bold and inverse text so that nothing depends on colour alone.

## Error and empty states

**Proposed:**

| Situation | Behaviour |
|---|---|
| No project open | Startup screen |
| Empty search | "No results in <directory>" |
| Not a Git repository | Git panel offers to create one |
| `gh` or tool missing | Say what's missing and how to get it |
| Language server missing or crashed | Message once in the command bar, and keep editing normally |
| Invalid config | Show file and line, and keep the last valid config |
| Plugin error | Show its name, disable it, and keep the editor running |
| Terminal smaller than the layout | Hide the second panel, then the first |
| Run command fails | Terminal tab stays open with exit code and output |
| Binary file | Message only (Q-65) |
| File over 100 MB | Read-only, no highlighting, and a message saying so (Q-65) |
| Unsaved edits, and the file changed on disk | Keep the edits, show a message, and let the user choose |
| Directory missing | Mark it, and ask what to do (Q-66) |
| Update available | A message in the command bar, at most once a day (Q-79) |

## Accessibility

**Proposed:** Everything works by keyboard. Status never relies only on colour. Both themes are checked for contrast. The terminal's own font size setting applies. Screen reader support in TUIs is limited, so this is a known limitation. Colour-blind themes: not requested so far (Q-48).
