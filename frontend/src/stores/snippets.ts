import { get, writable } from 'svelte/store'
import { backend, emit } from '../services/backend'
import { notify } from './notifications'
import { activeTabId } from './connections'
import { currentToolWindowKind } from '../services/windowing'
import type { Snippet, SnippetInput } from '../types/snippet'

export const snippets = writable<Snippet[]>([])

/** Loads snippets visible for the given host (0 = all). */
export async function loadSnippets(hostId = 0): Promise<void> {
  try {
    snippets.set((await backend.listSnippets(hostId)) ?? [])
  } catch (e) {
    notify('error', `Could not load snippets: ${e instanceof Error ? e.message : String(e)}`)
  }
}

export async function createSnippet(input: SnippetInput): Promise<void> {
  const snippet = await backend.createSnippet(input)
  snippets.update((list) => [...list, snippet].sort((a, b) => a.name.localeCompare(b.name)))
}

export async function updateSnippet(id: number, input: SnippetInput): Promise<void> {
  const snippet = await backend.updateSnippet(id, input)
  snippets.update((list) => list.map((s) => (s.id === id ? snippet : s)).sort((a, b) => a.name.localeCompare(b.name)))
}

export async function deleteSnippet(id: number): Promise<void> {
  await backend.deleteSnippet(id)
  snippets.update((list) => list.filter((s) => s.id !== id))
}

/** Sends a snippet's command (plus a newline to run it) to the active tab. */
export async function runSnippet(snippet: Snippet): Promise<void> {
  if (currentToolWindowKind === 'snippets') {
    await emit('snippet:run', { command: snippet.command })
    return
  }
  const tabId = get(activeTabId)
  if (!tabId) {
    notify('info', 'Open a session first to run a snippet.')
    return
  }
  try {
    await backend.sendToTab(tabId, snippet.command.endsWith('\n') ? snippet.command : snippet.command + '\n')
  } catch (e) {
    notify('error', `Could not run snippet: ${e instanceof Error ? e.message : String(e)}`)
  }
}
