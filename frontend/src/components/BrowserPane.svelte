<script lang="ts">
  import Icon from './Icon.svelte'
  import { t } from '../services/i18n'
  import { backend } from '../services/backend'
  import { ensureFavicon, faviconOrigin, favicons } from '../stores/favicons'
  import { mayAutofill, normalizePanelUrl } from '../lib/panelUrl'
  import type { Rect } from '../lib/layoutTree'
  import { onMount } from 'svelte'

  // visible: this pane is part of the current layout (soloed tab, or a tiled
  // pane in split mode). rect: its box in percent when tiled, else null.
  export let tabId: string
  export let url: string
  export let resourceHostId: number | undefined = undefined
  export let visible: boolean
  export let rect: Rect | null = null

  let iframeEl: HTMLIFrameElement | null = null
  let autofillTimers: ReturnType<typeof setTimeout>[] = []
  let credentials: { email: string; password: string } | null = null
  let credentialsHostId: number | undefined
  let autofillComplete = false

  // Defence in depth. Every caller already routes through normalizePanelUrl,
  // but this is the sink a hostile scheme would actually execute in, so the URL
  // is vetted again immediately before it becomes the frame's src. about:blank
  // rather than '' for a rejected URL: an empty src resolves to the app's own
  // document, which would load the shell inside its own iframe.
  $: frameUrl = normalizePanelUrl(url) || 'about:blank'

  // The panel's favicon via the shared backend cache, globe as fallback.
  $: void ensureFavicon(url)
  $: favicon = $favicons[faviconOrigin(url)]

  $: layerStyle = rect
    ? `left:${rect.left}%;top:${rect.top}%;width:${rect.width}%;height:${rect.height}%;right:auto;bottom:auto;`
    : ''

  // Reassigning src reloads the frame even for a cross-origin panel, where
  // contentWindow.location.reload() would throw.
  function reload(): void {
    // An explicit reload is also the opt-in to retry autofill after logout or
    // after correcting a saved password.
    autofillComplete = false
    credentials = null
    credentialsHostId = undefined
    clearAutofillTimers()
    if (iframeEl) iframeEl.src = frameUrl
  }

  function clearAutofillTimers(): void {
    for (const timer of autofillTimers) clearTimeout(timer)
    autofillTimers = []
  }

  async function loadAutofillCredentials(): Promise<{ email: string; password: string } | null> {
    if (!resourceHostId) return null
    if (credentialsHostId === resourceHostId && credentials) return credentials
    const [email, password] = await Promise.all([
      backend.hostUsername(resourceHostId),
      backend.webPassword(resourceHostId)
    ])
    credentialsHostId = resourceHostId
    // Some panels (notably FRITZ!Box) preselect the account and only ask for a
    // password. A missing username must not suppress password-only autofill.
    credentials = password ? { email, password } : null
    return credentials
  }

  // WebKit injects the receiver into the cross-origin frame. Messages are sent
  // only to the exact configured origin; repeating briefly also covers SPAs and
  // two-step login pages that render or navigate after the iframe load event.
  async function autofill(): Promise<void> {
    clearAutofillTimers()
    if (autofillComplete || !iframeEl?.contentWindow || !resourceHostId) return
    // A plaintext panel on a routable address would hand the password to
    // anyone on the path, with no user action beyond opening the tab. Homelab
    // hardware on the local network is exempt — see lib/panelUrl.
    if (!mayAutofill(url)) return
    try {
      const targetOrigin = new URL(url).origin
      const saved = await loadAutofillCredentials()
      if (!saved || !iframeEl?.contentWindow) return
      const send = () => iframeEl?.contentWindow?.postMessage({
        type: 'ssh-first:web-autofill',
        targetOrigin,
        email: saved.email,
        password: saved.password
      }, targetOrigin)
      for (const delay of [0, 250, 750, 1500, 3000, 6000, 10000]) {
        autofillTimers.push(setTimeout(send, delay))
      }
    } catch {
      // Invalid URLs are rejected by the host editor; a stale workspace entry
      // should simply keep ordinary browser behavior instead of failing a tab.
    }
  }

  onMount(() => {
    const handleAutofillComplete = (event: MessageEvent) => {
      if (event.source !== iframeEl?.contentWindow) return
      if (!event.data || event.data.type !== 'ssh-first:web-autofill-submitted') return
      try {
        if (event.origin !== new URL(url).origin || event.data.targetOrigin !== event.origin) return
      } catch {
        return
      }
      autofillComplete = true
      clearAutofillTimers()
    }
    window.addEventListener('message', handleAutofillComplete)
    return () => {
      window.removeEventListener('message', handleAutofillComplete)
      clearAutofillTimers()
    }
  })

  function openExternally(): void {
    // The vetted URL, so a rejected scheme is not handed to the system browser
    // either.
    backend.openExternalURL(frameUrl)
  }
