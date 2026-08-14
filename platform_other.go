//go:build !linux && !darwin

// Fallback for targets without dedicated support (currently Windows). The app
// builds and runs, but the platform-specific chrome set up in
// platform_linux.go / platform_darwin.go is left at Wails' defaults.

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

func applyPlatformEnv() {}

func applyPlatformWindowOptions(_ *application.App, _ *application.WebviewWindowOptions, _ *application.Menu, _ []byte) {
}
