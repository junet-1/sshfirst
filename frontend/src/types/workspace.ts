export const WORKSPACE_FORMAT_VERSION = 1 as const

export type WorkspaceDirection = 'horizontal' | 'vertical'

/**
 * Resources are deliberately open-ended. Core resources use `ssh` and
 * `browser`; plugins can add fields and resource types without a format bump.
 */
export type WorkspaceResource = {
  type: string
  [key: string]: unknown
}

export type WorkspaceLeafNode = {
  id?: string
  type: string
  resource?: string
  ratio?: number
  title?: string
  [key: string]: unknown
}

export type WorkspaceSplitNode = {
  id?: string
  type: 'split'
  direction: WorkspaceDirection
  ratio?: number
  children: WorkspaceLayoutNode[]
  [key: string]: unknown
}

export type WorkspaceLayoutNode = WorkspaceSplitNode | WorkspaceLeafNode

export function isWorkspaceSplitNode(node: WorkspaceLayoutNode): node is WorkspaceSplitNode {
  return node.type === 'split' && Array.isArray((node as WorkspaceSplitNode).children)
}

/**
 * `tabs` and `activeTab` are optional v1 extensions. A portable hand-written
 * workspace only needs `layout`; SSH First writes the extensions so tabs that
 * are currently hidden behind the tab bar can be restored as well.
 */
export type WorkspaceDefinition = {
  version: typeof WORKSPACE_FORMAT_VERSION
  name: string
  resources: Record<string, WorkspaceResource>
  layout: WorkspaceLayoutNode
  tabs?: WorkspaceLeafNode[]
  activeTab?: string
  [key: string]: unknown
}

export interface WorkspaceParseResult {
  definition: WorkspaceDefinition
  warnings: string[]
}

export interface WorkspaceBuildLeaf<TPane> {
  type: 'leaf'
  pane: TPane
  source: WorkspaceLeafNode
  ratio: number
}

export interface WorkspaceBuildSplit<TPane> {
  type: 'split'
  direction: WorkspaceDirection
  children: WorkspaceBuildNode<TPane>[]
  ratio: number
}

export type WorkspaceBuildNode<TPane> = WorkspaceBuildLeaf<TPane> | WorkspaceBuildSplit<TPane>

export interface WorkspaceBuildResult<TPane> {
  root: WorkspaceBuildNode<TPane> | null
  panes: Map<string, TPane>
  activePane: TPane | null
  warnings: string[]
}

export interface WorkspaceSummary {
  name: string
}
