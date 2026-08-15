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
// GtkOverlay in around that web view; panel views are then added as overlay
// children, each positioned by its own margins.
//
// Note what is deliberately absent: no container spanning the overlay. A
// full-size GtkFixed would be the obvious way to place children at absolute
// coordinates, but an overlay child covers the whole window for input picking
// even when it draws nothing, so every click outside a panel would land on it
// and the entire interface below would stop responding.
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

    gtk_box_append(GTK_BOX(parent), overlay);

    *shell_out = shell;
    return overlay;
}


// A panel that opens a window — target=_blank, window.open, and every OAuth
// flow that authenticates in a popup — gets a real one. Without this the
// request is dropped silently and such a login simply never completes.
//
// The popup has to be created from the panel's own view so the two stay
// related: that is what keeps window.opener working, which is how the popup
// hands its result back.
static void pv_popup_ready(WebKitWebView *popup, gpointer user_data) {
    GtkWidget *window = gtk_window_new();
    gtk_window_set_title(GTK_WINDOW(window), "SSH First — Web Panel");

    WebKitWindowProperties *props = webkit_web_view_get_window_properties(popup);
    int width = 900, height = 700;
    if (props != NULL) {
        GdkRectangle geometry;
        webkit_window_properties_get_geometry(props, &geometry);
        if (geometry.width > 100) width = geometry.width;
        if (geometry.height > 100) height = geometry.height;
    }
    gtk_window_set_default_size(GTK_WINDOW(window), width, height);
    gtk_window_set_child(GTK_WINDOW(window), GTK_WIDGET(popup));
    gtk_window_present(GTK_WINDOW(window));
}

static void pv_popup_close(WebKitWebView *popup, gpointer user_data) {
    GtkWidget *root = GTK_WIDGET(gtk_widget_get_root(GTK_WIDGET(popup)));
    if (GTK_IS_WINDOW(root)) {
        gtk_window_destroy(GTK_WINDOW(root));
    }
}

static GtkWidget *pv_on_create(WebKitWebView *view, WebKitNavigationAction *action, gpointer user_data) {
    // WebKitGTK 6.0 dropped the dedicated constructor for this; the
    // "related-view" construct property is what is left, and it is what keeps
    // window.opener intact between the popup and the panel that opened it.
    WebKitWebView *popup = WEBKIT_WEB_VIEW(
        g_object_new(WEBKIT_TYPE_WEB_VIEW, "related-view", view, NULL));
    g_signal_connect(popup, "ready-to-show", G_CALLBACK(pv_popup_ready), NULL);
    g_signal_connect(popup, "close", G_CALLBACK(pv_popup_close), NULL);
    return GTK_WIDGET(popup);
}

// Without this a failed load leaves an empty rectangle and no explanation. The
// page is rendered for the address that failed, so a reload retries the panel
// rather than the error page.
static void pv_show_error(WebKitWebView *view, const char *failing_uri, const char *headline, const char *detail) {
    char *safe_uri = g_markup_escape_text(failing_uri ? failing_uri : "", -1);
    char *safe_detail = g_markup_escape_text(detail ? detail : "", -1);
    char *html = g_strdup_printf(
        "<!doctype html><meta charset='utf-8'>"
        "<style>html{height:100%%}body{margin:0;height:100%%;display:flex;align-items:center;"
        "justify-content:center;background:#1e2022;color:#c8ccd0;"
        "font:13px 'Noto Sans',Cantarell,system-ui,sans-serif}"
        "div{max-width:32em;padding:0 2em;text-align:center}"
        "h1{font-size:15px;font-weight:600;color:#eff0f1;margin:0 0 .6em}"
        "p{margin:.4em 0;line-height:1.5}code{color:#8f979e;word-break:break-all}</style>"
        "<div><h1>%s</h1><p>%s</p><p><code>%s</code></p></div>",
        headline, safe_detail, safe_uri);
    webkit_web_view_load_alternate_html(view, html, failing_uri, NULL);
    g_free(html);
    g_free(safe_uri);
    g_free(safe_detail);
}

static gboolean pv_on_load_failed(WebKitWebView *view, WebKitLoadEvent event,
                                  gchar *failing_uri, GError *error, gpointer user_data) {
    // Cancelled loads are the normal consequence of navigating away.
    if (g_error_matches(error, WEBKIT_NETWORK_ERROR, WEBKIT_NETWORK_ERROR_CANCELLED)) {
        return FALSE;
    }
    pv_show_error(view, failing_uri, "This panel did not load", error ? error->message : "Unknown error");
    return TRUE;
}

