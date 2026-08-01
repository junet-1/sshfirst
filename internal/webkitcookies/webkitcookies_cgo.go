//go:build linux && cgo && !gtk3

// Package webkitcookies configures persistent cookie storage on the WebKitGTK
// default network session. Wails creates every WebviewWindow against that same
// default session but never enables on-disk cookies, so without this a login to
// a web-panel host (protocol "web") is lost the moment the app restarts.
package webkitcookies

/*
#cgo pkg-config: webkitgtk-6.0
#include <webkit/webkit.h>
#include <stdlib.h>

static void ssh_first_set_persistent_cookies(const char *path) {
    WebKitNetworkSession *session = webkit_network_session_get_default();
    if (session == NULL) {
        return;
    }
    WebKitCookieManager *cm = webkit_network_session_get_cookie_manager(session);
    if (cm == NULL) {
        return;
    }
    // Backs the cookie jar with an SQLite file and loads whatever is already
    // stored there, so sessions survive a restart.
    webkit_cookie_manager_set_persistent_storage(cm, path, WEBKIT_COOKIE_PERSISTENT_STORAGE_SQLITE);
    // Panels load inside cross-origin iframes, so their session cookies are
    // third-party relative to the wails:// top frame; accept them explicitly
    // (the default policy drops third-party cookies).
    webkit_cookie_manager_set_accept_policy(cm, WEBKIT_COOKIE_POLICY_ACCEPT_ALWAYS);
}
*/
import "C"

import "unsafe"

// SetPersistentStorage points the default network session's cookie jar at the
// given SQLite file. It MUST run on the GTK main thread — dispatch it via
// application.InvokeAsync, never from an arbitrary goroutine.
func SetPersistentStorage(path string) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	C.ssh_first_set_persistent_cookies(cpath)
}
