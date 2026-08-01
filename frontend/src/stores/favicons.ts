import { writable } from 'svelte/store'
import { backend } from '../services/backend'

// Cached panel favicons as data: URLs, keyed by origin. The backend fetches and
// persists them; this store just memoizes what's been loaded this session so the
// sidebar doesn't ask twice for the same origin.
export const favicons = writable<Record<string, string>>({})

const inFlight = new Set<string>()

/** scheme://host[:port] for a URL, or '' if it can't be parsed. */
export function faviconOrigin(url: string): string {
  try {
    return new URL(/^[a-z][a-z0-9+.-]*:\/\//i.test(url) ? url : `https://${url}`).origin
  } catch {
    return ''
  }
}

/** Loads a host's favicon once per origin. Silent on failure (globe fallback). */
export async function ensureFavicon(url: string): Promise<void> {
  const origin = faviconOrigin(url)
  if (!origin || inFlight.has(origin)) return
  inFlight.add(origin)
  try {
    const dataURL = await backend.favicon(url)
    if (dataURL) favicons.update((all) => ({ ...all, [origin]: dataURL }))
  } catch {
    // A missing favicon is not worth surfacing; the sidebar shows the globe.
  }
}
