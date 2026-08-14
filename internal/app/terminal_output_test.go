package app

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

func TestTerminalOutputBrokerWaitsForReadyAndAcknowledgement(t *testing.T) {
	events := make(chan TerminalDataEvent, 4)
	broker := newTerminalOutputBroker(func(event TerminalDataEvent) { events <- event })
	broker.register("tab", "generation")

	if !broker.publish("tab", "generation", []byte("first")) {
		t.Fatal("publish unexpectedly rejected")
	}
	select {
	case event := <-events:
		t.Fatalf("event dispatched before frontend readiness: %+v", event)
	default:
	}

	broker.start("tab")
	first := receiveTerminalEvent(t, events)
	if first.Data != "first" || first.Sequence != 1 || first.SessionGeneration != "generation" {
		t.Fatalf("unexpected first event: %+v", first)
	}

	if !broker.publish("tab", "generation", []byte("second")) {
		t.Fatal("second publish unexpectedly rejected")
	}
	select {
	case event := <-events:
		t.Fatalf("second event dispatched before acknowledgement: %+v", event)
	default:
	}

	broker.acknowledge("tab", "generation", first.Sequence)
	second := receiveTerminalEvent(t, events)
	if second.Data != "second" || second.Sequence != 2 {
		t.Fatalf("unexpected second event: %+v", second)
	}
}

func TestTerminalOutputBrokerBatchesInOrder(t *testing.T) {
	events := make(chan TerminalDataEvent, 8)
	broker := newTerminalOutputBroker(func(event TerminalDataEvent) { events <- event })
	broker.register("tab", "generation")

	want := bytes.Repeat([]byte("abc123"), terminalOutputBatchBytes/3)
	if !broker.publish("tab", "generation", want) {
		t.Fatal("publish unexpectedly rejected")
	}
	broker.start("tab")

	var got []byte
	for len(got) < len(want) {
		event := receiveTerminalEvent(t, events)
		if len(event.Data) > terminalOutputBatchBytes {
			t.Fatalf("batch exceeds limit: %d", len(event.Data))
		}
		got = append(got, event.Data...)
		broker.acknowledge("tab", "generation", event.Sequence)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("batched output changed byte order or contents")
	}
}

func TestTerminalOutputBrokerAppliesAndReleasesBackpressure(t *testing.T) {
	events := make(chan TerminalDataEvent, 4)
	broker := newTerminalOutputBroker(func(event TerminalDataEvent) { events <- event })
	broker.register("tab", "generation")
	broker.start("tab")

	payload := bytes.Repeat([]byte{'x'}, terminalOutputBufferBytes+1)
	done := make(chan bool, 1)
	go func() { done <- broker.publish("tab", "generation", payload) }()

	first := receiveTerminalEvent(t, events)
	select {
	case <-done:
		t.Fatal("publisher crossed the byte limit without an acknowledgement")
	case <-time.After(30 * time.Millisecond):
	}

	for {
		broker.acknowledge("tab", "generation", first.Sequence)
		select {
		case ok := <-done:
			if !ok {
				t.Fatal("publisher rejected while tab remained active")
			}
			return
		case first = <-events:
		case <-time.After(time.Second):
			t.Fatal("publisher did not resume after acknowledgements")
		}
	}
}

func TestTerminalOutputBrokerReconnectRejectsOldGeneration(t *testing.T) {
	events := make(chan TerminalDataEvent, 4)
	broker := newTerminalOutputBroker(func(event TerminalDataEvent) { events <- event })
	broker.register("tab", "old")
	broker.start("tab")
	if !broker.publish("tab", "old", []byte("old output")) {
		t.Fatal("old publish unexpectedly rejected")
	}
	oldEvent := receiveTerminalEvent(t, events)

	broker.register("tab", "new")
	if broker.publish("tab", "old", []byte("stale")) {
		t.Fatal("replaced generation was accepted")
	}
	if !broker.publish("tab", "new", []byte("new output")) {
		t.Fatal("new generation was rejected")
	}
	newEvent := receiveTerminalEvent(t, events)
	if newEvent.SessionGeneration != "new" || newEvent.Data != "new output" {
		t.Fatalf("unexpected replacement event: %+v", newEvent)
	}

	broker.acknowledge("tab", "old", oldEvent.Sequence)
	if !broker.publish("tab", "new", []byte("after stale ack")) {
		t.Fatal("publish after stale acknowledgement was rejected")
	}
	select {
	case event := <-events:
		t.Fatalf("stale acknowledgement released new generation: %+v", event)
	default:
	}
	broker.acknowledge("tab", "new", newEvent.Sequence)
	if event := receiveTerminalEvent(t, events); event.Data != "after stale ack" {
		t.Fatalf("unexpected event after valid acknowledgement: %+v", event)
	}
}

