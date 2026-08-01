<script lang="ts">
  import { onMount } from 'svelte'

  import HostEditDialog from './dialogs/HostEditDialog.svelte'
  import FolderEditDialog from './dialogs/FolderEditDialog.svelte'
  import SettingsDialog from './dialogs/SettingsDialog.svelte'
  import AboutDialog from './dialogs/AboutDialog.svelte'
  import SnippetsDialog from './dialogs/SnippetsDialog.svelte'
  import CredentialsDialog from './dialogs/CredentialsDialog.svelte'
  import TransferDialog from './dialogs/TransferDialog.svelte'
  import ForwardingDialog from './dialogs/ForwardingDialog.svelte'
  import NotificationBanner from './NotificationBanner.svelte'

  import { backend, closeCurrentWindow, on } from '../services/backend'
  import type { StatusEvent } from '../types/connection'
  import { locale, type Locale } from '../services/i18n'
  import { loadHosts } from '../stores/hosts'
  import { initCredentialEvents, loadCredentials } from '../stores/credentials'
  import { initTransferEvents, transferDialogHostId } from '../stores/transfer'
  import { forwardingDialogHostId, initForwardingEvents } from '../stores/forwarding'
  import {
    aboutOpen,
    applyTheme,
    credentialsOpen,
    folderDialog,
    hostDialog,
    normalizeTerminalFontSize,
    settingsOpen,
    snippetsOpen,
    terminalFontSize,
    theme,
    type ThemePreference
  } from '../stores/ui'
  import {
    currentToolWindowKind as kind,
    toolWindowEntityId as entityId,
    toolWindowParentId as parentId
  } from '../services/windowing'

  let ready = false
  let closing = false
  let forwardingConnectionId: string | null = null

  onMount(() => {
    const offConnectionStatus =
      kind === 'forwarding' && entityId != null
        ? on<StatusEvent>('connection:status', (event) => {
            if (event.hostId === entityId) void refreshForwardingConnection()
          })
        : () => {}
    // The host editor's credential picker must reflect edits made in a
    // separately-opened Credentials window.
    const offCredentials = kind === 'host' || kind === 'credentials' ? initCredentialEvents() : () => {}
    void initialise()
    return () => {
      offConnectionStatus()
      offCredentials()
    }
  })

  async function refreshForwardingConnection(): Promise<void> {
    if (entityId == null) {
      forwardingConnectionId = null
      return
    }
    forwardingConnectionId = (await backend.connectedConnectionId(entityId)) || null
  }

  async function initialise(): Promise<void> {
    await loadWindowSettings()
    if (kind === 'host' || kind === 'folder' || kind === 'transfer') {
      await loadHosts()
    }
    if (kind === 'host' || kind === 'credentials') {
      await loadCredentials()
    }
    if (kind === 'transfer') initTransferEvents()
    if (kind === 'forwarding') {
      initForwardingEvents()
      await refreshForwardingConnection()
    }

    switch (kind) {
      case 'host':
        hostDialog.set({ open: true, editingId: entityId })
        break
      case 'folder':
        folderDialog.set({ open: true, editingId: entityId, parentId })
        break
      case 'settings':
        settingsOpen.set(true)
        break
      case 'about':
        aboutOpen.set(true)
        break
      case 'snippets':
        snippetsOpen.set(true)
        break
      case 'credentials':
        credentialsOpen.set(true)
        break
      case 'transfer':
        transferDialogHostId.set(entityId)
        break
      case 'forwarding':
        forwardingDialogHostId.set(entityId)
        break
    }
    ready = true
  }

  async function loadWindowSettings(): Promise<void> {
    const [themeSetting, localeSetting, terminalSizeSetting] = await Promise.all([
      backend.getSetting('theme'),
      backend.getSetting('locale'),
      backend.getSetting('terminalFontSize')
    ])
    if (themeSetting.exists) {
      const preference = themeSetting.value as ThemePreference
      theme.set(preference)
      applyTheme(preference)
    }
    if (localeSetting.exists) locale.set(localeSetting.value as Locale)
    if (terminalSizeSetting.exists) {
      terminalFontSize.set(normalizeTerminalFontSize(Number(terminalSizeSetting.value)))
    }
  }

  $: isOpen =
    kind === 'host'
      ? $hostDialog.open
      : kind === 'folder'
        ? $folderDialog.open
        : kind === 'settings'
          ? $settingsOpen
          : kind === 'about'
            ? $aboutOpen
            : kind === 'snippets'
              ? $snippetsOpen
              : kind === 'credentials'
                ? $credentialsOpen
                : kind === 'transfer'
                  ? $transferDialogHostId != null
                  : kind === 'forwarding'
                    ? $forwardingDialogHostId != null
                    : false

  $: if (ready && !isOpen && !closing) {
    closing = true
    void closeCurrentWindow()
  }
</script>

<main class="tool-window">
  {#if kind === 'host'}
    <HostEditDialog />
  {:else if kind === 'folder'}
    <FolderEditDialog />
  {:else if kind === 'settings'}
    <SettingsDialog />
  {:else if kind === 'about'}
    <AboutDialog />
  {:else if kind === 'snippets'}
    <SnippetsDialog hostId={entityId} />
  {:else if kind === 'credentials'}
    <CredentialsDialog />
  {:else if kind === 'transfer'}
    <TransferDialog />
  {:else if kind === 'forwarding'}
    <ForwardingDialog connectionId={forwardingConnectionId} />
  {/if}
</main>

<NotificationBanner />

<style>
  .tool-window {
    width: 100%;
    height: 100%;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
    background: var(--window-bg);
  }
</style>
