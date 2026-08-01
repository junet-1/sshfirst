// Pure, framework-free layout tree for the terminal split view.
//
// A workspace layout is an n-ary tree of split nodes and leaves. Each leaf
// points at a terminal tab (by tabId); each split arranges its children in a
// row (side by side, vertical dividers) or column (stacked, horizontal
// dividers), with per-child fractional sizes that sum to 1. Nesting a split of
// the opposite direction inside a child yields arbitrary tmux-style layouts.
//
// Everything here is a pure function over immutable-ish nodes so it can be unit
// tested without a DOM. Geometry is emitted as percentages; the rendering layer
// turns rects into inline styles on the (stable, never-remounted) panes and the
// divider list into draggable handles.

export type SplitDir = 'row' | 'column'

export type LayoutNode =
  | { id: string; type: 'leaf'; tabId: string }
  | { id: string; type: 'split'; dir: SplitDir; children: LayoutNode[]; sizes: number[] }

/** A pane's box within the terminal area, in percent (0–100). */
export interface Rect {
  left: number
  top: number
  width: number
  height: number
}

/** A draggable boundary between children `index` and `index+1` of a split. */
export interface DividerRect {
  splitId: string
  index: number
  dir: SplitDir
  left: number
  top: number
  length: number // perpendicular extent (%) — how long the divider line is
  span: number // along-drag-axis extent (%) of the parent split — to map px drags to size fractions
}

const MIN_FRACTION = 0.05

function nextId(): string {
  const c = (globalThis as { crypto?: { randomUUID?: () => string } }).crypto
  return c?.randomUUID ? c.randomUUID() : `n${Math.random().toString(36).slice(2)}`
}

function leaf(tabId: string): LayoutNode {
  return { id: nextId(), type: 'leaf', tabId }
}

function splitNode(dir: SplitDir, children: LayoutNode[]): LayoutNode {
  const n = children.length
  return { id: nextId(), type: 'split', dir, children, sizes: children.map(() => 1 / n) }
}

/**
 * Builds a balanced grid tree from every tab id, choosing near-square
 * dimensions (cols = ceil(sqrt(n))). Rows are stacked in a column split; each
 * row lays its cells side by side in a row split. Returns null for no tabs and
 * a bare leaf for one.
 */
export function tileAll(tabIds: string[]): LayoutNode | null {
  const ids = tabIds.filter((id) => id.length > 0)
  if (ids.length === 0) return null
  if (ids.length === 1) return leaf(ids[0] as string)

  const cols = Math.ceil(Math.sqrt(ids.length))
  const rowNodes: LayoutNode[] = []
  for (let start = 0; start < ids.length; start += cols) {
    const slice = ids.slice(start, start + cols)
    rowNodes.push(slice.length === 1 ? leaf(slice[0] as string) : splitNode('row', slice.map(leaf)))
  }
  return rowNodes.length === 1 ? (rowNodes[0] as LayoutNode) : splitNode('column', rowNodes)
}

/**
 * Splits the leaf holding targetTabId, placing newTabId next to it in the given
 * direction. If the target is a direct child of a split that already runs in
 * that direction, the new pane is inserted as a sibling (sharing the target's
 * space) so existing divider positions are preserved; otherwise the leaf is
 * replaced by a fresh two-child split.
 */
export function splitLeaf(root: LayoutNode, targetTabId: string, newTabId: string, dir: SplitDir): LayoutNode {
  const walk = (node: LayoutNode): LayoutNode => {
    if (node.type === 'leaf') {
      if (node.tabId !== targetTabId) return node
      return splitNode(dir, [leaf(node.tabId), leaf(newTabId)])
    }
    if (node.dir === dir) {
      const idx = node.children.findIndex((c) => c.type === 'leaf' && c.tabId === targetTabId)
      if (idx >= 0) {
        const half = (node.sizes[idx] ?? 1 / node.children.length) / 2
        const children = [...node.children.slice(0, idx + 1), leaf(newTabId), ...node.children.slice(idx + 1)]
        const sizes = [...node.sizes.slice(0, idx), half, half, ...node.sizes.slice(idx + 1)]
        return { ...node, children, sizes }
      }
    }
    return { ...node, children: node.children.map(walk) }
  }
  return walk(root)
}

