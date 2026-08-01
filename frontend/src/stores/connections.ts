import { get, writable } from 'svelte/store'
import { backend, on } from '../services/backend'
import { confirmDialog } from './confirm'
import { notify } from './notifications'
import { t } from '../services/i18n'
import type { ConnectionStatus, StatusEvent, TabKind } from '../types/connection'
import type { Host } from '../types/host'

export interface TabViewModel {
  tabId: string
  connectionId: string
  title: string
  closed: boolean
  unread: boolean
  bell: boolean
  kind: TabKind
  /** Only set for kind === 'browser': the control-panel URL shown in the iframe. */
  url?: string
}

export interface ConnectionViewModel {
  connectionId: string
  hostId: number
  hostLabel: string
  quickTarget?: string
  status: ConnectionStatus
  latencyMs?: number
  error?: string
  serverVersion?: string
  authMethod?: string
  protocol: 'ssh' | 'sftp'
  connectedAt?: number
  tabOrder: string[]
}

export interface RecentConnection {
  kind: 'host' | 'quick'
  hostId?: number
  target?: string
  label: string
  usedAt: number
}

interface ClosedTab {
  connectionId: string
  hostId: number
  hostLabel: string
  quickTarget?: string
  title: string
}

export const connections = writable<Record<string, ConnectionViewModel>>({})
export const tabs = writable<Record<string, TabViewModel>>({})
export const activeConnectionId = writable<string | null>(null)
export const activeTabId = writable<string | null>(null)
export const terminalSizes = writable<Record<string, { cols: number; rows: number }>>({})
export const recentConnections = writable<RecentConnection[]>([])
export const closedTabs = writable<ClosedTab[]>([])

// Visiting a tab acknowledges both ordinary background output and a terminal
// bell. A store subscription covers mouse clicks, keyboard cycling and tabs
// focused programmatically after connect/reconnect.
activeTabId.subscribe((tabId) => {
  if (tabId) clearTabAttention(tabId)
})

/** Host IDs with a Connect() call in flight — lets the sidebar show a
 * "connecting" indicator before the backend has produced a connection ID. */
export const connectingHostIds = writable<Set<number>>(new Set())
const connectingQuickTargets = new Set<string>()

const RECENT_CONNECTIONS_SETTING = 'recentConnections'

export async function loadRecentConnections(): Promise<void> {
  try {
    const setting = await backend.getSetting(RECENT_CONNECTIONS_SETTING)
    if (!setting.exists) return
    const parsed = JSON.parse(setting.value)
    if (!Array.isArray(parsed)) return
    recentConnections.set(
      parsed
        .filter((item): item is RecentConnection =>
          item != null &&
          (item.kind === 'host' || item.kind === 'quick') &&
          typeof item.label === 'string' &&
          typeof item.usedAt === 'number'
        )
        .slice(0, 5)
    )
  } catch {
    // A corrupt optional UI setting should not prevent connections.
  }
}

function rememberRecent(entry: Omit<RecentConnection, 'usedAt'>): void {
  recentConnections.update((all) => {
    const key = entry.kind === 'host' ? `host:${entry.hostId}` : `quick:${entry.target}`
    const next = [
      { ...entry, usedAt: Date.now() },
      ...all.filter((item) => (item.kind === 'host' ? `host:${item.hostId}` : `quick:${item.target}`) !== key)
    ].slice(0, 5)
    void backend.setSetting(RECENT_CONNECTIONS_SETTING, JSON.stringify(next)).catch(() => {})
    return next
  })
}

