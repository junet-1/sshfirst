//go:build linux

package app

import (
	"sort"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"ssh-first/internal/storage"
)

type wailsTray struct {
	app  *App
	icon []byte

	mu   sync.Mutex
	tray *application.SystemTray
}

func newTrayService(app *App, icon []byte) trayService {
	return &wailsTray{app: app, icon: append([]byte(nil), icon...)}
}

func (t *wailsTray) Start() bool {
	if t.app.ui == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tray = t.app.ui.SystemTray.New().
		SetIcon(t.icon).
		OnClick(t.app.ShowMainWindow)
	t.tray.SetTooltip("SSH First")
	t.rebuildMenuLocked()
	return true
}

func (t *wailsTray) Refresh() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.tray != nil {
		t.rebuildMenuLocked()
	}
}

func (t *wailsTray) rebuildMenuLocked() {
	menu := t.app.ui.NewMenu()
	menu.Add("Open Main Window").OnClick(func(*application.Context) { t.app.ShowMainWindow() })
	menu.Add("Preferences").OnClick(func(*application.Context) {
		_ = t.app.OpenToolWindow("settings", -1, -1)
	})
	menu.Add("About").OnClick(func(*application.Context) {
		_ = t.app.OpenToolWindow("about", -1, -1)
	})
	menu.AddSeparator()

	hosts, err := t.app.ListHosts()
	if err != nil {
		menu.Add("Connections unavailable").SetEnabled(false)
	} else if len(hosts) == 0 {
		menu.Add("No saved connections").SetEnabled(false)
	} else {
		for _, host := range trayHosts(hosts) {
			host := host
			tooltip := host.Hostname
			if host.User != "" {
				tooltip = host.User + "@" + host.Hostname
			}
			menu.Add(host.Label).
				SetTooltip(tooltip).
				SetBitmap(trayProtocolIcon(host.Protocol)).
				OnClick(func(*application.Context) {
					t.app.ShowMainWindow()
					t.app.emit("tray:connect-host", map[string]any{
						"hostId":    host.ID,
						"hostLabel": host.Label,
					})
				})
		}
	}

	menu.AddSeparator()
	menu.Add("New Connection…").OnClick(func(*application.Context) {
		_ = t.app.OpenToolWindow("host", -1, -1)
	})
	menu.AddSeparator()
	menu.Add("Quit").OnClick(func(*application.Context) { t.app.Quit() })
	t.tray.SetMenu(menu)
}

func trayHosts(hosts []storage.Host) []storage.Host {
	ordered := append([]storage.Host(nil), hosts...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Favorite != ordered[j].Favorite {
			return ordered[i].Favorite
		}
		if ordered[i].LastUsedAt != nil && ordered[j].LastUsedAt != nil && *ordered[i].LastUsedAt != *ordered[j].LastUsedAt {
			return *ordered[i].LastUsedAt > *ordered[j].LastUsedAt
		}
		if (ordered[i].LastUsedAt != nil) != (ordered[j].LastUsedAt != nil) {
			return ordered[i].LastUsedAt != nil
		}
		return ordered[i].Label < ordered[j].Label
	})
	return ordered
}

func (t *wailsTray) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.tray != nil {
		t.tray.Destroy()
		t.tray = nil
	}
}
