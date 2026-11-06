# Writing a plugin

A plugin is a folder with two files, written in Lua 5.1. It has the same power as you: it can read your files and run any command. That's why LazyCode asks before adding one.

## A first plugin

Create a folder named after your plugin in `plugins/` in the config folder (`~/.config/lazycode/plugins/` on Linux, `~/Library/Application Support/lazycode/plugins/` on macOS, `%AppData%\lazycode\plugins\` on Windows).

`plugins/hello/plugin.toml`:

```toml
name = "hello"              # Must match the folder. Letters, digits, dots, dashes, underscores.
version = "1.0.0"           # A new version asks the user again.
api_version = 1
description = "Says hello"  # A short summary.
does = "Adds :hello, which shows a greeting."  # Required. Shown when asking for consent.
```

`plugins/hello/main.lua`:

```lua
local lc = require("lazycode")

lc.command("hello", function()
  lc.message("Hello!")
end)
```

Start LazyCode, press `a` to accept, then run `:hello`. The folder `examples/plugins` in the repository has three more: `trim-on-save`, `word-count` and `todo-count`.

## API

| Call | What it does |
|---|---|
| `lc.command(name, fn)` | Adds `:name`. Built-in commands win over plugin ones. |
| `lc.on("buffer_save", fn)` | Runs `fn(buf)` just before a file is written, so your changes are saved too. Also on auto-save. |
| `lc.on("buffer_open", fn)` | Runs `fn(buf)` when a file is opened from the tree. Not for tabs restored at start. |
| `lc.key(key, command)` | Binds a key such as `ctrl+alt+t` to a command. The user's own bindings win. |
| `lc.message(text)` | Shows text on the status line. |
| `lc.current_buffer()` | The current file, or `nil` if the tab isn't one. |
| `buf:path()` | The file's path. |
| `buf:len()` | Its size in bytes. |
| `buf:text(start, end)` | Its text. Bytes from 1, end included, like `string.sub`. No arguments means the whole file. |
| `buf:replace(start, end, text)` | Replaces those bytes, as one undo step. `buf:replace(5, 4, "x")` inserts before byte 5. |

Lua's `os` and `io` libraries are there as usual.

## Limits

- A call that runs over 100 ms is stopped, and after 3 of those the plugin is disabled. Starting a plugin has 1 second.
- The limit is checked between Lua instructions, so a blocking call such as `os.execute("sleep 9")` still freezes the editor. Keep such work short.
- Any error disables the plugin and shows a message. The editor keeps running. `:plugin on NAME` starts it again.
- Files over 100 MB are read-only, and don't send events.
- A `buffer_save` handler should change nothing when the file is already right, or the file never settles.

## Language packs

Add a `language.toml` beside `plugin.toml` to give a language colours, a server for go to definition, rename and format, or both. `main.lua` can be empty. `examples/plugins/sql-colours` is a working one.

```toml
name = "rust"              # Also the language ID sent to the server.
extensions = ["rs"]

[server]
command = "rust-analyzer"  # Run directly, never through a shell.
args = []
markers = ["Cargo.toml"]   # Files that mark a project's root.

[[highlight]]              # Up to 64 rules, in Go's regular expression syntax.
class = "comment"          # keyword, string, number, constant, function, type, variable,
pattern = '//.*$'          # property, operator, tag or comment.

[[highlight]]
class = "string"
pattern = '"(\\.|[^"\\])*"'

[[highlight]]
class = "keyword"
pattern = '\b(fn|let|match|if|else)\b'
```

The leftmost match wins, so `//` inside a string stays part of the string. At the same place, the first rule listed wins. `^` and `$` match at each line, and `(?s)` lets `.` cross lines, as in `(?s)/\*.*?\*/`. `(?i)` ignores case. Rules don't understand nesting, so nested block comments colour wrongly.

A pack can't replace a language that's built in.

## Publishing

See the store's README: you add an entry to its `index.toml` by pull request, pinning a commit and a checksum. `:store` then lists your plugin.

## Commands

`:store` browses the store and installs a plugin, asking first. `:plugin` lists what's running. `:plugin off NAME` stops a plugin and remembers that. `:plugin on NAME` asks again.

## Testing

There's no test harness: run the plugin in LazyCode with a scratch file. Errors appear on the status line, and `:plugin on NAME` reloads your code after a change (accept it again).

## Compatibility

`api_version` is a single number. Additions keep it, and removals or changes raise it. A plugin with another number is refused with a message.
