//go:build darwin

// macOS counterpart to platform_linux.go. The WKWebView backend needs none of
// the GTK/Wayland workarounds, but the menu bar is owned by the application
// rather than by a window.

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// applyPlatformEnv is a no-op on macOS: there is no GDK backend to pin and
// WKWebView's JavaScriptCore does not need the JIT workaround the WebKitGTK
// web-content process does.
func applyPlatformEnv() {}

// applyPlatformWindowOptions installs the menu bar. On macOS a single menu is
// shared by the whole application (including tool windows), so it is set on the
// app instead of on each window.
func applyPlatformWindowOptions(app *application.App, opts *application.WebviewWindowOptions, menu *application.Menu, _ []byte) {
	app.Menu.SetApplicationMenu(menu)

	// A plain, opaque title bar. The window keeps the standard traffic lights;
	// only Esc handling is adjusted so the frontend's own modals can close on
	// Esc while the window is fullscreen.
	opts.Mac = application.MacWindow{
		Backdrop:                     application.MacBackdropNormal,
		TitleBar:                     application.MacTitleBarDefault,
		DisableEscapeExitsFullscreen: true,
	}
}
