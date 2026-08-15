package app

import (
	"encoding/json"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"

	"ssh-first/internal/panelview"
	"ssh-first/internal/webautofill"
)

// Web panels are rendered in a real WebKit view stacked over the window rather
// than in an iframe, because every panel worth embedding — Cloudflare, GitHub,
// most SaaS dashboards — refuses to be framed. See internal/panelview.
//
// The frontend owns the layout, so it measures each panel pane and reports the
// rectangle here; this file is the thin, main-thread-safe bridge to the native
// side. Where panel views are unavailable (macOS, the gtk3 fallback, CGO off)
// PanelViewsSupported reports false and the frontend keeps its iframe.

// PanelViewsSupported tells the frontend which rendering path to use.
func (a *App) PanelViewsSupported() bool {
	return panelview.Supported()
}

// InstallPanelViews prepares the main window to host panel views. Safe to call
// repeatedly. It rearranges the window's widget tree, so it must run on the UI
// thread — the composition root already dispatches it there.
//
//wails:ignore
func (a *App) InstallPanelViews(window *application.WebviewWindow) bool {
	if !panelview.Supported() || window == nil {
		return false
	}
	return panelview.Install(window.NativeWindow())
}

// OpenPanelView creates the view for a browser tab and starts loading url. The
// URL is validated exactly like a stored panel URL, so a hostile scheme cannot
// reach a real browsing context either.
func (a *App) OpenPanelView(tabID, url string) error {
	if !panelview.Supported() {
		return fmt.Errorf("panel views are not available on this build")
	}
	if tabID == "" {
		return fmt.Errorf("a tab id is required")
	}
	if err := validatePanelURL(url); err != nil {
		return err
	}
	application.InvokeAsync(func() {
		panelview.Open(tabID, url, webautofill.Script)
	})
	return nil
}

// SetPanelViewBounds positions a panel view over its pane. Coordinates are CSS
// pixels measured in the main window's viewport; the native side scales them.
func (a *App) SetPanelViewBounds(tabID string, x, y, width, height, viewportWidth, viewportHeight int, visible bool) {
	if !panelview.Supported() || tabID == "" {
		return
	}
	bounds := panelview.Bounds{
		X: x, Y: y, Width: width, Height: height,
		ViewportW: viewportWidth, ViewportH: viewportHeight,
		Visible: visible,
	}
	application.InvokeAsync(func() {
		panelview.SetBounds(tabID, bounds)
	})
}

// ClosePanelView destroys a panel view when its tab goes away.
func (a *App) ClosePanelView(tabID string) {
	if !panelview.Supported() || tabID == "" {
		return
	}
	application.InvokeAsync(func() {
		panelview.Close(tabID)
	})
}

// ReloadPanelView reloads a panel's page.
func (a *App) ReloadPanelView(tabID string) {
	if !panelview.Supported() || tabID == "" {
		return
	}
	application.InvokeAsync(func() {
		panelview.Reload(tabID)
	})
}

// AutofillPanelView delivers a web host's saved credentials into a panel view.
//
// A panel view is a top-level document, so the frontend cannot postMessage into
// it the way it does with an iframe. Instead the credentials are handed to the
// bridge script already running in that page — and only that page. They never
// pass through the application's own frontend, which is one fewer place a saved
// password exists at all.
func (a *App) AutofillPanelView(tabID string, hostID int64) error {
	if !panelview.Supported() || tabID == "" {
		return nil
	}
	if err := a.requireStore(); err != nil {
		return err
	}
	password, err := a.WebPassword(hostID)
	if err != nil || password == "" {
		return err
	}
	host, err := a.store.GetHost(hostID)
	if err != nil {
		return err
	}

	message, err := json.Marshal(map[string]string{
		"type":  "ssh-first:web-autofill",
		"email": host.User,
		// The bridge only acts when this matches the page's own origin, so a
		// panel that redirected elsewhere in the meantime gets nothing.
		"targetOrigin": "",
		"password":     password,
	})
	if err != nil {
		return err
	}

	// location.origin is filled in inside the page rather than here: the app
	// knows the configured URL, the page knows where it actually ended up, and
	// the bridge compares the two.
	js := fmt.Sprintf(
		"(() => { const m = %s; m.targetOrigin = location.origin; window.postMessage(m, location.origin); })();",
		string(message),
	)
	application.InvokeAsync(func() {
		panelview.Evaluate(tabID, js)
	})
	return nil
}
