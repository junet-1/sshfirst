import { get, writable, type Unsubscriber } from 'svelte/store'
import { backend } from '../services/backend'
import { connections } from './connections'

export type ReconnectPhase = 'connecting' | 'failed' | 'waiting' | 'reconnected'

export interface ReconnectViewState {
  connectionId: string
  phase: ReconnectPhase
  attempt: number
  startedAt: number
  attemptStartedAt: number
  nextAttemptAt?: number
  timedOut?: boolean
}

export const reconnectStates = writable<Record<string, ReconnectViewState>>({})

const activeRuns = new Map<string, symbol>()
const retryDelays = [2_000, 4_000, 8_000, 16_000, 30_000]
let watcherCleanup: Unsubscriber | null = null

function isCurrentRun(connectionId: string, run: symbol): boolean {
  return activeRuns.get(connectionId) === run && Boolean(get(connections)[connectionId])
}

function updateState(connectionId: string, state: ReconnectViewState): void {
  reconnectStates.update((all) => ({ ...all, [connectionId]: state }))
}

function clearState(connectionId: string, expectedStartedAt?: number): void {
  reconnectStates.update((all) => {
    const current = all[connectionId]
    if (!current || (expectedStartedAt != null && current.startedAt !== expectedStartedAt)) return all
    const { [connectionId]: _removed, ...rest } = all
    return rest
  })
}

function wait(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms))
}

function isTimeoutError(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error)
  return /timed?\s*out|timeout|deadline exceeded/i.test(message)
}

function retryDelayFor(attempt: number): number {
  return retryDelays[Math.min(attempt - 1, retryDelays.length - 1)] ?? 30_000
}

async function runReconnectLoop(connectionId: string, run: symbol, startedAt: number): Promise<void> {
  let attempt = 1

  while (isCurrentRun(connectionId, run)) {
    const attemptStartedAt = Date.now()
    updateState(connectionId, {
      connectionId,
      phase: 'connecting',
      attempt,
      startedAt,
      attemptStartedAt
    })
    connections.update((all) => {
      const current = all[connectionId]
      if (!current) return all
      return { ...all, [connectionId]: { ...current, status: 'connecting', error: undefined } }
    })

    try {
      const info = await backend.reconnect(connectionId)
      if (!isCurrentRun(connectionId, run)) return

      connections.update((all) => {
        const current = all[connectionId]
        if (!current) return all
        return {
          ...all,
          [connectionId]: {
            ...current,
            status: 'connected',
            connectedAt: Date.now(),
            serverVersion: info.serverVersion ?? current.serverVersion,
            authMethod: info.authMethod ?? current.authMethod,
            error: undefined
          }
        }
      })
      updateState(connectionId, {
        connectionId,
        phase: 'reconnected',
        attempt,
        startedAt,
        attemptStartedAt
      })
      activeRuns.delete(connectionId)

      // Input is unblocked as soon as the phase becomes "reconnected". Keep
      // the compact success acknowledgement visible briefly, then fade it.
      window.setTimeout(() => clearState(connectionId, startedAt), 1_000)
      return
    } catch (error) {
      if (!isCurrentRun(connectionId, run)) return

      const retryDelay = retryDelayFor(attempt)
      const nextAttemptAt = Date.now() + retryDelay
      updateState(connectionId, {
        connectionId,
        phase: 'failed',
        attempt,
        startedAt,
        attemptStartedAt,
        nextAttemptAt,
        timedOut: isTimeoutError(error)
      })

      // Let the failure be legible before changing to the live countdown.
      await wait(Math.min(800, retryDelay))
      if (!isCurrentRun(connectionId, run)) return
      updateState(connectionId, {
        connectionId,
        phase: 'waiting',
        attempt,
        startedAt,
        attemptStartedAt,
        nextAttemptAt,
        timedOut: isTimeoutError(error)
      })
      await wait(Math.max(0, nextAttemptAt - Date.now()))
      attempt += 1
    }
  }
}

function startReconnect(connectionId: string): void {
  if (activeRuns.has(connectionId) || !get(connections)[connectionId]) return
  const run = Symbol(connectionId)
  const startedAt = Date.now()
  activeRuns.set(connectionId, run)
  void runReconnectLoop(connectionId, run, startedAt)
}

/** Starts an immediate attempt and lets the controller own all later retries. */
export async function reconnectConnection(connectionId: string): Promise<void> {
  startReconnect(connectionId)
}

/**
 * Watches connection lifecycle changes once at app startup. A keepalive error
 * starts the same retry controller used by the explicit Reconnect action.
 */
export function initReconnectEvents(): void {
  if (watcherCleanup) return
  let previousStatuses: Record<string, string> = {}

  watcherCleanup = connections.subscribe((all) => {
    for (const connectionId of activeRuns.keys()) {
      if (!all[connectionId]) {
        activeRuns.delete(connectionId)
        clearState(connectionId)
      }
    }

    for (const connection of Object.values(all)) {
      if (
        connection.status === 'error' &&
        previousStatuses[connection.connectionId] !== 'error' &&
        connection.tabOrder.length > 0
      ) {
        startReconnect(connection.connectionId)
      }
    }

    previousStatuses = Object.fromEntries(Object.values(all).map((connection) => [connection.connectionId, connection.status]))
  })
}
