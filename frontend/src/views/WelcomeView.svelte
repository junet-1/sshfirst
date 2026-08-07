<script lang="ts">
  import { tick } from 'svelte'
  import { get } from 'svelte/store'
  import Icon from '../components/Icon.svelte'
  import { t } from '../services/i18n'
  import { hosts } from '../stores/hosts'
  import {
    openHost,
    connectQuickTarget,
    closeQuickConnectTab,
    activeTabId,
    recentConnections,
    type RecentConnection
  } from '../stores/connections'
  import type { Host } from '../types/host'

  export let quickTabId: string | null = null
  export let active = true

  type RecentItem =
    | { key: string; kind: 'host'; host: Host; label: string; detail: string }
    | { key: string; kind: 'quick'; target: string; label: string; detail: string }

  let target = ''
  let connecting = false
  let targetInput: HTMLInputElement

  $: if (active && targetInput) void focusTarget()

  async function focusTarget(): Promise<void> {
    await tick()
    targetInput?.focus()
  }

  $: recentItems = buildRecentItems(
    $recentConnections,
    [...$hosts]
      .filter((host) => host.lastUsedAt)
      .sort((a, b) => (b.lastUsedAt ?? '').localeCompare(a.lastUsedAt ?? '')),
    $hosts
  )

  function buildRecentItems(stored: RecentConnection[], fallback: Host[], allHosts: Host[]): RecentItem[] {
    const result: RecentItem[] = []
    const seen = new Set<string>()

    for (const item of stored) {
      if (item.kind === 'host' && item.hostId != null) {
        const host = allHosts.find((candidate) => candidate.id === item.hostId)
        if (!host) continue
        const key = `host:${host.id}`
        seen.add(key)
        result.push({ key, kind: 'host', host, label: host.label, detail: hostDetail(host) })
      } else if (item.kind === 'quick' && item.target) {
        const key = `quick:${item.target}`
        seen.add(key)
        result.push({ key, kind: 'quick', target: item.target, label: item.label, detail: item.target })
      }
      if (result.length === 5) return result
    }

    for (const host of fallback) {
      const key = `host:${host.id}`
      if (seen.has(key)) continue
      result.push({ key, kind: 'host', host, label: host.label, detail: hostDetail(host) })
      if (result.length === 5) break
    }
    return result
  }

  function hostDetail(host: Host): string {
    const login = host.user ? `${host.user}@` : ''
    return `${login}${host.hostname}${host.port !== 22 ? `:${host.port}` : ''}`
  }

  async function quickConnect(value = target): Promise<void> {
    const trimmed = value.trim()
    if (!trimmed || connecting) return
    connecting = true
    try {
      const connected = await connectQuickTarget(trimmed, quickTabId ?? undefined)
      if (connected) {
        target = ''
        if (quickTabId) closeQuickConnectTab(quickTabId)
      }
    } finally {
      connecting = false
    }
  }

  async function openRecent(item: RecentItem): Promise<void> {
    if (item.kind === 'host') {
      openHost(item.host, true)
      if (quickTabId && get(activeTabId) !== quickTabId) closeQuickConnectTab(quickTabId)
    } else await quickConnect(item.target)
  }
</script>

<div class="welcome" class:active>
  <div class="quick-connect">
    <div class="heading">
      <Icon name="terminal" size={24} />
      <div>
        <h1>{$t('quickConnect.title')}</h1>
        <p>{$t('quickConnect.body')}</p>
      </div>
    </div>

    <form on:submit|preventDefault={() => quickConnect()}>
      <input
        bind:this={targetInput}
        type="text"
        bind:value={target}
        aria-label={$t('quickConnect.placeholder')}
        placeholder={$t('quickConnect.placeholder')}
        autocomplete="off"
        spellcheck="false"
      />
      <button class="primary" type="submit" disabled={!target.trim() || connecting}>
        {connecting ? $t('quickConnect.connecting') : $t('quickConnect.connect')}
      </button>
    </form>
    <p class="example">{$t('quickConnect.example')}</p>

    {#if recentItems.length > 0}
      <div class="recent-heading">{$t('quickConnect.recent')}</div>
      <div class="recent-list">
        {#each recentItems as item (item.key)}
          <button class="recent-row" disabled={connecting} on:click={() => openRecent(item)}>
            <span class="status-dot" />
            <span class="recent-copy">
              <strong>{item.label}</strong>
              <small>{item.detail}</small>
            </span>
            <span class="connect-label">{$t('quickConnect.connect')}</span>
          </button>
        {/each}
      </div>
    {/if}
  </div>
</div>

<style>
  .welcome {
    position: absolute;
    inset: 0;
    display: none;
    place-items: center;
    overflow: auto;
    padding: 28px;
    color: var(--text-color);
    background: var(--view-bg);
  }

  .welcome.active {
    display: grid;
  }

  .quick-connect {
    width: min(520px, 100%);
  }

  .heading {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 14px;
    color: var(--text-color-secondary);
  }

  h1 {
    margin: 0;
    color: var(--text-color);
    font-size: 15px;
    font-weight: 600;
  }

  p {
    margin: 2px 0 0;
    color: var(--disabled-text-color);
    font-size: 11.5px;
  }

  form {
    display: flex;
    gap: 6px;
  }

  form input {
    flex: 1;
    min-width: 0;
    height: 30px;
    padding: 3px 8px;
    background: var(--window-bg);
    color: var(--text-color);
    border-color: var(--border-color);
    caret-color: var(--accent-color);
    font-family: 'Cascadia Code', 'JetBrains Mono', ui-monospace, monospace;
    font-size: 12px;
  }

  form button {
    min-width: 82px;
  }

  .example {
    margin: 5px 0 18px 1px;
    font-family: 'Cascadia Code', 'JetBrains Mono', ui-monospace, monospace;
    font-size: 10.5px;
  }

  .recent-heading {
    padding: 0 2px 6px;
    border-bottom: 1px solid var(--separator-color);
    color: var(--text-color-secondary);
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .recent-list {
    display: flex;
    flex-direction: column;
  }

  .recent-row {
    width: 100%;
    min-height: 43px;
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 5px 7px;
    border: none;
    border-bottom: 1px solid var(--separator-color);
    border-radius: 0;
    background: transparent;
    color: var(--text-color);
    text-align: left;
  }

  .recent-row:hover:not(:disabled),
  .recent-row:focus-visible {
    background: var(--hover-bg);
  }

  .status-dot {
    width: 6px;
    height: 6px;
    flex: none;
    border-radius: 50%;
    background: var(--disabled-text-color);
  }

  .recent-copy {
    min-width: 0;
    display: flex;
    flex: 1;
    flex-direction: column;
  }

  .recent-copy strong {
    overflow: hidden;
    text-overflow: ellipsis;
    font-size: 12px;
    font-weight: 500;
  }

  .recent-copy small {
    overflow: hidden;
    color: var(--disabled-text-color);
    font-family: 'Cascadia Code', 'JetBrains Mono', ui-monospace, monospace;
    font-size: 10.5px;
    text-overflow: ellipsis;
  }

  .connect-label {
    color: var(--disabled-text-color);
    font-size: 10.5px;
    opacity: 0;
  }

  .recent-row:hover .connect-label,
  .recent-row:focus-visible .connect-label {
    opacity: 1;
  }
</style>
