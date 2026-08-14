//go:build darwin

package platform

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// osascriptNotifier posts notifications through AppleScript's "display
// notification", which reaches Notification Center without any of the
// preconditions UNUserNotificationCenter has: that API raises an exception when
// the process has no bundle identifier, so a plain `go run` build would crash
// instead of merely failing to notify. Notifications are unobtrusive status
// updates here, so reliability beats branding.
type osascriptNotifier struct {
	appName string
}

// NewNotifier returns the macOS notifier.
func NewNotifier(appName string) Notifier {
	return &osascriptNotifier{appName: appName}
}

func (n *osascriptNotifier) Notify(title, body string) error {
	if title == "" {
		title = n.appName
	}
	script := "display notification " + appleScriptString(body) +
		" with title " + appleScriptString(title)

	// osascript occasionally hangs when the user session is locked; a timeout
	// keeps a background transfer from blocking on its completion notice.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "osascript", "-e", script).Run()
}

// appleScriptString quotes s as an AppleScript string literal. AppleScript
// knows no escape sequences other than \" and \\, so anything else — including
// a hostname or an error message pasted in from the server — is safe once those
// two are doubled up.
func appleScriptString(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(s) + `"`
}
