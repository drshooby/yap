package model

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/drshooby/yap/internal/events"
)

// A call that works costs one attempt and returns what the provider gave back.
func TestSucceedsWithoutRetrying(t *testing.T) {
	fake := NewFakeClient(Succeed("the sky is green", 412, 88))
	c := WithRetry(fake, 3)

	res, err := c.Complete(context.Background(), "claude-sonnet-5", "what colour is the sky")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if got, want := res.Message, "the sky is green"; got != want {
		t.Errorf("Message = %q, want %q", got, want)
	}
	if res.TokensIn != 412 || res.TokensOut != 88 {
		t.Errorf("tokens = (%d, %d), want (412, 88)", res.TokensIn, res.TokensOut)
	}
	if got := fake.Calls(); got != 1 {
		t.Errorf("made %d calls, want 1", got)
	}
}

// Token counts reach the caller on the result rather than being reported by the
// client itself, which is what lets the agent loop hand them to the budget.
func TestResultCarriesTokenCounts(t *testing.T) {
	fake := NewFakeClient(Succeed("ok", 100, 25))

	res, err := fake.Complete(context.Background(), "m", "p")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got, want := res.TokensIn+res.TokensOut, 125; got != want {
		t.Errorf("total tokens = %d, want %d", got, want)
	}
}

// The done-when for #6: a transient failure is retried and the eventual success
// is returned, with no sign of the failures in the result.
func TestRetriesThenSucceeds(t *testing.T) {
	fake := NewFakeClient(
		Fail(events.FailReasonRateLimited),
		Fail(events.FailReasonAPIError),
		Succeed("made it", 10, 20),
	)
	c := withRetryBackoff(fake, 3, time.Millisecond)

	res, err := c.Complete(t.Context(), "claude-sonnet-5", "prompt")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if got, want := res.Message, "made it"; got != want {
		t.Errorf("Message = %q, want %q", got, want)
	}
	if got := fake.Calls(); got != 3 {
		t.Errorf("made %d calls, want 3", got)
	}
}

// Which failures are worth retrying is the classification the schema's reason
// values exist for. Retrying a refusal wastes money on an answer that will not
// change; not retrying a rate limit throws away a call that would have worked.
func TestRetryClassification(t *testing.T) {
	tests := []struct {
		name      string
		reason    events.FailReason
		wantCalls int
	}{
		{name: "rate limit is transient", reason: events.FailReasonRateLimited, wantCalls: 3},
		{name: "api error is transient", reason: events.FailReasonAPIError, wantCalls: 3},
		{name: "timeout is transient", reason: events.FailReasonTimeout, wantCalls: 3},
		{name: "refusal is final", reason: events.FailReasonRefusal, wantCalls: 1},
		{name: "malformed is final", reason: events.FailReasonMalformed, wantCalls: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := NewFakeClient(Fail(tt.reason))
			c := withRetryBackoff(fake, 3, time.Millisecond)

			_, err := c.Complete(t.Context(), "m", "p")
			if err == nil {
				t.Fatal("Complete succeeded, want an error")
			}

			if got := fake.Calls(); got != tt.wantCalls {
				t.Errorf("made %d calls, want %d", got, tt.wantCalls)
			}

			// The reason must survive however many times the error was wrapped,
			// because call_failed events are written from it.
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error does not unwrap to *APIError: %v", err)
			}
			if apiErr.Reason != tt.reason {
				t.Errorf("Reason = %q, want %q", apiErr.Reason, tt.reason)
			}
		})
	}
}

