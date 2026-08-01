<script lang="ts">
  import Icon from './Icon.svelte'
  import { t } from '../services/i18n'
  import { activeConnectionId, activeTabId, connections, terminalSizes } from '../stores/connections'
  import { hosts } from '../stores/hosts'

  $: connection = $activeConnectionId ? $connections[$activeConnectionId] : null
  $: host = connection ? $hosts.find((h) => h.id === connection?.hostId) ?? null : null
  $: size = $activeTabId ? $terminalSizes[$activeTabId] : null

  $: statusLabel = !connection
    ? $t('statusbar.disconnected')
    : connection.status === 'connecting'
      ? $t('statusbar.connecting')
      : connection.status === 'connected'
        ? $t('statusbar.connected')
        : $t('statusbar.error')
</script>

<div class="statusbar">
  <div class="segment status" class:connected={connection?.status === 'connected'} class:error={connection?.status === 'error'}>
    <Icon name="dot" size={10} />
    <span>{statusLabel}</span>
  </div>

  {#if host}
    <div class="segment">
      <span>{host.user ? `${host.user}@` : ''}{host.hostname}</span>
    </div>
    {#if host.proxyJump}
      <div class="segment">
        <Icon name="link" size={11} />
        <span>{host.proxyJump}</span>
      </div>
    {/if}
  {:else if connection}
    <div class="segment mono">
      <span>{connection.hostLabel}</span>
    </div>
  {/if}

  {#if connection?.latencyMs != null}
    <div class="segment">
      <span>{$t('statusbar.latency')}: {connection.latencyMs} ms</span>
    </div>
  {/if}

  {#if connection?.serverVersion}
    <div class="segment mono">
      <span>{connection.serverVersion}</span>
    </div>
  {/if}

  <div class="spacer" />

  {#if size}
    <div class="segment">
      <span>{size.cols}×{size.rows}</span>
    </div>
  {/if}
  <div class="segment">
    <span>{$t('statusbar.encoding')}</span>
  </div>
</div>

<style>
  .statusbar {
    display: flex;
    align-items: center;
    height: 22px;
    padding: 0 8px;
    background: var(--header-bg);
    border-top: 1px solid var(--border-color);
    font-size: 11px;
    color: var(--text-color-secondary);
    flex-shrink: 0;
  }

  .segment {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 0 8px;
    border-right: 1px solid var(--separator-color);
    height: 100%;
  }

  .segment:last-child {
    border-right: none;
  }

  .segment.mono {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
  }

  .status :global(.icon) {
    color: var(--disabled-text-color);
  }

  .status.connected :global(.icon) {
    color: var(--success-color);
  }

  .status.error :global(.icon) {
    color: var(--error-color);
  }

  .spacer {
    flex: 1;
  }
</style>
