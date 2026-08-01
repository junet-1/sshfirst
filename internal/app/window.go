package app

// ShowMainWindow restores the existing WebView without rebuilding the
// frontend. Open terminals therefore keep their scrollback and session state.
//
//wails:ignore
func (a *App) ShowMainWindow() {
	if a.mainWindow == nil {
		return
	}
	a.mainWindow.Show().Restore()
	a.mainWindow.Focus()
}

// HideMainWindowOnClose is used by the main window's closing hook. Tool
// windows close normally; only the terminal workspace persists in the tray.
//
//wails:ignore
func (a *App) HideMainWindowOnClose() bool {
	return !a.quitting.Load() && a.trayActive.Load()
}

// Quit is the one intentional process-exit path shared by native and tray
// menus.
//
//wails:ignore
func (a *App) Quit() {
	a.quitting.Store(true)
	if a.ui != nil {
		a.ui.Quit()
	}
}

func (a *App) ToggleFullscreen() bool {
	if a.mainWindow == nil {
		return false
	}
	if a.mainWindow.IsFullscreen() {
		a.mainWindow.UnFullscreen()
		return false
	}
	a.mainWindow.Fullscreen()
	return true
}

func (a *App) IsFullscreen() bool {
	return a.mainWindow != nil && a.mainWindow.IsFullscreen()
}

func (a *App) ToggleMaximise() bool {
	if a.mainWindow == nil {
		return false
	}
	if a.mainWindow.IsMaximised() {
		a.mainWindow.UnMaximise()
		return false
	}
	a.mainWindow.Maximise()
	return true
}
