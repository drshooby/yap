// Package events defines the event schema and its writer and reader.
package events

import "time"

// MessageID identifies a single message. Used for message identity, parent
// links, and inbox references — all the same kind of thing, so all one type.
type MessageID string

// Event type discriminators, written to the "type" field.
const (
	TypeRunHeader     = "run_header"
	TypeExchange      = "exchange"
	TypeBeliefUpdated = "belief_updated"
	TypeCallFailed    = "call_failed"
	TypeRoundSummary  = "round_summary"
)

type CommonHeaders struct {
	Round int       `json:"round"`
	Ts    time.Time `json:"ts"`
	Type  string    `json:"type"`
}

type Cohort struct {
	Name       string `json:"name"`
	Count      int    `json:"count"`
	Model      string `json:"model"`
	Persona    string `json:"persona"`
	SeedBelief string `json:"seed_belief,omitempty"`
}

type PromptVersions struct {
	BeliefUpdate string `json:"belief_update"`
	Observer     string `json:"observer,omitempty"`
}

type RunHeaderEvent struct {
	CommonHeaders
	RunID          string         `json:"run_id"`
	Seed           int64          `json:"seed"`
	Rounds         int            `json:"rounds"`
	PeersPerRound  int            `json:"peers_per_round"`
	TokenBudget    int            `json:"token_budget"`
	Cohorts        []Cohort       `json:"cohorts"`
	PromptVersions PromptVersions `json:"prompt_versions"`
}

type ExchangeEvent struct {
	CommonHeaders
	From       string    `json:"from"`
	FromCohort string    `json:"from_cohort"`
	To         string    `json:"to"`
	MsgID      MessageID `json:"msg_id"`
	ParentID   MessageID `json:"parent_id,omitempty"`
	Text       string    `json:"text"`
	TokensIn   int       `json:"tokens_in"`
	TokensOut  int       `json:"tokens_out"`
}

type BeliefUpdatedEvent struct {
	CommonHeaders
	Agent         string      `json:"agent"`
	Cohort        string      `json:"cohort"`
	Before        string      `json:"before"`
	After         string      `json:"after"`
	InboxMsgIDs   []MessageID `json:"inbox_msg_ids"`
	PromptVersion string      `json:"prompt_version"`
}

type FailReason string

const (
	FailReasonRefusal     FailReason = "refusal"
	FailReasonTimeout     FailReason = "timeout"
	FailReasonRateLimited FailReason = "rate_limit"
	FailReasonMalformed   FailReason = "malformed"
	FailReasonAPIError    FailReason = "api_error"
)

type CallFailedEvent struct {
	CommonHeaders
	Agent     string     `json:"agent"`
	Cohort    string     `json:"cohort"`
	Reason    FailReason `json:"reason"`
	Attempt   int        `json:"attempt"`
	WillRetry bool       `json:"will_retry"`
	Detail    string     `json:"detail,omitempty"`
}

type RoundSummaryEvent struct {
	CommonHeaders
	AgentCount       int                `json:"agent_count"`
	Exchanges        int                `json:"exchanges"`
	UpdatesOK        int                `json:"updates_ok"`
	UpdatesFailed    int                `json:"updates_failed"`
	FailuresByReason map[FailReason]int `json:"failures_by_reason,omitempty"`
	TotalTokensSpent int                `json:"total_tokens_spent"`
	RoundTokensSpent int                `json:"round_tokens_spent"`
}
