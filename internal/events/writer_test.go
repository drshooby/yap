package events

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// The done-when for #3: 100 goroutines emitting at once yield 100 parseable
// lines. Without the mutex the encoders interleave into the same bufio.Writer
// and lines come out spliced together — and -race reports the unsynchronized
// access even on the runs where the bytes happen to land cleanly.
func TestConcurrentWrites(t *testing.T) {
	const n = 100

	var buf bytes.Buffer
	ew := NewEventWriter(&buf)

	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ev := &ExchangeEvent{
				CommonHeaders: CommonHeaders{Round: i, Ts: testDate, Type: TypeExchange},
				From:          "agent-042",
				To:            "agent-119",
				MsgID:         MessageID("msg-" + strings.Repeat("x", i%7)),
				Text:          strings.Repeat("the sky is green. ", 40),
				TokensIn:      i,
			}
			if err := ew.Write(ev); err != nil {
				t.Errorf("write: %v", err)
			}
		}()
	}
	wg.Wait()

	if err := ew.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	lines := jsonlLines(t, buf.String())
	if len(lines) != n {
		t.Fatalf("got %d lines, want %d", len(lines), n)
	}

	// Every line must parse on its own. Interleaved writes produce garbage
	// here long before they produce a wrong count.
	rounds := make(map[int]bool, n)
	for i, line := range lines {
		var got ExchangeEvent
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d is not valid JSON: %v\nline: %q", i, err, line)
		}
		if got.Type != TypeExchange {
			t.Errorf("line %d: type = %q, want %q", i, got.Type, TypeExchange)
		}
		if got.TokensIn != got.Round {
			t.Errorf("line %d: tokens_in = %d, want %d — fields crossed between events", i, got.TokensIn, got.Round)
		}
		rounds[got.Round] = true
	}

	// Each goroutine used a distinct round, so a duplicate or a gap means a
	// write was lost or doubled.
	if len(rounds) != n {
		t.Errorf("got %d distinct rounds, want %d", len(rounds), n)
	}
}

// JSONL is one compact object per line: no array wrapper, no indentation, and
// every line — including the last — terminated by a newline.
func TestJSONLFormat(t *testing.T) {
	var buf bytes.Buffer
	ew := NewEventWriter(&buf)

	events := []Event{
		&RunHeaderEvent{
			CommonHeaders: CommonHeaders{Round: 0, Ts: testDate, Type: TypeRunHeader},
			RunID:         "run-1",
			Seed:          42,
			Cohorts:       []Cohort{{Name: "crowd", Count: 2, Model: "m", Persona: "p"}},
		},
		&ExchangeEvent{
			CommonHeaders: CommonHeaders{Round: 1, Ts: testDate, Type: TypeExchange},
			From:          "agent-001",
			To:            "agent-002",
			MsgID:         "msg-1",
			Text:          "the sky is green",
		},
		&RoundSummaryEvent{
			CommonHeaders: CommonHeaders{Round: 1, Ts: testDate, Type: TypeRoundSummary},
			AgentCount:    2,
			Exchanges:     1,
		},
	}
	for _, e := range events {
		if err := ew.Write(e); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := ew.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	out := buf.String()

	if strings.HasPrefix(out, "[") {
		t.Errorf("output starts with %q, want a bare object per line, not a JSON array", out[:1])
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("output does not end in a newline, want every line terminated")
	}
	if strings.HasSuffix(out, "\n\n") {
		t.Errorf("output ends in a blank line, want exactly one trailing newline")
	}

	lines := jsonlLines(t, out)
	if len(lines) != len(events) {
		t.Fatalf("got %d lines, want %d", len(lines), len(events))
	}

	for i, line := range lines {
		if strings.ContainsAny(line, "\n\t") {
			t.Errorf("line %d contains a newline or tab, want compact encoding: %q", i, line)
		}
		if strings.Contains(line, ": ") && strings.Contains(line, "\n  ") {
			t.Errorf("line %d looks pretty-printed: %q", i, line)
		}
		if !strings.HasPrefix(line, "{") || !strings.HasSuffix(line, "}") {
			t.Errorf("line %d is not a bare JSON object: %q", i, line)
		}
		if !json.Valid([]byte(line)) {
			t.Errorf("line %d is not valid JSON: %q", i, line)
		}
	}
}

// Callers shouldn't each have to remember to stamp a time, but a caller that
// did set one must not have it overwritten.
func TestTimestampFilling(t *testing.T) {
	t.Run("zero timestamp filled at write time", func(t *testing.T) {
		before := time.Now()

		var buf bytes.Buffer
		ew := NewEventWriter(&buf)

		ev := &ExchangeEvent{
			CommonHeaders: CommonHeaders{Round: 1, Type: TypeExchange},
			MsgID:         "msg-1",
		}
		if err := ew.Write(ev); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := ew.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}

		after := time.Now()

		var got ExchangeEvent
		decodeLine(t, buf.String(), 0, &got)

		if got.Ts.IsZero() {
			t.Fatalf("ts = zero, want a timestamp filled in at write time")
		}
		if got.Ts.Before(before) || got.Ts.After(after) {
			t.Errorf("ts = %v, want a time within [%v, %v]", got.Ts, before, after)
		}
	})

	t.Run("existing timestamp preserved", func(t *testing.T) {
		var buf bytes.Buffer
		ew := NewEventWriter(&buf)

		ev := &ExchangeEvent{
			CommonHeaders: CommonHeaders{Round: 1, Ts: testDate, Type: TypeExchange},
			MsgID:         "msg-1",
		}
		if err := ew.Write(ev); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := ew.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}

		var got ExchangeEvent
		decodeLine(t, buf.String(), 0, &got)

		if !got.Ts.Equal(testDate) {
			t.Errorf("ts = %v, want %v", got.Ts, testDate)
		}
	})

	t.Run("caller's event is stamped in place", func(t *testing.T) {
		var buf bytes.Buffer
		ew := NewEventWriter(&buf)

		ev := &ExchangeEvent{
			CommonHeaders: CommonHeaders{Round: 1, Type: TypeExchange},
			MsgID:         "msg-1",
		}
		if err := ew.Write(ev); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := ew.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}

		// The writer fills the header through the Event interface, so the
		// caller's own struct sees the stamp. That only holds for pointers —
		// a value passed as an Event is copied and the stamp is lost.
		if ev.Ts.IsZero() {
			t.Errorf("caller's ts = zero, want the writer's stamp visible on the event it was given")
		}
	})
}

