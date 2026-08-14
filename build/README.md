# Build Directory

Build inputs and outputs:

* `appicon.png` — the 1024px icon master every platform's icon is derived from
* `bin` — output directory for `task build` and friends
* `windows` — leftover Wails v2 template files for Windows, unused by the
  current Taskfile

## Mac

macOS packaging lives in `packaging/darwin`, alongside the Linux packaging, and
is driven by `task darwin:package`:

- `packaging/darwin/Info.plist` — bundle metadata
- `packaging/darwin/entitlements.plist` — used when signing with a Developer ID
- `packaging/darwin/bundle.sh` — assembles `build/bin/SSH First.app`, including
  the `.icns` generated from `appicon.png`

The `build/darwin` directory this project started with held Wails v2 templates
(`{{.Info.ProductName}}` placeholders) that the v3 toolchain never reads; they
were removed rather than left to rot.

## Windows

The `windows` directory contains the manifest and rc files from the original
Wails v2 template. Nothing in the current build path consumes them — Windows is
not a supported target yet.

- `icon.ico` - The icon used for the application.
- `installer/*` - The files used to create the Windows installer.
- `info.json` - Application details used for Windows builds.
- `wails.exe.manifest` - The main application manifest file.
