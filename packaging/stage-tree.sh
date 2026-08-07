#!/usr/bin/env bash
# Builds the canonical install tree that every package format is derived from.
# Keeping this in one place means the .deb, the .rpm, the tarball and the
# Flatpak bundle can never drift apart in what they ship or where they put it.
#
#   packaging/stage-tree.sh <version> <path/to/ssh-first> <output-dir>
#
# The output dir is created and must not already exist.

set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: $0 <version> <binary> <output-dir>" >&2
  exit 2
fi

version=$1
binary=$2
out=$3
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

if [[ ! -x $binary ]]; then
  echo "stage-tree: $binary is not an executable file" >&2
  exit 1
fi
if [[ -e $out ]]; then
  echo "stage-tree: $out already exists" >&2
  exit 1
fi

install -Dm755 "$binary" "$out/bin/ssh-first"

# The desktop file is installed under the GTK application ID that Wails derives
# from LinuxOptions.ProgramName ("ssh-first" -> org.wails.ssh_first). Desktop
# shells match a toplevel window to its .desktop entry by that ID, so renaming
# it here costs you the taskbar icon.
install -Dm644 "$repo_root/packaging/ssh-first.desktop" \
  "$out/share/applications/org.wails.ssh_first.desktop"

# Autostart entry: starts into the tray on login instead of opening a window.
install -Dm644 "$repo_root/packaging/ssh-first-autostart.desktop" \
  "$out/share/autostart/ssh-first.desktop"

# pixmaps is the guaranteed fallback for the Icon=ssh-first name; hicolor
# carries the full-resolution icon for HiDPI themes.
install -Dm644 "$repo_root/build/appicon.png" \
  "$out/share/icons/hicolor/1024x1024/apps/ssh-first.png"
install -Dm644 "$repo_root/build/appicon.png" \
  "$out/share/pixmaps/ssh-first.png"

install -Dm644 "$repo_root/LICENSE" "$out/LICENSE"
install -Dm644 "$repo_root/README.md" "$out/README.md"

# AppStream metadata, with the <releases> block rewritten to the version being
# built so software centres show the right number.
python3 - "$repo_root/packaging/org.wails.ssh_first.metainfo.xml" \
         "$out/share/metainfo/org.wails.ssh_first.metainfo.xml" \
         "$version" "$(date -u +%Y-%m-%d)" <<'PY'
import os, re, sys

src, dst, version, today = sys.argv[1:5]
with open(src, encoding="utf-8") as handle:
    doc = handle.read()

release = (
    f'  <releases>\n'
    f'    <release version="{version}" date="{today}">\n'
    f'      <url type="details">https://github.com/junet-1/sshfirst/releases/tag/v{version}</url>\n'
    f'    </release>\n'
    f'  </releases>'
)
doc, count = re.subn(r"  <releases>.*?</releases>", release, doc, flags=re.S)
if count != 1:
    sys.exit(f"metainfo: expected exactly one <releases> block, found {count}")

os.makedirs(os.path.dirname(dst), exist_ok=True)
with open(dst, "w", encoding="utf-8") as handle:
    handle.write(doc)
PY

# Installer for the plain tarball. Packages ignore it; rpmbuild leaving it
# unpackaged in the build dir is harmless.
cat > "$out/install.sh" <<'SH'
#!/usr/bin/env sh
# Installs SSH First from this tarball. Override the target with PREFIX,
# for example: PREFIX=$HOME/.local ./install.sh
set -eu

PREFIX=${PREFIX:-/usr/local}
here=$(cd "$(dirname "$0")" && pwd)

echo "Installing SSH First to $PREFIX"
install -Dm755 "$here/bin/ssh-first" "$PREFIX/bin/ssh-first"
install -Dm644 "$here/share/applications/org.wails.ssh_first.desktop" \
  "$PREFIX/share/applications/org.wails.ssh_first.desktop"
install -Dm644 "$here/share/metainfo/org.wails.ssh_first.metainfo.xml" \
  "$PREFIX/share/metainfo/org.wails.ssh_first.metainfo.xml"
install -Dm644 "$here/share/icons/hicolor/1024x1024/apps/ssh-first.png" \
  "$PREFIX/share/icons/hicolor/1024x1024/apps/ssh-first.png"
install -Dm644 "$here/share/pixmaps/ssh-first.png" \
  "$PREFIX/share/pixmaps/ssh-first.png"

echo
echo "Done. SSH First needs GTK 4 and WebKitGTK 6.0 from your distribution."
echo "To start in the tray on login, copy share/autostart/ssh-first.desktop"
echo "to ~/.config/autostart/."
SH
chmod 755 "$out/install.sh"

echo "stage-tree: staged $version in $out"
