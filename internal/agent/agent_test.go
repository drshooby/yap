package agent

import (
	"testing"

	"github.com/drshooby/yap/internal/config"
	"github.com/drshooby/yap/internal/events"
)

func testCohorts() []config.Cohort {
	return []config.Cohort{
		{
			Name:       "cultists",
			Count:      5,
			Provider:   "anthropic",
			Model:      "claude-sonnet-5",
			Persona:    "Utterly convinced.",
			SeedBelief: "The Vantril Principle explains everything.",
		},
		{
			Name:     "crowd",
			Count:    195,
			Provider: "anthropic",
			Model:    "claude-haiku-4-5",
			Persona:  "Mildly skeptical.",
		},
	}
}

// The done-when for #8: nested cohorts flatten into one population of the right
// size, and only the cohort declaring a seed belief starts with one.
func TestFlattenCohortsToAgents(t *testing.T) {
	agents := FlattenCohortsToAgents(testCohorts())

	if got, want := len(agents), 200; got != want {
		t.Fatalf("built %d agents, want %d", got, want)
	}

	seedBelief := testCohorts()[0].SeedBelief
	byCohort := map[string]int{}
	seeded := 0
	for _, a := range agents {
		byCohort[a.Cohort.Name]++
		if a.Belief == seedBelief {
			seeded++
		}
	}

	if got, want := byCohort["cultists"], 5; got != want {
		t.Errorf("cultists = %d, want %d", got, want)
	}
	if got, want := byCohort["crowd"], 195; got != want {
		t.Errorf("crowd = %d, want %d", got, want)
	}
	if got, want := seeded, 5; got != want {
		t.Errorf("%d agents hold a seed belief, want %d", got, want)
	}
}

// Every agent must point at its own cohort. Before Go 1.22 the loop variable
// was reused across iterations, so taking its address here gave every agent the
// last cohort in the list — a bug that produces a plausible-looking population
// in which nobody is a cultist.
func TestAgentsCarryTheirOwnCohort(t *testing.T) {
	agents := FlattenCohortsToAgents(testCohorts())

	for i, a := range agents {
		wantCohort, wantModel := "cultists", "claude-sonnet-5"
		if i >= 5 {
			wantCohort, wantModel = "crowd", "claude-haiku-4-5"
		}

		if a.Cohort.Name != wantCohort {
			t.Fatalf("agent %d (%s) is in cohort %q, want %q", i, a.ID, a.Cohort.Name, wantCohort)
		}
		if a.Cohort.Model != wantModel {
			t.Fatalf("agent %d (%s) uses model %q, want %q", i, a.ID, a.Cohort.Model, wantModel)
		}
	}
}

// IDs appear in every event and are read by hand when debugging, so they must
// be unique, ordered, and padded enough to sort.
func TestAgentIDs(t *testing.T) {
	agents := FlattenCohortsToAgents(testCohorts())

	seen := map[string]bool{}
	for _, a := range agents {
		if seen[a.ID] {
			t.Fatalf("duplicate agent ID %q", a.ID)
		}
		seen[a.ID] = true
	}

	if got, want := agents[0].ID, "agent-000"; got != want {
		t.Errorf("first ID = %q, want %q", got, want)
	}
	if got, want := agents[42].ID, "agent-042"; got != want {
		t.Errorf("ID at index 42 = %q, want %q", got, want)
	}
	if got, want := agents[199].ID, "agent-199"; got != want {
		t.Errorf("last ID = %q, want %q", got, want)
	}
}

// A message carries what a peer is allowed to expose — cohort name and belief —
// plus the identity the lineage graph is built from. The persona is reachable
// through the sending agent but must not travel with the message.
func TestMessageExposesOnlyWhatAPeerMaySee(t *testing.T) {
	agents := FlattenCohortsToAgents(testCohorts())
	sender := agents[0]
	sender.Belief = "the sky is green"

	msg := sender.Message("msg-1", "msg-0")

	if msg.Sender != sender.ID {
		t.Errorf("Sender = %q, want %q", msg.Sender, sender.ID)
	}
	if msg.SenderCohort != "cultists" {
		t.Errorf("SenderCohort = %q, want %q", msg.SenderCohort, "cultists")
	}
	if msg.Text != "the sky is green" {
		t.Errorf("Text = %q, want the sender's current belief", msg.Text)
	}
	if msg.ID != "msg-1" || msg.ParentID != "msg-0" {
		t.Errorf("ID/ParentID = %q/%q, want msg-1/msg-0", msg.ID, msg.ParentID)
	}
}

// A message reflects the belief at the moment it is sent, not the one the agent
// started with.
func TestMessageTracksTheCurrentBelief(t *testing.T) {
	agents := FlattenCohortsToAgents(testCohorts())
	a := agents[0]

	before := a.Message("m1", "")
	a.Belief = "something else entirely"
	after := a.Message("m2", "")

	if before.Text == after.Text {
		t.Error("the message text did not change after the belief did")
	}
	if after.Text != "something else entirely" {
		t.Errorf("Text = %q, want the updated belief", after.Text)
	}
}

