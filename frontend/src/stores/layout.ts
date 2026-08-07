import { derived, get, writable } from 'svelte/store'
import {
  type LayoutNode,
  type SplitDir,
  computeRects,
  leaves,
  removeLeaf,
  resizeDivider,
  splitLeaf,
  tileAll
} from '../lib/layoutTree'
import { activeTabId, tabs } from './connections'

// The global terminal layout. null (or a single leaf, which we normalise to
// null) means "tabbed": exactly one pane visible, driven by activeTabId as
// before. A tree with 2+ leaves means "split": those panes are tiled and
// resizable, with activeTabId marking the focused one.
export const layoutTree = writable<LayoutNode | null>(null)

export const isSplit = derived(layoutTree, ($tree) => $tree !== null)

// True when the focused pane is a terminal and there is another open terminal
// not yet tiled that a split could bring in beside it.
export const canSplit = derived([activeTabId, tabs, layoutTree], ([$active, $tabs, $tree]) => {
  if (!$active) return false
  const meta = $tabs[$active]
  if (!meta || meta.kind !== 'terminal') return false
  const placed = new Set(leaves($tree))
  return Object.values($tabs).some((t) => t.kind === 'terminal' && t.tabId !== $active && !placed.has(t.tabId))
})
export const layoutRects = derived(layoutTree, ($tree) => computeRects($tree))
export const visibleTabIds = derived(layoutTree, ($tree) => new Set(leaves($tree)))

// A one-leaf tree is indistinguishable from tabbed mode, so collapse it to null
// and solo that tab. Keeps the invariant: tree is null or has >= 2 leaves.
function setTree(next: LayoutNode | null): void {
  if (next && next.type === 'leaf') {
    activeTabId.set(next.tabId)
    next = null
  }
  layoutTree.set(next)
}

/** Applies a fully-built declarative workspace tree through the same invariants as manual splits. */
export function applyWorkspaceLayout(next: LayoutNode | null): void {
  setTree(next)
}

function terminalTabIds(): string[] {
  return Object.values(get(tabs))
    .filter((tab) => tab.kind === 'terminal')
    .map((tab) => tab.tabId)
}

/** One-click tile of every terminal tab across all connections into a grid. */
export function tileAllTerminals(): void {
  const ids = terminalTabIds()
  setTree(tileAll(ids))
  if (!ids.includes(get(activeTabId) ?? '') && ids.length > 0) {
    activeTabId.set(ids[0] as string)
  }
}

/** An existing terminal tab that isn't the focused one and isn't already tiled. */
function nextUnplacedTerminal(focusedTab: string, placed: Set<string>): string | null {
  const candidate = Object.values(get(tabs)).find(
    (tab) => tab.kind === 'terminal' && tab.tabId !== focusedTab && !placed.has(tab.tabId)
  )
  return candidate?.tabId ?? null
}

/**
 * Splits the focused pane in the given direction by placing an already-open
 * terminal next to it — it never opens a new console. Works from tabbed mode
 * (starts a new tree) or within an existing split. No-op if there is no other
 * terminal available to bring in.
 */
export function splitFocused(dir: SplitDir): void {
  const focusedTab = get(activeTabId)
  if (!focusedTab) return
  const meta = get(tabs)[focusedTab]
  if (!meta || meta.kind !== 'terminal') return

  const tree = get(layoutTree)
  const placed = new Set(leaves(tree))
  const bringIn = nextUnplacedTerminal(focusedTab, placed)
  if (!bringIn) return

  const base = tree && placed.has(focusedTab) ? tree : tileAll([focusedTab])
  if (!base) return
  setTree(splitLeaf(base, focusedTab, bringIn, dir))
  activeTabId.set(bringIn)
}

/** Leaves split mode, soloing the currently focused tab. */
export function closeSplit(): void {
  layoutTree.set(null)
}

/** Adjusts a divider (called live during a drag). */
export function resizeDividerBy(splitId: string, index: number, deltaFraction: number): void {
  layoutTree.update((tree) => (tree ? resizeDivider(tree, splitId, index, deltaFraction) : tree))
}

// Prune the tree whenever a tab it references disappears (closed, connection
// dropped). Decoupled from connections.ts via this subscription so there is no
// circular import. Collapsing to a single leaf returns to tabbed mode.
tabs.subscribe(($tabs) => {
  const tree = get(layoutTree)
  if (!tree) return
  const missing = leaves(tree).filter((id) => !$tabs[id])
  if (missing.length === 0) return

  let next: LayoutNode | null = tree
  for (const id of missing) {
    if (!next) break
    next = removeLeaf(next, id)
  }
  setTree(next)
})
