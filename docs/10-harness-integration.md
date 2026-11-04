# AI Harness Integration

**Status:** Draft, updated up to Q-79. In the first version (Q-51). **Decided** comes from your answers, and **Proposed** is for you to review.

## Principles

**Decided:**

- LazyCode doesn't implement any AI. It runs the tools' own TUIs in a terminal tab (Q-33).
- It lists only sessions started from LazyCode, plus plain shells (Q-32, Q-60).
- Working sessions keep running when switching, and stop when the project or window closes (Q-34).
- Idle sessions sleep to free their RAM. Clicking one reopens that exact session and chat (Q-58).
- Removing a session only hides it. LazyCode never deletes a tool's data (Q-60).

## Harness definition format

**Decided (Q-35, Q-59):** Defaults for tools that open instantly. Users add others with a name and command, plus optional resume fields.

```toml
[[harness]]
name = "Claude Code"
command = "claude"
start_args = ["--session-id", "{id}"]
resume_args = ["--resume", "{id}"]

[[harness]]
name = "OpenCode"
command = "opencode"
start_args = ["--session", "{id}"]
resume_args = ["--session", "{id}"]

[[harness]]
name = "Codex"
command = "codex"
# Resume fields not verified: Codex isn't installed on the test machine.

[[harness]]
name = "Any other tool"
command = "tool"
# No resume fields: clicking starts a new session, and it never sleeps automatically.
```

## What was verified

From the `--help` output on the owner's machine. The flags weren't actually run.

| Tool | Version | Set ID on start | Resume |
|---|---|---|---|
| Claude Code | 2.1.318 | `--session-id <uuid>` | `--resume <id>` |
| OpenCode | 2.2.0 | `--session <id>`: "continue, or create if it does not exist" | `--session <id>` |
| Codex | Not installed | Unknown | Unknown |

LazyCode choosing the ID is the key idea. It knows which session is which without reading the tool's internal files. OpenCode's IDs may need a particular format. Sleep depends on all of this, so it has a prototype test (`M0-08`).

## Launch flow

**Proposed:**

```text
Pick tool ─► Pick directory ─► Installed? ─ no ─► Message with the missing tool's name
                                   │ yes
                                Trusted? ─ no ─► Trust prompt (see 11)
                                   │ yes
           Create ID ─► Open PTY in directory ─► Start tool ─► Add tab and list entry
```

The tool gets the normal environment, with `TERM` set appropriately, plus the correct terminal size. The tool's command and arguments are passed directly, never as a shell string built from names.

## Session lifecycle

```text
                  start, or click to resume
                            │
                            ▼
    output again      ┌───────────┐     finished, or asking
   ┌────────────────► │  Running  │ ─────────────────────┐
   │                  │     ◐     │                      │
   │                  └─────┬─────┘                      ▼
┌──┴────────┐               │                     ┌───────────┐
│  Viewed   │ ◄─────────────┼──────── user views ─│ Attention │
└──┬────────┘               │                     │  green ●  │
   │                        │                     └─────┬─────┘
   │ idle rules met (Q-77)  │ project or window closes  │ idle rules met (Q-77)
   ▼                        ▼                           ▼
┌────────────────────────────────────────────────────────┐
│                     Sleeping ○                         │
│  Tool stopped, RAM freed. Click resumes the same chat. │
└────────────────────────────────────────────────────────┘
```

| State | Process | Memory | Display |
|---|---|---|---|
| Running | Alive | Used | `◐` |
| Attention | Alive | Used | Green `●`, plus an OS notification |
| Sleeping | None | Only a saved list entry | `○` |
| Exited | Ended on its own | Terminal contents until the tab closes | Exit message |

### Automatic sleep

**Decided (Q-58):** A session you worked with and left should sleep, so that it doesn't use RAM.

**This is a guess, and the costs of guessing wrong are real.** A tool can be silent while it's still working, such as during a long test run, and stopping it would interrupt that work. A tool waiting on a question may lose it.

**Decided (Q-77).** A session sleeps only when **all** of these hold:

1. No output for 30 minutes, which can be changed or turned off
2. Its tab isn't focused
3. It has resume fields, so that the chat can come back
4. Nothing it started is still running or using CPU

On Linux, the fourth check can use the process tree. macOS and Windows need their own versions.

**How it stops:** LazyCode asks the tool to exit normally, then forces it after a short wait. The terminal emulator and scrollback are freed. On resume, the tool draws its own screen and chat again.

Whether Q-34's "keep running when switching" conflicts with this: it doesn't, because a session that's actively working never meets rule 1.

### Closing a project or window

LazyCode warns if a session looks like it's still working. Each tool is then asked to stop, and forced after a short wait. Sessions show as Sleeping next time, and resume when clicked.

## Status detection

**Decided (Q-36, Q-72):** A green detail and an OS notification when a session finishes or asks for something. Detection is approximate.

**Proposed, in order of reliability:**

1. **Terminal signals:** A bell or an OSC 9/777 notification from the tool.
2. **Quiet period:** No output for a few seconds after activity.
3. **Tool hooks, later:** Some tools can run a command on events, which would be more exact. It would need per-tool setup, and must not change users' settings without asking.

Testing with each default tool is needed before promising more than this.

**OS notifications:** Sent directly by LazyCode, since not every terminal handles them. Not sent for the tab being viewed.

## Focus and key handling

- A focused terminal tab receives all keys, including Escape.
- **Reserved key (Q-74):** Ctrl+\\ always returns focus to LazyCode, and it's never sent to the tool.
- **Terminal keyboard modes:** Some tools ask for mouse or keyboard modes. The emulator must track them for each tab, and restore them when switching tabs.
- **Pasted text:** Forwarded using bracketed paste when the tool asks for it.

## Sessions and directories

A session's tab belongs to the directory it started in (Q-62). The sessions panel shows all of the project's sessions, in any directory. **Proposed:** Selecting one in another directory switches there first.

## Watching AI changes

Modified lines are covered in [06](06-components.md#watching-ai-changes) (Q-53, Q-76). It only reads files, and doesn't need anything from the tool.

## Failure handling

**Proposed:**

| Failure | Behaviour |
|---|---|
| Tool isn't installed | Mark it as unavailable in the list, with the name of the missing command |
| Exits at once | Keep the tab open, showing output and exit code |
| Directory is missing | Show a message, and keep the entry (Q-66) |
| Resume fails | Show the tool's error, and offer a new session |
| No resume fields | Start a new session (Q-59) |
| Slept by mistake | Click resumes it. Interrupted work may need restarting, which is why the rules are conservative. |
| LazyCode crashes | Child processes stop through process groups or Job Objects. Sessions show as Sleeping on next start. |
| Resize during heavy output | Resize the PTY, and redraw on the next frame |

## Compatibility

Each tool can change its flags. Defaults are data in the config, so users can fix them without a new release. Release tests should check defaults against current versions.
