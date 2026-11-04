# Requirements

**Status:** Updated from all answers in [03-open-questions.md](03-open-questions.md), up to Q-80. Each requirement is **Decided** unless it says `TBD` with a question number.

## 1. Vision

A very light code editor that runs in the terminal, between Visual Studio Code and Neovim: not purely command-driven like Neovim, and without a heavy interface like VS Code. Its style follows apps like lazygit, lazysql and lazydocker.

It should feel modern for the AI age: useful for coding by hand, and excellent for watching code change while an AI works, with the modified lines clearly shown.

## 2. Scope

| Item | Value | Source |
|---|---|---|
| Working name | LazyCode. May change. | Q-04 |
| Target users | Any developer at any level, for professional and agentic coding. | Q-01, Q-02 |
| Distribution model | Open source, MIT licence. | Q-01, Q-05 |
| Goal | A side project growing to be as strong and useful as VS Code or Zed. No deadline and no user-count target. | Q-02, Q-52 |
| Operating systems | Windows, macOS and Linux at launch. | Q-07 |
| Implementation language | Go. | Q-09 |
| Editor core | LazyCode's own, not embedded Neovim or Helix. | Q-11 |
| Editing style | Non-modal. | Q-12 |
| Where it runs | In the user's default terminal, like other lazy apps. Replaces the "own terminal" idea. | Q-08, Q-50 |
| Priority terminals | Windows Terminal, macOS Terminal and Kitty. Others should work well enough, without a guarantee. | Q-08, Q-75 |
| First version | F1, F2, F3, F6, F7, F9, F10, F11, syntax highlighting, AI sessions, and modified-line markers (F12). | Q-49, Q-51, Q-53 |

### Out of scope for now

| ID | Item | Source |
|---|---|---|
| OUT-1 | Building an AI harness. LazyCode only hosts the existing tools' TUIs. | Original brief |
| OUT-2 | Remote or SSH features. | Q-10 |
| OUT-3 | Regex search. | Q-19 |
| OUT-4 | GitLab, Bitbucket, or a LazyCode GitHub login separate from `gh`. Pull requests and Actions stay on github.com. | Q-21, Q-64 |
| OUT-5 | Listing sessions that were not started in LazyCode. | Q-32 |
| OUT-6 | Telemetry. | Q-44 |
| OUT-7 | Interface languages other than English. | Q-48 |
| OUT-8 | Autocomplete, diagnostics and debugger, for now. | Q-28 |
| OUT-9 | Dragging with the mouse, for now. The design must leave room for it. | Q-13, Q-15 |
| OUT-10 | Code signing. Releases are unsigned. | Q-71 |

## 3. Functional requirements

### F1. Panels (first version)

| ID | Requirement | Source |
|---|---|---|
| FR-1.1 | Directory tree panel like VS Code's. Shows the selected directory by default, with an option to show all of the project's directories. | Q-41 |
| FR-1.2 | Git history panel in the style of VS Code's history view, drawn for a TUI. | Q-20 |
| FR-1.3 | Buttons for push, pull and fetch. | Q-20, Q-63 |
| FR-1.4 | Create a GitHub repository from the editor, using `gh` for login. | Q-20, Q-21 |
| FR-1.5 | Search panel: file names and contents in the active directory, with replace. | Q-19 |
| FR-1.6 | At most two panels. Everything starts in one left sidebar. A second panel can take some views. Each can sit on the side the user wants. | Q-18 |
| FR-1.7 | Later: Git diffs, stage, commit, branches and merge conflicts. | Q-63 |

### F2. Tabs (first version)

| ID | Requirement | Source |
|---|---|---|
| FR-2.1 | Files open in tabs, like VS Code. | Original brief |
| FR-2.2 | Opening a file that's already open goes to the existing tab, and shows a message in the command bar. | Q-15 |
| FR-2.3 | Split views, like VS Code and Zed. Dragging comes later. | Q-13, Q-15 |
| FR-2.4 | Terminal tabs are visually different from file tabs, and show that their output is changing. | Q-25 |
| FR-2.5 | Each directory has its own tabs. Switching directory changes the tabs too, like closing one folder and opening another in VS Code. | Q-62 |

### F3. Commands and editing (first version)

| ID | Requirement | Source |
|---|---|---|
| FR-3.1 | Escape then `:` opens the command bar. Commands have full names (`:play`, `:save`, `:quit`) and short forms like Neovim's (`:q`). | Q-22 |
| FR-3.2 | Other shortcuts are the usual VS Code ones, where the terminal allows it. Terminal-safe keys replace clashes, listed in [07](07-ui-ux.md). Linux and Windows share keys, and macOS uses Ctrl+K in place of Alt (Q-80). | Q-22, Q-74, Q-80 |
| FR-3.3 | Normal non-modal typing. | Q-12 |
| FR-3.4 | Multi-cursor, including VS Code's Ctrl+D and Ctrl+F2. | Q-17 |
| FR-3.5 | Find and replace in a file. | Q-17 |
| FR-3.6 | Click and scroll with the mouse. | Q-13 |
| FR-3.7 | Auto-save is off by default, and offered during onboarding. | Q-16 |
| FR-3.8 | Unsaved edits survive a crash through recovery files. LazyCode asks before closing unsaved work. | Q-61 |

