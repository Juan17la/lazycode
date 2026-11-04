# Testing Strategy

**Status:** Proposed. Your own editor core (Q-11) and three operating systems (Q-07) make these the two main areas of test effort.

## Test levels

| Level | What | Tool | When |
|---|---|---|---|
| Unit | Buffer, cursors, config, keymap, parsing | `go test` | Every push |
| Property and fuzz | Buffer and editing | Go's built-in fuzzing | Every push (short), nightly (long) |
| UI snapshot | Layout and rendering | tcell's simulation screen, golden files | Every push |
| Integration | Git, LSP, terminal, plugins | Real binaries or fake servers | Every push |
| End to end | Full app driven through a terminal | Test harness over a PTY | Every push on Linux, before release on all |
| Performance | See [12](12-performance-budget.md) | `go test -bench`, scripts | Pull requests |
| Manual | Real terminals and low-end hardware | Checklist | Before each release |

Use Go's standard testing first, and add tools only when they're needed.

## Editor core tests

Bugs here lose user work, so this gets the most testing.

- **Model test:** Random inserts and deletes are applied to both the piece table and a plain string, and the results must always match.
- **Undo:** Any sequence of edits, followed by undoing everything, returns the original text.
- **Multi-cursor:** Overlapping selections, cursors merging, and edits that change line counts.
- **Unicode:** Emoji, combining characters, right-to-left text and CJK width.
- **Line endings:** LF, CRLF and mixed files stay unchanged unless edited.
- **Saving:**
  - A failed write leaves the original file intact. Tests simulate a full disk and missing permissions.
  - Symlinks and file permissions are preserved.
  - Files changed on disk by an AI tool are detected, and never silently overwritten.
- **Large files:** 100 MB open, edit and save, with memory measured.

Regression rule: every bug fix adds a test that fails without the fix.

## TUI snapshot tests

Key sequences run against a simulated screen, and the result is compared with a saved text file. Covers layouts, focus, tabs, the two-panel configuration, small window sizes, dark, light and low-colour modes.

## Embedded terminal tests

- **Fake tool:** A small test program prints known sequences. Includes colours, alternate screen, mouse modes, bracketed paste and resize.
- **Key forwarding:** The tool receives exact bytes, and the reserved key never gets through.
- **Real tools before release:** Claude Code, Codex, OpenCode, `vim` and `htop` display correctly.
- **Processes:** Closing a project ends all its children. No orphan processes remain after a forced crash.
- **Fuzz:** Random output can't crash the emulator wrapper.

## Compatibility matrix

| | Linux | macOS | Windows |
|---|---|---|---|
| Unit and fuzz | CI | CI | CI |
| Terminal with PTY or ConPTY | CI | CI | CI |
| C parser builds | CI | CI | CI |
| Install script and update | CI | CI | CI |
| App icon opens a terminal | CI, xterm | CI, Terminal.app | CI, starts only |
| Keys arrive in priority terminals (Q-74, Q-75) | Kitty, CI | macOS Terminal, CI | Windows Terminal, CI |

**Priority terminals (Q-75):** Windows Terminal, macOS Terminal and Kitty must work fully. Release checks cover these. Others, including your own Warp, only get occasional checks.

CI now runs the prototype on all three systems, in a real PTY or ConPTY with a stand-in tool, and parses all four languages with the C parser. A real AI tool has only run on Linux (`M0-03`). Priority terminals get simulated key presses on CI, and those for macOS and Windows only report, because they depend on what the runners allow. Physical keyboards are still a manual check.

## Sleep and live update tests

These are new, and failures here cost users real work.

- **Sleep rules:** A fake tool stays silent while a child process is busy, and must not be stopped. Each rule from Q-77 has a test.
- **Resume:** The real tools, before each release: the same chat returns after sleeping and after closing the project.
- **Live updates:** A script rewrites watched files quickly. The screen stays responsive, and clean buffers match the disk.
- **Unsaved edits:** A file changes on disk during unsaved edits, and the edits survive.
- **Markers:** Marked lines match a known diff, against the last commit and in a non-Git folder.

## Harness integration tests

- A fake tool with configurable start and resume flags tests the session lifecycle without any AI account.
- Before each release, check the default flags against current versions of the tools. Those tests run manually or nightly, because they depend on installed tools and logins.

## Other integration tests

- **Git:** Temporary repositories cover history, push and pull using a local bare remote, and errors.
- **`gh`:** A fake `gh` script in tests. The real one is only checked manually, so tests never create real repositories.
- **LSP:** A fake server for protocol tests, plus real `gopls` on a small project.
- **Lua:** Scripts cover errors, time limits and permissions.
- **Config:** Invalid files, old versions and missing files.
- **Security:** Names containing shell characters, and untrusted directories starting nothing.

## CI setup

GitHub Actions, on Linux, macOS and Windows runners.

```text
Pull request ─► gofmt, go vet, staticcheck, govulncheck
             ─► unit, fuzz (short), snapshot, integration, with -race
             ─► benchmarks compared with the last release

Nightly      ─► long fuzz, leak tests, real-tool checks

Tag          ─► all of the above, then build and test packages
```

## Definition of done

A feature is done when its tests pass on all three operating systems, it meets the performance budget, documentation is updated, and no crash or data-loss bug is open against it.
