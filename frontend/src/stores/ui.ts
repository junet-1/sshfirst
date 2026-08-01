import { writable } from 'svelte/store'

export type ThemePreference = 'system' | 'light' | 'dark'

export const DEFAULT_TERMINAL_FONT_SIZE = 13
export const MIN_TERMINAL_FONT_SIZE = 9
export const MAX_TERMINAL_FONT_SIZE = 24

export const sidebarVisible = writable(true)
export const inspectorVisible = writable(false)
export const commandPaletteOpen = writable(false)
export const settingsOpen = writable(false)
export const aboutOpen = writable(false)
export const snippetsOpen = writable(false)
export const credentialsOpen = writable(false)
export const showRecent = writable(true)
export const theme = writable<ThemePreference>('system')
export const terminalFontSize = writable(DEFAULT_TERMINAL_FONT_SIZE)
// Incremented by the global shortcut. Only the currently active terminal
// pane consumes a request, keeping each pane's search UI and query local.
export const terminalSearchRequest = writable(0)

export interface HostDialogState {
  open: boolean
  editingId: number | null
}

export const hostDialog = writable<HostDialogState>({ open: false, editingId: null })

export interface FolderDialogState {
  open: boolean
  editingId: number | null
  parentId: number | null
}

export const folderDialog = writable<FolderDialogState>({ open: false, editingId: null, parentId: null })
export const selectedHostId = writable<number | null>(null)

export function applyTheme(pref: ThemePreference): void {
  const root = document.documentElement
  if (pref === 'system') {
    root.removeAttribute('data-theme')
  } else {
    root.setAttribute('data-theme', pref)
  }
}

export function normalizeTerminalFontSize(value: number): number {
  if (!Number.isFinite(value)) return DEFAULT_TERMINAL_FONT_SIZE
  return Math.min(MAX_TERMINAL_FONT_SIZE, Math.max(MIN_TERMINAL_FONT_SIZE, Math.round(value)))
}
