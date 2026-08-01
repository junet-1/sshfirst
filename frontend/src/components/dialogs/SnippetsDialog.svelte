<script lang="ts">
  import Modal from './Modal.svelte'
  import Icon from '../Icon.svelte'
  import { t } from '../../services/i18n'
  import { snippetsOpen } from '../../stores/ui'
  import { activeConnectionId, connections } from '../../stores/connections'
  import { createSnippet, deleteSnippet, loadSnippets, runSnippet, snippets, updateSnippet } from '../../stores/snippets'
  import { notify } from '../../stores/notifications'
  import type { Snippet } from '../../types/snippet'

  export let hostId: number | null = null

  // Host of the active connection, so a snippet can be scoped to it.
  $: activeHostId = hostId ?? ($activeConnectionId ? ($connections[$activeConnectionId]?.hostId ?? null) : null)

  let editingId: number | null = null
  let formName = ''
  let formCommand = ''
  let formScopeHost = false
  let error: string | null = null

  $: if ($snippetsOpen) {
    void loadSnippets(activeHostId ?? 0)
  }

  function resetForm(): void {
    editingId = null
    formName = ''
    formCommand = ''
    formScopeHost = false
    error = null
  }

  function startEdit(s: Snippet): void {
    editingId = s.id
    formName = s.name
    formCommand = s.command
    formScopeHost = s.hostId != null
    error = null
  }

  async function save(): Promise<void> {
    error = null
    const name = formName.trim()
    const command = formCommand.trim()
    if (!name || !command) {
      error = 'Name and command are required.'
      return
    }
    const hostId = formScopeHost ? activeHostId : null
    try {
      if (editingId != null) await updateSnippet(editingId, { name, command, hostId })
      else await createSnippet({ name, command, hostId })
      resetForm()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }

  async function remove(id: number): Promise<void> {
    try {
      await deleteSnippet(id)
      if (editingId === id) resetForm()
    } catch (e) {
      notify('error', e instanceof Error ? e.message : String(e))
    }
  }

  function close(): void {
    snippetsOpen.set(false)
    resetForm()
  }
</script>

{#if $snippetsOpen}
  <Modal titleText={$t('snippets.title')} on:close={close} width={480}>
    <div class="content">
      {#if $snippets.length === 0}
        <p class="empty">{$t('snippets.empty')}</p>
      {:else}
        <ul class="list">
          {#each $snippets as s (s.id)}
            <li>
              <div class="info">
                <span class="name">{s.name}{#if s.hostId != null}<span class="badge">host</span>{/if}</span>
                <span class="cmd mono">{s.command}</span>
              </div>
              <div class="row-actions">
                <button class="icon" title={$t('snippets.run')} on:click={() => runSnippet(s)}>
                  <Icon name="play" size={12} />
                </button>
                <button class="icon" title={$t('snippets.edit')} on:click={() => startEdit(s)}>
                  <Icon name="settings" size={12} />
                </button>
                <button class="icon danger" title={$t('snippets.delete')} on:click={() => remove(s.id)}>
                  <Icon name="x" size={12} />
                </button>
              </div>
            </li>
          {/each}
        </ul>
      {/if}

      <form class="editor" on:submit|preventDefault={save}>
        <div class="field-row">
          <label class="grow">
            <span>{$t('snippets.name')}</span>
            <input type="text" bind:value={formName} />
          </label>
          <label class="scope">
            <input type="checkbox" bind:checked={formScopeHost} disabled={activeHostId == null} />
            <span>{$t('snippets.scopeHost')}</span>
          </label>
        </div>
        <label>
          <span>{$t('snippets.command')}</span>
          <textarea rows="2" class="mono" bind:value={formCommand} placeholder="sudo systemctl status nginx" />
        </label>
        {#if error}<p class="error">{error}</p>{/if}
      </form>
    </div>

    <svelte:fragment slot="footer">
      <button on:click={close}>{$t('snippets.close')}</button>
      <button class="primary" on:click={save}>{editingId != null ? $t('snippets.save') : $t('snippets.add')}</button>
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
    max-height: 40vh;
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

  .info {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
    flex: 1;
  }

  .name {
    font-size: 12.5px;
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .badge {
    font-size: 9.5px;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    padding: 0 4px;
    border-radius: 3px;
    background: var(--view-bg-alt);
    border: 1px solid var(--border-color);
    color: var(--text-color-secondary);
  }

  .cmd {
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
  }

  .scope {
    flex-direction: row;
    align-items: center;
    gap: 6px;
    white-space: nowrap;
  }

  .mono {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11.5px;
  }

  .error {
    color: var(--error-color);
    font-size: 12px;
    margin: 0;
  }
</style>
