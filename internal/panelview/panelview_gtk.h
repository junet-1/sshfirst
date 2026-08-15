// Declarations for the WebKitGTK side of the panel views. The implementation
// lives in panelview_gtk.c rather than in the Go file's preamble because cgo
// forbids function definitions there once a file exports Go callbacks back to
// C — and popups need exactly that.

#ifndef SSH_FIRST_PANELVIEW_GTK_H
#define SSH_FIRST_PANELVIEW_GTK_H

#include <gtk/gtk.h>
#include <webkit/webkit.h>

// Splices a GtkOverlay around the application's own web view and returns it.
// shell_out receives that web view, whose size is the scale reference.
GtkWidget *pv_install(GtkWindow *window, GtkWidget **shell_out);

// Creates an empty panel view, injecting script into every frame of whatever it
// later loads. Loading is a separate step so the caller can register the view
// before the notifications the load produces arrive.
WebKitWebView *pv_new_view(GtkWidget *overlay, const char *script);
void pv_load_uri(WebKitWebView *view, const char *uri);

// Places a view WebKit itself created (a popup) into the overlay.
void pv_adopt_view(GtkWidget *overlay, WebKitWebView *view);

void pv_set_bounds(GtkWidget *overlay, GtkWidget *shell, WebKitWebView *view,
                   int x, int y, int w, int h,
                   int viewport_w, int viewport_h, int visible);
void pv_remove(GtkWidget *overlay, WebKitWebView *view);
void pv_reload(WebKitWebView *view);
void pv_evaluate(WebKitWebView *view, const char *js);

#endif
