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
