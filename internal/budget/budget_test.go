package budget

import (
	"context"
	"sync"
	"testing"
	"time"
)

// newTestBudget wires a budget to a real cancellable context, which is the
// arrangement every test here needs.
func newTestBudget(t *testing.T, ceiling int) (*Budget, context.Context) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return NewBudget(ceiling, cancel), ctx
}

// isDone reports whether ctx has been cancelled, without blocking.
func isDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// Spending below the ceiling must leave the run alone. A budget that cancels
// early is worse than one that cancels late.
func TestUnderCeilingDoesNotCancel(t *testing.T) {
	b, ctx := newTestBudget(t, 1000)

	b.Add(100, 50)
	b.Add(200, 100)

	if got, want := b.Spent(), 450; got != want {
		t.Errorf("Spent() = %d, want %d", got, want)
	}
	if isDone(ctx) {
		t.Error("context cancelled at 450 of 1000 tokens")
	}
}

// The done-when for #5: crossing the ceiling cancels the root context, which
// is how in-flight model calls learn to stop. Cancellation is the budget's
// only output — there is no error return and no Exceeded() to consult.
func TestCrossingCeilingCancels(t *testing.T) {
	tests := []struct {
		name       string
		ceiling    int
		in, out    int
		wantCancel bool
	}{
		{name: "just under", ceiling: 100, in: 40, out: 59, wantCancel: false},
		// Spending the ceiling exhausts it: the configured number is the most a
		// run may spend, not one below it.
		{name: "exactly at", ceiling: 100, in: 50, out: 50, wantCancel: true},
		{name: "well over", ceiling: 100, in: 500, out: 500, wantCancel: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, ctx := newTestBudget(t, tt.ceiling)

			b.Add(tt.in, tt.out)

			if got := isDone(ctx); got != tt.wantCancel {
				t.Errorf("spent %d of %d: cancelled = %v, want %v",
					tt.in+tt.out, tt.ceiling, got, tt.wantCancel)
			}
		})
	}
}

// Tokens accumulate across calls rather than being compared per call, so a
// ceiling is reached by many small calls the same way as by one large one.
func TestAccumulatesToCeiling(t *testing.T) {
	b, ctx := newTestBudget(t, 100)

	for i := range 9 {
		b.Add(5, 5)
		if isDone(ctx) {
			t.Fatalf("cancelled after %d calls (%d tokens), ceiling is 100", i+1, b.Spent())
		}
	}

	b.Add(5, 5) // the tenth call reaches 100 exactly

	if !isDone(ctx) {
		t.Errorf("not cancelled at %d tokens, ceiling is 100", b.Spent())
	}
}

// Every model call reports through Add from its own worker goroutine, so the
// total has to be correct under contention. Without the mutex this loses
// increments and -race reports the unsynchronized access.
func TestConcurrentAdd(t *testing.T) {
	const (
		goroutines = 100
		perCall    = 10
	)

	// Ceiling high enough that this test is about accounting, not cancellation.
	b, _ := newTestBudget(t, goroutines*perCall*10)

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Add(perCall/2, perCall/2)
		}()
	}
	wg.Wait()

	if got, want := b.Spent(), goroutines*perCall; got != want {
		t.Errorf("Spent() = %d, want %d — %d increments lost", got, want, want-got)
	}
}

// Spent is read from the round loop while workers are still adding, so it must
// take the same lock as Add. This is a -race test: it asserts nothing about the
// value, only that reading concurrently with writing is safe.
func TestSpentIsSafeUnderConcurrentAdd(t *testing.T) {
	b, _ := newTestBudget(t, 1_000_000)

	done := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				_ = b.Spent()
			}
		}
	}()

	for range 200 {
		b.Add(1, 1)
	}
	close(done)
	wg.Wait()
}

// Many goroutines can cross the ceiling at once. context.CancelFunc is
// idempotent, so repeated cancellation is harmless — this pins that, since a
// future guard around cancellation would need its own synchronization.
func TestConcurrentCrossingIsSafe(t *testing.T) {
	b, ctx := newTestBudget(t, 10)

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Add(5, 5)
		}()
	}
	wg.Wait()

	if !isDone(ctx) {
		t.Error("context not cancelled after every goroutine crossed the ceiling")
	}
	if got, want := b.Spent(), 500; got != want {
		t.Errorf("Spent() = %d, want %d", got, want)
	}
}

// Add records tokens after a call completes, so the in-flight calls at the
// moment of crossing push the total past the ceiling. The overshoot is bounded
// by concurrency, and a run reporting slightly more than its ceiling is
// expected rather than a bug.
func TestOvershootIsRecorded(t *testing.T) {
	b, ctx := newTestBudget(t, 100)

	b.Add(60, 60) // 120 in one call

	if !isDone(ctx) {
		t.Fatal("not cancelled after exceeding the ceiling")
	}
	if got := b.Spent(); got <= 100 {
		t.Errorf("Spent() = %d, want the full overshoot above the 100 ceiling", got)
	}
}

// Cancellation must reach a goroutine that is only watching ctx.Done(), which
// is how the worker pool and every model call actually observe it.
func TestCancellationReachesWaiters(t *testing.T) {
	b, ctx := newTestBudget(t, 50)

	observed := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(observed)
	}()

	b.Add(25, 25)

	select {
	case <-observed:
	case <-time.After(time.Second):
		t.Fatal("a goroutine waiting on ctx.Done() was never woken")
	}
}
