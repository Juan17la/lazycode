# Performance Budget

**Status:** Draft. **Decided:** The editor itself stays under 300 MB, not counting AI tools and other third-party apps (Q-70). As quick as Zed, disk under VS Code in the worst case (Q-42), and it must run well on an i3 with 8 GB of RAM and a 216 GB disk (Q-43).

## Targets

| Metric | Target | Status |
|---|---|---|
| Editor RAM, in any normal use | **Under 300 MB** | **Decided** |
| Installed size, in the worst case | Under 1,020 MB (VS Code here) | **Decided** |
| Idle, no project | Under 50 MB | Proposed |
| Typical project: 4 directories, 10 tabs, no terminals | Under 150 MB | Proposed |
| Each terminal tab with full scrollback | Under 40 MB | Proposed |
| 100 MB file open (Q-14) | Under 100 MB extra, usable within 1 second | Proposed |
| Startup to usable | Under 100 ms | Proposed |
| Key press to screen | Under 16 ms, on the i3 | Proposed |
| Directory or project switch | Under 50 ms | Proposed |
| Download size | Under 50 MB | Proposed |

"As quick as Zed" has no number yet. The proposed speed targets stand in for it, and Zed can be measured on your machine for comparison at any point.

## What the targets include

| Counted | Not counted (Q-70) |
|---|---|
| LazyCode's own process, including terminal tabs and plugins | AI tools |
| | Language servers |
| | Commands run by the user |
| | The user's terminal app |

## Baselines

Measured on 2026-11-03, on the owner's Linux machine.

### Comparison

| Item | Memory | Disk |
|---|---|---|
| Go, tcell | 5.9 MB RSS | 3.7 MB binary |
| Go, tcell, Lua | 7.8 MB RSS | 4.9 MB binary |
| Neovim, no config | 10 MB RSS | 5 MB binary |
| Zed | Not measured | 327 MB |
| VS Code | 1,405 MB PSS, 20 processes, workload uncontrolled | 1,020 MB, plus 426 MB of extensions |
| Claude Code, each | About 325 MB PSS | |

### Terminal tab scrollback

`charmbracelet/x/vt` v0.1.0, 120 columns, full.

| Scrollback | RSS |
|---|---|
| 1,000 | 36 MB |
| 5,000 | 102 MB |
| 10,000 | 185 MB |
| 10,000 with `GOGC=50` | 143 MB |

**Default: 1,000 lines (proposed).** That's about 140 bytes per cell, so a more compact store could allow longer history.

### Highlighting

TypeScript, repetitive synthetic input, parsed once. RSS includes the test program.

| File | Official C binding v0.25.0 | `gotreesitter` v0.56.0 (pure Go) |
|---|---|---|
| 100 KB | 30 ms, 13 MB | 62 ms, 54 MB |
| 1 MB | 260 ms, 75 MB | 632 ms, 204 MB |
| 10 MB | 3.0 s, 679 MB | 6.4 s, 1,592 MB |

**Chosen: the C binding, with highlighting off above about 1 MB (Q-69).** The first full parse of a 1 MB file takes 260 ms here and longer on an i3, so it must run in the background. Later edits are incremental.

Measured with highlighting built (`E1`), on an Intel Core Ultra 5 125H with dense 1 MiB Go, in the background: first parse and query about 372 ms, and one key press about 142 ms, mostly the query over the whole tree. A frame of that file takes 1.30–1.38 ms, and of a 100 MB file 1.16–1.28 ms. RSS stays flat over repeated open and close. That parse tree is about 40 MB of C memory.

### Directory search

`BenchmarkSearch50000Files` (`B1`): 50,000 files of about 2 KB, warm cache, 18 logical cores: about 0.57 s outside Git, and 0.73 s in Git with every file untracked. This machine is not the i3 reference.

### Prototype

Measured on 2026-11-04, on the owner's Linux machine (`M0-09`), with `TestMemoryStaysUnderLimit`. Tabs are 120 columns with a full 1,000-line scrollback. The tools' own memory isn't counted (Q-70). **There's no editor yet.** CI fails if any step reaches 300 MB, or if over 100 MB stays after closing every tab.

| State | RSS | Peak |
|---|---|---|
| Idle, no tabs | 19 MB | 19 MB |
| 1 full tab | 57 MB | 57 MB |
| 5 full tabs | 191 MB | 191 MB |
| 10 full tabs | 236 MB | 239 MB |
| All tabs closed | 38 MB | 239 MB |

Before fixes, 10 tabs reached 362 MB, and closing them gave nothing back. A 250 MiB Go memory limit, clearing the closed tab's slot and returning memory on close fixed that. `GOGC=50` also helped (274 MB at 10 tabs), but the limit did better, and only applies when it's needed. Past about 15 full tabs, the limit makes the garbage collector work harder. That wasn't measured.

### Text buffer

Measured on 2026-11-04, on the owner's Linux machine (Intel Core Ultra 5 125H, not the i3), with the benchmarks in `internal/buffer` (`CORE-01`). The file is 100 MiB of 50-byte lines.

| Operation | Time | Memory allocated |
|---|---|---|
| Open, from disk (`CORE-02`) | 77 ms | 108 MB: the file itself, plus 3.5 MB, mostly temporary |
| Save (`CORE-02`) | 0.25 s, and it blocks the UI for now | 0.1 MB |
| Write a recovery file (`CORE-02`) | 0.15 s, on another goroutine | 0.1 MB |
| Build the buffer from bytes already in memory | 10 ms | 3.5 MB, mostly temporary |
| One edit, after 20,000 scattered edits | 9 µs | 5.9 KB, the upper limit on what one undo step keeps |
| Draw a whole frame at 120 by 50 (`CORE-03`) | 1.1 ms | 0.4 MB |
| Read 60 lines, after 20,000 scattered edits | 0.12 ms | 3.8 KB |
| Replace 100,000 places, as one undo step | 0.46 s | 574 MB, mostly temporary |
| Each key in the find bar, for a word that isn't there (`CORE-06`) | 62 ms, over the 16 ms budget | 1 MB |

