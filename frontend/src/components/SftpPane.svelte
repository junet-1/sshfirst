<script lang="ts">
  import Icon from './Icon.svelte'
  import ReconnectOverlay from './ReconnectOverlay.svelte'
  import { backend } from '../services/backend'
  import { t } from '../services/i18n'
  import { connections, tabs } from '../stores/connections'
  import { hosts } from '../stores/hosts'
  import { notify } from '../stores/notifications'
  import { reconnectStates } from '../stores/reconnect'
  import type { SFTPEntry } from '../types/connection'

  export let tabId: string
  export let active = false

  let currentPath = ''
  let pathInput = ''
  let entries: SFTPEntry[] = []
  let selectedPath: string | null = null
  let history: string[] = []
  let loading = false
  let transferring = false
  let error = ''
  let loadGeneration = 0
  let loadedStateKey = ''

  $: tab = $tabs[tabId]
  $: connection = tab ? $connections[tab.connectionId] : null
  $: host = connection ? $hosts.find((item) => item.id === connection?.hostId) : null
  $: startPath = host?.remotePath || '.'
  $: selected = selectedPath ? entries.find((entry) => entry.path === selectedPath) ?? null : null
  $: reconnectState = connection ? $reconnectStates[connection.connectionId] : null
  $: stateKey = connection?.status === 'connected' ? `${connection.connectionId}:${connection.connectedAt ?? 0}` : ''
  $: if (active && stateKey && stateKey !== loadedStateKey) {
    loadedStateKey = stateKey
    void loadDirectory(currentPath || startPath, false)
  }

  async function loadDirectory(path: string, remember = true): Promise<void> {
    if (!connection || connection.status !== 'connected') return
    const generation = ++loadGeneration
    loading = true
    error = ''
    try {
      const result = await backend.listSFTP(tabId, path || '.')
      if (generation !== loadGeneration) return
      if (remember && currentPath && result.path !== currentPath) history = [...history, currentPath]
      currentPath = result.path
      pathInput = result.path
      entries = result.entries ?? []
      selectedPath = null
    } catch (caught) {
      if (generation !== loadGeneration) return
      error = caught instanceof Error ? caught.message : String(caught)
    } finally {
      if (generation === loadGeneration) loading = false
    }
  }

  function parentPath(path: string): string {
    if (!path || path === '/') return '/'
    const trimmed = path.replace(/\/+$/, '')
    const slash = trimmed.lastIndexOf('/')
    return slash <= 0 ? '/' : trimmed.slice(0, slash)
  }

  function goBack(): void {
    const previous = history[history.length - 1]
    if (!previous) return
    history = history.slice(0, -1)
    void loadDirectory(previous, false)
  }

  function activateEntry(entry: SFTPEntry): void {
    if (entry.isDir) void loadDirectory(entry.path)
  }

  async function upload(): Promise<void> {
    if (!currentPath || transferring) return
    const localPath = await backend.pickFile()
    if (!localPath) return
    transferring = true
    try {
      await backend.uploadSFTP(tabId, localPath, currentPath)
      await loadDirectory(currentPath, false)
      notify('info', $t('sftp.uploadDone'))
    } catch (caught) {
      notify('error', `${$t('sftp.uploadFailed')}: ${caught instanceof Error ? caught.message : String(caught)}`)
    } finally {
      transferring = false
    }
  }

  async function download(): Promise<void> {
    if (!selected || selected.isDir || transferring) return
    const localDir = await backend.pickDirectory()
    if (!localDir) return
    transferring = true
    try {
      const destination = await backend.downloadSFTP(tabId, selected.path, localDir)
      notify('info', $t('sftp.downloadDone', { path: destination }))
    } catch (caught) {
      notify('error', `${$t('sftp.downloadFailed')}: ${caught instanceof Error ? caught.message : String(caught)}`)
    } finally {
      transferring = false
    }
  }

  function formatSize(entry: SFTPEntry): string {
    if (entry.isDir) return '—'
    const units = ['B', 'KB', 'MB', 'GB', 'TB']
    let value = entry.size
    let unit = 0
    while (value >= 1024 && unit < units.length - 1) {
      value /= 1024
      unit += 1
    }
    return `${unit === 0 ? value : value.toFixed(value < 10 ? 1 : 0)} ${units[unit]}`
  }
</script>

<div
  class="sftp-pane"
  class:active
  aria-label={$t('sftp.browser')}
