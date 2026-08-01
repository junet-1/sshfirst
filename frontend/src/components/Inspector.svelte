<script lang="ts">
  import { onMount } from 'svelte'
  import { hosts } from '../stores/hosts'
  import { activeConnectionId, connections } from '../stores/connections'
  import { selectedHostId } from '../stores/ui'
  import {
    activeForwards,
    forwardingDialogHostId,
    forwardRules,
    loadActiveForwards,
    loadForwardRules,
    startForward,
    stopForward
  } from '../stores/forwarding'
  import { notify } from '../stores/notifications'
  import type { ForwardKind, ForwardRule } from '../types/forwarding'

  let now = Date.now()

  onMount(() => {
    const timer = window.setInterval(() => (now = Date.now()), 1_000)
    return () => window.clearInterval(timer)
  })

  $: activeConnection = $activeConnectionId ? $connections[$activeConnectionId] : null
  $: inspectedHostId = activeConnection?.hostId ?? $selectedHostId
  $: host = inspectedHostId != null ? $hosts.find((h) => h.id === inspectedHostId) ?? null : null
  $: route = host
    ? host.proxyJump
      ? `${host.proxyJump.split(',').map((hop) => hop.trim()).filter(Boolean).join(' → ')} → ${host.hostname}`
      : `Direct → ${host.hostname}`
    : activeConnection?.quickTarget
      ? `Direct → ${activeConnection.quickTarget}`
      : '—'
  $: uptime =
    activeConnection?.status === 'connected' && activeConnection.connectedAt != null
      ? formatDuration(now - activeConnection.connectedAt)
      : '—'

  function formatDuration(milliseconds: number): string {
    const totalSeconds = Math.max(0, Math.floor(milliseconds / 1_000))
    const days = Math.floor(totalSeconds / 86_400)
    const hours = Math.floor((totalSeconds % 86_400) / 3_600)
    const minutes = Math.floor((totalSeconds % 3_600) / 60)
    const seconds = totalSeconds % 60

    if (days > 0) return `${days}d ${hours}h ${minutes}m`
    if (hours > 0) return `${hours}h ${String(minutes).padStart(2, '0')}m ${String(seconds).padStart(2, '0')}s`
    return `${minutes}m ${String(seconds).padStart(2, '0')}s`
  }

  function authLabel(method: string): string {
    if (method === 'identity') return 'Identity file'
    if (method === 'password') return 'Password'
    if (method === 'agent') return 'SSH agent'
    return method || 'Unknown'
  }

  // Port forwarding: rules for the inspected host and, if it has a live
  // connection, which of them are currently running.
  $: forwardHostId = host && host.protocol !== 'sftp' ? host.id : null
  let loadedForHost: number | null = null
  $: if (forwardHostId != null && forwardHostId !== loadedForHost) {
    loadedForHost = forwardHostId
    void loadForwardRules(forwardHostId)
  }
  $: rules = forwardHostId != null ? ($forwardRules[forwardHostId] ?? []) : []
  $: forwardConnected =
    activeConnection && activeConnection.status === 'connected' && activeConnection.hostId === forwardHostId
      ? activeConnection
      : null
  $: liveForwards = forwardConnected ? ($activeForwards[forwardConnected.connectionId] ?? {}) : {}
  $: if (forwardConnected) void loadActiveForwards(forwardConnected.connectionId)

  function flag(kind: ForwardKind): string {
    return kind === 'local' ? '-L' : kind === 'remote' ? '-R' : '-D'
  }

  function describeForward(rule: ForwardRule): string {
    const bind = `${rule.bindAddr || '127.0.0.1'}:${rule.bindPort}`
    if (rule.kind === 'dynamic') return `${bind} · SOCKS5`
    return `${bind} → ${rule.destHost}:${rule.destPort}`
  }

  async function toggleForward(rule: ForwardRule): Promise<void> {
    if (!forwardConnected) return
    try {
      if (liveForwards[rule.id]) await stopForward(forwardConnected.connectionId, rule.id)
      else await startForward(forwardConnected.connectionId, rule.id)
    } catch (e) {
      notify('error', e instanceof Error ? e.message : String(e))
    }
  }
</script>

