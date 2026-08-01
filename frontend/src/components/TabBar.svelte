<script lang="ts">
  import Icon from './Icon.svelte'
  import ContextMenu from './ContextMenu.svelte'
  import type { ContextMenuItem } from '../types/contextMenu'
  import { t } from '../services/i18n'
  import {
    activeConnectionId,
    activeTabId,
    closeTab,
    closedTabs,
    duplicateTab,
    connections,
    openNewTab,
    reopenLastClosedTab,
    renameTab,
    tabs
  } from '../stores/connections'
  import { broadcastActive, broadcastMembers, pruneMembers, toggleMember } from '../stores/broadcast'
  import { ensureFavicon, faviconOrigin, favicons } from '../stores/favicons'

  let renamingTabId: string | null = null
  let renameValue = ''
  let contextMenu: { x: number; y: number; tabId: string } | null = null

  // Browser tabs show the panel's favicon via the shared backend cache (same
  // source as the sidebar), falling back to a globe until/if it loads.
  $: tabList
    .filter((tab) => tab.kind === 'browser' && tab.url)
    .forEach((tab) => void ensureFavicon(tab.url as string))

  $: tabList = Object.values($tabs)
  // Keep the broadcast group in sync with the set of open tabs.
  $: terminalTabIds = tabList.filter((tab) => tab.kind === 'terminal').map((tab) => tab.tabId)
  $: pruneMembers(new Set(terminalTabIds))

  function selectTab(tabId: string, connectionId: string, kind: string): void {
    activeTabId.set(tabId)
    activeConnectionId.set(kind === 'quick-connect' || kind === 'browser' ? null : connectionId)
  }

  function startRename(tabId: string, current: string): void {
    renamingTabId = tabId
    renameValue = current
  }

  function commitRename(tabId: string): void {
    const trimmed = renameValue.trim()
    if (trimmed) renameTab(tabId, trimmed)
    renamingTabId = null
  }

  async function onNewTab(): Promise<void> {
    openNewTab()
  }

  function statusClass(connectionId: string): string {
    return $connections[connectionId]?.status ?? 'disconnected'
  }

  function onTabMouseDown(event: MouseEvent, tabId: string): void {
    if (event.button !== 1) return
    event.preventDefault()
    event.stopPropagation()
    void closeTab(tabId)
  }

  function openContextMenu(event: MouseEvent, tabId: string, connectionId: string, kind: string): void {
    event.preventDefault()
    selectTab(tabId, connectionId, kind)
    contextMenu = { x: event.clientX, y: event.clientY, tabId }
  }

  $: contextTab = contextMenu ? $tabs[contextMenu.tabId] : null
  $: contextItems = (contextTab?.kind === 'terminal'
    ? [
        { id: 'duplicate', label: $t('menu.session.duplicateTab') },
        { id: 'reopen', label: $t('menu.session.reopenClosedTab'), disabled: $closedTabs.length === 0 },
        { id: 'close', label: $t('menu.session.closeTab'), separatorBefore: true }
      ]
    : [{ id: 'close', label: $t('menu.session.closeTab') }]) satisfies ContextMenuItem[]

  function onContextSelect(action: string): void {
    const tabId = contextMenu?.tabId
    if (action === 'duplicate' && tabId) void duplicateTab(tabId)
    else if (action === 'reopen') void reopenLastClosedTab()
    else if (action === 'close' && tabId) void closeTab(tabId)
  }
</script>

