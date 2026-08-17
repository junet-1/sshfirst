import { writable } from 'svelte/store'
import { backend, on } from '../services/backend'
import { notify } from './notifications'
import { openControlPanelTab } from './connections'
import type { DiscoveredForward, DiscoveredPort } from '../types/discovery'
import type { ForwardStatusEvent } from '../types/forwarding'
import type { StatusEvent } from '../types/connection'

/** Scan state for one connection. */
export interface DiscoveryState {
  scanning: boolean
  /** True once a scan has completed, so "nothing found" reads differently from "not scanned yet". */
  scanned: boolean
  ports: DiscoveredPort[]
  error: string
}

const emptyState: DiscoveryState = { scanning: false, scanned: false, ports: [], error: '' }

/** Scan results keyed by connection ID. */
export const discoveries = writable<Record<string, DiscoveryState>>({})

/** Ad-hoc tunnels opened from a scan, keyed by connection ID, then remote port. */
export const discoveredForwards = writable<Record<string, Record<number, DiscoveredForward>>>({})

function patch(connectionId: string, changes: Partial<DiscoveryState>): void {
  discoveries.update((all) => ({
    ...all,
    [connectionId]: { ...(all[connectionId] ?? emptyState), ...changes }
  }))
}

function message(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

/** Asks the host what it is listening on. Safe to call repeatedly. */
export async function scanPorts(connectionId: string): Promise<void> {
  patch(connectionId, { scanning: true, error: '' })
  try {
    const ports = (await backend.discoverPorts(connectionId)) ?? []
    patch(connectionId, { scanning: false, scanned: true, ports, error: '' })
  } catch (e) {
    patch(connectionId, { scanning: false, scanned: true, ports: [], error: message(e) })
  }
}

function rememberForward(connectionId: string, forward: DiscoveredForward): void {
  discoveredForwards.update((all) => ({
    ...all,
    [connectionId]: { ...(all[connectionId] ?? {}), [forward.port]: forward }
  }))
}

/**
 * Tunnels a discovered port to a free local port. Returns the local address, or
 * null when the tunnel could not be opened.
 */
export async function tunnelDiscoveredPort(
  connectionId: string,
  entry: DiscoveredPort
): Promise<DiscoveredForward | null> {
  try {
    const forward = await backend.forwardDiscoveredPort(connectionId, entry.port, entry.address)
    rememberForward(connectionId, forward)
    return forward
  } catch (e) {
    notify('error', `Could not forward port ${entry.port}: ${message(e)}`)
    return null
  }
}

/**
 * Opens a panel tab on the local end of an existing tunnel.
 *
 * Every tunnelled port can be opened, whether or not it was recognised as a
 * web service. The scheme guess only decides http vs https, so a service we
 * failed to identify is still one click away instead of being unreachable —
 * and a wrong guess no longer opens a useless tab on its own.
 */
export function openForwardedPort(
  hostLabel: string,
  entry: DiscoveredPort,
  forward: DiscoveredForward
): void {
  const scheme = entry.scheme || 'http'
  const title = entry.service || entry.process || `Port ${entry.port}`
  openControlPanelTab(`${hostLabel} · ${title}`, `${scheme}://${forward.localAddr}`)
}

function forgetConnection(connectionId: string): void {
  discoveries.update((all) => {
    if (!all[connectionId]) return all
    const next = { ...all }
    delete next[connectionId]
    return next
  })
  discoveredForwards.update((all) => {
    if (!all[connectionId]) return all
    const next = { ...all }
    delete next[connectionId]
    return next
  })
}

function forgetForward(connectionId: string, ruleId: number): void {
  discoveredForwards.update((all) => {
    const forwards = all[connectionId]
    if (!forwards) return all
    const match = Object.entries(forwards).find(([, forward]) => forward.ruleId === ruleId)
    if (!match) return all
    const next = { ...forwards }
    delete next[Number(match[0])]
    return { ...all, [connectionId]: next }
  })
}

let eventsInitialized = false

/** Wires discovery lifecycle events into the store. Call once at startup. */
export function initDiscoveryEvents(): void {
  if (eventsInitialized) return
  eventsInitialized = true

  // A scan describes one moment on one connection. Once that connection is
  // gone the results are stale and its ad-hoc tunnels no longer exist.
  on<StatusEvent>('connection:status', (evt) => {
    if (evt.status === 'connected') return
    forgetConnection(evt.connectionId)
  })

  // Ad-hoc forwards can also be stopped from the Inspector's forward list.
  on<ForwardStatusEvent>('forward:status', (evt) => {
    if (!evt.active && evt.ruleId < 0) forgetForward(evt.connectionId, evt.ruleId)
  })
}
