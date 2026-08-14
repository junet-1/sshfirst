package main

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	backendapp "ssh-first/internal/app"
	"ssh-first/internal/webautofill"
	"ssh-first/internal/webkitcookies"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed migrations
var migrationsDir embed.FS

//go:embed build/appicon.png
var appIcon []byte

var singleInstanceKey = [32]byte{
	0x73, 0x73, 0x68, 0x2d, 0x66, 0x69, 0x72, 0x73,
	0x74, 0x2d, 0x64, 0x65, 0x73, 0x6b, 0x74, 0x6f,
	0x70, 0x2d, 0x73, 0x69, 0x6e, 0x67, 0x6c, 0x65,
	0x2d, 0x69, 0x6e, 0x73, 0x74, 0x61, 0x6e, 0x63,
}

func shouldStartHidden(args []string) bool {
	for _, arg := range args {
		if arg == "--start-in-tray" || arg == "--hidden" {
			return true
		}
	}
	return false
}

func main() {
	// Wayland forbids a client from setting its own toplevel position, so the
	// window-geometry restore below would be a no-op there and the compositor
	// would place windows wherever it likes. Running under XWayland (like most
	// browsers do) lets us restore the exact last position ourselves. Only
	// forced when the user has not picked a backend explicitly.
	if os.Getenv("GDK_BACKEND") == "" {
		_ = os.Setenv("GDK_BACKEND", "x11")
	}

	// WebKitGTK's JavaScriptCore JIT crashes the web-content process (heap
	// corruption / SIGABRT) after a few minutes of rendering some panel SPAs.
	// Disabling the JIT trades a little JS throughput — irrelevant for admin
	// panels — for stability. Set before any WebKit init so the web process
	// inherits it; only forced when the user hasn't chosen, so JSC_useJIT=1
	// can re-enable it for testing.
	if os.Getenv("JSC_useJIT") == "" {
		_ = os.Setenv("JSC_useJIT", "0")
	}

	migrations, err := fs.Sub(migrationsDir, "migrations")
	if err != nil {
		log.Fatalf("ssh-first: embed migrations: %v", err)
	}

	backend := backendapp.New(migrations, appIcon)
	var mainWindow *application.WebviewWindow

	desktop := application.New(application.Options{
		Name:        "SSH First",
		Description: "A focused SSH and SFTP workspace",
		Icon:        appIcon,
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Services: []application.Service{
			application.NewService(backend),
		},
		Linux: application.LinuxOptions{
			ProgramName:                   "ssh-first",
			DisableQuitOnLastWindowClosed: true,
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:      "com.sshfirst.desktop",
			EncryptionKey: singleInstanceKey,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if mainWindow != nil {
					mainWindow.Show().Restore()
					mainWindow.Focus()
				}
			},
		},
	})

	backend.ConfigureApplication(desktop)
	mainMenu := backend.BuildMenu()
	mainOptions := application.WebviewWindowOptions{
		Name:             "main",
		Title:            "SSH First",
		Hidden:           shouldStartHidden(os.Args[1:]),
		Width:            1280,
		Height:           800,
		MinWidth:         960,
		MinHeight:        600,
		URL:              "/",
		BackgroundColour: application.NewRGB(30, 32, 34),
		Linux: application.LinuxWindow{
			Icon:      appIcon,
			Menu:      mainMenu,
			MenuStyle: application.LinuxMenuStyleMenuBar,
		},
	}
	backend.RestoreWindowGeometry(&mainOptions, "main", true)
	mainWindow = desktop.Window.NewWithOptions(mainOptions)
	backend.SetMainWindow(mainWindow)
	backend.TrackWindowGeometry(mainWindow, "main", true)
	mainWindow.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if backend.HideMainWindowOnClose() {
			mainWindow.Hide()
			event.Cancel()
		}
	})

	// Cross-origin panels are deliberately isolated in iframes, so ordinary
	// frontend JavaScript cannot reach their login fields. Install a tiny native
	// WebKit user script in all frames; it contains no secrets and only reacts to
	// credentials explicitly targeted at the panel's configured origin.
	var installAutofillOnce sync.Once
	mainWindow.RegisterHook(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		installAutofillOnce.Do(func() {
			application.InvokeAsync(func() {
				webautofill.Install(mainWindow.NativeWindow())
			})
		})
	})

	// Enable on-disk cookie storage on the WebKit default network session (which
	// every webview, including the web-panel iframes, shares) so logins to web
	// hosts survive restarts. WebKit must already be initialised and the call
	// must land on the GTK main thread, hence RegisterHook + InvokeAsync; the
	// sync.Once keeps a re-fired runtime-ready event from re-running it.
	// Escape hatch for bisecting: SSHFIRST_NO_PERSIST_COOKIES=1 skips the WebKit
	// cookie call entirely (sessions then live in memory again).
	if os.Getenv("SSHFIRST_NO_PERSIST_COOKIES") == "" {
		var persistCookiesOnce sync.Once
		mainWindow.RegisterHook(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
			persistCookiesOnce.Do(func() {
				application.InvokeAsync(func() {
					dir, err := backendapp.DataDir()
					if err != nil {
						return
					}
					if err := os.MkdirAll(dir, 0o700); err != nil {
						return
					}
					webkitcookies.SetPersistentStorage(filepath.Join(dir, "cookies.sqlite"))
				})
			})
		})
	}

	if err := desktop.Run(); err != nil {
		log.Fatalf("ssh-first: %v", err)
	}
}
