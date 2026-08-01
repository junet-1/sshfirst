// Package platform isolates OS-specific integrations (desktop notifications,
// system theme detection) behind small interfaces so Linux-first
// implementations can be joined by Windows/macOS ones later without touching
// call sites in internal/app.
package platform

// Notifier delivers a desktop notification through the host system's native
// notification center (e.g. KDE Plasma's notification popups via D-Bus).
type Notifier interface {
	Notify(title, body string) error
}
