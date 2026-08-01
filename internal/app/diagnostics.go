package app

import "log"

// LogFrontendError lets the frontend report otherwise-invisible JS errors
// (window.onerror / unhandledrejection, see frontend/src/main.ts) into the
// same log the Go backend writes to, so a crash inside the webview is still
// debuggable from ~/.local/share/ssh-first/ssh-first.log.
func (a *App) LogFrontendError(message string) {
	log.Printf("ssh-first: frontend error: %s", message)
}