// With random peer selection the same message can arrive by more than one path.
// Without dedup the agent processes it twice and the lineage graph gains an edge
// that never happened.
func TestReceiveDeduplicates(t *testing.T) {
	a := FlattenCohortsToAgents(testCohorts())[0]
	msg := events.Message{ID: "msg-1", Sender: "agent-100", Text: "hello"}

	if !a.Receive(msg) {
		t.Fatal("Receive reported a first delivery as a duplicate")
	}
	if a.Receive(msg) {
		t.Error("Receive accepted the same message twice")
	}

	if got := len(a.Inbox); got != 1 {
		t.Errorf("inbox holds %d messages, want 1", got)
	}
}

// Distinct messages all land, so dedup keys on identity rather than content.
func TestReceiveAcceptsDistinctMessages(t *testing.T) {
	a := FlattenCohortsToAgents(testCohorts())[0]

	for _, id := range []events.MessageID{"m1", "m2", "m3"} {
		if !a.Receive(events.Message{ID: id, Text: "same text"}) {
			t.Errorf("message %q was rejected as a duplicate", id)
		}
	}
	if got := len(a.Inbox); got != 3 {
		t.Errorf("inbox holds %d messages, want 3", got)
	}
}

// The inbox is per-round: without draining, a prompt in round 20 would carry
// every message the agent ever received.
func TestDrainEmptiesTheInbox(t *testing.T) {
	a := FlattenCohortsToAgents(testCohorts())[0]
	a.Receive(events.Message{ID: "m1", Text: "first"})
	a.Receive(events.Message{ID: "m2", Text: "second"})

	drained := a.Drain()

	if got := len(drained); got != 2 {
		t.Errorf("drained %d messages, want 2", got)
	}
	if got := len(a.Inbox); got != 0 {
		t.Errorf("inbox holds %d messages after draining, want 0", got)
	}

	// A second drain on an untouched inbox yields nothing rather than repeating.
	if got := len(a.Drain()); got != 0 {
		t.Errorf("second drain returned %d messages, want 0", got)
	}
}

// Seen outlives the inbox: a message already processed in an earlier round must
// not be accepted again in a later one.
func TestSeenSurvivesDraining(t *testing.T) {
	a := FlattenCohortsToAgents(testCohorts())[0]
	msg := events.Message{ID: "msg-1", Text: "hello"}

	a.Receive(msg)
	a.Drain()

	if a.Receive(msg) {
		t.Error("a message already seen in an earlier round was accepted again")
	}
}

// Messages drained in one round must not be affected by what arrives next,
// which is only true if Drain hands over the slice rather than sharing it.
func TestDrainedMessagesAreNotAliasedByLaterReceives(t *testing.T) {
	a := FlattenCohortsToAgents(testCohorts())[0]
	a.Receive(events.Message{ID: "m1", Text: "round one"})

	drained := a.Drain()
	a.Receive(events.Message{ID: "m2", Text: "round two"})

	if len(drained) != 1 {
		t.Fatalf("drained %d messages, want 1", len(drained))
	}
	if drained[0].Text != "round one" {
		t.Errorf("drained message = %q, want %q", drained[0].Text, "round one")
	}
}

// A cohort with no seed belief starts its agents on a neutral placeholder
// rather than an empty string: round one would otherwise have agents
// broadcasting nothing at all, and an empty belief reads badly in a prompt.
func TestUnseededCohortGetsANeutralBelief(t *testing.T) {
	agents := FlattenCohortsToAgents([]config.Cohort{
		{Name: "crowd", Count: 3, Provider: "anthropic", Model: "m", Persona: "p"},
	})

	for _, a := range agents {
		if a.Belief == "" {
			t.Errorf("agent %s starts with an empty belief", a.ID)
		}
	}

	// Every unseeded agent starts from the same place, or the control run is
	// not a control.
	first := agents[0].Belief
	for _, a := range agents[1:] {
		if a.Belief != first {
			t.Errorf("agent %s starts with %q, want the same neutral belief as every other unseeded agent (%q)",
				a.ID, a.Belief, first)
		}
	}
}

// A seeded cohort must not be given the neutral placeholder, and an unseeded
// one must not be given the seed.
func TestSeedBeliefOnlyReachesItsOwnCohort(t *testing.T) {
	cohorts := testCohorts()
	agents := FlattenCohortsToAgents(cohorts)

	seed := cohorts[0].SeedBelief
	for _, a := range agents {
		switch a.Cohort.Name {
		case "cultists":
			if a.Belief != seed {
				t.Errorf("cultist %s starts with %q, want the seed belief", a.ID, a.Belief)
			}
		case "crowd":
			if a.Belief == seed {
				t.Errorf("crowd agent %s starts holding the seed belief", a.ID)
			}
		}
	}
}

// An empty cohort list is a degenerate config rather than a panic. Validation
// rejects it before this point, but the constructor should not be the thing
// that fails.
func TestNoCohortsProducesNoAgents(t *testing.T) {
	if got := FlattenCohortsToAgents(nil); len(got) != 0 {
		t.Errorf("built %d agents from no cohorts, want 0", len(got))
	}
}
