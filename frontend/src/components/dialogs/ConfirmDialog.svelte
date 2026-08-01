<script lang="ts">
  import Modal from './Modal.svelte'
  import { confirmRequest } from '../../stores/confirm'

  function respond(ok: boolean): void {
    const req = $confirmRequest
    confirmRequest.set(null)
    req?.resolve(ok)
  }
</script>

{#if $confirmRequest}
  <Modal titleText={$confirmRequest.title} width={380} on:close={() => respond(false)}>
    <p class="message">{$confirmRequest.message}</p>

    <svelte:fragment slot="footer">
      <button on:click={() => respond(false)}>{$confirmRequest.cancelLabel}</button>
      <button class="primary" on:click={() => respond(true)}>{$confirmRequest.confirmLabel}</button>
    </svelte:fragment>
  </Modal>
{/if}

<style>
  .message {
    margin: 0;
    font-size: 13px;
    line-height: 1.5;
    color: var(--text-color);
    white-space: pre-line;
  }
</style>
