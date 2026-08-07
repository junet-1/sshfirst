<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from './Icon.svelte'
  import { t } from '../services/i18n'
  import { closeTab, connectionAttempts, retryConnectionAttempt } from '../stores/connections'
  import type { Rect } from '../lib/layoutTree'

  export let tabId: string
  export let visible = false
  export let rect: Rect | null = null

  let now = Date.now()

  onMount(() => {
    const timer = window.setInterval(() => (now = Date.now()), 250)
    return () => window.clearInterval(timer)
  })

  $: attempt = $connectionAttempts[tabId]
  $: label = attempt?.spec.kind === 'host' ? attempt.spec.hostLabel : attempt?.spec.hostLabel || attempt?.spec.target || ''
  $: elapsed = attempt ? Math.max(0, Math.floor((now - attempt.startedAt) / 1_000)) : 0
  $: title = !attempt || attempt.phase === 'connecting'
    ? $t('connectionAttempt.connecting', { host: label })
    : attempt.reason === 'unreachable'
      ? $t('connectionAttempt.unreachable')
      : attempt.reason === 'authentication'
        ? $t('connectionAttempt.authentication')
        : $t('connectionAttempt.failed')
  $: layerStyle = rect
    ? `left:${rect.left}%;top:${rect.top}%;width:${rect.width}%;height:${rect.height}%;right:auto;bottom:auto;`
    : ''
</script>

<div class="attempt-pane" class:visible aria-hidden={!visible} style={layerStyle}>
  {#if attempt}
    <div class="attempt-card" role="status" aria-live="polite">
      <span class="state-icon" class:spinning={attempt.phase === 'connecting'} class:error={attempt.phase === 'failed'}>
        <Icon name={attempt.phase === 'connecting' ? 'refresh' : 'warning'} size={23} />
      </span>
      <div class="copy">
        <h1>{title}</h1>
        <p class="target">{label}</p>
        {#if attempt.phase === 'failed' && attempt.error}
          <p class="detail">{attempt.error}</p>
        {:else}
          <p class="elapsed">{$t('connectionAttempt.elapsed', { seconds: elapsed })}</p>
        {/if}
      </div>
      {#if attempt.phase === 'failed'}
        <div class="actions">
          <button class="primary" on:click={() => retryConnectionAttempt(tabId)}>
            <Icon name="refresh" size={12} /> {$t('connectionAttempt.retry')}
          </button>
          <button on:click={() => closeTab(tabId)}>{$t('connectionAttempt.close')}</button>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .attempt-pane {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    padding: 28px;
    visibility: hidden;
    pointer-events: none;
    color: var(--terminal-fg);
    background: var(--terminal-bg);
  }
  .attempt-pane.visible { visibility: visible; pointer-events: auto; }
  .attempt-card { display: grid; grid-template-columns: 34px minmax(0, 430px); gap: 4px 12px; width: min(500px, 100%); }
  .state-icon { grid-row: 1 / span 2; display: inline-flex; align-items: flex-start; justify-content: center; padding-top: 1px; color: var(--accent-color); }
  .state-icon.error { color: var(--warning-color); }
  .state-icon.spinning :global(.icon) { animation: attempt-spin .85s linear infinite; }
  .copy { min-width: 0; }
  h1 { margin: 0; font-size: 15px; font-weight: 600; }
  p { margin: 5px 0 0; }
  .target { color: var(--terminal-fg); font-family: 'Cascadia Code', 'JetBrains Mono', ui-monospace, monospace; font-size: 12px; opacity: .76; }
  .detail { max-height: 110px; overflow: auto; padding: 7px 9px; border-left: 2px solid var(--warning-color); color: var(--terminal-fg); background: rgba(92, 96, 100, .22); font-family: 'Cascadia Code', 'JetBrains Mono', ui-monospace, monospace; font-size: 11px; line-height: 1.45; overflow-wrap: anywhere; }
  .elapsed { color: var(--terminal-fg); font-size: 11px; opacity: .6; }
  .actions { grid-column: 2; display: flex; gap: 6px; margin-top: 12px; }
  .actions button { display: inline-flex; align-items: center; gap: 5px; }
  @keyframes attempt-spin { to { transform: rotate(360deg); } }
  @media (prefers-reduced-motion: reduce) { .state-icon.spinning :global(.icon) { animation: none; } }
</style>