export async function connectToHost(hostId: number, hostLabel: string): Promise<string> {
  connectingHostIds.update((set) => new Set(set).add(hostId))
  try {
    const info = await backend.connect(hostId, 80, 24)
    connections.update((all) => ({
      ...all,
      [info.connectionId]: {
        connectionId: info.connectionId,
        hostId,
        hostLabel,
        status: info.status,
        serverVersion: info.serverVersion,
        authMethod: info.authMethod,
        protocol: info.protocol,
        connectedAt: info.status === 'connected' ? Date.now() : undefined,
        error: info.error,
        tabOrder: [info.initialTabId]
      }
    }))
    tabs.update((all) => ({
      ...all,
      [info.initialTabId]: {
        tabId: info.initialTabId,
        connectionId: info.connectionId,
        title: info.initialTabKind === 'sftp' ? hostLabel : nextTerminalTitle(info.connectionId, hostLabel),
        closed: false,
        unread: false,
        bell: false,
        kind: info.initialTabKind
      }
    }))
    activeConnectionId.set(info.connectionId)
    activeTabId.set(info.initialTabId)
    if (info.warning) notify('info', info.warning)
    rememberRecent({ kind: 'host', hostId, label: hostLabel })
    return info.connectionId
  } finally {
    connectingHostIds.update((set) => {
      const next = new Set(set)
      next.delete(hostId)
      return next
    })
  }
}

/** Opens a one-off SSH target without creating a sidebar host. */
export async function connectQuickTarget(target: string): Promise<string | null> {
  const normalized = target.trim()
  if (!normalized || connectingQuickTargets.has(normalized)) return null

  const existing = Object.values(get(connections)).find(
    (connection) => connection.quickTarget === normalized && connection.status === 'connected'
  )
  if (existing) {
    const tabId = await openTerminalTab(existing.connectionId)
    if (!tabId) return null
    rememberRecent({ kind: 'quick', target: normalized, label: existing.hostLabel })
    return existing.connectionId
  }

  connectingQuickTargets.add(normalized)
  try {
    const info = await backend.quickConnect(normalized, 80, 24)
    const hostLabel = info.hostLabel || normalized
    connections.update((all) => ({
      ...all,
      [info.connectionId]: {
        connectionId: info.connectionId,
        hostId: info.hostId,
        hostLabel,
        quickTarget: normalized,
        status: info.status,
        serverVersion: info.serverVersion,
        authMethod: info.authMethod,
        protocol: info.protocol,
        connectedAt: info.status === 'connected' ? Date.now() : undefined,
        error: info.error,
        tabOrder: [info.initialTabId]
      }
    }))
    tabs.update((all) => ({
      ...all,
      [info.initialTabId]: {
        tabId: info.initialTabId,
        connectionId: info.connectionId,
        title: nextTerminalTitle(info.connectionId, hostLabel),
        closed: false,
        unread: false,
        bell: false,
        kind: 'terminal'
      }
    }))
    activeConnectionId.set(info.connectionId)
    activeTabId.set(info.initialTabId)
    if (info.warning) notify('info', info.warning)
    rememberRecent({ kind: 'quick', target: normalized, label: hostLabel })
    return info.connectionId
  } catch (error) {
    notify('error', `Could not connect to ${normalized}: ${error instanceof Error ? error.message : String(error)}`)
    return null
  } finally {
    connectingQuickTargets.delete(normalized)
  }
}

/** Connects to hostId, or if a live connection already exists, focuses it
 * (opening a fresh tab if all its tabs were closed). A stale/dead record is
 * discarded so the host can always be reopened. Failures surface as an error
 * banner; this never rejects, so callers can fire-and-forget. */
export async function connectOrFocusHost(hostId: number, hostLabel: string, openFreshSession = false): Promise<void> {
  // A connect call only enters the connection store after the backend has
  // completed its handshake. Guard that gap so repeated clicks/Enter presses
  // cannot create hidden duplicate connections for the same host.
  if (get(connectingHostIds).has(hostId)) return

  const existing = Object.values(get(connections)).find((c) => c.hostId === hostId)
  if (existing?.status === 'connected') {
    if (openFreshSession && existing.protocol === 'ssh') {
      await openTerminalTab(existing.connectionId)
      return
    }
    activeConnectionId.set(existing.connectionId)
    if (existing.tabOrder.length > 0) {
      activeTabId.set(existing.tabOrder[existing.tabOrder.length - 1] ?? null)
    } else {
      await openTerminalTab(existing.connectionId)
    }
    return
  }
  if (existing?.status === 'connecting') {
    activeConnectionId.set(existing.connectionId)
    activeTabId.set(existing.tabOrder[existing.tabOrder.length - 1] ?? null)
    return
  }
  if (existing) {
    // Error/disconnected leftover — drop it so a fresh connection can be made.
    connections.update((all) => {
      const { [existing.connectionId]: _removed, ...rest } = all
      return rest
    })
  }
  try {
    await connectToHost(hostId, hostLabel)
  } catch (e) {
    notify('error', `Could not connect to ${hostLabel}: ${e instanceof Error ? e.message : String(e)}`)
  }
}

