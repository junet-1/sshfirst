<script lang="ts">
  import { onMount, tick } from 'svelte'
  import Icon from './Icon.svelte'
  import ContextMenu from './ContextMenu.svelte'
  import type { ContextMenuItem } from '../types/contextMenu'
  import HostListItem from './HostListItem.svelte'
  import { t } from '../services/i18n'
  import {
    allTags,
    confirmAndDeleteHost,
    deleteFolder,
    favoriteHosts,
    filteredHosts,
    folders,
    hosts,
    duplicateHost,
    importSSHConfigWithFeedback,
    moveFolder,
    moveHostToFolder,
    recentHosts,
    searchQuery,
    setFavorite,
    updateHost
  } from '../stores/hosts'
  import { connectingHostIds, connections, openControlPanelTab, openHost } from '../stores/connections'
  import { notify } from '../stores/notifications'
  import { backend } from '../services/backend'
  import { confirmDialog } from '../stores/confirm'
  import { folderDialog, hostDialog, selectedHostId, showRecent } from '../stores/ui'
  import { transferDialogHostId } from '../stores/transfer'
  import { forwardingDialogHostId } from '../stores/forwarding'
  import { hostToInput, type Folder, type Host } from '../types/host'

  const HOST_DND = 'application/x-ssh-first-host'
  const FOLDER_DND = 'application/x-ssh-first-folder'
  const DEFAULT_SIDEBAR_WIDTH = 260
  const MIN_SIDEBAR_WIDTH = 190
  const MAX_SIDEBAR_WIDTH = 480
  type HostConnectionStatus = 'offline' | 'connecting' | 'online'

  let sectionOpen: Record<string, boolean> = { favorites: true, recent: true, folders: true, tags: false }
  let renamingHostId: number | null = null
  let contextMenu: { x: number; y: number; hostId: number } | null = null
  let activeTagFilter: string | null = null
  let dragOverFolderId: number | 'root' | null = null
  let selectedRow: string | null = null // "<section>:<hostId>" of the exact clicked row
  let collapsedFolders = new Set<number>()
  let folderContextMenu: { x: number; y: number; folderId: number } | null = null
  let sidebarWidth = DEFAULT_SIDEBAR_WIDTH
  let resizing = false
  let resizeStartX = 0
  let resizeStartWidth = DEFAULT_SIDEBAR_WIDTH

  onMount(() => {
    void backend.getSetting('sidebarWidth').then((setting) => {
      if (!setting.exists) return
      const savedWidth = Number(setting.value)
      if (Number.isFinite(savedWidth)) sidebarWidth = clampSidebarWidth(savedWidth)
    })
  })

  function clampSidebarWidth(width: number): number {
    return Math.min(MAX_SIDEBAR_WIDTH, Math.max(MIN_SIDEBAR_WIDTH, Math.round(width)))
  }

  function saveSidebarWidth(): void {
    void backend.setSetting('sidebarWidth', String(sidebarWidth))
  }

  function startSidebarResize(e: PointerEvent): void {
    e.preventDefault()
    resizing = true
    resizeStartX = e.clientX
    resizeStartWidth = sidebarWidth
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  }

  function resizeSidebar(e: PointerEvent): void {
    if (!resizing) return
    sidebarWidth = clampSidebarWidth(resizeStartWidth + e.clientX - resizeStartX)
  }

  function finishSidebarResize(e: PointerEvent): void {
    if (!resizing) return
    resizing = false
    const handle = e.currentTarget as HTMLElement
    if (handle.hasPointerCapture(e.pointerId)) handle.releasePointerCapture(e.pointerId)
    saveSidebarWidth()
  }

  function resizeSidebarWithKeyboard(e: KeyboardEvent): void {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
    e.preventDefault()
    sidebarWidth = clampSidebarWidth(sidebarWidth + (e.key === 'ArrowLeft' ? -10 : 10))
    saveSidebarWidth()
  }

  function resetSidebarWidth(): void {
    sidebarWidth = DEFAULT_SIDEBAR_WIDTH
    saveSidebarWidth()
  }

  function toggleFolder(id: number): void {
    const next = new Set(collapsedFolders)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    collapsedFolders = next
  }

  // Flatten the folder tree (respecting parentId) into an ordered list with a
  // depth for indentation. Descendants of a collapsed folder are omitted.
  $: folderTree = ((): { folder: Folder; depth: number }[] => {
    const byParent = new Map<number | null, Folder[]>()
    for (const f of $folders) {
      const p = f.parentId ?? null
      const arr = byParent.get(p) ?? []
      arr.push(f)
      byParent.set(p, arr)
    }
    for (const arr of byParent.values()) arr.sort((a, b) => a.name.localeCompare(b.name))
    const out: { folder: Folder; depth: number }[] = []
    const walk = (parent: number | null, depth: number): void => {
      for (const f of byParent.get(parent) ?? []) {
        out.push({ folder: f, depth })
        if (!collapsedFolders.has(f.id)) walk(f.id, depth + 1)
      }
    }
    walk(null, 0)
    return out
  })()

  // A connection record can remain in the store after a connection error, so
  // its mere presence does not mean that the host is online. Build the sidebar
  // state from the actual lifecycle status instead. A connected session wins
  // if a host ever has more than one connection record.
  $: hostConnectionStatuses = (() => {
    const statuses = new Map<number, HostConnectionStatus>()

    for (const hostId of $connectingHostIds) statuses.set(hostId, 'connecting')
    for (const connection of Object.values($connections)) {
      if (connection.status === 'connected') {
        statuses.set(connection.hostId, 'online')
      } else if (connection.status === 'connecting' && statuses.get(connection.hostId) !== 'online') {
        statuses.set(connection.hostId, 'connecting')
      }
    }

    return statuses
  })()

  $: tagFiltered = activeTagFilter
    ? $filteredHosts.filter((h) => h.tags.includes(activeTagFilter as string))
    : $filteredHosts

  $: unfoldered = tagFiltered.filter((h) => h.folderId == null)
  $: byFolder = (folderId: number) => tagFiltered.filter((h) => h.folderId === folderId)

  function toggle(section: string): void {
    sectionOpen = { ...sectionOpen, [section]: !sectionOpen[section] }
  }

  function clearSelection(): void {
    selectedHostId.set(null)
    selectedRow = null
  }

  function onWindowPointerDown(e: PointerEvent): void {
    const target = e.target
    if (target instanceof Element && target.closest('[data-sidebar-host-row]')) return
    clearSelection()
  }

  async function onConnect(host: Host): Promise<void> {
    // Connecting again to an already-connected host opens another terminal on
    // the same connection rather than just refocusing it. A web host opens (or
    // refocuses) its control-panel browser tab instead of an SSH transport.
    openHost(host, true)
    clearSelection()
  }

  // A host can appear in several sections at once (Recent, Favorites and its
  // tree location). selectedHostId drives host-level actions (inspector, keyboard,
  // delete); selectedRow tracks the exact row clicked so only that one
  // highlights instead of every copy of the same host.
  function onSelect(host: Host, section: string): void {
    selectedHostId.set(host.id)
    selectedRow = `${section}:${host.id}`
  }

  function openContextMenu(host: Host, section: string, e: MouseEvent): void {
    onSelect(host, section)
    contextMenu = { x: e.clientX, y: e.clientY, hostId: host.id }
  }

  function contextItemsFor(host: Host): ContextMenuItem[] {
    return [
      { id: 'connect', label: $t('sidebar.context.connect') },
      { id: 'edit', label: $t('sidebar.context.edit') },
      { id: 'duplicate', label: $t('sidebar.context.duplicate') },
      {
        id: 'favorite',
        label: host.favorite ? $t('sidebar.context.unfavorite') : $t('sidebar.context.favorite')
      },
      ...(host.protocol !== 'web'
        ? [
            { id: 'copy', label: $t('sidebar.context.copySSHCommand') },
            { id: 'copyUser', label: $t('sidebar.context.copyUsername') },
            { id: 'copyPassword', label: $t('sidebar.context.copyPassword') }
          ]
        : []),
      ...(host.protocol !== 'web' && host.controlPanelUrl
        ? [{ id: 'panel', label: $t('sidebar.context.controlPanel'), separatorBefore: true }]
        : []),
      ...(host.protocol === 'ssh'
        ? [
            { id: 'forward', label: $t('sidebar.context.forward'), separatorBefore: true },
            { id: 'transfer', label: $t('sidebar.context.transfer') }
          ]
        : []),
      { id: 'delete', label: $t('sidebar.context.delete'), destructive: true, separatorBefore: true }
    ]
  }

  async function onContextSelect(host: Host, action: string): Promise<void> {
    switch (action) {
      case 'connect':
        await onConnect(host)
        break
      case 'edit':
        hostDialog.set({ open: true, editingId: host.id })
        break
      case 'duplicate':
        await duplicateAndRename(host)
        break
      case 'favorite':
        await setFavorite(host.id, !host.favorite)
        break
      case 'copy': {
        const command = host.protocol === 'sftp' ? 'sftp' : 'ssh'
        const portFlag = host.protocol === 'sftp' ? '-P' : '-p'
        const port = host.port !== 22 ? ` ${portFlag} ${host.port}` : ''
        const userPart = host.user ? `${host.user}@` : ''
        await navigator.clipboard.writeText(`${command}${port} ${userPart}${host.hostname}`)
        break
      }
      case 'copyUser': {
        const user = await backend.hostUsername(host.id)
        if (user) {
          await navigator.clipboard.writeText(user)
          notify('info', $t('sidebar.copied.username'))
        } else {
          notify('info', $t('sidebar.copied.noUsername'))
        }
        break
      }
      case 'copyPassword': {
        const password = await backend.hostPassword(host.id)
        if (password) {
          await navigator.clipboard.writeText(password)
          notify('info', $t('sidebar.copied.password'))
        } else {
          notify('info', $t('sidebar.copied.noPassword'))
        }
        break
      }
      case 'panel':
        openControlPanelTab(host.label, host.controlPanelUrl)
        break
      case 'forward':
        forwardingDialogHostId.set(host.id)
        break
      case 'transfer':
        transferDialogHostId.set(host.id)
        break
      case 'delete':
        await confirmAndDeleteHost(host)
        break
    }
  }

  function startRename(host: Host): void {
    renamingHostId = host.id
  }

  // Duplicate a host's settings, then drop the new "<name> (2)" row straight into
  // inline rename so the user can name it before editing its credentials.
  async function duplicateAndRename(host: Host): Promise<void> {
    try {
      const copy = await duplicateHost(host.id)
      if (!copy) return
      selectedHostId.set(copy.id)
      // Clear any active search so the freshly-created row is actually rendered
      // (and thus focusable) rather than filtered out under the old query.
      searchQuery.set('')
      await tick()
      renamingHostId = copy.id
    } catch (e) {
      notify('error', e instanceof Error ? e.message : String(e))
    }
  }

  async function commitRename(host: Host, newLabel: string): Promise<void> {
    renamingHostId = null
    const input = hostToInput(host)
    input.label = newLabel
    await updateHost(host.id, input)
  }

  // effectAllowed/dropEffect are pinned to a single 'move' so the drag resolves
  // to exactly one GDK action. Without this, WebKitGTK forwards a combined
  // COPY|MOVE action to gdk_drop_finish(), which asserts gdk_drag_action_is_unique
  // and corrupts GTK's drop state → SIGABRT after a couple of drops.
  function onDragStart(host: Host, e: DragEvent): void {
    if (!e.dataTransfer) return
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData(HOST_DND, String(host.id))
  }

  function onFolderDragStart(folder: Folder, e: DragEvent): void {
    if (!e.dataTransfer) return
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData(FOLDER_DND, String(folder.id))
  }

  function onDragOver(target: number | 'root', e: DragEvent): void {
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
    dragOverFolderId = target
  }

  // Folder rows accept dropped hosts/folders. The section header and loose
  // root-host area use target "root" to remove folder assignment / nesting.
  async function onDrop(target: number | 'root', e: DragEvent): Promise<void> {
    dragOverFolderId = null
    const hostIdStr = e.dataTransfer?.getData(HOST_DND)
    if (hostIdStr) {
      await moveHostToFolder(Number(hostIdStr), target === 'root' ? null : target)
      return
    }
    const folderIdStr = e.dataTransfer?.getData(FOLDER_DND)
    if (folderIdStr) {
      const id = Number(folderIdStr)
      if (target !== 'root' && target === id) return // no-op onto itself
      try {
        await moveFolder(id, target === 'root' ? null : target)
      } catch (err) {
        notify('error', err instanceof Error ? err.message : String(err))
      }
    }
  }

  function openFolderContextMenu(folder: Folder, e: MouseEvent): void {
    folderContextMenu = { x: e.clientX, y: e.clientY, folderId: folder.id }
  }

  function folderContextItems(): ContextMenuItem[] {
    return [
      { id: 'edit', label: $t('sidebar.context.editFolder') },
      { id: 'delete', label: $t('sidebar.context.deleteFolder'), destructive: true, separatorBefore: true }
    ]
  }

  async function onFolderContextSelect(folder: Folder, action: string): Promise<void> {
    if (action === 'edit') {
      folderDialog.set({ open: true, editingId: folder.id, parentId: folder.parentId ?? null })
    } else if (action === 'delete') {
      const ok = await confirmDialog(
        $t('sidebar.deleteFolder.title'),
        $t('sidebar.deleteFolder.body', { name: folder.name }),
        $t('confirmDelete.confirm'),
        $t('confirmDelete.cancel')
      )
      if (ok) {
        try {
          await deleteFolder(folder.id)
        } catch (e) {
          notify('error', e instanceof Error ? e.message : String(e))
        }
      }
    }
  }

  async function onImport(): Promise<void> {
    await importSSHConfigWithFeedback()
  }

  function onAddFolder(): void {
    sectionOpen = { ...sectionOpen, folders: true }
    folderDialog.set({ open: true, editingId: null, parentId: null })
  }

  export function focusSearch(): void {
    document.getElementById('host-search')?.focus()
  }

  function onTreeKeydown(e: KeyboardEvent): void {
    const id = $selectedHostId
    if (id == null) return
    const host = $hosts.find((h) => h.id === id)
    if (!host) return

    if (e.key === 'Enter') {
      e.preventDefault()
      void onConnect(host)
    } else if (e.key === 'F2') {
      e.preventDefault()
      startRename(host)
    } else if (e.key === 'Delete') {
      e.preventDefault()
      void confirmAndDeleteHost(host)
    }
  }

  export function renameSelected(): void {
    const id = $selectedHostId
    const host = id != null ? $hosts.find((h) => h.id === id) : null
    if (host) startRename(host)
  }
