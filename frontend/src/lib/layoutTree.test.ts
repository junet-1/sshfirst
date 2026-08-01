import { describe, it, expect } from 'vitest'
import {
  type LayoutNode,
  type Rect,
  tileAll,
  splitLeaf,
  removeLeaf,
  resizeDivider,
  computeRects,
  leaves
} from './layoutTree'

function overlaps(a: Rect, b: Rect): boolean {
  return a.left < b.left + b.width && b.left < a.left + a.width && a.top < b.top + b.height && b.top < a.top + a.height
}

const approx = (a: number, b: number, eps = 1e-9) => Math.abs(a - b) < eps

describe('tileAll', () => {
  it('returns null for no tabs and a leaf for one', () => {
    expect(tileAll([])).toBeNull()
    const one = tileAll(['a'])
    expect(one?.type).toBe('leaf')
    expect(leaves(one)).toEqual(['a'])
  })

  it('builds a near-square grid containing every tab exactly once', () => {
    const ids = Array.from({ length: 10 }, (_, i) => `t${i}`)
    const tree = tileAll(ids)
    expect(tree).not.toBeNull()
    const ls = leaves(tree)
    expect(ls.sort()).toEqual([...ids].sort())
    expect(new Set(ls).size).toBe(ids.length) // no duplicates
  })

  it('uses ceil(sqrt) columns (4 tabs -> 2x2)', () => {
    const tree = tileAll(['a', 'b', 'c', 'd'])
    expect(tree?.type).toBe('split')
    if (tree?.type === 'split') {
      expect(tree.dir).toBe('column')
      expect(tree.children).toHaveLength(2) // 2 rows
    }
  })
})

describe('computeRects', () => {
  it('returns empty geometry for null', () => {
    const { rects, dividers } = computeRects(null)
    expect(rects.size).toBe(0)
    expect(dividers).toHaveLength(0)
  })

  it('gives a single leaf the whole area and no dividers', () => {
    const { rects, dividers } = computeRects(tileAll(['solo']))
    expect(rects.get('solo')).toEqual({ left: 0, top: 0, width: 100, height: 100 })
    expect(dividers).toHaveLength(0)
  })

  it('partitions a row split by size with one divider between panes', () => {
    const tree: LayoutNode = {
      id: 's',
      type: 'split',
      dir: 'row',
      children: [
        { id: 'l1', type: 'leaf', tabId: 'a' },
        { id: 'l2', type: 'leaf', tabId: 'b' }
      ],
      sizes: [0.3, 0.7]
    }
    const { rects, dividers } = computeRects(tree)
    expect(rects.get('a')).toEqual({ left: 0, top: 0, width: 30, height: 100 })
    expect(rects.get('b')).toEqual({ left: 30, top: 0, width: 70, height: 100 })
    expect(dividers).toHaveLength(1)
    expect(dividers[0]).toMatchObject({ splitId: 's', index: 0, dir: 'row', left: 30 })
  })

  it('produces non-overlapping rects that cover the area for a grid', () => {
    const ids = Array.from({ length: 6 }, (_, i) => `g${i}`)
    const { rects } = computeRects(tileAll(ids))
    const boxes = [...rects.values()]
    for (let i = 0; i < boxes.length; i++) {
      for (let j = i + 1; j < boxes.length; j++) {
        expect(overlaps(boxes[i] as Rect, boxes[j] as Rect)).toBe(false)
      }
    }
    const area = boxes.reduce((sum, b) => sum + (b.width * b.height) / 100, 0)
    expect(approx(area, 100, 1e-6)).toBe(true)
  })
})

