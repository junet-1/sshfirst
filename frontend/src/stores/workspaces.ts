import { get, writable } from 'svelte/store'
import { WorkspaceManager, type WorkspaceStorage } from '../lib/workspaceManager'
import type { LayoutNode } from '../lib/layoutTree'
import type {
  WorkspaceBuildNode,
  WorkspaceDefinition,
  WorkspaceLayoutNode,
  WorkspaceLeafNode,
  WorkspaceResource,
  WorkspaceSummary
} from '../types/workspace'
import type { Host } from '../types/host'
import { backend } from '../services/backend'
import {
  activeConnectionId,
  activeTabId,
  connectQuickTarget,
  connectWorkspaceResource,
  connections,
  openControlPanelTab,
  openTerminalTab,
  renameTab,
  resetWorkspaceEnvironment,
  tabs
} from './connections'
import { hosts } from './hosts'
import { applyWorkspaceLayout, layoutTree } from './layout'
import { setBroadcast } from './broadcast'

interface RestoreContext {
  hosts: Host[]
  terminalConnections: Map<string, string>
}

const storage: WorkspaceStorage = {
  list: async () => (await backend.listWorkspaces()).map((name) => ({ name })),
  load: (name) => backend.loadWorkspace(name),
  save: (name, json) => backend.saveWorkspace(name, json),
  delete: (name) => backend.deleteWorkspace(name),
  importJSON: () => backend.importWorkspaceJSON(),
  exportJSON: (filename, json) => backend.exportWorkspaceJSON(filename, json)
}

export const workspaceManager = new WorkspaceManager<string, RestoreContext>(storage)
export const workspaceDialogOpen = writable(false)
export const workspaceSummaries = writable<WorkspaceSummary[]>([])
export const activeWorkspaceName = writable<string | null>(null)

function stringField(resource: WorkspaceResource, key: string): string {
  const value = resource[key]
  return typeof value === 'string' ? value.trim() : ''
}

function resolveHost(resource: WorkspaceResource, availableHosts: Host[]): Host | null {
  const hostID = resource.hostId
  if (typeof hostID === 'number') {
    const byID = availableHosts.find((host) => host.id === hostID)
    if (byID) return byID
  }
  const reference = stringField(resource, 'host')
  if (!reference) return null
  return (
    availableHosts.find((host) => host.label === reference) ??
    availableHosts.find((host) => host.label.toLocaleLowerCase() === reference.toLocaleLowerCase()) ??
    availableHosts.find((host) => host.hostname === reference) ??
    null
  )
}

workspaceManager.registerNodeType('terminal', async ({ node, resourceId, resource, context }) => {
  if (resource.type !== 'ssh') throw new Error(`terminal requires an ssh resource; received "${resource.type}"`)
  let connectionID = context.terminalConnections.get(resourceId)
  let tabID: string | null = null
  if (connectionID) {
    tabID = await openTerminalTab(connectionID)
  } else {
    const host = resolveHost(resource, context.hosts)
    if (host) {
      if (host.protocol === 'web') throw new Error(`host "${host.label}" is a web resource, not ssh`)
      connectionID = await connectWorkspaceResource(host.id, host.label, 'terminal')
    } else {
      const target = stringField(resource, 'host')
      if (!target) throw new Error('ssh resource has no resolvable host')
      connectionID = await connectQuickTarget(target) ?? undefined
      if (!connectionID) throw new Error(`could not connect to "${target}"`)
    }
    context.terminalConnections.set(resourceId, connectionID)
    tabID = get(connections)[connectionID]?.tabOrder[0] ?? get(activeTabId)
  }
  if (!tabID) throw new Error('connection did not create a terminal tab')
  if (node.title) renameTab(tabID, node.title)
  return tabID
})

workspaceManager.registerNodeType('browser', ({ node, resourceId, resource, context }) => {
  if (resource.type !== 'browser') throw new Error(`browser requires a browser resource; received "${resource.type}"`)
  const host = resolveHost(resource, context.hosts)
  const url = host?.protocol === 'web' ? host.controlPanelUrl : stringField(resource, 'url')
  if (!url) throw new Error('browser resource has no URL')
  const title = node.title || host?.label || stringField(resource, 'title') || resourceId
  return openControlPanelTab(title, url, host?.protocol === 'web' ? host.id : undefined)
})

