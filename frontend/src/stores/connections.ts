import { get, writable } from 'svelte/store'
import { backend, on } from '../services/backend'
import { confirmDialog } from './confirm'
import { notify } from './notifications'
import { t } from '../services/i18n'
import type { ConnectionInfo, ConnectionStatus, StatusEvent, TabKind } from '../types/connection'
import type { Host } from '../types/host'
import { reorderTabIds, type TabDropPosition } from '../lib/tabOrder'
import { normalizePanelUrl } from '../lib/panelUrl'

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
  /** True when the native view for this tab already exists and must not be created again. */
  adoptedPanel?: boolean
  /** Persisted web host backing this browser tab, when it came from the host catalog. */
  resourceHostId?: number
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

export type ConnectionAttemptReason = 'unreachable' | 'authentication' | 'failed'
export type ConnectionAttemptSpec =
  | { kind: 'host'; hostId: number; hostLabel: string; nodeType?: 'terminal' | 'sftp' }
  | { kind: 'quick'; target: string; hostLabel: string }

export interface ConnectionAttemptState {
  tabId: string
  spec: ConnectionAttemptSpec
  phase: 'connecting' | 'failed'
  startedAt: number
  error?: string
  reason?: ConnectionAttemptReason
}

export const connectionAttempts = writable<Record<string, ConnectionAttemptState>>({})
const activeAttemptRuns = new Map<string, symbol>()

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

function nextFrontendTabId(prefix: string): string {
  const randomPart = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `${prefix}-${randomPart}`
}

function connectionFailureReason(error: unknown): ConnectionAttemptReason {
  const message = error instanceof Error ? error.message : String(error)
  if (/permission denied|authentication|unable to authenticate|no supported methods|credentials?/i.test(message)) {
    return 'authentication'
  }
  if (/network is unreachable|no route|connection refused|timed?\s*out|timeout|no such host|name or service not known|lookup .*:|i\/o timeout|host is down/i.test(message)) {
    return 'unreachable'
  }
  return 'failed'
}

function attemptLabel(spec: ConnectionAttemptSpec): string {
  return spec.kind === 'host' ? spec.hostLabel : spec.hostLabel || spec.target
}

function createConnectionAttempt(spec: ConnectionAttemptSpec, reuseTabId?: string): string {
  const existing = reuseTabId ? get(tabs)[reuseTabId] : undefined
  const tabId = existing && reuseTabId ? reuseTabId : nextFrontendTabId('connection-attempt')
  const tab: TabViewModel = {
    tabId,
    connectionId: '',
    title: attemptLabel(spec),
    closed: false,
    unread: false,
    bell: false,
    kind: 'connection-attempt'
  }
  tabs.update((all) => ({ ...all, [tabId]: tab }))
  connectionAttempts.update((all) => ({
    ...all,
    [tabId]: { tabId, spec, phase: 'connecting', startedAt: Date.now() }
  }))
  activeConnectionId.set(null)
  activeTabId.set(tabId)
  return tabId
}

function updateConnectionAttempt(tabId: string, patch: Partial<ConnectionAttemptState>): void {
  connectionAttempts.update((all) => {
    const current = all[tabId]
    return current ? { ...all, [tabId]: { ...current, ...patch } } : all
  })
}

function completeConnectionAttempt(tabId: string, info: ConnectionInfo, spec: ConnectionAttemptSpec): string {
  const hostLabel = info.hostLabel || attemptLabel(spec)
  connections.update((all) => ({
    ...all,
    [info.connectionId]: {
      connectionId: info.connectionId,
      hostId: info.hostId,
      hostLabel,
      ...(spec.kind === 'quick' ? { quickTarget: spec.target } : {}),
      status: info.status,
      serverVersion: info.serverVersion,
      authMethod: info.authMethod,
      protocol: info.protocol,
      connectedAt: info.status === 'connected' ? Date.now() : undefined,
      error: info.error,
      tabOrder: [info.initialTabId]
    }
  }))
  const nextTab: TabViewModel = {
    tabId: info.initialTabId,
    connectionId: info.connectionId,
    title: info.initialTabKind === 'sftp' ? hostLabel : nextTerminalTitle(info.connectionId, hostLabel),
    closed: false,
    unread: false,
    bell: false,
    kind: info.initialTabKind
  }
  tabs.update((all) => {
    const entries = Object.entries(all).flatMap(([id, tab]) => id === tabId ? [[info.initialTabId, nextTab] as const] : [[id, tab] as const])
    return Object.fromEntries(entries)
  })
  connectionAttempts.update((all) => {
    const { [tabId]: _removed, ...rest } = all
    return rest
  })
  activeConnectionId.set(info.connectionId)
  activeTabId.set(info.initialTabId)
  if (info.warning) notify('info', info.warning)
  if (spec.kind === 'host') rememberRecent({ kind: 'host', hostId: spec.hostId, label: spec.hostLabel })
  else rememberRecent({ kind: 'quick', target: spec.target, label: hostLabel })
  return info.connectionId
}

