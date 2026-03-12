package sse

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// makeEvent builds a minimal *river.Event for test use.
func makeEvent(kind river.EventKind, jobID int64, jobKind string) *river.Event {
	return &river.Event{
		Kind: kind,
		Job: &rivertype.JobRow{
			ID:   jobID,
			Kind: jobKind,
		},
	}
}

// receiveWithTimeout reads one event from ch within the given deadline.
// Returns (event, true) on success, (nil, false) on timeout.
func receiveWithTimeout(ch <-chan *river.Event, d time.Duration) (*river.Event, bool) {
	select {
	case ev := <-ch:
		return ev, true
	case <-time.After(d):
		return nil, false
	}
}

// TestBroker_Subscribe_ReceivesEvents: send an event on subCh, assert subscriber receives it.
func TestBroker_Subscribe_ReceivesEvents(t *testing.T) {
	subCh := make(chan *river.Event, 10)
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.Start(ctx, subCh)

	ch, unsub := b.Subscribe()
	defer unsub()

	ev := makeEvent(river.EventKindJobCompleted, 1, "scrape_source")
	subCh <- ev

	got, ok := receiveWithTimeout(ch, time.Second)
	if !ok {
		t.Fatal("timed out waiting for event")
	}
	if got.Job.ID != 1 {
		t.Errorf("got job ID %d, want 1", got.Job.ID)
	}
}

// TestBroker_Subscribe_FanOut: two subscribers both receive the same event.
func TestBroker_Subscribe_FanOut(t *testing.T) {
	subCh := make(chan *river.Event, 10)
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.Start(ctx, subCh)

	ch1, unsub1 := b.Subscribe()
	defer unsub1()
	ch2, unsub2 := b.Subscribe()
	defer unsub2()

	ev := makeEvent(river.EventKindJobCompleted, 42, "scrape_source")
	subCh <- ev

	got1, ok1 := receiveWithTimeout(ch1, time.Second)
	got2, ok2 := receiveWithTimeout(ch2, time.Second)

	if !ok1 {
		t.Error("subscriber 1 did not receive event")
	}
	if !ok2 {
		t.Error("subscriber 2 did not receive event")
	}
	if ok1 && got1.Job.ID != 42 {
		t.Errorf("subscriber 1: got job ID %d, want 42", got1.Job.ID)
	}
	if ok2 && got2.Job.ID != 42 {
		t.Errorf("subscriber 2: got job ID %d, want 42", got2.Job.ID)
	}
}

// TestBroker_Unsubscribe_ChannelClosed: cancel() closes the channel.
func TestBroker_Unsubscribe_ChannelClosed(t *testing.T) {
	b := NewBroker()

	ch, cancel := b.Subscribe()
	cancel()

	// After cancel, the channel should be closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed (ok=false)")
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for channel to be closed")
	}
}

// TestBroker_Unsubscribe_NoLongerReceivesEvents: after cancel(), no events delivered.
func TestBroker_Unsubscribe_NoLongerReceivesEvents(t *testing.T) {
	subCh := make(chan *river.Event, 10)
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.Start(ctx, subCh)

	ch, unsub := b.Subscribe()
	unsub() // cancel immediately

	// Drain any events that may have been buffered before cancel
	// Ensure channel is closed (already tested above)
	// Now send an event — it must NOT appear on ch (closed channel panics on send, but broker should not send to it)
	ev := makeEvent(river.EventKindJobCompleted, 99, "scrape_source")
	subCh <- ev

	// Give some time for broker goroutine to process
	time.Sleep(50 * time.Millisecond)

	// Channel is closed, reading from it should return zero value immediately
	select {
	case v, ok := <-ch:
		if ok {
			t.Errorf("received unexpected event after cancel: %v", v)
		}
		// ok=false means channel closed — acceptable
	default:
		// nothing in channel — also acceptable (broker dropped it)
	}
}

// TestBroker_CancelIdempotent: calling cancel() twice does not panic.
func TestBroker_CancelIdempotent(t *testing.T) {
	b := NewBroker()
	_, cancel := b.Subscribe()

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("cancel() panicked: %v", r)
		}
	}()

	cancel()
	cancel()
}

