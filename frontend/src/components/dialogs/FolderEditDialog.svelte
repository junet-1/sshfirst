<script lang="ts">
  import Icon from '../Icon.svelte'
  import Modal from './Modal.svelte'
  import { t } from '../../services/i18n'
  import { createFolder, folders, updateFolder } from '../../stores/hosts'
  import { folderDialog } from '../../stores/ui'

  const iconOptions = ['folder', 'server', 'terminal', 'cloud', 'database', 'globe', 'code', 'home', 'shield', 'archive']

  let name = ''
  let icon = 'folder'
  let saving = false
  let error = ''
  let loadedKey = ''

  $: dialogKey = $folderDialog.open ? String($folderDialog.editingId ?? 'new') : ''
  $: if (dialogKey && dialogKey !== loadedKey) {
    loadedKey = dialogKey
    const folder = $folderDialog.editingId == null
      ? null
      : $folders.find((item) => item.id === $folderDialog.editingId) ?? null
    name = folder?.name ?? ''
    icon = folder?.icon || 'folder'
    error = ''
    saving = false
  }
  $: if (!dialogKey) loadedKey = ''

  function close(): void {
    if (saving) return
    folderDialog.set({ open: false, editingId: null, parentId: null })
  }

  async function save(): Promise<void> {
    const trimmed = name.trim()
    if (!trimmed || saving) {
      if (!trimmed) error = $t('folderDialog.validation.name')
      return
    }
    saving = true
    error = ''
    try {
      if ($folderDialog.editingId == null) {
        await createFolder(trimmed, $folderDialog.parentId, icon)
      } else {
        await updateFolder($folderDialog.editingId, trimmed, icon)
      }
      folderDialog.set({ open: false, editingId: null, parentId: null })
    } catch (caught) {
      error = caught instanceof Error ? caught.message : String(caught)
    } finally {
      saving = false
    }
  }
</script>

{#if $folderDialog.open}
  <Modal
    titleText={$folderDialog.editingId == null ? $t('folderDialog.titleNew') : $t('folderDialog.titleEdit')}
    width={420}
    on:close={close}
  >
    <form id="folder-editor-form" class="form" on:submit|preventDefault={save}>
      <label class="form-row">
        <span>{$t('folderDialog.name')}</span>
        <input type="text" bind:value={name} required />
      </label>

      <div class="form-row align-start">
        <span>{$t('folderDialog.icon')}</span>
        <div class="icon-grid" role="group" aria-label={$t('folderDialog.icon')}>
          {#each iconOptions as option (option)}
            <button
              type="button"
              class:selected={icon === option}
              title={$t(`folderDialog.icon.${option}`)}
              aria-label={$t(`folderDialog.icon.${option}`)}
              aria-pressed={icon === option}
              on:click={() => (icon = option)}
            >
              <Icon name={option} size={18} />
            </button>
          {/each}
        </div>
      </div>

      {#if error}<p class="error" role="alert">{error}</p>{/if}
    </form>

    <svelte:fragment slot="footer">
      <button type="button" disabled={saving} on:click={close}>{$t('folderDialog.cancel')}</button>
      <button type="submit" form="folder-editor-form" class="primary" disabled={saving}>
        {saving
          ? $t('folderDialog.saving')
          : $folderDialog.editingId == null
            ? $t('folderDialog.create')
            : $t('folderDialog.save')}
      </button>
    </svelte:fragment>
  </Modal>
{/if}

<style>
  .form { display: flex; flex-direction: column; gap: 12px; }
  .form-row { display: grid; grid-template-columns: 92px minmax(0, 1fr); align-items: center; gap: 10px; font-size: 12px; }
  .form-row > span { color: var(--text-color-secondary); }
  .align-start { align-items: start; }
  .align-start > span { padding-top: 7px; }
  input { width: 100%; }
  .icon-grid { display: grid; grid-template-columns: repeat(5, 34px); gap: 5px; }
  .icon-grid button { display: flex; align-items: center; justify-content: center; width: 34px; height: 32px; padding: 0; color: var(--text-color-secondary); background: var(--view-bg); }
  .icon-grid button:hover { color: var(--text-color); }
  .icon-grid button.selected { color: var(--accent-color); background: var(--active-bg); border-color: var(--accent-color); box-shadow: inset 0 0 0 1px var(--accent-color); }
  .error { margin: 0; padding: 6px 8px; color: var(--error-color); border: 1px solid var(--error-color); border-radius: 3px; font-size: 11.5px; }
  @media (max-width: 390px) {
    .form-row { grid-template-columns: 1fr; gap: 4px; }
    .align-start > span { padding-top: 0; }
  }
</style>
