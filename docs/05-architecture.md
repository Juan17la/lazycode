# Architecture

**Status:** Draft, updated up to Q-79. **Decided** comes from your answers, **Proposed** is a design for you to review, and **TBD** waits on a question in [03-open-questions.md](03-open-questions.md).

## Goals and constraints

| Goal or constraint | Status | Source |
|---|---|---|
| Written in Go | Decided | Q-09 |
| Own editor core | Decided | Q-11 |
| Windows, macOS and Linux at launch | Decided | Q-07 |
| Lua plugins | Decided | Q-30 |
| The editor itself stays under 300 MB | Decided | Q-70 |
| As quick as Zed | Decided, numbers proposed | Q-42 |
| Runs well on an i3 with 8 GB | Decided | Q-43 |
| Runs in the user's default terminal, like other lazy apps | Decided | Q-50 |
| Show modified lines live while an AI works | Decided | Q-53, Q-76 |

## System overview

**Proposed:** one Go program that also manages child processes. It's a modular monolith, with no separate server or daemon.

```text
┌───────────────────────── User's default terminal ─────────────────────────┐
│ ┌────────────────────────── LazyCode process ───────────────────────────┐ │
│ │                                                                       │ │
│ │   Key and mouse input ──► Keymap ──► UI loop ──► Screen diff (tcell)  │ │
│ │                                         ▲                             │ │
│ │                                         │ messages                    │ │
│ │  ┌────────┬────────┬───────┬────────────┼──┬──────┬────────┬───────┐  │ │
│ │  │ Editor │ Syntax │ Tree, │ Git,       │  │ Term │ Lua    │Project│  │ │
│ │  │ buffer │        │ search│ GitHub     │  │Marks │ plugins│config │  │ │
│ │  └────────┴────────┴───────┴─────┬──────┴──┴──┬───┴────────┴───────┘  │ │
│ └──────────────────────────────────┼────────────┼───────────────────────┘ │
└────────────────────────────────────┼────────────┼─────────────────────────┘
                                     │            │
                   pipes (JSON-RPC, CLI)          │ pseudo-terminal (PTY or ConPTY)
                                     ▼            ▼
                  gopls, pyright,              claude, codex, opencode,
                  git, gh                      npm run dev, shells
```

The app icon only opens the default terminal running `lazycode` (Q-50).

## Process model

**Proposed,** following your answers:

| Process | Started | Stopped | Connected by |
|---|---|---|---|
| LazyCode | By the user | By the user | |
| Language server (later version) | On the first file of that language, in a trusted directory | When unused, or on project close | Pipes (LSP) |
| `git`, `gh` | For each operation | When the operation ends | Pipes |
| AI session | When the user starts or clicks it | When idle (Q-58, Q-77), or on project or window close (Q-34) | Pseudo-terminal |
| Run command | On `:play` | When it ends, or on project or window close (Q-39) | Pseudo-terminal |

If LazyCode crashes, child processes must not linger: use process groups on Unix and Job Objects on Windows.

## Threading and event loop

**Proposed:** one goroutine, the UI loop, owns all editor state. Everything else sends it messages.

```text
input ──────────┐
terminal output ┤
LSP replies ────┼──► channel ──► UI loop: update state, then redraw when needed
git results ────┤
file changes ───┘
```

- This removes most locking, and most race conditions.
- Nothing slow runs in the UI loop: no disk, network or child process waits.
- Redraws are combined, at most about 60 a second. Terminal output can be very fast.
- Results that arrive for an old state, such as an old search, are dropped by version number.
- Lua runs in the UI loop with a time limit (see [09](09-language-and-plugin-api.md)).

### Key routing

**Proposed:**

```text
Key press
  ├─ Ctrl+\ (Q-74)? ─► always handled by LazyCode
  ├─ Terminal tab focused? ─► sent to the tool inside
  └─ Otherwise: command bar ► panel ► editor, using the keymap
```

The user's terminal sees every key before LazyCode does, and may never pass some on. That's why each main action also has a `:` command.

## Rendering

**Proposed:** tcell draws a cell grid and sends only what changed. It supports Windows consoles and low-colour terminals, and it asks for the newer keyboard protocol on terminals that have it. The test binary emitted the protocol's enable sequence on startup. Actual key delivery wasn't tested.

Bubble Tea was not chosen: it builds each frame from text, which suits forms better than an editor with large buffers.

## Live file updates

**Proposed (Q-53):** The OS's file notifications (`fsnotify`) report changes in the active directory and any directory with an awake session. Changes are combined, then reloaded in the background, and the UI loop applies them to clean buffers. Buffers with unsaved edits are never overwritten.

The piece table makes this cheap, because reloading only needs a new original file and index. Modified-line markers are computed in the background. See [06](06-components.md#watching-ai-changes).

## Go-specific rules

**Proposed:**

- Text content lives in large byte slices, not many small objects, so that garbage collection stays cheap.
- Set `GOMEMLIMIT` near the 300 MB limit, and tune `GOGC`. In testing, `GOGC=50` reduced one terminal's use from 185 MB to 143 MB.
- Measure with `pprof` before optimising.

## Technology stack

| Purpose | Choice | Status | Evidence |
|---|---|---|---|
| Language | Go | Decided (Q-09) | Local base: 5.9 MB, binary 3.7 MB |
| Terminal drawing | `gdamore/tcell/v2` | Proposed | v2.13.10 builds for all three OSes |
| Lua | `yuin/gopher-lua` | Proposed, following Q-30 | Adds about 2 MB, used by micro |
| Pseudo-terminal | `aymanbagabas/go-pty` | Proposed | v0.2.3 passes CI on all three, needs Go 1.25+. |
| Terminal emulator | `charmbracelet/x/vt` | Proposed, with risk | v0.1.0, MIT, heavy memory per cell |
| Highlighting | Official `tree-sitter/go-tree-sitter` | Chosen under Q-69 | 75 MB total on a 1 MB file, measured on Linux. Needs a C compiler. Parser tests pass in CI on all three. |
| Text buffer | Own piece table | Proposed | See [06](06-components.md) |
| Git and GitHub | The `git` and `gh` commands | Decided for `gh` (Q-21), proposed for `git` | No libraries to keep updated |
| Language servers | Own small LSP client | Proposed | Only 3 features needed at first |
| File changes | `fsnotify` | Proposed | Not tested yet |
| Config | A Go TOML library | Decided TOML (Q-26) | |

Add dependencies only when needed.

The C binding means the project no longer builds as pure Go. Language packs adding a grammar need to account for this: see [09](09-language-and-plugin-api.md).

## Alternatives considered

| Alternative | Why not |
|---|---|
| Rust | Suggested earlier, but you chose Go. Testing shows Go's base memory is fine. |
| Embedding Neovim or Helix | You chose your own editor (Q-11). |
| Separate background server | Unneeded, since sessions stop when the window closes (Q-34). |
| WASM plugins | You chose Lua (Q-30). |
| Own window, or a bundled terminal | Dropped by Q-50. |
| Pure-Go tree-sitter | Two to three times heavier than the C binding in testing. |