</script>

<div class="browser-layer" class:visible aria-hidden={!visible} style={layerStyle}>
  <div class="panel-toolbar">
    <span class="panel-icon">
      {#if favicon}
        <img class="favicon" src={favicon} alt="" />
      {:else}
        <Icon name="globe" size={12} />
      {/if}
    </span>
    <span class="panel-url" title={url}>{url}</span>
    <button class="tool" title={$t('browser.reload')} aria-label={$t('browser.reload')} on:click={reload}>
      <Icon name="refresh" size={12} />
    </button>
    <button
      class="tool"
      title={$t('browser.openExternal')}
      aria-label={$t('browser.openExternal')}
      on:click={openExternally}
    >
      <Icon name="link" size={12} />
    </button>
  </div>
  <div class="frame-wrap">
    <!-- A cross-origin iframe cannot reach the app's Wails bindings (same-origin
         policy), so the panel is isolated from the SSH backend by construction. -->
    <iframe
      bind:this={iframeEl}
      src={frameUrl}
      title={$t('browser.frameTitle')}
      data-tab-id={tabId}
      on:load={autofill}
    ></iframe>
    <p class="blocked-hint">
      {$t('browser.blockedHint')}
    </p>
  </div>
</div>

<style>
  .browser-layer {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    display: flex;
    flex-direction: column;
    background: var(--view-bg);
    visibility: hidden;
    pointer-events: none;
  }

  .browser-layer.visible {
    visibility: visible;
    pointer-events: auto;
  }

  .panel-toolbar {
    display: flex;
    align-items: center;
    gap: 6px;
    height: 26px;
    padding: 0 6px;
    background: var(--header-bg);
    border-bottom: 1px solid var(--border-color);
    flex-shrink: 0;
  }

  .panel-icon {
    display: inline-flex;
    align-items: center;
    color: var(--accent-color);
    flex-shrink: 0;
  }

  .favicon {
    width: 14px;
    height: 14px;
    object-fit: contain;
    border-radius: 2px;
  }

  .panel-url {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 11.5px;
    color: var(--text-color-secondary);
  }

  .tool {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    padding: 0;
    background: transparent;
    border: none;
    color: var(--text-color-secondary);
    flex-shrink: 0;
  }

  .tool:hover {
    background: var(--hover-bg);
    color: var(--text-color);
  }

  .frame-wrap {
    position: relative;
    flex: 1;
    min-height: 0;
  }

  iframe {
    position: relative;
    z-index: 1;
    width: 100%;
    height: 100%;
    border: none;
    /* Transparent (not white) so that a panel which refuses embedding leaves an
       empty frame, letting the .blocked-hint behind it show through. A panel
       that loads paints its own body background over the hint. */
    background: transparent;
  }

  /* Sits behind the iframe. When a panel refuses embedding (X-Frame-Options /
     frame-ancestors), the frame paints nothing and this hint shows through. */
  .blocked-hint {
    position: absolute;
    inset: 0;
    z-index: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    margin: 0;
    padding: 24px;
    text-align: center;
    color: var(--text-color-secondary);
    font-size: 12px;
    line-height: 1.5;
  }
</style>
