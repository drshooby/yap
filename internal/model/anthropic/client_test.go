package anthropic

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/drshooby/yap/internal/events"
	"github.com/drshooby/yap/internal/model"
)

// These tests make real API calls and cost real tokens, so they skip unless
// ANTHROPIC_API_KEY is set. Run them with:
//
//	source .env && go test ./internal/model/anthropic/
//
// Everything they assert is a guess about the SDK's shape until a real response
// confirms it — which is the point of running them at all.
func requireKey(t *testing.T) string {
	t.Helper()

	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY not set; skipping live API test")
	}
	return key
}

// The done-when for #17: a real call returns text and the token counts the
// budget will be charged. Field names on the SDK response are unverified until
// this passes.
func TestLiveCompletion(t *testing.T) {
	key := requireKey(t)

	c := New(key)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	res, err := c.Complete(ctx, "claude-haiku-4-5", "Reply with exactly the word: green")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	t.Logf("message: %q", res.Message)
	t.Logf("tokens: in=%d out=%d", res.TokensIn, res.TokensOut)

	if res.Message == "" {
		t.Error("Message is empty; collect found no text blocks in the response")
	}
	if res.TokensIn <= 0 {
		t.Error("TokensIn is not positive; Usage.InputTokens is not being read correctly")
	}
	if res.TokensOut <= 0 {
		t.Error("TokensOut is not positive; Usage.OutputTokens is not being read correctly")
	}
}

// The other half of the done-when: a bad key must surface as a classified
// *model.APIError rather than a bare SDK error or a panic, because the agent
// loop writes call_failed events from the Reason.
func TestLiveBadKeyIsClassified(t *testing.T) {
	requireKey(t) // only run where the network is reachable

	c := New("sk-ant-not-a-real-key")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, err := c.Complete(ctx, "claude-haiku-4-5", "hello")
	if err == nil {
		t.Fatal("Complete succeeded with an invalid key")
	}

	t.Logf("error: %v", err)

	var apiErr *model.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is not a *model.APIError: %T", err)
	}
	if apiErr.Reason != events.FailReasonAPIError {
		t.Errorf("Reason = %q, want %q", apiErr.Reason, events.FailReasonAPIError)
	}
	// The SDK's own error must still be reachable, or debugging a failed run
	// means guessing what the provider actually said.
	if apiErr.Err == nil {
		t.Error("APIError.Err is nil; the underlying SDK error was discarded")
	}
}

// A cancelled context must not be reported as a provider failure. This is what
// happens to every in-flight call when the budget hits its ceiling.
func TestLiveCancellationIsClassified(t *testing.T) {
	key := requireKey(t)

	c := New(key)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.Complete(ctx, "claude-haiku-4-5", "hello")
	if err == nil {
		t.Fatal("Complete succeeded on a cancelled context")
	}

	var apiErr *model.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is not a *model.APIError: %T", err)
	}
	if apiErr.Reason != events.FailReasonCancelled {
		t.Errorf("Reason = %q, want %q", apiErr.Reason, events.FailReasonCancelled)
	}
	if !errors.Is(err, context.Canceled) {
		t.Error("errors.Is could not reach context.Canceled through the APIError")
	}
}

// An unknown model is a configuration mistake, and the adapter must classify it
// rather than letting the SDK error escape unwrapped. This is the failure #17
// wants caught at startup rather than at round one.
func TestLiveUnknownModelIsClassified(t *testing.T) {
	key := requireKey(t)

	c := New(key)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, err := c.Complete(ctx, "claude-does-not-exist-9", "hello")
	if err == nil {
		t.Fatal("Complete succeeded with a nonexistent model")
	}

	t.Logf("error: %v", err)

	var apiErr *model.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is not a *model.APIError: %T", err)
	}
	if apiErr.Reason != events.FailReasonAPIError {
		t.Errorf("Reason = %q, want %q", apiErr.Reason, events.FailReasonAPIError)
	}
}

// The retrier composes with the real adapter the same way it does with the
// fake: a working call passes through untouched and costs one attempt.
func TestLiveThroughRetrier(t *testing.T) {
	key := requireKey(t)

	c := model.WithRetry(New(key), 3)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	res, err := c.Complete(ctx, "claude-haiku-4-5", "Reply with exactly the word: blue")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if res.Message == "" {
		t.Error("Message is empty")
	}
}
