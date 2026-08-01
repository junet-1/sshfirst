<script lang="ts">
  import type { DividerRect } from '../lib/layoutTree'
  import { resizeDividerBy } from '../stores/layout'

  export let dividers: DividerRect[]

  let overlay: HTMLDivElement
  // During a drag we track the split being resized plus the fixed geometry we
  // need to turn pixel movement into a size fraction. Deltas are applied
  // incrementally (last -> current), so the live tree recompute doesn't matter.
  let drag: { divider: DividerRect; areaAlong: number; last: number } | null = null

  function beginDrag(event: PointerEvent, divider: DividerRect): void {
    event.preventDefault()
    const box = overlay.getBoundingClientRect()
    const areaAlong = divider.dir === 'row' ? box.width : box.height
    drag = { divider, areaAlong, last: divider.dir === 'row' ? event.clientX : event.clientY }
  }

  function onMove(event: PointerEvent): void {
    if (!drag) return
    const pos = drag.divider.dir === 'row' ? event.clientX : event.clientY
    const movedPx = pos - drag.last
    drag.last = pos
    const spanFraction = drag.divider.span / 100
    if (spanFraction <= 0 || drag.areaAlong <= 0) return
    // px of the whole area -> fraction of the area -> fraction of this split.
    const deltaFraction = movedPx / drag.areaAlong / spanFraction
    if (deltaFraction !== 0) resizeDividerBy(drag.divider.splitId, drag.divider.index, deltaFraction)
  }

  function endDrag(): void {
    drag = null
  }
</script>

<div class="dividers" bind:this={overlay}>
  {#each dividers as divider (`${divider.splitId}:${divider.index}:${divider.dir}`)}
    <div
      class="handle {divider.dir}"
      style={divider.dir === 'row'
        ? `left:${divider.left}%;top:${divider.top}%;height:${divider.length}%;`
        : `left:${divider.left}%;top:${divider.top}%;width:${divider.length}%;`}
      role="separator"
      aria-orientation={divider.dir === 'row' ? 'vertical' : 'horizontal'}
      on:pointerdown={(e) => beginDrag(e, divider)}
    >
      <span class="seam" />
    </div>
  {/each}
</div>

<!-- While dragging, a full-area shield captures every pointer move/up so the
     drag stays smooth over the terminals (and their pointer-swallowing xterm
     surfaces) and doesn't get lost if a handle re-renders. -->
{#if drag}
  <div
    class="shield"
    style="cursor:{drag.divider.dir === 'row' ? 'col-resize' : 'row-resize'}"
    on:pointermove={onMove}
    on:pointerup={endDrag}
    on:pointerleave={endDrag}
  />
{/if}

<style>
  .dividers {
    position: absolute;
    inset: 0;
    z-index: 2;
    pointer-events: none;
  }

  .handle {
    position: absolute;
    pointer-events: auto;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .handle.row {
    width: 12px;
    transform: translateX(-50%);
    cursor: col-resize;
  }

  .handle.column {
    height: 12px;
    transform: translateY(-50%);
    cursor: row-resize;
  }

  /* Always-visible 7.5px seam in the UI chrome grey (--border-color matches the
     terminal background in the dark theme, so it uses --header-bg instead);
     highlights in the accent colour on hover. */
  .seam {
    background: var(--header-bg);
  }

  .handle.row .seam {
    width: 7.5px;
    height: 100%;
  }

  .handle.column .seam {
    width: 100%;
    height: 7.5px;
  }

  .handle:hover .seam {
    background: var(--accent-color);
  }

  .shield {
    position: absolute;
    inset: 0;
    z-index: 3;
    pointer-events: auto;
  }
</style>
