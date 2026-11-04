# Feasibility: Memory, Complexity and Realism

**Status:** Updated with all answers up to Q-79. **Measured** values come from throwaway tests on the owner's Linux machine on 2026-11-03, using test data and small programs rather than LazyCode. **Estimate** values are judgement.

## Verdict

| Question | Answer |
|---|---|
| Is it possible? | **Yes.** Your stack has a direct precedent: the micro editor uses Go, tcell and gopher-lua. |
| Is 300 MB realistic? | **Yes, based on the parts measured so far,** as long as scrollback and highlighting have limits. |
| How complex is it? | **High,** although your latest answers removed some work: no own window, and a single install method. |
| Is it realistic? | **Yes, as a long-term side project with no deadline** (Q-52). Reaching VS Code's breadth will take years, and it's the kind of goal that benefits from contributors. |

## Memory

**Decided (Q-70):** The editor itself stays under 300 MB. AI tools and other third-party apps aren't counted.

### Measured here

| Item | Memory |
|---|---|
| Go + tcell, full screen drawn | 5.9 MB RSS |
| Go + tcell + Lua | 7.8 MB RSS |
| Neovim, no config | 10 MB RSS |
| Each Claude Code session | About 325 MB PSS |
| VS Code, as open at the time | 1,405 MB PSS across 20 processes |

### Embedded terminal scrollback

Go vt library, 120 columns, buffer filled with 50,000 lines.

| Scrollback | RSS |
|---|---|
| 1,000 lines | 36 MB |
| 5,000 lines | 102 MB |
| 10,000 lines (library default) | 185 MB |

Setting scrollback to 0 also gave 185 MB, so 0 appears to mean "default", not "none". These are worst cases, since the buffer must be full to reach them.

### Syntax parsing

TypeScript test code, repetitive and synthetic, parsed once. RSS includes the whole test program.

| File size | Official C binding | Pure Go |
|---|---|---|
| 100 KB | 30 ms, 13 MB | 62 ms, 54 MB |
| 1 MB | 260 ms, 75 MB | 632 ms, 204 MB |
| 10 MB | 3.0 s, 679 MB | 6.4 s, 1,592 MB |

**Choice under Q-69: the official C binding.** It's two to three times lighter and faster, which fits the 300 MB limit with a cap: normal source files cost little, and files over about 1 MB don't get highlighting. The binary was 5.6 MB with the TypeScript grammar.

The cost is a C compiler in the build. Building for Windows from Linux failed here without a Windows toolchain, so releases need CI runners on each system. It hasn't been built on Windows or macOS yet. The regex alternative wasn't measured, because this result already fits.

### LazyCode's own process (estimate)

| Situation | Memory |
|---|---|
| Empty, panels only | 10–25 MB |
| Typical project: 4 directories, 10 tabs | 50–150 MB |
| Each awake terminal tab, scrollback 1,000 | Up to 36 MB |
| Sleeping session | Nearly nothing |

With 5 awake terminal tabs at their worst, the total could reach about 330 MB, slightly over the limit. Automatic sleep (Q-58) and a more compact scrollback store are the ways to stay under it. This has to be measured in the prototype.

### The whole Facebook example on your 8 GB minimum machine

| Part | All 5 sessions awake | 1 awake, 4 asleep |
|---|---|---|
| LazyCode | 75–330 MB | 50–190 MB |
| AI sessions | About 1.6 GB | About 325 MB |
| 4 language servers (later version, estimate) | 0.8–4 GB | 0.8–4 GB |
| **Total** | **About 2.5–6 GB** | **About 1.2–4.5 GB** |

Automatic sleep makes a real difference on 8 GB. AI sessions are measured for Claude Code, and assume other tools are similar.

## Complexity by feature

Effort assumes one experienced developer, full time. It's an estimate, and doesn't count any speedup from AI assistance.

| Feature | Difficulty | Weeks | Main difficulty |
|---|---|---|---|
| Editor core, multi-cursor, splits, find and replace | High | 8–12 | No ready-made Go text buffer, and bugs can lose work. |
| Layout, tabs, tree, two panels | Medium | 3–4 | Focus, resizing, and swapping tabs per directory. |
| Command bar and rebindable keys | Low–medium | 1–2 | Key differences across terminals. |
| Syntax highlighting | Medium | 2–4 | C build on three systems. |
| Language servers: definition, rename, format | High | 3–6 | Each server's quirks. Now in a later version. |
| Git panel: history view and sync buttons | Medium | 3–5 | Drawing the history graph. |
| Create repository through `gh` | Low | 1 | |
| Later Git: diffs, stage, commit, branches, conflicts | Medium | 4–6 | New from Q-63. Needs its own diff view. |
| Directory search and replace | Low–medium | 1–2 | Safe bulk replace. |
| Embedded terminal, Windows included | High | 4–6 | Nested TUIs, ConPTY, key and mouse forwarding. |
| AI sessions: start, list, rename, shells | Low | 1 | |
| AI sessions: automatic sleep, resume, status, notifications | Medium | 2–3 | Guessing idle safely. |
| Live file updates and modified-line markers | Medium | 1–2 | New from Q-53. |
| Projects, switching, restore | Low–medium | 2–3 | Now swaps tabs per directory. |
| `:play` and custom shortcuts | Low–medium | 1–2 | |
| Onboarding and trust | Low–medium | 1–2 | |
| App icon launcher | Low | 1 | Each OS works differently. |
| Install script, GitHub releases, update check | Low–medium | 1–2 | Was 3–5 for seven formats. |
| Lua plugin system | Very high | 6–12 | A stable public API is a long-term commitment. |
| Store | Medium | 3–5 | Plus ongoing plugin reviews. |
| **Total** | | **49–81** | |

