<script lang="ts">
  import { createEventDispatcher, tick } from 'svelte'
  import Icon from './Icon.svelte'
  import { ensureFavicon, faviconOrigin, favicons } from '../stores/favicons'
  import type { Host } from '../types/host'

  export let host: Host
  export let selected: boolean
  export let status: 'offline' | 'connecting' | 'online'
  export let editing: boolean
  export let reorderable = false

  const dispatch = createEventDispatcher<{
    select: void
    connect: void
    contextmenu: MouseEvent
    favorite: void
    renameCommit: string
    renameCancel: void
    dragStart: DragEvent
    dragEnd: DragEvent
  }>()

  let editValue = host.label
  let inputEl: HTMLInputElement | undefined
  let dragging = false

  // Web hosts show their panel's cached favicon (globe until/if it loads). The
  // backend fetches and persists it; we just request it once per host here.
  $: isWeb = host.protocol === 'web' && !!host.controlPanelUrl
  $: if (isWeb) void ensureFavicon(host.controlPanelUrl)
  $: favicon = isWeb ? $favicons[faviconOrigin(host.controlPanelUrl)] : undefined

  $: if (editing) {
    editValue = host.label
    focusInput()
  }

  async function focusInput(): Promise<void> {
    await tick()
    inputEl?.focus()
    inputEl?.select()
  }

  function commit(): void {
    const trimmed = editValue.trim()
    if (trimmed && trimmed !== host.label) {
      dispatch('renameCommit', trimmed)
    } else {
      dispatch('renameCancel')
    }
  }

  function onEditKeydown(e: KeyboardEvent): void {
    if (e.key === 'Enter') {
      e.preventDefault()
      commit()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      dispatch('renameCancel')
    }
    e.stopPropagation()
  }

  function selectAndFocus(e: MouseEvent): void {
    dispatch('select')
    ;(e.currentTarget as HTMLElement).focus()
  }

  function startDrag(e: DragEvent): void {
    dragging = true
    dispatch('dragStart', e)
  }

  function endDrag(e: DragEvent): void {
    dragging = false
    dispatch('dragEnd', e)
  }
</script>

<!-- Enter/F2/Delete are handled by the parent .tree container's keydown
     listener via normal event bubbling, so no separate keydown here. -->
<!-- svelte-ignore a11y-click-events-have-key-events -->
<div
  class="row"
  data-sidebar-host-row
  class:selected
  class:dragging
  role="option"
  aria-selected={selected}
  aria-keyshortcuts={reorderable ? 'Alt+ArrowUp Alt+ArrowDown' : undefined}
  aria-describedby={reorderable ? 'sidebar-reorder-hint' : undefined}
  tabindex="0"
  draggable={!editing}
  on:click={selectAndFocus}
  on:dblclick={() => dispatch('connect')}
  on:contextmenu|preventDefault={(e) => dispatch('contextmenu', e)}
  on:dragstart={startDrag}
  on:dragend={endDrag}
>
  <span
    class="protocol-icon"
    title={host.protocol === 'sftp' ? 'SFTP' : host.protocol === 'web' ? 'Web' : 'SSH'}
    aria-label={host.protocol === 'sftp' ? 'SFTP host' : host.protocol === 'web' ? 'Web host' : 'SSH host'}
  >
    {#if favicon}
      <img class="favicon" src={favicon} alt="" />
    {:else}
      <Icon name={host.protocol === 'sftp' ? 'sftp' : host.protocol === 'web' ? 'globe' : 'terminal'} size={13} />
    {/if}
  </span>

  {#if editing}
    <input
      bind:this={inputEl}
      class="rename-input"
      type="text"
      bind:value={editValue}
      on:keydown={onEditKeydown}
      on:blur={commit}
      on:click|stopPropagation
    />
  {:else}
    <span class="label" title="{host.user ? host.user + '@' : ''}{host.hostname}">{host.label}</span>
  {/if}

  <button
    class="favorite"
    class:active={host.favorite}
    title="Favorite"
    on:click|stopPropagation={() => dispatch('favorite')}
  >
    <Icon name={host.favorite ? 'star-filled' : 'star'} size={12} />
  </button>

  <span
    class="status-dot"
    class:online={status === 'online'}
    class:connecting={status === 'connecting'}
    title={status}
  />
</div>

<style>
  .row {
    display: flex;
    align-items: center;
    gap: 6px;
    height: 24px;
    padding: 0 8px 0 6px;
    border-radius: 3px;
    cursor: default;
    white-space: nowrap;
    overflow: hidden;
  }

  .row:hover {
    background: var(--hover-bg);
  }

  .row.selected {
    background: var(--highlight-bg);
    color: var(--highlight-text);
  }

  .row.dragging {
    opacity: 0.55;
  }

  .status-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--disabled-text-color);
    flex-shrink: 0;
    margin: 0 1px 0 2px;
  }

  .status-dot.online {
    background: var(--success-color);
  }

  .status-dot.connecting {
    background: var(--warning-color);
    animation: pulse 1s ease-in-out infinite;
  }

  @keyframes pulse {
    0%,
    100% {
      opacity: 1;
    }
    50% {
      opacity: 0.35;
    }
  }

  .protocol-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 14px;
    flex: 0 0 14px;
    color: var(--text-color-secondary);
  }

  .protocol-icon .favicon {
    width: 14px;
    height: 14px;
    object-fit: contain;
    border-radius: 2px;
  }

  .row.selected .protocol-icon {
    color: inherit;
    opacity: 0.9;
  }

  .label {
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }

  .favorite {
    margin-left: auto;
    padding: 0;
    width: 18px;
    height: 18px;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: none;
    color: var(--disabled-text-color);
    opacity: 0;
  }

  .row:hover .favorite,
  .favorite.active {
    opacity: 1;
  }

  .favorite.active {
    color: var(--warning-color);
  }

  .rename-input {
    flex: 1;
    height: 18px;
    padding: 0 4px;
    font-size: 12.5px;
  }
</style>
