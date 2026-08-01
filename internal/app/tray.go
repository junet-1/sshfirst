package app

// trayService keeps the platform-specific status icon isolated from the
// application lifecycle. Unsupported desktops return false from Start, so a
// window close still exits instead of leaving an unreachable background app.
type trayService interface {
	Start() bool
	Refresh()
	Stop()
}

func (a *App) startTray() {
	a.tray = newTrayService(a, a.trayIcon)
	if a.tray != nil {
		a.trayActive.Store(a.tray.Start())
	}
}

func (a *App) refreshTray() {
	if a.trayActive.Load() && a.tray != nil {
		a.tray.Refresh()
	}
}

func (a *App) stopTray() {
	if !a.trayActive.Swap(false) || a.tray == nil {
		return
	}
	a.tray.Stop()
}
