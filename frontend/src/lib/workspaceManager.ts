import {
  WORKSPACE_FORMAT_VERSION,
  isWorkspaceSplitNode,
  type WorkspaceBuildNode,
  type WorkspaceBuildResult,
  type WorkspaceDefinition,
  type WorkspaceLayoutNode,
  type WorkspaceLeafNode,
  type WorkspaceParseResult,
  type WorkspaceResource,
  type WorkspaceSummary
} from '../types/workspace'

const CORE_LEAF_TYPES = new Set(['terminal', 'browser', 'sftp'])

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

function jsonLocation(source: string, position: number): string {
  const before = source.slice(0, Math.max(0, position))
  const lines = before.split('\n')
  return `line ${lines.length}, column ${(lines[lines.length - 1]?.length ?? 0) + 1}`
}

function syntaxMessage(source: string, error: unknown): string {
  const raw = error instanceof Error ? error.message : String(error)
  const match = /position\s+(\d+)/i.exec(raw)
  return match?.[1] ? `Invalid JSON at ${jsonLocation(source, Number(match[1]))}: ${raw}` : `Invalid JSON: ${raw}`
}

function normalizeRatios(children: WorkspaceLayoutNode[]): WorkspaceLayoutNode[] {
  if (children.length === 0) return children
  const weights = children.map((child) => child.ratio ?? 1)
  const total = weights.reduce((sum, weight) => sum + weight, 0)
  return children.map((child, index) => ({ ...child, ratio: (weights[index] ?? 1) / total }))
}

function cloneAndValidateNode(
  value: unknown,
  path: string,
  errors: string[],
  warnings: string[]
): WorkspaceLayoutNode | null {
  if (!isRecord(value)) {
    errors.push(`${path} must be an object`)
    return null
  }
  if (typeof value.type !== 'string' || value.type.trim() === '') {
    errors.push(`${path}.type must be a non-empty string`)
    return null
  }
  if (value.ratio !== undefined && (typeof value.ratio !== 'number' || !Number.isFinite(value.ratio) || value.ratio <= 0)) {
    errors.push(`${path}.ratio must be a finite number greater than 0`)
  }
  if (value.id !== undefined && (typeof value.id !== 'string' || value.id.trim() === '')) {
    errors.push(`${path}.id must be a non-empty string when present`)
  }

  if (value.type === 'split') {
    if (value.direction !== 'horizontal' && value.direction !== 'vertical') {
      errors.push(`${path}.direction must be "horizontal" or "vertical"`)
    }
    if (!Array.isArray(value.children) || value.children.length === 0) {
      errors.push(`${path}.children must be a non-empty array`)
      return {
        ...value,
        type: 'split',
        direction: value.direction === 'vertical' ? 'vertical' : 'horizontal',
        children: []
      }
    }
    const children = value.children
      .map((child, index) => cloneAndValidateNode(child, `${path}.children[${index}]`, errors, warnings))
      .filter((child): child is WorkspaceLayoutNode => child !== null)
    return {
      ...value,
      type: 'split',
      direction: value.direction === 'vertical' ? 'vertical' : 'horizontal',
      children: normalizeRatios(children)
    }
  }

  if (CORE_LEAF_TYPES.has(value.type)) {
    if (typeof value.resource !== 'string' || value.resource.trim() === '') {
      errors.push(`${path}.resource must be a non-empty string`)
    }
  } else {
    warnings.push(`${path}: unknown node type "${value.type}" was preserved and will be skipped unless a plugin handles it`)
  }
  return { ...value, type: value.type } as WorkspaceLeafNode
}

function walkLeaves(node: WorkspaceLayoutNode, visit: (leaf: WorkspaceLeafNode, path: string) => void, path = '$.layout'): void {
  if (!isWorkspaceSplitNode(node)) {
    visit(node, path)
    return
  }
  node.children.forEach((child, index) => walkLeaves(child, visit, `${path}.children[${index}]`))
}

function validateResource(resource: unknown, path: string, errors: string[]): WorkspaceResource | null {
  if (!isRecord(resource)) {
    errors.push(`${path} must be an object`)
    return null
  }
  if (typeof resource.type !== 'string' || resource.type.trim() === '') {
    errors.push(`${path}.type must be a non-empty string`)
    return null
  }
  if (resource.type === 'ssh') {
    const validHost = typeof resource.host === 'string' && resource.host.trim() !== ''
    const validHostID = typeof resource.hostId === 'number' && Number.isInteger(resource.hostId) && resource.hostId > 0
    if (!validHost && !validHostID) errors.push(`${path} must define a non-empty host or a positive hostId`)
  }
  if (resource.type === 'browser' && (typeof resource.url !== 'string' || resource.url.trim() === '')) {
    errors.push(`${path}.url must be a non-empty string`)
  }
  return { ...resource, type: resource.type }
}