These are outside the 100 MiB file:

| Operation | Time | Memory allocated |
|---|---|---|
| One key press with 10,000 cursors (`CORE-05`) | 84 ms, over the 16 ms budget | 64 MB, mostly temporary |
| Frame with the cursor at the end of a 1 MB line | 12 ms | 1 MB |

With track D's sessions, watching and a once-a-second status check, `TestMemoryStaysUnderLimit` measured 17 MB idle, 191 MB with 5 full tabs, 235 MB peak with 10, and 37 MB after closing them.

**Very long lines are a weak spot.** That 1 MB line is under budget on this machine, but will likely be over on the i3. It's quicker nearer the start of the line. The same line of non-ASCII text is several times slower, because only ASCII has a fast path.

**Work that scales with the file or the cursor count runs on the UI goroutine.** Normal files and normal cursor counts are far under the budget. Counting find matches in a 100 MiB file and typing with 10,000 cursors aren't. Counting in the background is the upgrade if that gets noticed.

**A 100 MiB file slightly misses the "under 100 MB extra" target,** because it's read into memory (P-15), and comes close to the 300 MB limit alongside 5 full terminal tabs: about 191 MB plus 108 MB. That combination hasn't been measured. Memory-mapping wouldn't fix it: on the same machine, scanning a mapped 100 MiB file for lines added 100 MB of RSS too.

Undo history has no limit yet. At the upper limit of 5.9 KB per step, 10,000 steps keep about 59 MB alive, and typed text joined into one step keeps far less.

### Budget check (estimate)

| Part | Worst normal case |
|---|---|
| Editor and open buffers | 50–150 MB |
| Syntax trees for visible files | Included above, unless files are near the 1 MB cap |
| 5 awake terminal tabs, full | 191 MB, measured, including the base |
| **Total** | **About 240–340 MB** |

The high end is over the limit, although the memory limit should hold total use down while there's memory to reclaim. The editor's real size decides this, so measure again after `CORE-02`. If it's too high: automatic sleep, a lower scrollback, or a more compact store.

### Startup and switching (2026-11-05)

Measured on the owner's machine (Core Ultra 5 125H), **not the i3**. `scripts/reference-check.sh` runs both, so the i3 result is still needed for `REL-3`.

| Measure | Result | Target |
|---|---|---|
| Startup to first screen, real binary, 1,000-file folder (`TestStartupTime`) | About 15 ms median, 19 ms worst | 100 ms |
| Project switch and next frame, 1,000 files (`BenchmarkSwitchProject`) | About 4 ms over 3 runs, once 24 ms in a short run | 50 ms |
| Linux amd64 binary, built as a release | 17.6 MB | 50 MB |

Pinning to 2 cores gave similar results, but that doesn't match the i3's slower cores. To check on the i3: one key press in a 1 MiB file took about 290 ms to re-highlight here, and typing with 10,000 cursors took about 100 ms a key.

## Benchmark scenarios

**Proposed:** Each one runs on a fixed test project.

1. Start, open nothing, and wait 5 seconds.
2. Open the 4-directory test project with 10 tabs per directory.
3. Open and scroll 100 KB, 10 MB and 100 MB files.
4. Type 1,000 characters into a 1 MB file, and record key-to-screen time.
5. Edit with 100 cursors.
6. Search a 50,000-file directory.
7. Stream 50,000 lines through a terminal tab.
8. Switch between 5 directories and 5 projects, 100 times, and check for memory growth.
9. Run 8 hours with 5 terminal tabs, some sleeping and resuming, and check for leaks.
10. Load 10 plugins, and measure the cost.
11. Have a script rewrite 100 open and watched files repeatedly, and record screen responsiveness.

## How to measure

**Own process, Linux:**

```bash
ps -o rss=,cmd= -p "$(pgrep -x lazycode)"
```

**Multi-process apps such as Zed and VS Code,** in MB. PSS counts shared memory fairly:

```bash
total=0; for p in $(pgrep -x zed-editor); do
  v=$(awk '/^Pss:/{print $2}' /proc/$p/smaps_rollup 2>/dev/null)
  total=$((total+${v:-0}))
done; echo "$((total/1024)) MB"
```

Wait 5 minutes after opening the project, with no extensions or language servers running.

**In Go:** `pprof` heap and CPU profiles, plus `go test -bench`.

**Key latency:** Recorded inside the app from input to drawn frame. Final figures also need an external tool such as Typometer, because the terminal adds its own delay.

## Reference machine

**Decided (Q-43):** i3, 8 GB of RAM, 216 GB of disk. Release checks must run on this class of machine, or a virtual machine limited to match it. It needs to be low-end hardware at least once per release, because a development machine hides problems.

## Regression checks in CI

**Proposed:**

- Benchmarks run on each pull request, on the same type of runner.
- Fail if memory rises over 300 MB in any scenario, or by over 10% from the last release.
- CI runners vary in speed, so memory checks are strict and timing checks are warnings.
- Every release publishes its numbers.

Only the terminal tab and idle targets above have been measured, on the prototype. The rest need the editor.
