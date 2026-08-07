export type TabDropPosition = 'before' | 'after'

/** Returns a new order with source inserted on the requested side of target. */
export function reorderTabIds(
  ids: string[],
  sourceID: string,
  targetID: string,
  position: TabDropPosition
): string[] {
  if (sourceID === targetID || !ids.includes(sourceID) || !ids.includes(targetID)) return [...ids]
  const next = ids.filter((id) => id !== sourceID)
  const targetIndex = next.indexOf(targetID)
  next.splice(targetIndex + (position === 'after' ? 1 : 0), 0, sourceID)
  return next
}
