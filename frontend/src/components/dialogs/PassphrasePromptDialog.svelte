<script lang="ts">
  import Modal from './Modal.svelte'
  import { t } from '../../services/i18n'
  import { backend } from '../../services/backend'
  import { dequeuePassphrase, passphraseQueue } from '../../stores/prompts'

  let passphrase = ''
  let remember = false
  let shownRequestId: string | null = null

  $: current = $passphraseQueue[0] ?? null
  // Reset only for a genuinely new request, so it can never wipe mid-typing.
  $: if (current && current.requestId !== shownRequestId) {
    shownRequestId = current.requestId
    passphrase = ''
    remember = false
  }

  async function submit(): Promise<void> {
    if (!current) return
    const requestId = current.requestId
    dequeuePassphrase(requestId)
    await backend.respondPassphrase(requestId, passphrase, remember)
  }

  async function cancel(): Promise<void> {
    if (!current) return
    const requestId = current.requestId
    dequeuePassphrase(requestId)
    await backend.cancelPassphrasePrompt(requestId)
  }
</script>

{#if current}
  <Modal titleText={$t('passphrasePrompt.title')} dismissable={false} width={380}>
    <form class="form" on:submit|preventDefault={submit}>
      <p class="body">{$t('passphrasePrompt.body', { file: current.identityFile })}</p>
      <input type="password" bind:value={passphrase} autocomplete="off" />
      <label class="checkbox">
        <input type="checkbox" bind:checked={remember} />
        <span>{$t('passphrasePrompt.remember')}</span>
      </label>
    </form>

    <svelte:fragment slot="footer">
      <button on:click={cancel}>{$t('passphrasePrompt.cancel')}</button>
      <button class="primary" on:click={submit}>{$t('passphrasePrompt.unlock')}</button>
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
    overflow-wrap: anywhere;
  }

  .checkbox {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
  }
</style>
