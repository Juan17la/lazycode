# Security and Privacy

**Status:** Draft. **Decided** comes from your answers, and **Proposed** is for you to review. A code editor runs other people's code, so this needs attention even though no server exists.

## Threat model

**Proposed:**

| Threat | Example | Defence |
|---|---|---|
| Malicious repository | A cloned repo makes a language server run its code | Not prevented: language servers start only on request, and nothing asks first (P-64) |
| Malicious plugin | A plugin reads SSH keys | Store review, pinned commits, and consent before adding (Q-67). There's no sandbox. |
| Tampered update | A bad release reaches users | Checksums, GitHub releases only. Releases are unsigned (Q-71). |
| Install command misuse | A language tool's install runs the wrong thing | Show the exact command, and run it only after confirmation (Q-68) |
| Plugin changes after review | Author pushes bad code | Store pins an exact commit and checksum |
| Malicious file contents | Escape sequences or bidirectional controls in a file affect the terminal, or hide what code really does | The editor draws them as `?`, and tests check that none reach the screen |
| Malicious terminal output | Escape sequences set the clipboard or move text | Emulator filters them, never passes them to the user's terminal |
| Command injection | A file name containing `; rm -rf` | Pass argument lists, never build shell strings |
| Tampered download | Modified release | Published checksums, verified by the install script |
| Bulk changes go wrong | Replace or rename damages files | Show changes first, then write safely |

**Out of scope:** Protection against malware already running as the user, or against what an AI tool chooses to do. Those tools have their own permission systems, and LazyCode doesn't interfere.

## Trusting directories

**Decided (Q-45, replaced by P-64):** LazyCode doesn't ask. Running `:play`, an AI session, a language server or a Git operation in any folder just runs it, so opening a repository you don't trust and using those features can run its code. Language servers still start only when F12, F2 or format is pressed (P-48).

- Git operations can also run hooks. LazyCode's automatic background operations are read-only, and turn off the programs a repository can name: `core.fsmonitor`, `log.showSignature`, external diffs, text conversion, and filters through an empty attribute source on Git 2.40 or newer. A test proves `core.fsmonitor` doesn't run. `log.showSignature` is turned off from Git's documentation, without a test of its own, and the filter setting's effect couldn't be shown either way.

## Credentials and tokens

**Decided (Q-21, Q-46):**

- GitHub login stays with `gh`. LazyCode stores no GitHub token.
- If LazyCode ever needs to hold a secret, it follows VS Code and Zed by using the OS's secure store: Keychain on macOS, Credential Manager on Windows, and Secret Service on Linux.
- Secrets never go in TOML, state, logs or recovery files.

AI tools manage their own logins.

**Proposed:** Recovery files can contain unsaved secrets from edited files, so they're created readable by the user only.

## Plugin permissions

**Decided (Q-31, Q-67):** Store review is the safeguard for store plugins. Before any plugin is added, the user sees what it does and must accept. Local plugins can do anything. See [09](09-language-and-plugin-api.md#consent).

**A plugin has the same power as the user.** It can read SSH keys and run commands. Review reduces the risk but doesn't remove it, and an author's description can't be verified, so the consent prompt must say that plainly.

## Terminal escape sequence handling

**Proposed:**

- Output is interpreted by the emulator, and turned into cells. It never reaches the user's terminal unfiltered.
- Clipboard writes (OSC 52): Allowed in AI and run tabs only if a setting permits, and reads are never allowed.
- Hyperlinks: The full URL is shown before opening.
- Text shown in LazyCode's own interface (file names, Git messages) has control characters removed.

## Installing language servers

**Decided (Q-68):** LazyCode shows the exact command first, and runs it only after confirmation.

## Unsigned releases and updates

**Decided (Q-71):** Releases are unsigned. Updates come from GitHub's latest release, never pre-releases.

**The limit:** Checksums detect a corrupted download, but they come from the same place as the file. If the GitHub account or release process were compromised, users wouldn't be protected. That makes account protection essential: two-factor login, protected release tags, and releases built only in CI. Signing can be reconsidered as users grow.

## Telemetry and privacy

**Decided (Q-44):** No telemetry.

**Proposed rule:** LazyCode makes no network connection unless the user acts.

| Connection | Trigger |
|---|---|
| Plugin store | Opening the store |
| `git`, `gh` | User's operation |
| AI tools | Their own, and outside LazyCode's control |
| Update check | **The one automatic connection.** To GitHub's latest release, at most once a day at startup (Q-79). GitHub sees the IP address, and no other data is sent. A setting turns it off. |

`scripts/privacy-check.sh` lists what can connect or start a program. Only `internal/update`, and `internal/store` when the user runs `:store`, make a connection themselves. Without being asked, LazyCode also starts two local programs: `git`, for modified-line markers, and `ps`, for session activity on Linux and macOS.

Local logs are for debugging only. They contain no file contents, and rotate at a limited size. User counts can only come from public data such as release downloads.

## Vulnerability reporting

**Proposed:** A `SECURITY.md` asks for reports through GitHub's private vulnerability reporting, with the latest release as the supported version.

## Build and dependency safety

**Proposed:** Commit `go.sum`, run `govulncheck` in CI, and pin CI actions to exact commits. Releases are built only in CI, and publish checksums.
