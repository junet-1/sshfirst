<script lang="ts">
  import Modal from './Modal.svelte'
  import { t } from '../../services/i18n'
  import { backend } from '../../services/backend'
  import {
    dequeueKeyboardInteractive,
    keyboardInteractiveQueue
  } from '../../stores/prompts'

  let answers: string[] = []
  let shownRequestId: string | null = null

  $: current = $keyboardInteractiveQueue[0] ?? null
  $: if (current && current.requestId !== shownRequestId) {
    shownRequestId = current.requestId
    answers = current.questions.map(() => '')
  }

  async function submit(): Promise<void> {
    if (!current) return
    const requestId = current.requestId
    const submittedAnswers = [...answers]
    dequeueKeyboardInteractive(requestId)
    await backend.respondKeyboardInteractive(requestId, submittedAnswers)
  }

  async function cancel(): Promise<void> {
    if (!current) return
    const requestId = current.requestId
    dequeueKeyboardInteractive(requestId)
    await backend.cancelKeyboardInteractivePrompt(requestId)
  }
</script>

{#if current}
  <Modal titleText={$t('keyboardInteractivePrompt.title')} dismissable={false} width={420}>
    <form class="form" on:submit|preventDefault={submit}>
      <p class="body">
        {current.instruction || $t('keyboardInteractivePrompt.body', { user: current.user, hostname: current.hostname })}
      </p>

      {#each current.questions as question, index (`${current.requestId}-${index}`)}
        <label>
          <span>{question.prompt || $t('keyboardInteractivePrompt.question', { number: index + 1 })}</span>
          {#if question.echo}
            <input type="text" bind:value={answers[index]} autocomplete="off" />
          {:else}
            <input type="password" bind:value={answers[index]} autocomplete="off" />
          {/if}
        </label>
      {/each}
    </form>

    <svelte:fragment slot="footer">
      <button on:click={cancel}>{$t('keyboardInteractivePrompt.cancel')}</button>
      <button class="primary" on:click={submit}>{$t('keyboardInteractivePrompt.submit')}</button>
    </svelte:fragment>
  </Modal>
{/if}

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .body {
    margin: 0;
    color: var(--text-color-secondary);
    font-size: 12.5px;
    white-space: pre-wrap;
  }

  label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 12px;
  }
</style>
