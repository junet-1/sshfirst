<script lang="ts">
  import { createEventDispatcher, onMount, tick } from 'svelte'

  import { fitCurrentWindowHeight } from '../../services/backend'

  export let titleText: string
  export let dismissable = true
  export let width = 420

  const dispatch = createEventDispatcher<{ close: void }>()
  const dedicatedWindow = document.documentElement.dataset.toolWindow != null
  let panel: HTMLDivElement
  let bodyEl: HTMLDivElement
  let footerEl: HTMLDivElement

  onMount(() => {
    panel.querySelector<HTMLElement>('input, textarea, select, button')?.focus()
    if (dedicatedWindow) void fitWindow()
  })

  // In a dedicated tool window the panel fills the window, so measure the
  // natural (unstretched) content instead: body.scrollHeight reports the real
  // content height whether it overflows or leaves slack, and the footer keeps
  // its intrinsic height. Resize the window to that once the layout settles.
  async function fitWindow(): Promise<void> {
    await tick()
    await document.fonts?.ready
    await new Promise((resolve) => requestAnimationFrame(() => resolve(null)))
    const desired = bodyEl.scrollHeight + (footerEl?.offsetHeight ?? 0)
    if (desired > 0) await fitCurrentWindowHeight(desired)
  }

  function onKeydown(e: KeyboardEvent): void {
    if (e.key === 'Escape' && dismissable) {
      e.preventDefault()
      dispatch('close')
    }
  }

  function onBackdropClick(): void {
    if (dismissable) dispatch('close')
  }
</script>

<svelte:window on:keydown={onKeydown} />

<div class="backdrop" class:dedicated={dedicatedWindow} role="presentation" on:mousedown={onBackdropClick}>
  <!-- svelte-ignore a11y-no-noninteractive-element-interactions -->
  <div
    class="panel"
    style="--dialog-width: {width}px;"
    role={dedicatedWindow ? 'region' : 'dialog'}
    aria-modal={dedicatedWindow ? undefined : 'true'}
    aria-label={titleText}
    bind:this={panel}
    on:mousedown|stopPropagation
  >
    <div class="titlebar">
      <span>{titleText}</span>
    </div>
    <div class="body" bind:this={bodyEl}>
      <slot />
    </div>
    {#if $$slots.footer}
      <div class="footer" bind:this={footerEl}>
        <slot name="footer" />
      </div>
    {/if}
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.35);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 500;
    padding: 12px;
  }

  .backdrop.dedicated {
    position: relative;
    width: 100%;
    height: 100%;
    padding: 0;
    display: block;
    background: var(--window-bg);
  }

  .dedicated .panel {
    width: 100%;
    height: 100%;
    max-width: none;
    max-height: none;
    border: 0;
    border-radius: 0;
    box-shadow: none;
  }

  .dedicated .titlebar {
    display: none;
  }

  .dedicated .body {
    padding: 18px;
  }

  .dedicated .footer {
    padding: 10px 18px;
  }

  .panel {
    width: min(var(--dialog-width), calc(100vw - 24px));
    max-width: 100%;
    background: var(--window-bg);
    border: 1px solid var(--border-color);
    border-radius: 5px;
    box-shadow: 0 12px 32px var(--shadow-color);
    display: flex;
    flex-direction: column;
    max-height: calc(100vh - 24px);
    min-width: 0;
    overflow: hidden;
  }

  .titlebar {
    padding: 8px 12px;
    font-weight: 600;
    font-size: 12.5px;
    border-bottom: 1px solid var(--separator-color);
  }

  .body {
    padding: 12px;
    overflow-y: auto;
    min-height: 0;
  }

  .footer {
    display: flex;
    justify-content: flex-end;
    gap: 6px;
    padding: 8px 12px;
    border-top: 1px solid var(--separator-color);
    flex-wrap: wrap;
  }
</style>
