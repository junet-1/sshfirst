#!/usr/bin/env bash
# Assembles SSH First.app around an already-compiled binary. This is the macOS
# counterpart to stage-tree.sh: one place that decides what a shipped macOS
# build contains, so the .app, a .dmg and a CI artifact can never drift apart.
#
#   packaging/darwin/bundle.sh <version> <path/to/ssh-first> <output-dir>
#
# The bundle is written to <output-dir>/SSH First.app, replacing any previous
# one. Signing is controlled by two optional environment variables:
#
#   SIGN_IDENTITY   codesign identity. Defaults to "-" (ad-hoc), which is what
#                   Apple Silicon needs in order to run a locally built app at
#                   all. A real "Developer ID Application: …" identity
#                   additionally enables the hardened runtime, which is a
#                   precondition for notarisation.
#   ENTITLEMENTS    entitlements plist, defaults to the one next to this script.
#
# Must run on macOS: sips, iconutil and codesign have no Linux equivalents.

set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: $0 <version> <binary> <output-dir>" >&2
  exit 2
fi

version=$1
binary=$2
out=$3
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd "$here/../.." && pwd)

if [[ ! -x $binary ]]; then
  echo "bundle: $binary is not an executable file" >&2
  exit 1
fi
if [[ $(uname -s) != Darwin ]]; then
  echo "bundle: macOS bundles can only be assembled on macOS" >&2
  exit 1
fi

app="$out/SSH First.app"
contents="$app/Contents"
rm -rf "$app"
mkdir -p "$contents/MacOS" "$contents/Resources"

install -m 755 "$binary" "$contents/MacOS/ssh-first"

sed "s|@VERSION@|$version|g" "$here/Info.plist" > "$contents/Info.plist"

# Classic four-byte type/creator record. Modern macOS reads Info.plist instead,
# but a bundle without PkgInfo still trips up the occasional older tool.
printf 'APPL????' > "$contents/PkgInfo"

# .icns from the same 1024px master the Linux packages use. Every size is
# required: Finder, the Dock and Spotlight each pick a different one, and a
# missing entry shows up as the generic application icon.
iconset=$(mktemp -d)/appicon.iconset
mkdir -p "$iconset"
for size in 16 32 128 256 512; do
  sips -z "$size" "$size" "$repo_root/build/appicon.png" \
    --out "$iconset/icon_${size}x${size}.png" >/dev/null
  sips -z $((size * 2)) $((size * 2)) "$repo_root/build/appicon.png" \
    --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil --convert icns "$iconset" --output "$contents/Resources/appicon.icns"
rm -rf "$(dirname "$iconset")"

identity=${SIGN_IDENTITY:--}
entitlements=${ENTITLEMENTS:-$here/entitlements.plist}
sign_args=(--force --timestamp=none --sign "$identity")
if [[ $identity != "-" ]]; then
  # Only a real identity can carry the hardened runtime; an ad-hoc signature
  # with --options runtime is rejected at launch.
  sign_args=(--force --timestamp --options runtime --entitlements "$entitlements" --sign "$identity")
fi

# The executable is signed before the bundle so the outer signature covers the
# final bits of the inner one.
codesign "${sign_args[@]}" "$contents/MacOS/ssh-first"
codesign "${sign_args[@]}" "$app"
codesign --verify --deep --strict "$app"

echo "bundle: wrote $app ($(if [[ $identity == "-" ]]; then echo "ad-hoc signed"; else echo "signed as $identity"; fi))"
