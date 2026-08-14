//go:build !(linux && cgo && !gtk3) && !(darwin && cgo)

package webautofill

import "unsafe"

// Install is implemented by the WebKitGTK and WKWebView builds. Other build
// configurations (CGO disabled, the gtk3 fallback, Windows) keep the embedded
// browser unchanged until their native frame-injection APIs are wired up.
func Install(unsafe.Pointer) {}
