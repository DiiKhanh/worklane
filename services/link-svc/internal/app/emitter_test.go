package app_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/duykhanh/worklane/pkg/contracts/link"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
)

type waitingPublisher struct{ started, release chan struct{} }

func (p waitingPublisher) Publish(ctx context.Context, _ string, _ any) error {
	close(p.started)
	<-ctx.Done()
	<-p.release
	return ctx.Err()
}

func TestEmitterBoundsAdmissionAndShutdownDuringKafkaOutage(t *testing.T) {
	publisher := waitingPublisher{started: make(chan struct{}), release: make(chan struct{})}
	defer close(publisher.release)
	emitter := app.NewEmitter(publisher, 1)
	if !emitter.TryEmit(link.ClickedEvent{Code: "G"}) {
		t.Fatal("first event rejected")
	}
	<-publisher.started
	if !emitter.TryEmit(link.ClickedEvent{Code: "H"}) || emitter.TryEmit(link.ClickedEvent{Code: "I"}) {
		t.Fatal("queue must admit exactly its remaining capacity")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := emitter.Close(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("bounded shutdown: %v", err)
	}
	if stats := emitter.Stats(); stats.Shutdown != 2 {
		t.Fatalf("shutdown returned before accounting for admitted events: %+v", stats)
	}
	if emitter.TryEmit(link.ClickedEvent{Code: "J"}) {
		t.Fatal("admitted an event after shutdown")
	}
	drain, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	// Let the publisher complete only after checking deadline-time accounting.
	publisher.release <- struct{}{}
	if err := emitter.Close(drain); err != nil {
		t.Fatalf("worker did not exit after cancellation: %v", err)
	}
	stats := emitter.Stats()
	if stats.Published != 0 || stats.Overflow != 1 || stats.Failed != 0 || stats.Shutdown != 3 {
		t.Fatalf("incorrect loss accounting: %+v", stats)
	}
}

type failingPublisher struct{}

func (failingPublisher) Publish(context.Context, string, any) error {
	return errors.New("sensitive broker details must never be logged")
}

func TestEmitterReportsPublicationFailuresAndConcurrentShutdown(t *testing.T) {
	emitter := app.NewEmitter(failingPublisher{}, 16)
	if !emitter.TryEmit(link.ClickedEvent{Code: "G"}) {
		t.Fatal("first event rejected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := emitter.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if stats := emitter.Stats(); stats.Failed != 1 || stats.Published != 0 {
		t.Fatalf("publication failure was not counted: %+v", stats)
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			emitter.TryEmit(link.ClickedEvent{Code: "G"})
			if err := emitter.Close(ctx); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if stats := emitter.Stats(); stats.Shutdown != 16 {
		t.Fatalf("post-shutdown admissions were not counted: %+v", stats)
	}
}
