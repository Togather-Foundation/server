// Package sse provides a Server-Sent Events broker that fans River job events
// out to N connected SSE clients.
package sse

import (
	"context"
	"sync"

	"github.com/riverqueue/river"
)

// Broker fans River job events out to N connected SSE clients.
type Broker struct {
	mu      sync.RWMutex
	clients map[chan *river.Event]struct{}
}

// NewBroker creates a broker. Call Start separately.
func NewBroker() *Broker {
	return &Broker{clients: make(map[chan *river.Event]struct{})}
}

// Start begins reading from subCh and fanning events to all subscribers.
// Runs until ctx is cancelled or subCh is closed (nil event = River stopped).
// Must be called with a subCh obtained from riverClient.Subscribe BEFORE riverClient.Start.
func (b *Broker) Start(ctx context.Context, subCh <-chan *river.Event) {
	go b.run(ctx, subCh)
}

func (b *Broker) run(ctx context.Context, subCh <-chan *river.Event) {
	for {
		select {
		case event, ok := <-subCh:
			if !ok || event == nil {
				return // River stopped
			}
			b.broadcast(event)
		case <-ctx.Done():
			return
		}
	}
}

func (b *Broker) broadcast(event *river.Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- event:
		default: // drop if client slow
		}
	}
}

// Subscribe returns a channel that receives events and a cancel func.
// The cancel func removes the client and closes the channel (unblocking any reader).
// Safe to call cancel multiple times.
func (b *Broker) Subscribe() (<-chan *river.Event, func()) {
	ch := make(chan *river.Event, 16)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.clients, ch)
			b.mu.Unlock()
			close(ch) // unblocks any reader; safe: nobody can send after delete
		})
	}
	return ch, cancel
}
