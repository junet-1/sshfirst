//go:build linux

package platform

import "github.com/godbus/dbus/v5"

// dbusNotifier sends desktop notifications via the freedesktop.org
// Notifications D-Bus interface, which KDE Plasma, GNOME and most other
// Linux desktops implement natively.
type dbusNotifier struct {
	appName string
}

// NewNotifier returns the Linux D-Bus notifier.
func NewNotifier(appName string) Notifier {
	return &dbusNotifier{appName: appName}
}

func (n *dbusNotifier) Notify(title, body string) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return err
	}
	obj := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	call := obj.Call("org.freedesktop.Notifications.Notify", 0,
		n.appName, // app_name
		uint32(0), // replaces_id
		"",        // app_icon
		title,
		body,
		[]string{},                // actions
		map[string]dbus.Variant{}, // hints
		int32(5000),               // expire_timeout (ms)
	)
	return call.Err
}
