<script lang="ts">
  import { createEventDispatcher, onDestroy, onMount } from 'svelte'
  import type { ContextMenuItem } from '../types/contextMenu'
  import { openContextMenus } from '../stores/ui'

  export let x: number
  export let y: number
  export let items: ContextMenuItem[]

  const dispatch = createEventDispatcher<{ select: string; close: void }>()
  let menuEl: HTMLDivElement

  // Announced globally so panel views, which are native widgets stacked above
  // the page, get out of the way while this menu is on screen.
  openContextMenus.update((n) => n + 1)
  onDestroy(() => openContextMenus.update((n) => Math.max(0, n - 1)))

  onMount(() => {
    const rect = menuEl.getBoundingClientRect()
    const overflowX = x + rect.width - window.innerWidth
    const overflowY = y + rect.height - window.innerHeight
    if (overflowX > 0) menuEl.style.left = `${Math.max(4, x - overflowX)}px`
    if (overflowY > 0) menuEl.style.top = `${Math.max(4, y - overflowY)}px`
    menuEl.focus()
  })

  function select(id: string): void {
    if (items.find((item) => item.id === id)?.disabled) return
    dispatch('select', id)
    dispatch('close')
  }

  function onKeydown(e: KeyboardEvent): void {
    if (e.key === 'Escape') dispatch('close')
  }
</script>

<!-- Bubble phase (not capture): a mousedown inside the menu is stopped by the
     menu div's own handler below, so only clicks OUTSIDE reach here and close
     it. With capture this fired before the item's click, closing the menu
     before the selection could register. -->
<svelte:window on:mousedown={() => dispatch('close')} />

<div
  bind:this={menuEl}
  class="context-menu"
  style="left: {x}px; top: {y}px;"
  tabindex="-1"
  role="menu"
  on:mousedown|stopPropagation
  on:keydown={onKeydown}
>
  {#each items as item (item.id)}
    {#if item.separatorBefore}<div class="separator" />{/if}
    <button role="menuitem" class="item" class:destructive={item.destructive} disabled={item.disabled} on:click={() => select(item.id)}>
      {item.label}
    </button>
  {/each}
</div>

<style>
  .context-menu {
    position: fixed;
    min-width: 180px;
    background: var(--view-bg);
    border: 1px solid var(--border-color);
    border-radius: 4px;
    box-shadow: 0 4px 14px var(--shadow-color);
    padding: 4px;
    z-index: 1000;
  }

  .item {
    display: block;
    width: 100%;
    text-align: left;
    padding: 5px 10px;
    background: transparent;
    border: none;
    border-radius: 3px;
    font-size: 12.5px;
  }

  .item:hover {
    background: var(--highlight-bg);
    color: var(--highlight-text);
  }

  .item:disabled {
    color: var(--disabled-text-color);
    opacity: 0.65;
  }

  .item:disabled:hover {
    background: transparent;
  }

  .item.destructive {
    color: var(--error-color);
  }

  .item.destructive:hover {
    background: var(--error-color);
    color: #fff;
  }

  .separator {
    height: 1px;
    background: var(--separator-color);
    margin: 4px 2px;
  }
</style>
