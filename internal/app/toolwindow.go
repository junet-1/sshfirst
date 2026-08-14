package app

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type toolWindowSpec struct {
	title               string
	width, height       int
	minWidth, minHeight int
	resizable           bool
}

// height is only a first-paint estimate: once the dialog mounts it measures its
// content and calls the runtime to fit the window (see Modal.svelte). minHeight
// is kept low so that content-fit shrink is not clamped back up by GTK.
//
// Every window must stay resizable for that to work at all: Wails implements
// SetSize on GTK4 as gtk_window_set_default_size, which a window with
// gtk_window_set_resizable(FALSE) ignores. A fixed window therefore keeps its
// first-paint estimate forever, and any viewport smaller than the estimate
// assumed — a HiDPI webview zoom shrinks it, for instance — leaves the dialog
// scrolling inside a window that refuses to grow.
var toolWindowSpecs = map[string]toolWindowSpec{
	"host":        {title: "Host — SSH First", width: 780, height: 720, minWidth: 620, minHeight: 400, resizable: true},
	"folder":      {title: "Folder — SSH First", width: 440, height: 360, minWidth: 390, minHeight: 240, resizable: true},
	"settings":    {title: "Preferences — SSH First", width: 430, height: 300, minWidth: 390, minHeight: 200, resizable: true},
	"about":       {title: "About SSH First", width: 460, height: 380, minWidth: 420, minHeight: 280, resizable: true},
	"snippets":    {title: "Snippets — SSH First", width: 600, height: 620, minWidth: 500, minHeight: 360, resizable: true},
	"credentials": {title: "Credentials — SSH First", width: 520, height: 560, minWidth: 440, minHeight: 320, resizable: true},
	"transfer":    {title: "Transfer — SSH First", width: 700, height: 680, minWidth: 580, minHeight: 380, resizable: true},
	"forwarding":  {title: "Port Forwarding — SSH First", width: 650, height: 520, minWidth: 540, minHeight: 320, resizable: true},
}

// OpenToolWindow opens or focuses a native top-level window. entityID and
// parentID use -1 for "not set" to keep the generated frontend binding simple.
func (a *App) OpenToolWindow(kind string, entityID, parentID int64) error {
	spec, ok := toolWindowSpecs[kind]
	if !ok {
		return fmt.Errorf("unknown tool window %q", kind)
	}
	if a.ui == nil {
		return fmt.Errorf("application window manager is unavailable")
	}
	if (kind == "transfer" || kind == "forwarding") && entityID < 0 {
		return fmt.Errorf("%s window requires a host", kind)
	}

	key := toolWindowKey(kind, entityID, parentID)
	a.windowMu.Lock()
	if existing := a.toolWindows[key]; existing != nil {
		a.windowMu.Unlock()
		existing.Show().Restore()
		existing.Focus()
		return nil
	}

	query := url.Values{"window": []string{kind}}
	if entityID >= 0 {
		query.Set("id", strconv.FormatInt(entityID, 10))
	}
	if parentID >= 0 {
		query.Set("parent", strconv.FormatInt(parentID, 10))
	}
	options := application.WebviewWindowOptions{
		Name:             "tool-" + key,
		Title:            spec.title,
		Width:            spec.width,
		Height:           spec.height,
		MinWidth:         spec.minWidth,
		MinHeight:        spec.minHeight,
		DisableResize:    !spec.resizable,
		URL:              "/?" + query.Encode(),
		BackgroundColour: application.NewRGB(30, 32, 34),
		Linux: application.LinuxWindow{
			Icon: a.trayIcon,
		},
	}
	// Remember placement per window kind, not per entity, so e.g. the host
	// editor always reopens where it was last, regardless of which host.
	a.RestoreWindowGeometry(&options, kind, false)
	window := a.ui.Window.NewWithOptions(options)
	a.toolWindows[key] = window
	a.windowMu.Unlock()

	a.TrackWindowGeometry(window, kind, false)
	window.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		a.windowMu.Lock()
		delete(a.toolWindows, key)
		a.windowMu.Unlock()
	})
	window.Show().Focus()
	return nil
}

func toolWindowKey(kind string, entityID, parentID int64) string {
	switch kind {
	case "host", "folder", "transfer", "forwarding":
		return fmt.Sprintf("%s-%d-%d", kind, entityID, parentID)
	default:
		return kind
	}
}
