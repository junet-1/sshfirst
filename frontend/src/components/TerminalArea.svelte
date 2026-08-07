<script lang="ts">
  import TerminalPane from './TerminalPane.svelte'
  import SftpPane from './SftpPane.svelte'
  import BrowserPane from './BrowserPane.svelte'
  import ConnectionAttemptPane from './ConnectionAttemptPane.svelte'
  import WelcomeView from '../views/WelcomeView.svelte'
  import SplitDividers from './SplitDividers.svelte'
  import { activeTabId, tabs } from '../stores/connections'
  import { isSplit, layoutRects, visibleTabIds } from '../stores/layout'

  $: tabList = Object.values($tabs)
  $: rects = $layoutRects.rects
</script>

<div class="terminal-area" class:split={$isSplit}>
  {#if tabList.length === 0}
    <WelcomeView />
  {:else}
    {#each tabList as tab (tab.tabId)}
      {@const focused = $activeTabId === tab.tabId}
      {@const visible = $isSplit ? $visibleTabIds.has(tab.tabId) : focused}
      {#if tab.kind === 'sftp'}
        <SftpPane
          tabId={tab.tabId}
          {visible}
          rect={$isSplit ? rects.get(tab.tabId) ?? null : null}
        />
      {:else if tab.kind === 'connection-attempt'}
        <ConnectionAttemptPane
          tabId={tab.tabId}
          {visible}
          rect={$isSplit ? rects.get(tab.tabId) ?? null : null}
        />
      {:else if tab.kind === 'browser'}
        <BrowserPane
          tabId={tab.tabId}
          url={tab.url ?? ''}
          {visible}
          rect={$isSplit ? rects.get(tab.tabId) ?? null : null}
        />
      {:else if tab.kind === 'quick-connect'}
        <WelcomeView quickTabId={tab.tabId} active={visible} />
      {:else}
        <TerminalPane
          tabId={tab.tabId}
          {visible}
          {focused}
          rect={$isSplit ? rects.get(tab.tabId) ?? null : null}
        />
      {/if}
    {/each}
    {#if $isSplit}
      <SplitDividers dividers={$layoutRects.dividers} />
    {/if}
  {/if}
</div>

<style>
  .terminal-area {
    position: relative;
    flex: 1;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
    isolation: isolate;
    background: var(--terminal-bg);
  }
</style>
