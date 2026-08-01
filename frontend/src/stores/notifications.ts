import { writable } from 'svelte/store'

export interface AppNotification {
  id: number
  kind: 'error' | 'info'
  message: string
}

export const notifications = writable<AppNotification[]>([])

let nextId = 1

/** Shows a transient banner. Errors persist until dismissed; info auto-dismisses. */
export function notify(kind: AppNotification['kind'], message: string): void {
  const id = nextId++
  notifications.update((list) => [...list, { id, kind, message }])
  if (kind === 'info') {
    setTimeout(() => dismiss(id), 4000)
  }
}

export function dismiss(id: number): void {
  notifications.update((list) => list.filter((n) => n.id !== id))
}

/** Runs an async action, surfacing any failure as an error banner instead of
 * letting it escape as an unhandled promise rejection. */
export async function withErrorBanner(fn: () => Promise<unknown>): Promise<void> {
  try {
    await fn()
  } catch (e) {
    notify('error', e instanceof Error ? e.message : String(e))
  }
}
