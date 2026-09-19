package anthropic

import (
	"context"
	"errors"
	"strings"

	"github.com/drshooby/yap/internal/events"
	"github.com/drshooby/yap/internal/model"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Claude Note:
// maxTokens caps a single belief. Beliefs are a few sentences, so this is
// generous; a response that hits the cap is treated as malformed rather than
// truncated silently.
const maxTokens = 1024

type Client struct {
	anthropicClient anthropic.Client
}

var _ model.Client = (*Client)(nil)

func New(apiKey string) *Client {
	return &Client{
		anthropicClient: anthropic.NewClient(
			option.WithAPIKey(apiKey),
		),
	}
}

func collect(msg *anthropic.Message) string {
	var sb strings.Builder
	for _, block := range msg.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}
	return sb.String()
}

// Claude Note:
// translateError maps an SDK failure onto one of the schema's FailReason
// values. The original error is kept so errors.Is still reaches it through
// APIError.Unwrap.
//
// Retry policy does not live here: this says what kind of failure happened, and
// the retrier decides what to do about it.
func translateError(err error) *model.APIError {
	// Check context completion first
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &model.APIError{Reason: events.FailReasonCancelled, Err: err}
	}

	var sdkErr *anthropic.Error
	if !errors.As(err, &sdkErr) {
		// Claude Note:
		// The SDK only wraps responses it actually received. Anything else is a
		// transport failure, which is worth another attempt.
		return &model.APIError{Reason: events.FailReasonTimeout, Err: err}
	}

	reason := events.FailReasonAPIError
	if sdkErr.StatusCode == 429 {
		reason = events.FailReasonRateLimited
	}
	return &model.APIError{Reason: reason, Err: err}
}

// Claude Note:
// translateStopReason classifies a response the API considered successful.
// A refusal arrives as HTTP 200 with tokens billed, so it is only visible here
// — see the Notes on #17.
func translateStopReason(msg *anthropic.Message) *model.APIError {
	switch msg.StopReason {
	case anthropic.StopReasonRefusal:
		return &model.APIError{Reason: events.FailReasonRefusal}
	case anthropic.StopReasonMaxTokens, anthropic.StopReasonModelContextWindowExceeded:
		// A belief cut off mid-sentence is corrupt
		return &model.APIError{Reason: events.FailReasonMalformed}
	default:
		return nil
	}
}

func (c *Client) Complete(ctx context.Context, modelName, prompt string) (model.CompletionResult, error) {
	message, err := c.anthropicClient.Messages.New(ctx, anthropic.MessageNewParams{
		MaxTokens: maxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
		Model: anthropic.Model(modelName),
	})
	if err != nil {
		return model.CompletionResult{}, translateError(err)
	}

	if apiErr := translateStopReason(message); apiErr != nil {
		return model.CompletionResult{}, apiErr
	}

	text := collect(message)
	if text == "" {
		return model.CompletionResult{}, &model.APIError{Reason: events.FailReasonMalformed}
	}

	return model.CompletionResult{
		Message:   text,
		TokensIn:  int(message.Usage.InputTokens),
		TokensOut: int(message.Usage.OutputTokens),
	}, nil
}
