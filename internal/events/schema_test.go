package events

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"
)

var testDate = time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

func TestEmbeddingFlattens(t *testing.T) {
	testEvent := RunHeaderEvent{
		CommonHeaders: CommonHeaders{
			Round: 1,
			Ts:    testDate,
			Type:  TypeRunHeader,
		},
		RunID:         "abcd",
		Seed:          42,
		Rounds:        100,
		PeersPerRound: 3,
		TokenBudget:   2_000_000,
		Cohorts: []Cohort{
			{
				Name:       "test cohort",
				Count:      2,
				Model:      "frontier-agi-model-2",
				Persona:    "good listener",
				SeedBelief: "context rot is an artistic depiction of a fulfilling chat",
			},
		},
	}

	jsonBytes, err := json.Marshal(testEvent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(jsonBytes, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// should work
	if _, ok := got["round"]; !ok {
		t.Errorf("nothing found on 'round' search, expected 1")
	}

	// shouldn't work
	if _, ok := got["CommonHeaders"]; ok {
		t.Errorf("found key 'CommonHeaders', expected embedded fields to be flat")
	}
}

// Wire names are fixed by docs/design.md. A typo'd struct tag round-trips
// fine — marshal and unmarshal share the same wrong name — so the only thing
// that catches it is asserting the key strings directly.
func TestWireFieldNames(t *testing.T) {
	tests := []struct {
		name  string
		event any
		want  []string
	}{
		{
			name: "run_header",
			event: RunHeaderEvent{
				CommonHeaders: CommonHeaders{Round: 0, Ts: testDate, Type: TypeRunHeader},
				RunID:         "abcd",
				Seed:          42,
				Rounds:        100,
				PeersPerRound: 3,
				TokenBudget:   2_000_000,
			},
			want: []string{
				"round", "ts", "type", "run_id", "seed", "rounds",
				"peers_per_round", "token_budget", "cohorts", "prompt_versions",
			},
		},
		{
			name: "exchange",
			event: ExchangeEvent{
				CommonHeaders: CommonHeaders{Round: 7, Ts: testDate, Type: TypeExchange},
				From:          "agent-042",
				FromCohort:    "crowd",
				To:            "agent-119",
				MsgID:         "msg-1",
				ParentID:      "msg-0",
				Text:          "the sky is green",
				TokensIn:      412,
				TokensOut:     88,
			},
			want: []string{
				"round", "ts", "type", "from", "from_cohort", "to",
				"msg_id", "parent_id", "text", "tokens_in", "tokens_out",
			},
		},
		{
			name: "belief_updated",
			event: BeliefUpdatedEvent{
				CommonHeaders: CommonHeaders{Round: 7, Ts: testDate, Type: TypeBeliefUpdated},
				Agent:         "agent-042",
				Cohort:        "crowd",
				Before:        "the sky is blue",
				After:         "the sky is green",
				InboxMsgIDs:   []MessageID{"msg-1", "msg-2"},
				PromptVersion: "sha256:abcd",
			},
			want: []string{
				"round", "ts", "type", "agent", "cohort", "before", "after",
				"inbox_msg_ids", "prompt_version",
			},
		},
		{
			name: "call_failed",
			event: CallFailedEvent{
				CommonHeaders: CommonHeaders{Round: 7, Ts: testDate, Type: TypeCallFailed},
				Agent:         "agent-042",
				Cohort:        "crowd",
				Reason:        FailReasonRefusal,
				Attempt:       2,
				WillRetry:     true,
				Detail:        "model declined",
			},
			want: []string{
				"round", "ts", "type", "agent", "cohort", "reason",
				"attempt", "will_retry", "detail",
			},
		},
		{
			name: "round_summary",
			event: RoundSummaryEvent{
				CommonHeaders:    CommonHeaders{Round: 7, Ts: testDate, Type: TypeRoundSummary},
				AgentCount:       200,
				Exchanges:        600,
				UpdatesOK:        197,
				UpdatesFailed:    3,
				FailuresByReason: map[FailReason]int{FailReasonRateLimited: 2, FailReasonRefusal: 1},
				TotalTokensSpent: 681204,
				RoundTokensSpent: 14002,
			},
			want: []string{
				"round", "ts", "type", "agent_count", "exchanges", "updates_ok",
				"updates_failed", "failures_by_reason", "total_tokens_spent",
				"round_tokens_spent",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.event)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var got map[string]any
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			for _, key := range tt.want {
				if _, ok := got[key]; !ok {
					t.Errorf("missing key %q", key)
				}
			}

			// Anything not in want is a field the design doc doesn't describe.
			for key := range got {
				if !slices.Contains(tt.want, key) {
					t.Errorf("unexpected key %q", key)
				}
			}
		})
	}
}

// omitempty fields must vanish when unset, and appear when set. Absent and
// empty-string are different states in the log.
func TestOmitEmpty(t *testing.T) {
	t.Run("absent when unset", func(t *testing.T) {
		ex := ExchangeEvent{
			CommonHeaders: CommonHeaders{Round: 1, Ts: testDate, Type: TypeExchange},
			From:          "agent-001",
			To:            "agent-002",
			MsgID:         "msg-1",
			Text:          "a root message has no parent",
		}

		got := marshalToMap(t, ex)
		if _, ok := got["parent_id"]; ok {
			t.Errorf("parent_id present on a root message, want omitted")
		}
	})

	t.Run("present when set", func(t *testing.T) {
		ex := ExchangeEvent{
			CommonHeaders: CommonHeaders{Round: 1, Ts: testDate, Type: TypeExchange},
			MsgID:         "msg-2",
			ParentID:      "msg-1",
		}

		got := marshalToMap(t, ex)
		if got["parent_id"] != "msg-1" {
			t.Errorf("parent_id = %v, want msg-1", got["parent_id"])
		}
	})

	t.Run("empty failure map omitted", func(t *testing.T) {
		rs := RoundSummaryEvent{
			CommonHeaders: CommonHeaders{Round: 1, Ts: testDate, Type: TypeRoundSummary},
			AgentCount:    200,
		}

		got := marshalToMap(t, rs)
		if _, ok := got["failures_by_reason"]; ok {
			t.Errorf("failures_by_reason present on a clean round, want omitted")
		}
	})
}

// The done-when for #2: every event type survives a trip through JSON unchanged.
func TestRoundTrip(t *testing.T) {
	t.Run("run_header", func(t *testing.T) {
		want := RunHeaderEvent{
			CommonHeaders:  CommonHeaders{Round: 0, Ts: testDate, Type: TypeRunHeader},
			RunID:          "run-1",
			Seed:           42,
			Rounds:         50,
			PeersPerRound:  3,
			TokenBudget:    2_000_000,
			Cohorts:        []Cohort{{Name: "cultists", Count: 5, Model: "m", Persona: "p", SeedBelief: "s"}, {Name: "crowd", Count: 195, Model: "m2", Persona: "p2"}},
			PromptVersions: PromptVersions{BeliefUpdate: "sha256:aaa", Observer: "sha256:bbb"},
		}

		var got RunHeaderEvent
		roundTrip(t, want, &got)

		if !reflect.DeepEqual(want, got) {
			t.Errorf("round trip mismatch\n want %+v\n  got %+v", want, got)
		}
	})

	t.Run("exchange", func(t *testing.T) {
		want := ExchangeEvent{
			CommonHeaders: CommonHeaders{Round: 7, Ts: testDate, Type: TypeExchange},
			From:          "agent-042",
			FromCohort:    "crowd",
			To:            "agent-119",
			MsgID:         "msg-1",
			ParentID:      "msg-0",
			Text:          "the sky is green",
			TokensIn:      412,
			TokensOut:     88,
		}

		var got ExchangeEvent
		roundTrip(t, want, &got)

		if !reflect.DeepEqual(want, got) {
			t.Errorf("round trip mismatch\n want %+v\n  got %+v", want, got)
		}
	})

	t.Run("belief_updated", func(t *testing.T) {
		want := BeliefUpdatedEvent{
			CommonHeaders: CommonHeaders{Round: 7, Ts: testDate, Type: TypeBeliefUpdated},
			Agent:         "agent-042",
			Cohort:        "crowd",
			Before:        "the sky is blue",
			After:         "the sky is green",
			InboxMsgIDs:   []MessageID{"msg-1", "msg-2"},
			PromptVersion: "sha256:abcd",
		}

		var got BeliefUpdatedEvent
		roundTrip(t, want, &got)

		if !reflect.DeepEqual(want, got) {
			t.Errorf("round trip mismatch\n want %+v\n  got %+v", want, got)
		}
	})

	t.Run("call_failed", func(t *testing.T) {
		want := CallFailedEvent{
			CommonHeaders: CommonHeaders{Round: 7, Ts: testDate, Type: TypeCallFailed},
			Agent:         "agent-042",
			Cohort:        "crowd",
			Reason:        FailReasonRateLimited,
			Attempt:       2,
			WillRetry:     true,
			Detail:        "429 from upstream",
		}

		var got CallFailedEvent
		roundTrip(t, want, &got)

		if !reflect.DeepEqual(want, got) {
			t.Errorf("round trip mismatch\n want %+v\n  got %+v", want, got)
		}
	})

	t.Run("round_summary", func(t *testing.T) {
		want := RoundSummaryEvent{
			CommonHeaders:    CommonHeaders{Round: 7, Ts: testDate, Type: TypeRoundSummary},
			AgentCount:       200,
			Exchanges:        600,
			UpdatesOK:        197,
			UpdatesFailed:    3,
			FailuresByReason: map[FailReason]int{FailReasonRateLimited: 2, FailReasonRefusal: 1},
			TotalTokensSpent: 681204,
			RoundTokensSpent: 14002,
		}

		var got RoundSummaryEvent
		roundTrip(t, want, &got)

		if !reflect.DeepEqual(want, got) {
			t.Errorf("round trip mismatch\n want %+v\n  got %+v", want, got)
		}
	})
}

// FailReason is a string type so it reads plainly on the wire and works as a
// JSON object key in failures_by_reason.
func TestFailReasonOnTheWire(t *testing.T) {
	cf := CallFailedEvent{
		CommonHeaders: CommonHeaders{Round: 1, Ts: testDate, Type: TypeCallFailed},
		Reason:        FailReasonRefusal,
	}

	got := marshalToMap(t, cf)
	if got["reason"] != "refusal" {
		t.Errorf("reason = %v, want the string \"refusal\"", got["reason"])
	}

	rs := RoundSummaryEvent{
		CommonHeaders:    CommonHeaders{Round: 1, Ts: testDate, Type: TypeRoundSummary},
		FailuresByReason: map[FailReason]int{FailReasonMalformed: 4},
	}

	summary := marshalToMap(t, rs)
	reasons, ok := summary["failures_by_reason"].(map[string]any)
	if !ok {
		t.Fatalf("failures_by_reason = %T, want a JSON object", summary["failures_by_reason"])
	}
	// Numbers come back as float64 through map[string]any.
	if reasons["malformed"] != float64(4) {
		t.Errorf("failures_by_reason[malformed] = %v, want 4", reasons["malformed"])
	}
}

func TestContentHash(t *testing.T) {
	const text = "the sky is green"
	const want = "cc2d82b6857857f92a704a90d2d22510f682d761e17d9bd32d1f062f9efe42b5"

	if got := ContentHash(text); got != want {
		t.Errorf("ContentHash(%q) = %q, want %q", text, got, want)
	}

	if ContentHash("a") == ContentHash("a ") {
		t.Error("ContentHash is normalizing whitespace; it should hash the string as given")
	}
}

// marshalToMap marshals v and decodes it into a generic map, for asserting on
// the JSON shape rather than the round-tripped value.
func marshalToMap(t *testing.T, v any) map[string]any {
	t.Helper()

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return got
}

// roundTrip marshals want and unmarshals it back into got, which must be a
// pointer to the same type.
func roundTrip(t *testing.T, want any, got any) {
	t.Helper()

	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := json.Unmarshal(b, got); err != nil {
		t.Fatalf("unmarshal: %v\njson: %s", err, b)
	}
}
