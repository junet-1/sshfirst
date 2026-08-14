<script lang="ts">
  import { onMount } from 'svelte'
  import { get } from 'svelte/store'
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import { SearchAddon } from '@xterm/addon-search'
  import { WebLinksAddon } from '@xterm/addon-web-links'
  import '@xterm/xterm/css/xterm.css'
  import { backend, on } from '../services/backend'
  import { isMac } from '../services/platform'
  import type { TerminalClosedEvent, TerminalDataEvent } from '../types/connection'
  import { activeConnectionId, activeTabId, markTabActivity, tabs, terminalSizes } from '../stores/connections'
  import { reconnectStates } from '../stores/reconnect'
  import type { Rect } from '../lib/layoutTree'
  import { normalizeTerminalFontSize, terminalFontSize, terminalSearchRequest } from '../stores/ui'
  import { notify } from '../stores/notifications'
  import { broadcastTargetsFor } from '../stores/broadcast'
  import ContextMenu from './ContextMenu.svelte'
  import PasteGuard from './PasteGuard.svelte'
  import ReconnectOverlay from './ReconnectOverlay.svelte'
  import TerminalSearchBar from './TerminalSearchBar.svelte'
  import type { ContextMenuItem } from '../types/contextMenu'

  export let tabId: string
  // visible: the pane is part of the current layout (in tabbed mode, the soloed
  // tab; in split mode, a tiled pane). focused: it owns keyboard focus, search
  // and the focus ring. rect: its box in percent when tiled, else null (the pane
  // fills the whole area via the default CSS).
  export let visible: boolean
  export let focused: boolean
  export let rect: Rect | null = null

  $: layerStyle = rect
    ? `left:${rect.left}%;top:${rect.top}%;width:${rect.width}%;height:${rect.height}%;right:auto;bottom:auto;`
    : ''
  // Shown on hover in split mode so a tiled pane can be traced back to its host.
  $: paneTitle = $tabs[tabId]?.title ?? ''

  function focusThisPane(): void {
    if (get(activeTabId) === tabId) return
    activeTabId.set(tabId)
    activeConnectionId.set(connectionId ?? null)
  }

  let container: HTMLDivElement
  let term: Terminal | undefined
  let fitAddon: FitAddon | undefined
  let searchAddon: SearchAddon | undefined
  let resizeObserver: ResizeObserver | undefined
  let contextMenu: { x: number; y: number } | null = null
  let appliedFontSize = 0
  let fontWheelDelta = 0
  let pendingPaste: string | null = null
  let searchOpen = false
  let searchQuery = ''
  let searchResultIndex = -1
  let searchResultCount = 0
  let handledSearchRequest = 0
  let searchResultsDisposable: { dispose(): void } | undefined
  // True once this pane's session has ended or the component is torn down.
  // Guards every backend call so we never resize/write to a tab the backend
  // has already dropped (which otherwise floods unhandled rejections).
  let inactive = false

  $: connectionId = $tabs[tabId]?.connectionId
  $: reconnectState = connectionId ? $reconnectStates[connectionId] : undefined
  $: inputBlocked = Boolean(reconnectState && reconnectState.phase !== 'reconnected') || pendingPaste !== null

  function currentTheme() {
    const styles = getComputedStyle(document.documentElement)
    return {
      background: styles.getPropertyValue('--terminal-bg').trim() || '#1b1e20',
      foreground: styles.getPropertyValue('--terminal-fg').trim() || '#eff0f1',
      cursor: styles.getPropertyValue('--accent-color').trim() || '#3daee9'
    }
  }

  function fit(): void {
    if (inactive || !fitAddon || !term) return
    try {
      fitAddon.fit()
    } catch {
      return // container not laid out yet (e.g. still hidden behind an inactive tab)
    }
    terminalSizes.update((all) => ({ ...all, [tabId]: { cols: term!.cols, rows: term!.rows } }))
    // Fire-and-forget: a resize racing against a just-closed tab is expected
    // and harmless, so swallow the rejection rather than leaking it.
    void backend.resizeTerminal(tabId, term.cols, term.rows).catch(() => {})
  }

  let refitFrame = 0
  // Coalesce bursts of resize events (window maximise, fullscreen toggle,
  // sidebar/inspector toggles) into a single fit on the next frame. Some
  // WebKitGTK builds don't reliably fire the ResizeObserver on a window
  // maximise, so this window-level listener guarantees a horizontal reflow.
  function scheduleFit(): void {
    if (refitFrame) cancelAnimationFrame(refitFrame)
    refitFrame = requestAnimationFrame(() => {
      refitFrame = 0
      if (visible) fit()
    })
  }

  function stopSession(): void {
    inactive = true
    resizeObserver?.disconnect()
    resizeObserver = undefined
  }

  function resumeSession(): void {
    if (!term || !container) return
    inactive = false
    if (!resizeObserver) {
      resizeObserver = new ResizeObserver(() => scheduleFit())
      resizeObserver.observe(container)
    }
    scheduleFit()
  }

  function copySelection(): void {
    const selection = term?.getSelection()
    if (selection) void navigator.clipboard.writeText(selection).catch(() => {})
  }

  function pasteClipboard(): void {
    if (inputBlocked) return
    void navigator.clipboard
      .readText()
      .then((text) => {
        if (text && !inactive) requestPaste(text)
      })
      .catch(() => {})
  }

  function requestPaste(text: string): void {
    if (!text || inactive || inputBlocked) return
    if (/\r|\n/.test(text) || text.length >= 200) {
      pendingPaste = text
      return
    }
    term?.paste(text)
  }

  function onNativePaste(event: ClipboardEvent): void {
    if (inactive || inputBlocked) return
    const text = event.clipboardData?.getData('text/plain') ?? ''
    if (!text) return
    event.preventDefault()
    event.stopImmediatePropagation()
    requestPaste(text)
  }

  function confirmPaste(): void {
    const text = pendingPaste
    pendingPaste = null
    // Let Svelte release the input block before xterm emits the pasted data.
    setTimeout(() => {
      if (text && !inactive) term?.paste(text)
      term?.focus()
    }, 0)
  }

  function cancelPaste(): void {
    pendingPaste = null
    term?.focus()
  }

  const searchOptions = {
    decorations: {
      matchBackground: '#46515c',
      matchBorder: '#7c8792',
      matchOverviewRuler: '#7c8792',
      activeMatchBackground: '#2876a8',
      activeMatchBorder: '#66b8ef',
      activeMatchColorOverviewRuler: '#3daee9'
    }
  }

  function updateSearch(query: string): void {
    searchQuery = query
    if (!searchAddon || !query) {
      searchAddon?.clearDecorations()
      searchResultIndex = -1
      searchResultCount = 0
      return
    }
    searchAddon.findNext(query, { ...searchOptions, incremental: true })
  }

  function findNext(): void {
    if (searchQuery) searchAddon?.findNext(searchQuery, searchOptions)
  }

  function findPrevious(): void {
    if (searchQuery) searchAddon?.findPrevious(searchQuery, searchOptions)
  }

  function closeSearch(): void {
    searchOpen = false
    searchAddon?.clearDecorations()
    term?.focus()
  }

  function activateLink(event: MouseEvent, uri: string): void {
    if (!(event.ctrlKey || event.metaKey) || !/^https?:\/\//i.test(uri)) return
    event.preventDefault()
    backend.openExternalURL(uri)
  }

  function adjustTerminalFontSize(delta: number): void {
    const next = normalizeTerminalFontSize($terminalFontSize + delta)
    if (next === $terminalFontSize) return
    terminalFontSize.set(next)
    void backend.setSetting('terminalFontSize', String(next)).catch(() => {})
  }

  function terminalBufferText(): string {
    if (!term) return ''
    const buffer = term.buffer.normal
    let output = ''
    let hasLine = false

    for (let index = 0; index < buffer.length; index += 1) {
      const line = buffer.getLine(index)
      if (!line) continue
      if (hasLine && !line.isWrapped) output += '\n'
      output += line.translateToString(true)
      hasLine = true
    }

    return output.replace(/[ \t\n]+$/, '')
  }

  function clearScreen(): void {
    if (inactive || inputBlocked) return
    // Form feed is the shell-native clear-screen action and therefore keeps
    // the remote PTY and local cursor state synchronized while retaining
    // scrollback.
    void backend.writeTerminalInput(tabId, '\x0c').catch(() => {})
  }

  function clearScrollback(): void {
    term?.clear()
  }

  function resetTerminal(): void {
    term?.reset()
    scheduleFit()
  }

  function copyAllOutput(): void {
    const output = terminalBufferText()
    if (!output) {
      notify('info', 'The terminal has no output to copy.')
      return
    }
    void navigator.clipboard
      .writeText(output)
      .then(() => notify('info', 'Copied all terminal output.'))
      .catch((error) => notify('error', `Could not copy terminal output: ${String(error)}`))
  }

  function transcriptFilename(): string {
    const title = $tabs[tabId]?.title ?? 'terminal'
    const safeTitle = title.replace(/[<>:"/\\|?*\x00-\x1f]/g, '-').trim() || 'terminal'
    const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19)
    return `${safeTitle}-${timestamp}.txt`
  }

  async function exportAllOutput(): Promise<void> {
    const output = terminalBufferText()
    if (!output) {
      notify('info', 'The terminal has no output to export.')
      return
    }
    try {
      const path = await backend.exportTerminalOutput(transcriptFilename(), `${output}\n`)
      if (path) notify('info', `Terminal output exported to ${path}`)
    } catch (error) {
      notify('error', `Could not export terminal output: ${error instanceof Error ? error.message : String(error)}`)
    }
  }

  function onContextMenu(e: MouseEvent): void {
    e.preventDefault()
    contextMenu = { x: e.clientX, y: e.clientY }
  }

  $: contextItems = [
    { id: 'copy', label: 'Copy' },
    { id: 'paste', label: 'Paste' },
    { id: 'selectAll', label: 'Select All', separatorBefore: true },
    { id: 'clearScreen', label: 'Clear Screen', separatorBefore: true },
    { id: 'clearScrollback', label: 'Clear Scrollback' },
    { id: 'resetTerminal', label: 'Reset Terminal' },
    { id: 'copyAll', label: 'Copy All Output', separatorBefore: true },
    { id: 'exportAll', label: 'Export Output…' }
  ] satisfies ContextMenuItem[]

  function onContextSelect(id: string): void {
    if (id === 'copy') copySelection()
    else if (id === 'paste') pasteClipboard()
    else if (id === 'selectAll') term?.selectAll()
    else if (id === 'clearScreen') clearScreen()
    else if (id === 'clearScrollback') clearScrollback()
    else if (id === 'resetTerminal') resetTerminal()
    else if (id === 'copyAll') copyAllOutput()
    else if (id === 'exportAll') void exportAllOutput()
    term?.focus()
  }

  onMount(() => {
    try {
      term = new Terminal({
        fontFamily: "'Cascadia Code', 'JetBrains Mono', 'Fira Code', ui-monospace, monospace",
        fontSize: $terminalFontSize,
        lineHeight: 1.2,
        cursorBlink: true,
        scrollback: 10000,
        theme: currentTheme(),
        linkHandler: {
          activate: activateLink,
          allowNonHttpProtocols: false
        }
      })
      fitAddon = new FitAddon()
      searchAddon = new SearchAddon()
      term.loadAddon(fitAddon)
      term.loadAddon(searchAddon)
      term.loadAddon(new WebLinksAddon(activateLink))
      term.open(container)
      container.addEventListener('paste', onNativePaste, true)
      searchResultsDisposable = searchAddon.onDidChangeResults((results) => {
        searchResultIndex = results.resultIndex
        searchResultCount = results.resultCount
      })
      fit()
      if (focused) term.focus()

      term.onData((data) => {
        if (inactive || inputBlocked) return
        const targets = broadcastTargetsFor(tabId)
        if (targets) {
          for (const t of targets) void backend.writeTerminalInput(t, data).catch(() => {})
        } else {
          void backend.writeTerminalInput(tabId, data).catch(() => {})
        }
      })

      // Konsole-style copy/paste. Ctrl+C is left alone (it must reach the
      // shell as SIGINT), so copy is Ctrl/Cmd+Shift+C and paste is
      // Ctrl/Cmd+Shift+V. Paste deliberately stays a native browser action:
      // WebKitGTK exposes external clipboard contents reliably on the paste
      // event, but not consistently through navigator.clipboard.readText().
      //
      // macOS additionally gets plain ⌘C/⌘V, which is what every terminal
      // there uses: Command is not a modifier the shell can see, so unlike
      // Ctrl+C it is free to mean copy.
      //
      // The Edit menu's standard Copy/Paste roles claim those two shortcuts
      // first, but AppKit only lets an *enabled* menu item swallow a key
      // equivalent, and it asks the web view whether it can copy. A terminal
      // selection is drawn by xterm rather than being a DOM selection, so the
      // answer is no and the event arrives here. Paste is the other way round —
      // the hidden textarea is editable, so WebKit handles ⌘V itself and the
      // branch below is only a fallback.
      term.attachCustomKeyEventHandler((e) => {
        if (e.type !== 'keydown') return true
        const mod = e.ctrlKey || e.metaKey
        const command = isMac && e.metaKey && !e.ctrlKey && !e.shiftKey
        if (command && e.key.toLowerCase() === 'c' && term?.hasSelection()) {
          e.preventDefault()
          copySelection()
          return false
        }
        if (command && e.key.toLowerCase() === 'v') {
          return false
        }
        if (mod && e.shiftKey && e.key.toLowerCase() === 'c') {
          e.preventDefault()
          copySelection()
          return false
        }
        if (mod && e.shiftKey && e.key.toLowerCase() === 'v') {
          // Stop xterm from sending Ctrl+V to the PTY, but do not prevent the
          // browser default. The resulting paste event is intercepted by
          // onNativePaste above and routed through the paste guard.
          return false
        }
        return true
      })

      // Keep the regular wheel behavior for terminal scrollback. With Ctrl
      // (or Cmd), consume the event here so WebKit does not zoom the whole
      // application and adjust the shared terminal font size instead.
      term.attachCustomWheelEventHandler((event) => {
        if (!(event.ctrlKey || event.metaKey)) {
          fontWheelDelta = 0
          return true
        }

        event.preventDefault()
        event.stopPropagation()

        if (fontWheelDelta !== 0 && Math.sign(fontWheelDelta) !== Math.sign(event.deltaY)) {
          fontWheelDelta = 0
        }
        fontWheelDelta += event.deltaY

        // Physical mouse wheels usually report a large pixel delta, while
        // trackpads emit many tiny deltas. Accumulating the latter keeps zoom
        // changes deliberate and limited to one font-size step at a time.
        const threshold = event.deltaMode === 1 ? 1 : 40
        if (Math.abs(fontWheelDelta) >= threshold) {
          adjustTerminalFontSize(fontWheelDelta < 0 ? 1 : -1)
          fontWheelDelta = 0
        }

        return false
      })
    } catch (e) {
      // A terminal that fails to initialise must never take down the whole
      // Svelte flush (which would freeze the entire UI); degrade gracefully.
      void backend.logFrontendError(`terminal init failed for tab ${tabId}: ${e instanceof Error ? (e.stack ?? e.message) : String(e)}`)
    }

    const offData = on<TerminalDataEvent>('terminal:data', (evt) => {
      if (evt.tabId !== tabId) return
      // Only mark unread/bell attention for off-screen panes. A tiled (visible)
      // pane is already in view, so its redraw output — e.g. during a resize —
      // must not light the tab up as a notification.
      if (!visible) markTabActivity(tabId, evt.data.includes('\x07'))
      term?.write(evt.data)
    })
    const offClosed = on<TerminalClosedEvent>('terminal:closed', (evt) => {
      if (evt.tabId !== tabId) return
      // Connection lifecycle belongs to the native overlay/status UI. Never
      // mutate the terminal buffer with synthetic disconnect/reconnect text.
      stopSession() // session is gone on the backend — stop resizing/writing to it
    })

    resizeObserver = new ResizeObserver(() => fit())
    resizeObserver.observe(container)

    return () => {
      stopSession()
      offData()
      offClosed()
      container?.removeEventListener('paste', onNativePaste, true)
      searchResultsDisposable?.dispose()
      searchResultsDisposable = undefined
      const t = term
      term = undefined
      fitAddon = undefined
      searchAddon = undefined
      t?.dispose()
    }
  })

  // Every visible pane fits its box; only the focused one grabs the keyboard.
  $: if (visible && term && !inactive) fit()
  $: if (focused && term && !inactive) term.focus()

  $: if (term && appliedFontSize !== $terminalFontSize) {
    appliedFontSize = $terminalFontSize
    term.options.fontSize = $terminalFontSize
    scheduleFit()
  }

  $: if ($terminalSearchRequest > handledSearchRequest) {
    handledSearchRequest = $terminalSearchRequest
    if (focused && term) searchOpen = true
  }

  // An xterm instance is never recreated for reconnect. If the old PTY had
  // already reported closed before keepalive detected the outage, reactivate
  // the same viewport once its replacement session is installed.
  $: if (reconnectState?.phase === 'reconnected' && term) {
    resumeSession()
  }
</script>

<svelte:window on:resize={scheduleFit} />

<div class="terminal-layer" class:visible class:focused aria-hidden={!visible} style={layerStyle}>
  <!-- svelte-ignore a11y-no-static-element-interactions -->
  <div class="pane" bind:this={container} on:pointerdown={focusThisPane} on:contextmenu={onContextMenu} />

  <div class="pane-label">{paneTitle}</div>

  {#if reconnectState}
    <ReconnectOverlay state={reconnectState} />
  {/if}

  {#if searchOpen && focused}
    <TerminalSearchBar
      query={searchQuery}
      resultIndex={searchResultIndex}
      resultCount={searchResultCount}
      on:query={(event) => updateSearch(event.detail)}
      on:next={findNext}
      on:previous={findPrevious}
      on:close={closeSearch}
    />
  {/if}

  {#if pendingPaste !== null && focused}
    <PasteGuard content={pendingPaste} on:confirm={confirmPaste} on:cancel={cancelPaste} />
  {/if}
</div>

{#if contextMenu}
  <ContextMenu
    x={contextMenu.x}
    y={contextMenu.y}
    items={contextItems}
    on:select={(e) => onContextSelect(e.detail)}
    on:close={() => (contextMenu = null)}
  />
{/if}

<style>
  .terminal-layer {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    visibility: hidden;
    pointer-events: none;
  }

  .terminal-layer.visible {
    visibility: visible;
    pointer-events: auto;
  }

  /* In split mode a focus ring marks the pane that takes keyboard input. The
     :global guard keys off the split class the TerminalArea sets on its wrapper
     so the ring never shows in ordinary tabbed mode. */
  :global(.terminal-area.split) .terminal-layer.focused {
    outline: 2px solid var(--accent-color);
    outline-offset: -2px;
    z-index: 1;
  }

  /* Host label: hidden by default, revealed on hover but only while tiled. */
  .pane-label {
    position: absolute;
    top: 5px;
    left: 7px;
    z-index: 2;
    max-width: 85%;
    padding: 1px 7px;
    border-radius: 3px;
    border: 1px solid var(--border-color);
    background: var(--header-bg);
    color: var(--text-color);
    font-size: 11px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    pointer-events: none;
    opacity: 0;
    transition: opacity 0.1s ease;
  }

  :global(.terminal-area.split) .terminal-layer:hover .pane-label {
    opacity: 1;
  }

  .pane {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    overflow: hidden;
    padding: 4px 0 0 6px;
    background: var(--terminal-bg);
  }

  .pane :global(.xterm) {
    height: 100%;
  }
</style>