workspaceManager.registerNodeType('sftp', async ({ node, resource, context }) => {
  if (resource.type !== 'ssh') throw new Error(`sftp requires an ssh resource; received "${resource.type}"`)
  const host = resolveHost(resource, context.hosts)
  if (!host) throw new Error(`SFTP host "${stringField(resource, 'host')}" was not found`)
  if (host.protocol === 'web') throw new Error(`host "${host.label}" is a web resource, not ssh`)
  const connectionID = await connectWorkspaceResource(host.id, host.label, 'sftp')
  const tabID = get(connections)[connectionID]?.tabOrder[0] ?? get(activeTabId)
  if (!tabID) throw new Error('connection did not create an SFTP tab')
  if (node.title) renameTab(tabID, node.title)
  return tabID
})

function nextRuntimeNodeID(): string {
  return globalThis.crypto?.randomUUID?.() ?? `workspace-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function toRuntimeLayout(node: WorkspaceBuildNode<string>): LayoutNode {
  if (node.type === 'leaf') return { id: nextRuntimeNodeID(), type: 'leaf', tabId: node.pane }
  return {
    id: nextRuntimeNodeID(),
    type: 'split',
    dir: node.direction === 'horizontal' ? 'row' : 'column',
    children: node.children.map(toRuntimeLayout),
    sizes: node.children.map((child) => child.ratio)
  }
}

function resourceSlug(value: string): string {
  return value
    .toLocaleLowerCase()
    .normalize('NFKD')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '') || 'resource'
}

function uniqueID(base: string, used: Set<string>): string {
  let id = base
  let suffix = 2
  while (used.has(id)) id = `${base}-${suffix++}`
  used.add(id)
  return id
}

function workspaceNodeForTab(
  tabID: string,
  index: number,
  resources: Record<string, WorkspaceResource>,
  resourceByTarget: Map<string, string>,
  usedResourceIDs: Set<string>
): WorkspaceLeafNode | null {
  const tab = get(tabs)[tabID]
  if (!tab || tab.kind === 'quick-connect') return null
  const id = `${tab.kind}-${index + 1}`
  if (tab.kind === 'browser') {
    const url = tab.url ?? ''
    if (!url) return null
    const targetKey = `browser:${url}`
    let resourceID = resourceByTarget.get(targetKey)
    if (!resourceID) {
      resourceID = uniqueID(resourceSlug(tab.title), usedResourceIDs)
      resourceByTarget.set(targetKey, resourceID)
      const catalogHost = tab.resourceHostId ? get(hosts).find((host) => host.id === tab.resourceHostId) : undefined
      resources[resourceID] = {
        type: 'browser',
        url,
        title: tab.title,
        ...(catalogHost ? { host: catalogHost.label, hostId: catalogHost.id } : {})
      }
    }
    return { id, type: 'browser', resource: resourceID, title: tab.title }
  }

  const connection = get(connections)[tab.connectionId]
  if (!connection) return null
  const target = connection.quickTarget || connection.hostLabel
  const targetKey = connection.quickTarget ? `quick:${target}` : `host:${connection.hostId}`
  let resourceID = resourceByTarget.get(targetKey)
  if (!resourceID) {
    resourceID = uniqueID(resourceSlug(target), usedResourceIDs)
    resourceByTarget.set(targetKey, resourceID)
    resources[resourceID] = {
      type: 'ssh',
      host: target,
      ...(connection.hostId > 0 && !connection.quickTarget ? { hostId: connection.hostId } : {})
    }
  }
  return { id, type: tab.kind, resource: resourceID, title: tab.title }
}

function captureLayout(node: LayoutNode, workspaceTabs: Map<string, WorkspaceLeafNode>): WorkspaceLayoutNode | null {
  if (node.type === 'leaf') return workspaceTabs.get(node.tabId) ?? null
  const children: WorkspaceLayoutNode[] = []
  node.children.forEach((child, index) => {
    const captured = captureLayout(child, workspaceTabs)
    if (captured) children.push({ ...captured, ratio: node.sizes[index] ?? 1 })
  })
  if (children.length === 0) return null
  if (children.length === 1) return children[0] ?? null
  return {
    type: 'split',
    direction: node.dir === 'row' ? 'horizontal' : 'vertical',
    children
  }
}

export function captureCurrentWorkspace(name: string): WorkspaceDefinition {
  const currentTabs = Object.values(get(tabs)).filter(
    (tab) => tab.kind !== 'quick-connect' && tab.kind !== 'connection-attempt'
  )
  if (currentTabs.length === 0) throw new Error('Open at least one terminal, browser, or SFTP tab before saving a workspace.')

  const resources: Record<string, WorkspaceResource> = {}
  const resourceByTarget = new Map<string, string>()
  const usedResourceIDs = new Set<string>()
  const capturedPairs = currentTabs.flatMap((tab, index) => {
    const node = workspaceNodeForTab(tab.tabId, index, resources, resourceByTarget, usedResourceIDs)
    return node ? [{ runtimeTabID: tab.tabId, node }] : []
  })
  const capturedTabs = capturedPairs.map((pair) => pair.node)
  if (capturedTabs.length === 0) throw new Error('The current tabs cannot be represented as a workspace.')

  const byRuntimeID = new Map<string, WorkspaceLeafNode>()
  capturedPairs.forEach((pair) => byRuntimeID.set(pair.runtimeTabID, pair.node))
  const activeRuntimeTab = get(activeTabId)
  const activeNode = activeRuntimeTab ? byRuntimeID.get(activeRuntimeTab) : undefined
  const currentLayout = get(layoutTree)
  const layout = (currentLayout ? captureLayout(currentLayout, byRuntimeID) : activeNode) ?? capturedTabs[0]
  if (!layout) throw new Error('The current layout is empty.')

  return {
    version: 1,
    name: name.trim(),
    resources,
    tabs: capturedTabs,
    ...(activeNode?.id ? { activeTab: activeNode.id } : {}),
    layout
  }
}

export async function refreshWorkspaces(): Promise<void> {
  workspaceSummaries.set(await workspaceManager.list())
}

export async function saveCurrentWorkspace(name: string): Promise<WorkspaceDefinition> {
  const definition = captureCurrentWorkspace(name)
  await workspaceManager.save(definition)
  activeWorkspaceName.set(definition.name)
  await refreshWorkspaces()
  return definition
}

export async function restoreWorkspace(definition: WorkspaceDefinition): Promise<string[]> {
  const cleanupWarnings = await resetWorkspaceEnvironment()
  setBroadcast(false, [])
  applyWorkspaceLayout(null)
  const built = await workspaceManager.createUI(definition, {
    hosts: get(hosts),
    terminalConnections: new Map()
  })
  const runtimeRoot = built.root ? toRuntimeLayout(built.root) : null
  applyWorkspaceLayout(runtimeRoot)
  const active = built.activePane ?? (built.root?.type === 'leaf' ? built.root.pane : null)
  if (active) {
    const tab = get(tabs)[active]
    activeTabId.set(active)
    activeConnectionId.set(tab && tab.kind !== 'browser' ? tab.connectionId : null)
  }
  activeWorkspaceName.set(definition.name)
  return [...cleanupWarnings, ...built.warnings]
}

export async function loadAndRestoreWorkspace(name: string): Promise<string[]> {
  const parsed = await workspaceManager.load(name)
  return [...parsed.warnings, ...(await restoreWorkspace(parsed.definition))]
}

export async function importWorkspace(): Promise<WorkspaceDefinition | null> {
  const parsed = await workspaceManager.importJSON()
  if (!parsed) return null
  await refreshWorkspaces()
  return parsed.definition
}

export async function exportWorkspace(name: string): Promise<string> {
  const parsed = await workspaceManager.load(name)
  return workspaceManager.exportJSON(parsed.definition)
}

export async function deleteWorkspace(name: string): Promise<void> {
  await workspaceManager.delete(name)
  if (get(activeWorkspaceName) === name) activeWorkspaceName.set(null)
  await refreshWorkspaces()
}