func TestTerminalOutputBrokerPauseRequeuesInflightBatch(t *testing.T) {
	events := make(chan TerminalDataEvent, 4)
	broker := newTerminalOutputBroker(func(event TerminalDataEvent) { events <- event })
	broker.register("tab", "generation")
	broker.start("tab")
	if !broker.publish("tab", "generation", []byte("preserve me")) {
		t.Fatal("publish unexpectedly rejected")
	}
	first := receiveTerminalEvent(t, events)

	broker.pause("tab")
	broker.acknowledge("tab", "generation", first.Sequence)
	select {
	case event := <-events:
		t.Fatalf("paused broker dispatched output: %+v", event)
	default:
	}

	broker.start("tab")
	replayed := receiveTerminalEvent(t, events)
	if replayed.Data != first.Data || replayed.Sequence == first.Sequence {
		t.Fatalf("inflight batch was not safely replayed: first=%+v replayed=%+v", first, replayed)
	}
}

func TestTerminalOutputBrokerSuspendReleasesPublisherAndRetainsReadiness(t *testing.T) {
	events := make(chan TerminalDataEvent, 4)
	broker := newTerminalOutputBroker(func(event TerminalDataEvent) { events <- event })
	broker.register("tab", "old")
	broker.start("tab")

	done := make(chan bool, 1)
	go func() {
		done <- broker.publish("tab", "old", bytes.Repeat([]byte{'x'}, terminalOutputBufferBytes+1))
	}()
	first := receiveTerminalEvent(t, events)
	waitForTerminalBuffer(t, broker, "tab", terminalOutputBufferBytes)

	broker.suspend("tab")
	select {
	case accepted := <-done:
		if accepted {
			t.Fatal("old publisher reported success after suspension")
		}
	case <-time.After(time.Second):
		t.Fatal("suspending generation did not release blocked publisher")
	}

	broker.register("tab", "new")
	if !broker.publish("tab", "new", []byte("replacement")) {
		t.Fatal("replacement publish unexpectedly rejected")
	}
	replacement := receiveTerminalEvent(t, events)
	if replacement.SessionGeneration != "new" || replacement.Data != "replacement" {
		t.Fatalf("replacement was not dispatched with retained readiness: old=%+v new=%+v", first, replacement)
	}
}

func TestTerminalOutputBrokerRemoveReleasesBlockedPublisher(t *testing.T) {
	broker := newTerminalOutputBroker(func(TerminalDataEvent) {})
	broker.register("tab", "generation")

	done := make(chan bool, 1)
	go func() {
		done <- broker.publish("tab", "generation", bytes.Repeat([]byte{'x'}, terminalOutputBufferBytes+1))
	}()
	waitForTerminalBuffer(t, broker, "tab", terminalOutputBufferBytes)
	broker.remove("tab")

	select {
	case accepted := <-done:
		if accepted {
			t.Fatal("publisher reported success after its tab was removed")
		}
	case <-time.After(time.Second):
		t.Fatal("removing the tab did not release the blocked publisher")
	}
}

func TestTerminalOutputBrokerConcurrentPublishersStayBounded(t *testing.T) {
	broker := newTerminalOutputBroker(func(TerminalDataEvent) {})
	broker.register("tab", "generation")

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			broker.publish("tab", "generation", bytes.Repeat([]byte{'x'}, terminalOutputBufferBytes))
		}()
	}
	waitForTerminalBuffer(t, broker, "tab", terminalOutputBufferBytes)

	broker.mu.Lock()
	buffered := broker.bufferedBytesLocked(broker.tabs["tab"])
	broker.mu.Unlock()
	if buffered > terminalOutputBufferBytes {
		t.Fatalf("buffer exceeded limit: %d", buffered)
	}

	broker.close()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("closing broker did not release concurrent publishers")
	}
}

func receiveTerminalEvent(t *testing.T, events <-chan TerminalDataEvent) TerminalDataEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for terminal event")
		return TerminalDataEvent{}
	}
}

func waitForTerminalBuffer(t *testing.T, broker *terminalOutputBroker, tabID string, size int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		broker.mu.Lock()
		state := broker.tabs[tabID]
		buffered := 0
		if state != nil {
			buffered = broker.bufferedBytesLocked(state)
		}
		broker.mu.Unlock()
		if buffered == size {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("terminal buffer did not reach expected size")
}
