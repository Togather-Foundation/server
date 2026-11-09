package geoloc

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestBudget_AllowExhaustsLookups(t *testing.T) {
	b := NewBudget(2, 0)
	if !b.Allow() {
		t.Fatal("first lookup should be allowed")
	}
	if !b.Allow() {
		t.Fatal("second lookup should be allowed")
	}
	if b.Allow() {
		t.Fatal("third lookup should be refused")
	}
}

func TestBudget_ZeroLookupsRefusesEverything(t *testing.T) {
	b := NewBudget(0, 0)
	if b.Allow() {
		t.Fatal("a zero-lookup budget must refuse every lookup")
	}
}

func TestBudget_NegativeLookupsIsUnlimited(t *testing.T) {
	b := NewBudget(-1, 0)
	for i := 0; i < 100; i++ {
		if !b.Allow() {
			t.Fatalf("lookup %d should be allowed with an unlimited budget", i)
		}
	}
}

func TestBudget_WindowExpiry(t *testing.T) {
	b := NewBudget(100, time.Nanosecond)
	time.Sleep(time.Millisecond)
	if b.Allow() {
		t.Fatal("an expired window must refuse lookups")
	}
}

func TestBudget_Deadline(t *testing.T) {
	var nilBudget *Budget
	if _, ok := nilBudget.Deadline(); ok {
		t.Error("a nil budget has no deadline")
	}
	if _, ok := NewBudget(1, 0).Deadline(); ok {
		t.Error("an unbounded window has no deadline")
	}

	b := NewBudget(1, time.Minute)
	deadline, ok := b.Deadline()
	if !ok {
		t.Fatal("a bounded budget must report a deadline")
	}
	if time.Until(deadline) <= 0 || time.Until(deadline) > time.Minute {
		t.Errorf("deadline = %v, want within (now, now+1m]", deadline)
	}
}

func TestBudget_Stats(t *testing.T) {
	b := NewBudget(10, 0)
	b.RecordResolved()
	b.RecordResolved()
	b.RecordUnresolved()
	b.RecordSkipped()

	got := b.Stats()
	want := BatchStats{Resolved: 2, Unresolved: 1, Skipped: 1}
	if got != want {
		t.Errorf("Stats() = %+v, want %+v", got, want)
	}
}

func TestBudget_NilSafe(t *testing.T) {
	var b *Budget
	if !b.Allow() {
		t.Error("a nil budget must allow lookups")
	}
	b.RecordResolved()
	b.RecordUnresolved()
	b.RecordSkipped()
	if got := b.Stats(); got != (BatchStats{}) {
		t.Errorf("nil budget Stats() = %+v, want zero", got)
	}
}

func TestBudget_ContextRoundTrip(t *testing.T) {
	if got := BudgetFromContext(context.Background()); got != nil {
		t.Fatalf("BudgetFromContext(background) = %v, want nil", got)
	}
	//nolint:staticcheck // intentionally exercising the nil-context guard
	if got := BudgetFromContext(nil); got != nil {
		t.Fatalf("BudgetFromContext(nil) = %v, want nil", got)
	}

	b := NewBudget(1, 0)
	ctx := WithBudget(context.Background(), b)
	if got := BudgetFromContext(ctx); got != b {
		t.Fatalf("BudgetFromContext = %p, want %p", got, b)
	}

	if got := WithBudget(ctx, nil); got != ctx {
		t.Fatal("WithBudget(ctx, nil) must return ctx unchanged")
	}
}

func TestBudget_ConcurrentReservationsRespectCap(t *testing.T) {
	const maxLookups = 7
	b := NewBudget(maxLookups, 0)

	var wg sync.WaitGroup
	allowed := make(chan struct{}, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if b.Allow() {
				allowed <- struct{}{}
			}
		}()
	}
	wg.Wait()
	close(allowed)

	count := 0
	for range allowed {
		count++
	}
	if count != maxLookups {
		t.Errorf("allowed %d lookups, want exactly %d", count, maxLookups)
	}
}
