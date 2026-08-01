import { writable } from 'svelte/store'
import { backend, on } from '../services/backend'
import { notify } from './notifications'
import type { Credential, CredentialInput } from '../types/credential'

export const credentials = writable<Credential[]>([])

function byName(a: Credential, b: Credential): number {
  return a.name.localeCompare(b.name)
}

export async function loadCredentials(): Promise<void> {
  try {
    credentials.set((await backend.listCredentials()) ?? [])
  } catch (e) {
    notify('error', `Could not load credentials: ${e instanceof Error ? e.message : String(e)}`)
  }
}

export async function createCredential(input: CredentialInput): Promise<Credential> {
  const credential = await backend.createCredential(input)
  credentials.update((list) => [...list, credential].sort(byName))
  return credential
}

export async function updateCredential(id: number, input: CredentialInput): Promise<Credential> {
  const credential = await backend.updateCredential(id, input)
  credentials.update((list) => list.map((c) => (c.id === id ? credential : c)).sort(byName))
  return credential
}

export async function deleteCredential(id: number): Promise<void> {
  await backend.deleteCredential(id)
  credentials.update((list) => list.filter((c) => c.id !== id))
}

// Keeps every window's credential list in sync: the manager and the host editor
// live in separate native windows, so a change in one must reload the other.
export function initCredentialEvents(): () => void {
  return on<null>('credentials:changed', () => void loadCredentials())
}
