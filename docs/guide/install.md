# Install

LazyCode is one file, with no installer to click through. Releases are not signed, and the scripts below only check that your download matches the checksum published with the release.

## Linux and macOS

```bash
curl -fsSL https://raw.githubusercontent.com/Juan17la/lazycode/main/install.sh | sh
```

Want to read it first? It is [install.sh](../../install.sh), about 100 lines. The script:

1. Works out your system (Linux or macOS, Intel/AMD or ARM).
2. Downloads the program and `checksums.txt` from the [latest release](https://github.com/Juan17la/lazycode/releases/latest), and refuses to go on if the SHA-256 doesn't match.
3. Puts `lazycode` in `~/.local/bin`. It never uses `sudo`.
4. If that folder isn't on your `PATH`, adds one line to your shell's startup file (`~/.zshrc`, `~/.bashrc` or `~/.bash_profile` on macOS, or fish's `config.fish`) and tells you which. Running it again won't add it twice.
5. Creates an app icon, so searching for LazyCode in your system's apps opens your terminal running it. If this fails you get a warning, and LazyCode is still installed.

Options, set before `sh`:

| Variable | Effect |
|---|---|
| `LAZYCODE_INSTALL_DIR` | Install somewhere other than `~/.local/bin` |
| `LAZYCODE_NO_MODIFY_PATH=1` | Don't touch any shell startup file |

For example: `curl -fsSL .../install.sh | LAZYCODE_NO_MODIFY_PATH=1 sh`.

## Windows

In PowerShell (64-bit Intel or AMD only):

```powershell
irm https://raw.githubusercontent.com/Juan17la/lazycode/main/install.ps1 | iex
```

It checks the SHA-256, installs to `%LOCALAPPDATA%\Programs\lazycode`, adds that folder to your user `PATH` (not the whole machine's), and adds a Start menu shortcut. It needs no administrator rights. Read it first: [install.ps1](../../install.ps1). Open a new terminal afterwards.

## By hand

Download `lazycode_<os>_<cpu>` for your system from the [releases page](https://github.com/Juan17la/lazycode/releases), such as `lazycode_linux_amd64`, `lazycode_darwin_arm64` or `lazycode_windows_amd64.exe`. Check it against `checksums.txt` (`sha256sum -c --ignore-missing checksums.txt`), rename it to `lazycode`, mark it executable (`chmod +x`), and put it on your `PATH`.

## From source

You need Go and a C compiler (syntax highlighting uses a C library).

```bash
git clone https://github.com/Juan17la/lazycode
cd lazycode
go build -o lazycode ./cmd/lazycode
```

## Check it worked

```bash
lazycode --version
```

## Updating

LazyCode checks GitHub for a newer release at most once a day, and only shows a message. Type `:update` to download it, verify its checksum and replace the old file; then restart. To stop the daily check, set `check = false` under `[updates]` in `:config`. A build from source reports `dev` and never updates itself.

## Uninstalling

Delete the `lazycode` file, the app icon (`~/.local/share/applications/lazycode.desktop` on Linux, `~/Applications/LazyCode.app` on macOS, the Start menu shortcut on Windows), and the `# added by the lazycode installer` line in your shell startup file. Your settings stay in the config folder until you remove it ([Keys and config](keys-and-config.md)).

Problems? See [Troubleshooting](troubleshooting.md).
