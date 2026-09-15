package events

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

// The done-when for #4: every event type survives the writer and comes back
// from the reader as its original concrete type with its field values intact.
// Type dispatch is the part at risk — a wrong case in the switch still returns
// a valid Event, just the wrong one.
func TestReaderRoundTrip(t *testing.T) {
	want := []Event{
		&RunHeaderEvent{
			CommonHeaders:  CommonHeaders{Round: 0, Ts: testDate, Type: TypeRunHeader},
			RunID:          "run-1",
			Seed:           42,
			Rounds:         50,
			PeersPerRound:  3,
			TokenBudget:    2_000_000,
			Cohorts:        []Cohort{{Name: "cultists", Count: 5, Model: "m", Persona: "p", SeedBelief: "s"}},
			PromptVersions: PromptVersions{BeliefUpdate: "sha256:aaa"},
		},
		&ExchangeEvent{
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
		&BeliefUpdatedEvent{
			CommonHeaders: CommonHeaders{Round: 7, Ts: testDate, Type: TypeBeliefUpdated},
			Agent:         "agent-042",
			Cohort:        "crowd",
			Before:        "the sky is blue",
			After:         "the sky is green",
			InboxMsgIDs:   []MessageID{"msg-1", "msg-2"},
			PromptVersion: "sha256:abcd",
		},
		&CallFailedEvent{
			CommonHeaders: CommonHeaders{Round: 7, Ts: testDate, Type: TypeCallFailed},
			Agent:         "agent-042",
			Cohort:        "crowd",
			Reason:        FailReasonRateLimited,
			Attempt:       2,
			WillRetry:     true,
			Detail:        "429 from upstream",
		},
		&RoundSummaryEvent{
			CommonHeaders:    CommonHeaders{Round: 7, Ts: testDate, Type: TypeRoundSummary},
			AgentCount:       200,
			Exchanges:        600,
			UpdatesOK:        197,
			UpdatesFailed:    3,
			FailuresByReason: map[FailReason]int{FailReasonRateLimited: 2, FailReasonRefusal: 1},
			TotalTokensSpent: 681204,
			RoundTokensSpent: 14002,
		},
	}

	got := readAll(t, writeAll(t, want...))

	if len(got) != len(want) {
		t.Fatalf("read %d events, want %d", len(got), len(want))
	}
	for i := range want {
		if reflect.TypeOf(got[i]) != reflect.TypeOf(want[i]) {
			t.Errorf("event %d: type %T, want %T", i, got[i], want[i])
			continue
		}
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("event %d round trip mismatch\n want %+v\n  got %+v", i, want[i], got[i])
		}
	}
}

