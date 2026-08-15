//go:build linux && cgo && !gtk3

// WebKitGTK implementation. See panelview.go for what this is for; the C lives
// in panelview_gtk.c, because a file that exports Go functions back to C may
// only declare, not define, in its preamble.

package panelview

/*
#cgo pkg-config: gtk4 webkitgtk-6.0
#include <stdlib.h>
#include "panelview_gtk.h"
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

// Every call here touches GTK and must therefore run on the main thread; the
// caller is responsible for that (see internal/app, which wraps these in the
// runtime's main-thread dispatch). The mutex only guards the Go-side bookkeeping.
//
// It must never be held across a call into C. WebKit emits notify::uri and
// notify::title synchronously — load_uri, reload and disposal all do it before
// returning — and those signals come straight back here through the exported
// callbacks, which need the same mutex. Holding it across the call deadlocks
// the main thread and freezes the whole window. So: read what is needed under
// the lock, release, then call.
var (
	mu      sync.Mutex
	overlay *C.GtkWidget
	shell   *C.GtkWidget
	views   = map[string]*C.WebKitWebView{}

	popupCount   int
	popupHandler func(Popup)
	closeHandler func(id string)
	infoHandler  func(Info)
)

// Supported reports whether panel views can be used on this build.
func Supported() bool { return true }

// OnPopup registers what happens when a panel opens a window: window.open, a
// target=_blank link, or the popup half of an OAuth login.
func OnPopup(handler func(Popup)) {
	mu.Lock()
	defer mu.Unlock()
	popupHandler = handler
}

// OnClosed registers what happens when a page closes itself, which is how most
// OAuth popups finish.
func OnClosed(handler func(id string)) {
	mu.Lock()
	defer mu.Unlock()
	closeHandler = handler
}

// OnInfo registers what happens when a panel's page changes its title or
// address, so a tab can follow it the way a browser tab does.
func OnInfo(handler func(Info)) {
	mu.Lock()
	defer mu.Unlock()
	infoHandler = handler
}

// Install prepares the window to host panel views and reports whether that
// worked. It is idempotent; nativeWindow must be the GtkWindow from
// application.WebviewWindow.NativeWindow.
func Install(nativeWindow unsafe.Pointer) bool {
	mu.Lock()
	already := overlay != nil
	mu.Unlock()
	if already {
		return true
	}
	if nativeWindow == nil {
		return false
	}

	var shellOut *C.GtkWidget
	layer := C.pv_install((*C.GtkWindow)(nativeWindow), &shellOut)
	if layer == nil {
		return false
	}

	mu.Lock()
	overlay = layer
	shell = shellOut
	mu.Unlock()
	return true
}

// Open creates the view for a panel tab and starts loading uri. script is
// injected into every frame of the page; pass "" for none.
func Open(id, uri, script string) {
	mu.Lock()
	layer := overlay
	exists := views[id] != nil
	mu.Unlock()
	if layer == nil || exists {
		return
	}

	var cscript *C.char
	if script != "" {
		cscript = C.CString(script)
		defer C.free(unsafe.Pointer(cscript))
	}

	// Created empty first and registered before anything is loaded, so the
	// notifications the load produces can already be matched to this id.
	view := C.pv_new_view(layer, cscript)

	mu.Lock()
	views[id] = view
	mu.Unlock()

	curi := C.CString(uri)
	defer C.free(unsafe.Pointer(curi))
	C.pv_load_uri(view, curi)
}

// SetBounds moves the panel view to the rectangle the frontend measured, or
// hides it when the tab is not on screen.
func SetBounds(id string, b Bounds) {
	mu.Lock()
	layer, base, view := overlay, shell, views[id]
	mu.Unlock()
	if layer == nil || view == nil {
		return
	}
	visible := C.int(0)
	if b.Visible {
		visible = 1
	}
	C.pv_set_bounds(layer, base, view,
		C.int(b.X), C.int(b.Y), C.int(b.Width), C.int(b.Height),
		C.int(b.ViewportW), C.int(b.ViewportH), visible)
}

// Close destroys a panel's view.
func Close(id string) {
	mu.Lock()
	layer, view := overlay, views[id]
	// Forgotten before the widget goes away: disposal emits notifications, and
	// they should no longer resolve to a tab that is being closed.
	delete(views, id)
	mu.Unlock()
	if layer == nil || view == nil {
		return
	}
	C.pv_remove(layer, view)
}

// Reload reloads a panel's page.
func Reload(id string) {
	mu.Lock()
	view := views[id]
	mu.Unlock()
	if view != nil {
		C.pv_reload(view)
	}
}

// Evaluate runs JavaScript inside a panel's page. It is how credentials reach
// the autofill bridge: a panel view is a top-level document, so there is no
// parent frame for the application to post them from.
func Evaluate(id, js string) {
	mu.Lock()
	view := views[id]
	mu.Unlock()
	if view == nil {
		return
	}
	cjs := C.CString(js)
	defer C.free(unsafe.Pointer(cjs))
	C.pv_evaluate(view, cjs)
}

// panelViewPopupReady is called by WebKit, through pv_popup_ready, once a popup
// this application created has an address and wants to be shown. The view
// already exists — WebKit made it, and it must be the one WebKit made, or
// window.opener would not connect it to the page that opened it — so it is
// adopted into the overlay under a fresh id and handed on for a tab.
//
//export panelViewPopupReady
func panelViewPopupReady(view *C.WebKitWebView, uri *C.char) {
	mu.Lock()
	if overlay == nil {
		mu.Unlock()
		return
	}
	popupCount++
	id := fmt.Sprintf("panel-popup-%d", popupCount)
	views[id] = view
	layer := overlay
	handler := popupHandler
	mu.Unlock()

	C.pv_adopt_view(layer, view)

	if handler != nil {
		handler(Popup{ID: id, URL: C.GoString(uri)})
	}
}

// panelViewInfoChanged is called whenever a panel's page changes its title or
// address.
//
//export panelViewInfoChanged
func panelViewInfoChanged(view *C.WebKitWebView, title *C.char, uri *C.char) {
	mu.Lock()
	id := idForView(view)
	handler := infoHandler
	mu.Unlock()

	if id != "" && handler != nil {
		handler(Info{ID: id, Title: C.GoString(title), URL: C.GoString(uri)})
	}
}

// panelViewPopupClosed is called when a page closes itself.
//
//export panelViewPopupClosed
func panelViewPopupClosed(view *C.WebKitWebView) {
	mu.Lock()
	id := idForView(view)
	handler := closeHandler
	mu.Unlock()

	if id != "" && handler != nil {
		handler(id)
	}
}

// idForView maps a WebKit view back to the id it was registered under. Callers
// must hold mu.
func idForView(view *C.WebKitWebView) string {
	for id, known := range views {
		if known == view {
			return id
		}
	}
	return ""
}
