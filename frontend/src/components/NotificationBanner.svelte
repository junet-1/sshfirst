<script lang="ts">
  import Icon from './Icon.svelte'
  import { dismiss, notifications } from '../stores/notifications'
</script>

{#if $notifications.length}
  <div class="stack">
    {#each $notifications as n (n.id)}
      <div class="banner {n.kind}" role="alert">
        <Icon name={n.kind === 'error' ? 'warning' : 'dot'} size={14} />
        <span class="message">{n.message}</span>
        <button class="close" title="Dismiss" on:click={() => dismiss(n.id)}>
          <Icon name="x" size={11} />
        </button>
      </div>
    {/each}
  </div>
{/if}

<style>
  .stack {
    position: fixed;
    bottom: 28px;
    right: 12px;
    display: flex;
    flex-direction: column;
    gap: 6px;
    z-index: 700;
    max-width: 460px;
  }

  .banner {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 8px 10px;
    border-radius: 4px;
    font-size: 12.5px;
    background: var(--view-bg);
    border: 1px solid var(--border-color);
    box-shadow: 0 4px 14px var(--shadow-color);
  }

  .banner.error {
    border-color: var(--error-color);
  }

  .banner.error :global(.icon) {
    color: var(--error-color);
  }

  .message {
    flex: 1;
    overflow-wrap: anywhere;
    line-height: 1.4;
  }

  .close {
    flex-shrink: 0;
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

  .close:hover {
    color: var(--text-color);
  }
</style>
