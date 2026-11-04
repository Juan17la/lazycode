# Roadmap

**Status:** Draft, updated up to Q-79. The features in each milestone follow your answers, and the order is **Proposed**. **There's no deadline: this is a side project growing towards the strength of VS Code or Zed (Q-52).** Effort is estimated full-time weeks for one experienced developer ([04-feasibility.md](04-feasibility.md)).

Progress, start order and parallel tracks are tracked in [backlog.html](backlog.html).

## Overview

```text
M0 Prototype ─► M1 Editor ─► M2 Workspace and AI ─► M3 First release ─► M4 Languages and Git ─► M5 Plugins ─► M6 Store
```

## M0: Prototype

**Goal:** Test the riskiest technical questions before investing in the rest.

**Includes:**
- A project switcher, a file tree and a terminal tab
- A terminal tab running a real AI tool
- Running on Windows and macOS, not only compiling, including the C parser build
- The app icon opening the default terminal
- Testing that the chosen keys arrive in Windows Terminal, macOS Terminal and Kitty (Q-74, Q-75, Q-80)
- Testing that each tool can stop and resume its exact chat (Q-58)

**Exit criteria:** Terminal tabs work on all three OSes, every chosen key is confirmed or replaced, resume works, measurements are recorded in [12](12-performance-budget.md), and you find it useful for a week.

**Effort:** 3–4 weeks.

## First usable version

### M1: Editor

**Includes:** Buffer, undo, safe saving, recovery files, non-modal typing, multi-cursor, find and replace, tabs, splits, mouse clicks, themes, the command bar, keymap and config.

**Exit criteria:** You can edit real code all day without losing work. The 100 MB file target is met.

**Effort:** 11–17 weeks.

### M2: Workspace and AI

**Includes:** Projects, directory and project switching, per-directory tabs, restore, two panels, tree, search and replace, git history, push, pull and fetch, `gh` repository creation, terminal tabs, `:play`, custom shortcuts, syntax highlighting, AI sessions, sleep and resume, status and notifications, live file updates, and modified-line markers.

**Exit criteria:** The full Facebook example works, with 5 sessions. You can watch an AI change code, with the modified lines marked. Sessions sleep and resume the same chat. The editor stays under 300 MB.

**Effort:** 19–30 weeks. Several tracks can run in parallel with contributors.

### M3: First release (v0.1)

**Includes:** Onboarding, trust, the app icon, the install script, GitHub releases, update check, documentation.

**Exit criteria:**
- Installed by a developer who wasn't involved, on all three operating systems
- Every included feature meets the [definition of done](13-testing-strategy.md#definition-of-done)
- [Performance targets](12-performance-budget.md) are met on the i3 reference machine

**Effort:** 3–5 weeks.

## Later versions

### M4: Languages and Git

**Includes:** Language servers for definition, rename and format (3–6 weeks). Git diffs, stage, commit, branches and merge conflicts (4–6 weeks).

**Exit criteria:** All four languages work on a real project, and the normal Git workflow no longer needs a shell tab.

### M5: Plugins

**Includes:** Lua runtime, first API, local plugins with user consent, author guide.

**Exit criteria:** At least 3 useful plugins exist, with no editor changes needed for them.

**Effort:** 6–12 weeks.

### M6: Store

**Includes:** Store repository, review process, in-editor browse and install, contributor language packs.

**Exit criteria:** A contributor outside the project publishes a plugin successfully.

**Effort:** 3–5 weeks, plus ongoing reviews.

### Towards VS Code and Zed strength

Your long-term goal (Q-52) needs more than these milestones. Candidates, roughly in order of what people will expect:

1. Autocomplete and diagnostics (Q-28 made these later)
2. More languages, through the store
3. A changed-files list and diff view for AI work, if marked lines turn out not to be enough
4. Mouse dragging
5. Pull requests in LazyCode
6. Regex search
7. Debugger
8. Remote features

## Not planned

| Item | Source |
|---|---|
| Telemetry | Q-44 |
| Building an AI harness | Original brief |
| Vim-style modal editing as the default | Q-12 |
| Reading other tools' session history | Q-32 |
| Code signing, for now | Q-71 |

## Success measures

**Decided (Q-02, Q-52):** As strong and useful as VS Code or Zed. No deadline and no user-count target.

**Proposed checkpoints:** You use it daily instead of your current editor. Then, people outside the project use it. No telemetry means daily use can't be counted (Q-44). Public signs are available instead: release downloads, GitHub stars, issues, and plugin submissions.
