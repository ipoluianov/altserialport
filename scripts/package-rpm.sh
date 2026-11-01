#!/usr/bin/env bash
# Makes dist/altserialport-<version>-<release>.<arch>.rpm from the binary
# built by build.sh (it is built when missing). The arch is amd64 or arm64.
# rpmbuild is needed: the package rpm-build (Fedora) or rpm (Debian).
set -euo pipefail

ARCH="${1:-}"
case "$ARCH" in
amd64) RPMARCH=x86_64 ;;
arm64) RPMARCH=aarch64 ;;
*)
	echo "usage: $0 amd64|arm64" >&2
	exit 2
	;;
esac
if ! command -v rpmbuild >/dev/null; then
	echo "rpmbuild is not found: install rpm-build (Fedora, RHEL) or rpm (Debian, Ubuntu) to make the .rpm" >&2
	exit 1
fi

cd "$(dirname "$0")/.."
VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}"
export VERSION
BIN="dist/altserialport-linux-$ARCH"
[ -f "$BIN" ] || scripts/build.sh "linux/$ARCH"
[ -d dist/icons ] || go run ./cmd/mkicon -out dist/icons -sizes 16,32,48,64,128,256,512

# An RPM version has no "-": v1.2.3-4-gabcdef is version 1.2.3, release
# 4.gabcdef; a version without a tag is 0.0.0, release git<hash>
V="${VERSION#v}"
case "$V" in
[0-9]*.[0-9]*)
	RPMVER="${V%%-*}"
	RPMREL="1"
	if [ "$V" != "$RPMVER" ]; then
		RPMREL="$(printf '%s' "${V#*-}" | tr -c 'A-Za-z0-9.\n' '.')"
	fi
	;;
*)
	RPMVER="0.0.0"
	RPMREL="git$(printf '%s' "$V" | tr -c 'A-Za-z0-9.\n' '.')"
	;;
esac

TOP="$(mktemp -d)"
trap 'rm -rf "$TOP"' EXIT
mkdir -p "$TOP"/{BUILD,RPMS,SOURCES,SPECS,SRPMS}
SRC="$TOP/SOURCES/files"
install -Dm755 "$BIN" "$SRC/altserialport"
cp -r dist/icons "$SRC/icons"
cat >"$SRC/altserialport.desktop" <<DESKTOP
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

SIZES="16 32 48 64 128 256 512"
cat >"$TOP/SPECS/altserialport.spec" <<SPEC
# The binary is built by Go with no C; nothing is compiled here
%define debug_package %{nil}
%global __strip /bin/true
%global _build_id_links none

Name:           altserialport
Version:        $RPMVER
Release:        $RPMREL
Summary:        Terminal for serial ports
License:        MIT
URL:            https://altbins.pro/altserialport/
Requires:       libX11

%description
AltSerialPort is a terminal for serial ports, made for low-level debugging
of hardware: microcontroller boards, USB-UART adapters, modems, GPS and
industrial devices. Text, hex and mixed views, timestamps, logging,
checksums, DTR/RTS control and reconnection when a device comes back.

%install
install -Dm755 $SRC/altserialport %{buildroot}%{_bindir}/altserialport
install -Dm644 $SRC/altserialport.desktop %{buildroot}%{_datadir}/applications/altserialport.desktop
for size in $SIZES; do
	install -Dm644 $SRC/icons/altserialport-\$size.png %{buildroot}%{_datadir}/icons/hicolor/\${size}x\${size}/apps/altserialport.png
done

%post
if [ "\$1" -eq 1 ]; then
	echo "AltSerialPort: the serial ports belong to the group dialout."
	echo "If a port cannot be opened, add yourself to it and log in again:"
	echo "  sudo usermod -aG dialout \\\$USER"
fi
touch --no-create %{_datadir}/icons/hicolor >/dev/null 2>&1 || :

%files
%{_bindir}/altserialport
%{_datadir}/applications/altserialport.desktop
%{_datadir}/icons/hicolor/*/apps/altserialport.png
SPEC

rpmbuild --quiet -bb --target "$RPMARCH" --define "_topdir $TOP" "$TOP/SPECS/altserialport.spec"
OUT="dist/altserialport-$RPMVER-$RPMREL.$RPMARCH.rpm"
cp "$TOP/RPMS/$RPMARCH/"*.rpm "$OUT"
echo "$OUT"