/**
 * Removes the leaf for tabId, collapsing any split left with a single child and
 * renormalising the remaining siblings' sizes to sum to 1. Returns null if the
 * tree becomes empty.
 */
export function removeLeaf(root: LayoutNode, tabId: string): LayoutNode | null {
  const prune = (node: LayoutNode): LayoutNode | null => {
    if (node.type === 'leaf') return node.tabId === tabId ? null : node

    const kept: LayoutNode[] = []
    const keptSizes: number[] = []
    node.children.forEach((child, i) => {
      const res = prune(child)
      if (res) {
        kept.push(res)
        keptSizes.push(node.sizes[i] ?? 0)
      }
    })

    if (kept.length === 0) return null
    if (kept.length === 1) return kept[0] as LayoutNode
    const total = keptSizes.reduce((a, b) => a + b, 0) || 1
    return { ...node, children: kept, sizes: keptSizes.map((s) => s / total) }
  }
  return prune(root)
}

/**
 * Adjusts the boundary at `index` within the split `splitId` by deltaFraction
 * (of the split's extent), taking from one neighbour and giving to the other,
 * with both clamped to a minimum fraction so a pane can't be dragged away.
 */
export function resizeDivider(
  root: LayoutNode,
  splitId: string,
  index: number,
  deltaFraction: number,
  minFraction = MIN_FRACTION
): LayoutNode {
  const walk = (node: LayoutNode): LayoutNode => {
    if (node.type === 'leaf') return node
    if (node.id === splitId) {
      const a = node.sizes[index]
      const b = node.sizes[index + 1]
      if (a === undefined || b === undefined) return node
      let na = a + deltaFraction
      let nb = b - deltaFraction
      if (na < minFraction) {
        nb -= minFraction - na
        na = minFraction
      }
      if (nb < minFraction) {
        na -= minFraction - nb
        nb = minFraction
      }
      const sizes = [...node.sizes]
      sizes[index] = na
      sizes[index + 1] = nb
      return { ...node, sizes }
    }
    return { ...node, children: node.children.map(walk) }
  }
  return walk(root)
}

/** Computes each leaf's percentage rect and the list of draggable dividers. */
export function computeRects(root: LayoutNode | null): { rects: Map<string, Rect>; dividers: DividerRect[] } {
  const rects = new Map<string, Rect>()
  const dividers: DividerRect[] = []
  if (!root) return { rects, dividers }

  const walk = (node: LayoutNode, rect: Rect): void => {
    if (node.type === 'leaf') {
      rects.set(node.tabId, rect)
      return
    }
    const total = node.sizes.reduce((a, b) => a + b, 0) || 1
    const span = node.dir === 'row' ? rect.width : rect.height
    let offset = node.dir === 'row' ? rect.left : rect.top

    node.children.forEach((child, i) => {
      const size = (span * (node.sizes[i] ?? 0)) / total
      const childRect: Rect =
        node.dir === 'row'
          ? { left: offset, top: rect.top, width: size, height: rect.height }
          : { left: rect.left, top: offset, width: rect.width, height: size }
      walk(child, childRect)

      if (i < node.children.length - 1) {
        const pos = offset + size
        dividers.push(
          node.dir === 'row'
            ? { splitId: node.id, index: i, dir: 'row', left: pos, top: rect.top, length: rect.height, span: rect.width }
            : { splitId: node.id, index: i, dir: 'column', left: rect.left, top: pos, length: rect.width, span: rect.height }
        )
      }
      offset += size
    })
  }

  walk(root, { left: 0, top: 0, width: 100, height: 100 })
  return { rects, dividers }
}

/** All tab ids referenced by the tree, in left-to-right / top-to-bottom order. */
export function leaves(root: LayoutNode | null): string[] {
  if (!root) return []
  if (root.type === 'leaf') return [root.tabId]
  return root.children.flatMap(leaves)
}
