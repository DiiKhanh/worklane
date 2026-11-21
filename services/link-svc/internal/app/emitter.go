package app

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/duykhanh/worklane/pkg/contracts/link"
)

const ClickPublishTimeout = 2 * time.Second

type Publisher interface {
	Publish(context.Context, string, any) error
}

type EmissionStats struct {
	Published, Overflow, Failed, Shutdown uint64
}

// Emitter has one worker and a fixed queue. Admission never waits for a broker.
type Emitter struct {
	publisher Publisher
	queue     chan link.ClickedEvent
	done      chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.RWMutex
	closed    bool
	aborted   bool
	admitted  atomic.Uint64
	published atomic.Uint64
	overflow  atomic.Uint64
	failed    atomic.Uint64
	shutdown  atomic.Uint64
}

func NewEmitter(publisher Publisher, capacity int) *Emitter {
	ctx, cancel := context.WithCancel(context.Background())
	e := &Emitter{publisher: publisher, queue: make(chan link.ClickedEvent, capacity), done: make(chan struct{}), ctx: ctx, cancel: cancel}
	go e.run()
	return e
}

func (e *Emitter) TryEmit(event link.ClickedEvent) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.closed {
		e.shutdown.Add(1)
		return false
	}
	select {
	case e.queue <- event:
		e.admitted.Add(1)
		return true
	default:
		e.overflow.Add(1)
		return false
	}
}

func (e *Emitter) run() {
	defer close(e.done)
	defer e.cancel()
	for event := range e.queue {
		if e.ctx.Err() != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(e.ctx, ClickPublishTimeout)
		err := e.publisher.Publish(ctx, "link.clicked", event)
		cancel()
		e.mu.RLock()
		if !e.aborted {
			if err != nil {
				e.failed.Add(1)
			} else {
				e.published.Add(1)
			}
		}
		e.mu.RUnlock()
	}
}

// Close drains admitted events until the caller's deadline, then cancels work.
// Publishers must honor context cancellation to release the worker promptly.
func (e *Emitter) Close(ctx context.Context) error {
	e.mu.Lock()
	if !e.closed {
		e.closed = true
		close(e.queue)
	}
	e.mu.Unlock()
	select {
	case <-e.done:
		return nil
	case <-ctx.Done():
		e.mu.Lock()
		if !e.aborted {
			e.aborted = true
			e.cancel()
			// Count queued and in-flight publications before returning, even if
			// the publisher has not yet observed cancellation. Delivery is uncertain.
			e.shutdown.Add(e.admitted.Load() - e.published.Load() - e.failed.Load())
		}
		e.mu.Unlock()
		return ctx.Err()
	}
}

func (e *Emitter) Stats() EmissionStats {
	return EmissionStats{Published: e.published.Load(), Overflow: e.overflow.Load(), Failed: e.failed.Load(), Shutdown: e.shutdown.Load()}
}
