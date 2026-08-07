<script lang="ts">
  import Modal from './Modal.svelte'
  import Icon from '../Icon.svelte'
  import { t } from '../../services/i18n'
  import { tabs } from '../../stores/connections'
  import { confirmDialog } from '../../stores/confirm'
  import { notify } from '../../stores/notifications'
  import {
    activeWorkspaceName,
    deleteWorkspace,
    exportWorkspace,
    importWorkspace,
    loadAndRestoreWorkspace,
    refreshWorkspaces,
    saveCurrentWorkspace,
    workspaceDialogOpen,
    workspaceSummaries
  } from '../../stores/workspaces'

  let name = ''
  let selected = ''
  let busy = false
  let loaded = false
  let error = ''

  $: if ($workspaceDialogOpen && !loaded) {
    loaded = true
    selected = $activeWorkspaceName ?? ''
    name = selected
    void refresh().catch(showError)
  }
  $: if (!$workspaceDialogOpen) loaded = false

  function showError(caught: unknown): void {
    error = caught instanceof Error ? caught.message : String(caught)
  }

  async function refresh(): Promise<void> {
    await refreshWorkspaces()
    if (selected && !$workspaceSummaries.some((item) => item.name === selected)) selected = ''
  }

  function selectWorkspace(workspaceName: string): void {
    selected = workspaceName
    name = workspaceName
    error = ''
  }

  async function save(): Promise<void> {
    const workspaceName = name.trim()
    if (!workspaceName) {
      error = $t('workspaces.validation.name')
      return
    }
    busy = true
    error = ''
    try {
      await saveCurrentWorkspace(workspaceName)
      selected = workspaceName
      notify('info', $t('workspaces.saved', { name: workspaceName }))
    } catch (caught) {
      showError(caught)
    } finally {
      busy = false
    }
  }

  function reportWarnings(warnings: string[]): void {
    if (warnings.length === 0) return
    const detail = warnings.slice(0, 3).join(' · ')
    notify('info', $t('workspaces.restoredWithWarnings', { count: warnings.length, detail }))
  }

  async function restore(): Promise<void> {
    if (!selected) return
    if (Object.keys($tabs).length > 0) {
      const confirmed = await confirmDialog(
        $t('workspaces.replace.title'),
        $t('workspaces.replace.body'),
        $t('workspaces.replace.confirm'),
        $t('workspaces.cancel')
      )
      if (!confirmed) return
    }
    busy = true
    error = ''
    try {
      const warnings = await loadAndRestoreWorkspace(selected)
      reportWarnings(warnings)
      workspaceDialogOpen.set(false)
    } catch (caught) {
      showError(caught)
    } finally {
      busy = false
    }
  }

  async function importFile(): Promise<void> {
    busy = true
    error = ''
    try {
      const definition = await importWorkspace()
      if (definition) {
        selected = definition.name
        name = definition.name
        notify('info', $t('workspaces.imported', { name: definition.name }))
      }
    } catch (caught) {
      showError(caught)
    } finally {
      busy = false
    }
  }

  async function exportFile(): Promise<void> {
    if (!selected) return
    busy = true
    error = ''
    try {
      const path = await exportWorkspace(selected)
      if (path) notify('info', $t('workspaces.exported', { path }))
    } catch (caught) {
      showError(caught)
    } finally {
      busy = false
    }
  }

  async function remove(): Promise<void> {
    if (!selected) return
    const confirmed = await confirmDialog(
      $t('workspaces.delete.title'),
      $t('workspaces.delete.body', { name: selected }),
      $t('workspaces.delete.confirm'),
      $t('workspaces.cancel')
    )
    if (!confirmed) return
    busy = true
    error = ''
    try {
      await deleteWorkspace(selected)
      selected = ''
      name = ''
    } catch (caught) {
      showError(caught)
    } finally {
      busy = false
    }
  }
</script>

