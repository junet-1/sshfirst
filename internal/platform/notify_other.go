//go:build !linux && !darwin

package platform

// noopNotifier is the placeholder for platforms without a native
// notification integration yet (Windows support is planned but not
// implemented).
type noopNotifier struct{}

// NewNotifier returns a no-op notifier on platforms without an integration.
func NewNotifier(appName string) Notifier {
	return &noopNotifier{}
}

func (noopNotifier) Notify(title, body string) error {
	return nil
}
