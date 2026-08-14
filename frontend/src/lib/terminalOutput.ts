import type { TerminalDataEvent } from '../types/connection'

export type TerminalWrite = (data: string, parsed: () => void) => void
export type TerminalAcknowledge = (event: TerminalDataEvent) => Promise<void>

// Serializes the small number of batches that can overlap while a backend
// session generation is being replaced. The normal steady state has exactly
// one entry: the backend sends another batch only after acknowledge resolves.
export class TerminalOutputFlow {
  private readonly queue: TerminalDataEvent[] = []
  private writing = false
  private disposed = false

  constructor(
    private readonly write: TerminalWrite,
    private readonly acknowledge: TerminalAcknowledge,
    private readonly onError: (error: unknown) => void = () => {}
  ) {}

  enqueue(event: TerminalDataEvent): void {
    if (this.disposed) return
    this.queue.push(event)
    this.writeNext()
  }

  dispose(): void {
    this.disposed = true
    this.queue.length = 0
  }

  private writeNext(): void {
    if (this.disposed || this.writing) return
    const event = this.queue.shift()
    if (!event) return

    this.writing = true
    let completed = false
    const parsed = () => {
      if (completed) return
      completed = true
      void this.finish(event)
    }
    try {
      this.write(event.data, parsed)
    } catch (error) {
      this.onError(error)
      // Do not acknowledge data xterm did not parse. The backend remains
      // bounded and applies pressure to SSH instead of silently losing bytes.
    }
  }

  private async finish(event: TerminalDataEvent): Promise<void> {
    if (this.disposed) return
    try {
      await this.acknowledge(event)
    } catch (error) {
      this.onError(error)
      // Keep writing=true: without a successful ACK the backend deliberately
      // must not release another batch.
      return
    }
    if (this.disposed) return
    this.writing = false
    this.writeNext()
  }
}