// bufio holds small writes back, so nothing is durable until Close. Every
// other test in this file closes before asserting for exactly this reason.
func TestCloseFlushes(t *testing.T) {
	var buf bytes.Buffer
	ew := NewEventWriter(&buf)

	ev := &ExchangeEvent{
		CommonHeaders: CommonHeaders{Round: 1, Ts: testDate, Type: TypeExchange},
		MsgID:         "msg-1",
		Text:          "small enough to sit in the buffer",
	}
	if err := ew.Write(ev); err != nil {
		t.Fatalf("write: %v", err)
	}

	if n := buf.Len(); n != 0 {
		t.Errorf("underlying writer has %d bytes before Close, want 0 — the write should still be buffered", n)
	}

	if err := ew.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	lines := jsonlLines(t, buf.String())
	if len(lines) != 1 {
		t.Fatalf("got %d lines after Close, want 1", len(lines))
	}

	var got ExchangeEvent
	decodeLine(t, buf.String(), 0, &got)
	if got.MsgID != "msg-1" {
		t.Errorf("msg_id = %q, want msg-1", got.MsgID)
	}
}

func TestCloseSemantics(t *testing.T) {
	t.Run("double close is a no-op", func(t *testing.T) {
		var buf bytes.Buffer
		ew := NewEventWriter(&buf)

		ev := &ExchangeEvent{
			CommonHeaders: CommonHeaders{Round: 1, Ts: testDate, Type: TypeExchange},
			MsgID:         "msg-1",
		}
		if err := ew.Write(ev); err != nil {
			t.Fatalf("write: %v", err)
		}

		if err := ew.Close(); err != nil {
			t.Fatalf("first close: %v", err)
		}
		afterFirst := buf.String()

		if err := ew.Close(); err != nil {
			t.Errorf("second close: got %v, want nil", err)
		}
		if got := buf.String(); got != afterFirst {
			t.Errorf("second Close changed the output\n got %q\nwant %q", got, afterFirst)
		}
	})

	t.Run("write after close is rejected", func(t *testing.T) {
		var buf bytes.Buffer
		ew := NewEventWriter(&buf)

		if err := ew.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}

		ev := &ExchangeEvent{
			CommonHeaders: CommonHeaders{Round: 1, Ts: testDate, Type: TypeExchange},
			MsgID:         "msg-1",
		}
		if err := ew.Write(ev); err != ErrClosed {
			t.Errorf("write after close: got %v, want %v", err, ErrClosed)
		}
		if n := buf.Len(); n != 0 {
			t.Errorf("underlying writer has %d bytes after a rejected write, want 0", n)
		}
	})

	t.Run("write after close does not stamp the event", func(t *testing.T) {
		var buf bytes.Buffer
		ew := NewEventWriter(&buf)

		if err := ew.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}

		ev := &ExchangeEvent{
			CommonHeaders: CommonHeaders{Round: 1, Type: TypeExchange},
			MsgID:         "msg-1",
		}
		if err := ew.Write(ev); err != ErrClosed {
			t.Fatalf("write after close: got %v, want %v", err, ErrClosed)
		}
		if !ev.Ts.IsZero() {
			t.Errorf("ts = %v on a rejected write, want zero", ev.Ts)
		}
	})

	t.Run("close with nothing written produces empty output", func(t *testing.T) {
		var buf bytes.Buffer
		ew := NewEventWriter(&buf)

		if err := ew.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		if got := buf.String(); got != "" {
			t.Errorf("output = %q, want empty", got)
		}
	})
}

