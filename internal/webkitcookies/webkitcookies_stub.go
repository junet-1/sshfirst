//go:build !(linux && cgo && !gtk3)

package webkitcookies

// SetPersistentStorage is a no-op on build configurations without the
// WebKitGTK 6.0 network-session API (CGO disabled, the gtk3 fallback, or
// non-Linux). Cookies then remain in-memory, as before.
func SetPersistentStorage(string) {}
