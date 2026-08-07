package app

import "github.com/wailsapp/wails/v3/pkg/application"

// BuildMenu constructs the native application menu. Frontend-owned actions
// are emitted with stable IDs so the same handlers serve toolbar, shortcuts,
// tray entries and the menu bar.
//
//wails:ignore
func (a *App) BuildMenu() *application.Menu {
	m := a.ui.NewMenu()

	file := m.AddSubmenu("File")
	file.Add("New Host…").OnClick(a.menuAction("file.newHost"))
	file.Add("Import ~/.ssh/config").OnClick(a.menuAction("file.importConfig"))
	file.Add("Workspaces…").OnClick(a.menuAction("file.workspaces"))
	file.AddSeparator()
	file.Add("Settings…").OnClick(a.menuAction("file.settings"))
	file.AddSeparator()
	file.Add("Quit").SetAccelerator("CmdOrCtrl+Q").OnClick(func(*application.Context) { a.Quit() })

	edit := m.AddSubmenu("Edit")
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
	view.Add("Toggle Fullscreen").SetAccelerator("F11").OnClick(a.menuAction("view.fullscreen"))

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

	help := m.AddSubmenu("Help")
	help.Add("About SSH First").OnClick(a.menuAction("help.about"))
	return m
}

func (a *App) menuAction(id string) func(*application.Context) {
	return func(*application.Context) { a.emit("menu:action", id) }
}