static gboolean pv_on_tls_error(WebKitWebView *view, gchar *failing_uri,
                                GTlsCertificate *certificate, GTlsCertificateFlags errors,
                                gpointer user_data) {
    // Deliberately not accepted automatically. Self-signed certificates are
    // normal for homelab panels, but silently trusting whatever answers would
    // throw away the only protection the connection has.
    pv_show_error(view, failing_uri, "Certificate not trusted",
                  "The panel's TLS certificate could not be verified. If it is self-signed, "
                  "add it to your system trust store — or open the panel in your browser and "
                  "accept it there.");
    return TRUE;
}

static WebKitWebView *pv_new_view(GtkWidget *overlay, const char *uri, const char *script) {
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

    g_signal_connect(view, "create", G_CALLBACK(pv_on_create), NULL);
    g_signal_connect(view, "load-failed", G_CALLBACK(pv_on_load_failed), NULL);
    g_signal_connect(view, "load-failed-with-tls-errors", G_CALLBACK(pv_on_tls_error), NULL);

    GtkWidget *widget = GTK_WIDGET(view);
    // Anchored top-left so the overlay honours the margins as absolute
    // coordinates instead of stretching the view across the window.
    gtk_widget_set_halign(widget, GTK_ALIGN_START);
    gtk_widget_set_valign(widget, GTK_ALIGN_START);
    // Hidden until the frontend reports a rectangle, so a new panel never
    // flashes across the window first — and, while hidden, it takes no input.
    gtk_widget_set_visible(widget, FALSE);
    gtk_widget_set_size_request(widget, 1, 1);
    gtk_overlay_add_overlay(GTK_OVERLAY(overlay), widget);
    webkit_web_view_load_uri(view, uri);
    return view;
}

// The frontend measures in CSS pixels; the widget lives in window pixels. The
// two agree only at zoom 1, so the ratio is taken from the application's own
// web view, whose widget size and reported viewport size describe exactly that
// scaling.
static void pv_set_bounds(GtkWidget *overlay, GtkWidget *shell, WebKitWebView *view,
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
    gtk_widget_set_margin_start(widget, sx);
    gtk_widget_set_margin_top(widget, sy);
    gtk_widget_set_visible(widget, TRUE);
}

static void pv_remove(GtkWidget *overlay, WebKitWebView *view) {
    gtk_overlay_remove_overlay(GTK_OVERLAY(overlay), GTK_WIDGET(view));
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
	mu      sync.Mutex
	overlay *C.GtkWidget
	shell   *C.GtkWidget
	views   = map[string]*C.WebKitWebView{}
)

// Supported reports whether panel views can be used on this build.
func Supported() bool { return true }

// Install prepares the window to host panel views and reports whether that
// worked. It is idempotent; nativeWindow must be the GtkWindow from
// application.WebviewWindow.NativeWindow.
func Install(nativeWindow unsafe.Pointer) bool {
	mu.Lock()
	defer mu.Unlock()
	if overlay != nil {
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
	overlay = layer
	shell = shellOut
	return true
}

// Open creates the view for a panel tab and starts loading uri. script is
// injected into every frame of the page; pass "" for none.
func Open(id, uri, script string) {
	mu.Lock()
	defer mu.Unlock()
	if overlay == nil || views[id] != nil {
		return
	}

	curi := C.CString(uri)
	defer C.free(unsafe.Pointer(curi))

	var cscript *C.char
	if script != "" {
		cscript = C.CString(script)
		defer C.free(unsafe.Pointer(cscript))
	}

	views[id] = C.pv_new_view(overlay, curi, cscript)
}

// SetBounds moves the panel view to the rectangle the frontend measured, or
// hides it when the tab is not on screen.
func SetBounds(id string, b Bounds) {
	mu.Lock()
	defer mu.Unlock()
	view := views[id]
	if overlay == nil || view == nil {
		return
	}
	visible := C.int(0)
	if b.Visible {
		visible = 1
	}
	C.pv_set_bounds(overlay, shell, view,
		C.int(b.X), C.int(b.Y), C.int(b.Width), C.int(b.Height),
		C.int(b.ViewportW), C.int(b.ViewportH), visible)
}

// Close destroys a panel's view.
func Close(id string) {
	mu.Lock()
	defer mu.Unlock()
	view := views[id]
	if overlay == nil || view == nil {
		return
	}
	C.pv_remove(overlay, view)
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
