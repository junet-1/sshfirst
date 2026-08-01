package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// windowGeometry is a window's last-known placement. X/Y are absolute
// coordinates in the virtual-desktop space (WebviewWindow.Position), so the
// window returns to the same physical monitor — relative-to-monitor coordinates
// would lose which monitor it was on and reopen on the primary one.
type windowGeometry struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
}

// geometryStore persists window geometries to a small JSON file in the data
// dir. It is deliberately independent of the SQLite settings store so it can be
// read while windows are being created — before ServiceStartup opens the store.
type geometryStore struct {
	mu   sync.Mutex
	path string
	data map[string]windowGeometry
}

func newGeometryStore() *geometryStore {
	gs := &geometryStore{data: map[string]windowGeometry{}}
	dir, err := dataDir()
	if err != nil {
		return gs // persistence disabled; geometry stays in-memory for the session
	}
	gs.path = filepath.Join(dir, "window-geometry.json")
	if raw, err := os.ReadFile(gs.path); err == nil {
		_ = json.Unmarshal(raw, &gs.data)
	}
	return gs
}

func (gs *geometryStore) get(key string) (windowGeometry, bool) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	g, ok := gs.data[key]
	return g, ok
}

func (gs *geometryStore) put(key string, g windowGeometry) {
	gs.mu.Lock()
	gs.data[key] = g
	raw, err := json.MarshalIndent(gs.data, "", "  ")
	path := gs.path
	gs.mu.Unlock()

	if err != nil || path == "" {
		return
	}
	// Write atomically so a crash mid-write can't corrupt the file.
	tmp := path + ".tmp"
	if os.WriteFile(tmp, raw, 0o600) == nil {
		_ = os.Rename(tmp, path)
	}
}

// RestoreWindowGeometry seeds window options with the last-known placement for
// key. Position is always restored; size only when trackSize is set (the main
// window) — tool windows size themselves to their content (see Modal.svelte),
// so restoring a stale height would fight the content-fit.
//
// Positioning only takes effect under X11/XWayland (see the GDK_BACKEND note in
// main.go); on native Wayland the compositor owns placement and this is inert.
//
//wails:ignore
func (a *App) RestoreWindowGeometry(opts *application.WebviewWindowOptions, key string, trackSize bool) {
	g, ok := a.geom.get(key)
	if !ok {
		return
	}
	// Wails only honours an absolute start position when a Screen is provided
	// (it computes WorkArea.X + opts.X). A sentinel screen anchored at the
	// origin makes opts.X/Y absolute, so the window lands on the same physical
	// monitor. Without this, WindowXY is applied relative to whichever monitor
	// the window is created on and always reopens on the primary.
	opts.InitialPosition = application.WindowXY
	opts.Screen = &application.Screen{WorkArea: application.Rect{X: 0, Y: 0}}
	opts.X = g.X
	opts.Y = g.Y
	if trackSize && g.Width > 0 && g.Height > 0 {
		opts.Width = g.Width
		opts.Height = g.Height
	}
}

// TrackWindowGeometry records a window's placement whenever it is moved or
// resized, keyed by key, so RestoreWindowGeometry can bring it back later.
//
//wails:ignore
func (a *App) TrackWindowGeometry(window *application.WebviewWindow, key string, trackSize bool) {
	save := func(*application.WindowEvent) {
		x, y := window.Position()
		g := windowGeometry{X: x, Y: y}
		if trackSize {
			g.Width, g.Height = window.Size()
		}
		a.geom.put(key, g)
	}
	window.OnWindowEvent(events.Common.WindowDidMove, save)
	window.OnWindowEvent(events.Common.WindowDidResize, save)
}
