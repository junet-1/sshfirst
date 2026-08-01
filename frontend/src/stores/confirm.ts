import { writable } from 'svelte/store'

export interface ConfirmRequest {
  title: string
  message: string
  confirmLabel: string
  cancelLabel: string
  resolve: (ok: boolean) => void
}

export const confirmRequest = writable<ConfirmRequest | null>(null)

/**
 * Shows an in-app confirmation modal (rendered inside the main window's webview,
 * so it always appears on the right monitor and follows the app theme) and
 * resolves to the user's choice. Drop-in replacement for the old native
 * ConfirmDialog: same (title, message, confirmLabel, cancelLabel) signature.
 */
export function confirmDialog(
  title: string,
  message: string,
  confirmLabel: string,
  cancelLabel: string
): Promise<boolean> {
  return new Promise((resolve) => {
    confirmRequest.update((current) => {
      // Only one confirm at a time; treat any prior request as declined.
      current?.resolve(false)
      return { title, message, confirmLabel, cancelLabel, resolve }
    })
  })
}