async function runHostConnectionAttempt(spec: Extract<ConnectionAttemptSpec, { kind: 'host' }>, tabId: string): Promise<string> {
  const run = Symbol(tabId)
  activeAttemptRuns.set(tabId, run)
  updateConnectionAttempt(tabId, { phase: 'connecting', startedAt: Date.now(), error: undefined, reason: undefined })
  connectingHostIds.update((set) => new Set(set).add(spec.hostId))
  try {
    const info = spec.nodeType
      ? await backend.connectWorkspaceResource(spec.hostId, spec.nodeType, 80, 24)
      : await backend.connect(spec.hostId, 80, 24)
    if (activeAttemptRuns.get(tabId) !== run || !get(tabs)[tabId]) {
      await backend.disconnect(info.connectionId).catch(() => {})
      return ''
    }
    activeAttemptRuns.delete(tabId)
    return completeConnectionAttempt(tabId, info, spec)
  } catch (error) {
    if (activeAttemptRuns.get(tabId) === run && get(tabs)[tabId]) {
      activeAttemptRuns.delete(tabId)
      updateConnectionAttempt(tabId, {
        phase: 'failed',
        error: error instanceof Error ? error.message : String(error),
        reason: connectionFailureReason(error)
      })
    }
    throw error
  } finally {
    connectingHostIds.update((set) => {
      const next = new Set(set)
      next.delete(spec.hostId)
      return next
    })
  }
}

export function connectToHost(hostId: number, hostLabel: string): Promise<string> {
  const spec = { kind: 'host', hostId, hostLabel } as const
  return runHostConnectionAttempt(spec, createConnectionAttempt(spec))
}

/** Connects a shared SSH resource in the mode selected by a workspace leaf. */
export function connectWorkspaceResource(
  hostId: number,
  hostLabel: string,
  nodeType: 'terminal' | 'sftp'
): Promise<string> {
  const spec = { kind: 'host', hostId, hostLabel, nodeType } as const
  return runHostConnectionAttempt(spec, createConnectionAttempt(spec))
}

/** Opens a one-off SSH target without creating a sidebar host. */
export async function connectQuickTarget(target: string, reuseTabId?: string): Promise<string | null> {
  const normalized = target.trim()
  if (!normalized) return null

  const existing = Object.values(get(connections)).find(
    (connection) => connection.quickTarget === normalized && connection.status === 'connected'
  )
  if (existing) {
    const tabId = await openTerminalTab(existing.connectionId)
    if (!tabId) return null
    rememberRecent({ kind: 'quick', target: normalized, label: existing.hostLabel })
    return existing.connectionId
  }

  const existingAttempt = Object.values(get(connectionAttempts)).find(
    (attempt) => attempt.spec.kind === 'quick' && attempt.spec.target === normalized
  )
  if (existingAttempt) {
    activeConnectionId.set(null)
    activeTabId.set(existingAttempt.tabId)
    if (existingAttempt.phase === 'failed') void retryConnectionAttempt(existingAttempt.tabId)
    return null
  }
  if (connectingQuickTargets.has(normalized)) return null

  const spec = { kind: 'quick', target: normalized, hostLabel: normalized } as const
  const tabId = createConnectionAttempt(spec, reuseTabId)
  const run = Symbol(tabId)
  activeAttemptRuns.set(tabId, run)
  connectingQuickTargets.add(normalized)
  try {
    const info = await backend.quickConnect(normalized, 80, 24)
    if (activeAttemptRuns.get(tabId) !== run || !get(tabs)[tabId]) {
      await backend.disconnect(info.connectionId).catch(() => {})
      return null
    }
    activeAttemptRuns.delete(tabId)
    return completeConnectionAttempt(tabId, info, spec)
  } catch (error) {
    if (activeAttemptRuns.get(tabId) === run && get(tabs)[tabId]) {
      activeAttemptRuns.delete(tabId)
      updateConnectionAttempt(tabId, {
        phase: 'failed',
        error: error instanceof Error ? error.message : String(error),
        reason: connectionFailureReason(error)
      })
    }
    return null
  } finally {
    connectingQuickTargets.delete(normalized)
  }
}

