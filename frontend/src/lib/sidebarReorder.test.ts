import { describe, expect, it } from 'vitest'

import { adjacentItem, treeDropPosition } from './sidebarReorder'

describe('treeDropPosition', () => {
  it('splits host rows into before and after halves', () => {
    expect(treeDropPosition(109, 100, 40, false)).toBe('before')
    expect(treeDropPosition(121, 100, 40, false)).toBe('after')
  })

  it('reserves the middle half of folders for moving inside', () => {
    expect(treeDropPosition(104, 100, 40, true)).toBe('before')
    expect(treeDropPosition(120, 100, 40, true)).toBe('inside')
    expect(treeDropPosition(136, 100, 40, true)).toBe('after')
  })
})

describe('adjacentItem', () => {
  const items = [{ id: 1 }, { id: 2 }, { id: 3 }]

  it('finds keyboard reorder targets in either direction', () => {
    expect(adjacentItem(items, 2, -1)?.id).toBe(1)
    expect(adjacentItem(items, 2, 1)?.id).toBe(3)
  })

  it('does not wrap at list boundaries', () => {
    expect(adjacentItem(items, 1, -1)).toBeNull()
    expect(adjacentItem(items, 3, 1)).toBeNull()
  })
})