// TestBroker_SlowConsumer_DropsEvent: subscriber with full buffer (cap=1) does not block broadcast.
func TestBroker_SlowConsumer_DropsEvent(t *testing.T) {
	subCh := make(chan *river.Event, 10)
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.Start(ctx, subCh)

	// Create a slow consumer with buffer size 1 and fill it manually.
	// We do this by subscribing (which gives us a buf-16 chan) — but we need to
	// test the drop path. Instead, we directly manipulate: subscribe normally,
	// then fill the buffer by subscribing and pre-filling to capacity.
	// The default subscriber channel has cap=16. To test the drop path cleanly,
	// we use a separate goroutine: subscribe, fill the buffer, and then send
	// more events. The broadcast must not block.
	_, unsub := b.Subscribe()
	defer unsub()

	// Fill the subscriber's buffer completely by draining through subCh.
	// We need to prevent reads while filling.  Send 16 events first to saturate.
	for i := 0; i < 16; i++ {
		subCh <- makeEvent(river.EventKindJobCompleted, int64(i), "scrape_source")
	}

	// Give broker goroutine time to process those events (fill ch buffer)
	time.Sleep(50 * time.Millisecond)

	// Now the buffer is full. Send one more — broadcast must not block.
	done := make(chan struct{})
	go func() {
		subCh <- makeEvent(river.EventKindJobCompleted, 999, "scrape_source")
		// Give broker time to attempt broadcast
		time.Sleep(50 * time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
		// broadcast completed without blocking — pass
	case <-time.After(2 * time.Second):
		t.Error("broadcast blocked on slow consumer")
	}
}

// TestBroker_ShutdownOnNilEvent: sending nil on subCh stops the broker run loop.
func TestBroker_ShutdownOnNilEvent(t *testing.T) {
	subCh := make(chan *river.Event, 10)
	b := NewBroker()
	ctx := context.Background()

	b.Start(ctx, subCh)

	// Subscribe so we can verify events stopped arriving after nil
	ch, unsub := b.Subscribe()
	defer unsub()

	// Send a real event first, verify it arrives
	subCh <- makeEvent(river.EventKindJobCompleted, 1, "scrape_source")
	_, ok := receiveWithTimeout(ch, time.Second)
	if !ok {
		t.Fatal("event before nil not received")
	}

	// Send nil to signal shutdown
	subCh <- nil

	// Give broker goroutine time to exit
	time.Sleep(50 * time.Millisecond)

	// Send another event — broker goroutine has stopped, so it won't be forwarded
	// (The event will sit in subCh unread — that's fine)
	subCh <- makeEvent(river.EventKindJobCompleted, 2, "scrape_source")

	// The subscriber should NOT receive this event (broker stopped)
	_, got := receiveWithTimeout(ch, 100*time.Millisecond)
	if got {
		t.Error("received event after nil shutdown signal — broker did not stop")
	}
}

// TestBroker_ShutdownOnCtxCancel: cancelling ctx stops the broker run loop.
func TestBroker_ShutdownOnCtxCancel(t *testing.T) {
	subCh := make(chan *river.Event, 10)
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())

	b.Start(ctx, subCh)

	ch, unsub := b.Subscribe()
	defer unsub()

	// Send a real event first
	subCh <- makeEvent(river.EventKindJobCompleted, 1, "scrape_source")
	_, ok := receiveWithTimeout(ch, time.Second)
	if !ok {
		t.Fatal("event before ctx cancel not received")
	}

	// Cancel context
	cancel()

	// Give broker goroutine time to exit
	time.Sleep(50 * time.Millisecond)

	// Send another event — should NOT be forwarded
	subCh <- makeEvent(river.EventKindJobCompleted, 2, "scrape_source")

	_, got := receiveWithTimeout(ch, 100*time.Millisecond)
	if got {
		t.Error("received event after context cancel — broker did not stop")
	}
}
