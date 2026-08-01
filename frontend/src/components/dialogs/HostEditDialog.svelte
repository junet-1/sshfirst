<script lang="ts">
  import Modal from './Modal.svelte'
  import { t } from '../../services/i18n'
  import { backend } from '../../services/backend'
  import { createHost, folders, hosts, updateHost } from '../../stores/hosts'
  import { loadSnippets, snippets } from '../../stores/snippets'
  import { hostDialog } from '../../stores/ui'
  import { credentials } from '../../stores/credentials'
  import { normalizePanelUrl } from '../../stores/connections'
  import { emptyHostInput, hostToInput, type HostInput } from '../../types/host'

  let form: HostInput = emptyHostInput()
  let tagsText = ''
  let saving = false
  let error: string | null = null
  let loadedDialogKey = ''

  // A web host is addressed by its control-panel URL; the SSH/SFTP fields
  // (hostname, auth, forwarding, startup script) don't apply and are hidden.
  $: isWeb = form.protocol === 'web'
  // When a shared credential is referenced, user/auth/identity come from it, so
  // the host's own inline versions of those fields are hidden.
  $: usesCredential = form.credentialId != null

  $: dialogKey = $hostDialog.open ? String($hostDialog.editingId ?? 'new') : ''
  $: if (dialogKey && dialogKey !== loadedDialogKey) {
    loadedDialogKey = dialogKey
    resetForm($hostDialog.editingId)
    void loadSnippets($hostDialog.editingId ?? 0)
  }
  $: if (!dialogKey) loadedDialogKey = ''

  function resetForm(editingId: number | null): void {
    const editing = editingId != null ? $hosts.find((host) => host.id === editingId) : null
    const next = editing ? hostToInput(editing) : emptyHostInput()
    form = next
    tagsText = next.tags.join(', ')
    error = null
    saving = false
  }

  function close(): void {
    if (saving) return
    hostDialog.set({ open: false, editingId: null })
  }

  async function addIdentityFiles(): Promise<void> {
    const files = await backend.pickIdentityFiles()
    form.identityFiles = [...form.identityFiles, ...files.filter((file) => !form.identityFiles.includes(file))]
  }

  function removeIdentityFile(path: string): void {
    form.identityFiles = form.identityFiles.filter((file) => file !== path)
  }

  function insertSnippetIntoLoginScript(id: number): void {
    const snippet = $snippets.find((item) => item.id === id)
    if (!snippet) return
    const separator = form.loginScript && !form.loginScript.endsWith('\n') ? '\n' : ''
    form.loginScript = form.loginScript + separator + snippet.command
  }

  function validationError(): string | null {
    if (!form.label.trim()) return $t('hostDialog.validation.label')
    if (form.protocol === 'web') {
      if (!normalizePanelUrl(form.controlPanelUrl)) return $t('hostDialog.validation.url')
      return null
    }
    if (!form.hostname.trim()) return $t('hostDialog.validation.hostname')
    if (!Number.isInteger(Number(form.port)) || Number(form.port) < 1 || Number(form.port) > 65535) {
      return $t('hostDialog.validation.port')
    }
    // User, auth method and identity files come from the referenced credential;
    // only validate the host's own inline versions when none is selected.
    if (!usesCredential) {
      if (!form.user.trim()) return $t('hostDialog.validation.user')
      if (form.authMethod === 'identity' && form.identityFiles.length === 0) {
        return $t('hostDialog.validation.identity')
      }
    }
    return null
  }

  async function save(): Promise<void> {
    if (saving) return
    error = validationError()
    if (error) return

    saving = true
    try {
      const input: HostInput = {
        ...form,
        label: form.label.trim(),
        hostname: form.protocol === 'web' ? '' : form.hostname.trim(),
        user: form.protocol === 'web' ? '' : form.user.trim(),
        identityFiles: form.protocol !== 'web' && form.authMethod === 'identity' ? [...form.identityFiles] : [],
        controlPanelUrl: normalizePanelUrl(form.controlPanelUrl),
        remotePath: form.protocol === 'sftp' ? form.remotePath.trim() || '.' : '.',
        forwardAgent: form.protocol === 'ssh' ? form.forwardAgent : false,
        loginScript: form.protocol === 'ssh' ? form.loginScript : '',
        tags: tagsText
          .split(',')
          .map((tag) => tag.trim())
          .filter(Boolean)
      }
      if ($hostDialog.editingId != null) await updateHost($hostDialog.editingId, input)
      else await createHost(input)
      hostDialog.set({ open: false, editingId: null })
    } catch (caught) {
      error = caught instanceof Error ? caught.message : String(caught)
    } finally {
      saving = false
    }
  }
</script>

