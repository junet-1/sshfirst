<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  import { t } from '../services/i18n'

  export let content: string

  const dispatch = createEventDispatcher<{ confirm: void; cancel: void }>()
  let cancelButton: HTMLButtonElement

  $: lineCount = content.split(/\r\n|\r|\n/).length
  $: preview = content
    .replace(/\x1b/g, '␛')
    .replace(/\r(?!\n)/g, '↵\n')

  onMount(() => cancelButton.focus())

  function onKeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      event.preventDefault()
      dispatch('cancel')
    } else if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
      event.preventDefault()
      dispatch('confirm')
    }
  }
</script>

<svelte:window on:keydown={onKeydown} />

<div class="guard" role="presentation">
  <section role="alertdialog" aria-modal="true" aria-labelledby="paste-title" aria-describedby="paste-description">
    <header id="paste-title">{$t('pasteGuard.title')}</header>
    <div class="content">
      <p id="paste-description">{$t('pasteGuard.body', { lines: lineCount, characters: content.length })}</p>
      <pre>{preview}</pre>
    </div>
    <footer>
      <span>{$t('pasteGuard.hint')}</span>
      <button bind:this={cancelButton} on:click={() => dispatch('cancel')}>{$t('pasteGuard.cancel')}</button>
      <button class="primary" on:click={() => dispatch('confirm')}>{$t('pasteGuard.paste')}</button>
    </footer>
  </section>
</div>

<style>
  .guard {
    position: absolute;
    z-index: 20;
    inset: 0;
    display: grid;
    place-items: center;
    padding: 24px;
    background: rgba(36, 39, 42, 0.42);
    backdrop-filter: blur(3px);
  }

  section {
    width: min(560px, 90%);
    max-height: min(420px, 82%);
    display: flex;
    flex-direction: column;
    background: var(--window-bg);
    border: 1px solid var(--border-color);
    border-radius: 4px;
    box-shadow: 0 10px 30px var(--shadow-color);
  }

  header {
    padding: 9px 12px;
    border-bottom: 1px solid var(--separator-color);
    font-size: 12.5px;
    font-weight: 600;
  }

  .content {
    min-height: 0;
    padding: 10px 12px;
  }

  p {
    margin: 0 0 8px;
    color: var(--text-color-secondary);
    font-size: 12px;
  }

  pre {
    max-height: 250px;
    margin: 0;
    padding: 8px;
    overflow: auto;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    color: var(--terminal-fg);
    background: var(--terminal-bg);
    border: 1px solid var(--border-color);
    border-radius: 3px;
    font-family: 'Cascadia Code', 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11.5px;
    line-height: 1.35;
    user-select: text;
  }

  footer {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 6px;
    padding: 8px 12px;
    border-top: 1px solid var(--separator-color);
  }

  footer span {
    margin-right: auto;
    color: var(--disabled-text-color);
    font-size: 10.5px;
  }

  @media (prefers-reduced-motion: no-preference) {
    .guard {
      animation: guard-in 160ms ease-out;
    }
  }

  @keyframes guard-in {
    from { opacity: 0; }
    to { opacity: 1; }
  }
</style>
