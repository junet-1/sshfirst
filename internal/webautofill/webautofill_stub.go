//go:build !(linux && cgo && !gtk3)

package webautofill

import "unsafe"

// Install is currently implemented by the WebKitGTK build. Other platforms
// keep the embedded browser unchanged until their native frame-injection APIs
// are wired up.
func Install(unsafe.Pointer) {}
