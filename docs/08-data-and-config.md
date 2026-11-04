# Data Model and Configuration

**Status:** Draft. **Decided:** TOML files opened with `:config` (Q-26), and everything is stored locally (Q-38). File layouts are **Proposed**.

## Entities

```text
Project * ── * Directory           A directory can be in several projects (Q-66)
              │ 1
              ├── * Tab ── (file | terminal)    Tabs belong to the directory (Q-62)
              ├── * Session ── 1 Harness
              ├── 0..1 RunCommand
              └── 1 TrustState
```

| Entity | Fields |
|---|---|
| Project | id, name, directories, active directory |
| Directory | path, open tabs, cursors, layout |
| Harness | name, command, optional start and resume fields (Q-59) |
| Session | id, display name, harness, directory, tool session ID, hidden, status |
| Tab | type, file path or session ID, order |
| Run command | directory, command (Q-56) |
| Key binding | key, action, context |

## Config format and precedence

Settings are applied in this order, later winning:

1. Built-in defaults
2. `config.toml`
3. `keys.toml`
4. `projects.toml`, for project-specific values

No repository config file exists, since all project configuration is local (Q-38). This removes a whole class of security problems (see [11](11-security-and-privacy.md)).

### `config.toml`

```toml
version = 1

[editor]
auto_save = false       # Q-16
tab_size = 4

[ui]
theme = "dark"          # Q-48
sidebar = "left"        # Q-18

[terminal]
scrollback = 1000

[sessions]
sleep_after_minutes = 30   # Q-77. 0 turns it off.

[updates]
check = true               # Q-79. Set false to turn it off.

[[harness]]             # Q-35, Q-59. See 10-harness-integration.md
name = "Claude Code"
command = "claude"
start_args = ["--session-id", "{id}"]
resume_args = ["--resume", "{id}"]
```

### `keys.toml`

```toml
[keys]
"ctrl+b" = "panel.toggle_second"
"alt+p" = "run.play"     # Q-24, Q-74
```

### `projects.toml`

```toml
[[project]]
name = "Facebook"

[[project.directory]]
path = "~/code/frontend-facebook"

[[project.directory]]
path = "~/code/microservice-facebook-auth"
```

### `directories.toml`

Settings for each directory, wherever it's used, since a directory can be in several projects (Q-66). LazyCode writes this when it learns a run command (Q-56).

```toml
[[directory]]
path = "~/code/rust-service"
play = "cargo build"
```

## File locations

Found with Go's `os.UserConfigDir` and `os.UserCacheDir`.

| OS | Config | State and recovery |
|---|---|---|
| Linux | `~/.config/lazycode/` | `~/.cache/lazycode/` |
| macOS | `~/Library/Application Support/lazycode/` | `~/Library/Caches/lazycode/` |
| Windows | `%AppData%\lazycode\` | `%LocalAppData%\lazycode\` |

A cache folder can be cleared by the OS. Losing state must never break LazyCode: it only loses the saved tabs and sessions.

**Recovery files are not kept there**, because the OS could clear unsaved work. They go in `recovery/` in the config folder.

Plugins live in `plugins/` in the config folder.

## Saved state

`state/<project-id>.json`: not meant for hand editing, and written atomically. A corrupt file is ignored with a message.

```json
{
  "active_directory": "~/code/frontend-facebook",
  "directories": {
    "~/code/frontend-facebook": {
      "layout": "<saved splits and panels>",
      "tabs": [{"type": "file", "path": "src/app.ts", "cursor": [12, 4]}]
    }
  },
  "sessions": [{
    "name": "auth rewrite",
    "harness": "Claude Code",
    "directory": "~/code/microservice-facebook-auth",
    "tool_session_id": "<uuid>",
    "hidden": false
  }]
}
```

**Decided (Q-39, Q-62, Q-73):** Open tabs for each directory, sessions in use, cursor positions and layout. Terminal contents aren't saved, because a resumed tool draws its own screen.

## Crash recovery

**Decided (Q-61):** Unsaved edits survive a crash, and LazyCode asks before closing unsaved work.

**Proposed:** Edits are written to a recovery file about a second after typing stops, and are offered again on the next start. Files are deleted when the edit is saved or discarded. Recovery files are readable by the user only, since they can contain secrets.

Each file holds a one-line JSON header (path, process ID, time) followed by the full text. Files from a process that's still running aren't offered, so a second LazyCode window can't take another's work. Restoring leaves the text unsaved, with undo going back to what's on disk.

## Missing directories

**Decided (Q-66):** A missing directory is marked, and the user chooses whether to remove it. It's never removed silently, because an unplugged drive looks identical to a deleted folder.

## Config migrations

**Proposed:** Each file has a `version`. LazyCode upgrades older files, keeping a `.bak` copy. A file from a newer version isn't overwritten, and LazyCode warns instead.
