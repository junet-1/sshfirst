<script lang="ts">
  import Modal from './Modal.svelte'
  import { t } from '../../services/i18n'
  import { backend } from '../../services/backend'
  import { dequeuePassword, passwordQueue } from '../../stores/prompts'

  let password = ''
  let remember = false
  let shownRequestId: string | null = null

  $: current = $passwordQueue[0] ?? null
  // Reset only when a genuinely new request appears (tracked by requestId), so
  // this can never wipe the field mid-typing even if the block re-evaluates.
  $: if (current && current.requestId !== shownRequestId) {
    shownRequestId = current.requestId
    password = ''
    remember = false
  }

  async function submit(): Promise<void> {
    if (!current) return
    const requestId = current.requestId
    dequeuePassword(requestId)
    await backend.respondPassword(requestId, password, remember)
  }

  async function cancel(): Promise<void> {
    if (!current) return
    const requestId = current.requestId
    dequeuePassword(requestId)
    await backend.cancelPasswordPrompt(requestId)
  }
</script>

{#if current}
  <Modal titleText={$t('passwordPrompt.title')} dismissable={false} width={360}>
    <form class="form" on:submit|preventDefault={submit}>
      <p class="body">{$t('passwordPrompt.body', { user: current.user, hostname: current.hostname })}</p>
      <input type="password" bind:value={password} autocomplete="off" />
      {#if current.allowRemember}
        <label class="checkbox">
          <input type="checkbox" bind:checked={remember} />
          <span>{$t('passwordPrompt.remember')}</span>
        </label>
      {/if}
    </form>

    <svelte:fragment slot="footer">
      <button on:click={cancel}>{$t('passwordPrompt.cancel')}</button>
      <button class="primary" on:click={submit}>{$t('passwordPrompt.connect')}</button>
    </svelte:fragment>
  </Modal>
{/if}

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .body {
    margin: 0;
    font-size: 12.5px;
  }

  .checkbox {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
  }
</style>
