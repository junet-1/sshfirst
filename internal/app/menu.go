package app

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// BuildMenu constructs the native application menu. Frontend-owned actions
// are emitted with stable IDs so the same handlers serve toolbar, shortcuts,
// tray entries and the menu bar.
//
// The layout follows each platform's conventions: macOS expects About,
// Preferences and Quit in the application menu (the first submenu, which AppKit
// renders under the app's own name) rather than in File.
//
//wails:ignore
func (a *App) BuildMenu() *application.Menu {
	m := a.ui.NewMenu()
	mac := runtime.GOOS == "darwin"

	if mac {
		// Built by hand rather than with application.AppMenu so that About and
		// Preferences open SSH First's own windows instead of AppKit's generic
		// about panel.
		appMenu := m.AddSubmenu("SSH First")
		appMenu.Add("About SSH First").OnClick(a.menuAction("help.about"))
		appMenu.AddSeparator()
		appMenu.Add("Settings…").SetAccelerator("CmdOrCtrl+,").OnClick(a.menuAction("file.settings"))
		appMenu.AddSeparator()
		appMenu.AddRole(application.ServicesMenu)
		appMenu.AddSeparator()
		appMenu.AddRole(application.Hide)
		appMenu.AddRole(application.HideOthers)
		appMenu.AddRole(application.UnHide)
		appMenu.AddSeparator()
		// Not application.Quit: the app's own Quit tears down sessions and the
		// tray before terminating.
		appMenu.Add("Quit SSH First").SetAccelerator("CmdOrCtrl+Q").OnClick(func(*application.Context) { a.Quit() })
	}

	file := m.AddSubmenu("File")
	file.Add("New Host…").OnClick(a.menuAction("file.newHost"))
	file.Add("Import ~/.ssh/config").OnClick(a.menuAction("file.importConfig"))
	file.Add("Workspaces…").OnClick(a.menuAction("file.workspaces"))
	if !mac {
		file.AddSeparator()
		file.Add("Settings…").OnClick(a.menuAction("file.settings"))
		file.AddSeparator()
		file.Add("Quit").SetAccelerator("CmdOrCtrl+Q").OnClick(func(*application.Context) { a.Quit() })
	}

	edit := m.AddSubmenu("Edit")
	if mac {
		// WKWebView routes ⌘X/⌘C/⌘V through the responder chain, so the
		// standard roles must be present in the menu bar or the shortcuts do
		// nothing inside the app's own UI.
		edit.AddRole(application.Undo)
		edit.AddRole(application.Redo)
		edit.AddSeparator()
		edit.AddRole(application.Cut)
		edit.AddRole(application.Copy)
		edit.AddRole(application.Paste)
		edit.AddRole(application.SelectAll)
		edit.AddSeparator()
	}
	edit.Add("Edit Host…").OnClick(a.menuAction("edit.editHost"))
	edit.Add("Delete Host").OnClick(a.menuAction("edit.deleteHost"))
	edit.Add("Rename").OnClick(a.menuAction("edit.rename"))
	edit.AddSeparator()
	edit.Add("Find in Terminal…").OnClick(a.menuAction("edit.findTerminal"))

	view := m.AddSubmenu("View")
	view.AddCheckbox("Show Sidebar", true).OnClick(a.menuAction("view.toggleSidebar"))
	view.AddCheckbox("Show Inspector", false).OnClick(a.menuAction("view.toggleInspector"))
	view.AddCheckbox("Show Recent", true).OnClick(a.menuAction("view.toggleRecent"))
	view.AddSeparator()
	view.Add("Maximize Window").OnClick(a.menuAction("view.maximize"))
	fullscreenAccelerator := "F11"
	if mac {
		fullscreenAccelerator = "Ctrl+Cmd+F"
	}
	view.Add("Toggle Fullscreen").SetAccelerator(fullscreenAccelerator).OnClick(a.menuAction("view.fullscreen"))

	session := m.AddSubmenu("Session")
	session.Add("Connect").OnClick(a.menuAction("session.connect"))
	session.Add("Disconnect").OnClick(a.menuAction("session.disconnect"))
	session.Add("Reconnect").OnClick(a.menuAction("session.reconnect"))
	session.AddSeparator()
	session.Add("New Tab").OnClick(a.menuAction("session.newTab"))
	session.Add("Duplicate Tab").OnClick(a.menuAction("session.duplicateTab"))
	session.Add("Reopen Closed Tab").OnClick(a.menuAction("session.reopenClosedTab"))
	session.Add("Close Tab").OnClick(a.menuAction("session.closeTab"))
	session.AddSeparator()
	session.Add("Copy Connection Command").OnClick(a.menuAction("session.copySSHCommand"))
	session.AddSeparator()
	session.Add("Broadcast Input to All Tabs").OnClick(a.menuAction("session.broadcast"))

	tools := m.AddSubmenu("Tools")
	tools.Add("Command Palette…").OnClick(a.menuAction("tools.commandPalette"))
	tools.Add("Snippets…").OnClick(a.menuAction("tools.snippets"))
	tools.Add("Credentials…").OnClick(a.menuAction("tools.credentials"))

	if mac {
		// Minimise/Zoom/Bring All to Front, which macOS users expect to find.
		m.AddRole(application.WindowMenu)
	}

	if !mac {
		// On macOS this menu would hold nothing but About, which already lives
		// in the application menu.
		help := m.AddSubmenu("Help")
		help.Add("About SSH First").OnClick(a.menuAction("help.about"))
	}
	return m
}

func (a *App) menuAction(id string) func(*application.Context) {
	return func(*application.Context) { a.emit("menu:action", id) }
}
