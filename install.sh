#!/bin/sh
# Install jk from GitHub Releases.
#
#   curl -fsSL https://raw.githubusercontent.com/phuquocchamp/jk/main/install.sh | sh
#   curl -fsSL .../install.sh | sh -s -- --version v0.1.0 --dir ~/bin
#
# Downloads the archive for this OS and architecture, checks it against the
# release's checksums.txt and installs the jk binary to $JK_INSTALL_DIR
# (default ~/.local/bin). No sudo, no changes to shell rc files.
#
# Environment: JK_VERSION (tag, default latest), JK_INSTALL_DIR,
# JK_BASE_URL (default https://github.com/phuquocchamp/jk/releases; the
# archive is fetched from $JK_BASE_URL/download/<tag>/).
set -eu

base=${JK_BASE_URL:-https://github.com/phuquocchamp/jk/releases}
version=${JK_VERSION:-}
dir=${JK_INSTALL_DIR:-$HOME/.local/bin}

say() { printf '%s\n' "$*" >&2; }
fail() {
	say "install.sh: $*"
	exit 1
}

while [ $# -gt 0 ]; do
	case $1 in
	--version)
		[ $# -ge 2 ] || fail "--version needs a tag, e.g. v0.1.0"
		version=$2
		shift
		;;
	--dir)
		[ $# -ge 2 ] || fail "--dir needs a directory"
		dir=$2
		shift
		;;
	-h | --help)
		say "usage: install.sh [--version vX.Y.Z] [--dir DIR]"
		exit 0
		;;
	*) fail "unknown argument: $1" ;;
	esac
	shift
done

command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v tar >/dev/null 2>&1 || fail "tar is required"

case $(uname -s) in
Darwin) os=darwin ;;
Linux) os=linux ;;
MINGW* | MSYS* | CYGWIN* | Windows_NT)
	fail "on Windows, download the .zip from $base or run: go install github.com/phuquocchamp/jk/cmd/jk@latest"
	;;
*) fail "unsupported OS $(uname -s); try: go install github.com/phuquocchamp/jk/cmd/jk@latest" ;;
esac
case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "unsupported architecture $(uname -m)" ;;
esac

if [ -z "$version" ]; then
	# The /latest redirect avoids the rate-limited API.
	url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$base/latest") ||
		fail "cannot reach $base/latest"
	version=${url##*/}
	case $version in
	v[0-9]*) ;;
	*) fail "no release found at $base (got $url)" ;;
	esac
fi
case $version in
v*) ;;
*) version="v$version" ;;
esac

archive="jk_${version#v}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

say "downloading jk $version ($os/$arch)"
curl -fsSL -o "$tmp/$archive" "$base/download/$version/$archive" ||
	fail "cannot download $archive for $version"
curl -fsSL -o "$tmp/checksums.txt" "$base/download/$version/checksums.txt" ||
	fail "cannot download checksums.txt for $version"

want=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")
[ -n "$want" ] || fail "$archive is not listed in checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
	got=$(sha256sum "$tmp/$archive" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
	got=$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')
else
	fail "sha256sum or shasum is required to verify the download"
fi
[ "$got" = "$want" ] || fail "checksum mismatch for $archive (want $want, got $got)"

tar -xzf "$tmp/$archive" -C "$tmp" jk || fail "the archive has no jk binary"
mkdir -p "$dir"
chmod 0755 "$tmp/jk"
mv -f "$tmp/jk" "$dir/jk"
say "installed $dir/jk ($version, checksum verified)"

case ":$PATH:" in
*":$dir:"*) ;;
*) say "note: $dir is not on PATH; add it, e.g. export PATH=\"$dir:\$PATH\"" ;;
esac

say ""
say "next steps:"
say "  jk auth login          # store your Jenkins API token in the keyring"
say "  jk setup skill         # agent skill for Claude Code"
say "  jk setup mcp --tier read"