<div class="tabbar" role="tablist" aria-label="Connection tabs">
  {#each tabList as tab (tab.tabId)}
    <div
      class="tab"
      class:active={$activeTabId === tab.tabId}
      class:broadcasting={$broadcastActive && $broadcastMembers.has(tab.tabId)}
      class:attention={tab.unread && $activeTabId !== tab.tabId}
      role="tab"
      aria-selected={$activeTabId === tab.tabId}
      title={tab.bell ? `${tab.title} — terminal bell` : tab.unread ? `${tab.title} — new output` : tab.title}
      tabindex="-1"
      on:click={() => selectTab(tab.tabId, tab.connectionId, tab.kind)}
      on:mousedown={(e) => onTabMouseDown(e, tab.tabId)}
      on:contextmenu={(e) => openContextMenu(e, tab.tabId, tab.connectionId, tab.kind)}
      on:dblclick={() => startRename(tab.tabId, tab.title)}
      on:keydown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') selectTab(tab.tabId, tab.connectionId, tab.kind)
      }}
    >
      {#if tab.kind === 'quick-connect'}
        <span class="quick-icon"><Icon name="terminal" size={11} /></span>
      {:else if tab.kind === 'browser'}
        {@const fav = $favicons[faviconOrigin(tab.url ?? '')]}
        {#if fav}
          <img class="favicon" src={fav} alt="" />
        {:else}
          <span class="quick-icon"><Icon name="globe" size={11} /></span>
        {/if}
      {:else if $broadcastActive && tab.kind === 'terminal'}
        <button
          class="bcast"
          class:on={$broadcastMembers.has(tab.tabId)}
          title={$broadcastMembers.has(tab.tabId) ? 'Broadcasting to this tab (click to exclude)' : 'Excluded from broadcast (click to include)'}
          on:click|stopPropagation={() => toggleMember(tab.tabId)}
        >
          <Icon name="broadcast" size={11} />
        </button>
      {:else}
        <span class="dot {statusClass(tab.connectionId)}" />
      {/if}
      {#if renamingTabId === tab.tabId}
        <!-- svelte-ignore a11y-autofocus -->
        <input
          class="rename"
          type="text"
          bind:value={renameValue}
          on:blur={() => commitRename(tab.tabId)}
          on:keydown={(e) => {
            if (e.key === 'Enter') commitRename(tab.tabId)
            if (e.key === 'Escape') (renamingTabId = null)
          }}
          on:click|stopPropagation
          autofocus
        />
      {:else}
        <span class="title">{tab.title}</span>
      {/if}
      {#if tab.bell && $activeTabId !== tab.tabId}
        <span class="bell" title="Terminal bell"><Icon name="bell" size={11} /></span>
      {/if}
      <button
        class="close"
        title={$t('menu.session.closeTab')}
        on:click|stopPropagation={() => closeTab(tab.tabId)}
      >
        <Icon name="x" size={10} />
      </button>
    </div>
  {/each}
  <button
    class="new-tab"
    title={$t('menu.session.newTab')}
    on:click={onNewTab}
  >
    <Icon name="plus" size={12} />
  </button>
</div>

{#if contextMenu}
  <ContextMenu
    x={contextMenu.x}
    y={contextMenu.y}
    items={contextItems}
    on:select={(event) => onContextSelect(event.detail)}
    on:close={() => (contextMenu = null)}
  />
{/if}

<style>
  .tabbar {
    position: relative;
    z-index: 1;
    display: flex;
    align-items: stretch;
    height: var(--workspace-header-height);
    background: var(--header-bg);
    border-bottom: 1px solid var(--border-color);
    overflow-x: auto;
    flex-shrink: 0;
  }

  .tab {
    display: flex;
    align-items: center;
    gap: 5px;
    padding: 0 6px 0 8px;
    max-width: 180px;
    border-right: 1px solid var(--separator-color);
    font-size: 12px;
    white-space: nowrap;
    color: var(--text-color-secondary);
  }

  .tab:hover {
    background: var(--hover-bg);
  }

  .tab.active {
    background: var(--view-bg);
    color: var(--text-color);
  }

  .tab.attention {
    background: var(--warning-bg);
    box-shadow: inset 0 -2px 0 var(--warning-color);
    color: var(--warning-color);
  }

  .tab.attention:hover {
    background: var(--warning-bg-hover);
  }

  .tab.broadcasting {
    box-shadow: inset 0 -2px 0 var(--accent-color);
  }

  .bcast {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 16px;
    height: 16px;
    padding: 0;
    flex-shrink: 0;
    background: transparent;
    border: none;
    color: var(--disabled-text-color);
  }

  .bcast.on {
    color: var(--accent-color);
  }

  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--disabled-text-color);
    flex-shrink: 0;
  }

  .quick-icon {
    display: inline-flex;
    color: var(--accent-color);
    flex-shrink: 0;
  }

  .favicon {
    width: 13px;
    height: 13px;
    flex-shrink: 0;
    object-fit: contain;
    border-radius: 2px;
  }

  .dot.connected {
    background: var(--success-color);
  }

  .dot.connecting {
    background: var(--warning-color);
  }

  .dot.error {
    background: var(--error-color);
  }

  .title {
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .bell {
    display: flex;
    align-items: center;
    color: var(--warning-color);
    flex-shrink: 0;
  }

  .rename {
    width: 100px;
    height: 18px;
    font-size: 12px;
    padding: 0 3px;
  }

  .close {
    width: 16px;
    height: 16px;
    padding: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: none;
    color: inherit;
    opacity: 0.6;
  }

  .close:hover {
    opacity: 1;
    background: var(--hover-bg);
  }

  .new-tab {
    width: 26px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: none;
    border-right: 1px solid transparent;
    color: var(--text-color-secondary);
  }

  .new-tab:hover:not(:disabled) {
    background: var(--hover-bg);
  }
</style>
