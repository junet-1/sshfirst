package app

import "sync"

const (
	terminalOutputBatchBytes  = 64 * 1024
	terminalOutputBufferBytes = 1024 * 1024
)

// terminalOutputBroker applies backpressure between SSH readers and the
// frontend terminal. Each tab may have at most one batch in flight; more data
// is retained in a bounded queue until the frontend acknowledges that xterm
// has parsed the previous batch.
//
// A session generation prevents a delayed acknowledgement from an old PTY
// from releasing output belonging to a replacement session after reconnect.
type terminalOutputBroker struct {
	mu       sync.Mutex
	cond     *sync.Cond
	tabs     map[string]*terminalOutputState
	dispatch func(TerminalDataEvent)
	closed   bool
}

type terminalOutputState struct {
	generation string
	ready      bool
	closed     bool

	pending      [][]byte
	pendingBytes int
	inflight     []byte
	inflightSeq  uint64
	nextSeq      uint64
}

func newTerminalOutputBroker(dispatch func(TerminalDataEvent)) *terminalOutputBroker {
	b := &terminalOutputBroker{
		tabs:     make(map[string]*terminalOutputState),
		dispatch: dispatch,
	}
	b.cond = sync.NewCond(&b.mu)
	return b
}

// register installs a fresh session generation for tabID. Readiness survives
// reconnect because the existing TerminalPane remains mounted, while blocked
// publishers from the replaced generation are released and stopped.
func (b *terminalOutputBroker) register(tabID, generation string) {
	b.mu.Lock()
	ready := false
	if previous := b.tabs[tabID]; previous != nil {
		ready = previous.ready
		previous.closed = true
	}
	b.tabs[tabID] = &terminalOutputState{
		generation: generation,
		ready:      ready,
		nextSeq:    1,
	}
	b.cond.Broadcast()
	b.mu.Unlock()
}

// publish copies data into a bounded per-tab queue. It blocks when the queue
// reaches its byte limit, propagating pressure back through the SSH channel
// instead of growing the Go heap and xterm's write buffer without bound.
// False means the tab/session was replaced or closed and the caller should
// stop its output pump.
func (b *terminalOutputBroker) publish(tabID, generation string, data []byte) bool {
	for len(data) > 0 {
		b.mu.Lock()
		state := b.tabs[tabID]
		for b.activeLocked(state, generation) && b.bufferedBytesLocked(state) >= terminalOutputBufferBytes {
			b.cond.Wait()
			state = b.tabs[tabID]
		}
		if !b.activeLocked(state, generation) {
			b.mu.Unlock()
			return false
		}

		available := terminalOutputBufferBytes - b.bufferedBytesLocked(state)
		n := len(data)
		if n > available {
			n = available
		}
		chunk := append([]byte(nil), data[:n]...)
		state.pending = append(state.pending, chunk)
		state.pendingBytes += n
		event, ok := b.nextEventLocked(tabID, state)
		b.mu.Unlock()

		if ok {
			b.dispatchEvent(event)
		}
		data = data[n:]
	}
	return true
}

// start marks the frontend terminal ready after its event listener and xterm
// instance exist. Output produced during the SSH handshake is retained until
// this call, so the initial shell prompt cannot race the component mount.
func (b *terminalOutputBroker) start(tabID string) {
	b.mu.Lock()
	state := b.tabs[tabID]
	if state == nil || state.closed || b.closed {
		b.mu.Unlock()
		return
	}
	state.ready = true
	event, ok := b.nextEventLocked(tabID, state)
	b.mu.Unlock()
	if ok {
		b.dispatchEvent(event)
	}
}

// pause stops delivery while a TerminalPane is unmounted. An unacknowledged
// batch is put back at the head of the queue so a remounted frontend can
// recover instead of leaving the SSH reader blocked forever.
func (b *terminalOutputBroker) pause(tabID string) {
	b.mu.Lock()
	state := b.tabs[tabID]
	if state != nil && !state.closed {
		state.ready = false
		if len(state.inflight) > 0 {
			state.pending = append([][]byte{state.inflight}, state.pending...)
			state.pendingBytes += len(state.inflight)
			state.inflight = nil
			state.inflightSeq = 0
		}
		b.cond.Broadcast()
	}
	b.mu.Unlock()
}

