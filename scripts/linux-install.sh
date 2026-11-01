#!/usr/bin/env bash
# Installs AltSerialPort for the current user, without root:
#
#   ~/.local/bin/altserialport
#   ~/.local/share/applications/altserialport.desktop
#   ~/.local/share/icons/hicolor/256x256/apps/altserialport.png
#
# The latest release is downloaded from GitHub:
#
#   curl -fsSL https://raw.githubusercontent.com/ipoluianov/altserialport/main/scripts/linux-install.sh | bash
#
# Options:
#   --uninstall   remove those files (the settings in ~/.altbins/.altserialport stay)
#   --from DIR    take altserialport-linux-<arch> and altserialport.png from DIR
#                 (dist/ of build.sh) instead of downloading them
set -euo pipefail

REPO_URL="https://github.com/ipoluianov/altserialport/releases/latest/download"
BIN_DIR="$HOME/.local/bin"
APPS_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
ICONS_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor"
BIN="$BIN_DIR/altserialport"
DESKTOP="$APPS_DIR/altserialport.desktop"
ICON="$ICONS_DIR/256x256/apps/altserialport.png"

UNINSTALL=0
FROM=""
while [ $# -gt 0 ]; do
	case "$1" in
	--uninstall) UNINSTALL=1 ;;
	--from)
		FROM="${2:?--from needs a directory}"
		shift
		;;
	-h | --help)
		sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "unknown option: $1" >&2
		exit 2
		;;
	esac
	shift
done

refresh() {
	command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database -q "$APPS_DIR" 2>/dev/null || true
	# A cache of the user's icons is refreshed only when there is one: a new
	# one would hide the icons added later by other programs
	if [ -f "$ICONS_DIR/icon-theme.cache" ] && command -v gtk-update-icon-cache >/dev/null 2>&1; then
		gtk-update-icon-cache -q -t "$ICONS_DIR" 2>/dev/null || true
	fi
}

if [ "$UNINSTALL" = 1 ]; then
	rm -f "$BIN" "$DESKTOP" "$ICON"
	refresh
	echo "AltSerialPort is removed. The settings in ~/.altbins/.altserialport are kept."
	exit 0
fi

case "$(uname -m)" in
x86_64 | amd64) ARCH=amd64 ;;
aarch64 | arm64) ARCH=arm64 ;;
*)
	echo "There is no build of AltSerialPort for $(uname -m): build it from the source with go build" >&2
	exit 1
	;;
esac

# fetch URL FILE downloads with curl or wget
fetch() {
	if command -v curl >/dev/null; then
		curl -fL --progress-bar -o "$2" "$1"
	elif command -v wget >/dev/null; then
		wget -q --show-progress -O "$2" "$1"
	else
		echo "Neither curl nor wget is found" >&2
		return 1
	fi
}

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

if [ -n "$FROM" ]; then
	cp "$FROM/altserialport-linux-$ARCH" "$TMP/altserialport"
	cp "$FROM/altserialport.png" "$TMP/altserialport.png" 2>/dev/null || true
else
	echo "Downloading AltSerialPort for linux/$ARCH"
	fetch "$REPO_URL/altserialport-linux-$ARCH" "$TMP/altserialport"
	fetch "$REPO_URL/altserialport.png" "$TMP/altserialport.png" 2>/dev/null ||
		echo "The icon is not downloaded: the menu shows the program without it"
fi

mkdir -p "$BIN_DIR" "$APPS_DIR"
install -m755 "$TMP/altserialport" "$BIN"
if [ -s "$TMP/altserialport.png" ]; then
	install -Dm644 "$TMP/altserialport.png" "$ICON"
fi
cat >"$DESKTOP" <<DESKTOPFILE
[Desktop Entry]
Type=Application
Name=AltSerialPort
GenericName=Serial Terminal
Comment=A terminal for serial ports
Exec=$BIN
Icon=altserialport
Terminal=false
Categories=Development;Electronics;
Keywords=serial;uart;com;tty;terminal;
StartupWMClass=altserialport
DESKTOPFILE
refresh

echo "AltSerialPort is installed: $BIN"
case ":$PATH:" in
*":$BIN_DIR:"*) ;;
*) echo "$BIN_DIR is not in PATH: start it from the menu or add it to PATH" ;;
esac

# The serial ports belong to the group dialout (uucp on Arch)
group=dialout
getent group dialout >/dev/null 2>&1 || group=uucp
if getent group "$group" >/dev/null 2>&1 && ! id -nG | tr ' ' '\n' | grep -qx "$group"; then
	echo
	echo "You are not in the group $group, which owns the serial ports."
	echo "Add yourself to it and log in again:"
	echo "  sudo usermod -aG $group \$USER"
fi
