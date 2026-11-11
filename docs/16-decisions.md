# Decision Log

**Status:** Current up to Q-80. To change a decision, add a new entry that replaces the old one, instead of editing history.

## Decided by the owner

Date for all entries: 2026-11-03.

| ID | Decision | Consequence | Source |
|---|---|---|---|
| D-01 | For any developer at any level, open source | Onboarding and documentation matter | Q-01 |
| D-02 | MIT licence | Dependencies must be compatible | Q-05 |
| D-03 | Windows, macOS and Linux at launch | Testing and terminal work on all three from the start | Q-07 |
| D-04 | Go | Light base. Needs careful memory design. | Q-09 |
| D-05 | Own editor core | Largest cost, and the highest risk of losing work | Q-11 |
| D-06 | Non-modal editing | Needs the Esc-then-`:` rule | Q-12 |
| D-07 | Lua plugins | `gopher-lua`, and Neovim plugins aren't compatible | Q-30 |
| D-08 | Plugin store through a reviewed Git repository | Ongoing review work | Q-06, Q-31, Q-67 |
| D-09 | Only sessions started from LazyCode are listed, plus plain shells | No dependence on tool storage formats | Q-32, Q-60 |
| D-10 | AI sessions and run output open as terminal tabs | Needs an embedded terminal | Q-25, Q-33 |
| D-12 | Tools added by name and command, plus optional resume fields | Tools without resume can't sleep | Q-35, Q-59 |
| D-13 | `gh` for GitHub. Pull requests and Actions stay on github.com. | No token storage in LazyCode | Q-21, Q-64 |
| D-14 | Configuration in local TOML, with `:config` | No repository config | Q-26, Q-38 |
| D-15 | Up to two panels, left sidebar by default | | Q-18 |
| D-16 | `:` commands and the usual VS Code shortcuts, with terminal-safe replacements | See D-41 | Q-22 |
| D-17 | Languages: TypeScript, JavaScript, Python, Go | | Q-27 |
| D-19 | No telemetry | Usage can't be counted | Q-44 |
| D-20 | Trust prompt | Untrusted directories run nothing | Q-45 |
| D-21 | No remote features for now | | Q-10 |
| D-23 | Runs in the user's default terminal. The app icon opens it. | No own window. Terminal defaults can take keys. | Q-50 |
| D-24 | First version adds syntax highlighting and AI sessions. Language servers come next. | First version grows | Q-51 |
| D-25 | No deadline and no user-count target | Roadmap has no dates | Q-52 |
| D-26 | Modern for the AI age: watch AI changes with modified lines shown, and nothing more | New feature F12, kept small | Q-53, Q-76 |
| D-27 | `:play` detects four languages, otherwise asks and remembers. Pressing it again goes to its tab. | | Q-56, Q-57 |
| D-28 | Idle sessions sleep and free RAM. Clicking resumes the exact chat. | Sleeps after 30 minutes, only if unfocused, not busy, and resumable | Q-58, Q-77 |
| D-29 | Crash recovery files, and asking before closing unsaved work | | Q-61 |
| D-30 | Each directory has its own tabs. Switching changes every view. | Replaces the one-tab-bar-per-project assumption | Q-62 |
| D-31 | First-version Git: history, push, pull, fetch, create repository. The rest is later. | No commit in the UI at first | Q-20, Q-63 |
| D-32 | Binary files show a message. Files over 100 MB open read-only without highlighting. | | Q-14, Q-65 |
| D-33 | Directories can be in several projects, and non-Git. Missing ones are marked, not removed silently. | Replaces Q-40's automatic removal | Q-40, Q-66 |
| D-34 | Store shows only accepted plugins. Local plugins can do anything, after the user accepts. | Consent is the only protection for local plugins | Q-67 |
| D-35 | Onboarding runs a language tool's install command after confirmation | | Q-29, Q-68 |
| D-36 | The editor itself stays under 300 MB, excluding third-party apps | Scrollback and highlighting need limits | Q-70 |
| D-37 | Curl and PowerShell install, unsigned. Update check at most daily, message only, `:update` installs. No pre-releases. | Replaces most of Q-47's formats | Q-71, Q-78, Q-79 |
| D-38 | Green detail and OS notification for session status | Detection is approximate | Q-72 |
| D-39 | Restore cursor positions and layout too | | Q-39, Q-73 |
| D-40 | Use measurements to choose highlighting: the official C binding, with no highlighting above about 1 MB | C compiler needed in builds | Q-69 |
| D-41 | Keys: Ctrl+\\ leaves a terminal tab, Alt+D directories, Alt+O projects, Alt+P run, Alt+Left and Right tabs. `:dir` and `:project` as backups. | Tab keys and macOS replaced by D-43 | Q-74 |
| D-42 | Priority terminals: Windows Terminal, macOS Terminal, Kitty. Others should work well enough. | Focuses testing | Q-75 |
| D-43 | Linux and Windows share keys: Alt+D, Alt+O, Alt+P, Alt+, and Alt+. for tabs, Ctrl+F2 to select all occurrences. macOS uses Ctrl+K then the same key, and Ctrl+K then A to select all occurrences. | Replaces part of D-41. Chosen from documented shortcuts, and tested in `M0-07`. | Q-80 |
| D-44 | The app behaves like a GUI: code tabs on top and a bottom dock for terminals and AI, a menu bar, windows for settings, appearance, themes, plugins, shortcuts and forms, dragging with the mouse, and shortcuts in families (Alt shows, Alt+Shift makes, Ctrl for the file) | Replaces Alt+O, Alt+I, Alt+H, Alt+S for split, Shift+Alt+F and Ctrl+K then A. A terminal app can't change its font or how much text fits, which is the terminal's zoom, so "fonts" are bold and italics plus the icon set | Owner's request, 2026-11-05 |

