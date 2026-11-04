# Open Questions

Write your decision after each **Answer:**. You can answer in any order, and "not sure, use the suggestion" is a valid answer.

★ marks a question that blocks architecture or scope, so answer these first.

Suggestions are only opinions. None has been applied to the documents.

## A. Vision and scope

**Q-01 ★ Who is it for?** Only you, an open-source community, or a commercial product?
- Suggestion: Build it for yourself first, in public if you like.
- **Answer:** For any developer from any level. Open source to everyone and modern for the latest reviews of AI sessions

**Q-02 ★ What does "fight VS Code, Neovim and Zed" mean to you?** What must be true in 12 months for this to be a success? For example: you use it daily, it has N users, or people switch from a specific editor. What is deliberately not a goal?
- Suggestion: "I replaced my editor with it daily, and multi-repo AI work is better than in any of the three."
- **Answer:** It has all the funcitonabilities, pluggins and compatibily with multiple languages for me, and N number of user to use daily for profesional coding, agentic coding or more things. Be as good as installing vsc

**Q-03 ★ How much time do you have?** Solo or team, and how many hours a week?
- **Answer:** This is a free time project, no time its set for the moment

**Q-04 Is LazyCode the final name?** It sits close to LazyVim and lazygit, so it may be confused with them. Your folder name is the only reason it is used so far.
- **Answer:** No, it could change but due it's based on other apps like lazygit, lazysql and lazydocker

**Q-05 What licence?** MIT or Apache-2.0 (permissive), GPL (derivatives stay open), or closed source?
- Suggestion: Decide alongside Q-01.
- **Answer:** MIT

**Q-06 Your list jumps from feature 7 to 9.** Was there a feature 8 that got lost?
- **Answer:** Yeah, it was the store for the pluggins and more languages support from contributors for different language or frameworks

**Q-49 ★ What is in the first usable version?** Rank features 1 to 10 as must-have, later, or never.
- Suggestion: Projects, directory switching, tree, tabs, AI sessions and custom shortcuts first. These are what make it different.
- **Answer:** f1,2,3,6,7,9,10

## B. Platform and technology

**Q-07 ★ Which operating systems at launch?** Linux, macOS, Windows?
- Suggestion: Linux and macOS first. Windows terminal handling adds significant work.
- **Answer:** windows, macos y linux

**Q-08 Which terminals must be supported?** Your current one is Warp. Others include Kitty, Alacritty, WezTerm, Ghostty, iTerm2, GNOME Terminal and Windows Terminal. This decides which shortcuts are safe.
- **Answer:** Should be its own terminal to not interfeer with other terminals shortcuts

**Q-09 ★ Which implementation language?** Rust, Go, Zig, C++, or something else? Do you already know one well? That matters as much as its technical strengths.
- Suggestion: Rust, unless you're much stronger in another language. It has the most relevant TUI, text-buffer, tree-sitter and terminal-emulator libraries.
- **Answer:** Go

**Q-10 Must it work over SSH on remote machines?**
- Suggestion: Yes, as a side effect of being a TUI, but with no special remote features.
- **Answer:** no remote feature for the moment

## C. Editing

**Q-11 ★ Write your own editor core, or embed an existing one?** This is the largest scope decision in the project.
- A: Write your own. Full control over the experience, but the most work.
- B: Embed Neovim or Helix inside LazyCode's panels and tabs. Far less work, but it needs that editor installed and limits how integrated it feels.
- C: Start with B to prove the idea, then replace it with A if the project works.
- Suggestion: C. See [04-feasibility.md](04-feasibility.md).
- **Answer:** start with A

**Q-12 ★ What editing style?** Normal typing like VS Code (non-modal), Vim-style modes, or both?
- Suggestion: Non-modal by default, since it matches your "not only commands" goal. Add Vim keys later if wanted.
- **Answer:** non-modal

**Q-13 How much mouse support?** None, clicks and scrolling, or also dragging panels and selecting text?
- **Answer:** click, scrolling and gap for a future the draggable

**Q-14 What is the largest file it must handle well?** For example 10 MB, 100 MB or 1 GB. What should happen with binary files?
- **Answer:** 100MB though flexible for in the future to be bigger

**Q-15 How do tabs behave?** Does each directory have its own tabs, or does the project share one tab bar? Are split views needed? When switching directory, do its tabs swap in? What happens when you open a file that's already open?
- **Answer:** the project it's like the ones of zed, yes, like in vsc and zed with draggable, like in vsc, it doesnt open and shows a message in the command bar

