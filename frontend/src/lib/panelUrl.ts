/**
 * Normalizes and vets a control-panel URL.
 *
 * This is a security boundary, not a convenience: the returned string is used
 * directly as an `<iframe src>` (BrowserPane) and as the `targetOrigin` the
 * saved web password is posted to. Two things must never get through.
 *
 * A `javascript:` URL in an iframe's src executes in the *embedding* document's
 * origin — the app shell — where `window.wails` and every backend binding live,
 * including the ones that read stored passwords and write into live SSH
 * sessions. Verified against WebKitGTK 6.0: such a frame reaches `parent`
 * unrestricted, while a `data:` URL is correctly isolated.
 *
 * Credentials in the URL make the origin disagree with what the address bar
 * shows: `https://panel.corp.example@evil.tld/` displays as the panel but has
 * the origin `evil.tld`, which is where the password would be sent.
 *
 * Returns '' for anything not usable, which every caller already treats as
 * "no panel".
 */
export function normalizePanelUrl(raw: string): string {
  const trimmed = raw.trim()
  if (!trimmed) return ''

  // A bare "panel.example.com:8006" is a host, not a scheme, so assume https.
  const candidate = /^[a-z][a-z0-9+.-]*:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`

  let parsed: URL
  try {
    parsed = new URL(candidate)
  } catch {
    return ''
  }

  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return ''
  if (parsed.username || parsed.password) return ''
  if (!parsed.hostname) return ''

  return parsed.toString()
}

/**
 * Whether a panel origin is safe enough to autofill a saved password into.
 *
 * Plaintext HTTP is normal for homelab hardware (routers, NAS boxes, IPMI) and
 * refusing it outright would break the feature for the people it was built for.
 * But on a public host the same behaviour hands the password to anyone on the
 * path, unprompted, seconds after the tab opens. So HTTP is allowed only where
 * the network itself is the trust boundary: loopback, RFC1918, link-local,
 * CGNAT and .local/.home names.
 */
export function mayAutofill(url: string): boolean {
  let parsed: URL
  try {
    parsed = new URL(url)
  } catch {
    return false
  }
  if (parsed.protocol === 'https:') return true
  if (parsed.protocol !== 'http:') return false
  return isPrivateHost(parsed.hostname)
}

function isPrivateHost(hostname: string): boolean {
  const host = hostname.toLowerCase().replace(/^\[|\]$/g, '')

  if (host === 'localhost' || host.endsWith('.localhost')) return true
  if (host.endsWith('.local') || host.endsWith('.home') || host.endsWith('.lan') || host.endsWith('.internal')) {
    return true
  }

  // IPv6 loopback and unique-local/link-local ranges.
  if (host === '::1') return true
  if (/^f[cd][0-9a-f]{2}:/.test(host)) return true
  if (/^fe80:/.test(host)) return true

  const v4 = host.match(/^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/)
  if (!v4) return false
  const a = Number(v4[1])
  const b = Number(v4[2])
  if (a === 127 || a === 10) return true
  if (a === 192 && b === 168) return true
  if (a === 172 && b >= 16 && b <= 31) return true
  if (a === 169 && b === 254) return true
  if (a === 100 && b >= 64 && b <= 127) return true
  return false
}
