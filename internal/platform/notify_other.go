//go:build !linux

package platform

// noopNotifier is the placeholder for platforms without a native
// notification integration yet (Windows/macOS support is planned but not
// implemented — see docs/design.md).
type noopNotifier struct{}

// NewNotifier returns a no-op notifier on non-Linux platforms.
func NewNotifier(appName string) Notifier {
	return &noopNotifier{}
}

func (noopNotifier) Notify(title, body string) error {
	return nil
}
