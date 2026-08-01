# SSH First — Design Decisions

Kurzdokument der zentralen UX- und Architekturentscheidungen, festgehalten vor der
Implementierung. Dient als Referenz, nicht als vollständige Spezifikation (siehe
Produkt-Briefing für Details).

## UX-Leitentscheidung

Referenz-Vorbilder: KDE Dolphin (Sidebar + Ansichtsstruktur), Kate/KWrite
(Tab-Leiste, Statusleiste), Konsole (Terminal-Verhalten, Split-Ansicht),
JetBrains IDEs (Toolwindows, kontextbezogene Toolbars), Royal TS
(Host-Baum + Session-Tabs-Kombination).

Bewusst vermieden: Kartenlayouts, große Whitespace-Flächen, Hero-Bereiche,
Gradients, übertriebene Radii, Illustrationen, mobile-first Breakpoints.
Die Anwendung ist **informationsdicht**, nicht minimalistisch im Web-Sinn.

Konkrete Umsetzung:

- **Layout**: klassisches Drei-Spalten-Desktop-Layout — Sidebar (fix, resizbar,
  ~260px) | Hauptbereich mit Tab-Leiste | optionaler Inspector (einblendbar,
  ~280px), kein Card-Grid.
- **Menüleiste** oben (`File`, `Edit`, `View`, `Session`, `Tools`, `Help`),
  darunter eine kompakte, kontextsensitive Toolbar (Icon + optional Label,
  24px Icons, keine 48px-Touch-Buttons).
- **Typografie**: Systemschrift (KDE: Noto Sans o.ä. via `font-family: system-ui`
  Fallback-Kette), Basisgröße 13px UI-Text, 14px Terminal (monospace, konfigurierbar).
  Keine Headline-Skalen wie bei Marketingseiten.
- **Farben**: CSS-Variablen, die KDE-Breeze-Farbnamen spiegeln
  (`--window-bg`, `--view-bg`, `--highlight-bg`, `--text-color`,
  `--disabled-text-color`, `--separator-color`). Light/Dark über
  `prefers-color-scheme` plus manuellen Toggle, später optional echte
  Breeze-Farb-Extraktion via Plasma-Theme-Datei.
- **Radii/Spacing**: 2–3px Radius max (Buttons/Inputs), 4/8px Spacing-Raster,
  1px Separatoren statt Schatten/Card-Borders.
- **States**: sichtbarer 2px Fokusring (`--focus-color`, Breeze-Blau),
  Hover = leichte Hintergrundabdunklung/-aufhellung, Selected = Highlight-Farbe
  mit Kontrast-Text, Disabled = reduzierte Deckkraft + `not-allowed`-Cursor.
- **Interaktion**: Doppelklick verbindet, Rechtsklick öffnet Kontextmenü,
  Drag&Drop verschiebt Hosts zwischen Ordnern, kein modales Web-Overlay für
  Standarddialoge (Datei-Auswahl, native Wails-Dialoge wo möglich).
- **Terminal**: xterm.js ohne sichtbaren Chrome-Rahmen um das Grid, eigene
  Tab-Leiste pro Fenster, Split horizontal/vertikal über CSS-Grid-Panes,
  keine Terminal-"Card".

## Architekturentscheidung

Schichtenmodell, strikt getrennt (siehe Zielstruktur in `internal/`):

- **`internal/config`**: Nur Lesen/Parsen von `~/.ssh/config` (inkl. `Include`),
  reine Funktion ohne Storage-Kenntnis. Ergebnis wird explizit in den Host-Store
  importiert (kein Live-Sync/Mutation der Datei in der ersten Ausbaustufe).
- **`internal/storage`**: SQLite (via `mattn/go-sqlite3` oder `modernc.org/sqlite`
  für CGO-freien Build — Entscheidung: `modernc.org/sqlite`, da CGO-frei besser
  für reproduzierbare Arch-Builds und Cross-Compilation). Migrationsbasiert
  (`migrations/*.sql`, eingebettet via `embed.FS`). Enthält **niemals** Secrets
  oder private Keys — nur Metadaten, Pfade, Referenzen auf Secret-Service-Einträge.
- **`internal/secrets`**: D-Bus Secret Service Client (org.freedesktop.secrets),
  kompatibel zu KWallet- und GNOME-Keyring-Implementierungen. Passwörter und
  Key-Passphrasen werden ausschließlich hier gespeichert/gelesen.
- **`internal/ssh`**: Verbindungsaufbau (`golang.org/x/crypto/ssh`), Auth-Strategien
  (Agent zuerst, dann Identity File, dann Passwort-Callback → Frontend-Dialog),
  Host-Key-Verifizierung gegen `known_hosts` mit expliziter Erstkontakt-/
  Änderungs-Bestätigung über das Frontend, ProxyJump-Ketten.