/** Connects to hostId, or if a live connection already exists, focuses it
 * (opening a fresh tab if all its tabs were closed). A stale/dead record is
 * discarded so the host can always be reopened. Failed first attempts remain
 * visible as retryable in-frame tabs; this never rejects to UI callers. */
export async function connectOrFocusHost(hostId: number, hostLabel: string, openFreshSession = false): Promise<void> {
  const attempt = Object.values(get(connectionAttempts)).find(
    (candidate) => candidate.spec.kind === 'host' && candidate.spec.hostId === hostId
  )
  if (attempt) {
    activeConnectionId.set(null)
    activeTabId.set(attempt.tabId)
    if (attempt.phase === 'failed') void retryConnectionAttempt(attempt.tabId)
    return
  }
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
  await connectToHost(hostId, hostLabel).catch(() => {})
}

export async function retryConnectionAttempt(tabId: string): Promise<void> {
  const attempt = get(connectionAttempts)[tabId]
  if (!attempt || attempt.phase === 'connecting') return
  if (attempt.spec.kind === 'host') {
    await runHostConnectionAttempt(attempt.spec, tabId).catch(() => {})
    return
  }
  const spec = attempt.spec
  const run = Symbol(tabId)
  activeAttemptRuns.set(tabId, run)
  updateConnectionAttempt(tabId, { phase: 'connecting', startedAt: Date.now(), error: undefined, reason: undefined })
  connectingQuickTargets.add(spec.target)
  try {
    const info = await backend.quickConnect(spec.target, 80, 24)
    if (activeAttemptRuns.get(tabId) !== run || !get(tabs)[tabId]) {
      await backend.disconnect(info.connectionId).catch(() => {})
      return
    }
    activeAttemptRuns.delete(tabId)
    completeConnectionAttempt(tabId, info, spec)
  } catch (error) {
    if (activeAttemptRuns.get(tabId) === run && get(tabs)[tabId]) {
      activeAttemptRuns.delete(tabId)
      updateConnectionAttempt(tabId, {
        phase: 'failed',
        error: error instanceof Error ? error.message : String(error),
        reason: connectionFailureReason(error)
      })
    }
  } finally {
    connectingQuickTargets.delete(spec.target)
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

export { normalizePanelUrl }

/** Opens a host by its protocol: a web host becomes an embedded browser tab, an
 * SSH/SFTP host opens (or refocuses) its transport. The single entry point every
 * "connect" affordance funnels through so the three protocols stay symmetric. */
export function openHost(host: Host, openFreshSession = false): void {
  if (host.protocol === 'web') {
    openControlPanelTab(host.label, host.controlPanelUrl, host.id)
    return
  }
  void connectOrFocusHost(host.id, host.label, openFreshSession)
}

/** Opens a host's web control panel as an embedded browser tab, or focuses the
 * existing tab if that URL is already open. No SSH connection is involved. */
export function openControlPanelTab(hostLabel: string, rawUrl: string, resourceHostId?: number): string {
  const url = normalizePanelUrl(rawUrl)
  if (!url) return ''

  const existing = Object.values(get(tabs)).find((tab) => tab.kind === 'browser' && tab.url === url)
  if (existing) {
    if (resourceHostId && existing.resourceHostId !== resourceHostId) {
      tabs.update((all) => ({ ...all, [existing.tabId]: { ...existing, resourceHostId } }))
    }
    activeConnectionId.set(null)
    activeTabId.set(existing.tabId)
    return existing.tabId
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
      url,
      resourceHostId
    }
  }))
  activeConnectionId.set(null)
  activeTabId.set(tabId)
  return tabId
}

