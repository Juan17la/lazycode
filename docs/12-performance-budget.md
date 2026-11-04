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

**Chosen: the C binding, with highlighting off above about 1 MB (Q-69).** The first full parse of a 1 MB file takes 260 ms here and longer on an i3, so it must run in the background. Later edits are incremental, which wasn't measured.

### Budget check (estimate)

| Part | Worst normal case |
|---|---|
| Editor and open buffers | 50–150 MB |
| Syntax trees for visible files | Included above, unless files are near the 1 MB cap |
| 5 awake terminal tabs, full | Up to 180 MB |
| **Total** | **Up to 330 MB** |

That's slightly over the limit, so it needs work if it proves true: automatic sleep, a lower scrollback, or a more compact store. The prototype must measure this.

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

No target above has been tested against LazyCode, because LazyCode doesn't exist yet.
