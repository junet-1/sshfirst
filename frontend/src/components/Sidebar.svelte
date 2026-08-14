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
    byFolderOrder,
    byHostOrder,
    deleteFolder,
    favoriteHosts,
    filteredHosts,
    folders,
    hosts,
    duplicateHost,
    importSSHConfigWithFeedback,
    reorderFolder,
    reorderHost,
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
  import { adjacentItem, treeDropPosition, type RelativeDropPosition } from '../lib/sidebarReorder'

  const HOST_DND = 'application/x-ssh-first-host'
  const FOLDER_DND = 'application/x-ssh-first-folder'
  const DEFAULT_SIDEBAR_WIDTH = 260
  const MIN_SIDEBAR_WIDTH = 190
  const MAX_SIDEBAR_WIDTH = 480
  type HostConnectionStatus = 'offline' | 'connecting' | 'online'
  type DragItem = { kind: 'host' | 'folder'; id: number }
  type DropTarget =
    | { kind: 'host'; id: number; folderId: number | null; position: RelativeDropPosition }
    | { kind: 'folder'; id: number; parentId: number | null; position: RelativeDropPosition | 'inside' }
    | { kind: 'root'; surface: 'header' | 'list' }

  let sectionOpen: Record<string, boolean> = { favorites: true, recent: true, folders: true, tags: false }
  let renamingHostId: number | null = null
  let contextMenu: { x: number; y: number; hostId: number } | null = null
  let activeTagFilter: string | null = null
  let draggedItem: DragItem | null = null
  let dropTarget: DropTarget | null = null
  let reorderAnnouncement = ''
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
    for (const arr of byParent.values()) arr.sort(byFolderOrder)
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

  $: unfoldered = tagFiltered.filter((h) => h.folderId == null).sort(byHostOrder)
  $: byFolder = (folderId: number) => tagFiltered.filter((h) => h.folderId === folderId).sort(byHostOrder)

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
        openControlPanelTab(host.label, host.controlPanelUrl, host.id)
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
    draggedItem = { kind: 'host', id: host.id }
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData(HOST_DND, String(host.id))
  }

  function onFolderDragStart(folder: Folder, e: DragEvent): void {
    if (!e.dataTransfer) return
    draggedItem = { kind: 'folder', id: folder.id }
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData(FOLDER_DND, String(folder.id))
  }

  function finishDrag(): void {
    draggedItem = null
    dropTarget = null
  }

  function dragItemFromEvent(e: DragEvent): DragItem | null {
    if (draggedItem) return draggedItem
    const hostID = Number(e.dataTransfer?.getData(HOST_DND))
    if (Number.isFinite(hostID) && hostID > 0) return { kind: 'host', id: hostID }
    const folderID = Number(e.dataTransfer?.getData(FOLDER_DND))
    if (Number.isFinite(folderID) && folderID > 0) return { kind: 'folder', id: folderID }
    return null
  }

  function allowMove(e: DragEvent): void {
    e.preventDefault()
    e.stopPropagation()
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
  }

  function relativePosition(e: DragEvent, inside = false): RelativeDropPosition | 'inside' {
    const bounds = (e.currentTarget as HTMLElement).getBoundingClientRect()
    return treeDropPosition(e.clientY, bounds.top, bounds.height, inside)
  }

  function onHostDragOver(host: Host, folderId: number | null, e: DragEvent): void {
    if (draggedItem) e.stopPropagation()
    if (draggedItem?.kind !== 'host' || draggedItem.id === host.id) return
    allowMove(e)
    dropTarget = { kind: 'host', id: host.id, folderId, position: relativePosition(e) as RelativeDropPosition }
  }

  async function onHostDrop(host: Host, folderId: number | null, e: DragEvent): Promise<void> {
    const item = dragItemFromEvent(e)
    if (item) e.stopPropagation()
    if (item?.kind !== 'host' || item.id === host.id) {
      finishDrag()
      return
    }
    allowMove(e)
    const position =
      dropTarget?.kind === 'host' && dropTarget.id === host.id
        ? dropTarget.position
        : (relativePosition(e) as RelativeDropPosition)
    finishDrag()
    try {
      await reorderHost(item.id, folderId, host.id, position === 'before')
      const moved = $hosts.find((candidate) => candidate.id === item.id)
      if (moved) reorderAnnouncement = $t('sidebar.reorder.host', { name: moved.label })
    } catch (err) {
      notify('error', err instanceof Error ? err.message : String(err))
    }
  }

  function onFolderDragOver(folder: Folder, e: DragEvent): void {
    if (draggedItem) e.stopPropagation()
    if (!draggedItem || (draggedItem.kind === 'folder' && draggedItem.id === folder.id)) return
    allowMove(e)
    dropTarget = {
      kind: 'folder',
      id: folder.id,
      parentId: folder.parentId ?? null,
      position: draggedItem.kind === 'host' ? 'inside' : relativePosition(e, true)
    }
  }

  async function onFolderDrop(folder: Folder, e: DragEvent): Promise<void> {
    const item = dragItemFromEvent(e)
    if (item) e.stopPropagation()
    if (!item || (item.kind === 'folder' && item.id === folder.id)) {
      finishDrag()
      return
    }
    allowMove(e)
    const position =
      dropTarget?.kind === 'folder' && dropTarget.id === folder.id
        ? dropTarget.position
        : item.kind === 'host'
          ? 'inside'
          : relativePosition(e, true)
    finishDrag()
    try {
      if (item.kind === 'host') {
        await reorderHost(item.id, folder.id, null, false)
        const moved = $hosts.find((candidate) => candidate.id === item.id)
        if (moved) reorderAnnouncement = $t('sidebar.reorder.host', { name: moved.label })
      } else if (position === 'inside') {
        await reorderFolder(item.id, folder.id, null, false)
        const moved = $folders.find((candidate) => candidate.id === item.id)
        if (moved) reorderAnnouncement = $t('sidebar.reorder.folder', { name: moved.name })
      } else {
        await reorderFolder(item.id, folder.parentId ?? null, folder.id, position === 'before')
        const moved = $folders.find((candidate) => candidate.id === item.id)
        if (moved) reorderAnnouncement = $t('sidebar.reorder.folder', { name: moved.name })
      }
    } catch (err) {
      notify('error', err instanceof Error ? err.message : String(err))
    }
  }

  function onRootDragOver(surface: 'header' | 'list', e: DragEvent): void {
    if (!draggedItem) return
    allowMove(e)
    dropTarget = { kind: 'root', surface }
  }

  async function onRootDrop(e: DragEvent): Promise<void> {
    const item = dragItemFromEvent(e)
    if (!item) {
      finishDrag()
      return
    }
    allowMove(e)
    finishDrag()
    try {
      if (item.kind === 'host') {
        await reorderHost(item.id, null, null, false)
        const moved = $hosts.find((candidate) => candidate.id === item.id)
        if (moved) reorderAnnouncement = $t('sidebar.reorder.host', { name: moved.label })
      } else {
        await reorderFolder(item.id, null, null, false)
        const moved = $folders.find((candidate) => candidate.id === item.id)
        if (moved) reorderAnnouncement = $t('sidebar.reorder.folder', { name: moved.name })
      }
    } catch (err) {
      notify('error', err instanceof Error ? err.message : String(err))
    }
  }

  function onDragLeave(e: DragEvent): void {
    const current = e.currentTarget as HTMLElement
    if (e.relatedTarget instanceof Node && current.contains(e.relatedTarget)) return
    dropTarget = null
  }

  async function moveHostWithKeyboard(host: Host, direction: -1 | 1): Promise<void> {
    const folderId = host.folderId ?? null
    const siblings = tagFiltered.filter((candidate) => (candidate.folderId ?? null) === folderId).sort(byHostOrder)
    const target = adjacentItem(siblings, host.id, direction)
    if (!target) return
    try {
      await reorderHost(host.id, folderId, target.id, direction < 0)
      reorderAnnouncement = $t('sidebar.reorder.host', { name: host.label })
    } catch (err) {
      notify('error', err instanceof Error ? err.message : String(err))
    }
  }

  async function moveFolderWithKeyboard(folder: Folder, direction: -1 | 1): Promise<void> {
    const parentId = folder.parentId ?? null
    const siblings = $folders.filter((candidate) => (candidate.parentId ?? null) === parentId).sort(byFolderOrder)
    const target = adjacentItem(siblings, folder.id, direction)
    if (!target) return
    try {
      await reorderFolder(folder.id, parentId, target.id, direction < 0)
      reorderAnnouncement = $t('sidebar.reorder.folder', { name: folder.name })
    } catch (err) {
      notify('error', err instanceof Error ? err.message : String(err))
    }
  }

  function onFolderKeydown(folder: Folder, e: KeyboardEvent): void {
    // Folder rows live inside the host listbox; never let their keys act on a
    // previously selected host through the tree's bubbled key handler.
    e.stopPropagation()
    if (e.altKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
      e.preventDefault()
      void moveFolderWithKeyboard(folder, e.key === 'ArrowUp' ? -1 : 1)
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      toggleFolder(folder.id)
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

    if (
      e.altKey &&
      (e.key === 'ArrowUp' || e.key === 'ArrowDown') &&
      (selectedRow === 'root:' + host.id ||
        (selectedRow?.startsWith('folder-') && selectedRow.endsWith(':' + host.id)))
    ) {
      e.preventDefault()
      void moveHostWithKeyboard(host, e.key === 'ArrowUp' ? -1 : 1)
    } else if (e.key === 'Enter') {
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
              on:dragEnd={finishDrag}
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
              on:dragEnd={finishDrag}
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
        class:drag-over={dropTarget?.kind === 'root' && dropTarget.surface === 'header'}
        on:dragover={(e) => onRootDragOver('header', e)}
        on:dragleave={onDragLeave}
        on:drop={onRootDrop}
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
          <div
            class="folder-header"
            class:drop-inside={dropTarget?.kind === 'folder' && dropTarget.id === folder.id && dropTarget.position === 'inside'}
            class:drop-before={dropTarget?.kind === 'folder' && dropTarget.id === folder.id && dropTarget.position === 'before'}
            class:drop-after={dropTarget?.kind === 'folder' && dropTarget.id === folder.id && dropTarget.position === 'after'}
            class:dragging={draggedItem?.kind === 'folder' && draggedItem.id === folder.id}
            style="padding-left: {6 + depth * 14}px"
            role="button"
            tabindex="0"
            aria-expanded={!collapsed}
            aria-keyshortcuts="Alt+ArrowUp Alt+ArrowDown"
            aria-describedby="sidebar-reorder-hint"
            draggable={true}
            on:click={() => toggleFolder(folder.id)}
            on:keydown={(e) => onFolderKeydown(folder, e)}
            on:contextmenu|preventDefault|stopPropagation={(e) => openFolderContextMenu(folder, e)}
            on:dragstart|stopPropagation={(e) => onFolderDragStart(folder, e)}
            on:dragend={finishDrag}
            on:dragover={(e) => onFolderDragOver(folder, e)}
            on:dragleave={onDragLeave}
            on:drop={(e) => onFolderDrop(folder, e)}
          >
            <Icon name={collapsed ? 'chevron-right' : 'chevron-down'} size={11} />
            <span class="folder-icon"><Icon name={folder.icon || 'folder'} size={12} /></span>
            <span class="folder-name">{folder.name}</span>
            <span class="count">{folderHosts.length}</span>
          </div>
          {#if !collapsed}
            <div class="children" style="margin-left: {13 + depth * 14}px">
              {#each folderHosts as host (host.id)}
                <div
                  class="host-drop-target"
                  role="presentation"
                  class:drop-before={dropTarget?.kind === 'host' && dropTarget.id === host.id && dropTarget.position === 'before'}
                  class:drop-after={dropTarget?.kind === 'host' && dropTarget.id === host.id && dropTarget.position === 'after'}
                  on:dragover={(e) => onHostDragOver(host, folder.id, e)}
                  on:dragleave|stopPropagation={onDragLeave}
                  on:drop={(e) => onHostDrop(host, folder.id, e)}
                >
                  <HostListItem
                    {host}
                    selected={selectedRow === `folder-${folder.id}:${host.id}`}
                    status={hostConnectionStatuses.get(host.id) ?? 'offline'}
                    editing={renamingHostId === host.id}
                    reorderable={true}
                    on:select={() => onSelect(host, `folder-${folder.id}`)}
                    on:connect={() => onConnect(host)}
                    on:contextmenu={(e) => openContextMenu(host, `folder-${folder.id}`, e.detail)}
                    on:favorite={() => setFavorite(host.id, !host.favorite)}
                    on:renameCommit={(e) => commitRename(host, e.detail)}
                    on:renameCancel={() => (renamingHostId = null)}
                    on:dragStart={(e) => onDragStart(host, e.detail)}
                    on:dragEnd={finishDrag}
                  />
                </div>
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
          class:drag-over={dropTarget?.kind === 'root' && dropTarget.surface === 'list'}
          on:dragover={(e) => onRootDragOver('list', e)}
          on:dragleave={onDragLeave}
          on:drop={onRootDrop}
        >
          {#each unfoldered as host (host.id)}
            <div
              class="host-drop-target"
              role="presentation"
              class:drop-before={dropTarget?.kind === 'host' && dropTarget.id === host.id && dropTarget.position === 'before'}
              class:drop-after={dropTarget?.kind === 'host' && dropTarget.id === host.id && dropTarget.position === 'after'}
              on:dragover={(e) => onHostDragOver(host, null, e)}
              on:dragleave|stopPropagation={onDragLeave}
              on:drop={(e) => onHostDrop(host, null, e)}
            >
              <HostListItem
                {host}
                selected={selectedRow === `root:${host.id}`}
                status={hostConnectionStatuses.get(host.id) ?? 'offline'}
                editing={renamingHostId === host.id}
                reorderable={true}
                on:select={() => onSelect(host, 'root')}
                on:connect={() => onConnect(host)}
                on:contextmenu={(e) => openContextMenu(host, 'root', e.detail)}
                on:favorite={() => setFavorite(host.id, !host.favorite)}
                on:renameCommit={(e) => commitRename(host, e.detail)}
                on:renameCancel={() => (renamingHostId = null)}
                on:dragStart={(e) => onDragStart(host, e.detail)}
                on:dragEnd={finishDrag}
              />
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  <div id="sidebar-reorder-hint" class="sr-only">{$t('sidebar.reorder.hint')}</div>
  <div class="sr-only" aria-live="polite">{reorderAnnouncement}</div>

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
    position: relative;
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

  .folder-header.dragging {
    opacity: 0.55;
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

  .folder-header.drop-inside {
    background: var(--active-bg);
    outline: 1px dashed var(--accent-color);
    outline-offset: -1px;
  }

  .host-drop-target {
    position: relative;
  }

  .host-drop-target.drop-before::before,
  .host-drop-target.drop-after::after,
  .folder-header.drop-before::before,
  .folder-header.drop-after::after {
    content: '';
    position: absolute;
    z-index: 3;
    left: 4px;
    right: 4px;
    height: 2px;
    border-radius: 1px;
    background: var(--accent-color);
    pointer-events: none;
  }

  .host-drop-target.drop-before::before,
  .folder-header.drop-before::before {
    top: -1px;
  }

  .host-drop-target.drop-after::after,
  .folder-header.drop-after::after {
    bottom: -1px;
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

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
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
