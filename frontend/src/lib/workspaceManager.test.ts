import { describe, expect, it } from 'vitest'
import { WorkspaceFormatError, WorkspaceManager, type WorkspaceStorage } from './workspaceManager'
import { isWorkspaceSplitNode } from '../types/workspace'

const storage: WorkspaceStorage = {
  list: async () => [],
  load: async () => '',
  save: async () => {},
  delete: async () => {},
  importJSON: async () => '',
  exportJSON: async () => ''
}

function manager(): WorkspaceManager<string, void> {
  return new WorkspaceManager(storage)
}

describe('WorkspaceManager', () => {
  it('normalizes ratios recursively', () => {
    const result = manager().parse(JSON.stringify({
      version: 1,
      name: 'HomeLab',
      resources: {
        server: { type: 'ssh', host: 'home' },
        docs: { type: 'browser', url: 'https://example.com' }
      },
      layout: {
        type: 'split',
        direction: 'horizontal',
        children: [
          { type: 'terminal', resource: 'server', ratio: 2 },
          { type: 'browser', resource: 'docs', ratio: 1 }
        ]
      }
    }))
    if (!isWorkspaceSplitNode(result.definition.layout)) throw new Error('expected split')
    expect(result.definition.layout.children.map((node) => node.ratio)).toEqual([2 / 3, 1 / 3])
  })

  it('reports a useful JSON syntax location', () => {
    expect(() => manager().parse('{\n  "version": 1,\n  nope\n}')).toThrow(/line 3/i)
  })

  it('rejects missing resources with a JSON path', () => {
    expect(() => manager().parse(JSON.stringify({
      version: 1,
      name: 'Broken',
      resources: {},
      layout: { type: 'terminal', resource: 'missing' }
    }))).toThrow(/\$\.layout\.resource references missing resource "missing"/)
  })

  it('preserves and skips unknown node types', async () => {
    const instance = manager()
    instance.registerNodeType('terminal', ({ resource }) => String(resource.host))
    const parsed = instance.parse(JSON.stringify({
      version: 1,
      name: 'Future',
      resources: { server: { type: 'ssh', host: 'home' } },
      layout: {
        type: 'split',
        direction: 'horizontal',
        children: [
          { type: 'terminal', resource: 'server' },
          { type: 'kubernetes', resource: 'cluster', plugin: 'k8s' }
        ]
      }
    }))
    expect(parsed.definition.layout.type).toBe('split')
    expect(parsed.warnings).toHaveLength(1)
    const built = await instance.createUI(parsed.definition, undefined)
    expect(built.root?.type).toBe('leaf')
    expect(built.warnings[0]).toMatch(/kubernetes/)
  })

  it('collects multiple structural errors', () => {
    try {
      manager().parse(JSON.stringify({ version: 2, name: '', resources: [], layout: null }))
      throw new Error('expected parser to fail')
    } catch (error) {
      expect(error).toBeInstanceOf(WorkspaceFormatError)
      expect((error as WorkspaceFormatError).issues.length).toBeGreaterThan(2)
    }
  })

  it('creates tab declarations once and reuses them in the layout', async () => {
    const instance = manager()
    let calls = 0
    instance.registerNodeType('terminal', () => `pane-${++calls}`)
    const parsed = instance.parse(JSON.stringify({
      version: 1,
      name: 'Tabs',
      resources: { server: { type: 'ssh', host: 'home' } },
      tabs: [{ id: 'terminal-1', type: 'terminal', resource: 'server' }],
      activeTab: 'terminal-1',
      layout: { id: 'terminal-1', type: 'terminal', resource: 'server' }
    }))
    const built = await instance.createUI(parsed.definition, undefined)
    expect(calls).toBe(1)
    expect(built.root?.type).toBe('leaf')
    expect(built.activePane).toBe('pane-1')
  })
})
