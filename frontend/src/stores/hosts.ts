import { derived, get, writable } from 'svelte/store'
import { backend } from '../services/backend'
import { t } from '../services/i18n'
import { confirmDialog } from './confirm'
import { notify } from './notifications'
import { hostToInput, type Folder, type Host, type HostInput } from '../types/host'

export const hosts = writable<Host[]>([])
export const folders = writable<Folder[]>([])
export const searchQuery = writable('')
export const hostsLoading = writable(false)
export const hostsError = writable<string | null>(null)

export function byHostLabel(a: Host, b: Host): number {
  return a.label.localeCompare(b.label)
}

export function byHostOrder(a: Host, b: Host): number {
  return a.sortOrder - b.sortOrder || byHostLabel(a, b) || a.id - b.id
}

export function byFolderOrder(a: Folder, b: Folder): number {
  return a.sortOrder - b.sortOrder || a.name.localeCompare(b.name) || a.id - b.id
}

export async function loadHosts(): Promise<void> {
  hostsLoading.set(true)
  hostsError.set(null)
  try {
    const [hostList, folderList] = await Promise.all([backend.listHosts(), backend.listFolders()])
    hosts.set(hostList ?? [])
    folders.set(folderList ?? [])
  } catch (e) {
    hostsError.set(String(e))
  } finally {
    hostsLoading.set(false)
  }
}

export async function createHost(input: HostInput): Promise<Host> {
  const host = await backend.createHost(input)
  hosts.update((list) => [...list, host])
  return host
}

// Picks "<base> (2)", "(3)", … — the smallest suffix not already taken by an
// existing host label, so duplicating twice yields (2) then (3) rather than
// colliding. A bare "<base> (2)" style also reads better than "copy of".
function nextDuplicateLabel(baseLabel: string): string {
  // Strip an existing " (n)" suffix so duplicating a duplicate stays tidy.
  const base = baseLabel.replace(/ \(\d+\)$/, '')
  const taken = new Set(get(hosts).map((h) => h.label))
  let n = 2
  while (taken.has(`${base} (${n})`)) n++
  return `${base} (${n})`
}

// Duplicates a host's settings under a new auto-numbered label. Secrets are NOT
// copied (they live in the Secret Service keyed by host ID); the copy is meant
// as a starting point whose credentials the user then edits.
export async function duplicateHost(id: number): Promise<Host | null> {
  const source = get(hosts).find((h) => h.id === id)
  if (!source) return null
  const input = hostToInput(source)
  input.label = nextDuplicateLabel(source.label)
  return createHost(input)
}

export async function updateHost(id: number, input: HostInput): Promise<Host> {
  const host = await backend.updateHost(id, input)
  hosts.update((list) => list.map((h) => (h.id === id ? host : h)))
  return host
}

export async function deleteHost(id: number): Promise<void> {
  await backend.deleteHost(id)
  hosts.update((list) => list.filter((h) => h.id !== id))
}

/** Shows the delete confirmation dialog and deletes the host if accepted. */
export async function confirmAndDeleteHost(host: Host): Promise<void> {
  const translate = get(t)
  const ok = await confirmDialog(
    translate('confirmDelete.title'),
    translate('confirmDelete.body', { label: host.label }),
    translate('confirmDelete.confirm'),
    translate('confirmDelete.cancel')
  )
  if (ok) await deleteHost(host.id)
}

export async function setFavorite(id: number, favorite: boolean): Promise<void> {
  await backend.setFavorite(id, favorite)
  hosts.update((list) => list.map((h) => (h.id === id ? { ...h, favorite } : h)))
}

export async function touchLastUsedLocally(id: number, whenIso: string): Promise<void> {
  hosts.update((list) => list.map((h) => (h.id === id ? { ...h, lastUsedAt: whenIso } : h)))
}

export async function importSSHConfig(): Promise<number> {
  const result = await backend.importSSHConfig()
  await loadHosts()
  return result.importedCount
}

/** Import entry point for UI actions: runs the import and surfaces success or
 * failure as a banner, never rejecting. */
export async function importSSHConfigWithFeedback(): Promise<void> {
  try {
    const count = await importSSHConfig()
    notify('info', count === 0 ? 'No hosts found in ~/.ssh/config.' : `Imported ${count} host${count === 1 ? '' : 's'}.`)
  } catch (e) {
    notify('error', e instanceof Error ? e.message : String(e))
  }
}

export async function createFolder(name: string, parentId: number | null, icon = 'folder'): Promise<Folder> {
  const folder = await backend.createFolder(name, parentId, icon)
  folders.update((list) => [...list, folder])
  return folder
}

export async function updateFolder(id: number, name: string, icon: string): Promise<Folder> {
  const folder = await backend.updateFolder(id, name, icon)
  folders.update((list) => list.map((item) => (item.id === id ? folder : item)))
  return folder
}

export async function renameFolder(id: number, name: string): Promise<void> {
  await backend.renameFolder(id, name)
  folders.update((list) => list.map((f) => (f.id === id ? { ...f, name } : f)))
}

export async function deleteFolder(id: number): Promise<void> {
  await backend.deleteFolder(id)
  folders.update((list) => list.filter((f) => f.id !== id))
  hosts.update((list) => list.map((h) => (h.folderId === id ? { ...h, folderId: undefined } : h)))
}

export async function moveFolder(id: number, parentId: number | null): Promise<void> {
  await backend.moveFolder(id, parentId)
  folders.set((await backend.listFolders()) ?? [])
}

export async function moveHostToFolder(hostId: number, folderId: number | null): Promise<void> {
  await backend.moveHostToFolder(hostId, folderId)
  hosts.set((await backend.listHosts()) ?? [])
}

export async function reorderHost(
  hostId: number,
  folderId: number | null,
  targetHostId: number | null,
  before: boolean
): Promise<void> {
  await backend.reorderHost(hostId, folderId, targetHostId, before)
  hosts.set((await backend.listHosts()) ?? [])
}

export async function reorderFolder(
  folderId: number,
  parentId: number | null,
  targetFolderId: number | null,
  before: boolean
): Promise<void> {
  await backend.reorderFolder(folderId, parentId, targetFolderId, before)
  folders.set((await backend.listFolders()) ?? [])
}

export const filteredHosts = derived([hosts, searchQuery], ([$hosts, $query]) => {
  const q = $query.trim().toLowerCase()
  if (!q) return $hosts
  return $hosts.filter(
    (h) =>
      h.label.toLowerCase().includes(q) ||
      h.hostname.toLowerCase().includes(q) ||
      h.user.toLowerCase().includes(q) ||
      (h.tags ?? []).some((tag) => tag.toLowerCase().includes(q))
  )
})

export const favoriteHosts = derived(filteredHosts, ($hosts) => $hosts.filter((h) => h.favorite).sort(byHostLabel))

export const recentHosts = derived(filteredHosts, ($hosts) =>
  $hosts
    .filter((h) => h.lastUsedAt)
    .sort((a, b) => ((b.lastUsedAt ?? '') > (a.lastUsedAt ?? '') ? 1 : -1))
    .slice(0, 8)
)

export const allTags = derived(hosts, ($hosts) => {
  const set = new Set<string>()
  for (const h of $hosts) {
    for (const tag of h.tags ?? []) set.add(tag)
  }
  return [...set].sort()
})
