<script lang="ts">
  import Modal from './Modal.svelte'
  import Icon from '../Icon.svelte'
  import { t } from '../../services/i18n'
  import { backend } from '../../services/backend'
  import { dequeueHostKey, hostKeyQueue } from '../../stores/prompts'

  $: current = $hostKeyQueue[0] ?? null

  async function respond(accept: boolean): Promise<void> {
    if (!current) return
    const requestId = current.requestId
    dequeueHostKey(requestId)
    await backend.respondHostKey(requestId, accept)
  }
</script>

{#if current}
  <Modal
    titleText={current.status === 'changed' ? $t('hostKeyPrompt.title.changed') : $t('hostKeyPrompt.title.unknown')}
    dismissable={false}
    width={440}
  >
    <div class="content">
      {#if current.status === 'changed'}
        <div class="warning">
          <Icon name="warning" size={20} />
          <p>{$t('hostKeyPrompt.body.changed', { hostname: current.hostname })}</p>
        </div>
      {:else}
        <p>{$t('hostKeyPrompt.body.unknown', { hostname: current.hostname })}</p>
      {/if}

      <div class="fingerprint">
        <span class="label">{$t('hostKeyPrompt.fingerprint.new', { algorithm: current.algorithm })}</span>
        <span class="value mono">{current.fingerprint}</span>
      </div>
      {#if current.status === 'changed' && current.previousFingerprints?.length}
        <div class="fingerprint previous">
          <span class="label">{$t('hostKeyPrompt.fingerprint.previous')}</span>
          {#each current.previousFingerprints as fingerprint (fingerprint)}
            <span class="value mono">{fingerprint}</span>
          {/each}
        </div>
      {/if}
    </div>

    <svelte:fragment slot="footer">
      <button on:click={() => respond(false)}>{$t('hostKeyPrompt.reject')}</button>
      <button class="primary" class:danger={current.status === 'changed'} on:click={() => respond(true)}>
        {$t('hostKeyPrompt.accept')}
      </button>
    </svelte:fragment>
  </Modal>
{/if}

<style>
  .content {
    display: flex;
    flex-direction: column;
    gap: 12px;
    font-size: 12.5px;
  }

  .warning {
    display: flex;
    gap: 8px;
    align-items: flex-start;
    color: var(--error-color);
  }

  .warning p {
    margin: 0;
    font-weight: 600;
  }

  .fingerprint {
    display: flex;
    flex-direction: column;
    gap: 3px;
    background: var(--view-bg-alt);
    border: 1px solid var(--border-color);
    border-radius: 3px;
    padding: 8px;
  }

  .label {
    color: var(--text-color-secondary);
    font-size: 11px;
  }

  .mono {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    overflow-wrap: anywhere;
  }

  .previous {
    border-color: color-mix(in srgb, var(--error-color) 55%, var(--border-color));
  }

  .primary.danger {
    background: var(--error-color);
    border-color: var(--error-color);
  }
</style>