**Q-16 Saving and recovery.** Do you want auto-save? Should unsaved work survive a crash? What should happen when closing or switching with unsaved changes?
- **Answer:** auto-save option should be on the onboard and should be disable by default but could be added

**Q-17 Which editing features are required in the first version?** Multi-cursor, find and replace, format on save, snippets, code folding, or others?
- **Answer:** multicursor and find and replace for the moment. Those editing featuer that help coding fastr in vsc with ctrl+f2 or ctrl+D

## D. Panels

**Q-18 What should the layout look like?** One sidebar with switchable views, as in VS Code, or several panels at once? Left or right? Hidden until requested? What does "not overwhelmed" look like at startup?
- Suggestion: One sidebar with switchable views, plus the editor, and nothing else visible until requested.
- **Answer:** max two panels selectable to have every feature in the side the user want. by default all it's on the left sidebar, but there can be two dividing the views to ex.files, git and search and AI sessions

**Q-19 What should search cover?** File names, file contents, or both? The active directory or all directories in the project? Is replace needed? Regex?
- **Answer:** For active directory and both names and content. Yes replace needed. Regex, no for the moment.

**Q-20 How much git?** History only, or also diffs, staging, commit, push and pull, branches, and merge conflicts?
- Suggestion: History, diffs, stage, commit, push and pull. Leave complex operations to the CLI or lazygit.
- **Answer:** history (copy of vsc style view but in TUI style) and buttons to push full fetch and more. Also the option to create a repository from the code editor like in visual by login to github

**Q-21 ★ What should GitHub login enable?** Pull requests, issues, Actions status, or cloning repositories? Should it reuse an existing `gh` CLI login (already on your machine) or have its own login? Is GitLab or Bitbucket needed?
- Suggestion: Reuse `gh` at first, and start with pull request viewing.
- **Answer:** for the moment only gh to create remote repositories from the same code editor the github actions and prs be done in there

## E. Commands and shortcuts

**Q-22 How should quick command access work?** Command palette (like VS Code's Ctrl+Shift+P), leader key sequences (like Neovim's Space then keys), a `:` command line, or a combination?
- Suggestion: A palette for discovery, plus a leader key for speed.
- **Answer:** escape and : for the commands. Like :play excute code, :save  to save, :quit to quit and full names though the style of :q like in nvim should be also enable. Other shortcuts should be the typical of vsc like ctrl-tab to switch from tabs or ctrl+b to open the second panel. 

**Q-23 ★ Alt+Tab cannot be used for directory switching.** The desktop normally captures it before any terminal app sees it. Which shortcuts do you want instead?
- Suggestion: Alt+D for directories and Alt+Shift+P for projects, tested on your terminals. Note that Alt+P is already your run example.
- **Answer:** my bad should be basic ctrl + shift/alt or other combinations to not interrup other shortcuts

**Q-24 How do custom shortcuts work?** Per user, project or directory? Can one shortcut do different things in different directories (`npm run dev` in the frontend, `go build` in a service)? Can they trigger editor actions as well as shell commands? What about conflicts?
- Suggestion: Yes to per-directory. It's what makes Alt+P valuable in your multi-repo setup.
- **Answer:** custom shortcuts mean the user can changese the shortcuts basic and it's action in some cases for example for the build and runs it can be like a shortcut to not do :play

**Q-25 How does a run command behave?** Where does output appear (panel, tab, or terminal)? What happens if you press it while it's still running: restart, ignore, or ask? What happens to it when you switch project?
- **Answer:** opens a tab like a terminal, not the panel under, no, a tab as a teminal. But different from files tab to be recognizable and changing

**Q-26 How is configuration edited?** Config files only, or also in-app settings screens? Which format: TOML, JSON, YAML or Lua?
- Suggestion: A TOML file, with a command that opens it.
- **Answer:** TOML just basic config files and :config to open it

## F. Languages and plugins

**Q-27 Which languages must work out of the box?**
- **Answer:** Typescript, Javascript, python and Go 

**Q-28 Which language features are needed in the first version?** Syntax highlighting, autocomplete, go-to-definition, error diagnostics, rename, format, debugger? Using tree-sitter and LSP servers is the standard approach unless you prefer otherwise.
- Suggestion: Highlighting first, then LSP. Debugger support much later, if at all.
- **Answer:** Syntax Highlighting, go to definition, rename, format/ LSP and the rest of the functions should be for later

