//go:build !(linux && cgo && !gtk3)

package panelview

import "unsafe"

// Panel views are implemented for the WebKitGTK backend only. Everywhere else
// Supported reports false and the frontend keeps using an iframe, which works
// for any panel that does not refuse to be framed. macOS would need the same
// treatment with a WKWebView added as a subview of the window's content view.

func Supported() bool { return false }

func Install(unsafe.Pointer) bool { return false }

func Open(id, uri, script string) {}

func SetBounds(id string, b Bounds) {}

func Close(id string) {}

func Reload(id string) {}

func Evaluate(id, js string) {}

func OnPopup(handler func(Popup)) {}

func OnClosed(handler func(id string)) {}

func OnInfo(handler func(Info)) {}