</script>

<svelte:window on:pointerdown={onWindowPointerDown} />

<div class="sidebar" class:resizing style="width: {sidebarWidth}px">
  <div class="search">
    <Icon name="search" />
    <input
      id="host-search"
      type="search"
      placeholder={$t('sidebar.search.placeholder')}
      bind:value={$searchQuery}
    />
  </div>

  {#if $hosts.length === 0 && $folders.length === 0}
    <div class="empty-state">
      <p class="empty-title">{$t('sidebar.noHosts.title')}</p>
      <p class="empty-body">{$t('sidebar.noHosts.body')}</p>
      <button class="primary" on:click={() => hostDialog.set({ open: true, editingId: null })}>
        {$t('sidebar.noHosts.addHost')}
      </button>
      <button on:click={onImport}>{$t('sidebar.noHosts.import')}</button>
      <button on:click={onAddFolder}>{$t('sidebar.noHosts.addFolder')}</button>
    </div>
  {:else}
    <div class="tree" role="listbox" tabindex="-1" aria-label="Hosts" on:keydown={onTreeKeydown}>
      {#if $favoriteHosts.length}
        <button class="section-header" on:click={() => toggle('favorites')}>
          <Icon name={sectionOpen.favorites ? 'chevron-down' : 'chevron-right'} size={11} />
          <span>{$t('sidebar.favorites')}</span>
        </button>
        {#if sectionOpen.favorites}
          {#each $favoriteHosts as host (host.id)}
            <HostListItem
              {host}
              selected={selectedRow === `favorites:${host.id}`}
              status={hostConnectionStatuses.get(host.id) ?? 'offline'}
              editing={renamingHostId === host.id}
              on:select={() => onSelect(host, 'favorites')}
              on:connect={() => onConnect(host)}
              on:contextmenu={(e) => openContextMenu(host, 'favorites', e.detail)}
              on:favorite={() => setFavorite(host.id, !host.favorite)}
              on:renameCommit={(e) => commitRename(host, e.detail)}
              on:renameCancel={() => (renamingHostId = null)}
              on:dragStart={(e) => onDragStart(host, e.detail)}
            />
          {/each}
        {/if}
      {/if}

      {#if $showRecent && $recentHosts.length}
        <button class="section-header" on:click={() => toggle('recent')}>
          <Icon name={sectionOpen.recent ? 'chevron-down' : 'chevron-right'} size={11} />
          <span>{$t('sidebar.recent')}</span>
        </button>
        {#if sectionOpen.recent}
          {#each $recentHosts as host (host.id)}
            <HostListItem
              {host}
              selected={selectedRow === `recent:${host.id}`}
              status={hostConnectionStatuses.get(host.id) ?? 'offline'}
              editing={false}
              on:select={() => onSelect(host, 'recent')}
              on:connect={() => onConnect(host)}
              on:contextmenu={(e) => openContextMenu(host, 'recent', e.detail)}
              on:favorite={() => setFavorite(host.id, !host.favorite)}
              on:dragStart={(e) => onDragStart(host, e.detail)}
            />
          {/each}
        {/if}
      {/if}

      {#if $allTags.length}
        <button class="section-header" on:click={() => toggle('tags')}>
          <Icon name={sectionOpen.tags ? 'chevron-down' : 'chevron-right'} size={11} />
          <span>{$t('sidebar.tags')}</span>
        </button>
        {#if sectionOpen.tags}
          <div class="tag-chips">
            {#each $allTags as tag (tag)}
              <button
                class="tag-chip"
                class:active={activeTagFilter === tag}
                on:click={() => (activeTagFilter = activeTagFilter === tag ? null : tag)}
              >
                <Icon name="tag" size={10} />{tag}
              </button>
            {/each}
          </div>
        {/if}
      {/if}

      <div
        class="section-header-row"
        role="group"
        aria-label={$t('sidebar.folders')}
        class:drag-over={dragOverFolderId === 'root'}
        on:dragover|preventDefault={() => (dragOverFolderId = 'root')}
        on:dragleave={() => (dragOverFolderId = null)}
        on:drop|preventDefault={(e) => onDrop('root', e)}
      >
        <button class="section-header" on:click={() => toggle('folders')}>
          <Icon name={sectionOpen.folders ? 'chevron-down' : 'chevron-right'} size={11} />
          <span>{$t('sidebar.folders')}</span>
        </button>
        <button class="add-folder" title="New Folder" on:click={onAddFolder}>
          <Icon name="plus" size={11} />
        </button>
      </div>
      {#if sectionOpen.folders}
        {#each folderTree as { folder, depth } (folder.id)}
          {@const folderHosts = byFolder(folder.id)}
          {@const collapsed = collapsedFolders.has(folder.id)}
          <!-- svelte-ignore a11y-no-static-element-interactions a11y-click-events-have-key-events -->
          <div
            class="folder-header"
            class:drag-over={dragOverFolderId === folder.id}
            style="padding-left: {6 + depth * 14}px"
            draggable={true}
            on:click={() => toggleFolder(folder.id)}
            on:contextmenu|preventDefault|stopPropagation={(e) => openFolderContextMenu(folder, e)}
            on:dragstart|stopPropagation={(e) => onFolderDragStart(folder, e)}
            on:dragover|preventDefault={() => (dragOverFolderId = folder.id)}
            on:dragleave={() => (dragOverFolderId = null)}
            on:drop|preventDefault={(e) => onDrop(folder.id, e)}
          >
            <Icon name={collapsed ? 'chevron-right' : 'chevron-down'} size={11} />
            <span class="folder-icon"><Icon name={folder.icon || 'folder'} size={12} /></span>
            <span class="folder-name">{folder.name}</span>
            <span class="count">{folderHosts.length}</span>
          </div>
          {#if !collapsed}
            <div class="children" style="margin-left: {13 + depth * 14}px">
              {#each folderHosts as host (host.id)}
                <HostListItem
                  {host}
                  selected={selectedRow === `folder-${folder.id}:${host.id}`}
                  status={hostConnectionStatuses.get(host.id) ?? 'offline'}
                  editing={renamingHostId === host.id}
                  on:select={() => onSelect(host, `folder-${folder.id}`)}
                  on:connect={() => onConnect(host)}
                  on:contextmenu={(e) => openContextMenu(host, `folder-${folder.id}`, e.detail)}
                  on:favorite={() => setFavorite(host.id, !host.favorite)}
                  on:renameCommit={(e) => commitRename(host, e.detail)}
                  on:renameCancel={() => (renamingHostId = null)}
                  on:dragStart={(e) => onDragStart(host, e.detail)}
                />
              {/each}
              {#if folderHosts.length === 0}
                <div class="empty-hint">{$t('sidebar.dropHostsHere')}</div>
              {/if}
            </div>
          {/if}
        {/each}

        <!-- Loose hosts live directly at tree root; this area also accepts
             drops to remove a host from a folder. -->
        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <div
          class="root-hosts"
          class:drag-over={dragOverFolderId === 'root'}
          on:dragover|preventDefault={() => (dragOverFolderId = 'root')}
          on:dragleave={() => (dragOverFolderId = null)}
          on:drop|preventDefault={(e) => onDrop('root', e)}
        >
          {#each unfoldered as host (host.id)}
            <HostListItem
              {host}
              selected={selectedRow === `root:${host.id}`}
              status={hostConnectionStatuses.get(host.id) ?? 'offline'}
              editing={renamingHostId === host.id}
              on:select={() => onSelect(host, 'root')}
              on:connect={() => onConnect(host)}
              on:contextmenu={(e) => openContextMenu(host, 'root', e.detail)}
              on:favorite={() => setFavorite(host.id, !host.favorite)}
              on:renameCommit={(e) => commitRename(host, e.detail)}
              on:renameCancel={() => (renamingHostId = null)}
              on:dragStart={(e) => onDragStart(host, e.detail)}
            />
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  <button
    type="button"
    class="resize-handle"
    aria-label="Resize sidebar, current width {sidebarWidth} pixels"
    title="Resize sidebar · Double-click to reset"
    on:pointerdown={startSidebarResize}
    on:pointermove={resizeSidebar}
    on:pointerup={finishSidebarResize}
    on:pointercancel={finishSidebarResize}
    on:keydown={resizeSidebarWithKeyboard}
    on:dblclick={resetSidebarWidth}
  ></button>
</div>

{#if contextMenu}
  {@const host = $hosts.find((h) => h.id === contextMenu?.hostId)}
  {#if host}
    <ContextMenu
      x={contextMenu.x}
      y={contextMenu.y}
      items={contextItemsFor(host)}
      on:select={(e) => onContextSelect(host, e.detail)}
      on:close={() => (contextMenu = null)}
    />
  {/if}
{/if}

{#if folderContextMenu}
  {@const folder = $folders.find((f) => f.id === folderContextMenu?.folderId)}
  {#if folder}
    <ContextMenu
      x={folderContextMenu.x}
      y={folderContextMenu.y}
      items={folderContextItems()}
      on:select={(e) => onFolderContextSelect(folder, e.detail)}
      on:close={() => (folderContextMenu = null)}
    />
  {/if}
{/if}

<style>
  .sidebar {
    position: relative;
    display: flex;
    flex-direction: column;
    flex: 0 0 auto;
    height: 100%;
    background: var(--sidebar-bg);
    border-right: 1px solid var(--border-color);
    overflow: hidden;
  }

  .resize-handle {
    position: absolute;
    z-index: 4;
    top: 0;
    right: 0;
    bottom: 0;
    width: 5px;
    min-height: 0;
    padding: 0;
    background: transparent;
    border: 0;
    border-radius: 0;
    cursor: col-resize;
    touch-action: none;
  }

  .resize-handle:hover:not(:disabled) {
    background: transparent;
  }

  .resize-handle::after {
    content: '';
    position: absolute;
    top: 0;
    right: 0;
    bottom: 0;
    width: 2px;
    background: var(--accent-color);
    opacity: 0;
    transition: opacity 160ms ease;
  }

  .resize-handle:hover::after,
  .resize-handle:focus-visible::after,
  .sidebar.resizing .resize-handle::after {
    opacity: 0.75;
  }

  .search {
    display: flex;
    align-items: center;
    gap: 6px;
    height: var(--workspace-header-height);
    padding: 5px 6px;
    color: var(--text-color-secondary);
    border-bottom: 1px solid var(--separator-color);
    flex-shrink: 0;
  }

  .search input {
    flex: 1;
    height: 27px;
    border: 1px solid var(--border-color);
    background: var(--view-bg);
    border-radius: 3px;
    padding: 3px 6px;
    font-size: 12.5px;
  }

  .tree {
    flex: 1;
    overflow-y: auto;
    padding: 4px 4px 12px;
  }

  .section-header-row {
    display: flex;
    align-items: center;
    margin-top: 4px;
  }

  .section-header-row.drag-over,
  .root-hosts.drag-over {
    background: var(--active-bg);
    outline: 1px dashed var(--accent-color);
    outline-offset: -1px;
  }

  .section-header {
    display: flex;
    align-items: center;
    gap: 4px;
    flex: 1;
    padding: 4px 4px;
    background: transparent;
    border: none;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.03em;
    text-transform: uppercase;
    color: var(--text-color-secondary);
    text-align: left;
  }

  .section-header:hover {
    color: var(--text-color);
  }

  .add-folder {
    width: 18px;
    height: 18px;
    padding: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: none;
    color: var(--text-color-secondary);
  }

  .folder-header {
    display: flex;
    align-items: center;
    gap: 5px;
    height: 22px;
    padding: 0 6px;
    font-size: 12px;
    font-weight: 500;
    color: var(--text-color);
    border-radius: 3px;
    cursor: default;
  }

  .folder-header:hover {
    background: var(--hover-bg);
  }

  .folder-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .folder-header .count {
    margin-left: auto;
    font-size: 10.5px;
    font-weight: 400;
    color: var(--disabled-text-color);
    font-variant-numeric: tabular-nums;
  }

  .folder-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 14px;
    flex: 0 0 14px;
    color: var(--accent-color);
  }

  /* Nested content: indentation plus a subtle guide line to make the
     folder → hosts hierarchy visible at a glance. */
  .children {
    margin-left: 13px;
    padding-left: 5px;
    border-left: 1px solid var(--separator-color);
  }

  .empty-hint {
    font-size: 11px;
    color: var(--disabled-text-color);
    font-style: italic;
    padding: 2px 6px;
  }

  .folder-header.drag-over {
    background: var(--active-bg);
    outline: 1px dashed var(--accent-color);
  }

  .root-hosts {
    min-height: 6px;
    border-radius: 3px;
  }

  .tag-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    padding: 2px 6px 6px;
  }

  .tag-chip {
    display: flex;
    align-items: center;
    gap: 3px;
    font-size: 11px;
    padding: 2px 6px;
    border-radius: 9px;
    background: var(--view-bg-alt);
    border: 1px solid var(--border-color);
    color: var(--text-color-secondary);
  }

  .tag-chip.active {
    background: var(--accent-color);
    border-color: var(--accent-color);
    color: var(--accent-text-color);
  }

  .empty-state {
    padding: 24px 16px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .empty-title {
    font-weight: 600;
    margin: 0;
  }

  .empty-body {
    margin: 0;
    color: var(--text-color-secondary);
    font-size: 12px;
    line-height: 1.4;
  }
</style>