**Q-29 Who installs language servers?** The user, or LazyCode by automatic download? What should it show if one is missing?
- Suggestion: The user installs them, and LazyCode shows a clear hint.
- **Answer:** the user installs what he needs from the pluggins store or the on board

**Q-30 ★ What is a plugin?**
- A: Declarative only: config files adding languages, themes and keybindings.
- B: Code plugins in a sandbox (WASM, as Zed does).
- C: Code plugins in a scripting language (Lua, as Neovim does).
- D: External processes that talk to the editor over a protocol.
- Suggestion: A first, since it covers "easy language support" cheaply. Choose among B to D only after real demand appears.
- **Answer:** C

**Q-31 How are plugins distributed and secured?** Registry, Git URL or local folder? Can plugins read files or use the network, and do they need permission?
- **Answer:** Git, to get a pluggin on the story it should go throught the repo, for security, though a persona can make their own pluggins and dont put them on the repo and just use them locally, but they won't have place on the editor

## G. AI harnesses

**Q-32 ★ Which sessions appear in the panel?**
- A: Only sessions started from LazyCode. Simple and stable.
- B: Also existing sessions found in each tool's own storage. Much more useful, but depends on internal formats that can change.
- Suggestion: A first, then B for tools that offer a stable way to list sessions.
- **Answer:** only session from lazycode

**Q-33 ★ Where does the harness appear?**
- A: Embedded terminal inside LazyCode (in a tab or panel). Everything stays in one window, which matches your goal, but needs a terminal emulator inside the app.
- B: An external terminal window. Very simple, but it breaks "everything in one tab".
- C: tmux or Zellij panes. It's simple and sessions survive closing LazyCode, but it requires those tools.
- Also: which key returns focus from the harness to LazyCode? That key can never reach the harness.
- Suggestion: A, using an existing terminal-emulation library.
- **Answer:** in a tab it just another terminal  that stays open and sleeps when its doing nothing. when opnening back they are there not executing untilt he user clicks on them

**Q-34 What happens to a running session?** Does it keep running when you switch project or close LazyCode? Should there be a limit? Each Claude Code process used about 325 MB here, so this choice has a real memory cost.
- Suggestion: Keep running while switching, ask on quit, and reopen later using the tool's resume feature.
- **Answer:** they keep running until the user closes the project or the windows. if he swtich it keeps going, if he close the projects it stops

**Q-35 Which harnesses in the first version?** Can users add others with a simple config (command, resume flag)? What if a tool isn't installed?
- Suggestion: Config-defined from day one, so Claude Code, Codex and OpenCode are just default entries.
- **Answer:** add default (claude, codex, etc those that open instantly. And the option to add more by setting a name and a command like name:"claude code", "claude")

**Q-36 Should the panel show session status?** Running, waiting for your input, finished? Should it notify you? Detecting "waiting for input" reliably is hard from outside the tool.
- **Answer:** yes, it shoudl notify the user when a session finishes it's taks or ask for something through the notifications or a green detail 

**Q-37 Naming and cleanup.** Can sessions be renamed? Does removing one from the list delete it from the tool too? Should plain shell terminals be listed alongside AI sessions?
- Suggestion: Removing only hides it. Never delete another tool's data.
- **Answer:** yes they can be renamed

## H. Projects

**Q-38 Where are projects saved?** In your user config, or a file in a repository that your team can share?
- Suggestion: User config, since a project spans several repositories.
- **Answer:** in a config, all of the project its local and configuration

**Q-39 What is restored when you return to a project or directory?** Open tabs, cursor positions, layout, running sessions, running commands?
- **Answer:**open tabs, sessions that were being used, running commands never stop until the projects its closed or the window it's closed

**Q-40 Directory rules.** Can a directory belong to several projects? Must it be a git repository? What if it moves or is deleted? Can a project include an SSH path?
- **Answer:** if it it's deleted then the next time it should be removed from that direction and have to be added again, 

**Q-41 Can the tree show all of a project's directories at once?** Your example shows one active at a time. Should a combined view exist too?
- **Answer:** the options should be there, but it also the default it's the directory selected

## I. Performance