{#if $workspaceDialogOpen}
  <Modal titleText={$t('workspaces.title')} on:close={() => workspaceDialogOpen.set(false)} width={560}>
    <div class="workspace-grid">
      <section class="saved" aria-label={$t('workspaces.savedList')}>
        <div class="section-title">{$t('workspaces.savedList')}</div>
        {#if $workspaceSummaries.length === 0}
          <p class="empty">{$t('workspaces.empty')}</p>
        {:else}
          <div class="list" role="listbox" aria-label={$t('workspaces.savedList')}>
            {#each $workspaceSummaries as workspace (workspace.name)}
              <button
                class="workspace-row"
                class:selected={selected === workspace.name}
                role="option"
                aria-selected={selected === workspace.name}
                on:click={() => selectWorkspace(workspace.name)}
                on:dblclick={restore}
              >
                <Icon name="layers" size={13} />
                <span>{workspace.name}</span>
                {#if $activeWorkspaceName === workspace.name}<span class="active-badge">{$t('workspaces.active')}</span>{/if}
              </button>
            {/each}
          </div>
        {/if}
      </section>

      <section class="actions">
        <label>
          <span>{$t('workspaces.name')}</span>
          <input type="text" bind:value={name} maxlength="120" disabled={busy} placeholder={$t('workspaces.namePlaceholder')} />
        </label>
        <button class="primary full" disabled={busy || !name.trim()} on:click={save}>
          <Icon name="download" size={12} /> {$t('workspaces.saveCurrent')}
        </button>
        <p class="hint">{$t('workspaces.saveHint')}</p>

        <div class="file-actions">
          <button disabled={busy} on:click={importFile}><Icon name="upload" size={12} /> {$t('workspaces.import')}</button>
          <button disabled={busy || !selected} on:click={exportFile}><Icon name="download" size={12} /> {$t('workspaces.export')}</button>
        </div>
        <button class="danger-link" disabled={busy || !selected} on:click={remove}>{$t('workspaces.delete.action')}</button>
      </section>
    </div>

    {#if error}<p class="error" role="alert">{error}</p>{/if}

    <svelte:fragment slot="footer">
      <button disabled={busy} on:click={() => workspaceDialogOpen.set(false)}>{$t('workspaces.close')}</button>
      <button class="primary" disabled={busy || !selected} on:click={restore}>{$t('workspaces.restore')}</button>
    </svelte:fragment>
  </Modal>
{/if}

<style>
  .workspace-grid { display: grid; grid-template-columns: minmax(210px, 1fr) minmax(220px, 1fr); gap: 14px; min-height: 230px; }
  section { min-width: 0; }
  .section-title, label > span { display: block; margin-bottom: 5px; color: var(--text-color-secondary); font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: .04em; }
  .list { display: flex; flex-direction: column; gap: 2px; max-height: 250px; overflow-y: auto; border: 1px solid var(--border-color); border-radius: 4px; padding: 3px; background: var(--view-bg); }
  .workspace-row { display: flex; align-items: center; gap: 7px; width: 100%; min-height: 30px; padding: 4px 7px; text-align: left; background: transparent; border: 1px solid transparent; border-radius: 3px; color: var(--text-color); }
  .workspace-row:hover { background: var(--hover-bg); }
  .workspace-row.selected { background: var(--highlight-bg); border-color: var(--accent-color); color: var(--highlight-text); }
  .workspace-row > span:nth-child(2) { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .active-badge { margin-left: auto; padding: 1px 4px; border: 1px solid currentColor; border-radius: 3px; font-size: 9px; text-transform: uppercase; opacity: .8; }
  .actions { display: flex; flex-direction: column; gap: 8px; padding-left: 14px; border-left: 1px solid var(--separator-color); }
  label { display: block; }
  input { width: 100%; }
  button { display: inline-flex; align-items: center; justify-content: center; gap: 5px; }
  .full { width: 100%; }
  .hint, .empty { margin: 0; color: var(--text-color-secondary); font-size: 11px; line-height: 1.45; }
  .empty { padding: 18px 8px; text-align: center; border: 1px dashed var(--border-color); border-radius: 4px; }
  .file-actions { display: grid; grid-template-columns: 1fr 1fr; gap: 6px; margin-top: 8px; }
  .danger-link { align-self: flex-start; padding: 2px 0; background: transparent; border: 0; color: var(--error-color); font-size: 11px; }
  .danger-link:hover:not(:disabled) { text-decoration: underline; }
  .error { margin: 12px 0 0; padding: 7px 9px; border: 1px solid var(--error-color); border-radius: 3px; background: color-mix(in srgb, var(--error-color) 10%, transparent); color: var(--error-color); font-size: 11.5px; white-space: pre-wrap; }
  @media (max-width: 560px) { .workspace-grid { grid-template-columns: 1fr; } .actions { padding: 12px 0 0; border-left: 0; border-top: 1px solid var(--separator-color); } }
</style>
