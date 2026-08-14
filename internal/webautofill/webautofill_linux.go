//go:build linux && cgo && !gtk3

// WebKitGTK implementation of the autofill bridge. See bridge.go for the
// injected script and webautofill_darwin.go for the WKWebView counterpart.

package webautofill

/*
#cgo pkg-config: gtk4 webkitgtk-6.0
#include <gtk/gtk.h>
#include <webkit/webkit.h>
#include <stdlib.h>

static WebKitWebView *ssh_first_find_webview(GtkWidget *widget) {
    if (widget == NULL) {
        return NULL;
    }
    if (WEBKIT_IS_WEB_VIEW(widget)) {
        return WEBKIT_WEB_VIEW(widget);
    }
    for (GtkWidget *child = gtk_widget_get_first_child(widget);
         child != NULL;
         child = gtk_widget_get_next_sibling(child)) {
        WebKitWebView *found = ssh_first_find_webview(child);
        if (found != NULL) {
            return found;
        }
    }
    return NULL;
}

static void ssh_first_install_autofill(GtkWindow *window, const char *source) {
    WebKitWebView *webview = ssh_first_find_webview(GTK_WIDGET(window));
    if (webview == NULL) {
        return;
    }
    WebKitUserContentManager *manager = webkit_web_view_get_user_content_manager(webview);
    if (manager == NULL) {
        return;
    }
    WebKitUserScript *script = webkit_user_script_new(
        source,
        WEBKIT_USER_CONTENT_INJECT_ALL_FRAMES,
        WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_END,
        NULL,
        NULL
    );
    webkit_user_content_manager_add_script(manager, script);
    webkit_user_script_unref(script);
}
*/
import "C"

import "unsafe"

// Install adds the bridge to the main WebKit view. nativeWindow must be the
// GtkWindow returned by application.WebviewWindow.NativeWindow, and this call
// must run on GTK's main thread.
func Install(nativeWindow unsafe.Pointer) {
	if nativeWindow == nil {
		return
	}
	source := C.CString(bridgeScript)
	defer C.free(unsafe.Pointer(source))
	C.ssh_first_install_autofill((*C.GtkWindow)(nativeWindow), source)
}