Other details are recorded in [01-requirements.md](01-requirements.md).

## Technical choices

These were proposed on the basis that they'd become decisions when M0 started, unless you objected. M0 started on 2026-11-03, so they now stand. Add a new entry to change one.

| ID | Proposal | Why | Alternative |
|---|---|---|---|
| P-01 | Single Go process, with a UI loop that owns state | Simple, and avoids most locks | Client and server split |
| P-02 | tcell for the screen | Windows and keyboard protocol support, used by micro | Bubble Tea |
| P-03 | `gopher-lua` | Pure Go, about 2 MB, used by micro | Embedded C Lua |
| P-04 | `go-pty` and `charmbracelet/x/vt` | Builds for all three OSes | Micro's terminal package, or writing an emulator |
| P-05 | Piece table for the buffer | Handles large files with little memory | Rope, gap buffer |
| P-06 | Run the `git` command, not a library | Small, and uses the user's Git config | `go-git` |
| P-07 | 1,000-line default scrollback | 36 MB against 185 MB at 10,000 | Compact custom store |
| P-08 | LazyCode creates tool session IDs | Tracks sessions without reading internal files | Reading tool storage |
| P-09 | Language packs in TOML, with no code | Easy for contributors | Lua only |
| P-10 | Store index pins exact commits | Plugins can't change after review | Latest version on each install |
| P-11 | `fsnotify` for live updates | Standard choice for OS file notifications | Polling |
| P-12 | Alt+H replaces in a file, and Alt+S splits, instead of the proposed Ctrl+H and Alt+\\ | Tests showed those two can't be read in most terminals | Commands only |
| P-13 | Run OpenCode with `--standalone` | Otherwise its service keeps about 320 MB after sleeping | Accept the service, sharing it between sessions |
| P-14 | 250 MiB Go memory limit | Keeps 10 full tabs at 239 MB, against 362 MB | Lower scrollback |
| P-15 | Read editable files into memory, and map only read-only files over 100 MiB | Tested on Linux: rewriting a mapped file changes the text under the buffer, and truncating it crashes the editor. Mapping didn't lower RSS either | Memory-map all large files |
| P-16 | Detect external changes by comparing contents | Times can miss a quick same-size rewrite, and a touch isn't a change | Size and modification time |
| P-17 | Clipboard through the OS's tools, then the terminal's OSC 52 code, then an internal copy | macOS Terminal has no OSC 52, and SSH sessions have no tools. Adds no dependency | A clipboard library |
| P-18 | Check for external changes when focus enters a file, until `D5` watches for them | Saving already refuses to overwrite, and this shows changes without a watcher | Wait for `D5` |
| P-19 | A second view of a file keeps its cursor's line and column when the other view edits | Far simpler than mapping through each edit. The cursor can land on different text, but it's visible before typing | Map cursors through every edit |
| P-20 | Find matches exact text and case only | Your answer was plain text, and it avoids Unicode case rules. Easy to add a toggle if you want one | Case-insensitive by default, as VS Code does |
| P-21 | Alt+W moves between splits, and Alt+V moves a view between panels | Alt+Left and Alt+Right clash in Windows Terminal (Q-80). Both are in `cmd/keytest` | Commands only |
| P-22 | Not added: a command package. The `:` commands are a switch in `app` | Only plugins need them registered, so that can wait for M5 | A registry now |
| P-23 | One `state.json`, keyed by directory, instead of `state/<project-id>.json` | Tabs belong to directories, and a directory can be in several projects (Q-62, Q-66). Per-project files would disagree about the same directory | One file per project |
| P-24 | The tree watches names only: new, removed and renamed files in folders on screen, combined over 100 ms | Writes don't change the tree, so an AI tool's edits cause no refreshes. Other directories catch up when shown | Watch everything, or poll |
| P-25 | Contain the emulator's failures: recover from panics, feed output in pieces cut around escape sequences, and cap numbers in control sequences at 9,999 | Fuzzing found that printed output could crash `vt` v0.1.0, and that `x` then `ESC[200000000b` froze the editor | Fix or replace `vt` first |
| P-26 | "Nothing the tool started is busy" (Q-77) means using under 1 second of CPU over a minute, across the tool and its children, and unknown means stay awake | AI tools keep idle helpers running for as long as they run, so "still running" would never let one sleep. A child sleeping on a timer with no CPU can still be stopped | Any child alive blocks sleep |
| P-27 | A visible tab never sleeps, even without focus | Stricter than Q-77, because it might be what you're watching | Only the focused tab is protected |
| P-28 | Files over 100 MiB don't reload live. They say the file changed | Reloading scans all of it again, on every append to a growing log | Reload them too |
| P-29 | Minimal trust now: asked when `:play` or a session would run in a folder, stored in `trust.toml`, with `:trust` and `:untrust` | `:play` and sessions run a folder's code, so they can't wait for `F1`. `:term` isn't asked about, as you typed that command yourself | Ship `D2` and `D3` without asking |
| P-30 | One `sessions.json` for all projects, separate from Track A's `state.json` | Sessions are found by directory anyway, and losing it only loses the list | A `sessions` key in the project state |
| P-31 | Files, Git and Search move between panels together, and AI moves alone | Matches the mockups and how Files and AI already behaved | Each view moves on its own |
| P-32 | Git and Search panel keys are fixed letters, not in `keys.toml` | Plain letters only mean something inside those panels | A rebindable action per letter |
| P-33 | Search ignores ASCII case when the query is all lower case, and is exact with a capital. Results stop at 5,000 and say so | VS Code's habit, with no toggle to build. P-20 stays for find in a file | An explicit toggle |
| P-34 | Directory replace changes text, never file names. A file with unsaved edits, or that changed since the search, is skipped and named. Past 20,000 preview rows, changes still apply but aren't listed, and the preview says so | Never overwrites unsaved work or an AI tool's edit | Ask per file, or page the list |
| P-35 | Search uses `git ls-files` for `.gitignore`. Outside Git, `.gitignore` isn't read | All the ignore rules, with no new dependency | An ignore-file parser |
| P-36 | Every writing Git operation asks for trust, including stage and creating a branch. `git init` and `gh repo create` don't | Hooks and filters run on stage and commit, and those two run no repository code | Only push, commit and merge |
| P-37 | Pull always merges (`--no-rebase --no-edit`), overriding `pull.rebase`. A first push asks, then sets the upstream on `origin` | No rebase in this version. A first push is the common case | `--ff-only`, or only show git's error |
| P-38 | History loads 150 commits per page. The Git view checks every 3 seconds while on screen, backing off when nothing changes, and refreshes after each operation | Bounded on huge repositories, and needs no watcher on `.git` | `--all`, or watching `.git` |
| P-39 | Diffs and commit details are read-only editor tabs with a made-up path under `.git/lazycode-views/`, which aren't restored | A real tab, with no way to save over a real file | A separate viewer |
| P-40 | Marking a conflict resolved is refused while `<<<<<<<` markers remain. Branch names starting with `-` are hidden | Stops committing markers, and a hostile repository's option-like names | A warning, or showing them and refusing use |
| P-41 | Git's messages are shown in English (`LC_ALL=C`) | So that failures can be recognised | The user's language |
| P-42 | `:session hidden` lists hidden sessions, and picking one brings it back | Reuses the switcher (FR-5.11) | A key in the AI view |
| P-43 | `:project close` moves to the next project. The only project stays open, with its tabs closed. AI sessions become sleeping entries, and shells go. A directory in two projects loses its tabs in both | Something must always be open, and tabs belong to directories (Q-62) | An empty state, or tabs per project |
| P-44 | Four syntax classes: keyword, literal, name and comment, with weight carrying part of the meaning | Q-48 allows two colours | A colour per token type |
| P-45 | Highlighting parses after a 40 ms pause, with the first parse at once. Spans cover the whole file, and may be briefly stale after an edit | The UI never calls tree-sitter | Querying only the visible range |
| P-46 | An incremental parse comes from comparing old and new text for a common start and end | Needs no edit tracking, and undo and live reloads are covered too | Recording every edit |
| P-47 | After closing a parser on Linux, `malloc_trim` hands memory back | Otherwise RSS grew about 8 MB per open and close, though nothing leaked (D-28) | Accept the higher RSS |
| P-48 | A language server starts only when F12, F2 or format is pressed, after the trust prompt. Its root is the nearest marker file, such as `go.mod`, inside the directory | Nothing runs by itself, and monorepos work | Always the directory |
| P-49 | Servers get the full text, and only just before a request. A reply to a changed buffer is refused | The simplest correct sync | Incremental or debounced sync |
| P-50 | Requests time out after 30 seconds, 60 to start. Three timeouts in a row or three crashes stop the server until restart | A stuck or crashing server can't loop | Backoff over time |
| P-51 | Rename needs a saved file, and is refused whole if it would create, rename or delete files | It shares the replace review (P-34), and half a rename is worse than none | Applying to unsaved buffers |
| P-52 | On Windows only the server itself is killed, so a helper it started can stay behind | A job object can't be tested here | A job object |
| P-53 | Onboarding shows while `config.toml` doesn't exist, and skipping saves the defaults | No marker file is needed, and a skipped screen stays skipped | A separate "onboarded" flag in state |
| P-54 | An install runs in a terminal tab, from the home folder, through a shell | Output and failures stay readable. From a project, a `go.mod` can choose the Go toolchain and an `.npmrc` the registry, before that folder is trusted. The commands are fixed strings, and a shell finds `npm.cmd` on Windows | A hidden background install |
| P-55 | `trust.toml` also lists folders answered no, and `:untrust` adds to it. An unreadable file doesn't ask on opening | Otherwise no would mean asking on every start, and a broken file couldn't save the answer. Anything that runs still asks | Asking on every start |
| P-56 | Release assets are raw binaries named `lazycode_<os>_<arch>`, with no version, plus a `sha256sum`-format `checksums.txt`. The icon scripts are assets too, and verified the same way | The installers and `:update` share one simple rule, and the icon logic stays in `packaging/` | Archives, or the icon logic inlined in each installer |
| P-57 | The last check's time is in the cache folder, written before asking, so a failed check counts | A bad connection doesn't mean a try at every start | Retrying on the next start |
| P-58 | The tag must be exactly `vX.Y.Z` before it's used in a link, redirects must stay on HTTPS, and downloads are size-capped. A Windows update moves the old file to `.old`, removed at the next start | The tag is remote input, and a running Windows program can be renamed but not overwritten | Trusting the API's answer |
| P-59 | Linux releases build on the oldest Ubuntu runner, `ubuntu-22.04` | It sets the oldest glibc the binary can run on | Static linking, not tried with the C parser |
| P-60 | A tag makes a draft release, and the owner publishes it by hand | Doc 15's manual checks come first, and a draft isn't "latest", so nothing reaches users early | Publishing automatically |
| P-61 | Backgrounds are painted in one pass at the end of each frame, by region, over cells still on default colours | Drawing code stays unaware of regions, and terminal tabs match the theme | An explicit background in every style |
| P-62 | The default icon set uses plain Unicode symbols, and Nerd Font icons are a setting | A terminal app can't bring a font, and missing glyphs show as boxes | Nerd Font icons by default |
| P-63 | Language packs colour with regular expression rules, and more grammars are built in | Packs stay readable for review, and linear-time matching can't hang | Compiled grammars per OS, or WASM |
| P-64 | No trust prompt, and deleting from the file tree needs no question. A delete moves files to a per-run trash in the cache folder, and Ctrl+Z in the tree restores them until quit | The owner found both questions in the way, and an undo protects files without interrupting | Keeping the prompts |