// Picks the next terminal title for a connection: the first is the bare host
// label, further ones get " (2)", " (3)", … The smallest unused number is
// chosen so the sequence stays clean (and gaps from closed tabs are refilled)
// rather than tracking an ever-growing counter.
function nextTerminalTitle(connectionId: string, hostLabel: string): string {
  const used = new Set(
    Object.values(get(tabs))
      .filter((tab) => tab.kind === 'terminal' && tab.connectionId === connectionId)
      .map((tab) => tab.title)
  )
  const titleFor = (n: number): string => (n === 1 ? hostLabel : `${hostLabel} (${n})`)
  let n = 1
  while (used.has(titleFor(n))) n++
  return titleFor(n)
}

export async function openTerminalTab(connectionId: string): Promise<string | null> {
  const conn = get(connections)[connectionId]
  if (!conn || conn.status !== 'connected') return null
  try {
    const tab = await backend.newTerminalTab(connectionId, 80, 24)
    const title = nextTerminalTitle(connectionId, conn.hostLabel)
    tabs.update((all) => ({
      ...all,
      [tab.tabId]: { tabId: tab.tabId, connectionId, title, closed: false, unread: false, bell: false, kind: tab.kind }
    }))
    connections.update((all) => {
      const existing = all[connectionId]
      if (!existing) return all
      return { ...all, [connectionId]: { ...existing, tabOrder: [...existing.tabOrder, tab.tabId] } }
    })
    activeTabId.set(tab.tabId)
    activeConnectionId.set(connectionId)
    return tab.tabId
  } catch (e) {
    notify('error', `Could not open a new tab: ${e instanceof Error ? e.message : String(e)}`)
    return null
  }
}

/** Opens a local connection chooser. No backend connection or PTY exists
 * until the user selects a recent target or submits Quick Connect. */
export function openNewTab(): string {
  const randomPart = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`
  const tabId = `quick-connect-${randomPart}`
  tabs.update((all) => ({
    ...all,
    [tabId]: {
      tabId,
      connectionId: '',
      title: get(t)('quickConnect.title'),
      closed: false,
      unread: false,
      bell: false,
      kind: 'quick-connect'
    }
  }))
  activeConnectionId.set(null)
  activeTabId.set(tabId)
  return tabId
}

/** Normalizes a user-entered control-panel URL: trims it and, when no scheme is
 * present, assumes https so a bare "panel.example.com:8006" still resolves. */
export function normalizePanelUrl(raw: string): string {
  const trimmed = raw.trim()
  if (!trimmed) return ''
  return /^[a-z][a-z0-9+.-]*:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`
}

/** Opens a host by its protocol: a web host becomes an embedded browser tab, an
 * SSH/SFTP host opens (or refocuses) its transport. The single entry point every
 * "connect" affordance funnels through so the three protocols stay symmetric. */
export function openHost(host: Host, openFreshSession = false): void {
  if (host.protocol === 'web') {
    openControlPanelTab(host.label, host.controlPanelUrl)
    return
  }
  void connectOrFocusHost(host.id, host.label, openFreshSession)
}

/** Opens a host's web control panel as an embedded browser tab, or focuses the
 * existing tab if that URL is already open. No SSH connection is involved. */