- **`internal/terminal`**: PTY-Session-Verwaltung pro Tab (Request PTY, Shell,
  Resize, I/O-Streaming), unabhängig von Verbindungslogik in `internal/ssh`
  (eine SSH-Connection kann mehrere Terminal-Sessions/Channels haben).
- **`internal/sftp`** / **`internal/forwarding`**: eigene Bounded Contexts,
  MVP-Grundgerüst, volle Umsetzung folgt nach dem Terminal-MVP.
- **`internal/platform`**: Linux-spezifische Integrationen (Notifications,
  Systemfarben-Erkennung), als Interface gefasst, damit spätere Windows/macOS-
  Implementierungen ohne Umbau ergänzt werden können.
- **`internal/app`**: Wails-Bindings/Bootstrapping, orchestriert obige Pakete,
  hält kein eigenes Business-Logic.

Kein separates `cmd/`-Verzeichnis: Wails erwartet `main.go` im Modul-Root
(die Build-Pipeline generiert Bindings gegen genau dieses Package), daher
bleibt `main.go` dort als reiner Bootstrap-Layer, der direkt in `internal/app`
delegiert.

Kommunikation Go ↔ Svelte: Wails-generierte Bindings für Request/Response
(Host-CRUD, Config-Import, Connect/Disconnect), Wails-Events für
Terminal-Datenstrom (`terminal:data`, `terminal:closed`, payload trägt die
`tabId`) und Statusänderungen (`connection:status`, payload trägt die
`connectionId`), um Polling zu vermeiden.

Frontend-State: Svelte-Stores pro Domäne (`hosts`, `sessions`, `ui`, `settings`),
keine globale Monolith-Store. Services-Schicht (`frontend/src/services`) kapselt
alle Wails-Bindings-Aufrufe, Komponenten rufen nie direkt `window.go.*` auf.

## Nachtrag: native Menüleiste statt HTML-Menü

Die Menüleiste (`File`, `Edit`, `View`, `Session`, `Tools`, `Help`) wird
**nicht** als HTML/Svelte-Komponente nachgebaut, sondern über Wails'
`pkg/menu`-API als echtes natives GTK-Menü erzeugt (`internal/app/menu.go`,
`BuildMenu()`). Jeder Menüpunkt emittiert ein `menu:action`-Event, das die
Svelte-Seite behandelt — Go kann keine Dialoge rendern, daher bleibt jegliche
UI-Logik im Frontend, Go liefert nur die native Menüstruktur.

Bewusst **keine** OS-Level-Tastenkürzel (`keys.Accelerator`) auf Menüpunkten,
die auch im Produkt-Briefing als Shortcut gefordert sind (`Ctrl+N`, `Ctrl+T`,
`Ctrl+W`, `Ctrl+Shift+W`, `Ctrl+Shift+P`, `Ctrl+,`). Diese Tastenkombinationen
sind zugleich verbreitete Readline-/Shell-Shortcuts (`Ctrl+W` löscht in
bash/readline das letzte Wort); ein GTK-Accelerator würde sie global abfangen,
bevor sie das Terminal erreichen. Einzige Quelle der Wahrheit für diese
Shortcuts ist deshalb der globale `keydown`-Capture-Handler in `App.svelte`.
Für rein native Dialoge ohne Custom-Layout-Bedarf (Lösch-Bestätigung und
Dateiauswahl) werden Wails' native `MessageDialog`/`OpenFileDialog`-APIs
verwendet (`internal/app/dialogs.go`). Host, Ordner, Settings, About,
Snippets, Transfer und Port-Forwarding sind eigene Wails-v3-Fenster. Nur
terminalgebundene Sicherheits-Prompts bleiben über dem Terminal.

## Nachtrag: wiederverwendbare Credentials (Hybrid-Modell)

Anmeldedaten (Benutzer + Auth-Methode + Identity-Files) sind ein eigenständiges
Element (`credentials`-Tabelle), das viele Hosts referenzieren können — einmal
anlegen, überall nutzen. Bewusst **hybrid**: ein Host referenziert eine
Credential *optional* (`hosts.credential_id`, nullable). Ist sie gesetzt, erbt
der Host User/Auth/Identity; ist sie leer, gelten wie bisher die Inline-Felder
am Host. Vorteil: **keine Datenmigration** — bestehende Hosts bleiben inline,
beide Wege koexistieren.

Secrets folgen dem Referenz-Ziel: Passwort eines Credential-Hosts liegt unter
`credential-password:<credID>` (geteilt), eines Inline-Hosts weiter unter
`host-password:<hostID>`. Aufgelöst wird an genau einer Stelle: `resolveAuth`
(`internal/app/credentials.go`) liefert die effektiven Felder + den passenden
Secret-Key; `dial` und `providePassword` nutzen nur noch diese. Der
`internal/ssh`-Layer bleibt unverändert. Löschen einer referenzierten
Credential setzt die Host-Referenzen via `ON DELETE SET NULL` zurück (Host
fällt auf inline zurück, keine Host-Löschung) — die UI warnt vorher mit der
Anzahl betroffener Hosts. Verwaltung als eigenes Wails-Fenster
(`CredentialsDialog`), Auswahl per Dropdown im Host-Editor.

