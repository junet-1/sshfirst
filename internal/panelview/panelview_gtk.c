#include "panelview_gtk.h"
#include "_cgo_export.h"

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
GtkWidget *pv_install(GtkWindow *window, GtkWidget **shell_out) {
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

// A popup becomes a tab, not a window: the page it belongs to is already a tab,
// and an OAuth login that suddenly detaches into a floating window is jarring.
// Go is told once the popup is ready to be shown, because only then does it
// have the address the tab should be named after.
static void pv_popup_ready(WebKitWebView *popup, gpointer user_data) {
    const char *uri = webkit_web_view_get_uri(popup);
    panelViewPopupReady(popup, (char *)(uri ? uri : ""));
}

static void pv_popup_close(WebKitWebView *popup, gpointer user_data) {
    panelViewPopupClosed(popup);
}

// The popup has to be created from the panel's own view so the two stay
// related: that is what keeps window.opener working, which is how the popup
// hands its result back. WebKitGTK 6.0 dropped the dedicated constructor for
// this, leaving the "related-view" construct property.
static GtkWidget *pv_on_create(WebKitWebView *view, WebKitNavigationAction *action, gpointer user_data) {
    WebKitWebView *popup = WEBKIT_WEB_VIEW(
        g_object_new(WEBKIT_TYPE_WEB_VIEW, "related-view", view, NULL));
    g_signal_connect(popup, "ready-to-show", G_CALLBACK(pv_popup_ready), NULL);
    g_signal_connect(popup, "close", G_CALLBACK(pv_popup_close), NULL);
    g_signal_connect(popup, "load-failed", G_CALLBACK(pv_on_load_failed), NULL);
    g_signal_connect(popup, "load-failed-with-tls-errors", G_CALLBACK(pv_on_tls_error), NULL);
    // A popup may open further popups — a bank's 3-D Secure step, say.
    g_signal_connect(popup, "create", G_CALLBACK(pv_on_create), NULL);
    return GTK_WIDGET(popup);
}

static void pv_prepare(WebKitWebView *view) {
    GtkWidget *widget = GTK_WIDGET(view);
    // Anchored top-left so the overlay honours the margins as absolute
    // coordinates instead of stretching the view across the window.
    gtk_widget_set_halign(widget, GTK_ALIGN_START);
    gtk_widget_set_valign(widget, GTK_ALIGN_START);
    // Hidden until the frontend reports a rectangle, so a new panel never
    // flashes across the window first — and, while hidden, it takes no input.
    gtk_widget_set_visible(widget, FALSE);
    gtk_widget_set_size_request(widget, 1, 1);
}

WebKitWebView *pv_new_view(GtkWidget *overlay, const char *uri, const char *script) {
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

    pv_prepare(view);
    gtk_overlay_add_overlay(GTK_OVERLAY(overlay), GTK_WIDGET(view));
    webkit_web_view_load_uri(view, uri);
    return view;
}

void pv_adopt_view(GtkWidget *overlay, WebKitWebView *view) {
    pv_prepare(view);
    gtk_overlay_add_overlay(GTK_OVERLAY(overlay), GTK_WIDGET(view));
}

// The frontend measures in CSS pixels; the widget lives in window pixels. The
// two agree only at zoom 1, so the ratio is taken from the application's own
// web view, whose widget size and reported viewport size describe exactly that
// scaling.
void pv_set_bounds(GtkWidget *overlay, GtkWidget *shell, WebKitWebView *view,
                   int x, int y, int w, int h,
                   int viewport_w, int viewport_h, int visible) {
    (void)overlay;
    (void)viewport_h;
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

void pv_remove(GtkWidget *overlay, WebKitWebView *view) {
    gtk_overlay_remove_overlay(GTK_OVERLAY(overlay), GTK_WIDGET(view));
}

void pv_reload(WebKitWebView *view) {
    webkit_web_view_reload(view);
}

void pv_evaluate(WebKitWebView *view, const char *js) {
    webkit_web_view_evaluate_javascript(view, js, -1, NULL, NULL, NULL, NULL, NULL);
}
