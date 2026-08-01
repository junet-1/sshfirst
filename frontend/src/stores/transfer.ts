import { get, writable } from 'svelte/store'
import { backend, on } from '../services/backend'
import type { TransferDoneEvent, TransferOutputEvent, TransferRequest } from '../types/transfer'

export type TransferState = 'running' | 'done' | 'error'

export interface TransferView {
  id: string
  hostLabel: string
  upload: boolean
  state: TransferState
  lines: string[] // committed output (newline-terminated segments)
  progress: string // current transient progress line (carriage-return updates)
  error?: string
}

// The dialog only follows one transfer at a time (the last one started); the
// map keeps any others alive so their events aren't lost.
export const transfers = writable<Record<string, TransferView>>({})
export const activeTransferId = writable<string | null>(null)

/** Which host the transfer dialog is currently configured for (null = closed). */
export const transferDialogHostId = writable<number | null>(null)

export async function startTransfer(req: TransferRequest, hostLabel: string): Promise<void> {
  const id = await backend.startRsync(req)
  transfers.update((all) => ({
    ...all,
    [id]: { id, hostLabel, upload: req.upload, state: 'running', lines: [], progress: '' }
  }))
  activeTransferId.set(id)
}

export async function cancelTransfer(id: string): Promise<void> {
  await backend.cancelRsync(id)
}

let eventsInitialized = false

/** Wires rsync output/completion events into the store. Call once at startup. */
export function initTransferEvents(): void {
  if (eventsInitialized) return
  eventsInitialized = true

  on<TransferOutputEvent>('transfer:output', (evt) => {
    transfers.update((all) => {
      const t = all[evt.transferId]
      if (!t) return all
      if (evt.transient) {
        return { ...all, [evt.transferId]: { ...t, progress: evt.text } }
      }
      // A committed line: also clears the transient progress line.
      return { ...all, [evt.transferId]: { ...t, lines: [...t.lines, evt.text], progress: '' } }
    })
  })

  on<TransferDoneEvent>('transfer:done', (evt) => {
    transfers.update((all) => {
      const t = all[evt.transferId]
      if (!t) return all
      return {
        ...all,
        [evt.transferId]: { ...t, state: evt.success ? 'done' : 'error', error: evt.error, progress: '' }
      }
    })
  })
}

export function isRunning(id: string | null): boolean {
  if (!id) return false
  return get(transfers)[id]?.state === 'running'
}
