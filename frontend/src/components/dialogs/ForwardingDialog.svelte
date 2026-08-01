<script lang="ts">
  import Modal from './Modal.svelte'
  import Icon from '../Icon.svelte'
  import { t } from '../../services/i18n'
  import { connections } from '../../stores/connections'
  import { notify } from '../../stores/notifications'
  import {
    activeForwards,
    createForwardRule,
    deleteForwardRule,
    forwardingDialogHostId,
    forwardRules,
    loadActiveForwards,
    loadForwardRules,
    startForward,
    stopForward,
    updateForwardRule
  } from '../../stores/forwarding'
  import type { ForwardKind, ForwardRule } from '../../types/forwarding'

  export let connectionId: string | null = null

  $: hostId = $forwardingDialogHostId
  $: rules = hostId != null ? ($forwardRules[hostId] ?? []) : []

  // A connected session to this host lets rules be started/stopped live.
  $: activeConnection =
    connectionId
      ? { connectionId }
      : hostId != null
      ? Object.values($connections).find((c) => c.hostId === hostId && c.status === 'connected') ?? null
      : null
  $: liveForwards = activeConnection ? ($activeForwards[activeConnection.connectionId] ?? {}) : {}

  let loadedForHost: number | null = null
  $: if (hostId != null && hostId !== loadedForHost) {
    loadedForHost = hostId
    void loadForwardRules(hostId)
    resetForm()
  }
  $: if (activeConnection) void loadActiveForwards(activeConnection.connectionId)

  let editingId: number | null = null
  let formKind: ForwardKind = 'local'
  let formLabel = ''
  let formBindAddr = ''
  let formBindPort: number | null = null
  let formDestHost = ''
  let formDestPort: number | null = null
  let formAutoStart = false
  let error: string | null = null

  function resetForm(): void {
    editingId = null
    formKind = 'local'
    formLabel = ''
    formBindAddr = ''
    formBindPort = null
    formDestHost = ''
    formDestPort = null
    formAutoStart = false
    error = null
  }

  function startEdit(rule: ForwardRule): void {
    editingId = rule.id
    formKind = rule.kind
    formLabel = rule.label
    formBindAddr = rule.bindAddr
    formBindPort = rule.bindPort
    formDestHost = rule.destHost
    formDestPort = rule.kind === 'dynamic' ? null : rule.destPort
    formAutoStart = rule.autoStart
    error = null
  }

  async function save(): Promise<void> {
    if (hostId == null) return
    error = null
    if (!formBindPort || formBindPort < 1 || formBindPort > 65535) {
      error = $t('forwarding.error.bindPort')
      return
    }
    if (formKind !== 'dynamic') {
      if (!formDestHost.trim()) {
        error = $t('forwarding.error.destHost')
        return
      }
      if (!formDestPort || formDestPort < 1 || formDestPort > 65535) {
        error = $t('forwarding.error.destPort')
        return
      }
    }
    const input = {
      hostId,
      kind: formKind,
      label: formLabel.trim(),
      bindAddr: formBindAddr.trim(),
      bindPort: formBindPort,
      destHost: formKind === 'dynamic' ? '' : formDestHost.trim(),
      destPort: formKind === 'dynamic' ? 0 : formDestPort!,
      autoStart: formAutoStart
    }
    try {
      if (editingId != null) await updateForwardRule(editingId, input)
      else await createForwardRule(input)
      resetForm()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }

  async function remove(rule: ForwardRule): Promise<void> {
    if (hostId == null) return
    try {
      await deleteForwardRule(hostId, rule.id)
      if (editingId === rule.id) resetForm()
    } catch (e) {
      notify('error', e instanceof Error ? e.message : String(e))
    }
  }

  async function toggle(rule: ForwardRule): Promise<void> {
    if (!activeConnection) return
    try {
      if (liveForwards[rule.id]) await stopForward(activeConnection.connectionId, rule.id)
      else await startForward(activeConnection.connectionId, rule.id)
    } catch (e) {
      notify('error', e instanceof Error ? e.message : String(e))
    }
  }

  function flag(kind: ForwardKind): string {
    return kind === 'local' ? '-L' : kind === 'remote' ? '-R' : '-D'
  }

  function describe(rule: ForwardRule): string {
    const bind = `${rule.bindAddr || '127.0.0.1'}:${rule.bindPort}`
    if (rule.kind === 'dynamic') return `${bind} · SOCKS5`
    return `${bind} → ${rule.destHost}:${rule.destPort}`
  }

  function close(): void {
    forwardingDialogHostId.set(null)
    loadedForHost = null
    resetForm()
  }
</script>

{#if hostId != null}
  <Modal titleText={$t('forwarding.title')} on:close={close} width={520}>
    <div class="content">

      {#if rules.length === 0}
        <p class="empty">{$t('forwarding.empty')}</p>
      {:else}
        <ul class="list">
          {#each rules as rule (rule.id)}
            {@const live = liveForwards[rule.id]}
            {@const active = !!live}
            <li>
              <button
                class="toggle"
                class:active
                title={activeConnection ? (active ? $t('forwarding.stop') : $t('forwarding.start')) : $t('forwarding.notConnected')}
                disabled={!activeConnection}
                on:click={() => toggle(rule)}
              >
                <span class="dot" />
              </button>
              <div class="info">
                <span class="name">
                  <span class="flag">{flag(rule.kind)}</span>
                  {rule.label || describe(rule)}
                  {#if rule.autoStart}<span class="badge">auto</span>{/if}
                </span>
                <span class="detail mono">{describe(rule)}</span>
                {#if live?.boundAddr}
                  <span class="detail mono live">▸ {live.boundAddr}</span>
                {/if}
              </div>
              <div class="row-actions">
                <button class="icon" title={$t('forwarding.edit')} on:click={() => startEdit(rule)}>
                  <Icon name="settings" size={12} />
                </button>
                <button class="icon danger" title={$t('forwarding.delete')} on:click={() => remove(rule)}>
                  <Icon name="x" size={12} />
                </button>
              </div>
            </li>
          {/each}
        </ul>
      {/if}

      <form class="editor" on:submit|preventDefault={save}>
        <div class="field-row">
          <label>
            <span>{$t('forwarding.kind')}</span>
            <select bind:value={formKind}>
              <option value="local">{$t('forwarding.kind.local')}</option>
              <option value="remote">{$t('forwarding.kind.remote')}</option>
              <option value="dynamic">{$t('forwarding.kind.dynamic')}</option>
            </select>
          </label>
          <label class="grow">
            <span>{$t('forwarding.label')}</span>
            <input type="text" bind:value={formLabel} placeholder={$t('forwarding.label.placeholder')} />
          </label>
        </div>

        <div class="field-row">
          <label class="grow">
            <span>{$t('forwarding.bindAddr')}</span>
            <input type="text" bind:value={formBindAddr} placeholder="127.0.0.1" />
          </label>
          <label class="port">
            <span>{$t('forwarding.bindPort')}</span>
            <input type="number" min="1" max="65535" bind:value={formBindPort} />
          </label>
        </div>

        {#if formKind !== 'dynamic'}
          <div class="field-row">
            <label class="grow">
              <span>{$t('forwarding.destHost')}</span>
              <input type="text" bind:value={formDestHost} placeholder="db.internal" />
            </label>
            <label class="port">
              <span>{$t('forwarding.destPort')}</span>
              <input type="number" min="1" max="65535" bind:value={formDestPort} />
            </label>
          </div>
        {/if}

        <label class="checkbox">
          <input type="checkbox" bind:checked={formAutoStart} />
          <span>{$t('forwarding.autoStart')}</span>
        </label>

        {#if error}<p class="error">{error}</p>{/if}
      </form>
    </div>

    <svelte:fragment slot="footer">
      {#if editingId != null}
        <button on:click={resetForm}>{$t('forwarding.cancelEdit')}</button>
      {/if}
      <button on:click={close}>{$t('forwarding.close')}</button>
      <button class="primary" on:click={save}>
        {editingId != null ? $t('forwarding.save') : $t('forwarding.add')}
      </button>
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

  .toggle {
    width: 22px;
    height: 22px;
    flex: 0 0 auto;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 3px;
  }

  .toggle:disabled {
    cursor: default;
    opacity: 0.6;
  }

  .toggle .dot {
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: var(--disabled-text-color);
  }

  .toggle.active .dot {
    background: var(--success-color);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--success-color) 25%, transparent);
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

  .flag {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
    color: var(--accent-color, var(--text-color-secondary));
    font-weight: 600;
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

  .detail {
    font-size: 11px;
    color: var(--text-color-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .detail.live {
    color: var(--success-color);
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

  .port {
    width: 92px;
    flex: 0 0 auto;
  }

  .checkbox {
    flex-direction: row;
    align-items: center;
    gap: 6px;
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
