// Command panelsmoke exercises the native panel views against a real GTK window.
//
// It exists because the interesting failures in internal/panelview are not
// type errors but liveness ones: WebKit emits notify::uri and notify::title
// synchronously, straight back into Go, and a lock held across a call into C
// deadlocks the main thread and freezes the entire window. Nothing in a unit
// test catches that — GTK will not even initialise inside a Go test binary —
// so this is a small program that calls the same sequence the application does,
// with a watchdog that fails loudly instead of hanging.
//
//	xvfb-run -a go run ./cmd/panelsmoke
package main

import (
	"fmt"
	"os"
	"runtime"
	"time"
	"unsafe"

	"ssh-first/internal/panelview"
)

/*
#cgo pkg-config: gtk4 webkitgtk-6.0
#include <gtk/gtk.h>
#include <webkit/webkit.h>

// The widget tree Wails builds: GtkWindow -> GtkBox -> WebKitWebView.
static GtkWindow *smoke_window(void) {
    if (!gtk_init_check()) {
        return NULL;
    }
    GtkWidget *window = gtk_window_new();
    gtk_window_set_default_size(GTK_WINDOW(window), 1000, 700);
    GtkWidget *box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
    gtk_window_set_child(GTK_WINDOW(window), box);
    GtkWidget *shell = GTK_WIDGET(webkit_web_view_new());
    gtk_widget_set_hexpand(shell, TRUE);
    gtk_widget_set_vexpand(shell, TRUE);
    gtk_box_append(GTK_BOX(box), shell);
    gtk_window_present(GTK_WINDOW(window));
    return GTK_WINDOW(window);
}

static void smoke_pump(int iterations) {
    for (int i = 0; i < iterations && g_main_context_pending(NULL); i++) {
        g_main_context_iteration(NULL, FALSE);
    }
}
*/
import "C"

// step runs one call and reports how long it took. A step that never returns is
// the failure this program is looking for, so the watchdog below is what
// actually decides the outcome.
func step(name string, fn func()) {
	start := time.Now()
	fn()
	fmt.Printf("  %-28s ok (%v)\n", name, time.Since(start).Round(time.Millisecond))
}

func main() {
	runtime.LockOSThread()

	go func() {
		time.Sleep(25 * time.Second)
		fmt.Println("DEADLOCK: a call never returned")
		os.Exit(1)
	}()

	window := C.smoke_window()
	if window == nil {
		fmt.Println("SKIP: GTK could not initialise (no display?)")
		return
	}
	C.smoke_pump(50)

	popups := make(chan panelview.Popup, 4)
	infos := make(chan panelview.Info, 32)
	closed := make(chan string, 4)
	panelview.OnPopup(func(p panelview.Popup) { popups <- p })
	panelview.OnInfo(func(i panelview.Info) { infos <- i })
	panelview.OnClosed(func(id string) { closed <- id })

	if !panelview.Install(unsafe.Pointer(window)) {
		fmt.Println("FAIL: Install could not find the application web view")
		os.Exit(1)
	}
	fmt.Println("  install                      ok")

	// A page that opens a popup by itself, so the create/ready-to-show path is
	// exercised without a network.
	page := "data:text/html," +
		"<title>Smoke Panel</title>" +
		"<script>setTimeout(()=>window.open('about:blank'),300)</script>"

	step("open", func() { panelview.Open("smoke", page, "") })
	C.smoke_pump(200)

	step("set bounds", func() {
		panelview.SetBounds("smoke", panelview.Bounds{
			X: 100, Y: 60, Width: 500, Height: 400,
			ViewportW: 1000, ViewportH: 700, Visible: true,
		})
	})
	step("evaluate", func() { panelview.Evaluate("smoke", "void 0") })
	step("reload", func() { panelview.Reload("smoke") })

	// Give the page time to run its timer and WebKit time to deliver signals.
	for i := 0; i < 60; i++ {
		C.smoke_pump(100)
		time.Sleep(50 * time.Millisecond)
	}

	fmt.Printf("  title/url notifications      %d received\n", len(infos))
	select {
	case popup := <-popups:
		fmt.Printf("  popup adopted                %s (%q)\n", popup.ID, popup.URL)
		step("close popup", func() { panelview.Close(popup.ID) })
	default:
		fmt.Println("  popup adopted                NONE — window.open did not reach the handler")
	}

	step("close", func() { panelview.Close("smoke") })
	C.smoke_pump(100)

	fmt.Println("PASS: no call blocked")
}