## Nachtrag: WebKit-JIT-Workaround (Stabilität)

WebKitGTKs JavaScriptCore-JIT crasht den **Web-Content-Prozess** (separater
`WebKitWebProcess`, nicht der Hauptprozess) nach einigen Minuten beim Rendern
mancher Panel-SPAs — Heap-Korruption/`SIGABRT`. Diagnose: gdb am Hauptprozess
zeigte ihn stabil, während ein frischer `WebKitWebProcess` neben dem alten
auftauchte (Absturz + Respawn). Kein eigener Code im crashenden Prozess.
Workaround: `JSC_useJIT=0` wird in `main()` gesetzt (vor jeder WebKit-Init,
damit der Web-Prozess es erbt; nur wenn der Nutzer nichts vorgibt). Kostet
etwas JS-Tempo, für Admin-Panels irrelevant. Reproduziert wurde der Crash mit
offenem Web-Panel-Tab; GPU-/DMABUF-Workarounds halfen nicht, JIT-Deaktivierung
schon.

## Arch-Linux-Build

Wails v3 verwendet standardmäßig GTK4 und WebKitGTK 6.0. Der alte
GTK3/WebKit-4.1-Pfad ist nur noch als Fallback über den Build-Tag `gtk3`
vorhanden; Details stehen in `README.md`.

## MVP-Scope dieser Ausbaustufe

Enthalten: Config-Import, Host-Liste, manuelles Hinzufügen/Bearbeiten/Löschen,
Verbindung per Doppelklick, xterm.js-Terminal mit mehreren Tabs, SSH-Agent,
Identity Files, Passwort-Dialog, Secret-Service-Speicherung, known_hosts-Prüfung,
Reconnect/Disconnect, Favoriten, Ordner/Tags, Suche, Light/Dark, Shortcuts.

Zusätzlich umgesetzt: SFTP-Dateibrowser, Port-Forwarding, Split-Terminals,
Command Palette, persistentes System-Tray und separate Management-Fenster.

Nachtrag: Web als dritter Verbindungstyp. Neben `ssh` und `sftp` gibt es das
Host-Protokoll `web` (`HostProtocolWeb`). Ein Web-Host wird nicht über einen
SSH-Transport „verbunden", sondern durch seine `ControlPanelURL` adressiert:
Doppelklick/Connect öffnet einen eingebetteten Browser-Tab (`<iframe>`) neben
den Terminals — überall über den einen Router `openHost()` im Connections-
Store, damit die drei Protokolle symmetrisch bleiben. Die Validierung ist pro
Protokoll unterschiedlich (`validateHostInput`): ssh/sftp brauchen einen
Hostnamen, web braucht eine URL; SSH-/SFTP-only-Felder werden beim Speichern
eines Web-Hosts verworfen (`normalizeHostInput`).

Das Panel wird **direkt vom Client** geladen (kein Reverse-Proxy in der App).
Da der iframe cross-origin ist, erreicht das Panel per Same-Origin-Policy die
Wails-Bindings nicht — die Isolation vom SSH-Backend ergibt sich von selbst.
Für Panels, die Einbettung verweigern (`X-Frame-Options`/`frame-ancestors`),
gibt es einen „Im externen Browser öffnen"-Fallback in der Tab-Toolbar. Ein
ssh/sftp-Host kann zusätzlich eine begleitende `ControlPanelURL` tragen, die
über sein Kontextmenü als Panel-Tab geöffnet wird.

Session-Persistenz: Wails legt jede WebviewWindow gegen die WebKit-Default-
Network-Session an, aktiviert dort aber keine On-Disk-Cookies — ein Panel-Login
wäre nach Neustart weg. `internal/webkitcookies` (CGO, `webkitgtk-6.0`) hängt
daher beim Start einmalig einen persistenten SQLite-Cookie-Store an dieselbe
Default-Session (`cookies.sqlite` neben der DB im Data-Dir) und setzt
`ACCEPT_ALWAYS`, weil die Panels als cross-origin-iframes laufen und ihre
Cookies aus Sicht des `wails://`-Top-Frames Third-Party sind. Der Aufruf läuft
über `WindowRuntimeReady` + `application.InvokeAsync` auf dem GTK-Main-Thread;
Nicht-CGO-/gtk3-Builds bekommen einen No-op-Stub. Favicon des Panels als
Tab-Icon (`/favicon.ico` direkt von der Panel-Origin, Globus als Fallback) —
kein externer Favicon-Dienst, damit interne Hostnamen nicht nach außen gehen.
