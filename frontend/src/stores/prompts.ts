import { writable } from 'svelte/store'
import { on } from '../services/backend'
import type {
  HostKeyPromptEvent,
  KeyboardInteractivePromptEvent,
  PasswordPromptEvent,
  PassphrasePromptEvent
} from '../types/connection'

// Queues, not single slots: several connections (e.g. two hosts opened back
// to back, or a ProxyJump chain) can each need a prompt around the same time.
export const hostKeyQueue = writable<HostKeyPromptEvent[]>([])
export const passwordQueue = writable<PasswordPromptEvent[]>([])
export const passphraseQueue = writable<PassphrasePromptEvent[]>([])
export const keyboardInteractiveQueue = writable<KeyboardInteractivePromptEvent[]>([])

export function dequeueHostKey(requestId: string): void {
  hostKeyQueue.update((q) => q.filter((e) => e.requestId !== requestId))
}

export function dequeuePassword(requestId: string): void {
  passwordQueue.update((q) => q.filter((e) => e.requestId !== requestId))
}

export function dequeuePassphrase(requestId: string): void {
  passphraseQueue.update((q) => q.filter((e) => e.requestId !== requestId))
}

export function dequeueKeyboardInteractive(requestId: string): void {
  keyboardInteractiveQueue.update((q) => q.filter((e) => e.requestId !== requestId))
}

let eventsInitialized = false

/** Wires backend auth-prompt push events into the stores. Call once at app startup. */
export function initPromptEvents(): void {
  if (eventsInitialized) return
  eventsInitialized = true

  on<HostKeyPromptEvent>('hostkey:request', (evt) => hostKeyQueue.update((q) => [...q, evt]))
  on<PasswordPromptEvent>('password:request', (evt) => passwordQueue.update((q) => [...q, evt]))
  on<PassphrasePromptEvent>('passphrase:request', (evt) => passphraseQueue.update((q) => [...q, evt]))
  on<KeyboardInteractivePromptEvent>('keyboard-interactive:request', (evt) =>
    keyboardInteractiveQueue.update((q) => [...q, evt])
  )
}
