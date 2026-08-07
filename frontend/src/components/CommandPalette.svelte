<script lang="ts">
  import { get } from 'svelte/store'
  import { tick } from 'svelte'
  import Icon from './Icon.svelte'
  import { t } from '../services/i18n'
  import {
    aboutOpen,
    commandPaletteOpen,
    hostDialog,
    inspectorVisible,
    settingsOpen,
    sidebarVisible,
    snippetsOpen,
    terminalSearchRequest
  } from '../stores/ui'
  import { hosts, importSSHConfigWithFeedback } from '../stores/hosts'
  import { runSnippet, snippets } from '../stores/snippets'
  import {
    activeConnectionId,
    activeTabId,
    closeTab,
    closedTabs,
    openHost,
    connectQuickTarget,
    connections,
    disconnectConnection,
    duplicateTab,
    openControlPanelTab,
    openNewTab,
    reopenLastClosedTab,
    tabs
  } from '../stores/connections'
  import { canSplit, closeSplit, isSplit, splitFocused, tileAllTerminals } from '../stores/layout'
  import { reconnectConnection } from '../stores/reconnect'
  import { broadcastActive, setBroadcast } from '../stores/broadcast'
  import { workspaceDialogOpen } from '../stores/workspaces'

  type Group = 'quick' | 'tabs' | 'hosts' | 'commands' | 'snippets'

  interface Entry {
    id: string
    group: Group
    icon: string
    label: string
    hint?: string
    meta?: string
    keywords?: string
    shortcut?: string
    run: () => void | Promise<void>
    alternateRun?: () => void | Promise<void>
    alternateLabel?: string
  }

  interface ResultGroup {
    id: Group
    label: string
    entries: Entry[]
  }

  const RECENT_KEY = 'ssh-first.command-palette.recent'
  const GROUP_ORDER: Group[] = ['quick', 'tabs', 'hosts', 'commands', 'snippets']

  let query = ''
  let previousQuery = ''
  let selectedIndex = 0
  let inputEl: HTMLInputElement
  let recentIds: string[] = []

  try {
    const saved = JSON.parse(localStorage.getItem(RECENT_KEY) ?? '[]')
    if (Array.isArray(saved)) recentIds = saved.filter((id): id is string => typeof id === 'string').slice(0, 12)
  } catch {
    // Optional launcher history should never prevent the palette from opening.
  }

  $: activeConnection = $activeConnectionId ? $connections[$activeConnectionId] : null
  $: activeTab = $activeTabId ? $tabs[$activeTabId] : null
  $: activeHost = activeConnection ? $hosts.find((host) => host.id === activeConnection?.hostId) ?? null : null

  function switchToTab(tabId: string): void {
    const tab = get(tabs)[tabId]
    if (!tab) return
    activeTabId.set(tabId)
    activeConnectionId.set(
      tab.kind === 'quick-connect' || tab.kind === 'browser' || tab.kind === 'connection-attempt' ? null : tab.connectionId
    )
  }

  async function copyActiveConnectionCommand(): Promise<void> {
    if (activeHost) {
      const command = activeHost.protocol === 'sftp' ? 'sftp' : 'ssh'
      const portFlag = activeHost.protocol === 'sftp' ? '-P' : '-p'
      const port = activeHost.port !== 22 ? ` ${portFlag} ${activeHost.port}` : ''
      const user = activeHost.user ? `${activeHost.user}@` : ''
      await navigator.clipboard.writeText(`${command}${port} ${user}${activeHost.hostname}`)
    } else if (activeConnection?.quickTarget) {
      const target = activeConnection.quickTarget
      await navigator.clipboard.writeText(target.startsWith('ssh ') ? target : `ssh ${target}`)
    }
  }

  $: commandEntries = [
    {
      id: 'cmd.newTab',
      group: 'commands',
      icon: 'plus',
      label: $t('quickConnect.title'),
      hint: $t('quickConnect.body'),
      keywords: 'new tab session connect ssh',
      shortcut: 'Ctrl T',
      run: openNewTab
    },
    {
      id: 'cmd.newHost',
      group: 'commands',
      icon: 'server',
      label: $t('menu.file.newHost'),
      hint: $t('commandPalette.hint.newHost'),
      keywords: 'add create server ssh sftp',
      shortcut: 'Ctrl N',
      run: () => hostDialog.set({ open: true, editingId: null })
    },
    {
      id: 'cmd.importConfig',
      group: 'commands',
      icon: 'download',
      label: $t('menu.file.importConfig'),
      keywords: 'ssh config hosts import',
      run: importSSHConfigWithFeedback
    },
    {
      id: 'cmd.settings',
      group: 'commands',
      icon: 'settings',
      label: $t('menu.file.settings'),
      keywords: 'preferences theme language font',
      shortcut: 'Ctrl ,',
      run: () => settingsOpen.set(true)
    },
    {
      id: 'cmd.workspaces',
      group: 'commands',
      icon: 'layers',
      label: $t('workspaces.title'),
      hint: $t('workspaces.saveHint'),
      keywords: 'workspace layout save restore import export',
      run: () => workspaceDialogOpen.set(true)
    },
    {
      id: 'cmd.toggleSidebar',
      group: 'commands',
      icon: 'sidebar',
      label: $sidebarVisible ? $t('commandPalette.hideSidebar') : $t('commandPalette.showSidebar'),
      keywords: 'view navigation hosts',
      run: () => sidebarVisible.update((visible) => !visible)
    },
    {
      id: 'cmd.toggleInspector',
      group: 'commands',
      icon: 'inspector',
      label: $inspectorVisible ? $t('commandPalette.hideInspector') : $t('commandPalette.showInspector'),
      keywords: 'view details connection',
      run: () => inspectorVisible.update((visible) => !visible)
    },
    {
      id: 'cmd.snippets',
      group: 'commands',
      icon: 'code',
      label: $t('menu.tools.snippets'),
      hint: $t('commandPalette.hint.snippets'),
      keywords: 'commands scripts manage',
      run: () => snippetsOpen.set(true)
    },
    {
      id: 'cmd.about',
      group: 'commands',
      icon: 'shield',
      label: $t('menu.help.about'),
      keywords: 'version github license mit',
      run: () => aboutOpen.set(true)
    },
    ...(activeTab
      ? [
          {
            id: 'cmd.closeTab',
            group: 'commands' as const,
            icon: 'x',
            label: $t('menu.session.closeTab'),
            hint: activeTab.title,
            keywords: 'close terminal session',
            shortcut: 'Ctrl W',
            run: () => closeTab(activeTab.tabId)
          }
        ]
      : []),
    ...(activeTab?.kind === 'terminal'
      ? [
          {
            id: 'cmd.duplicateTab',
            group: 'commands' as const,
            icon: 'copy',
            label: $t('menu.session.duplicateTab'),
            hint: activeTab.title,
            keywords: 'clone terminal session',
            run: () => duplicateTab(activeTab.tabId)
          },
          {
            id: 'cmd.findTerminal',
            group: 'commands' as const,
            icon: 'search',
            label: $t('menu.edit.findTerminal'),
            keywords: 'search output',
            shortcut: 'Ctrl ⇧ F',
            run: () => terminalSearchRequest.update((value) => value + 1)
          }
        ]
      : []),
    ...($closedTabs.length
      ? [
          {
            id: 'cmd.reopenClosedTab',
            group: 'commands' as const,
            icon: 'refresh',
            label: $t('menu.session.reopenClosedTab'),
            keywords: 'restore undo terminal',
            shortcut: 'Ctrl ⇧ T',
            run: reopenLastClosedTab
          }
        ]
      : []),
    ...(activeConnection
      ? [
          {
            id: 'cmd.copyConnection',
            group: 'commands' as const,
            icon: 'copy',
            label: $t('menu.session.copySSHCommand'),
            hint: activeConnection.hostLabel,
            keywords: 'clipboard ssh sftp command',
            run: copyActiveConnectionCommand
          },
          {
            id: 'cmd.reconnect',
            group: 'commands' as const,
            icon: 'link',
            label: $t('menu.session.reconnect'),
            hint: activeConnection.hostLabel,
            keywords: 'retry connection',
            run: () => reconnectConnection(activeConnection.connectionId)
          },
          {
            id: 'cmd.disconnect',
            group: 'commands' as const,
            icon: 'unlink',
            label: $t('menu.session.disconnect'),
            hint: activeConnection.hostLabel,
            keywords: 'close connection offline',
            shortcut: 'Ctrl ⇧ W',
            run: () => disconnectConnection(activeConnection.connectionId)
          }
        ]
      : []),
    ...(Object.values($tabs).filter((tab) => tab.kind === 'terminal').length >= 2 || $broadcastActive
      ? [
          {
            id: 'cmd.broadcast',
            group: 'commands' as const,
            icon: 'broadcast',
            label: $broadcastActive ? $t('commandPalette.stopBroadcast') : $t('menu.session.broadcast'),
            keywords: 'send input all terminals',
            shortcut: 'Ctrl ⇧ B',
            run: () =>
              setBroadcast(
                !get(broadcastActive),
                Object.values(get(tabs))
                  .filter((tab) => tab.kind === 'terminal')
                  .map((tab) => tab.tabId)
              )
          }
        ]
      : []),
    ...($canSplit
      ? [
          {
            id: 'cmd.splitRight',
            group: 'commands' as const,
            icon: 'split-h',
            label: $t('split.right'),
            keywords: 'split pane vertical vertically side by side columns tile',
            run: () => splitFocused('row')
          },
          {
            id: 'cmd.splitDown',
            group: 'commands' as const,
            icon: 'split-v',
            label: $t('split.down'),
            keywords: 'split pane horizontal horizontally stacked rows tile',
            run: () => splitFocused('column')
          }
        ]
      : []),
    ...(Object.values($tabs).filter((tab) => tab.kind === 'terminal').length >= 2
      ? [
          {
            id: 'cmd.tileAll',
            group: 'commands' as const,
            icon: 'grid',
            label: $t('split.tileAll'),
            keywords: 'split all terminals tile grid arrange panes',
            run: tileAllTerminals
          }
        ]
      : []),
    ...($isSplit
      ? [
          {
            id: 'cmd.untile',
            group: 'commands' as const,
            icon: 'terminal',
            label: $t('split.untile'),
            keywords: 'untile back tabs single pane unsplit',
            run: closeSplit
          }
        ]
      : [])
  ] satisfies Entry[]

  $: tabEntries = Object.values($tabs).map(
    (tab): Entry => {
      const connection = tab.connectionId ? $connections[tab.connectionId] : null
      return {
        id: `tab.${tab.tabId}`,
        group: 'tabs',
        icon: tab.kind === 'browser' ? 'globe' : tab.kind === 'sftp' ? 'sftp' : 'terminal',
        label: tab.title,
        hint:
          tab.kind === 'quick-connect'
            ? $t('quickConnect.body')
            : tab.kind === 'browser'
              ? tab.url
              : connection?.hostLabel,
        meta: tab.tabId === $activeTabId ? $t('commandPalette.active') : tab.unread ? $t('commandPalette.activity') : undefined,
        keywords: `switch focus ${tab.kind}`,
        run: () => switchToTab(tab.tabId)
      }
    }
  )

  $: hostEntries = $hosts.map(
    (host): Entry => {
      const connection = Object.values($connections).find(
        (candidate) => candidate.hostId === host.id && candidate.status === 'connected'
      )
      return {
        id: `host.${host.id}`,
        group: 'hosts',
        icon: host.protocol === 'sftp' ? 'sftp' : host.protocol === 'web' ? 'globe' : 'terminal',
        label: host.label,
        hint:
          host.protocol === 'web'
            ? host.controlPanelUrl
            : `${host.user ? host.user + '@' : ''}${host.hostname}${host.port !== 22 ? ':' + host.port : ''}`,
        meta: connection ? $t('statusbar.connected') : undefined,
        keywords: `${host.tags.join(' ')} ${host.protocol}`,
        run: () => openHost(host),
        alternateRun: host.protocol === 'ssh' ? () => openHost(host, true) : undefined,
        alternateLabel: host.protocol === 'ssh' ? $t('commandPalette.newSession') : undefined
      }
    }
  )

  $: panelEntries = $hosts
    .filter((host) => host.protocol !== 'web' && host.controlPanelUrl)
    .map(
      (host): Entry => ({
        id: `panel.${host.id}`,
        group: 'commands',
        icon: 'globe',
        label: `${$t('sidebar.context.controlPanel')} — ${host.label}`,
        hint: host.controlPanelUrl,
        keywords: `control panel web browser dashboard ${host.label}`,
        run: () => { openControlPanelTab(host.label, host.controlPanelUrl, host.id) }
      })
    )

  $: snippetEntries = activeTab?.kind === 'terminal'
    ? $snippets.map(
        (snippet): Entry => ({
          id: `snippet.${snippet.id}`,
          group: 'snippets',
          icon: 'code',
          label: snippet.name,
          hint: snippet.command,
          keywords: 'run command script',
          run: () => runSnippet(snippet)
        })
      )
    : []

  function quickConnectEntry(value: string): Entry | null {
    const target = value.trim().replace(/^ssh\s+/i, '')
    if (!target || /\s/.test(target)) return null
    const looksLikeTarget =
      value.trim().toLowerCase().startsWith('ssh ') ||
      target.includes('@') ||
      target === 'localhost' ||
      target.includes('.') ||
      /^\[[0-9a-f:]+\](?::\d+)?$/i.test(target)
    if (!looksLikeTarget) return null
    return {
      id: `quick.${target}`,
      group: 'quick',
      icon: 'link',
      label: $t('commandPalette.connectTo', { target }),
      hint: $t('quickConnect.body'),
      meta: 'SSH',
      run: async () => {
        await connectQuickTarget(target)
      }
    }
  }

  function fuzzyScore(entry: Entry, rawQuery: string): number {
    const needle = rawQuery.trim().toLowerCase()
    if (!needle) return 1
    const haystack = `${entry.label} ${entry.hint ?? ''} ${entry.keywords ?? ''}`.toLowerCase()
    if (haystack === needle) return 1_000
    if (entry.label.toLowerCase().startsWith(needle)) return 800 - entry.label.length
    const direct = haystack.indexOf(needle)
    if (direct >= 0) return 600 - direct

    let position = -1
    let gapPenalty = 0
    for (const char of needle) {
      const next = haystack.indexOf(char, position + 1)
      if (next < 0) return -1
      if (position >= 0) gapPenalty += next - position - 1
      position = next
    }
    return 300 - gapPenalty
  }

  function recentRank(id: string): number {
    const index = recentIds.indexOf(id)
    return index < 0 ? 0 : recentIds.length - index
  }

  $: allEntries = [...tabEntries, ...hostEntries, ...commandEntries, ...panelEntries, ...snippetEntries]
  $: quickEntry = quickConnectEntry(query)
  $: resultGroups = GROUP_ORDER.map((group): ResultGroup | null => {
    if (group === 'quick') {
      return quickEntry ? { id: group, label: $t('commandPalette.group.quick'), entries: [quickEntry] } : null
    }
    const entries = allEntries
      .filter((entry) => entry.group === group)
      .map((entry) => ({ entry, score: fuzzyScore(entry, query) }))
      .filter(({ score }) => score >= 0)
      .sort((a, b) => b.score - a.score || recentRank(b.entry.id) - recentRank(a.entry.id) || a.entry.label.localeCompare(b.entry.label))
      .map(({ entry }) => entry)
      .slice(0, query.trim() ? 40 : group === 'commands' ? 8 : 6)
    if (!entries.length) return null
    return { id: group, label: $t(`commandPalette.group.${group}`), entries }
  }).filter((group): group is ResultGroup => group !== null)
  $: flatResults = resultGroups.flatMap((group) => group.entries)

  $: if (query !== previousQuery) {
    previousQuery = query
    selectedIndex = 0
  }
  $: if (selectedIndex >= flatResults.length) selectedIndex = Math.max(0, flatResults.length - 1)
  $: selectedEntry = flatResults[selectedIndex]

  function close(): void {
    commandPaletteOpen.set(false)
    query = ''
  }

  function rememberEntry(id: string): void {
    if (id.startsWith('quick.')) return
    recentIds = [id, ...recentIds.filter((recentId) => recentId !== id)].slice(0, 12)
    try {
      localStorage.setItem(RECENT_KEY, JSON.stringify(recentIds))
    } catch {
      // Private storage modes can reject local persistence; the command still runs.
    }
  }

  function runEntry(entry: Entry, alternate = false): void {
    rememberEntry(entry.id)
    close()
    void (alternate && entry.alternateRun ? entry.alternateRun() : entry.run())
  }

  async function moveSelection(delta: number): Promise<void> {
    if (!flatResults.length) return
    selectedIndex = (selectedIndex + delta + flatResults.length) % flatResults.length
    await tick()
    document.getElementById(`palette-result-${selectedIndex}`)?.scrollIntoView({ block: 'nearest' })
  }

  function onKeydown(e: KeyboardEvent): void {
    if (e.key === 'Escape') {
      e.preventDefault()
      close()
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      void moveSelection(1)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      void moveSelection(-1)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (selectedEntry) runEntry(selectedEntry, e.shiftKey)
    }
  }

  $: if ($commandPaletteOpen) {
    queueMicrotask(() => inputEl?.focus())
  }