{#if $hostDialog.open}
  <Modal
    titleText={$hostDialog.editingId != null ? $t('hostDialog.titleEdit') : $t('hostDialog.titleNew')}
    width={520}
    on:close={close}
  >
    <form id="host-editor-form" class="form" on:submit|preventDefault={save}>
      <fieldset>
        <legend>{$t('hostDialog.section.connection')}</legend>

        <label class="form-row">
          <span>{$t('hostDialog.protocol')}</span>
          <select bind:value={form.protocol}>
            <option value="ssh">{$t('hostDialog.protocol.ssh')}</option>
            <option value="sftp">{$t('hostDialog.protocol.sftp')}</option>
            <option value="web">{$t('hostDialog.protocol.web')}</option>
          </select>
        </label>

        <label class="form-row">
          <span>{$t('hostDialog.label')}</span>
          <input type="text" bind:value={form.label} required />
        </label>

        {#if isWeb}
          <label class="form-row">
            <span>{$t('hostDialog.url')}</span>
            <span class="control-stack">
              <input
                type="text"
                bind:value={form.controlPanelUrl}
                placeholder="https://panel.example.com"
                spellcheck="false"
                required
              />
              <small>{$t('hostDialog.url.help')}</small>
            </span>
          </label>
        {:else}
          <label class="form-row">
            <span>{$t('hostDialog.hostname')}</span>
            <input type="text" bind:value={form.hostname} spellcheck="false" required />
          </label>

          <label class="form-row">
            <span>{$t('hostDialog.port')}</span>
            <input class="short-control" type="number" min="1" max="65535" bind:value={form.port} />
          </label>

          {#if !usesCredential}
            <label class="form-row">
              <span>{$t('hostDialog.user')}</span>
              <input type="text" bind:value={form.user} spellcheck="false" required />
            </label>
          {/if}
        {/if}

        {#if form.protocol === 'sftp'}
          <label class="form-row">
            <span>{$t('hostDialog.remotePath')}</span>
            <span class="control-stack">
              <input class="mono-input" type="text" bind:value={form.remotePath} spellcheck="false" placeholder="." />
              <small>{$t('hostDialog.remotePath.help')}</small>
            </span>
          </label>
        {/if}
      </fieldset>

      {#if !isWeb}
      <fieldset>
        <legend>{$t('hostDialog.section.auth')}</legend>

        <label class="form-row">
          <span>{$t('hostDialog.credential')}</span>
          <span class="control-stack">
            <span class="credential-row">
              <select bind:value={form.credentialId}>
                <option value={null}>{$t('hostDialog.credential.none')}</option>
                {#each $credentials as credential (credential.id)}
                  <option value={credential.id}>{credential.name}</option>
                {/each}
              </select>
              <button type="button" class="fit-button" on:click={() => backend.openToolWindow('credentials')}>
                {$t('hostDialog.credential.manage')}
              </button>
            </span>
            {#if usesCredential}<small>{$t('hostDialog.credential.inheritNote')}</small>{/if}
          </span>
        </label>

        {#if !usesCredential}
        <label class="form-row">
          <span>{$t('hostDialog.authMethod')}</span>
          <select bind:value={form.authMethod}>
            <option value="agent">{$t('hostDialog.authMethod.agent')}</option>
            <option value="identity">{$t('hostDialog.authMethod.identity')}</option>
            <option value="password">{$t('hostDialog.authMethod.password')}</option>
          </select>
        </label>

        {#if form.authMethod === 'identity'}
          <div class="form-row align-start">
            <span class="row-label">{$t('hostDialog.identityFiles')}</span>
            <div class="control-stack">
              {#if form.identityFiles.length}
                <ul class="file-list">
                  {#each form.identityFiles as file (file)}
                    <li>
                      <span class="mono file-path" title={file}>{file}</span>
                      <button
                        type="button"
                        class="remove"
                        title={$t('hostDialog.identityFiles.remove')}
                        aria-label={$t('hostDialog.identityFiles.remove')}
                        on:click={() => removeIdentityFile(file)}
                      >×</button>
                    </li>
                  {/each}
                </ul>
              {/if}
              <button type="button" class="fit-button" on:click={addIdentityFiles}>
                {$t('hostDialog.identityFiles.add')}
              </button>
              <small>{$t('hostDialog.identityFiles.help')}</small>
            </div>
          </div>
        {:else if form.authMethod === 'password'}
          <div class="form-row">
            <span></span>
            <p class="hint">{$t('hostDialog.passwordNote')}</p>
          </div>
        {:else}
          <div class="form-row">
            <span></span>
            <p class="hint">{$t('hostDialog.agentNote')}</p>
          </div>
        {/if}
        {/if}

        {#if form.protocol === 'ssh'}
          <label class="form-row checkbox-row">
            <span>{$t('hostDialog.forwardAgent')}</span>
            <span class="checkbox-control">
              <input type="checkbox" bind:checked={form.forwardAgent} />
              <small>{$t('hostDialog.forwardAgent.help')}</small>
            </span>
          </label>
        {/if}
      </fieldset>
      {/if}

      <fieldset>
        <legend>{$t('hostDialog.section.organization')}</legend>

        <label class="form-row">
          <span>{$t('hostDialog.folder')}</span>
          <select bind:value={form.folderId}>
            <option value={null}>{$t('hostDialog.folder.none')}</option>
            {#each $folders as folder (folder.id)}
              <option value={folder.id}>{folder.name}</option>
            {/each}
          </select>
        </label>

        <label class="form-row">
          <span>{$t('hostDialog.tags')}</span>
          <input type="text" bind:value={tagsText} placeholder={$t('hostDialog.tags.placeholder')} />
        </label>
      </fieldset>

      <fieldset>
        <legend>{$t('hostDialog.advanced')}</legend>

        {#if !isWeb}
          <label class="form-row">
            <span>{$t('hostDialog.proxyJump')}</span>
            <span class="control-stack">
              <input type="text" bind:value={form.proxyJump} placeholder="user@bastion.example.com" spellcheck="false" />
              <small>{$t('hostDialog.proxyJump.help')}</small>
            </span>
          </label>

          <label class="form-row">
            <span>{$t('hostDialog.controlPanelUrl')}</span>
            <span class="control-stack">
              <input
                type="text"
                bind:value={form.controlPanelUrl}
                placeholder="https://panel.example.com"
                spellcheck="false"
              />
              <small>{$t('hostDialog.controlPanelUrl.help')}</small>
            </span>
          </label>
        {/if}

        {#if form.protocol === 'ssh'}
          <div class="form-row align-start">
            <span class="row-label">{$t('hostDialog.loginScript')}</span>
            <div class="control-stack">
              {#if $snippets.length}
                <select
                  class="snippet-picker"
                  aria-label={$t('hostDialog.insertSnippet')}
                  on:change={(event) => {
                    const id = Number(event.currentTarget.value)
                    if (id) insertSnippetIntoLoginScript(id)
                    event.currentTarget.value = ''
                  }}
                >
                  <option value="">{$t('hostDialog.insertSnippet')}</option>
                  {#each $snippets as snippet (snippet.id)}
                    <option value={snippet.id}>{snippet.name}</option>
                  {/each}
                </select>
              {/if}
              <textarea rows="3" bind:value={form.loginScript} placeholder="tmux attach || tmux new" class="mono-input" />
            </div>
          </div>
        {/if}

        <label class="form-row align-start">
          <span>{$t('hostDialog.notes')}</span>
          <textarea rows="3" bind:value={form.notes} />
        </label>
      </fieldset>

      {#if error}
        <p class="error" role="alert">{error}</p>
      {/if}
    </form>

    <svelte:fragment slot="footer">
      <button type="button" disabled={saving} on:click={close}>{$t('hostDialog.cancel')}</button>
      <button type="submit" form="host-editor-form" class="primary" disabled={saving}>
        {saving ? $t('hostDialog.saving') : $t('hostDialog.save')}
      </button>
    </svelte:fragment>
  </Modal>
{/if}

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: 10px;
    width: 100%;
    min-width: 0;
  }

  fieldset {
    min-width: 0;
    margin: 0;
    padding: 9px 10px 10px;
    border: 1px solid var(--border-color);
    border-radius: 3px;
  }

  legend {
    padding: 0 5px;
    color: var(--text-color);
    font-size: 12px;
    font-weight: 600;
  }

  .form-row {
    display: grid;
    grid-template-columns: 125px minmax(0, 1fr);
    align-items: center;
    gap: 8px;
    min-width: 0;
    margin-top: 7px;
    font-size: 12px;
  }

  fieldset > .form-row:first-of-type {
    margin-top: 2px;
  }

  .form-row > span:first-child,
  .row-label {
    color: var(--text-color-secondary);
  }

  .align-start {
    align-items: start;
  }

  .align-start > .row-label,
  .align-start > span:first-child {
    padding-top: 4px;
  }

  input:not([type='checkbox']),
  select,
  textarea {
    width: 100%;
    min-width: 0;
  }

  textarea {
    resize: vertical;
  }

  .short-control {
    width: 96px !important;
  }

  .control-stack {
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: 4px;
    min-width: 0;
  }

  .control-stack small,
  .checkbox-control small {
    color: var(--text-color-secondary);
    font-size: 10.5px;
    line-height: 1.35;
  }

  .checkbox-control {
    display: flex;
    align-items: flex-start;
    gap: 6px;
    min-width: 0;
  }

  .checkbox-control input {
    flex-shrink: 0;
    margin: 2px 0 0;
  }

  .hint {
    margin: 0;
    color: var(--text-color-secondary);
    font-size: 10.5px;
    line-height: 1.4;
  }

  .fit-button,
  .snippet-picker {
    align-self: flex-start;
    width: auto;
  }

  .credential-row {
    display: flex;
    gap: 6px;
    align-items: center;
    min-width: 0;
  }

  .credential-row select {
    flex: 1;
    min-width: 0;
  }

  .credential-row .fit-button {
    flex-shrink: 0;
    align-self: auto;
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

  .mono,
  .mono-input {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
  }

  .error {
    margin: 0;
    padding: 6px 8px;
    color: var(--error-color);
    background: var(--view-bg-alt);
    border: 1px solid var(--error-color);
    border-radius: 3px;
    font-size: 11.5px;
  }

  @media (max-width: 500px) {
    .form-row {
      grid-template-columns: 1fr;
      gap: 3px;
    }

    .form-row > span:empty {
      display: none;
    }

    .align-start > .row-label,
    .align-start > span:first-child {
      padding-top: 0;
    }

    .short-control {
      width: 100% !important;
    }
  }
</style>