export class WorkspaceFormatError extends Error {
  readonly issues: string[]

  constructor(issues: string[]) {
    super(`Invalid workspace:\n${issues.map((issue) => `- ${issue}`).join('\n')}`)
    this.name = 'WorkspaceFormatError'
    this.issues = issues
  }
}

export interface WorkspaceStorage {
  list(): Promise<WorkspaceSummary[]>
  load(name: string): Promise<string>
  save(name: string, json: string): Promise<void>
  delete(name: string): Promise<void>
  importJSON(): Promise<string>
  exportJSON(defaultFilename: string, json: string): Promise<string>
}

export interface WorkspaceNodeFactoryRequest<TContext> {
  node: WorkspaceLeafNode
  resourceId: string
  resource: WorkspaceResource
  context: TContext
}

export type WorkspaceNodeFactory<TPane, TContext> = (
  request: WorkspaceNodeFactoryRequest<TContext>
) => Promise<TPane | null> | TPane | null

/**
 * Owns the declarative workspace lifecycle. It knows JSON and node factories,
 * but nothing about Svelte stores, DOM nodes, Wails, or concrete widgets.
 */
export class WorkspaceManager<TPane, TContext = void> {
  private readonly factories = new Map<string, WorkspaceNodeFactory<TPane, TContext>>()

  constructor(private readonly storage: WorkspaceStorage) {}

  registerNodeType(type: string, factory: WorkspaceNodeFactory<TPane, TContext>): () => void {
    if (!type.trim()) throw new Error('Workspace node type must not be empty')
    this.factories.set(type, factory)
    return () => this.factories.delete(type)
  }

  parse(source: string): WorkspaceParseResult {
    let value: unknown
    try {
      value = JSON.parse(source)
    } catch (error) {
      throw new WorkspaceFormatError([syntaxMessage(source, error)])
    }
    if (!isRecord(value)) throw new WorkspaceFormatError(['$ must be a JSON object'])

    const errors: string[] = []
    const warnings: string[] = []
    if (value.version !== WORKSPACE_FORMAT_VERSION) {
      errors.push(`$.version must be ${WORKSPACE_FORMAT_VERSION}; received ${JSON.stringify(value.version)}`)
    }
    if (typeof value.name !== 'string' || value.name.trim() === '') errors.push('$.name must be a non-empty string')

    const resources: Record<string, WorkspaceResource> = {}
    if (!isRecord(value.resources)) {
      errors.push('$.resources must be an object')
    } else {
      for (const [id, resource] of Object.entries(value.resources)) {
        if (!id.trim()) {
          errors.push('$.resources contains an empty resource id')
          continue
        }
        const parsed = validateResource(resource, `$.resources.${id}`, errors)
        if (parsed) resources[id] = parsed
      }
    }

    const layout = cloneAndValidateNode(value.layout, '$.layout', errors, warnings)
    const tabs: WorkspaceLeafNode[] = []
    const tabIDs = new Set<string>()
    if (value.tabs !== undefined) {
      if (!Array.isArray(value.tabs)) {
        errors.push('$.tabs must be an array when present')
      } else {
        value.tabs.forEach((tab, index) => {
          const parsed = cloneAndValidateNode(tab, `$.tabs[${index}]`, errors, warnings)
          if (!parsed || parsed.type === 'split') {
            if (parsed?.type === 'split') errors.push(`$.tabs[${index}] must be a leaf node, not a split`)
            return
          }
          if (!parsed.id) {
            errors.push(`$.tabs[${index}].id is required`)
          } else if (tabIDs.has(parsed.id)) {
            errors.push(`$.tabs[${index}].id duplicates "${parsed.id}"`)
          } else {
            tabIDs.add(parsed.id)
          }
          tabs.push(parsed)
        })
      }
    }

    const validateReference = (leaf: WorkspaceLeafNode, path: string): void => {
      if (!CORE_LEAF_TYPES.has(leaf.type)) return
      if (typeof leaf.resource === 'string' && !resources[leaf.resource]) {
        errors.push(`${path}.resource references missing resource "${leaf.resource}"`)
      }
    }
    if (layout) walkLeaves(layout, validateReference)
    tabs.forEach((tab, index) => validateReference(tab, `$.tabs[${index}]`))

    if (value.activeTab !== undefined) {
      if (typeof value.activeTab !== 'string' || value.activeTab.trim() === '') {
        errors.push('$.activeTab must be a non-empty string when present')
      } else if (value.tabs !== undefined && !tabIDs.has(value.activeTab)) {
        errors.push(`$.activeTab references missing tab "${value.activeTab}"`)
      }
    }

    if (errors.length > 0 || !layout) throw new WorkspaceFormatError(errors)
    const definition: WorkspaceDefinition = {
      ...value,
      version: WORKSPACE_FORMAT_VERSION,
      name: (value.name as string).trim(),
      resources,
      layout,
      ...(value.tabs !== undefined ? { tabs } : {}),
      ...(typeof value.activeTab === 'string' ? { activeTab: value.activeTab } : {})
    }
    return { definition, warnings }
  }

