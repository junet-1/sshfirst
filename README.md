# SSH First

A native-feeling, local-first SSH connection manager for Linux, built with
Wails, Go and Svelte — designed to look and behave like a real KDE Plasma
desktop application (think Dolphin/Kate/Konsole), not a web page in a window.
It uses Wails v3 so management views can live in real independent native
windows while terminal sessions remain alive in the main workspace and tray.
See [`docs/design.md`](docs/design.md) for the UX and architecture rationale.

## Status

This is the first ausbaustufe (build-out stage): it compiles, runs on Arch
Linux, imports `~/.ssh/config`, and opens real interactive SSH terminal
sessions. It also does SFTP browsing/transfers and SSH port forwarding —
local (`-L`), remote (`-R`) and dynamic/SOCKS5 (`-D`) tunnels, defined per
host and toggled per connection (see `internal/forwarding`).

Alongside SSH and SFTP, a host can have a third protocol — **Web** — whose
"connection" is a web control panel (Proxmox, Portainer, Grafana, Nginx Proxy
Manager, …) opened as an embedded browser tab next to the terminals. The panel
is loaded directly from the client (no reverse proxy in between); a toolbar
button falls back to the external browser for panels that refuse framing. An
SSH/SFTP host may additionally carry a companion control-panel URL opened from
its context menu.

## Requirements (Arch Linux / KDE Plasma)

```sh
sudo pacman -S go nodejs npm gtk4 webkitgtk-6.0 base-devel pkgconf
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-alpha.97
```

Make sure `$(go env GOPATH)/bin` is on your `PATH` so the `wails3` binary is
found. The old `wails build` command belongs to Wails v2, looks for
`wails.json`, and cannot build this project anymore.

### Legacy GTK3 fallback

Wails v3 uses GTK4 and WebKitGTK 6.0 by default. On a system that only has the
legacy GTK3/WebKit 4.1 stack, install `gtk3 webkit2gtk-4.1` and build with:

```sh
wails3 build -tags gtk3
```

## Frontend development

Run `npm run dev` in `frontend/` for Vite hot reload. Native integration is
checked with a normal `wails3 build` and the binary under `build/bin/`.

## Building a release binary

```sh
wails3 build
```

The resulting binary is written to `build/bin/ssh-first`.

## Where things live

```text
ssh-first/
├── main.go              # Wails v3 entrypoint: app, native windows, menu and single-instance handling
├── Taskfile.yml         # Wails v3 binding/frontend/Linux build pipeline
├── internal/
│   ├── app/             # Wails-bound App struct: orchestrates everything below, owns the native menu
│   ├── config/          # ~/.ssh/config parsing (Host/Include), read-only
│   ├── storage/         # SQLite (modernc.org/sqlite, CGO-free) host/folder/tag/settings persistence
│   ├── secrets/         # Linux Secret Service (GNOME Keyring / KWallet) via github.com/zalando/go-keyring
│   ├── ssh/             # golang.org/x/crypto/ssh connection handling, auth, known_hosts, ProxyJump
│   ├── terminal/        # PTY session management (one SSH connection → many terminal tabs)
│   ├── platform/        # Linux desktop notifications (D-Bus), behind an OS-agnostic interface
│   ├── sftp/            # SFTP directory browsing + up/downloads
│   ├── forwarding/      # -L/-R/-D port forwarding engine (leak-free teardown)
├── migrations/          # SQL schema migrations, embedded into the binary
├── frontend/            # Svelte + TypeScript (strict) SPA, built with Vite
│   └── src/
│       ├── components/  # Sidebar, tabs, terminal panes, native tool-window views and prompts
│       ├── views/       # Full-area views (e.g. the empty-state WelcomeView)
│       ├── stores/      # Svelte stores per domain: hosts, connections, prompts, ui
│       ├── services/    # backend.ts (typed Wails binding wrapper), i18n.ts (en/de)
│       ├── types/       # Shared TypeScript types mirroring the Go JSON payloads
│       └── styles/      # KDE Breeze-inspired CSS variables, light/dark
└── docs/design.md       # UX + architecture decisions
```

Data (SQLite database and the app's own managed `known_hosts` file — separate
from `~/.ssh/known_hosts`) lives under `$XDG_DATA_HOME/ssh-first` (usually
`~/.local/share/ssh-first`).

## Tests

Go packages with real logic worth testing have unit tests:

```sh
go test ./...
```

Covered: SSH config parsing (including `Include` and wildcard-pattern
handling), SQLite storage CRUD, `ProxyJump` parsing, `known_hosts`
persistence/replacement, and Secret Service read/write/delete (using
`go-keyring`'s in-memory mock, so tests never touch a real keyring).

Frontend type-checking (TypeScript strict mode + Svelte's own diagnostics):

```sh
cd frontend && npm run check
```

## Security notes

- Passwords and private-key passphrases are never written to the SQLite
  database or to logs — they live only in the platform Secret Service.
- Host key verification is mandatory; unknown or changed host keys require
  explicit confirmation through a dialog (changed keys are shown as a
  security warning, not a routine prompt).
- SSH First maintains its own `known_hosts` file rather than touching
  `~/.ssh/known_hosts`, so it can't corrupt your system SSH client's state.
