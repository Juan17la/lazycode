# Changelog

All notable changes are listed here, in the [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. Versions follow [Semantic Versioning](https://semver.org/). Before 1.0, anything may change, except that your own files (config, keys, projects) keep migrating.

## [Unreleased]

The first release, v0.1.0, will contain:

### Added

- GUI-style windows in the terminal: a menu bar (File, Edit, View, Go, Terminal, Git, Help), right-click menus, close buttons on tabs, buttons on tab bars, and a clickable status line.
- A dock under the code holds terminal and AI tabs, which look different from file tabs. It can be hidden while its tools keep running.
- Menus for settings, appearance, themes (with live preview), plugins and shortcuts, and forms for a new project, a new directory and a new file. Each has a command as well.
- Mouse dragging: select text, move tabs, resize panels, the dock and splits, and drag a scroll bar. Double and triple clicks select a word and a line. Dragging in a terminal tab copies what's selected.
- Word wrap (Alt+Z or `:wrap`), and `ui.text_style`.
- A right-click menu in the file tree: new file, new folder, rename and delete.
- Dragged sizes are remembered after quitting, and tabs can be dragged between splits.
- A folder browser (File > Open Folder or `:dir open`) that reaches any folder on the device. The project and directory switchers offer new projects and other folders, and are also a key away (Alt+P, Alt+D).
- The project and folder names in the top bar, and Terminal and Help in the menu bar, open windows that list what's there, with × to remove a folder or close a terminal or AI session.
- Clearer layout: the area with focus gets a tinted background and a solid title bar, code and dock areas show their name, panels have a rule under the title and an inset, and windows have padding, section rules and gaps between options when they fit.
- Big pastes are quick and keep their line breaks. A paste with nowhere to go says so.
- Easier to start with: one word for each thing (Files, Folder), hints that fit where you are and are never cut off, a clearer empty screen, `:messages` for what the status line said, shortcuts you can look up by key, forms a stray click can't close, and a real dialog before deleting.
- Alt+Q closes a tab, even from inside a terminal. The status line of a terminal has Close and Leave buttons.
- Tools that ask for the mouse now get the whole drag, so they can select text themselves.
- More colour throughout: coloured borders that show focus, a colour per view, and a status line with a mode label.

### Changed

- Shortcuts now follow families: Alt+letter shows, Alt+Shift+letter makes, Ctrl+letter acts on the file. Project is now Alt+P (was Alt+O), run Alt+R (was Alt+P), AI sessions Alt+A (was Alt+I), split Alt+Shift+S (was Alt+S), replace Ctrl+R (was Alt+H), and format F3 (was Shift+Alt+F). `keys.toml` still overrides any of them.
- Terminal and AI tabs always open in the dock, so a split can't hold a terminal any more.

### Added in earlier work

- A terminal editor: tabs, splits, two panels, a file tree, the mouse, and a command bar (`:`).
- Editing with ordinary keys (no modes): undo and redo, copy, cut and paste, multi-cursor editing, find and replace, safe saves, crash recovery, and files up to 100 MB.
- Projects made of several directories, with a project and directory switcher.
- Terminal tabs running any command, and AI tool sessions that sleep when idle and resume the same chat.
- `:play` to run a directory's project, and live updates of open files that change on disk, with modified-line markers.
- Search across a directory, with a preview of every change before a replace.
- Git view (status, stage, commit, branches, merge, diffs) and GitHub through `gh`.
- Syntax highlighting for Go, Python, JavaScript, TypeScript, Rust, C, JSON, shell, HTML and CSS.
- Language server actions (go to definition, rename, format) for `gopls`, `pyright` and `typescript-language-server`.
- First-run onboarding, and a trust choice for each folder before anything runs there.
- Settings in `config.toml` and key bindings in `keys.toml`, applied on save.
- A daily update check against GitHub's latest release (it can be turned off) and `:update` to install a new version. `lazycode --version` prints the version.
- Install scripts for Linux, macOS and Windows that verify a SHA-256 checksum, and an app icon that opens LazyCode in your default terminal.
- Binaries for Linux (amd64, arm64), macOS (amd64, arm64) and Windows (amd64). They are not signed.
- Twelve themes including Gruvbox, Dracula, Nord and Tokyo Night, chosen with `:theme`, and `[ui.colors]` to change any colour. Richer syntax colours, panel and file icons, and Git state colours in the file tree.
- User guides in `docs/guide/`.
- Lua plugins from the config folder, added only after you accept what each one says it does, with `:plugin` to list, turn on and turn off. See `docs/guide/plugins.md`.
- A plugin store: `:store` browses reviewed plugins, verifies what it downloads against a pinned commit and checksum, and asks before adding it. Language packs add a language server, colours from text rules, or both.
