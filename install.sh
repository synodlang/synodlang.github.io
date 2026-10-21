#!/bin/sh
# Install the synod CLI from GitHub Releases.
#   curl -fsSL https://synodlang.github.io/install.sh | sh
# Environment: SYNOD_VERSION (tag, e.g. v0.1.0; default: latest),
#              SYNOD_INSTALL (directory; default: $HOME/.local/bin).
set -eu

REPO="synodlang/compiler"

say() { printf '%s\n' "$*"; }
die() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }

windows_help() {
  say "This installer supports macOS and Linux."
  say "On Windows, use Scoop:"
  say "  scoop bucket add synodlang https://github.com/synodlang/scoop-bucket"
  say "  scoop install synod"
  say "or download a .zip from https://synodlang.github.io/install/"
}

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) windows_help; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$1" -o "$2"; }
  fetch_stdout() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -q -O "$2" "$1"; }
  fetch_stdout() { wget -q -O - "$1"; }
else
  die "curl or wget is required"
fi

if command -v sha256sum >/dev/null 2>&1; then
  sha256() { sha256sum "$1" | cut -d ' ' -f 1; }
elif command -v shasum >/dev/null 2>&1; then
  sha256() { shasum -a 256 "$1" | cut -d ' ' -f 1; }
else
  die "sha256sum or shasum is required"
fi

tag="${SYNOD_VERSION:-}"
if [ -z "$tag" ]; then
  tag=$(fetch_stdout "https://api.github.com/repos/$REPO/releases/latest" |
    sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1) ||
    tag=""
  [ -n "$tag" ] || die "could not find the latest release; see https://github.com/$REPO/releases or set SYNOD_VERSION"
fi
case "$tag" in v*) ;; *) tag="v$tag" ;; esac

archive="synod-$tag-$os-$arch.tar.gz"
base="https://github.com/$REPO/releases/download/$tag"
dest="${SYNOD_INSTALL:-$HOME/.local/bin}"

tmp=$(mktemp -d "${TMPDIR:-/tmp}/synod-install.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

say "Downloading $archive"
fetch "$base/$archive" "$tmp/$archive" || die "download failed: $base/$archive"
fetch "$base/SHA256SUMS" "$tmp/SHA256SUMS" || die "download failed: $base/SHA256SUMS"

want=$(awk -v f="$archive" '$2 == f || $2 == "*" f { print $1; exit }' "$tmp/SHA256SUMS")
[ -n "$want" ] || die "$archive is not listed in SHA256SUMS"
got=$(sha256 "$tmp/$archive")
[ "$got" = "$want" ] || die "checksum mismatch for $archive (expected $want, got $got)"
say "Checksum OK"

mkdir "$tmp/x"
tar -xzf "$tmp/$archive" -C "$tmp/x"
bin=$(find "$tmp/x" -type f -name synod | head -n 1)
[ -n "$bin" ] || die "synod not found in $archive"

mkdir -p "$dest"
cp "$bin" "$dest/synod.new"
chmod 755 "$dest/synod.new"
mv -f "$dest/synod.new" "$dest/synod"

say "Installed synod $tag to $dest/synod"
case ":$PATH:" in
  *":$dest:"*) ;;
  *)
    say "Add it to your PATH:"
    say "  export PATH=\"$dest:\$PATH\""
    ;;
esac
say "Verify with: synod version"