<div class="inspector">
  {#if !host && !activeConnection}
    <p class="empty">Select a host to see its details.</p>
  {:else}
    <section>
      <h3>{host?.label ?? activeConnection?.hostLabel}</h3>
      <dl>
        {#if host}
          {#if host.protocol === 'web'}
            <dt>URL</dt>
            <dd class="mono">{host.controlPanelUrl}</dd>
            <dt>Protocol</dt>
            <dd>{host.protocol.toUpperCase()}</dd>
          {:else}
            <dt>Hostname</dt>
            <dd>{host.hostname}</dd>
            <dt>Protocol</dt>
            <dd>{host.protocol.toUpperCase()}</dd>
            <dt>Port</dt>
            <dd>{host.port}</dd>
            <dt>User</dt>
            <dd>{host.user || '—'}</dd>
            {#if host.protocol === 'sftp'}
              <dt>Start folder</dt>
              <dd class="mono">{host.remotePath || '.'}</dd>
            {:else}
              <dt>Agent Forwarding</dt>
              <dd>{host.forwardAgent ? 'Yes' : 'No'}</dd>
            {/if}
          {/if}
          <dt>Source</dt>
          <dd>{host.source === 'ssh_config' ? '~/.ssh/config' : 'Manual'}</dd>
          {#if host.lastUsedAt}
            <dt>Last used</dt>
            <dd>{new Date(host.lastUsedAt).toLocaleString()}</dd>
          {/if}
        {:else if activeConnection?.quickTarget}
          <dt>Target</dt>
          <dd class="mono">{activeConnection.quickTarget}</dd>
          <dt>Source</dt>
          <dd>Quick Connect</dd>
        {/if}
      </dl>
    </section>

    {#if activeConnection}
      <section class="connection-section">
        <h4>Connection</h4>
        <dl>
          <dt>Status</dt>
          <dd class="connection-status {activeConnection.status}">
            <span class="status-dot" />
            <span>{activeConnection.status}</span>
          </dd>
          <dt>Uptime</dt>
          <dd class="mono tabular">{uptime}</dd>
          <dt>Latency</dt>
          <dd class="tabular">
            {activeConnection.latencyMs != null ? `${activeConnection.latencyMs} ms` : 'Measuring…'}
          </dd>
          <dt>Server</dt>
          <dd class="mono">{activeConnection.serverVersion || 'Negotiating…'}</dd>
          <dt>Authentication</dt>
          <dd>{authLabel(activeConnection.authMethod || host?.authMethod || '')}</dd>
          <dt>Route</dt>
          <dd class="mono route">{route}</dd>
          <dt>{activeConnection.protocol === 'sftp' ? 'File browsers' : 'Terminals'}</dt>
          <dd>{activeConnection.tabOrder.length}</dd>
          {#if activeConnection.error}
            <dt>Last error</dt>
            <dd class="connection-error">{activeConnection.error}</dd>
          {/if}
        </dl>
      </section>
    {/if}

    {#if (host?.identityFiles ?? []).length}
      <section>
        <h4>Identity Files</h4>
        <ul class="plain">
          {#each host?.identityFiles ?? [] as file (file)}
            <li class="mono">{file}</li>
          {/each}
        </ul>
      </section>
    {/if}

    {#if (host?.tags ?? []).length}
      <section>
        <h4>Tags</h4>
        <div class="tags">
          {#each host?.tags ?? [] as tag (tag)}
            <span class="tag">{tag}</span>
          {/each}
        </div>
      </section>
    {/if}

    {#if host?.notes}
      <section>
        <h4>Notes</h4>
        <p class="notes">{host.notes}</p>
      </section>
    {/if}

    {#if forwardHostId != null}
      <section>
        <div class="section-head">
          <h4>Port Forwarding</h4>
          <button class="link" on:click={() => forwardingDialogHostId.set(forwardHostId)}>Manage…</button>
        </div>
        {#if rules.length === 0}
          <p class="muted">No forwards configured.</p>
        {:else}
          <ul class="forwards">
            {#each rules as rule (rule.id)}
              {@const active = !!liveForwards[rule.id]}
              <li>
                <button
                  class="fwd-toggle"
                  class:active
                  disabled={!forwardConnected}
                  title={forwardConnected ? (active ? 'Stop' : 'Start') : 'Connect to start'}
                  on:click={() => toggleForward(rule)}
                >
                  <span class="dot" />
                </button>
                <span class="fwd-flag mono">{flag(rule.kind)}</span>
                <span class="fwd-detail mono">{rule.label || describeForward(rule)}</span>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    {/if}
  {/if}
</div>

<style>
  .inspector {
    width: 100%;
    height: 100%;
    overflow-y: auto;
    padding: 10px 12px;
    background: var(--sidebar-bg);
    border-left: 1px solid var(--border-color);
    font-size: 12px;
  }

  .empty {
    color: var(--disabled-text-color);
  }

  section {
    margin-bottom: 14px;
  }

  .connection-section {
    padding-top: 10px;
    border-top: 1px solid var(--separator-color);
  }

  h3 {
    font-size: 13px;
    margin: 0 0 8px;
  }

  h4 {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    color: var(--text-color-secondary);
    margin: 0 0 6px;
  }

  dl {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 4px 10px;
    margin: 0;
  }

  dt {
    color: var(--text-color-secondary);
  }

  dd {
    margin: 0;
    overflow-wrap: anywhere;
  }

  .connection-status {
    display: flex;
    align-items: center;
    gap: 5px;
  }

  .status-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--disabled-text-color);
    flex: 0 0 auto;
  }

  .connection-status.connected .status-dot {
    background: var(--success-color);
  }

  .connection-status.connecting .status-dot {
    background: var(--warning-color);
  }

  .connection-status.error .status-dot,
  .connection-error {
    color: var(--error-color);
  }

  .connection-status.error .status-dot {
    background: var(--error-color);
  }

  .tabular {
    font-variant-numeric: tabular-nums;
  }

  .route {
    line-height: 1.45;
  }

  .plain {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .mono {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
    overflow-wrap: anywhere;
  }

  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }

  .tag {
    font-size: 11px;
    padding: 1px 6px;
    border-radius: 8px;
    background: var(--view-bg-alt);
    border: 1px solid var(--border-color);
  }

  .notes {
    white-space: pre-wrap;
    margin: 0;
  }

  .muted {
    color: var(--disabled-text-color);
    margin: 0;
  }

  .section-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 8px;
  }

  .link {
    background: transparent;
    border: none;
    padding: 0;
    font-size: 11px;
    color: var(--accent-color, var(--text-color-secondary));
    cursor: pointer;
  }

  .link:hover {
    text-decoration: underline;
  }

  .forwards {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .forwards li {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }

  .fwd-toggle {
    width: 18px;
    height: 18px;
    flex: 0 0 auto;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    background: transparent;
    border: none;
    border-radius: 3px;
  }

  .fwd-toggle:disabled {
    cursor: default;
    opacity: 0.6;
  }

  .fwd-toggle .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--disabled-text-color);
  }

  .fwd-toggle.active .dot {
    background: var(--success-color);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--success-color) 25%, transparent);
  }

  .fwd-flag {
    color: var(--accent-color, var(--text-color-secondary));
    font-weight: 600;
    flex: 0 0 auto;
  }

  .fwd-detail {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
