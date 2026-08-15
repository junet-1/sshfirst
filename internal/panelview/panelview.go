// Package panelview renders a web panel in a real WebKit view of its own,
// layered over the application's window, instead of in an iframe.
//
// The reason is not cosmetic. A panel in an iframe is a framed browsing
// context, so every site that sends X-Frame-Options: DENY or a
// frame-ancestors CSP — Cloudflare, GitHub, Google, most SaaS dashboards —
// refuses to render, and no client-side setting can change that: refusing is
// the entire point of the header. The same page loaded as a top-level document
// is not framed, so the header does not apply at all. Verified against
// WebKitGTK 6.0: identical page, blocked in an iframe, rendered in a view.
//
// The view is a native widget stacked above the window's HTML, which brings the
// usual consequence: it paints over anything the frontend draws underneath, so
// the frontend hides it while a modal or the command palette is open. Its
// position comes from the frontend in CSS pixels and is scaled here, because a
// HiDPI or zoomed webview does not measure in the same units the window does.
package panelview

// Bounds is a panel's rectangle inside the web view's own coordinate space,
// together with the viewport it was measured against. Both are in CSS pixels;
// the platform implementation converts them into widget coordinates.
type Bounds struct {
	X, Y, Width, Height  int
	ViewportW, ViewportH int
	Visible              bool
}