**Q-42 ★ What does "very light" mean in numbers?** Set targets for idle memory, startup time and typing latency. Does the memory target include language servers and AI tools? They run as separate processes and will dominate total usage.
- Suggestion: Measure LazyCode's own process only. Under 50 MB idle, under 150 MB on a typical project, startup under 100 ms and typing latency under 16 ms. Report the total for external tools separately.
- **Answer:** The objective it's to keep it as cheap in ram as zed (lower even), as quick as zed as well, and about disk space doesn't matter, but try to keep it under the space vsc uses in the worse case

**Q-43 What is the weakest machine it must run well on?**
- **Answer:** an inteli3 with 8gb of ram and over 216gb

## J. Security, privacy and distribution

**Q-44 Should there be telemetry?** None, opt-in, or opt-out?
- Suggestion: None.
- **Answer:** no

**Q-45 Trusting cloned code.** If a repository's config can define shortcuts or language servers, opening it could run someone else's commands. Should LazyCode ask before trusting a repository's config?
- Suggestion: Yes, if project-level config exists at all (see Q-24).
- **Answer:** yes like in other code editor 

**Q-46 Where are credentials stored?** The OS keychain, or only those managed by `gh`? Never in plain config files.
- **Answer:** like vsc and zed does it

**Q-47 How will it be installed and updated?** Single binary download, Homebrew, apt, cargo or others? Automatic updates?
- **Answer:** .deb, .rpm, tar.gz, portable version windows, .exe, curl, unviersal.dmg 

**Q-48 Interface language, themes and accessibility.** English only? Which themes? Do you need colour-blind-safe themes, or low-colour terminal support?
- **Answer:** English only, dark them and light theme, color decorations for borders and texts, lowe colour terminal support, just the necesary colors, something like claude code style,accent color and primary color

## K. Follow-up questions

These came from your answers: some answers conflict, some were partly answered, and some raised a new decision.

**Q-50 ★ What does "its own terminal" mean? (from Q-08)**
- A: LazyCode opens in its own window, using a bundled existing terminal (Alacritty or WezTerm) set up with no shortcuts of its own. It solves key conflicts with little work and matches your `.dmg` and `.exe` packages. Adds the bundled terminal's memory and disk use.
- B: LazyCode draws its own window, written in Go. Most control, but a large extra project (6–12 weeks), and the C libraries it needs complicate the build.
- C: LazyCode runs in the user's existing terminal, and only its AI and run tabs are "its own". Lightest, and works anywhere, but Ctrl+Tab and some Ctrl+Shift keys won't work in every terminal.
- Suggestion: A, while still allowing C for people who prefer their own terminal.
- **Answer:** My mistake, it was only if the user search for the icon app, it opens his default terminal with the command of lazycode like any other lazy app

**Q-51 ★ Do language support and AI sessions really wait? (from Q-49)** Your first version excludes F4 (languages) and F5 (AI sessions). But Q-28 lists language features for "the first version", and AI sessions are what make this project different. Without F4, the first version has no syntax highlighting.
- Suggestion: Add highlighting and AI sessions to the first version. Language servers follow next.
- **Answer:** Okay i agree I lost myself in the questions. that's fine, follow the suggestion

**Q-52 ★ Which goal comes first? (from Q-02, Q-03)** "Everything, as good as VS Code" takes about 45–76 full-time weeks for one developer, or about 3.5–6 years at 10 hours a week ([04-feasibility.md](04-feasibility.md)). What is your value of N users? Is there a date? If you want results sooner, what would you cut, or would you look for contributors?
- Suggestion: No date. Aim first for "I use it daily", then the first 100 users.
- **Answer:** i mean it like, we don't have to get it ready for a certain date, it's a side project of mine I will be developing to get as strong and useful as vsc or zed

**Q-53 What did you mean by "modern for the latest reviews of AI sessions"? (from Q-01)** A way to review what AI sessions changed? Support for the newest tools? Something else?
- **Answer:** I mean, that I should feel modern for the new AI age. Like useful to code by hand and incredible to see the code changing while the AI makes the code to review it or check on it

**Q-54 ★ Which key leaves a terminal tab? (from Q-22, Q-33)** AI tools need Escape, so Escape then `:` can't work inside their tabs. One key must be reserved for LazyCode, and it will never reach the tool.
- Suggestion: Ctrl+Shift+Space moves focus out of the terminal, after which Escape and `:` work as normal.
- **Answer:** I agree with the suggestion

