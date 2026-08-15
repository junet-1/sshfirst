package app

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

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

// panelOrigin reduces a stored panel URL to the origin a credential may be
// delivered to.
func panelOrigin(raw string) (string, error) {
	if err := validatePanelURL(raw); err != nil {
		return "", err
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

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

	// The origin the password belongs to, taken from the host's configured URL —
	// never from the page. A panel can navigate: an SSO redirect, a link, or a
	// hostile redirect all change location.origin, and a credential must not
	// follow it there. The iframe path got this from postMessage's targetOrigin,
	// which the engine enforces; injecting into a top-level document has to
	// state the expectation explicitly instead.
	origin, err := panelOrigin(host.ControlPanelURL)
	if err != nil {
		return err
	}

	message, err := json.Marshal(map[string]string{
		"type":         "ssh-first:web-autofill",
		"email":        host.User,
		"targetOrigin": origin,
		"password":     password,
	})
	if err != nil {
		return err
	}
	expected, err := json.Marshal(origin)
	if err != nil {
		return err
	}

	// Checked twice on purpose: the guard skips the injection when the page has
	// moved on, and postMessage's second argument makes the engine drop the
	// message anyway if it somehow did not.
	js := fmt.Sprintf(
		"(() => { const expected = %s; if (location.origin !== expected) return; window.postMessage(%s, expected); })();",
		string(expected), string(message),
	)
	application.InvokeAsync(func() {
		panelview.Evaluate(tabID, js)
	})
	return nil
}
