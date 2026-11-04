#!/bin/sh
# Creates the app icon on Linux and macOS (Q-50): searching for LazyCode in the
# system's apps opens the default terminal running it. REL-1 calls this from the
# install script.
#
# Usage: install-icon.sh /absolute/path/to/lazycode
set -eu

bin=${1:?usage: install-icon.sh /absolute/path/to/lazycode}
case $bin in
/*) ;;
*)
	echo "install-icon: the path must be absolute: $bin" >&2
	exit 1
	;;
esac
if [ ! -x "$bin" ]; then
	echo "install-icon: not an executable file: $bin" >&2
	exit 1
fi

case $(uname -s) in
Linux)
	dir=${XDG_DATA_HOME:-$HOME/.local/share}/applications
	mkdir -p "$dir"
	# Desktop entry quoting: backslash-escape \ " ` $ inside double quotes, and double %.
	exec_path=$(printf '%s' "$bin" | sed -e 's/[\\"`$]/\\&/g' -e 's/%/%%/g' -e 's/\\/\\\\/g')
	cat >"$dir/lazycode.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=LazyCode
Comment=A light code editor that runs in your terminal
Exec="$exec_path"
Icon=utilities-terminal
Terminal=true
Categories=Development;TextEditor;
EOF
	echo "$dir/lazycode.desktop"
	;;
Darwin)
	# macOS has no general default terminal, so this opens Terminal.app, which is
	# what opens a .command file unless the user changed that.
	app=$HOME/Applications/LazyCode.app
	mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
	cat >"$app/Contents/Info.plist" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>LazyCode</string>
	<key>CFBundleIdentifier</key><string>io.github.juan17la.lazycode</string>
	<key>CFBundleExecutable</key><string>LazyCode</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleVersion</key><string>0</string>
</dict>
</plist>
EOF
	quoted=$(printf '%s' "$bin" | sed "s/'/'\\\\''/g")
	printf "#!/bin/sh\nexec '%s'\n" "$quoted" >"$app/Contents/Resources/lazycode.command"
	cat >"$app/Contents/MacOS/LazyCode" <<'EOF'
#!/bin/sh
exec open "$(dirname "$0")/../Resources/lazycode.command"
EOF
	chmod +x "$app/Contents/Resources/lazycode.command" "$app/Contents/MacOS/LazyCode"
	echo "$app"
	;;
*)
	echo "install-icon: on Windows, use install-icon.ps1" >&2
	exit 1
	;;
esac