**Q-55 ★ Which exact keys switch directory and project? (from Q-23)** Avoid Ctrl+Alt+letter: on many Windows keyboards, including Spanish ones, it equals AltGr and types characters like `@`.
- Suggestion: Ctrl+Shift+D for directories and Ctrl+Shift+P for projects. Both can be changed.
- **Answer:** the suggestion looks good for me

**Q-56 What does `:play` run? (from Q-22, Q-24)**
- A: A command you set for each directory, stored in the project config.
- B: LazyCode detects it (`package.json`, `go.mod`, `main.py`).
- C: B proposes a command the first time, you confirm it, and it's saved as in A.
- Suggestion: C. It also means nothing runs without confirmation.
- **Answer:** It should detect the project the basic one for the moment (typescript, python, javascript, go ) if the /play can detects it then aske the user for the command to run so it stay like that for that directory. Ex. input="cargo build" for rust project

**Q-57 What happens if you press `:play` while it's already running? (from Q-25)** Restart it, go to its tab, or ask?
- Suggestion: Go to its tab and offer restart.
- **Answer:**go to its tab

**Q-58 What does "sleep" mean for a session? (from Q-33, Q-34)** Q-33 says sessions sleep when idle, and Q-34 says they keep running until the project closes. The docs currently combine them: running sessions stay in memory while the project is open, and a closed project's sessions show as sleeping until clicked. An idle process uses no CPU, but it keeps its RAM (about 325 MB per Claude Code session here). Do you also want idle sessions stopped automatically to free RAM?
- Suggestion: No automatic stop by default, because stopping a tool can lose its in-progress state. Offer it as an option later.
- **Answer:** like when you worked with a claude session adn left it there, it should sleeps and when click on it back again should open that exact session and chat, so that it doesnt consumes the ram 

**Q-59 Should a tool's config include resume? (from Q-35)** Name and command alone can only start a new conversation. So clicking a sleeping session wouldn't restore it. Optional extra fields fix this. Claude Code and OpenCode's help text show flags for it ([10-harness-integration.md](10-harness-integration.md)). Without them, should clicking simply start a new session?
- Suggestion: Add the optional fields, and start a new session if a tool has none.
- **Answer:** i agree with the suggestion

**Q-60 What about removing sessions and plain shells? (from Q-37)** Does removing a session from the list delete the tool's own data? Should plain shell terminals appear in the list?
- Suggestion: Removing only hides it, and LazyCode never deletes another tool's data. Yes to plain shells.
- **Answer:** agree with the suggestion

**Q-61 What about crashes and unsaved work? (from Q-16)** Should unsaved edits survive a crash? What should happen when closing a file, project or window with unsaved changes?
- Suggestion: Yes to recovery files, and ask before closing.
- **Answer:** yes to recovery files and ask before closing

**Q-62 Please confirm how tabs work. (from Q-15)** The docs assume one tab bar per project, as in a Zed workspace. Switching directory changes the tree, search and git views while tabs stay. Only switching project changes the tabs. Is that right? Also, Q-15 says draggable and Q-13 says dragging later. The docs assume later.
- **Answer:** no, the tabs also change like in vsc when you close a folder an opens another. its opening a new directory and every view changes to the directory selected

**Q-63 What else belongs in git? (from Q-20)** You listed history, push, pull, fetch "and more". Should it include diffs, stage, commit, branches and merge conflicts? Without commit, a new repository has nothing to push.
- Suggestion: Add diffs, stage, commit and switching branches. Merge conflicts later.
- **Answer:** yes all this options and feature should be added, but it future version, adding them in further updates

**Q-64 Where are pull requests and Actions handled? (from Q-21)** You wrote "be done in there". Does that mean on github.com, with LazyCode only creating repositories for now, or inside LazyCode?
- Suggestion: On github.com for now.
- **Answer:** On github.com for now

**Q-65 What about binary files and files over 100 MB? (from Q-14)** Refuse, open read-only, or show a hex view?
- Suggestion: Binary: message only. Over 100 MB: open read-only without highlighting.
- **Answer:** agree with suggestion

**Q-66 What are the rest of the directory rules? (from Q-40)** Can a directory belong to several projects? Can it be a non-Git folder? Automatic removal has a risk: an unplugged drive looks the same as a deleted folder.
- Suggestion: Yes to both. If the directory is missing, ask or mark it, instead of removing it silently.
- **Answer:** agree with suggestion

