//go:build linux && cgo && !gtk3

// WebKitGTK implementation. See panelview.go for what this is for.

package panelview

/*
#cgo pkg-config: gtk4 webkitgtk-6.0
#include <gtk/gtk.h>
#include <webkit/webkit.h>
#include <stdlib.h>

static WebKitWebView *pv_find_webview(GtkWidget *widget) {
    if (widget == NULL) {
        return NULL;
    }
    if (WEBKIT_IS_WEB_VIEW(widget)) {
        return WEBKIT_WEB_VIEW(widget);
    }
    for (GtkWidget *child = gtk_widget_get_first_child(widget);
         child != NULL;
         child = gtk_widget_get_next_sibling(child)) {
        WebKitWebView *found = pv_find_webview(child);
        if (found != NULL) {
            return found;
        }
    }
    return NULL;
}

// Wails builds GtkWindow -> GtkBox -> [menu bar] + WebKitWebView. This slips a
// GtkOverlay in around that web view and adds a GtkFixed on top of it, which is
// where panel views get placed at absolute coordinates. The box keeps its order,
// so the menu bar stays above the overlay.
static GtkWidget *pv_install(GtkWindow *window, GtkWidget **shell_out) {
    GtkWidget *shell = GTK_WIDGET(pv_find_webview(GTK_WIDGET(window)));
    if (shell == NULL) {
        return NULL;
    }
    GtkWidget *parent = gtk_widget_get_parent(shell);
    if (parent == NULL || !GTK_IS_BOX(parent)) {
        return NULL;
    }

    GtkWidget *overlay = gtk_overlay_new();
    gtk_widget_set_hexpand(overlay, TRUE);
    gtk_widget_set_vexpand(overlay, TRUE);

    // Hold a reference across the reparent, or removing it from the box drops
    // the last one and destroys the application's own web view.
    g_object_ref(shell);
    gtk_box_remove(GTK_BOX(parent), shell);
    gtk_overlay_set_child(GTK_OVERLAY(overlay), shell);
    g_object_unref(shell);

    GtkWidget *fixed = gtk_fixed_new();
    gtk_overlay_add_overlay(GTK_OVERLAY(overlay), fixed);
    gtk_box_append(GTK_BOX(parent), overlay);

    *shell_out = shell;
    return fixed;
}

static WebKitWebView *pv_new_view(GtkWidget *fixed, const char *uri, const char *script) {
    WebKitWebView *view = WEBKIT_WEB_VIEW(webkit_web_view_new());

    // No network session is passed, so the view uses the default one — the same
    // session the application configures persistent cookies on, which is why a
    // panel stays logged in across restarts.
    if (script != NULL) {
        WebKitUserContentManager *manager = webkit_web_view_get_user_content_manager(view);
        if (manager != NULL) {
            WebKitUserScript *user = webkit_user_script_new(
                script,
                WEBKIT_USER_CONTENT_INJECT_ALL_FRAMES,
                WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_END,
                NULL,
                NULL);
            webkit_user_content_manager_add_script(manager, user);
            webkit_user_script_unref(user);
        }
    }

    GtkWidget *widget = GTK_WIDGET(view);
    // Placed off-screen and hidden until the frontend reports a rectangle, so a
    // new panel never flashes across the window first.
    gtk_widget_set_visible(widget, FALSE);
    gtk_widget_set_size_request(widget, 1, 1);
    gtk_fixed_put(GTK_FIXED(fixed), widget, 0, 0);
    webkit_web_view_load_uri(view, uri);
    return view;
}

// The frontend measures in CSS pixels; the widget lives in window pixels. The
// two agree only at zoom 1, so the ratio is taken from the application's own
// web view, whose widget size and reported viewport size describe exactly that
// scaling.
static void pv_set_bounds(GtkWidget *fixed, GtkWidget *shell, WebKitWebView *view,
                          int x, int y, int w, int h,
                          int viewport_w, int viewport_h, int visible) {
    GtkWidget *widget = GTK_WIDGET(view);
    if (!visible) {
        gtk_widget_set_visible(widget, FALSE);
        return;
    }

    double scale = 1.0;
    if (viewport_w > 0) {
        int shell_w = gtk_widget_get_width(shell);
        if (shell_w > 0) {
            scale = (double)shell_w / (double)viewport_w;
        }
    }
    if (scale <= 0.0) {
        scale = 1.0;
    }

    // Rounded without math.h so the package needs no extra link flag.
    int sx = (int)(x * scale + 0.5);
    int sy = (int)(y * scale + 0.5);
    int sw = (int)(w * scale + 0.5);
    int sh = (int)(h * scale + 0.5);
    if (sw < 1) sw = 1;
    if (sh < 1) sh = 1;

    gtk_widget_set_size_request(widget, sw, sh);
    gtk_fixed_move(GTK_FIXED(fixed), widget, sx, sy);
    gtk_widget_set_visible(widget, TRUE);
}

static void pv_remove(GtkWidget *fixed, WebKitWebView *view) {
    gtk_fixed_remove(GTK_FIXED(fixed), GTK_WIDGET(view));
}

static void pv_reload(WebKitWebView *view) {
    webkit_web_view_reload(view);
}

static void pv_evaluate(WebKitWebView *view, const char *js) {
    webkit_web_view_evaluate_javascript(view, js, -1, NULL, NULL, NULL, NULL, NULL);
}
*/
import "C"

