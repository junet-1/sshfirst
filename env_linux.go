//go:build linux

package main

import (
	"os"
	"strings"
	"syscall"
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