// Events must survive the writer, not just encoding/json — every type, with
// the round trip going through the actual JSONL bytes.
func TestWriterRoundTrip(t *testing.T) {
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
		writeRoundTrip(t, &want, &got)

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
		writeRoundTrip(t, &want, &got)

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
		writeRoundTrip(t, &want, &got)

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
		writeRoundTrip(t, &want, &got)

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
		writeRoundTrip(t, &want, &got)

		if !reflect.DeepEqual(want, got) {
			t.Errorf("round trip mismatch\n want %+v\n  got %+v", want, got)
		}
	})
}

// Text carrying newlines, quotes and control characters has to stay on one
// line — an escaped \n in the payload, not a real line break that would split
// the record in two.
func TestTextWithNewlinesStaysOneLine(t *testing.T) {
	var buf bytes.Buffer
	ew := NewEventWriter(&buf)

	want := ExchangeEvent{
		CommonHeaders: CommonHeaders{Round: 1, Ts: testDate, Type: TypeExchange},
		MsgID:         "msg-1",
		Text:          "line one\nline two\t\"quoted\"\r\nline three",
	}
	if err := ew.Write(&want); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := ew.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	lines := jsonlLines(t, buf.String())
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1 — embedded newlines split the record", len(lines))
	}

	var got ExchangeEvent
	decodeLine(t, buf.String(), 0, &got)
	if got.Text != want.Text {
		t.Errorf("text = %q, want %q", got.Text, want.Text)
	}
}

// jsonlLines splits JSONL output into its records, rejecting blank lines.
func jsonlLines(t *testing.T, out string) []string {
	t.Helper()

	if out == "" {
		return nil
	}
	trimmed := strings.TrimSuffix(out, "\n")
	lines := strings.Split(trimmed, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			t.Fatalf("line %d is blank, want one JSON object per line", i)
		}
	}
	return lines
}

// decodeLine unmarshals line i of JSONL output into got, which must be a
// pointer to an event type.
func decodeLine(t *testing.T, out string, i int, got any) {
	t.Helper()

	lines := jsonlLines(t, out)
	if i >= len(lines) {
		t.Fatalf("want line %d, got only %d lines", i, len(lines))
	}
	if err := json.Unmarshal([]byte(lines[i]), got); err != nil {
		t.Fatalf("unmarshal line %d: %v\nline: %q", i, err, lines[i])
	}
}

// writeRoundTrip sends want through an EventWriter and decodes the resulting
// line back into got. want must be a pointer so the writer can stamp headers;
// got a pointer to the same type.
func writeRoundTrip(t *testing.T, want Event, got any) {
	t.Helper()

	var buf bytes.Buffer
	ew := NewEventWriter(&buf)

	if err := ew.Write(want); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := ew.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	decodeLine(t, buf.String(), 0, got)
}
