<script lang="ts">
  import Modal from './Modal.svelte'
  import { t } from '../../services/i18n'
  import { backend } from '../../services/backend'
  import { hosts } from '../../stores/hosts'
  import { notify } from '../../stores/notifications'
  import {
    activeTransferId,
    cancelTransfer,
    startTransfer,
    transferDialogHostId,
    transfers
  } from '../../stores/transfer'

  let localPath = ''
  let remotePath = ''
  let upload = true
  let archive = true
  let compress = true
  let deleteExtraneous = false
  let dryRun = false
  let preview = ''
  let lastHostId: number | null = null

  $: host = $transferDialogHostId != null ? $hosts.find((h) => h.id === $transferDialogHostId) ?? null : null
  $: current = $activeTransferId ? $transfers[$activeTransferId] ?? null : null
  $: running = current?.state === 'running'

  // Reset the form only when the dialog opens for a (different) host.
  $: if ($transferDialogHostId != null && $transferDialogHostId !== lastHostId) {
    lastHostId = $transferDialogHostId
    localPath = ''
    remotePath = ''
    upload = true
    archive = true
    compress = true
    deleteExtraneous = false
    dryRun = false
    preview = ''
  }
  $: if ($transferDialogHostId == null) lastHostId = null

  function request() {
    return {
      hostId: $transferDialogHostId as number,
      localPath,
      remotePath,
      upload,
      archive,
      compress,
      delete: deleteExtraneous,
      dryRun
    }
  }

  // Live preview of the exact rsync command.
  $: void updatePreview(localPath, remotePath, upload, archive, compress, deleteExtraneous, dryRun, $transferDialogHostId)
  async function updatePreview(..._deps: unknown[]): Promise<void> {
    if ($transferDialogHostId == null || !localPath || !remotePath) {
      preview = ''
      return
    }
    try {
      preview = await backend.previewRsyncCommand(request())
    } catch {
      preview = ''
    }
  }

  async function browseFolder(): Promise<void> {
    const p = await backend.pickDirectory()
    if (p) localPath = p
  }

  async function browseFile(): Promise<void> {
    const p = await backend.pickFile()
    if (p) localPath = p
  }

  async function run(): Promise<void> {
    if (!host) return
    if (!localPath || !remotePath) {
      notify('error', 'Please set both a local and a remote path.')
      return
    }
    try {
      await startTransfer(request(), host.label)
    } catch (e) {
      notify('error', e instanceof Error ? e.message : String(e))
    }
  }

  async function cancel(): Promise<void> {
    if ($activeTransferId) await cancelTransfer($activeTransferId)
  }

  function close(): void {
    transferDialogHostId.set(null)
  }
</script>

{#if $transferDialogHostId != null && host}
  <Modal titleText={$t('transfer.title', { host: host.label })} on:close={close} width={560}>
    <div class="content">
      <p class="auth-note">{$t('transfer.authNote')}</p>

      <div class="direction">
        <button class="seg" class:active={upload} on:click={() => (upload = true)}>{$t('transfer.upload')}</button>
        <button class="seg" class:active={!upload} on:click={() => (upload = false)}>{$t('transfer.download')}</button>
      </div>

      <div class="field">
        <span>{$t('transfer.localPath')}</span>
        <div class="path-row">
          <input type="text" bind:value={localPath} placeholder="/home/you/project/" class="mono" />
          <button on:click={browseFolder}>{$t('transfer.folder')}</button>
          <button on:click={browseFile}>{$t('transfer.file')}</button>
        </div>
      </div>

      <div class="field">
        <span>{$t('transfer.remotePath')} ({host.user ? host.user + '@' : ''}{host.hostname})</span>
        <input type="text" bind:value={remotePath} placeholder="/var/www/project/" class="mono" />
      </div>

      <div class="flags">
        <label><input type="checkbox" bind:checked={archive} /> {$t('transfer.archive')}</label>
        <label><input type="checkbox" bind:checked={compress} /> {$t('transfer.compress')}</label>
        <label><input type="checkbox" bind:checked={dryRun} /> {$t('transfer.dryRun')}</label>
        <label class="danger"><input type="checkbox" bind:checked={deleteExtraneous} /> {$t('transfer.delete')}</label>
      </div>

      {#if preview}
        <pre class="preview mono">{preview}</pre>
      {/if}

      {#if current}
        <div class="output mono" class:error={current.state === 'error'}>
          {#each current.lines as line}{line}{'\n'}{/each}{#if current.progress}<span class="progress">{current.progress}</span>{/if}
        </div>
        {#if current.state === 'done'}
          <p class="status ok">{$t('transfer.done')}</p>
        {:else if current.state === 'error'}
          <p class="status err">{current.error || $t('transfer.failed')}</p>
        {/if}
      {/if}
    </div>

    <svelte:fragment slot="footer">
      {#if running}
        <button on:click={cancel}>{$t('transfer.cancel')}</button>
      {:else}
        <button on:click={close}>{$t('transfer.close')}</button>
        <button class="primary" on:click={run}>{$t('transfer.start')}</button>
      {/if}
    </svelte:fragment>
  </Modal>
{/if}

<style>
  .content {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .auth-note {
    margin: 0;
    font-size: 11px;
    color: var(--text-color-secondary);
    line-height: 1.4;
  }

  .direction {
    display: flex;
    gap: 0;
    align-self: flex-start;
    border: 1px solid var(--border-color);
    border-radius: 3px;
    overflow: hidden;
  }

  .seg {
    border: none;
    border-radius: 0;
    padding: 4px 14px;
    background: var(--view-bg);
  }

  .seg.active {
    background: var(--accent-color);
    color: var(--accent-text-color);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 3px;
    font-size: 12px;
  }

  .field > span {
    color: var(--text-color-secondary);
  }

  .path-row {
    display: flex;
    gap: 6px;
  }

  .path-row input {
    flex: 1;
  }

  .mono {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11.5px;
  }

  .flags {
    display: flex;
    flex-wrap: wrap;
    gap: 10px 16px;
    font-size: 12px;
  }

  .flags label {
    display: flex;
    align-items: center;
    gap: 5px;
  }

  .flags .danger {
    color: var(--error-color);
  }

  .preview {
    margin: 0;
    padding: 6px 8px;
    background: var(--view-bg-alt);
    border: 1px solid var(--border-color);
    border-radius: 3px;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    color: var(--text-color-secondary);
    max-height: 70px;
    overflow-y: auto;
  }

  .output {
    background: var(--terminal-bg);
    color: var(--terminal-fg);
    border-radius: 3px;
    padding: 8px;
    height: 200px;
    overflow-y: auto;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font-size: 11.5px;
  }

  .output.error {
    outline: 1px solid var(--error-color);
  }

  .progress {
    color: var(--accent-color);
  }

  .status {
    margin: 0;
    font-size: 12px;
  }

  .status.ok {
    color: var(--success-color);
  }

  .status.err {
    color: var(--error-color);
    overflow-wrap: anywhere;
  }
</style>