**Q-67 What can plugins access? (from Q-31)** Git review protects the store, but not local plugins. Should Lua plugins be able to read files, run commands or use the network? Does "won't have place on the editor" mean only that local plugins aren't listed in the store?
- Suggestion: Plugins declare the permissions they need and the user approves them. Local plugins work but aren't listed.
- **Answer:** I mean that pluggins store should only show the pluggins that are check as a contribution to the project. The local can do anything, but it should ask for permission before adding a pluggin and the accept of the user of what that pluggin does

**Q-68 How do language servers get installed? (from Q-29)** The store comes later, but onboarding needs language servers now. Some need other tools: `gopls` needs Go, and TypeScript's and Pyright need Node.
- A: Onboarding shows the exact command to copy.
- B: Onboarding runs the command after you confirm.
- C: LazyCode downloads them itself. Most work, and adds a security responsibility.
- Suggestion: B.
- **Answer:** follow suggestion

**Q-69 ★ Can the build require a C compiler for highlighting? (from Q-09, Q-28)** A pure-Go tree-sitter parser used 204 MB on a 1 MB file in testing, which misses your memory target. The official C-based version wasn't measured yet.
- A: Official tree-sitter through C (cgo). The standard option, but the build needs a C compiler on each OS.
- B: Simpler pure-Go highlighting based on regex rules, as in micro. Light and simple to build, but less accurate.
- C: Pure-Go tree-sitter on small files only.
- Suggestion: Measure A first, and choose between A and B using those numbers.
- **Answer:** follow sugestion

**Q-70 ★ What are the actual numbers for "as light and quick as Zed"? (from Q-42)** Zed wasn't running here, and published figures disagree widely (95 MB to 890 MB). Either measure it (commands in [12-performance-budget.md](12-performance-budget.md)) or let me open Zed on your machine to measure. Does the target include the bundled terminal from Q-50?
- Suggestion: Measure first, and use provisional targets until then: 50 MB idle, 150 MB on a typical project.
- **Answer:** The goal it's to be in the range under 300mb not counting session of claude or other third apps. Just the code editor itself should be under 300mb

**Q-71 Signing and updates. (from Q-47)** Without paid signing, macOS and Windows warn users about the app. An Apple developer account costs about $99 a year, and Windows certificates cost extra. Will you pay for it, or start unsigned? Should the app check for updates? Any check contacts the internet, even without telemetry.
- Suggestion: Start unsigned with clear instructions, and only a manual update check.
- **Answer:** I feel like we should only do it for curl, to make it like opencode, claude terminal user interfaces. For that do everything unsinged and for the updates check the release in github repository for the latest, no pre-release

**Q-72 Can status detection be approximate? (from Q-36)** A tool doesn't generally report "finished" or "waiting" to the terminal. LazyCode can use bells, tool notifications and quiet periods, but it will sometimes be wrong. Is that acceptable? Should it also use OS notifications, or only the green detail?
- Suggestion: Accept approximate status, and start with the green detail.
- **Answer:** the green detail and OS notification I think it's enough for the sessions

**Q-73 Should cursor position and layout be restored? (from Q-39)** You listed open tabs and sessions only.
- Suggestion: Yes to both, at very little cost.
- **Answer:** agree with suggestion

## L. Questions raised by your latest answers

All earlier questions are answered. These six are new, and only ★ ones block planned work now.

**Q-74 ★ Your shortcuts clash with common terminals. Which keys should be the defaults? (from Q-22, Q-50, Q-54, Q-55)** LazyCode now runs in the user's terminal, which receives keys first. Checked defaults: Windows Terminal uses Ctrl+Shift+Space, Ctrl+Shift+D and Ctrl+Tab. Kitty uses Ctrl+Tab, and Ctrl+Shift+P as a prefix. Terminals without the newer keyboard protocol also can't tell Ctrl+Shift+D from Ctrl+D, which is your multi-cursor key.
- A: Keep your chosen keys, and users on those terminals rebind them or change the terminal's settings.
- B: Use keys that get through more often: Ctrl+\ to leave a terminal tab, Alt+D for directories, Alt+O for projects, Alt+P to run (your original example), and Alt+Left and Alt+Right for tabs. `:dir` and `:project` always work as a backup. On macOS, Alt needs a terminal setting in some apps, so the commands matter there.
- Suggestion: B, confirmed by the key test in the prototype. Every key can still be changed.
- **Answer:** go B