// A malformed line must name the line it was on. A truncated or corrupt log is
// the normal result of a killed run, and "invalid character" with no position
// is not enough to find it in a file with thousands of records.
func TestMalformedLineReportsPosition(t *testing.T) {
	tests := []struct {
		name    string
		log     string
		wantErr string
	}{
		{
			name: "unknown type",
			log: `{"type":"exchange","round":1}
{"type":"exchange","round":2}
{"type":"exchange_v2","round":3}
`,
			wantErr: `line 3: unknown event type "exchange_v2"`,
		},
		{
			name: "missing type",
			log: `{"type":"exchange","round":1}
{"round":2,"from":"agent-001"}
`,
			wantErr: `line 2: unknown event type ""`,
		},
		{
			name: "not json at all",
			log: `{"type":"exchange","round":1}
this is not json
`,
			wantErr: "line 2: probe event",
		},
		{
			name: "truncated final line",
			log: `{"type":"exchange","round":1}
{"type":"exchange","round":2,"text":"cut off here`,
			wantErr: "line 2: probe event",
		},
		{
			// The probe pass only reads "type", so a bad field survives it and
			// fails on the second pass instead.
			name: "wrong type for field",
			log: `{"type":"exchange","round":"seven"}
`,
			wantErr: "line 1: unmarshal exchange event",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewEventReader(strings.NewReader(tt.log))

			var err error
			for {
				_, err = r.Next()
				if err != nil {
					break
				}
			}

			if errors.Is(err, io.EOF) {
				t.Fatalf("reached EOF without an error, want %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// A bad line costs that line and nothing else. Scan consumes the line before
// it is parsed, so the error surfaces after the reader has already moved on and
// the next call resumes normally. A corrupt record in the middle of a log does
// not cost the records after it.
func TestBadLineDoesNotLoseOtherEvents(t *testing.T) {
	log := `{"type":"exchange","round":1}
{"type":"exchange","round":2}
{"type":"garbage","round":3}
{"type":"exchange","round":4}
`
	r := NewEventReader(strings.NewReader(log))

	var rounds []int
	var errs []string
	for {
		ev, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		rounds = append(rounds, ev.headers().Round)
	}

	// Round 4 is the point: it comes after the bad line and still arrives.
	wantRounds := []int{1, 2, 4}
	if !reflect.DeepEqual(rounds, wantRounds) {
		t.Errorf("rounds read = %v, want %v", rounds, wantRounds)
	}

	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0], "line 3") {
		t.Errorf("error = %q, want it to name line 3", errs[0])
	}
}

// bufio.Scanner defaults to a 64KB line limit, and a belief plus surrounding
// fields will exceed that in a real run. This fails against a default scanner,
// which is the whole reason NewEventReader configures the buffer.
func TestLongLine(t *testing.T) {
	const size = 200 * 1024

	want := &ExchangeEvent{
		CommonHeaders: CommonHeaders{Round: 1, Ts: testDate, Type: TypeExchange},
		MsgID:         "msg-1",
		Text:          strings.Repeat("x", size),
	}

	got := readAll(t, writeAll(t, want))
	if len(got) != 1 {
		t.Fatalf("read %d events, want 1", len(got))
	}

	ex, ok := got[0].(*ExchangeEvent)
	if !ok {
		t.Fatalf("got %T, want *ExchangeEvent", got[0])
	}
	if len(ex.Text) != size {
		t.Errorf("text length = %d, want %d", len(ex.Text), size)
	}
}

// Past the cap the scanner cannot proceed, and that has to surface as an error
// rather than as a silent short read that looks like a clean end of file.
func TestLineOverCapErrors(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(`{"type":"exchange","round":1,"text":"`)
	buf.WriteString(strings.Repeat("x", maxLineSize+1))
	buf.WriteString(`"}` + "\n")

	r := NewEventReader(bytes.NewReader(buf.Bytes()))

	_, err := r.Next()
	if err == nil {
		t.Fatal("no error on a line past the cap")
	}
	if errors.Is(err, io.EOF) {
		t.Fatalf("over-cap line reported as EOF, want a real error: %v", err)
	}
	if !errors.Is(err, bufio.ErrTooLong) {
		t.Logf("error was %q", err)
	}
}

// An empty log is a valid log — a run that died before its first event. It
// reads as an immediate EOF, not an error.
func TestEmptyLog(t *testing.T) {
	r := NewEventReader(strings.NewReader(""))

	_, err := r.Next()
	if !errors.Is(err, io.EOF) {
		t.Errorf("err = %v, want io.EOF", err)
	}
}

// Next keeps returning io.EOF once the log is exhausted, so a caller that
// misses the first one does not loop forever or panic.
func TestEOFIsRepeatable(t *testing.T) {
	r := NewEventReader(strings.NewReader(`{"type":"exchange","round":1}` + "\n"))

	if _, err := r.Next(); err != nil {
		t.Fatalf("first event: %v", err)
	}
	for i := range 3 {
		if _, err := r.Next(); !errors.Is(err, io.EOF) {
			t.Fatalf("call %d after the last event: err = %v, want io.EOF", i+1, err)
		}
	}
}

// writeAll sends events through an EventWriter and returns the JSONL bytes,
// so reader tests exercise output the writer actually produced.
func writeAll(t *testing.T, events ...Event) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := NewEventWriter(&buf)
	for i, e := range events {
		if err := w.Write(e); err != nil {
			t.Fatalf("write event %d: %v", i, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.Bytes()
}

// readAll drains an EventReader, failing on any error other than io.EOF.
func readAll(t *testing.T, data []byte) []Event {
	t.Helper()

	r := NewEventReader(bytes.NewReader(data))

	var got []Event
	for {
		ev, err := r.Next()
		if errors.Is(err, io.EOF) {
			return got
		}
		if err != nil {
			t.Fatalf("next: %v", err)
		}
		if ev == nil {
			t.Fatal("Next returned a nil event with a nil error")
		}
		got = append(got, ev)
	}
}
