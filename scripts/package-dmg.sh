#!/usr/bin/env bash
# Makes dist/AltSerialPort-<version>.dmg on macOS: AltSerialPort.app with a
# universal binary (arm64 + amd64) and a link to /Applications.
#
# Signing: $APPLE_SIGN_IDENTITY, e.g. "Developer ID Application: Name (TEAMID)",
# with the hardened runtime. Notarization: $APPLE_NOTARY_PROFILE, a profile
# saved by `xcrun notarytool store-credentials`. Each is skipped when its
# variable is not set.
set -euo pipefail

if [ "$(uname -s)" != Darwin ]; then
	echo "package-dmg.sh runs on macOS only: it needs lipo, sips, iconutil, codesign and hdiutil" >&2
	exit 1
fi

cd "$(dirname "$0")/.."
VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}"
export VERSION
[ -f dist/altserialport-darwin-arm64 ] || scripts/build.sh darwin/arm64
[ -f dist/altserialport-darwin-amd64 ] || scripts/build.sh darwin/amd64

# CFBundleShortVersionString takes numbers only: v1.2.3-4-gabc -> 1.2.3
SHORTVER="${VERSION#v}"
SHORTVER="${SHORTVER%%-*}"
case "$SHORTVER" in
[0-9]*.[0-9]*) ;;
*) SHORTVER="0.0.0" ;;
esac

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
APP="$WORK/AltSerialPort.app"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

lipo -create -output "$APP/Contents/MacOS/altserialport" \
	dist/altserialport-darwin-arm64 dist/altserialport-darwin-amd64
chmod 755 "$APP/Contents/MacOS/altserialport"

# The icon: the PNGs of mkicon into an .iconset, sips makes each size from
# the largest, iconutil makes the .icns
go run ./cmd/mkicon -out "$WORK/png" -sizes 1024 -ico=false
SET="$WORK/AltSerialPort.iconset"
mkdir -p "$SET"
for size in 16 32 128 256 512; do
	sips -z "$size" "$size" "$WORK/png/altserialport-1024.png" --out "$SET/icon_${size}x${size}.png" >/dev/null
	double=$((size * 2))
	sips -z "$double" "$double" "$WORK/png/altserialport-1024.png" --out "$SET/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$SET" -o "$APP/Contents/Resources/AltSerialPort.icns"

cat >"$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>AltSerialPort</string>
	<key>CFBundleDisplayName</key>
	<string>AltSerialPort</string>
	<key>CFBundleIdentifier</key>
	<string>pro.altbins.altserialport</string>
	<key>CFBundleExecutable</key>
	<string>altserialport</string>
	<key>CFBundleIconFile</key>
	<string>AltSerialPort</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleShortVersionString</key>
	<string>$SHORTVER</string>
	<key>CFBundleVersion</key>
	<string>$VERSION</string>
	<key>LSApplicationCategoryType</key>
	<string>public.app-category.developer-tools</string>
	<key>LSMinimumSystemVersion</key>
	<string>11.0</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>NSHumanReadableCopyright</key>
	<string>Copyright © 2026 Ivan Poluianov</string>
</dict>
</plist>
PLIST
plutil -lint "$APP/Contents/Info.plist" >/dev/null

if [ -n "${APPLE_SIGN_IDENTITY:-}" ]; then
	echo "Signing with $APPLE_SIGN_IDENTITY"
	codesign --force --options runtime --timestamp \
		--sign "$APPLE_SIGN_IDENTITY" "$APP/Contents/MacOS/altserialport"
	codesign --force --options runtime --timestamp \
		--identifier pro.altbins.altserialport --sign "$APPLE_SIGN_IDENTITY" "$APP"
	codesign --verify --strict --verbose=2 "$APP"
else
	echo "APPLE_SIGN_IDENTITY is not set: the app is not signed (Gatekeeper will warn)"
fi

# The dmg: the app and a link to /Applications to drag it onto
STAGE="$WORK/dmg"
mkdir -p "$STAGE"
cp -R "$APP" "$STAGE/"
ln -s /Applications "$STAGE/Applications"
OUT="dist/AltSerialPort-$VERSION.dmg"
rm -f "$OUT"
hdiutil create -volname "AltSerialPort" -srcfolder "$STAGE" -fs HFS+ -format UDZO -ov "$OUT" >/dev/null

if [ -n "${APPLE_SIGN_IDENTITY:-}" ]; then
	codesign --force --timestamp --sign "$APPLE_SIGN_IDENTITY" "$OUT"
fi

if [ -n "${APPLE_NOTARY_PROFILE:-}" ] && [ -n "${APPLE_SIGN_IDENTITY:-}" ]; then
	echo "Notarizing (this takes a few minutes)"
	xcrun notarytool submit "$OUT" --keychain-profile "$APPLE_NOTARY_PROFILE" --wait
	xcrun stapler staple "$OUT"
	xcrun stapler validate "$OUT"
elif [ -n "${APPLE_NOTARY_PROFILE:-}" ]; then
	echo "APPLE_NOTARY_PROFILE is set but APPLE_SIGN_IDENTITY is not: notarization needs a signed app, skipped"
else
	echo "APPLE_NOTARY_PROFILE is not set: notarization is skipped"
fi
echo "$OUT"