**Q-75 Which terminals must work fully? (from Q-08, Q-50)** Your new Q-50 answer reopens this. Others would be "should work, not guaranteed".
- Suggestion: Windows Terminal, iTerm2, macOS Terminal, GNOME Terminal, Kitty, Alacritty, WezTerm, Ghostty and Warp.
- **Answer:** windows(prioritary), macos (prioritary), Kitty (prioritary), for other terminals it should work just fine enough

**Q-76 ★ How far should AI code review go? (from Q-53)** You want to watch the code change while the AI works, and review it.
- A: Open files update live, and changed lines are marked.
- B: A, plus a list of changed files with a diff view.
- C: B, plus accepting or rejecting each change.
- And what are changes compared against: the last Git commit, or the files when the session started? Two sessions in one directory can't be told apart, because only the files are visible to LazyCode.
- Suggestion: B, comparing against the last Git commit. C can come later. For a non-Git folder, show only that a file changed.
- **Answer:** When i meant to review was just watching the lines that were modified and look at them nothing special, just show which lines were modified

**Q-77 When should an idle session go to sleep? (from Q-58)** Sleeping stops the tool to free its RAM, and clicking resumes the same chat. LazyCode can only guess at "idle": a tool can be silent while still working, and a stopped tool may lose a question it was waiting on.
- Suggestion: After 30 minutes without output, and only if its tab isn't focused, nothing started by the tool is still busy, and the tool has resume fields. The time can be changed, or turned off.
- **Answer:**follow suggestion

**Q-78 Please confirm curl is the only install method. (from Q-47, Q-71)** Q-47 listed seven formats, while Q-71 says only curl, like OpenCode and Claude's installers. Windows doesn't normally run curl install scripts, so it would need a PowerShell one-liner instead.
- Suggestion: Curl on Linux and macOS and PowerShell on Windows, with binaries on GitHub Releases. Drop `.deb`, `.rpm`, `.dmg` and `.exe` for now. The installer also creates the app icon from Q-50.
- **Answer:** follow suggestion

**Q-79 How should the update check behave? (from Q-44, Q-71)** It contacts GitHub, which sees your IP address, although no telemetry is sent.
- Suggestion: Check at most once a day at startup, and only show a message. `:update` installs it. A setting turns the check off.
- **Answer:** agree suggestion

## M. One small follow-up on keys

**Q-80 Three small key fixes for your priority terminals. (from Q-74, Q-75)** You chose option B, but my suggestion missed three things I found when checking it against Windows, macOS and Kitty. None of these blocks the prototype, which tests the keys anyway.
- **Tabs:** Windows Terminal uses Alt+Left and Alt+Right to move between panes. Suggestion: use Ctrl+PageUp and Ctrl+PageDown instead, which are VS Code's own tab keys and aren't Windows Terminal or Kitty defaults.
- **Select all occurrences:** macOS uses Ctrl+F2 to focus the menu bar. Suggestion: keep Ctrl+F2 elsewhere, and add Alt+F2 and `:selectall` as alternatives.
- **Alt keys on macOS Terminal:** Alt types special characters unless "Use Option as Meta Key" is on. Suggestion: onboarding explains the setting, and `:dir`, `:project` and `:play` always work.
- **Answer:** I'd that the linux and windows could be the same. macos can change. Try to search for a combination that doesn't affects the terminal shortcuts
- **Result of that search (D-43):** Checked against Windows Terminal's and Kitty's documented shortcut lists, and Apple's lists for Terminal and macOS. Real terminals are still tested in `M0-07`.
  - **Tabs on Linux and Windows:** Alt+, and Alt+. replace the Ctrl+PageUp suggestion, which GNOME Terminal reportedly uses to switch its own tabs.
  - **Everything on macOS:** Ctrl+K, then the same key as Alt. Option types special characters by default, so no Alt key is safe there without changing a setting. Ctrl+K reaches the app, and VS Code users already know it as a first key. Alt also works if the Option setting is on.
  - **Select all occurrences:** Ctrl+F2 stays on Linux and Windows. macOS uses Ctrl+K then A. Alt+F2 is dropped, because Linux desktops commonly use it to open a run dialog. `:selectall` always works.