### F4. Language support

| ID | Requirement | Release | Source |
|---|---|---|---|
| FR-4.1 | TypeScript, JavaScript, Python and Go are supported out of the box. | First | Q-27 |
| FR-4.2 | Lua plugins. | Later | Q-30 |
| FR-4.3 | Syntax highlighting. | First | Q-28, Q-51 |
| FR-4.4 | Onboarding shows the command that installs a language tool, and runs it after the user confirms. | First | Q-29, Q-68 |
| FR-4.5 | Go-to-definition, rename and format, through language servers. | Next | Q-28, Q-51 |

### F5. AI harness support (first version)

| ID | Requirement | Source |
|---|---|---|
| FR-5.1 | Panel listing sessions started in LazyCode, the way files are listed. | Q-32 |
| FR-5.2 | Default entries for tools that open instantly, such as Claude Code, Codex and OpenCode. | Q-35 |
| FR-5.3 | Users can add a tool with a name and a command, plus optional resume fields. | Q-35, Q-59 |
| FR-5.4 | Each session is a terminal tab, opened in the directory the user selects. | Q-33 |
| FR-5.5 | Several sessions of the same tool are allowed in a project. | Original brief |
| FR-5.6 | Working sessions keep running when switching directory or project. They stop when the project or window closes. | Q-34 |
| FR-5.7 | Idle sessions go to sleep automatically, and free their RAM: after 30 minutes without output, only if the tab isn't focused, nothing the tool started is busy, and it has resume fields. The time can be changed or turned off. | Q-58, Q-77 |
| FR-5.8 | Clicking a sleeping session reopens that exact session and chat. | Q-33, Q-58 |
| FR-5.9 | A green detail and an OS notification appear when a session finishes or asks for something. Detection is approximate. | Q-36, Q-72 |
| FR-5.10 | Sessions can be renamed. | Q-37 |
| FR-5.11 | Removing a session only hides it. LazyCode never deletes a tool's data. | Q-60 |
| FR-5.12 | Plain shell terminals are listed alongside AI sessions. | Q-60 |
| FR-5.13 | A tool with no resume fields starts a new session on click. | Q-59 |

### F12. Watching AI changes (first version)

Added from your answers to Q-53 and Q-76. It only shows which lines were modified, and nothing more.

| ID | Requirement | Source |
|---|---|---|
| FR-12.1 | Open files update live while an AI changes them. | Q-53 |
| FR-12.2 | Modified lines are marked in open files. | Q-53, Q-76 |

### F6. Projects (first version)

| ID | Requirement | Source |
|---|---|---|
| FR-6.1 | Named projects saved in local configuration. | Q-38 |
| FR-6.2 | A project groups several code directories and its AI sessions. | Original brief |
| FR-6.3 | Selecting a directory changes every view to it: tabs, tree, search and git. | Q-62 |
| FR-6.4 | Returning restores open tabs, sessions in use, cursor positions and layout. | Q-39, Q-73 |
| FR-6.5 | Running commands continue until the project or window is closed. | Q-39 |
| FR-6.6 | A missing directory is marked, and the user is asked what to do. It isn't removed silently. | Q-40, Q-66 |
| FR-6.7 | A directory can belong to several projects, and doesn't need to be a Git repository. | Q-66 |

### F7. Quick directory switching (first version)

| ID | Requirement | Source |
|---|---|---|
| FR-7.1 | Switch directory with Alt+D, or Ctrl+K then D on macOS. `:dir` also works. | Q-74, Q-80 |

### F8. Plugin and language store (later)

| ID | Requirement | Source |
|---|---|---|
| FR-8.1 | A store offers plugins and more languages and frameworks, from contributors. | Q-06 |
| FR-8.2 | The store only shows plugins accepted through its Git repository. | Q-31, Q-67 |
| FR-8.3 | Local plugins work without being in the store, and can do anything. | Q-31, Q-67 |
| FR-8.4 | Before adding any plugin, LazyCode shows what it does and asks the user to accept. | Q-67 |

### F9. Quick project switching (first version)

| ID | Requirement | Source |
|---|---|---|
| FR-9.1 | Switch project with Alt+O, or Ctrl+K then O on macOS. `:project` also works. | Q-74, Q-80 |

### F10. Custom shortcuts and run commands (first version)

| ID | Requirement | Source |
|---|---|---|
| FR-10.1 | The user can change the basic shortcuts and, in some cases, their actions. | Q-24 |
| FR-10.2 | `:play` detects how to run TypeScript, JavaScript, Python and Go projects, and can be bound to a shortcut. | Q-22, Q-24, Q-56 |
| FR-10.3 | Output opens in a terminal tab, not in a bottom panel. | Q-25 |
| FR-10.4 | Pressing `:play` while it's running goes to its tab. | Q-57 |
| FR-10.5 | If nothing is detected, LazyCode asks for the command, and remembers it for that directory. | Q-56 |