describe('splitLeaf', () => {
  it('replaces a solo leaf with a two-pane split', () => {
    const root = tileAll(['a']) as LayoutNode
    const next = splitLeaf(root, 'a', 'b', 'row')
    expect(next.type).toBe('split')
    expect(leaves(next)).toEqual(['a', 'b'])
    if (next.type === 'split') expect(next.sizes.map((s) => Math.round(s * 100))).toEqual([50, 50])
  })

  it('inserts as a sibling when the parent split runs the same direction, preserving other sizes', () => {
    const tree: LayoutNode = {
      id: 's',
      type: 'split',
      dir: 'row',
      children: [
        { id: 'l1', type: 'leaf', tabId: 'a' },
        { id: 'l2', type: 'leaf', tabId: 'b' }
      ],
      sizes: [0.5, 0.5]
    }
    const next = splitLeaf(tree, 'a', 'c', 'row')
    expect(leaves(next)).toEqual(['a', 'c', 'b'])
    if (next.type === 'split') {
      expect(next.children).toHaveLength(3)
      // 'a' (0.5) was halved into a + c; 'b' untouched
      expect(next.sizes.map((s) => Math.round(s * 100))).toEqual([25, 25, 50])
      expect(approx(next.sizes.reduce((a, b) => a + b, 0), 1)).toBe(true)
    }
  })

  it('nests an opposite-direction split inside the target leaf', () => {
    const tree: LayoutNode = {
      id: 's',
      type: 'split',
      dir: 'row',
      children: [
        { id: 'l1', type: 'leaf', tabId: 'a' },
        { id: 'l2', type: 'leaf', tabId: 'b' }
      ],
      sizes: [0.5, 0.5]
    }
    const next = splitLeaf(tree, 'b', 'd', 'column')
    expect(leaves(next)).toEqual(['a', 'b', 'd'])
  })
})

describe('removeLeaf', () => {
  it('collapses a split down to its sole survivor', () => {
    const root = splitLeaf(tileAll(['a']) as LayoutNode, 'a', 'b', 'row')
    const next = removeLeaf(root, 'b')
    expect(next?.type).toBe('leaf')
    expect(leaves(next)).toEqual(['a'])
  })

  it('returns null when the last leaf is removed', () => {
    expect(removeLeaf(tileAll(['a']) as LayoutNode, 'a')).toBeNull()
  })

  it('renormalises remaining sizes to sum to 1', () => {
    const tree: LayoutNode = {
      id: 's',
      type: 'split',
      dir: 'row',
      children: [
        { id: 'l1', type: 'leaf', tabId: 'a' },
        { id: 'l2', type: 'leaf', tabId: 'b' },
        { id: 'l3', type: 'leaf', tabId: 'c' }
      ],
      sizes: [0.2, 0.3, 0.5]
    }
    const next = removeLeaf(tree, 'b')
    expect(leaves(next)).toEqual(['a', 'c'])
    if (next?.type === 'split') {
      expect(approx(next.sizes.reduce((a, b) => a + b, 0), 1)).toBe(true)
      // 0.2 : 0.5 -> normalised
      expect(approx(next.sizes[0] as number, 0.2 / 0.7)).toBe(true)
    }
  })
})

describe('resizeDivider', () => {
  const tree = (): LayoutNode => ({
    id: 's',
    type: 'split',
    dir: 'row',
    children: [
      { id: 'l1', type: 'leaf', tabId: 'a' },
      { id: 'l2', type: 'leaf', tabId: 'b' }
    ],
    sizes: [0.5, 0.5]
  })

  it('moves space from one neighbour to the other', () => {
    const next = resizeDivider(tree(), 's', 0, 0.2)
    if (next.type === 'split') {
      expect(approx(next.sizes[0] as number, 0.7)).toBe(true)
      expect(approx(next.sizes[1] as number, 0.3)).toBe(true)
    }
  })

  it('clamps to the minimum fraction and keeps the sum stable', () => {
    const next = resizeDivider(tree(), 's', 0, 0.9, 0.05)
    if (next.type === 'split') {
      expect(next.sizes[0]).toBeCloseTo(0.95, 10)
      expect(next.sizes[1]).toBeCloseTo(0.05, 10)
      expect(approx((next.sizes[0] as number) + (next.sizes[1] as number), 1)).toBe(true)
    }
  })

  it('ignores an unknown split id', () => {
    const next = resizeDivider(tree(), 'nope', 0, 0.2)
    if (next.type === 'split') expect(next.sizes).toEqual([0.5, 0.5])
  })
})
