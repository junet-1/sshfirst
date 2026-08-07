<script lang="ts">
  import { get } from 'svelte/store'
  import Icon from './Icon.svelte'
  import { t } from '../services/i18n'
  import { activeConnectionId, connections, disconnectConnection, openNewTab, tabs } from '../stores/connections'
  import { canSplit, closeSplit, isSplit, splitFocused, tileAllTerminals } from '../stores/layout'
  import { reconnectConnection } from '../stores/reconnect'
  import { broadcastActive, setBroadcast } from '../stores/broadcast'
  import { hostDialog, inspectorVisible, selectedHostId, sidebarVisible } from '../stores/ui'
  import { hosts, importSSHConfigWithFeedback } from '../stores/hosts'
  import { forwardingDialogHostId } from '../stores/forwarding'
  import { activeWorkspaceName, workspaceDialogOpen } from '../stores/workspaces'

  $: activeConnection = $activeConnectionId ? $connections[$activeConnectionId] : null
  $: activeHost = activeConnection ? $hosts.find((h) => h.id === activeConnection?.hostId) ?? null : null
  // Port forwarding targets the active connection's host, else the selected one
  // in the sidebar — but only for SSH hosts (SFTP has no forwards).
  $: selectedHost = $selectedHostId != null ? $hosts.find((h) => h.id === $selectedHostId) ?? null : null
  $: forwardHost =
    activeHost?.protocol === 'ssh' ? activeHost : selectedHost?.protocol === 'ssh' ? selectedHost : null

  function onForwarding(): void {
    if (forwardHost) forwardingDialogHostId.set(forwardHost.id)
  }
  $: terminalTabIds = Object.values($tabs).filter((tab) => tab.kind === 'terminal').map((tab) => tab.tabId)
  $: tabCount = terminalTabIds.length

  function onTileToggle(): void {
    if ($isSplit) closeSplit()
    else tileAllTerminals()
  }

  function toggleBroadcast(): void {
    setBroadcast(!get(broadcastActive), Object.values(get(tabs)).filter((tab) => tab.kind === 'terminal').map((tab) => tab.tabId))
  }

  function onNewHost(): void {
    hostDialog.set({ open: true, editingId: null })
  }

  async function onImport(): Promise<void> {
    await importSSHConfigWithFeedback()
  }

  async function onDisconnect(): Promise<void> {
    if ($activeConnectionId) await disconnectConnection($activeConnectionId)
  }

  async function onReconnect(): Promise<void> {
    if ($activeConnectionId) await reconnectConnection($activeConnectionId)
  }

  async function onNewTab(): Promise<void> {
    openNewTab()
  }

  async function onCopyConnectionCommand(): Promise<void> {
    if (activeHost) {
      const command = activeHost.protocol === 'sftp' ? 'sftp' : 'ssh'
      const portFlag = activeHost.protocol === 'sftp' ? '-P' : '-p'
      const port = activeHost.port !== 22 ? ` ${portFlag} ${activeHost.port}` : ''
      const userPart = activeHost.user ? `${activeHost.user}@` : ''
      await navigator.clipboard.writeText(`${command}${port} ${userPart}${activeHost.hostname}`)
    } else if (activeConnection?.quickTarget) {
      await navigator.clipboard.writeText(
        activeConnection.quickTarget.startsWith('ssh ') ? activeConnection.quickTarget : `ssh ${activeConnection.quickTarget}`
      )
    }
  }
</script>

<div class="toolbar" role="toolbar" aria-label="Main toolbar">
  <button class="tool" title={$t('sidebar.noHosts.addHost')} on:click={onNewHost}>
    <Icon name="plus" />
  </button>
  <button class="tool" title={$t('menu.file.importConfig')} on:click={onImport}>
    <Icon name="refresh" />
  </button>

  <div class="sep" />

  <button class="tool" title={$t('menu.session.reconnect')} disabled={!activeConnection} on:click={onReconnect}>
    <Icon name="link" />
  </button>
  <button class="tool" title={$t('menu.session.disconnect')} disabled={!activeConnection} on:click={onDisconnect}>
    <Icon name="unlink" />
  </button>
  <button class="tool" title={$t('menu.session.newTab')} on:click={onNewTab}>
    <Icon name="terminal" />
  </button>
  <button class="tool" title={$t('menu.session.copySSHCommand')} disabled={!activeHost && !activeConnection?.quickTarget} on:click={onCopyConnectionCommand}>
    <Icon name="copy" />
  </button>
  <button class="tool" title={$t('forwarding.title')} disabled={!forwardHost} on:click={onForwarding}>
    <Icon name="globe" />
  </button>

  <div class="sep" />

  <button class="tool" title={$t('split.right')} disabled={!$canSplit} on:click={() => splitFocused('row')}>
    <Icon name="split-h" />
  </button>
  <button class="tool" title={$t('split.down')} disabled={!$canSplit} on:click={() => splitFocused('column')}>
    <Icon name="split-v" />
  </button>
  <button
    class="tool"
    class:active={$isSplit}
    title={$isSplit ? $t('split.untile') : $t('split.tileAll')}
    disabled={!$isSplit && tabCount < 2}
    on:click={onTileToggle}
  >
    <Icon name="grid" />
  </button>

  <div class="sep" />

  <button
    class="tool"
    class:active={$activeWorkspaceName !== null}
    title={$activeWorkspaceName ? $t('workspaces.activeTitle', { name: $activeWorkspaceName }) : $t('workspaces.title')}
    on:click={() => workspaceDialogOpen.set(true)}
  >
    <Icon name="layers" />
  </button>

  <div class="sep" />

  <button
    class="tool"
    class:active={$broadcastActive}
    title={$t('menu.session.broadcast')}
    disabled={activeConnection?.protocol === 'sftp' || (tabCount < 2 && !$broadcastActive)}
    on:click={toggleBroadcast}
  >
    <Icon name="broadcast" />
  </button>

  <div class="spacer" />

  <button
    class="tool"
    class:active={$sidebarVisible}
    title={$t('menu.view.toggleSidebar')}
    on:click={() => sidebarVisible.update((v) => !v)}
  >
    <Icon name="sidebar" />
  </button>
  <button
    class="tool"
    class:active={$inspectorVisible}
    title={$t('menu.view.toggleInspector')}
    on:click={() => inspectorVisible.update((v) => !v)}
  >
    <Icon name="inspector" />
  </button>
</div>

<style>
  .toolbar {
    position: relative;
    z-index: 1;
    display: flex;
    align-items: center;
    gap: 2px;
    height: 32px;
    padding: 0 6px;
    background: var(--header-bg);
    border-bottom: 1px solid var(--border-color);
    flex-shrink: 0;
  }

  .tool {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 24px;
    padding: 0;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 3px;
    color: var(--text-color);
  }

  .tool:hover:not(:disabled) {
    background: var(--hover-bg);
    border-color: var(--border-color);
  }

  .tool.active {
    background: var(--active-bg);
    border-color: var(--accent-color);
    color: var(--accent-color);
  }

  .tool:disabled {
    color: var(--disabled-text-color);
  }

  .sep {
    width: 1px;
    align-self: stretch;
    margin: 4px 4px;
    background: var(--separator-color);
  }

  .spacer {
    flex: 1;
  }
</style>
