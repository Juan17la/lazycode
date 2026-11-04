# Release, Distribution and Governance

**Status:** Draft, updated up to Q-79. **Decided** comes from your answers, and **Proposed** is for you to review.

## Name and licence

- **Name (Q-04):** LazyCode for now, following lazygit, lazysql and lazydocker. It may change. Before the first release, check for existing projects with that name.
- **Licence (Q-05):** MIT.
- **Dependencies:** Must be compatible with MIT. Checked so far: `charmbracelet/x/vt` is MIT and `tcell` is Apache-2.0, which is compatible but requires keeping its notice. Others must be checked before use. Releases include a third-party licence file.

## Install channels

**Decided (Q-71, Q-78):** Curl install, like OpenCode and Claude's installers, plus a PowerShell one-liner on Windows, with binaries on GitHub Releases. Unsigned. Q-47's `.deb`, `.rpm`, `.dmg` and `.exe` are dropped for now.

| Method | OS |
|---|---|
| `curl -fsSL <url>/install.sh \| sh` | Linux, macOS |
| PowerShell one-liner | Windows, because curl install scripts don't normally run there |

**The install script (details Proposed):**

1. Detects the OS and CPU: Linux and macOS on amd64 and arm64, and Windows on amd64.
2. Downloads the binary from GitHub Releases, and verifies its checksum.
3. Places it in a user folder, and adds it to `PATH` if needed.
4. Creates the app icon (Q-50), and can be read before running.

**App icon (Q-50):** Searching for LazyCode in the system's apps opens the user's default terminal running `lazycode`.

| OS | Approach | Note |
|---|---|---|
| Linux | A `.desktop` file with `Terminal=true` | Uses the desktop's default terminal |
| macOS | A small `.app` that opens a terminal | macOS has no general "default terminal", so it uses Terminal.app unless set otherwise |
| Windows | A Start menu shortcut | Opens in the system's default terminal |

Not tested yet. That's prototype item `M0-06`.

**Builds:** The C parser (Q-69) means releases are built on real Linux, macOS and Windows CI runners. Tools such as GoReleaser may still help with packaging and checksums.

**Unsigned releases:** Downloads through curl generally avoid the warnings browser downloads get, but this needs testing, and a macOS icon may behave differently. See [11](11-security-and-privacy.md#unsigned-releases-and-updates).

## Versioning

**Proposed:** Semantic versions. Everything before 1.0 may change, except that user files keep migrating ([08](08-data-and-config.md#config-migrations)). From 1.0, the plugin API follows its `api_version`.

## Update method

**Decided (Q-71):** LazyCode checks GitHub's latest release, which excludes pre-releases and drafts.

**Decided (Q-79):** Check at most once a day at startup, and show only a message. `:update` downloads the new binary, verifies its checksum, and replaces the old one. A setting turns the check off, and the first-run screen discloses it, to stay consistent with Q-44.

## Release process

**Proposed:**

```text
Changelog updated ─► Tag vX.Y.Z ─► CI tests ─► Build binaries ─► Install tests
                                                                     │
                       Manual checks on real terminals and i3 ◄──────┘
                                     │
           Draft GitHub release with checksums and performance numbers ─► Publish
```

## Changelog

**Proposed:** `CHANGELOG.md` in the Keep a Changelog format, with a clear note for each behaviour or plugin API change.

## Contribution rules

**Proposed:**

Repository files, created when the code starts: `README.md`, `LICENSE`, `CONTRIBUTING.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md` and `CHANGELOG.md`.

- Pull requests need passing CI on all three operating systems.
- A performance regression needs a good reason.
- A new dependency needs an explanation.
- Editor core changes need tests.
- Commit style: Conventional Commits.
- **New contributors:** `good first issue` labels. Language packs are the easiest starting point.

## Governance

**Proposed:** At first the owner decides, using the decision log ([16](16-decisions.md)). Add maintainers as regular contributors appear, and particularly for store reviews, since one person can't sustain those.

## Issue and support process

**Proposed:**

- **GitHub Issues:** For bugs and requests, with templates asking for OS, terminal and version, plus a `:debuginfo` command to collect them.
- **GitHub Discussions:** For questions.
- **Priorities:** Data loss and security first, then crashes, then the rest.
- **Security:** Reported privately ([11](11-security-and-privacy.md#vulnerability-reporting)).
- This is a free-time project, so the README states there's no guaranteed response time.

## Documentation for users

TBD, written alongside the first release:

- Install guide
- Getting started, for developers at any level (Q-01)
- Coming from VS Code, and coming from Neovim
- Keymap and configuration reference
- Plugin author guide ([09](09-language-and-plugin-api.md))
- Troubleshooting
