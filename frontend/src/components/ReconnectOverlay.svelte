<script lang="ts">
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { t } from '../services/i18n'
  import type { ReconnectViewState } from '../stores/reconnect'
  import Icon from './Icon.svelte'

  export let state: ReconnectViewState

  let now = Date.now()

  onMount(() => {
    const timer = window.setInterval(() => (now = Date.now()), 250)
    return () => window.clearInterval(timer)
  })

  $: elapsedSeconds = Math.max(0, Math.floor((now - state.startedAt) / 1_000))
  $: retrySeconds = state.nextAttemptAt == null ? 0 : Math.max(0, Math.ceil((state.nextAttemptAt - now) / 1_000))
  $: isBusy = state.phase === 'connecting' || state.phase === 'waiting'
  $: statusText = (() => {
    if (state.phase === 'reconnected') return $t('reconnect.reconnected')
    if (state.phase === 'failed') {
      return state.timedOut ? $t('reconnect.timedOut') : $t('reconnect.failed')
    }
    if (state.phase === 'waiting') return $t('reconnect.retrying', { seconds: retrySeconds })
    return state.attempt === 1 ? $t('reconnect.reconnecting') : $t('reconnect.connecting')
  })()
</script>

<div
  class="reconnect-overlay"
  class:success={state.phase === 'reconnected'}
  class:blocking={state.phase !== 'reconnected'}
  role="status"
  aria-live="polite"
  aria-label={statusText}
  transition:fade={{ duration: 180 }}
>
  <div class="status-strip">
    <span class="state-icon" class:spinning={isBusy} class:warning={state.phase === 'failed'} class:done={state.phase === 'reconnected'}>
      <Icon name={state.phase === 'failed' ? 'warning' : state.phase === 'reconnected' ? 'check' : 'refresh'} size={14} />
    </span>
    <span class="message">{statusText}</span>
    <span class="elapsed" title={$t('reconnect.elapsed')}>{elapsedSeconds}s</span>
  </div>
</div>

<style>
  .reconnect-overlay {
    position: absolute;
    z-index: 4;
    inset: 0;
    display: flex;
    justify-content: center;
    align-items: center;
    background: rgba(92, 96, 100, 0.42);
    -webkit-backdrop-filter: blur(4px);
    backdrop-filter: blur(4px);
    opacity: 1;
    transition: background-color 180ms ease, backdrop-filter 180ms ease;
  }

  .reconnect-overlay.blocking {
    pointer-events: auto;
    cursor: wait;
  }

  .reconnect-overlay.success {
    pointer-events: none;
    background: rgba(92, 96, 100, 0.24);
    -webkit-backdrop-filter: blur(2px);
    backdrop-filter: blur(2px);
  }

  .status-strip {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--terminal-fg);
    font-size: 13px;
    line-height: 1;
    text-shadow: 0 1px 2px rgba(0, 0, 0, 0.65);
  }

  .state-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: var(--accent-color);
    flex: 0 0 16px;
  }

  .state-icon.warning {
    color: var(--warning-color);
  }

  .state-icon.done {
    color: var(--success-color);
  }

  .state-icon.spinning :global(.icon) {
    animation: reconnect-spin 0.85s linear infinite;
  }

  .message {
    flex: 1;
    white-space: nowrap;
  }

  .elapsed {
    min-width: 30px;
    padding-left: 8px;
    border-left: 1px solid rgba(239, 240, 241, 0.4);
    color: rgba(239, 240, 241, 0.78);
    font-family: 'Cascadia Code', 'JetBrains Mono', 'Fira Code', ui-monospace, monospace;
    font-size: 11px;
    font-variant-numeric: tabular-nums;
    text-align: right;
  }

  @keyframes reconnect-spin {
    to {
      transform: rotate(360deg);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .state-icon.spinning :global(.icon) {
      animation: none;
    }
  }
</style>
