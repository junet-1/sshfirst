package app

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// faviconClient fetches panel favicons directly from the origin. TLS
// verification is skipped on purpose: web panels are typically the user's own
// self-signed services (Proxmox etc.), and only a favicon image is retrieved —
// no secrets, no credentials. Kept separate from the SSH/known-hosts trust path.
var faviconClient = &http.Client{
	Timeout: 6 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
	// A panel decides where its own redirects point, so without this it could
	// bounce the icon request onto any address the app can reach — 127.0.0.1 or
	// a device on the user's LAN — and use it as a probe from inside that
	// network. Staying on the same hostname keeps the fetch pointed at the
	// machine the user actually configured; a different port or an http→https
	// upgrade is still that machine.
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		if req.URL.Hostname() != via[0].URL.Hostname() {
			return fmt.Errorf("refusing redirect to another host (%s)", req.URL.Hostname())
		}
		return nil
	},
}

const maxFaviconBytes = 256 * 1024

// Favicon returns a data: URL for a web panel's favicon, cached per origin so it
// displays persistently in the sidebar (offline, across restarts) without
// re-fetching. Returns "" (not an error) when there is no usable favicon, so the
// frontend can simply fall back to the globe icon.
func (a *App) Favicon(rawURL string) (string, error) {
	if err := a.requireStore(); err != nil {
		return "", err
	}
	origin, err := faviconOrigin(rawURL)
	if err != nil {
		return "", nil
	}
	if cached, ok, err := a.store.GetFavicon(origin); err == nil && ok {
		return cached, nil
	}
	dataURL := fetchFavicon(origin)
	if dataURL == "" {
		return "", nil
	}
	if err := a.store.SetFavicon(origin, dataURL); err != nil {
		log.Printf("ssh-first: cache favicon for %s: %v", origin, err)
	}
	return dataURL, nil
}

// faviconOrigin reduces a panel URL to scheme://host[:port]. https is assumed
// when no scheme is present, matching how panel URLs are normalized elsewhere.
func faviconOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty url")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid url %q", raw)
	}
	return u.Scheme + "://" + u.Host, nil
}

// fetchFavicon tries the conventional /favicon.ico first, then falls back to
// the icon declared in the page's <head> (rel="icon" / "shortcut icon" /
// "apple-touch-icon") — many apps (e.g. Nginx Proxy Manager) only ship the
// latter and 404 the well-known path.
func fetchFavicon(origin string) string {
	if dataURL := fetchImageAsDataURL(origin + "/favicon.ico"); dataURL != "" {
		return dataURL
	}
	resp, err := faviconClient.Get(origin + "/")
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return ""
	}
	defer resp.Body.Close()
	href := iconHrefFromHTML(io.LimitReader(resp.Body, 512*1024))
	if href == "" {
		return ""
	}
	ref, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return ""
	}
	iconURL := resp.Request.URL.ResolveReference(ref)
	// The href is remote input: it comes out of the panel's own HTML. Without
	// this check a panel could aim it at any address the app can reach —
	// 127.0.0.1, or a device on the user's LAN — and use the icon fetch as a
	// probe from inside that network. An icon not served by the panel itself is
	// not worth that.
	if !sameOrigin(iconURL, resp.Request.URL) {
		return ""
	}
	return fetchImageAsDataURL(iconURL.String())
}

func sameOrigin(a, b *url.URL) bool {
	return a.Scheme == b.Scheme && a.Host == b.Host
}

// fetchImageAsDataURL downloads a single image URL and returns it as a data:
// URL, or "" if it isn't reachable or isn't an image.
func fetchImageAsDataURL(rawURL string) string {
	resp, err := faviconClient.Get(rawURL)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFaviconBytes))
	if err != nil || len(body) == 0 {
		return ""
	}
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		contentType = http.DetectContentType(body)
	}
	if !strings.HasPrefix(contentType, "image/") {
		return "" // an HTML error page, not an icon
	}
	if i := strings.IndexByte(contentType, ';'); i >= 0 {
		contentType = contentType[:i]
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(body)
}

// iconHrefFromHTML scans a document's <link> tags for the best icon href,
// preferring a real "icon" over an "apple-touch-icon" and ignoring SVG
// "mask-icon" entries.
func iconHrefFromHTML(r io.Reader) string {
	z := html.NewTokenizer(r)
	var icon, appleIcon string
	for {
		switch z.Next() {
		case html.ErrorToken:
			if icon != "" {
				return icon
			}
			return appleIcon
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			if string(name) != "link" || !hasAttr {
				continue
			}
			var rel, href string
			for hasAttr {
				var k, v []byte
				k, v, hasAttr = z.TagAttr()
				switch strings.ToLower(string(k)) {
				case "rel":
					rel = strings.ToLower(string(v))
				case "href":
					href = string(v)
				}
			}
			if href == "" || !strings.Contains(rel, "icon") || strings.Contains(rel, "mask") {
				continue
			}
			if strings.Contains(rel, "apple") {
				if appleIcon == "" {
					appleIcon = href
				}
			} else if icon == "" {
				icon = href
			}
		}
	}
}
