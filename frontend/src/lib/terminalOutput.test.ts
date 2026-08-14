import { describe, expect, it, vi } from 'vitest'

import { TerminalOutputFlow } from './terminalOutput'
import type { TerminalDataEvent } from '../types/connection'

function batch(sequence: number, data: string, generation = 'generation'): TerminalDataEvent {
  return { tabId: 'tab', sessionGeneration: generation, sequence, data }
}

describe('TerminalOutputFlow', () => {
  it('acknowledges only after xterm reports the batch parsed', async () => {
    const callbacks: Array<() => void> = []
    const write = vi.fn((_data: string, parsed: () => void) => callbacks.push(parsed))
    const acknowledge = vi.fn(async () => {})
    const flow = new TerminalOutputFlow(write, acknowledge)

    flow.enqueue(batch(1, 'first'))
    expect(write).toHaveBeenCalledWith('first', expect.any(Function))
    expect(acknowledge).not.toHaveBeenCalled()

    callbacks[0]?.()
    await vi.waitFor(() => expect(acknowledge).toHaveBeenCalledWith(batch(1, 'first')))
  })

  it('keeps reconnect-overlap batches ordered until acknowledgement resolves', async () => {
    const callbacks: Array<() => void> = []
    const write = vi.fn((_data: string, parsed: () => void) => callbacks.push(parsed))
    let releaseAck: (() => void) | undefined
    const acknowledge = vi.fn(() => new Promise<void>((resolve) => (releaseAck = resolve)))
    const flow = new TerminalOutputFlow(write, acknowledge)

    flow.enqueue(batch(1, 'old', 'old'))
    flow.enqueue(batch(1, 'new', 'new'))
    expect(write).toHaveBeenCalledTimes(1)

    callbacks[0]?.()
    await vi.waitFor(() => expect(acknowledge).toHaveBeenCalledTimes(1))
    expect(write).toHaveBeenCalledTimes(1)

    releaseAck?.()
    await vi.waitFor(() => expect(write).toHaveBeenCalledTimes(2))
    expect(write.mock.calls[1]?.[0]).toBe('new')
  })

  it('does not acknowledge an in-flight write after disposal', async () => {
    let parsed: (() => void) | undefined
    const acknowledge = vi.fn(async () => {})
    const flow = new TerminalOutputFlow((_data, callback) => (parsed = callback), acknowledge)

    flow.enqueue(batch(1, 'data'))
    flow.dispose()
    parsed?.()
    await Promise.resolve()

    expect(acknowledge).not.toHaveBeenCalled()
  })

  it('stalls safely when xterm rejects a batch', () => {
    const error = new Error('write failed')
    const acknowledge = vi.fn(async () => {})
    const onError = vi.fn()
    const flow = new TerminalOutputFlow(() => {
      throw error
    }, acknowledge, onError)

    flow.enqueue(batch(1, 'data'))

    expect(onError).toHaveBeenCalledWith(error)
    expect(acknowledge).not.toHaveBeenCalled()
  })
})
