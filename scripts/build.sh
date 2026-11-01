#!/usr/bin/env bash
# Builds the release binaries into dist/:
#
#   altserialport-linux-amd64    altserialport-linux-arm64
#   altserialport-windows-amd64.exe
#   altserialport-darwin-arm64   altserialport-darwin-amd64
#
# and the icons the packages take (dist/icons, dist/altserialport.png,
# dist/altserialport.ico). The version is $VERSION or the git tag.
# Only some targets: scripts/build.sh linux/amd64 windows/amd64
set -euo pipefail

cd "$(dirname "$0")/.."
VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}"
PKG=github.com/ipoluianov/altserialport
TARGETS=("$@")
if [ ${#TARGETS[@]} -eq 0 ]; then
	TARGETS=(linux/amd64 linux/arm64 windows/amd64 darwin/arm64 darwin/amd64)
fi

mkdir -p dist
echo "AltSerialPort $VERSION"
for t in "${TARGETS[@]}"; do
	os="${t%/*}"
	arch="${t#*/}"
	out="dist/altserialport-$os-$arch"
	ldflags="-s -w -X $PKG/app.Version=$VERSION"
	if [ "$os" = windows ]; then
		out="$out.exe"
		ldflags="$ldflags -H=windowsgui"
	fi
	echo "  $out"
	CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "$ldflags" -o "$out" .
done

go run ./cmd/mkicon -out dist/icons -sizes 16,32,48,64,128,256,512,1024
cp dist/icons/altserialport-256.png dist/altserialport.png
cp dist/icons/altserialport.ico dist/altserialport.ico
