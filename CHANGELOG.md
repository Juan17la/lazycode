# Changelog

All notable changes are listed here, in the [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. Versions follow [Semantic Versioning](https://semver.org/). Before 1.0, anything may change, except that your own files (config, keys, projects) keep migrating.

## [Unreleased]

The first release, v0.1.0, will contain:

### Added

- A terminal editor: tabs, splits, two panels, a file tree, the mouse, light and dark themes, and a command bar (`:`).
- Editing with ordinary keys (no modes): undo and redo, copy, cut and paste, multi-cursor editing, find and replace, safe saves, crash recovery, and files up to 100 MB.
- Projects made of several directories, with a project and directory switcher.
- Terminal tabs running any command, and AI tool sessions that sleep when idle and resume the same chat.
- `:play` to run a directory's project, and live updates of open files that change on disk, with modified-line markers.
- Search across a directory, with a preview of every change before a replace.
- Git view (status, stage, commit, branches, merge, diffs) and GitHub through `gh`.
- Syntax highlighting for Go, Python, JavaScript and TypeScript.
- Language server actions (go to definition, rename, format) for `gopls`, `pyright` and `typescript-language-server`.
- First-run onboarding, and a trust choice for each folder before anything runs there.
- Settings in `config.toml` and key bindings in `keys.toml`, applied on save.
- A daily update check against GitHub's latest release (it can be turned off) and `:update` to install a new version. `lazycode --version` prints the version.
- Install scripts for Linux, macOS and Windows that verify a SHA-256 checksum, and an app icon that opens LazyCode in your default terminal.
- Binaries for Linux (amd64, arm64), macOS (amd64, arm64) and Windows (amd64). They are not signed.
- User guides in `docs/guide/`.
