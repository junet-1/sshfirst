<script lang="ts">
  import Modal from './Modal.svelte'
  import Icon from '../Icon.svelte'
  import { t } from '../../services/i18n'
  import { backend } from '../../services/backend'
  import { credentialsOpen } from '../../stores/ui'
  import { createCredential, credentials, deleteCredential, loadCredentials, updateCredential } from '../../stores/credentials'
  import { confirmDialog } from '../../stores/confirm'
  import { notify } from '../../stores/notifications'
  import { emptyCredentialInput, type Credential, type CredentialInput } from '../../types/credential'

  let form: CredentialInput = emptyCredentialInput()
  let editingId: number | null = null
  let error: string | null = null

  $: if ($credentialsOpen) void loadCredentials()

  function resetForm(): void {
    form = emptyCredentialInput()
    editingId = null
    error = null
  }

  function startEdit(credential: Credential): void {
    editingId = credential.id
    form = {
      name: credential.name,
      user: credential.user,
      authMethod: credential.authMethod,
      identityFiles: [...credential.identityFiles]
    }
    error = null
  }

  async function addIdentityFiles(): Promise<void> {
    const files = await backend.pickIdentityFiles()
    form.identityFiles = [...form.identityFiles, ...files.filter((file) => !form.identityFiles.includes(file))]
  }

  function removeIdentityFile(path: string): void {
    form.identityFiles = form.identityFiles.filter((file) => file !== path)
  }

  async function save(): Promise<void> {
    error = null
    const name = form.name.trim()
    if (!name) {
      error = $t('credentials.validation.name')
      return
    }
    const input: CredentialInput = {
      name,
      user: form.user.trim(),
      authMethod: form.authMethod,
      identityFiles: form.authMethod === 'identity' ? [...form.identityFiles] : []
    }
    try {
      if (editingId != null) await updateCredential(editingId, input)
      else await createCredential(input)
      resetForm()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }

  async function remove(credential: Credential): Promise<void> {
    // Warn with the exact number of hosts that will revert to their own inline
    // login settings when this shared credential disappears.
    let inUse = 0
    try {
      inUse = (await backend.listHosts()).filter((host) => host.credentialId === credential.id).length
    } catch {
      // Counting is best-effort; fall through to a generic warning.
    }
    const confirmed = await confirmDialog(
      $t('credentials.deleteConfirm.title'),
      inUse > 0 ? $t('credentials.deleteConfirm.body', { name: credential.name, count: inUse }) : $t('credentials.deleteConfirm.bodyUnused', { name: credential.name }),
      $t('credentials.delete'),
      $t('credentials.cancel')
    )
    if (!confirmed) return
    try {
      await deleteCredential(credential.id)
      if (editingId === credential.id) resetForm()
    } catch (e) {
      notify('error', e instanceof Error ? e.message : String(e))
    }
  }

  function close(): void {
    credentialsOpen.set(false)
    resetForm()
  }

  function authLabel(method: string): string {
    return method === 'identity'
      ? $t('hostDialog.authMethod.identity')
      : method === 'password'
        ? $t('hostDialog.authMethod.password')
        : $t('hostDialog.authMethod.agent')
  }
</script>

{#if $credentialsOpen}
  <Modal titleText={$t('credentials.title')} on:close={close} width={500}>
    <div class="content">
      {#if $credentials.length === 0}
        <p class="empty">{$t('credentials.empty')}</p>
      {:else}
        <ul class="list">
          {#each $credentials as credential (credential.id)}
            <li class:active={editingId === credential.id}>
              <div class="info">
                <span class="name">{credential.name}</span>
                <span class="detail">
                  {credential.user ? `${credential.user} · ` : ''}{authLabel(credential.authMethod)}
                </span>
              </div>
              <div class="row-actions">
                <button class="icon" title={$t('credentials.edit')} on:click={() => startEdit(credential)}>
                  <Icon name="settings" size={12} />
                </button>
                <button class="icon danger" title={$t('credentials.delete')} on:click={() => remove(credential)}>
                  <Icon name="x" size={12} />
                </button>
              </div>
            </li>
          {/each}
        </ul>
      {/if}

      <form class="editor" on:submit|preventDefault={save}>
        <p class="editor-title">{editingId != null ? $t('credentials.editTitle') : $t('credentials.newTitle')}</p>

        <div class="field-row">
          <label class="grow">
            <span>{$t('credentials.name')}</span>
            <input type="text" bind:value={form.name} placeholder={$t('credentials.name.placeholder')} />
          </label>
          <label class="grow">
            <span>{$t('credentials.user')}</span>
            <input type="text" bind:value={form.user} spellcheck="false" />
          </label>
        </div>

        <label>
          <span>{$t('credentials.authMethod')}</span>
          <select bind:value={form.authMethod}>
            <option value="agent">{$t('hostDialog.authMethod.agent')}</option>
            <option value="identity">{$t('hostDialog.authMethod.identity')}</option>
            <option value="password">{$t('hostDialog.authMethod.password')}</option>
          </select>
        </label>

        {#if form.authMethod === 'identity'}
          <div class="identity">
            {#if form.identityFiles.length}
              <ul class="file-list">
                {#each form.identityFiles as file (file)}
                  <li>
                    <span class="mono file-path" title={file}>{file}</span>
                    <button type="button" class="remove" on:click={() => removeIdentityFile(file)} aria-label={$t('hostDialog.identityFiles.remove')}>×</button>
                  </li>
                {/each}
              </ul>
            {/if}
            <button type="button" class="fit-button" on:click={addIdentityFiles}>{$t('hostDialog.identityFiles.add')}</button>
          </div>
        {:else if form.authMethod === 'password'}
          <p class="hint">{$t('credentials.passwordNote')}</p>
        {:else}
          <p class="hint">{$t('hostDialog.agentNote')}</p>
        {/if}

        {#if error}<p class="error">{error}</p>{/if}
      </form>
    </div>

    <svelte:fragment slot="footer">
      {#if editingId != null}
        <button on:click={resetForm}>{$t('credentials.newButton')}</button>
      {/if}
      <button on:click={close}>{$t('credentials.close')}</button>
      <button class="primary" on:click={save}>{editingId != null ? $t('credentials.save') : $t('credentials.add')}</button>
    </svelte:fragment>
  </Modal>
{/if}

<style>
  .content {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .empty {
    margin: 0;
    color: var(--text-color-secondary);
    font-size: 12px;
    line-height: 1.4;
  }

  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
    max-height: 34vh;
    overflow-y: auto;
  }

  .list li {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 6px;
    border-radius: 3px;
  }

  .list li:hover {
    background: var(--hover-bg);
  }

  .list li.active {
    background: var(--highlight-bg);
  }

  .info {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
    flex: 1;
  }

  .name {
    font-size: 12.5px;
  }

  .detail {
    font-size: 11px;
    color: var(--text-color-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .row-actions {
    display: flex;
    gap: 2px;
    flex-shrink: 0;
  }

  .icon {
    width: 24px;
    height: 22px;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 3px;
    color: var(--text-color-secondary);
  }

  .icon:hover {
    background: var(--hover-bg);
    border-color: var(--border-color);
    color: var(--text-color);
  }

  .icon.danger:hover {
    color: var(--error-color);
  }

  .editor {
    display: flex;
    flex-direction: column;
    gap: 8px;
    border-top: 1px solid var(--separator-color);
    padding-top: 10px;
  }

  .editor-title {
    margin: 0;
    font-size: 12px;
    font-weight: 600;
    color: var(--text-color);
  }

  .field-row {
    display: flex;
    gap: 10px;
    align-items: flex-end;
  }

  label {
    display: flex;
    flex-direction: column;
    gap: 3px;
    font-size: 12px;
  }

  label > span {
    color: var(--text-color-secondary);
  }

  .grow {
    flex: 1;
    min-width: 0;
  }

  input,
  select {
    width: 100%;
    min-width: 0;
  }

  .identity {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .file-list {
    display: flex;
    flex-direction: column;
    gap: 3px;
    max-height: 88px;
    overflow-y: auto;
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .file-list li {
    display: flex;
    align-items: center;
    gap: 5px;
    min-width: 0;
    padding: 2px 4px 2px 6px;
    background: var(--view-bg-alt);
    border: 1px solid var(--separator-color);
    border-radius: 3px;
  }

  .file-path {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .remove {
    flex-shrink: 0;
    padding: 0 5px;
    color: var(--error-color);
    background: transparent;
    border: none;
  }

  .fit-button {
    align-self: flex-start;
    width: auto;
  }

  .mono {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
  }

  .hint {
    margin: 0;
    color: var(--text-color-secondary);
    font-size: 10.5px;
    line-height: 1.4;
  }

  .error {
    color: var(--error-color);
    font-size: 12px;
    margin: 0;
  }
</style>
