//go:build !(linux && cgo && !gtk3)

package webkitcookies

// SetPersistentStorage is a no-op on build configurations without the
// WebKitGTK 6.0 network-session API (CGO disabled, the gtk3 fallback, or
// non-Linux). Cookies then remain in-memory, as before.
//
// macOS needs nothing here: WKWebView's default WKWebsiteDataStore already
// persists cookies to disk inside the app's container, which is exactly what
// this call arranges on WebKitGTK.
func SetPersistentStorage(string) {}