// Exhausting every attempt must report the failure that actually happened, not
// a generic one — the last error is what the call_failed event records.
func TestExhaustedRetriesKeepsLastError(t *testing.T) {
	// Every attempt fails with a retryable reason, so the retrier runs out of
	// attempts and returns its own wrapped error rather than one of the inner
	// ones. That wrapper must still carry the reason through.
	fake := NewFakeClient(
		Fail(events.FailReasonAPIError),
		Fail(events.FailReasonRateLimited),
	)
	c := withRetryBackoff(fake, 2, time.Millisecond)

	_, err := c.Complete(t.Context(), "m", "p")
	if err == nil {
		t.Fatal("Complete succeeded, want an error")
	}

	if !strings.Contains(err.Error(), "after 2 attempts") {
		t.Errorf("error = %q, want it to say how many attempts were made", err)
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error does not unwrap to *APIError: %v", err)
	}
	// The last failure, not the first — a call_failed event should record what
	// actually ended the call.
	if apiErr.Reason != events.FailReasonRateLimited {
		t.Errorf("Reason = %q, want %q", apiErr.Reason, events.FailReasonRateLimited)
	}
	if got := fake.Calls(); got != 2 {
		t.Errorf("made %d calls, want 2", got)
	}
}

// An error carrying an underlying cause must let errors.Is reach it, so a
// cancelled context stays recognisable after the adapter has classified it.
func TestAPIErrorUnwrapsToCause(t *testing.T) {
	err := error(&APIError{
		Reason: events.FailReasonTimeout,
		Err:    context.DeadlineExceeded,
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Error("errors.Is could not reach the wrapped cause")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatal("errors.As did not match *APIError")
	}
	if apiErr.Reason != events.FailReasonTimeout {
		t.Errorf("Reason = %q, want %q", apiErr.Reason, events.FailReasonTimeout)
	}
}

// A context that is already done must stop the retrier before it spends
// anything. The budget cancels the root context, so an agent whose turn comes
// up after the ceiling is hit should never reach the provider.
func TestCancelledContextMakesNoCalls(t *testing.T) {
	fake := NewFakeClient(Succeed("should not happen", 1, 1))
	c := WithRetry(fake, 3)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.Complete(ctx, "m", "p")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if got := fake.Calls(); got != 0 {
		t.Errorf("made %d calls on a cancelled context, want 0", got)
	}
}

// Cancellation during backoff must be observed while the retrier is sleeping.
// time.Sleep cannot be interrupted, so this fails unless the backoff selects on
// ctx.Done() — and a run that ignores it keeps spending after the budget stops.
func TestCancelDuringBackoffReturnsPromptly(t *testing.T) {
	fake := NewFakeClient(Fail(events.FailReasonRateLimited))
	c := WithRetry(fake, 4)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := c.Complete(ctx, "m", "p")
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	// The first backoff alone is over a second; returning near the cancel
	// proves the sleep was interrupted rather than waited out.
	if elapsed > 500*time.Millisecond {
		t.Errorf("returned after %v, want promptly after cancellation at ~50ms", elapsed)
	}
}

// The model is a cohort property, so it travels per call rather than being
// fixed when the client is built.
func TestModelIsPerCall(t *testing.T) {
	fake := NewFakeClient(
		Succeed("a", 1, 1),
		Succeed("b", 1, 1),
	)

	if _, err := fake.Complete(t.Context(), "claude-sonnet-5", "p1"); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if _, err := fake.Complete(t.Context(), "claude-haiku-4-5", "p2"); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	want := []string{"claude-sonnet-5", "claude-haiku-4-5"}
	for i, w := range want {
		if fake.Models[i] != w {
			t.Errorf("call %d used model %q, want %q", i, fake.Models[i], w)
		}
	}
}

// A non-APIError must not be retried: the retrier can only reason about
// failures a provider adapter has classified, and retrying an unknown error
// risks repeating something that will never succeed.
func TestUnclassifiedErrorIsNotRetried(t *testing.T) {
	fake := NewFakeClient(Outcome{Err: errors.New("something unexpected")})
	c := withRetryBackoff(fake, 3, time.Millisecond)

	_, err := c.Complete(t.Context(), "m", "p")
	if err == nil {
		t.Fatal("Complete succeeded, want an error")
	}
	if got := fake.Calls(); got != 1 {
		t.Errorf("made %d calls on an unclassified error, want 1", got)
	}
}
