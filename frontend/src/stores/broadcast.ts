import { get, writable } from 'svelte/store'

// Broadcast / cluster mode: keystrokes typed into a member terminal are
// fanned out to every member session at once. Pure frontend concern — the
// backend just receives N writeTerminalInput calls.
export const broadcastActive = writable(false)
export const broadcastMembers = writable<Set<string>>(new Set())

/** Enables/disables broadcast. On enable, every currently open tab becomes a
 * member (the user can then exclude individual tabs). */
export function setBroadcast(active: boolean, allTabIds: string[]): void {
  if (active) {
    broadcastMembers.set(new Set(allTabIds))
  }
  broadcastActive.set(active)
}

/** Toggles a single tab's membership in the broadcast group. */
export function toggleMember(tabId: string): void {
  broadcastMembers.update((s) => {
    const next = new Set(s)
    if (next.has(tabId)) next.delete(tabId)
    else next.add(tabId)
    return next
  })
}

/** Drops members whose tabs no longer exist (called when tabs change). */
export function pruneMembers(openTabIds: Set<string>): void {
  broadcastMembers.update((s) => {
    const next = new Set([...s].filter((id) => openTabIds.has(id)))
    return next.size === s.size ? s : next
  })
}

/** Returns the tabs that input from `tabId` should be fanned out to, or null
 * if this tab isn't broadcasting (so the caller writes only to itself). */
export function broadcastTargetsFor(tabId: string): string[] | null {
  if (!get(broadcastActive)) return null
  const members = get(broadcastMembers)
  if (members.size > 1 && members.has(tabId)) return [...members]
  return null
}

export function isMember(tabId: string, members: Set<string>, active: boolean): boolean {
  return active && members.has(tabId)
}