import (
	"sync"
	"unsafe"
)

// Every call here touches GTK and must therefore run on the main thread; the
// caller is responsible for that (see internal/app, which wraps these in the
// runtime's main-thread dispatch). The mutex only guards the Go-side bookkeeping.
var (
	mu    sync.Mutex
	fixed *C.GtkWidget
	shell *C.GtkWidget
	views = map[string]*C.WebKitWebView{}
)

// Supported reports whether panel views can be used on this build.
func Supported() bool { return true }

// Install prepares the window to host panel views and reports whether that
// worked. It is idempotent; nativeWindow must be the GtkWindow from
// application.WebviewWindow.NativeWindow.
func Install(nativeWindow unsafe.Pointer) bool {
	mu.Lock()
	defer mu.Unlock()
	if fixed != nil {
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
	fixed = layer
	shell = shellOut
	return true
}

// Open creates the view for a panel tab and starts loading uri. script is
// injected into every frame of the page; pass "" for none.
func Open(id, uri, script string) {
	mu.Lock()
	defer mu.Unlock()
	if fixed == nil || views[id] != nil {
		return
	}

	curi := C.CString(uri)
	defer C.free(unsafe.Pointer(curi))

	var cscript *C.char
	if script != "" {
		cscript = C.CString(script)
		defer C.free(unsafe.Pointer(cscript))
	}

	views[id] = C.pv_new_view(fixed, curi, cscript)
}

// SetBounds moves the panel view to the rectangle the frontend measured, or
// hides it when the tab is not on screen.
func SetBounds(id string, b Bounds) {
	mu.Lock()
	defer mu.Unlock()
	view := views[id]
	if fixed == nil || view == nil {
		return
	}
	visible := C.int(0)
	if b.Visible {
		visible = 1
	}
	C.pv_set_bounds(fixed, shell, view,
		C.int(b.X), C.int(b.Y), C.int(b.Width), C.int(b.Height),
		C.int(b.ViewportW), C.int(b.ViewportH), visible)
}

// Close destroys a panel's view.
func Close(id string) {
	mu.Lock()
	defer mu.Unlock()
	view := views[id]
	if fixed == nil || view == nil {
		return
	}
	C.pv_remove(fixed, view)
	delete(views, id)
}

// Reload reloads a panel's page.
func Reload(id string) {
	mu.Lock()
	defer mu.Unlock()
	if view := views[id]; view != nil {
		C.pv_reload(view)
	}
}

// Evaluate runs JavaScript inside a panel's page. It is how credentials reach
// the autofill bridge: a panel view is a top-level document, so there is no
// parent frame for the application to post them from.
func Evaluate(id, js string) {
	mu.Lock()
	defer mu.Unlock()
	view := views[id]
	if view == nil {
		return
	}
	cjs := C.CString(js)
	defer C.free(unsafe.Pointer(cjs))
	C.pv_evaluate(view, cjs)
}
