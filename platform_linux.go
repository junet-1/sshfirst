//go:build linux

// Everything in this file is specific to the GTK4 + WebKitGTK backend Wails
// uses on Linux. platform_darwin.go and platform_other.go carry the
// equivalents for the other targets.

package main

import (
	"os"
	"strings"
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Force the GTK/WebKit backend to X11 (XWayland) on Wayland sessions.
//
// Native Wayland has a long-standing Wails/GTK bug where the window cannot be
// maximised (it stays stuck at roughly its initial size) or resized beyond a
// "phantom" maximum that depends on the active GTK theme — see
// https://github.com/wailsapp/wails/issues/2431. Running under XWayland avoids
// it entirely with identical rendering.
//
// Two things make this fiddly:
//   - Setting GDK_BACKEND from Go with os.Setenv is too late (under cgo, GTK
//     reads it before the value reaches the C environment), so we re-exec the
//     process with the variable already in its initial environment.
//   - KDE/GNOME Wayland sessions usually export GDK_BACKEND=wayland already,
//     and getenv() honours the FIRST occurrence, so appending is not enough —
//     the existing entry must be stripped and replaced.
//
// Set SSHFIRST_WAYLAND=1 to opt out and run natively on Wayland.
func init() {
	if os.Getenv("SSHFIRST_WAYLAND") != "" {
		return // explicit opt-in to native Wayland
	}
	if os.Getenv("SSHFIRST_XWAYLAND_REEXEC") == "1" {
		return // this is the re-exec'd child; don't loop
	}
	onWayland := os.Getenv("WAYLAND_DISPLAY") != "" || strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "wayland")
	if !onWayland {
		return // not a Wayland session — nothing to work around
	}

	exe, err := os.Executable()
	if err != nil {
		return // can't re-exec; fall through and run as-is
	}

	// Rebuild the environment with any existing GDK_BACKEND removed, then pin
	// it to x11 so getenv() resolves to XWayland.
	env := make([]string, 0, len(os.Environ())+2)
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "GDK_BACKEND=") {
			continue
		}
		env = append(env, e)
	}
	env = append(env, "GDK_BACKEND=x11", "SSHFIRST_XWAYLAND_REEXEC=1")

	// Replaces the current process image; on success this never returns.
	_ = syscall.Exec(exe, os.Args, env)
}

// applyPlatformEnv sets the GTK/WebKit environment the app expects. It must run
// before any WebKit initialisation, i.e. first thing in main.
func applyPlatformEnv() {
	// Wayland forbids a client from setting its own toplevel position, so the
	// window-geometry restore would be a no-op there and the compositor would
	// place windows wherever it likes. Running under XWayland (like most
	// browsers do) lets us restore the exact last position ourselves. Only
	// forced when the user has not picked a backend explicitly.
	if os.Getenv("GDK_BACKEND") == "" {
		_ = os.Setenv("GDK_BACKEND", "x11")
	}

	// WebKitGTK's JavaScriptCore JIT crashes the web-content process (heap
	// corruption / SIGABRT) after a few minutes of rendering some panel SPAs.
	// Disabling the JIT trades a little JS throughput — irrelevant for admin
	// panels — for stability. Set before any WebKit init so the web process
	// inherits it; only forced when the user hasn't chosen, so JSC_useJIT=1
	// can re-enable it for testing.
	if os.Getenv("JSC_useJIT") == "" {
		_ = os.Setenv("JSC_useJIT", "0")
	}
}

// applyPlatformWindowOptions attaches the platform's window chrome to the main
// window. GTK menus belong to the window itself, so the menu is passed through
// the Linux window options rather than being installed application-wide.
func applyPlatformWindowOptions(_ *application.App, opts *application.WebviewWindowOptions, menu *application.Menu, icon []byte) {
	opts.Linux = application.LinuxWindow{
		Icon:      icon,
		Menu:      menu,
		MenuStyle: application.LinuxMenuStyleMenuBar,
	}
}