</script>

{#if $commandPaletteOpen}
  <!-- svelte-ignore a11y-no-static-element-interactions a11y-click-events-have-key-events -->
  <div class="backdrop" role="presentation" on:mousedown={close}>
    <!-- svelte-ignore a11y-no-noninteractive-element-interactions -->
    <div class="palette" role="dialog" aria-modal="true" aria-label={$t('menu.tools.commandPalette')} on:mousedown|stopPropagation>
      <div class="search-row">
        <Icon name="search" size={18} />
        <input
          bind:this={inputEl}
          type="text"
          bind:value={query}
          placeholder={$t('commandPalette.placeholder')}
          autocomplete="off"
          spellcheck="false"
          on:keydown={onKeydown}
        />
        <kbd>Esc</kbd>
      </div>

      <div class="results" role="listbox" aria-label={$t('commandPalette.results')}>
        {#each resultGroups as group (group.id)}
          <section aria-label={group.label}>
            <div class="group-label">{group.label}</div>
            {#each group.entries as entry (entry.id)}
              {@const index = flatResults.indexOf(entry)}
              <button
                id="palette-result-{index}"
                class="result"
                class:selected={index === selectedIndex}
                role="option"
                aria-selected={index === selectedIndex}
                on:click={() => runEntry(entry)}
                on:mouseenter={() => (selectedIndex = index)}
              >
                <span class="entry-icon"><Icon name={entry.icon} size={15} /></span>
                <span class="entry-copy">
                  <span class="entry-label">{entry.label}</span>
                  {#if entry.hint}<span class="entry-hint">{entry.hint}</span>{/if}
                </span>
                {#if entry.meta}<span class="meta" class:connected={entry.meta === $t('statusbar.connected')}>{entry.meta}</span>{/if}
                {#if entry.shortcut}<kbd class="shortcut">{entry.shortcut}</kbd>{/if}
              </button>
            {/each}
          </section>
        {/each}
        {#if flatResults.length === 0}
          <div class="empty">
            <Icon name="search" size={20} />
            <span>{$t('commandPalette.noResults')}</span>
            <small>{$t('commandPalette.noResultsHint')}</small>
          </div>
        {/if}
      </div>

      <footer>
        <span class="result-count">{flatResults.length} {$t(flatResults.length === 1 ? 'commandPalette.result' : 'commandPalette.resultsCount')}</span>
        <span class="key-help"><kbd>↑</kbd><kbd>↓</kbd> {$t('commandPalette.navigate')}</span>
        {#if selectedEntry?.alternateRun}
          <span class="key-help"><kbd>⇧</kbd><kbd>↵</kbd> {selectedEntry.alternateLabel}</span>
        {/if}
        <span class="key-help"><kbd>↵</kbd> {$t('commandPalette.open')}</span>
      </footer>
    </div>
  </div>
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 600;
    display: flex;
    justify-content: center;
    padding: 9vh 16px 16px;
    background: rgba(9, 11, 13, 0.42);
    backdrop-filter: blur(4px);
  }

  .palette {
    width: min(680px, calc(100vw - 32px));
    max-height: min(72vh, 680px);
    align-self: flex-start;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    color: var(--text-color);
    background: color-mix(in srgb, var(--window-bg) 96%, transparent);
    border: 1px solid color-mix(in srgb, var(--border-color) 78%, var(--text-color-secondary));
    border-radius: 9px;
    box-shadow: 0 22px 70px rgba(0, 0, 0, 0.46), 0 2px 10px var(--shadow-color);
    animation: palette-in 170ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }

  .search-row {
    display: flex;
    align-items: center;
    gap: 11px;
    min-height: 58px;
    padding: 0 16px;
    color: var(--text-color-secondary);
    border-bottom: 1px solid var(--separator-color);
  }

  .search-row input {
    flex: 1;
    min-width: 0;
    height: 56px;
    padding: 0;
    color: var(--text-color);
    background: transparent;
    border: 0;
    border-radius: 0;
    outline: 0;
    font-size: 16px;
    letter-spacing: -0.01em;
  }

  .search-row input::placeholder {
    color: var(--disabled-text-color);
  }

  .results {
    flex: 1;
    min-height: 120px;
    overflow-y: auto;
    padding: 5px 6px 7px;
  }

  section + section {
    margin-top: 3px;
  }

  .group-label {
    padding: 8px 9px 4px;
    color: var(--text-color-secondary);
    font-size: 10px;
    font-weight: 650;
    letter-spacing: 0.055em;
    text-transform: uppercase;
  }

  .result {
    position: relative;
    width: 100%;
    min-height: 46px;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 5px 9px;
    color: var(--text-color);
    background: transparent;
    border: 0;
    border-radius: 5px;
    text-align: left;
  }

  .result:hover:not(:disabled),
  .result.selected {
    background: var(--active-bg);
  }

  .entry-icon {
    width: 30px;
    height: 30px;
    flex: 0 0 30px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: var(--text-color-secondary);
    background: var(--view-bg-alt);
    border: 1px solid var(--separator-color);
    border-radius: 5px;
  }

  .result.selected .entry-icon {
    color: var(--accent-color);
    border-color: color-mix(in srgb, var(--accent-color) 45%, var(--border-color));
  }

  .entry-copy {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .entry-label,
  .entry-hint {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .entry-label {
    font-size: 12.5px;
    font-weight: 520;
  }

  .entry-hint {
    color: var(--text-color-secondary);
    font-size: 10.5px;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
  }

  .meta {
    flex-shrink: 0;
    padding: 1px 6px;
    color: var(--text-color-secondary);
    background: var(--view-bg-alt);
    border: 1px solid var(--separator-color);
    border-radius: 8px;
    font-size: 9.5px;
    font-weight: 600;
    letter-spacing: 0.025em;
    text-transform: uppercase;
  }

  .meta.connected {
    color: var(--success-color);
  }

  kbd {
    min-width: 21px;
    height: 19px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0 5px;
    color: var(--text-color-secondary);
    background: var(--view-bg-alt);
    border: 1px solid var(--border-color);
    border-bottom-color: color-mix(in srgb, var(--border-color) 70%, black);
    border-radius: 4px;
    font: 10px 'JetBrains Mono', ui-monospace, monospace;
    white-space: nowrap;
  }

  .shortcut {
    min-width: auto;
    flex-shrink: 0;
  }

  .empty {
    min-height: 150px;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 5px;
    color: var(--text-color-secondary);
    font-size: 12.5px;
  }

  .empty small {
    color: var(--disabled-text-color);
    font-size: 10.5px;
  }

  footer {
    min-height: 34px;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 11px;
    padding: 5px 10px;
    color: var(--text-color-secondary);
    background: var(--header-bg);
    border-top: 1px solid var(--separator-color);
    font-size: 10px;
  }

  .result-count {
    margin-right: auto;
  }

  .key-help {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    white-space: nowrap;
  }

  footer kbd {
    min-width: 18px;
    height: 17px;
    padding: 0 4px;
    font-size: 9px;
  }

  @keyframes palette-in {
    from { opacity: 0; transform: translateY(-7px) scale(0.985); }
    to { opacity: 1; transform: translateY(0) scale(1); }
  }

  @media (max-width: 560px) {
    .backdrop { padding-top: 4vh; }
    .meta, .shortcut, .result-count { display: none; }
    footer { gap: 7px; }
  }

  @media (prefers-reduced-motion: reduce) {
    .palette { animation: none; }
  }
</style>