### F11. Onboarding, launch and configuration (first version)

| ID | Requirement | Source |
|---|---|---|
| FR-11.1 | First-run onboarding, including the auto-save option and language tools. | Q-16, Q-29 |
| FR-11.2 | Configuration in basic TOML files, opened with `:config`. | Q-26 |
| FR-11.3 | Trust prompt, as in other editors. | Q-45 |
| FR-11.4 | The app icon opens the user's default terminal running `lazycode`. | Q-50 |

## 4. Non-functional requirements

| ID | Requirement | Target | Source |
|---|---|---|---|
| NFR-1 | RAM | The editor itself stays under 300 MB. AI tools and other third-party apps aren't counted. | Q-70 |
| NFR-2 | Speed | As quick as Zed. Proposed numbers are in [12](12-performance-budget.md). | Q-42 |
| NFR-3 | Disk | Under VS Code's size in the worst case. VS Code uses 1,020 MB on the owner's machine. | Q-42 |
| NFR-4 | Minimum hardware | Intel i3, 8 GB RAM, 216 GB disk. | Q-43 |
| NFR-5 | File size | Handle 100 MB files, with a design that can go higher. Larger files open read-only without highlighting. Binary files show a message only. | Q-14, Q-65 |
| NFR-6 | Not overwhelming | One left sidebar by default. | Q-18 |
| NFR-7 | Privacy | No telemetry. | Q-44 |
| NFR-8 | Credentials | Handled the way VS Code and Zed do: never in plain config files. GitHub login stays with `gh`. | Q-21, Q-46 |
| NFR-9 | Appearance | Dark and light themes, one primary and one accent colour, colour decorations on borders and text, a simple style similar to Claude Code's, and low-colour terminal support. | Q-48 |
| NFR-10 | Installation | Curl on Linux and macOS, a PowerShell one-liner on Windows, and binaries on GitHub Releases. Unsigned. The installer creates the app icon. | Q-71, Q-78 |
| NFR-11 | Updates | Checks GitHub's latest release at most once a day at startup, never pre-releases, and only shows a message. `:update` installs it. A setting turns it off. | Q-71, Q-79 |
| NFR-12 | Terminal support | Runs in the user's default terminal. Windows Terminal, macOS Terminal and Kitty are priorities. | Q-50, Q-75 |

## 5. Constraints

| ID | Constraint | Consequence |
|---|---|---|
| C-1 | The user's terminal and OS receive keys first, and some have defaults of their own. | Default keys need testing in real terminals, and must be changeable. |
| C-2 | Terminals without the newer keyboard protocol can't tell Ctrl+Shift+letter from Ctrl+letter. | Defaults avoid Ctrl+Shift+letter (Q-74). |
| C-3 | Ctrl+Alt equals AltGr on many Windows keyboards, including Spanish layouts. | Avoid Ctrl+Alt+letter defaults. |
| C-4 | While a terminal tab has focus, the tool inside needs Escape. | Escape then `:` can't work in terminal tabs, so a reserved key is required. |
| C-5 | LazyCode can only guess whether an AI tool is idle. | Automatic sleep has safeguards (Q-77). |
| C-6 | Only tools with a resume flag can sleep and return to the same chat. | Tools without it stay running (Q-59, Q-77). |
| C-7 | macOS Terminal needs "Use Option as Meta Key" for Alt shortcuts. | macOS defaults use Ctrl+K instead, and commands always work (Q-80). |

## 6. Traceability

| Feature | Use cases | Design |
|---|---|---|
| F1 Panels | UC-01, UC-03, UC-04, UC-05 | [06](06-components.md), [07](07-ui-ux.md) |
| F2 Tabs | UC-01, UC-02 | [06](06-components.md), [07](07-ui-ux.md) |
| F3 Commands and editing | UC-06, UC-15 | [06](06-components.md), [07](07-ui-ux.md) |
| F4 Languages | UC-07, UC-08 | [09](09-language-and-plugin-api.md) |
| F5 Harnesses | UC-10, UC-11 | [10](10-harness-integration.md) |
| F6 Projects | UC-09, UC-13 | [08](08-data-and-config.md) |
| F7 Directory switch | UC-12 | [07](07-ui-ux.md) |
| F8 Store | UC-16 | [09](09-language-and-plugin-api.md) |
| F9 Project switch | UC-13 | [07](07-ui-ux.md) |
| F10 Shortcuts and run | UC-14, UC-17 | [07](07-ui-ux.md), [08](08-data-and-config.md) |
| F11 Onboarding and launch | UC-18 | [07](07-ui-ux.md), [15](15-release-and-governance.md) |
| F12 Watching AI changes | UC-19 | [06](06-components.md), [10](10-harness-integration.md) |
