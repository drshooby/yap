package model

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/drshooby/yap/internal/events"
)

type CompletionResult struct {
	Message   string
	TokensIn  int
	TokensOut int
}

type Client interface {
	Complete(ctx context.Context, model, prompt string) (CompletionResult, error)
}

type retrier struct {
	inner Client
	max   int
	// Claude Note:
	// base is the first backoff interval; each attempt doubles it. Injected so
	// tests can shrink it rather than sleeping through real backoff.
	base time.Duration
}

// this is for me to be less confused about how Go does
// it's version of [cough cough] inheritance--I mean "interface satisfaction"
var _ Client = (*retrier)(nil)

type APIError struct {
	Reason events.FailReason
	Err    error
}

func (e *APIError) Error() string {
	return fmt.Sprintf("model call failed: %s", e.Reason)
}

func (e *APIError) Unwrap() error {
	return e.Err
}

func retryable(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.Reason {
	case events.FailReasonRateLimited, events.FailReasonAPIError, events.FailReasonTimeout:
		return true
	default:
		return false
	}
}

func (r *retrier) Complete(ctx context.Context, model, prompt string) (CompletionResult, error) {
	var lastErr error
	for attempt := range r.max {
		if ctx.Err() != nil {
			return CompletionResult{}, ctx.Err()
		}
		// Complete will have to translate errors for retryable
		res, err := r.inner.Complete(ctx, model, prompt)
		if err == nil || !retryable(err) {
			return res, err
		}
		lastErr = err
		if attempt < r.max-1 {
			backoffWithJitter := time.Duration(1<<attempt)*r.base + time.Duration(rand.Intn(400))*r.base/1000
			select {
			case <-time.After(backoffWithJitter):
				// slept so try again
			case <-ctx.Done():
				return CompletionResult{}, ctx.Err()
			}
		}
	}
	return CompletionResult{}, fmt.Errorf("after %d attempts: %w", r.max, lastErr)
}

// WithRetry wraps a client so transient failures are retried with exponential
// backoff and jitter, starting at one second.
func WithRetry(inner Client, max int) Client {
	return withRetryBackoff(inner, max, time.Second)
}

func withRetryBackoff(inner Client, max int, base time.Duration) Client {
	return &retrier{inner: inner, max: max, base: base}
}
