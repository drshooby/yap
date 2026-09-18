package model

import (
	"context"
	"sync"

	"github.com/drshooby/yap/internal/events"
)

// FakeClient returns scripted outcomes instead of calling a provider, so the
// retrier and the agent loop can be tested without network access.
//
// Outcomes are consumed in order, one per Complete call. Once the script is
// exhausted the last outcome repeats, which keeps a test from depending on how
// many attempts the code under test happens to make.
type FakeClient struct {
	mu       sync.Mutex
	outcomes []Outcome
	calls    int

	// Prompts records every prompt the client was asked to complete, in order.
	Prompts []string
	// Models records the model named on each call, in order.
	Models []string
}

// Outcome is one scripted response: either a result or a failure.
type Outcome struct {
	Result CompletionResult
	Err    error
}

// Succeed scripts a successful completion.
func Succeed(message string, tokensIn, tokensOut int) Outcome {
	return Outcome{Result: CompletionResult{
		Message:   message,
		TokensIn:  tokensIn,
		TokensOut: tokensOut,
	}}
}

// Fail scripts a failure the caller can classify, as a provider adapter would.
func Fail(reason events.FailReason) Outcome {
	return Outcome{Err: &APIError{Reason: reason}}
}

// NewFakeClient returns a client that plays back the given outcomes in order.
func NewFakeClient(outcomes ...Outcome) *FakeClient {
	return &FakeClient{outcomes: outcomes}
}

var _ Client = (*FakeClient)(nil)

func (f *FakeClient) Complete(ctx context.Context, model, prompt string) (CompletionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++
	f.Prompts = append(f.Prompts, prompt)
	f.Models = append(f.Models, model)

	if len(f.outcomes) == 0 {
		return CompletionResult{}, nil
	}

	i := min(f.calls-1, len(f.outcomes)-1)
	out := f.outcomes[i]
	return out.Result, out.Err
}

// Calls reports how many times Complete was called.
func (f *FakeClient) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}