## Replaced decisions

| Old | New | Why |
|---|---|---|
| "Its own terminal" (Q-08) | Runs in the user's default terminal (Q-50) | The owner clarified that this meant the app icon only |
| One tab bar per project (assumed, Q-15) | Tabs per directory (Q-62) | The owner's correction |
| Deleted directories removed automatically (Q-40) | Marked, and the user decides (Q-66) | Unplugged drives look deleted |
| Sessions stay in memory until close (Q-34) | Idle sessions sleep (Q-58) | Saves RAM |
| Seven package formats (Q-47) | Curl and PowerShell (Q-71, Q-78) | Simpler, like OpenCode and Claude's installers |
| Language support after the first version (Q-49) | Highlighting in the first version (Q-51) | A code editor needs it |
| AI sessions after the first version (Q-49) | In the first version (Q-51) | They're what makes LazyCode different |
| Memory: as light as Zed (Q-42) | Under 300 MB (Q-70) | The owner's clear number |
| Ctrl+Shift+D, Ctrl+Shift+P, Ctrl+Shift+Space, Ctrl+Tab (Q-22, Q-54, Q-55) | Alt keys and Ctrl+\\ (Q-74) | They clash with common terminals |
| Alt+Left and Alt+Right for tabs (Q-74) | Alt+, and Alt+. (Q-80) | Windows Terminal uses them to move between panes |
| Alt keys on macOS, with a settings tip (Q-74, proposed in Q-80) | Ctrl+K then the same key (Q-80) | They type special characters by default |
| AI review with changed files and diffs (proposed) | Modified lines only (Q-76) | The owner only wants to see which lines changed |

## Corrections

| Date | Correction |
|---|---|
| 2026-11-03 | The first feasibility version estimated 5–30 MB per terminal tab. Measurement showed up to 185 MB with a 10,000-line scrollback, in the tested library. Default scrollback lowered to 1,000 as a result. |
| 2026-11-04 | The docs assumed OpenCode accepts any session ID, and that stopping a session frees its memory. It needs IDs starting with `ses`, and its background service stays. See [10](10-harness-integration.md). |
| 2026-11-03 | Roadmap estimates for M1 and M2 corrected to 11–17 and 13–20 weeks. They've since changed again, with the new first-version scope. |