export function openControlPanelTab(hostLabel: string, rawUrl: string): void {
  const url = normalizePanelUrl(rawUrl)
  if (!url) return

  const existing = Object.values(get(tabs)).find((tab) => tab.kind === 'browser' && tab.url === url)
  if (existing) {
    activeConnectionId.set(null)
    activeTabId.set(existing.tabId)
    return
  }

  const randomPart = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`
  const tabId = `browser-${randomPart}`
  tabs.update((all) => ({
    ...all,
    [tabId]: {
      tabId,
      connectionId: '',
      title: hostLabel,
      closed: false,
      unread: false,
      bell: false,
      kind: 'browser',
      url
    }
  }))
  activeConnectionId.set(null)
  activeTabId.set(tabId)
}

export async function duplicateTab(tabId: string): Promise<void> {
  const source = get(tabs)[tabId]
  if (!source || source.kind !== 'terminal') return
  const duplicateId = await openTerminalTab(source.connectionId)
  if (duplicateId) renameTab(duplicateId, source.title)
}

export function renameTab(tabId: string, title: string): void {
  tabs.update((all) => {
    const existing = all[tabId]
    if (!existing) return all
    return { ...all, [tabId]: { ...existing, title } }
  })
}

export function markTabActivity(tabId: string, bell = false): void {
  if (get(activeTabId) === tabId) return
  tabs.update((all) => {
    const existing = all[tabId]
    if (!existing || (existing.unread && (!bell || existing.bell))) return all
    return { ...all, [tabId]: { ...existing, unread: true, bell: existing.bell || bell } }
  })
}

export function clearTabAttention(tabId: string): void {
  tabs.update((all) => {
    const existing = all[tabId]
    if (!existing || (!existing.unread && !existing.bell)) return all
    return { ...all, [tabId]: { ...existing, unread: false, bell: false } }
  })
}

export async function closeTab(tabId: string): Promise<void> {
  const tab = get(tabs)[tabId]
  if (!tab) return
  // Quick-connect and browser tabs are purely frontend: no PTY/backend tab to close.
  if (tab.kind === 'quick-connect' || tab.kind === 'browser') {
    removeTabLocally(tabId)
    return
  }
  const connectionSnapshot = get(connections)[tab.connectionId]
  if (connectionSnapshot?.status === 'connected' && tab.kind === 'terminal') {
    const translate = get(t)
    const confirmed = await confirmDialog(
      translate('closeTabPrompt.title'),
      translate('closeTabPrompt.body', { title: tab.title }),
      translate('closeTabPrompt.confirm'),
      translate('closeTabPrompt.cancel')
    )
    if (!confirmed) return
  }
  await backend.closeTab(tabId)
  if (connectionSnapshot && tab.kind === 'terminal') {
    closedTabs.update((all) => [
      {
        connectionId: tab.connectionId,
        hostId: connectionSnapshot.hostId,
        hostLabel: connectionSnapshot.hostLabel,
        quickTarget: connectionSnapshot.quickTarget,
        title: tab.title
      },
      ...all
    ].slice(0, 10))
  }
  removeTabLocally(tabId)

  // A connection without terminal tabs has no visible owner in the UI. Close
  // the SSH transport with its last tab so the sidebar accurately returns to
  // offline instead of showing a hidden idle connection as green.
  const connection = get(connections)[tab.connectionId]
  if (connection && connection.tabOrder.length === 0) {
    await disconnectConnection(tab.connectionId)
  }
}

export async function reopenLastClosedTab(): Promise<void> {
  const closed = get(closedTabs)[0]
  if (!closed) return

  let connection = get(connections)[closed.connectionId]
  if (!connection || connection.status !== 'connected') {
    connection = Object.values(get(connections)).find((candidate) =>
      closed.quickTarget
        ? candidate.quickTarget === closed.quickTarget && candidate.status === 'connected'
        : candidate.hostId === closed.hostId && candidate.status === 'connected'
    )
  }

  let restoredTabId: string | null = null
  if (connection) {
    restoredTabId = await openTerminalTab(connection.connectionId)
  } else if (closed.quickTarget) {
    const connectionId = await connectQuickTarget(closed.quickTarget)
    restoredTabId = connectionId ? get(activeTabId) : null
  } else {
    try {
      await connectToHost(closed.hostId, closed.hostLabel)
      restoredTabId = get(activeTabId)
    } catch (error) {
      notify('error', `Could not restore ${closed.hostLabel}: ${error instanceof Error ? error.message : String(error)}`)
    }
  }

  if (!restoredTabId) return
  renameTab(restoredTabId, closed.title)
  closedTabs.update((all) => all.slice(1))
}

function removeTabLocally(tabId: string): void {
  const tab = get(tabs)[tabId]
  const wasActive = get(activeTabId) === tabId
  tabs.update((all) => {
    const { [tabId]: _removed, ...rest } = all
    return rest
  })
  if (tab) {
    connections.update((all) => {
      const conn = all[tab.connectionId]
      if (!conn) return all
      return { ...all, [tab.connectionId]: { ...conn, tabOrder: conn.tabOrder.filter((id) => id !== tabId) } }
    })
  }
  if (wasActive) {
    const remainingTabs = Object.values(get(tabs))
    const sameConnection = tab ? remainingTabs.filter((candidate) => candidate.connectionId === tab.connectionId) : []
    const next = sameConnection[sameConnection.length - 1] ?? remainingTabs[remainingTabs.length - 1]
    activeTabId.set(next?.tabId ?? null)
    activeConnectionId.set(next && next.kind !== 'quick-connect' && next.kind !== 'browser' ? next.connectionId : null)
  }
}

export function closeQuickConnectTab(tabId: string): void {
  const tab = get(tabs)[tabId]
  if (tab?.kind === 'quick-connect') removeTabLocally(tabId)
}

export async function disconnectConnection(connectionId: string): Promise<void> {
  const snapshot = get(connections)
  const selected = snapshot[connectionId]
  const connectionIds = selected
    ? selected.quickTarget
      ? [selected.connectionId]
      : Object.values(snapshot)
          .filter((connection) => !connection.quickTarget && connection.hostId === selected.hostId)
          .map((connection) => connection.connectionId)
    : [connectionId]
  const disconnectingIds = new Set(connectionIds)

  // Reflect the user's action immediately. This also makes every sidebar row
  // for the host gray while the backend closes its PTYs and SSH transport.
  connections.update((all) => {
    const next = { ...all }
    for (const id of connectionIds) {
      const connection = next[id]
      if (connection) next[id] = { ...connection, status: 'disconnected' }
    }
    return next
  })

  const results = await Promise.allSettled(connectionIds.map((id) => backend.disconnect(id)))
  const failed = results.find((result) => result.status === 'rejected')
  if (failed?.status === 'rejected') {
    // Even if a backend connection was already gone, drop all matching UI
    // records so the host cannot remain visually stuck as connected.
    notify('error', `Disconnect reported an error: ${failed.reason instanceof Error ? failed.reason.message : String(failed.reason)}`)
  }

  for (const id of connectionIds) {
    const conn = get(connections)[id]
    conn?.tabOrder.forEach(removeTabLocally)
  }
  connections.update((all) => {
    return Object.fromEntries(Object.entries(all).filter(([id]) => !disconnectingIds.has(id)))
  })
  activeConnectionId.update((current) => (current && disconnectingIds.has(current) ? null : current))
}

let eventsInitialized = false

/** Wires backend push events into the stores. Call once at app startup. */
export function initConnectionEvents(): void {
  if (eventsInitialized) return
  eventsInitialized = true

  on<StatusEvent>('connection:status', (evt) => {
    connections.update((all) => {
      const existing = all[evt.connectionId]
      if (!existing) return all
      return {
        ...all,
        [evt.connectionId]: {
          ...existing,
          status: evt.status,
          connectedAt:
            evt.status === 'connected' && existing.status !== 'connected' ? Date.now() : existing.connectedAt,
          latencyMs: evt.latencyMs ?? existing.latencyMs,
          error: evt.error
        }
      }
    })
  })
}