/** Wraps a tab around a native view that already exists, opened by a panel.
 *
 * The view is created by WebKit at the moment the page calls window.open, and
 * it has to stay that view — window.opener is what lets an OAuth popup hand its
 * result back to the panel that started the flow. So the tab adopts the id it
 * is given rather than opening anything, and takes focus, because a login step
 * nobody can see is worse than useless. */
export function adoptPanelPopupTab(tabId: string, url: string): void {
  let title = 'Popup'
  try {
    if (url) title = new URL(url).host || title
  } catch {
    // A popup can be handed to us before it has a usable address; the tab is
    // still worth showing, it just keeps the generic name.
  }
  tabs.update((all) => ({
    ...all,
    [tabId]: {
      tabId,
      connectionId: '',
      title,
      closed: false,
      unread: false,
      bell: false,
      kind: 'browser',
      url,
      adoptedPanel: true
    }
  }))
  activeConnectionId.set(null)
  activeTabId.set(tabId)
}

/** Clears the current runtime without confirmation before a workspace restore. */
export async function resetWorkspaceEnvironment(): Promise<string[]> {
  const connectionIDs = Object.keys(get(connections))
  const results = await Promise.allSettled(connectionIDs.map((connectionID) => backend.disconnect(connectionID)))
  const warnings = results.flatMap((result, index) =>
    result.status === 'rejected'
      ? [`Could not cleanly close connection ${connectionIDs[index] ?? ''}: ${String(result.reason)}`]
      : []
  )
  connections.set({})
  tabs.set({})
  activeAttemptRuns.clear()
  connectionAttempts.set({})
  activeConnectionId.set(null)
  activeTabId.set(null)
  terminalSizes.set({})
  closedTabs.set([])
  return warnings
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

/** Reorders the global tab strip and each affected connection's tab order. */
export function moveTab(tabId: string, targetTabId: string, position: TabDropPosition): void {
  const current = get(tabs)
  const orderedIDs = Object.keys(current)
  const nextIDs = reorderTabIds(orderedIDs, tabId, targetTabId, position)
  if (nextIDs.every((id, index) => id === orderedIDs[index])) return

  tabs.set(Object.fromEntries(nextIDs.flatMap((id) => current[id] ? [[id, current[id]]] : [])))
  connections.update((all) => {
    const next = { ...all }
    for (const [connectionID, connection] of Object.entries(all)) {
      const reordered = nextIDs.filter((id) => current[id]?.connectionId === connectionID)
      if (reordered.length === connection.tabOrder.length) next[connectionID] = { ...connection, tabOrder: reordered }
    }
    return next
  })
}

export function moveTabBy(tabId: string, delta: -1 | 1): void {
  const ids = Object.keys(get(tabs))
  const index = ids.indexOf(tabId)
  const target = ids[index + delta]
  if (!target) return
  moveTab(tabId, target, delta < 0 ? 'before' : 'after')
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
  if (tab.kind === 'connection-attempt') {
    activeAttemptRuns.delete(tabId)
    connectionAttempts.update((all) => {
      const { [tabId]: _removed, ...rest } = all
      return rest
    })
    removeTabLocally(tabId)
    return
  }
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
    } catch {
      // The failed connection attempt stays visible in its own tab.
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
    activeConnectionId.set(
      next && next.kind !== 'quick-connect' && next.kind !== 'browser' && next.kind !== 'connection-attempt'
        ? next.connectionId
        : null
    )
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

  // A web panel asked to open a window. The native view already exists; this
  // only gives it a tab and brings it to the front, because an OAuth step that
  // happens somewhere invisible is worse than no popup at all.
  on<{ tabId: string; url?: string }>('panel:popup', (evt) => {
    adoptPanelPopupTab(evt.tabId, evt.url ?? '')
  })

  // The page closed itself, which is how most OAuth popups end.
  on<{ tabId: string }>('panel:popup-closed', (evt) => {
    void closeTab(evt.tabId)
  })

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