  serialize(definition: WorkspaceDefinition): string {
    const normalized = this.parse(JSON.stringify(definition)).definition
    return `${JSON.stringify(normalized, null, 2)}\n`
  }

  list(): Promise<WorkspaceSummary[]> {
    return this.storage.list()
  }

  async load(name: string): Promise<WorkspaceParseResult> {
    return this.parse(await this.storage.load(name))
  }

  async save(definition: WorkspaceDefinition): Promise<void> {
    await this.storage.save(definition.name, this.serialize(definition))
  }

  async delete(name: string): Promise<void> {
    await this.storage.delete(name)
  }

  async importJSON(): Promise<WorkspaceParseResult | null> {
    const source = await this.storage.importJSON()
    if (!source) return null
    const parsed = this.parse(source)
    await this.save(parsed.definition)
    return parsed
  }

  async exportJSON(definition: WorkspaceDefinition): Promise<string> {
    const filename = `${definition.name.replace(/[^a-z0-9_-]+/gi, '-').replace(/^-|-$/g, '') || 'workspace'}.json`
    return this.storage.exportJSON(filename, this.serialize(definition))
  }

  async createUI(definition: WorkspaceDefinition, context: TContext): Promise<WorkspaceBuildResult<TPane>> {
    const warnings: string[] = []
    const panes = new Map<string, TPane>()
    const attempted = new Set<string>()

    const createPane = async (node: WorkspaceLeafNode, path: string): Promise<TPane | null> => {
      if (node.id && panes.has(node.id)) return panes.get(node.id) ?? null
      if (node.id && attempted.has(node.id)) return null
      if (node.id) attempted.add(node.id)
      const factory = this.factories.get(node.type)
      if (!factory) {
        warnings.push(`${path}: node type "${node.type}" is not available`)
        return null
      }
      const resourceID = node.resource
      if (typeof resourceID !== 'string') {
        warnings.push(`${path}: resource id is missing`)
        return null
      }
      const resource = definition.resources[resourceID]
      if (!resource) {
        warnings.push(`${path}: resource "${String(resourceID ?? '')}" is not available`)
        return null
      }
      try {
        const pane = await factory({ node, resourceId: resourceID, resource, context })
        if (pane !== null && node.id) panes.set(node.id, pane)
        return pane
      } catch (error) {
        warnings.push(`${path}: ${error instanceof Error ? error.message : String(error)}`)
        return null
      }
    }

    if (definition.tabs) {
      for (let index = 0; index < definition.tabs.length; index += 1) {
        const tab = definition.tabs[index]
        if (tab) await createPane(tab, `$.tabs[${index}]`)
      }
    }

    const build = async (node: WorkspaceLayoutNode, path: string): Promise<WorkspaceBuildNode<TPane> | null> => {
      if (!isWorkspaceSplitNode(node)) {
        const pane = await createPane(node, path)
        return pane === null ? null : { type: 'leaf', pane, source: node, ratio: node.ratio ?? 1 }
      }
      const children: WorkspaceBuildNode<TPane>[] = []
      for (let index = 0; index < node.children.length; index += 1) {
        const child = node.children[index]
        if (!child) continue
        const built = await build(child, `${path}.children[${index}]`)
        if (built) children.push(built)
      }
      if (children.length === 0) return null
      if (children.length === 1) return children[0] ?? null
      const total = children.reduce((sum, child) => sum + child.ratio, 0) || 1
      return {
        type: 'split',
        direction: node.direction,
        ratio: node.ratio ?? 1,
        children: children.map((child) => ({ ...child, ratio: child.ratio / total }))
      }
    }

    const root = await build(definition.layout, '$.layout')
    const activePane = definition.activeTab ? panes.get(definition.activeTab) ?? null : null
    return { root, panes, activePane, warnings }
  }
}
