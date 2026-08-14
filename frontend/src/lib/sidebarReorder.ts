export type RelativeDropPosition = 'before' | 'after'
export type TreeDropPosition = RelativeDropPosition | 'inside'

/** Maps the pointer to stable edge/inside zones without reading layout twice. */
export function treeDropPosition(
  clientY: number,
  top: number,
  height: number,
  allowInside: boolean
): TreeDropPosition {
  const ratio = height > 0 ? (clientY - top) / height : 0.5
  if (allowInside && ratio >= 0.25 && ratio <= 0.75) return 'inside'
  return ratio < 0.5 ? 'before' : 'after'
}

/** Returns the neighboring item used as the target for keyboard reordering. */
export function adjacentItem<T extends { id: number }>(items: T[], movingID: number, direction: -1 | 1): T | null {
  const index = items.findIndex((item) => item.id === movingID)
  if (index < 0) return null
  return items[index + direction] ?? null
}
