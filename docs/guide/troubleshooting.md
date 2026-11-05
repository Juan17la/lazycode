# Troubleshooting

## `lazycode: command not found`

The install folder isn't on your `PATH` in this terminal yet. Open a new terminal. If that doesn't help, check the startup file the installer said it changed (`~/.zshrc`, `~/.bashrc`, `~/.bash_profile` on macOS, or fish's `config.fish`), or add the folder yourself. If you installed with `LAZYCODE_NO_MODIFY_PATH=1`, the installer left your files alone. On Linux and macOS the folder is `~/.local/bin`, and on Windows `%LOCALAPPDATA%\Programs\lazycode`.

## macOS says it can't verify the app

Releases are not signed or notarized, so macOS may refuse something it considers downloaded from the internet. This depends on how the file arrived: files fetched by `curl`, like the install script does, are normally not marked, but a file downloaded in a browser is. We haven't been able to test every macOS version, so if you see the warning:

- For the `lazycode` file, remove the mark, then run it again:

  ```bash
  xattr -d com.apple.quarantine ~/.local/bin/lazycode
  ```

- For the app icon (`~/Applications/LazyCode.app`), right-click it, choose Open, and confirm. Or open System Settings, then Privacy & Security, and choose Open Anyway.

You can also read what you run: [install.sh](../../install.sh). If you'd rather not run it, [install by hand or from source](install.md).

## Windows shows SmartScreen

LazyCode isn't signed, so Windows may say it protected your PC. Choose More info, then Run anyway, only if you got LazyCode from this project's releases. You can check the file against `checksums.txt` (`Get-FileHash`).

## gopls looks uninstalled, but I installed it

`go install` puts programs in `~/go/bin` (or `$GOBIN`, or `$GOPATH/bin`; on Windows `%USERPROFILE%\go\bin`), and that folder often isn't on `PATH`. LazyCode finds language servers the way your shell does, through `PATH`, so it reports `gopls` as missing.

```bash
command -v gopls || ls ~/go/bin
```

If `ls` finds it, add the folder to your shell's startup file and open a new terminal, such as `export PATH="$PATH:$HOME/go/bin"`. The same goes for `pyright-langserver` and `typescript-language-server` if `npm install -g` put them somewhere not on `PATH`: see `npm config get prefix`. Start LazyCode from a terminal that has the right `PATH`: an app icon starts with the desktop's `PATH`, which may be shorter.

## A key does nothing

Your terminal gets every key first, and some it keeps for itself or can't pass on.

| Where | Clash |
|---|---|
| macOS | Option types special characters, so Alt shortcuts don't arrive. Use Ctrl+K, then the key. Alt+D is Ctrl+K, then D |
| Windows Terminal | Alt+Left and Alt+Right move between panes, so tabs use Alt+, and Alt+. instead. Ctrl+V pastes, which does the same job |
| Linux desktops | Alt+F2 often opens a run dialog. Some desktops use Alt+key for their own shortcuts |
| tmux, screen, other terminal programs inside your terminal | They may take Alt or Ctrl+B (tmux's default prefix is Ctrl+B, which LazyCode uses for the second panel) |
| Terminal tabs | Every key goes to the program there, except Ctrl+\\. Press Ctrl+\\ before a LazyCode key |

To work around a clash, rebind the key in `keys.toml` (`:config keys`, [Keys and config](keys-and-config.md)), or use the command instead: every action has a `:` command. In a file, Esc then `:` opens the command bar.

## I can't select text with the mouse

LazyCode takes the mouse so you can click. Hold Shift while you drag to select text the terminal's own way.

## A setting or key was reported as unusable

LazyCode names the file and the problem, such as `tab_size must be from 1 to 16`, and the rest of your settings keep working. Open it with `:config` or `:config keys`, fix it, and save.

## The app icon does nothing

On Linux it's a desktop entry (`~/.local/share/applications/lazycode.desktop`) that runs in your desktop's default terminal. If your desktop has no default terminal set, it may fail. Run `lazycode` from a terminal instead. The installer only warns if it can't make the icon.

## Updates

`:update` says "development build" if you built from source. Otherwise it needs a connection to GitHub. If you don't want the daily check, set `check = false` under `[updates]` in `:config`.

## Still stuck

Open an issue at <https://github.com/Juan17la/lazycode/issues>. Say your operating system, your terminal, and the output of `lazycode --version`. This is a free-time project and there is no guaranteed response time.