### How long that takes

| Scope | Full time | At 10 hours a week |
|---|---|---|
| First version: Q-49, plus highlighting, AI sessions and modified-line markers | 33–52 weeks | About 2.5–4 years |
| Everything | 49–81 weeks | About 3.8–6.2 years |

There's no deadline (Q-52), so these only show the scale. 10 hours a week is an example. Contributors and AI assistance can shorten this, but I can't say by how much. Later polish isn't included. Reaching VS Code's full breadth is well beyond these tables, since they don't include autocomplete, diagnostics or a debugger.

## Effects of your latest decisions

| Decision | Effect |
|---|---|
| Runs in the user's terminal (Q-50) | Saves building a window, and works like other lazy apps. But the terminal receives keys first, so defaults avoid known clashes (Q-74, Q-80). |
| Tabs per directory (Q-62) | A simple mental model, like VS Code. Each directory needs its own saved tab state. |
| Automatic sleep (Q-58) | Large RAM saving on 8 GB. Risk of stopping a silent tool that's still working, which is why it has four safeguards (Q-77). |
| Modified-line markers (Q-53, Q-76) | Adds 1–2 weeks. Small, because you only want the lines shown. |
| Curl and PowerShell install, unsigned (Q-71, Q-78) | Much less packaging. Files downloaded by curl generally avoid browser download warnings, but this needs testing on macOS and Windows. Unsigned releases mean users rely on GitHub alone for authenticity. |
| Local plugins can do anything (Q-67) | Simple, but a local plugin has the same power as the user. User consent is the only protection. |
| Git commit comes later (Q-63) | The first version can push but not commit from the UI. Commits happen in a shell tab or through an AI tool. |

## Competitive reality

- **Hard to beat:** VS Code's extensions, debuggers and remote development; Neovim's plugin community; Zed's graphical speed.
- **Where this idea can win:** One light workspace with several repositories and many AI sessions, where you can watch the AI's changes appear as they happen.
- **Competitors can react:** AI panels are moving quickly in existing editors, so check this again as the project progresses.

## Top risks

| Risk | Impact | Mitigation |
|---|---|---|
| Scope far exceeds free time | Project stalls before anyone can use it | Daily-use milestone first, and M0 prototype |
| Editor bugs lose work | Trust is lost | Atomic saves, fuzz tests, recovery files |
| Default keys don't reach LazyCode | Poor first impression | Key test in M0, command fallbacks |
| Automatic sleep interrupts real work | Lost work or trust | Conservative rules, and a setting to turn it off |
| Resume doesn't restore the exact chat | Sleep loses its value | Test with each tool in M0 |
| Memory limit missed through scrollback | Core promise fails | Measure in M0, 1,000-line default |
| Young dependencies (vt v0.1.0) | Bugs and API changes | Wrap behind one package, and keep a replacement option |
| Windows differences found late | Large rework | Test Windows in CI from the start |
| Plugin API released too early | Permanent maintenance | Small API with a version number |
| Solo maintainer overloaded by reviews | Store stalls | Launch the store only after the core is stable |

## Sources

- [micro's go.mod](https://raw.githubusercontent.com/micro-editor/micro/master/go.mod): confirms Go, tcell and gopher-lua.
- [gotreesitter](https://pkg.go.dev/github.com/odvcencio/gotreesitter@v0.5.2): the pure-Go parser.
- [Windows Terminal actions](https://learn.microsoft.com/en-us/windows/terminal/customize-settings/actions): default shortcuts.
- [GitHub releases API](https://docs.github.com/en/rest/releases/releases): "latest" excludes pre-releases and drafts.
- [macOS keyboard shortcuts](https://support.apple.com/en-us/102650): Ctrl+F2 is a system shortcut.
- [Devin's terminal compatibility notes](https://docs.devin.ai/cli/reference/terminal-compatibility): macOS Terminal needs Option as Meta for Alt shortcuts. A secondary source, not tested here.
- Kitty 0.41.1's default shortcuts were read from its installed source on the owner's machine.
