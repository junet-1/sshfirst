//go:build darwin && cgo

// WKWebView implementation of the autofill bridge. See bridge.go for the
// injected script and webautofill_linux.go for the WebKitGTK counterpart.

package webautofill

/*
#cgo CFLAGS: -mmacosx-version-min=10.13 -x objective-c
#cgo LDFLAGS: -framework Foundation -framework Cocoa -framework WebKit

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#include <stdlib.h>

static WKWebView *ssh_first_find_webview(NSView *view) {
    if (view == nil) {
        return nil;
    }
    if ([view isKindOfClass:[WKWebView class]]) {
        return (WKWebView *)view;
    }
    for (NSView *child in [view subviews]) {
        WKWebView *found = ssh_first_find_webview(child);
        if (found != nil) {
            return found;
        }
    }
    return nil;
}

static void ssh_first_install_autofill(void *window, const char *source) {
    NSWindow *nsWindow = (NSWindow *)window;
    if (nsWindow == nil) {
        return;
    }
    WKWebView *webview = ssh_first_find_webview([nsWindow contentView]);
    if (webview == nil) {
        return;
    }
    WKUserContentController *controller = webview.configuration.userContentController;
    if (controller == nil) {
        return;
    }
    NSString *js = [NSString stringWithUTF8String:source];
    if (js == nil) {
        return;
    }
    // forMainFrameOnly:NO is the whole point: the panel lives in a cross-origin
    // iframe that no script from the app's own frame can reach.
    WKUserScript *script = [[WKUserScript alloc] initWithSource:js
                                                  injectionTime:WKUserScriptInjectionTimeAtDocumentEnd
                                               forMainFrameOnly:NO];
    [controller addUserScript:script];
    // cgo compiles without ARC, so the script is owned here and the controller
    // retains it for as long as it needs it.
    [script release];
}
*/
import "C"

import "unsafe"

// Install adds the bridge to the window's WKWebView. nativeWindow must be the
// NSWindow returned by application.WebviewWindow.NativeWindow, and this call
// must run on the main thread.
func Install(nativeWindow unsafe.Pointer) {
	if nativeWindow == nil {
		return
	}
	source := C.CString(bridgeScript)
	defer C.free(unsafe.Pointer(source))
	C.ssh_first_install_autofill(nativeWindow, source)
}
