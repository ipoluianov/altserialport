#!/usr/bin/env bash
# Makes dist/altserialport_<version>_<arch>.deb from the binary built by
# build.sh (it is built when missing). The arch is amd64 or arm64.
set -euo pipefail

ARCH="${1:-}"
case "$ARCH" in
amd64 | arm64) ;;
*)
	echo "usage: $0 amd64|arm64" >&2
	exit 2
	;;
esac
command -v dpkg-deb >/dev/null || {
	echo "dpkg-deb is not found: install the package dpkg" >&2
	exit 1
}

cd "$(dirname "$0")/.."
VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}"
export VERSION
BIN="dist/altserialport-linux-$ARCH"
[ -f "$BIN" ] || scripts/build.sh "linux/$ARCH"
[ -d dist/icons ] || go run ./cmd/mkicon -out dist/icons -sizes 16,32,48,64,128,256,512

# A Debian version starts with a digit and has no "-" of its own:
# v1.2.3 -> 1.2.3, v1.2.3-4-gabcdef -> 1.2.3+4.gabcdef (after the tag),
# v1.2.3-rc1 -> 1.2.3~rc1 (before it), abcdef -> 0.0.0+gitabcdef
DEBVER="${VERSION#v}"
DEBVER="$(printf '%s' "$DEBVER" | sed -E 's/-([0-9]+)-g/+\1.g/; s/-/~/; s/-/./g')"
case "$DEBVER" in
[0-9]*.[0-9]*) ;;
*) DEBVER="0.0.0+git$(printf '%s' "$DEBVER" | tr -c 'A-Za-z0-9.+~\n' '.')" ;;
esac

umask 022
ROOT="$(mktemp -d)"
chmod 755 "$ROOT"
trap 'rm -rf "$ROOT"' EXIT

install -Dm755 "$BIN" "$ROOT/usr/bin/altserialport"
for size in 16 32 48 64 128 256 512; do
	install -Dm644 "dist/icons/altserialport-$size.png" \
		"$ROOT/usr/share/icons/hicolor/${size}x${size}/apps/altserialport.png"
done
install -d "$ROOT/usr/share/applications"
cat >"$ROOT/usr/share/applications/altserialport.desktop" <<DESKTOP
[Desktop Entry]
Type=Application
Name=AltSerialPort
GenericName=Serial Terminal
Comment=A terminal for serial ports
Exec=altserialport
Icon=altserialport
Terminal=false
Categories=Development;Electronics;
Keywords=serial;uart;com;tty;terminal;
StartupWMClass=altserialport
DESKTOP
install -Dm644 LICENSE "$ROOT/usr/share/doc/altserialport/copyright" 2>/dev/null || true

install -d "$ROOT/DEBIAN"
SIZE="$(du -sk --exclude=DEBIAN "$ROOT" | cut -f1)"
cat >"$ROOT/DEBIAN/control" <<CONTROL
Package: altserialport
Version: $DEBVER
Architecture: $ARCH
Maintainer: Ivan Poluianov <ipoluianov@gmail.com>
Installed-Size: $SIZE
Depends: libx11-6
Section: electronics
Priority: optional
Homepage: https://altbins.pro/altserialport/
Description: Terminal for serial ports
 AltSerialPort is a terminal for serial ports, made for low-level debugging
 of hardware: microcontroller boards, USB-UART adapters, modems, GPS and
 industrial devices. Text, hex and mixed views, timestamps, logging,
 checksums, DTR/RTS control and reconnection when a device comes back.
CONTROL
cat >"$ROOT/DEBIAN/postinst" <<'POSTINST'
#!/bin/sh
set -e
if [ "$1" = configure ]; then
	echo "AltSerialPort: the serial ports belong to the group dialout."
	echo "If a port cannot be opened, add yourself to it and log in again:"
	echo "  sudo usermod -aG dialout \$USER"
	if command -v update-desktop-database >/dev/null 2>&1; then
		update-desktop-database -q /usr/share/applications || true
	fi
	if command -v gtk-update-icon-cache >/dev/null 2>&1; then
		gtk-update-icon-cache -q -t /usr/share/icons/hicolor || true
	fi
fi
exit 0
POSTINST
chmod 755 "$ROOT/DEBIAN/postinst"

OUT="dist/altserialport_${DEBVER}_${ARCH}.deb"
dpkg-deb --root-owner-group -Zxz --build "$ROOT" "$OUT"
echo "$OUT"
