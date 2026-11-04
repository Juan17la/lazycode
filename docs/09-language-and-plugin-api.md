# Language Support and Plugin API

**Status:** Draft. **Decided** comes from your answers, and the API itself is **Proposed**.

## Default languages

**Decided (Q-27, Q-28, Q-51):** Syntax highlighting in the first version. Go-to-definition, rename and format come next. Everything else comes later.

| Language | Language server (proposed) | Needs |
|---|---|---|
| Go | `gopls` | Go |
| Python | `pyright` | Node |
| TypeScript, JavaScript | `typescript-language-server` | Node |

These are configured by default, but they aren't bundled. **The user installs them** from onboarding or the store (Q-29). Onboarding shows the exact command, and runs it after confirmation (Q-68). If the tool it needs, such as Node or Go, is missing, LazyCode says so.

**Highlighting (Q-69):** The official tree-sitter C binding, with grammars for these four languages built in. Files over about 1 MB aren't highlighted.

## Adding a language

**Proposed:** A language needs only a small TOML file, and no code. This meets your original "easy" requirement, and contributors can add one without knowing Lua.

```toml
name = "rust"
extensions = ["rs"]
comment = "//"
indent = 4

[server]
command = "rust-analyzer"
```

**Open technical point:** Tree-sitter grammars are compiled C, so a language pack can't simply add one as text. Options are shared libraries built per OS, or building more grammars into LazyCode. This needs a decision before the store (M6), and doesn't affect the first version.

## Plugin model

**Decided (Q-30):** Plugins are written in Lua.

**Proposed:**

- It runs on `gopher-lua`, which is Lua 5.1 implemented in pure Go. It adds about 2 MB and needs no C compiler.
- Each plugin has its own Lua state, created when first needed.
- The API is small, and designed around events.
- **Time limit:** A slow plugin must not freeze typing. Plugin calls are stopped if they exceed a limit, and repeat offenders are disabled.
- **Failure:** An error disables the plugin and shows a message, but never crashes the editor.

**Neovim plugins won't run.** They share the language, but they depend on Neovim's API and LuaJIT.

### Plugin layout

```text
plugins/my-plugin/
  plugin.toml
  main.lua
```

```toml
name = "my-plugin"
version = "1.0.0"
api_version = 1
description = "Trims trailing spaces on save"
does = "Removes trailing spaces from files when they are saved"
```

## API reference

**Proposed first version.** Only add more when plugins actually need it.

```lua
local lc = require("lazycode")

lc.command("trim", function() ... end)     -- adds :trim
lc.on("buffer_save", function(buf) ... end)
lc.on("buffer_open", function(buf) ... end)
lc.key("ctrl+alt+t", "trim")
lc.message("Trimmed")

local buf = lc.current_buffer()
buf:text(start_pos, end_pos)
buf:replace(start_pos, end_pos, "new text")
buf:path()
```

Panels, custom UI and access to running sessions: later, on demand.

## Consent

**Decided (Q-67):** Local plugins can do anything. Before a plugin is added, LazyCode shows what it does, and the user must accept.

```text
┌─ Add plugin? ────────────────────────────────────────┐
│ my-plugin 1.0.0                                      │
│ Removes trailing spaces from files when they are     │
│ saved.                                               │
│                                                      │
│ Plugins can read your files and run any command.     │
│ Only accept plugins you trust.                       │
│                                                      │
│                              [Decline]   [Accept]    │
└──────────────────────────────────────────────────────┘
```

**The description is written by the plugin's author, and LazyCode can't check it.** That's why the warning line is always shown. There's no sandbox, so plugins get Lua's normal `os` and `io` libraries.

**Proposed:** Store plugins go through the same prompt, and acceptance is remembered per plugin and version. An update asks again, because the code changed.

## Distribution

**Decided (Q-06, Q-31, Q-67):** A store with plugins and language support from contributors. The store only shows plugins accepted through its Git repository. Local plugins work without submission, and aren't listed.

**Proposed flow:**

```text
Author's repo ──► pull request to lazycode-store (adds an index entry)
                         │
                   review and CI checks
                         │
                     merged into index ──► editor reads index ──► user installs
```

An index entry pins an exact commit:

```toml
name = "my-plugin"
repository = "https://github.com/author/my-plugin"
commit = "<full commit hash>"
checksum = "<hash>"
```

Pinning matters: without it, a plugin could change after being reviewed. Each update is a new pull request.

**Costs:** Someone has to review all submissions, so the store comes after the core is stable (Q-49).

## Versioning and compatibility

**Proposed:** `api_version` is a single number. Additions keep the number, and removals or changes raise it. LazyCode supports the current and previous versions, and refuses others with a clear message.

## Plugin author guide

TBD, written once the API is implemented. It will cover a tutorial, API reference, testing and submitting to the store.
