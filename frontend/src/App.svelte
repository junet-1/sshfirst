<script lang="ts">
  import { onMount } from 'svelte'
  import { get } from 'svelte/store'

  import Toolbar from './components/Toolbar.svelte'
  import Sidebar from './components/Sidebar.svelte'
  import TabBar from './components/TabBar.svelte'
  import TerminalArea from './components/TerminalArea.svelte'
  import Inspector from './components/Inspector.svelte'
  import StatusBar from './components/StatusBar.svelte'
  import CommandPalette from './components/CommandPalette.svelte'
  import NotificationBanner from './components/NotificationBanner.svelte'
  import ToolWindow from './components/ToolWindow.svelte'
  import PasswordPromptDialog from './components/dialogs/PasswordPromptDialog.svelte'
  import PassphrasePromptDialog from './components/dialogs/PassphrasePromptDialog.svelte'
  import HostKeyPromptDialog from './components/dialogs/HostKeyPromptDialog.svelte'
  import KeyboardInteractivePromptDialog from './components/dialogs/KeyboardInteractivePromptDialog.svelte'
  import ConfirmDialog from './components/dialogs/ConfirmDialog.svelte'
  import WorkspaceDialog from './components/dialogs/WorkspaceDialog.svelte'

  import { backend, on } from './services/backend'
  import { locale, type Locale } from './services/i18n'
  import { confirmAndDeleteHost, hosts, importSSHConfigWithFeedback, loadHosts } from './stores/hosts'
  import {
    activeConnectionId,
    activeTabId,
    closeTab,
    connections,
    connectOrFocusHost,
    disconnectConnection,
    duplicateTab,
    initConnectionEvents,
    loadRecentConnections,
    openNewTab,
    reopenLastClosedTab,
    tabs
  } from './stores/connections'
  import { initReconnectEvents, reconnectConnection } from './stores/reconnect'
  import { broadcastActive, setBroadcast } from './stores/broadcast'
  import { initPromptEvents } from './stores/prompts'
  import {
    applyTheme,
    aboutOpen,
    commandPaletteOpen,
    DEFAULT_TERMINAL_FONT_SIZE,
    folderDialog,
    hostDialog,
    inspectorVisible,
    credentialsOpen,
    normalizeTerminalFontSize,
    selectedHostId,
    settingsOpen,
    showRecent,
    sidebarVisible,
    snippetsOpen,
    terminalFontSize,
    terminalSearchRequest,
    theme,
    type ThemePreference
  } from './stores/ui'
  import type { Host } from './types/host'
  import { loadSnippets } from './stores/snippets'
  import { initTransferEvents, transferDialogHostId } from './stores/transfer'
  import { forwardingDialogHostId, initForwardingEvents } from './stores/forwarding'
  import { initDiscoveryEvents } from './stores/discovery'
  import { currentToolWindowKind } from './services/windowing'
  import { withErrorBanner } from './stores/notifications'
  import { workspaceDialogOpen } from './stores/workspaces'

  let sidebar: Sidebar

  interface TrayConnectEvent {
    hostId: number
    hostLabel: string
  }

  onMount(() => {
    if (currentToolWindowKind) return

    initConnectionEvents()
    initReconnectEvents()
    initPromptEvents()
    initTransferEvents()
    initForwardingEvents()
    initDiscoveryEvents()
    void loadHosts()
    void loadSettings()
    void loadSnippets(0)
    void loadRecentConnections()

    const offMenu = on<string>('menu:action', (action) => void handleMenuAction(action))
    const offTrayConnect = on<TrayConnectEvent>('tray:connect-host', ({ hostId, hostLabel }) => {
      void connectOrFocusHost(hostId, hostLabel)
    })
    const offHostsChanged = on<null>('hosts:changed', () => void loadHosts())
    const offFoldersChanged = on<null>('folders:changed', () => void loadHosts())
    const offSettingsChanged = on<null>('settings:changed', () => void loadSettings())
    const offSnippetRun = on<{ command: string }>('snippet:run', ({ command }) => {
      const tabId = get(activeTabId)
      if (tabId) void backend.sendToTab(tabId, command.endsWith('\n') ? command : `${command}\n`)
    })

    return () => {
      offMenu()
      offTrayConnect()
      offHostsChanged()
      offFoldersChanged()
      offSettingsChanged()
      offSnippetRun()
    }
  })

  async function loadSettings(): Promise<void> {
    const [themeSetting, localeSetting, showRecentSetting, terminalFontSizeSetting] = await Promise.all([
      backend.getSetting('theme'),
      backend.getSetting('locale'),
      backend.getSetting('showRecent'),
      backend.getSetting('terminalFontSize')
    ])
    if (themeSetting.exists) {
      theme.set(themeSetting.value as ThemePreference)
      applyTheme(themeSetting.value as ThemePreference)
    }
    if (localeSetting.exists) {
      locale.set(localeSetting.value as Locale)
    }
    if (showRecentSetting.exists) {
      showRecent.set(showRecentSetting.value === 'true')
    }
    if (terminalFontSizeSetting.exists) {
      terminalFontSize.set(normalizeTerminalFontSize(Number(terminalFontSizeSetting.value)))
    }
  }

  function setTerminalFontSize(value: number): void {
    const next = normalizeTerminalFontSize(value)
    terminalFontSize.set(next)
    void backend.setSetting('terminalFontSize', String(next))
  }

  function toggleBroadcast(): void {
    setBroadcast(!get(broadcastActive), Object.keys(get(tabs)))
  }

  function selectedHost(): Host | null {
    const id = get(selectedHostId)
    return id != null ? (get(hosts).find((h) => h.id === id) ?? null) : null
  }

  function activeHost(): Host | null {
    const connId = get(activeConnectionId)
    if (connId) {
      const hostId = get(connections)[connId]?.hostId
      return hostId != null ? (get(hosts).find((h) => h.id === hostId) ?? null) : null
    }
    return selectedHost()
  }

  function launchHostWindow(editingId: number | null): void {
    hostDialog.set({ open: false, editingId: null })
    void withErrorBanner(() => backend.openToolWindow('host', editingId ?? -1))
  }

  function launchFolderWindow(editingId: number | null, parentId: number | null): void {
    folderDialog.set({ open: false, editingId: null, parentId: null })
    void withErrorBanner(() => backend.openToolWindow('folder', editingId ?? -1, parentId ?? -1))
  }

  function launchSingletonWindow(kind: 'settings' | 'about'): void {
    if (kind === 'settings') settingsOpen.set(false)
    else aboutOpen.set(false)
    void withErrorBanner(() => backend.openToolWindow(kind))
  }

  function launchSnippetsWindow(): void {
    snippetsOpen.set(false)
    void withErrorBanner(() => backend.openToolWindow('snippets', activeHost()?.id ?? -1))
  }

  function launchCredentialsWindow(): void {
    credentialsOpen.set(false)
    void withErrorBanner(() => backend.openToolWindow('credentials'))
  }

  function launchHostToolWindow(kind: 'transfer' | 'forwarding', hostId: number): void {
    if (kind === 'transfer') transferDialogHostId.set(null)
    else forwardingDialogHostId.set(null)
    void withErrorBanner(() => backend.openToolWindow(kind, hostId))
  }

  $: if (!currentToolWindowKind && $hostDialog.open) {
    launchHostWindow($hostDialog.editingId)
  }
  $: if (!currentToolWindowKind && $folderDialog.open) {
    launchFolderWindow($folderDialog.editingId, $folderDialog.parentId)
  }
  $: if (!currentToolWindowKind && $settingsOpen) launchSingletonWindow('settings')
  $: if (!currentToolWindowKind && $aboutOpen) launchSingletonWindow('about')
  $: if (!currentToolWindowKind && $snippetsOpen) launchSnippetsWindow()
  $: if (!currentToolWindowKind && $credentialsOpen) launchCredentialsWindow()
  $: if (!currentToolWindowKind && $transferDialogHostId != null) {
    launchHostToolWindow('transfer', $transferDialogHostId)
  }
  $: if (!currentToolWindowKind && $forwardingDialogHostId != null) {
    launchHostToolWindow('forwarding', $forwardingDialogHostId)
  }

  function cycleTab(delta: number): void {
    const list = Object.values(get(tabs))
    if (list.length === 0) return
    const currentIndex = list.findIndex((tab) => tab.tabId === get(activeTabId))
    const nextIndex = (currentIndex + delta + list.length) % list.length
    const next = list[nextIndex]
    if (!next) return
    activeTabId.set(next.tabId)
    activeConnectionId.set(
      next.kind === 'quick-connect' || next.kind === 'browser' || next.kind === 'connection-attempt'
        ? null
        : next.connectionId
    )
  }

  async function copySSHCommandForActiveHost(): Promise<void> {
    const host = activeHost()
    if (host) {
      if (host.protocol === 'web') {
        if (host.controlPanelUrl) await navigator.clipboard.writeText(host.controlPanelUrl)
        return
      }
      const command = host.protocol === 'sftp' ? 'sftp' : 'ssh'
      const portFlag = host.protocol === 'sftp' ? '-P' : '-p'
      const port = host.port !== 22 ? ` ${portFlag} ${host.port}` : ''
      const userPart = host.user ? `${host.user}@` : ''
      await navigator.clipboard.writeText(`${command}${port} ${userPart}${host.hostname}`)
      return
    }
    const connectionId = get(activeConnectionId)
    const target = connectionId ? get(connections)[connectionId]?.quickTarget : undefined
    if (target) await navigator.clipboard.writeText(target.startsWith('ssh ') ? target : `ssh ${target}`)
  }

  async function handleMenuAction(action: string): Promise<void> {
    switch (action) {
      case 'file.newHost':
        hostDialog.set({ open: true, editingId: null })
        break
      case 'file.importConfig':
        await importSSHConfigWithFeedback()
        break
      case 'file.settings':
        settingsOpen.set(true)
        break
      case 'file.workspaces':
        workspaceDialogOpen.set(true)
        break
      case 'edit.editHost': {
        const host = selectedHost()
        if (host) hostDialog.set({ open: true, editingId: host.id })
        break
      }
      case 'edit.deleteHost': {
        const host = selectedHost()
        if (host) await confirmAndDeleteHost(host)
        break
      }
      case 'edit.rename':
        sidebar?.renameSelected()
        break
      case 'edit.findTerminal':
        if (get(activeTabId)) terminalSearchRequest.update((value) => value + 1)
        break
      case 'view.toggleSidebar':
        sidebarVisible.update((v) => !v)
        break
      case 'view.toggleInspector':
        inspectorVisible.update((v) => !v)
        break
      case 'view.toggleRecent': {
        let next = true
        showRecent.update((v) => (next = !v))
        await backend.setSetting('showRecent', String(next))
        break
      }
      case 'view.maximize':
        await backend.toggleMaximise()
        break
      case 'view.fullscreen':
        await backend.toggleFullscreen()
        break
      case 'session.connect': {
        const host = selectedHost()
        if (host) await connectOrFocusHost(host.id, host.label)
        break
      }
      case 'session.disconnect': {
        const id = get(activeConnectionId)
        if (id) await disconnectConnection(id)
        break
      }
      case 'session.reconnect': {
        const id = get(activeConnectionId)
        if (id) await reconnectConnection(id)
        break
      }
      case 'session.newTab': {
        openNewTab()
        break
      }
      case 'session.closeTab': {
        const id = get(activeTabId)
        if (id) await closeTab(id)
        break
      }
      case 'session.duplicateTab': {
        const id = get(activeTabId)
        if (id) await duplicateTab(id)
        break
      }
      case 'session.reopenClosedTab':
        await reopenLastClosedTab()
        break
      case 'session.copySSHCommand':
        await copySSHCommandForActiveHost()
        break
      case 'session.broadcast':
        toggleBroadcast()
        break
      case 'tools.commandPalette':
        commandPaletteOpen.set(true)
        break
      case 'tools.credentials':
        credentialsOpen.set(true)
        break
      case 'tools.snippets':
        snippetsOpen.set(true)
        break
      case 'help.about':
        aboutOpen.set(true)
        break
    }
  }

  function isCtrl(e: KeyboardEvent): boolean {
    return e.ctrlKey || e.metaKey
  }

  // Plain Ctrl+<letter> belongs to whatever runs in the terminal: Ctrl+K cuts a
  // line in nano, Ctrl+W searches there and deletes a word in readline, Ctrl+N
  // and Ctrl+T are history and transpose. This listener runs in the capture
  // phase, so a shortcut it claims never reaches xterm at all — the conflicting
  // ones therefore have to stand down while a terminal has focus rather than
  // just skipping preventDefault.
  //
  // Application shortcuts keep their Ctrl+Shift form, which no shell binding
  // uses; the non-letter ones (Ctrl+Tab, Ctrl+comma, zoom) collide with nothing
  // and stay available everywhere.
  function terminalHasFocus(): boolean {
    const active = document.activeElement
    return active instanceof Element && active.closest('.xterm') !== null
  }

  function onGlobalKeydown(e: KeyboardEvent): void {
    if (currentToolWindowKind) return
    const key = e.key.toLowerCase()
    const shellOwnsLetter = terminalHasFocus() && !e.shiftKey

    if (isCtrl(e) && (key === '+' || key === '=' || e.code === 'NumpadAdd')) {
      e.preventDefault()
      e.stopPropagation()
      setTerminalFontSize(get(terminalFontSize) + 1)
      return
    }
    if (isCtrl(e) && (key === '-' || e.code === 'NumpadSubtract')) {
      e.preventDefault()
      e.stopPropagation()
      setTerminalFontSize(get(terminalFontSize) - 1)
      return
    }
    if (isCtrl(e) && (key === '0' || e.code === 'Numpad0')) {
      e.preventDefault()
      e.stopPropagation()
      setTerminalFontSize(DEFAULT_TERMINAL_FONT_SIZE)
      return
    }

    // Ctrl+Shift+Space toggles the command palette (launcher-style), in
    // addition to Ctrl+Shift+P. Kept as Ctrl-based so KWin doesn't grab it,
    // and Shift+Space avoids clobbering the shell's Ctrl+Space (set-mark).
    if (isCtrl(e) && e.shiftKey && e.code === 'Space') {
      e.preventDefault()
      e.stopPropagation()
      commandPaletteOpen.update((v) => !v)
      return
    }

    if (isCtrl(e) && e.shiftKey && key === 'f') {
      e.preventDefault()
      e.stopPropagation()
      if (get(activeTabId)) terminalSearchRequest.update((value) => value + 1)
    } else if (isCtrl(e) && e.shiftKey && key === 't') {
      e.preventDefault()
      e.stopPropagation()
      void reopenLastClosedTab()
    } else if (isCtrl(e) && key === 'k' && (e.shiftKey || !shellOwnsLetter)) {
      e.preventDefault()
      e.stopPropagation()
      sidebar?.focusSearch()
    } else if (isCtrl(e) && key === 'n' && !shellOwnsLetter) {
      e.preventDefault()
      e.stopPropagation()
      hostDialog.set({ open: true, editingId: null })
    } else if (isCtrl(e) && key === 'tab') {
      e.preventDefault()
      e.stopPropagation()
      cycleTab(e.shiftKey ? -1 : 1)
    } else if (isCtrl(e) && e.shiftKey && key === 'b') {
      e.preventDefault()
      e.stopPropagation()
      toggleBroadcast()
    } else if (isCtrl(e) && e.shiftKey && key === 'w') {
      e.preventDefault()
      e.stopPropagation()
      const id = get(activeConnectionId)
      if (id) void disconnectConnection(id)
    } else if (isCtrl(e) && key === 'w' && !shellOwnsLetter) {
      e.preventDefault()
      e.stopPropagation()
      const id = get(activeTabId)
      if (id) void closeTab(id)
    } else if (isCtrl(e) && key === 't' && !shellOwnsLetter) {
      e.preventDefault()
      e.stopPropagation()
      openNewTab()
    } else if (isCtrl(e) && e.shiftKey && key === 'p') {
      e.preventDefault()
      e.stopPropagation()
      commandPaletteOpen.set(true)
    } else if (isCtrl(e) && key === ',') {
      e.preventDefault()
      e.stopPropagation()
      settingsOpen.set(true)
    }
  }
</script>

<svelte:window on:keydown|capture={onGlobalKeydown} />

{#if currentToolWindowKind}
  <ToolWindow />
{:else}
  <div class="shell">
    <Toolbar />
    <div class="main">
      {#if $sidebarVisible}
        <Sidebar bind:this={sidebar} />
      {/if}
      <div class="center">
        <TabBar />
        <TerminalArea />
      </div>
      {#if $inspectorVisible}
        <Inspector />
      {/if}
    </div>
    <StatusBar />
  </div>

  <PasswordPromptDialog />
  <PassphrasePromptDialog />
  <HostKeyPromptDialog />
  <KeyboardInteractivePromptDialog />
  <WorkspaceDialog />
  <ConfirmDialog />
  <CommandPalette />
  <NotificationBanner />
{/if}

<style>
  .shell {
    display: flex;
    flex-direction: column;
    height: 100%;
    width: 100%;
  }

  .main {
    flex: 1;
    display: flex;
    min-height: 0;
  }

  .center {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
  }
</style>
