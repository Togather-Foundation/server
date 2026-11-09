package geoloc

import (
	"context"
	"sync"
	"time"
)

// BatchStats counts the outcomes of boundary-resolver lookups performed under a
// Budget. A lookup that is refused because the budget is exhausted counts as
// Skipped and is never attempted.
type BatchStats struct {
	// Resolved is the number of events the resolver could place inside the
	// boundary (and that were therefore accepted).
	Resolved int
	// Unresolved is the number of lookups that ran but did not produce an
	// accepted location: no result, a resolver error, or a resolved address
	// that is still outside the boundary.
	Unresolved int
	// Skipped is the number of events that were not looked up because the
	// budget (lookup count or wall-clock window) was already exhausted.
	Skipped int
}

// Budget bounds the work a single batch may spend on boundary-resolver lookups.
//
// A boundary geocoder asks an external service, so an unbounded batch can
// outlive the scraper's poll window. A Budget caps both the number of lookups
// and the wall-clock time spent looking up, letting the caller keep the offline
// decision for the rest of the batch instead of timing out mid-run.
//
// Budgets are carried in a context (WithBudget/BudgetFromContext) so every batch
// job gets its own counters and concurrent batches never share state. All
// methods are safe for concurrent use, and a nil *Budget is a no-op ("no
// budget"): Allow reports true and the Record methods do nothing.
type Budget struct {
	mu sync.Mutex

	// maxLookups is the hard cap on lookups. A value >= 0 caps lookups; a
	// negative value means "unlimited lookups" (still bounded by the window).
	maxLookups int
	// deadline is the end of the wall-clock window. Zero means no window.
	deadline time.Time
	// lookups is the number of lookups already reserved by Allow.
	lookups int
	stats   BatchStats
}

// budgetKey is the unexported context key for a *Budget. Using a private struct
// type prevents collisions with other packages' context values.
type budgetKey struct{}

// NewBudget builds a budget allowing maxLookups lookups within maxDuration.
// A non-positive maxDuration means "no wall-clock window". A negative
// maxLookups means "no lookup cap" (bounded only by the window).
func NewBudget(maxLookups int, maxDuration time.Duration) *Budget {
	b := &Budget{maxLookups: maxLookups}
	if maxDuration > 0 {
		b.deadline = time.Now().Add(maxDuration)
	}
	return b
}

// WithBudget returns a context carrying b. A nil budget returns ctx unchanged.
func WithBudget(ctx context.Context, b *Budget) context.Context {
	if b == nil || ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, budgetKey{}, b)
}

// BudgetFromContext returns the budget carried by ctx, or nil when ctx carries
// none (or ctx is nil).
func BudgetFromContext(ctx context.Context) *Budget {
	if ctx == nil {
		return nil
	}
	b, _ := ctx.Value(budgetKey{}).(*Budget)
	return b
}

// Allow reserves one lookup and reports whether it may proceed. It returns
// false once the lookup count or the wall-clock window is exhausted, in which
// case the caller must keep its existing (offline) decision and, typically,
// call RecordSkipped.
//
// Allow reserves the lookup when it returns true, so concurrent callers can
// never exceed the cap.
func (b *Budget) Allow() bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.maxLookups >= 0 && b.lookups >= b.maxLookups {
		return false
	}
	if !b.deadline.IsZero() && !time.Now().Before(b.deadline) {
		return false
	}
	b.lookups++
	return true
}

// Deadline reports when the wall-clock window ends. ok is false when the budget
// has no window. Callers use it to bound an in-flight lookup: checking the
// window only before starting would let a request started just before the
// deadline keep running past it.
func (b *Budget) Deadline() (deadline time.Time, ok bool) {
	if b == nil {
		return time.Time{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.deadline.IsZero() {
		return time.Time{}, false
	}
	return b.deadline, true
}

// RecordResolved counts a lookup that placed the event inside the boundary.
func (b *Budget) RecordResolved() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.stats.Resolved++
	b.mu.Unlock()
}

// RecordUnresolved counts a lookup that ran but did not produce an accepted
// location (no result, resolver error, or still outside the boundary).
func (b *Budget) RecordUnresolved() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.stats.Unresolved++
	b.mu.Unlock()
}

// RecordSkipped counts an event that was not looked up because the budget was
// exhausted.
func (b *Budget) RecordSkipped() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.stats.Skipped++
	b.mu.Unlock()
}

// Stats returns a snapshot of the counters.
func (b *Budget) Stats() BatchStats {
	if b == nil {
		return BatchStats{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stats
}