>
  <div class="browser-toolbar">
    <button title={$t('sftp.back')} disabled={!history.length || loading} on:click={goBack}><Icon name="back" /></button>
    <button title={$t('sftp.up')} disabled={!currentPath || currentPath === '/' || loading} on:click={() => loadDirectory(parentPath(currentPath))}><Icon name="up" /></button>
    <button title={$t('sftp.refresh')} disabled={loading} on:click={() => loadDirectory(currentPath || startPath, false)}><Icon name="refresh" /></button>
    <form class="path-form" on:submit|preventDefault={() => loadDirectory(pathInput)}>
      <input type="text" class="path" aria-label={$t('sftp.path')} bind:value={pathInput} spellcheck="false" />
    </form>
    <button class="action" disabled={transferring || loading || connection?.status !== 'connected'} on:click={upload}>
      <Icon name="upload" /> {$t('sftp.upload')}
    </button>
    <button class="action" disabled={!selected || selected.isDir || transferring || loading} on:click={download}>
      <Icon name="download" /> {$t('sftp.download')}
    </button>
  </div>

  <div class="file-view">
    <div class="header-row" role="row">
      <span>{$t('sftp.name')}</span>
      <span>{$t('sftp.size')}</span>
      <span>{$t('sftp.modified')}</span>
      <span>{$t('sftp.permissions')}</span>
    </div>
    {#if error}
      <div class="state error-state">
        <Icon name="warning" size={24} />
        <span>{error}</span>
        <button on:click={() => loadDirectory(currentPath || startPath, false)}>{$t('sftp.retry')}</button>
      </div>
    {:else if loading && entries.length === 0}
      <div class="state"><span class="spinner" /> {$t('sftp.loading')}</div>
    {:else if entries.length === 0}
      <div class="state">{$t('sftp.empty')}</div>
    {:else}
      <div class="rows" role="grid" aria-label={$t('sftp.directoryContents')}>
        {#each entries as entry (entry.path)}
          <!-- svelte-ignore a11y-click-events-have-key-events -->
          <div
            class="file-row"
            class:selected={selectedPath === entry.path}
            role="row"
            tabindex="0"
            aria-selected={selectedPath === entry.path}
            on:click={() => (selectedPath = entry.path)}
            on:dblclick={() => activateEntry(entry)}
            on:keydown={(event) => {
              if (event.key === 'Enter') activateEntry(entry)
              if (event.key === 'Backspace') {
                event.preventDefault()
                goBack()
              }
            }}
          >
            <span class="name-cell" role="gridcell"><Icon name={entry.isDir ? 'folder' : 'file'} /><span>{entry.name}</span></span>
            <span class="numeric" role="gridcell">{formatSize(entry)}</span>
            <span role="gridcell">{new Date(entry.modifiedAt).toLocaleString()}</span>
            <span class="mono" role="gridcell">{entry.mode}</span>
          </div>
        {/each}
      </div>
    {/if}
    {#if loading && entries.length > 0}<span class="corner-spinner spinner" aria-label={$t('sftp.loading')} />{/if}
  </div>

  <div class="browser-status">
    <span>{entries.length} {$t(entries.length === 1 ? 'sftp.item' : 'sftp.items')}</span>
    {#if selected}<span>{selected.name}</span>{/if}
    {#if transferring}<span class="transfer-state"><span class="spinner" /> {$t('sftp.transferring')}</span>{/if}
  </div>
  {#if reconnectState}<ReconnectOverlay state={reconnectState} />{/if}
</div>

<style>
  .sftp-pane { position: absolute; inset: 0; display: none; flex-direction: column; min-width: 0; background: var(--view-bg); color: var(--text-color); font-size: 12px; outline: none; }
  .sftp-pane.active { display: flex; }
  .browser-toolbar { display: flex; align-items: center; gap: 3px; min-height: 34px; padding: 4px 6px; background: var(--header-bg); border-bottom: 1px solid var(--border-color); }
  .browser-toolbar button { display: inline-flex; align-items: center; justify-content: center; gap: 5px; min-height: 24px; padding: 2px 7px; }
  .path-form { flex: 1; min-width: 80px; margin: 0 4px; }
  .path {
    width: 100%;
    height: 24px;
    padding: 3px 7px;
    appearance: none;
    background: var(--view-bg);
    color: var(--text-color);
    caret-color: var(--text-color);
    border: 1px solid var(--border-color);
    border-radius: 3px;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11.5px;
  }
  .path:focus { border-color: var(--focus-ring); }
  .file-view { position: relative; flex: 1; min-height: 0; overflow: auto; }
  .header-row, .file-row { display: grid; grid-template-columns: minmax(180px, 1fr) 85px 155px 100px; align-items: center; min-width: 620px; }
  .header-row { position: sticky; top: 0; z-index: 1; height: 25px; background: var(--view-bg-alt); border-bottom: 1px solid var(--border-color); color: var(--text-color-secondary); font-size: 11px; font-weight: 600; }
  .header-row span, .file-row > span { min-width: 0; padding: 0 8px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; border-right: 1px solid var(--separator-color); }
  .file-row { height: 25px; border-bottom: 1px solid color-mix(in srgb, var(--separator-color) 55%, transparent); }
  .file-row:hover { background: var(--hover-bg); }
  .file-row.selected { background: var(--highlight-bg); color: var(--highlight-text); }
  .name-cell { display: flex; align-items: center; gap: 7px; }
  .name-cell :global(.icon) { color: var(--accent-color); }
  .numeric { text-align: right; font-variant-numeric: tabular-nums; }
  .mono { font-family: 'JetBrains Mono', ui-monospace, monospace; font-size: 11px; }
  .state { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 9px; min-height: 180px; color: var(--text-color-secondary); }
  .error-state { color: var(--error-color); }
  .corner-spinner { position: absolute; top: 34px; right: 12px; }
  .spinner { display: inline-block; width: 13px; height: 13px; border: 2px solid var(--border-color); border-top-color: var(--accent-color); border-radius: 50%; animation: spin .7s linear infinite; }
  .browser-status { display: flex; align-items: center; gap: 14px; min-height: 23px; padding: 0 8px; background: var(--header-bg); border-top: 1px solid var(--border-color); color: var(--text-color-secondary); font-size: 11px; }
  .browser-status span:nth-child(2) { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .transfer-state { margin-left: auto; display: inline-flex; align-items: center; gap: 5px; }
  .transfer-state .spinner { width: 10px; height: 10px; border-width: 1.5px; }
  @keyframes spin { to { transform: rotate(360deg); } }
  @media (max-width: 700px) { .action { font-size: 0; gap: 0 !important; } }
</style>