func (b *terminalOutputBroker) acknowledge(tabID, generation string, sequence uint64) {
	b.mu.Lock()
	state := b.tabs[tabID]
	if !b.activeLocked(state, generation) || state.inflightSeq != sequence || len(state.inflight) == 0 {
		b.mu.Unlock()
		return
	}
	state.inflight = nil
	state.inflightSeq = 0
	b.cond.Broadcast()
	event, ok := b.nextEventLocked(tabID, state)
	b.mu.Unlock()
	if ok {
		b.dispatchEvent(event)
	}
}

func (b *terminalOutputBroker) remove(tabID string) {
	b.mu.Lock()
	if state := b.tabs[tabID]; state != nil {
		state.closed = true
		delete(b.tabs, tabID)
		b.cond.Broadcast()
	}
	b.mu.Unlock()
}

// suspend stops the current generation but retains whether the frontend pane
// is mounted. Reconnect can then register its replacement generation without
// requiring a second StartTerminalOutput call from the still-mounted pane.
func (b *terminalOutputBroker) suspend(tabID string) {
	b.mu.Lock()
	if state := b.tabs[tabID]; state != nil {
		state.closed = true
		state.pending = nil
		state.pendingBytes = 0
		state.inflight = nil
		state.inflightSeq = 0
		b.cond.Broadcast()
	}
	b.mu.Unlock()
}

func (b *terminalOutputBroker) removeGeneration(tabID, generation string) {
	b.mu.Lock()
	if state := b.tabs[tabID]; state != nil && state.generation == generation {
		state.closed = true
		delete(b.tabs, tabID)
		b.cond.Broadcast()
	}
	b.mu.Unlock()
}

func (b *terminalOutputBroker) close() {
	b.mu.Lock()
	if !b.closed {
		b.closed = true
		for _, state := range b.tabs {
			state.closed = true
		}
		b.tabs = make(map[string]*terminalOutputState)
		b.cond.Broadcast()
	}
	b.mu.Unlock()
}

func (b *terminalOutputBroker) activeLocked(state *terminalOutputState, generation string) bool {
	return !b.closed && state != nil && !state.closed && state.generation == generation
}

func (b *terminalOutputBroker) bufferedBytesLocked(state *terminalOutputState) int {
	return state.pendingBytes + len(state.inflight)
}

func (b *terminalOutputBroker) nextEventLocked(tabID string, state *terminalOutputState) (TerminalDataEvent, bool) {
	if b.closed || state == nil || state.closed || !state.ready || len(state.inflight) > 0 || state.pendingBytes == 0 {
		return TerminalDataEvent{}, false
	}

	size := state.pendingBytes
	if size > terminalOutputBatchBytes {
		size = terminalOutputBatchBytes
	}
	batch := make([]byte, 0, size)
	for len(batch) < size {
		chunk := state.pending[0]
		remaining := size - len(batch)
		if len(chunk) <= remaining {
			batch = append(batch, chunk...)
			state.pending = state.pending[1:]
			state.pendingBytes -= len(chunk)
			continue
		}
		batch = append(batch, chunk[:remaining]...)
		state.pending[0] = chunk[remaining:]
		state.pendingBytes -= remaining
	}

	sequence := state.nextSeq
	state.nextSeq++
	state.inflight = batch
	state.inflightSeq = sequence
	return TerminalDataEvent{
		TabID:             tabID,
		SessionGeneration: state.generation,
		Sequence:          sequence,
		Data:              string(batch),
	}, true
}

func (b *terminalOutputBroker) dispatchEvent(event TerminalDataEvent) {
	if b.dispatch != nil {
		b.dispatch(event)
	}
}
