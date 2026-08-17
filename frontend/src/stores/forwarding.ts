import { writable } from 'svelte/store'
import { backend, on } from '../services/backend'
import { notify } from './notifications'
import type { ActiveForward, ForwardRule, ForwardRuleInput, ForwardStatusEvent } from '../types/forwarding'
import type { StatusEvent } from '../types/connection'

// Saved rules keyed by host ID.
export const forwardRules = writable<Record<number, ForwardRule[]>>({})

// Running forwards keyed by connection ID, then rule ID.
export const activeForwards = writable<Record<string, Record<number, ActiveForward>>>({})

// Host whose forwarding manager dialog is open (null = closed).
export const forwardingDialogHostId = writable<number | null>(null)

export async function loadForwardRules(hostId: number): Promise<void> {
  try {
    const rules = (await backend.listForwardRules(hostId)) ?? []
    forwardRules.update((all) => ({ ...all, [hostId]: rules }))
  } catch (e) {
    notify('error', `Could not load port forwards: ${e instanceof Error ? e.message : String(e)}`)
  }
}

function upsertRule(rule: ForwardRule): void {
  forwardRules.update((all) => {
    const list = all[rule.hostId] ?? []
    const existing = list.some((r) => r.id === rule.id)
    const next = existing ? list.map((r) => (r.id === rule.id ? rule : r)) : [...list, rule]
    return { ...all, [rule.hostId]: next }
  })
}

export async function createForwardRule(input: ForwardRuleInput): Promise<void> {
  upsertRule(await backend.createForwardRule(input))
}

export async function updateForwardRule(id: number, input: ForwardRuleInput): Promise<void> {
  upsertRule(await backend.updateForwardRule(id, input))
}

export async function deleteForwardRule(hostId: number, id: number): Promise<void> {
  await backend.deleteForwardRule(id)
  forwardRules.update((all) => ({ ...all, [hostId]: (all[hostId] ?? []).filter((r) => r.id !== id) }))
}

export async function startForward(connectionId: string, ruleId: number): Promise<void> {
  // The forward:status event fills activeForwards; surface a start failure here.
  await backend.startForward(connectionId, ruleId)
}

export async function stopForward(connectionId: string, ruleId: number): Promise<void> {
  await backend.stopForward(connectionId, ruleId)
}

export async function loadActiveForwards(connectionId: string): Promise<void> {
  try {
    const list = (await backend.listActiveForwards(connectionId)) ?? []
    const map: Record<number, ActiveForward> = {}
    for (const af of list) map[af.ruleId] = af
    activeForwards.update((all) => ({ ...all, [connectionId]: map }))
  } catch {
    // Connection may have gone away; ignore.
  }
}

function setActive(connectionId: string, af: ActiveForward): void {
  activeForwards.update((all) => ({
    ...all,
    [connectionId]: { ...(all[connectionId] ?? {}), [af.ruleId]: af }
  }))
}

function clearActive(connectionId: string, ruleId: number): void {
  activeForwards.update((all) => {
    const conn = { ...(all[connectionId] ?? {}) }
    delete conn[ruleId]
    return { ...all, [connectionId]: conn }
  })
}

function clearConnection(connectionId: string): void {
  activeForwards.update((all) => {
    if (!all[connectionId]) return all
    return { ...all, [connectionId]: {} }
  })
}

let eventsInitialized = false

/** Wires forwarding lifecycle events into the store. Call once at startup. */
export function initForwardingEvents(): void {
  if (eventsInitialized) return
  eventsInitialized = true

  on<ForwardStatusEvent>('forward:status', (evt) => {
    if (evt.active) {
      setActive(evt.connectionId, {
        ruleId: evt.ruleId,
        kind: evt.kind ?? '',
        label: evt.label ?? '',
        boundAddr: evt.boundAddr ?? ''
      })
    } else {
      clearActive(evt.connectionId, evt.ruleId)
    }
  })

  on<ForwardStatusEvent>('forward:error', (evt) => {
    if (evt.error) notify('error', `Port forward: ${evt.error}`)
  })

  // A connection that leaves the connected state drops all its forwards; the
  // backend re-emits forward:status for any it reopens after a reconnect.
  on<StatusEvent>('connection:status', (evt) => {
    if (evt.status !== 'connected') clearConnection(evt.connectionId)
  })
}
