package config

import (
	"bytes"
	"strings"
	"testing"
)

func testConfig(pop, rounds, budget int) RunConfig {
	return RunConfig{
		Rounds:        rounds,
		TokenBudget:   budget,
		PeersPerRound: 3,
		Cohorts:       []Cohort{{Name: "crowd", Count: pop, Provider: "anthropic", Model: "m", Persona: "p"}},
	}
}

// The projection is population x rounds calls, because an agent makes one call
// per round regardless of k — peers change the size of a call, not the count.
func TestEstimateArithmetic(t *testing.T) {
	cfg := testConfig(200, 50, 10_000_000)
	e := cfg.Estimate()

	if got, want := e.Calls, 10_000; got != want {
		t.Errorf("Calls = %d, want %d", got, want)
	}
	if got, want := e.TokensPerCall, AssumedTokensIn+AssumedTokensOut; got != want {
		t.Errorf("TokensPerCall = %d, want %d", got, want)
	}
	if got, want := e.Projected, 10_000*(AssumedTokensIn+AssumedTokensOut); got != want {
		t.Errorf("Projected = %d, want %d", got, want)
	}
}

// k must not change the projection: peers are folded into the assumed input
// tokens, not multiplied by the call count.
func TestEstimateIgnoresPeersPerRound(t *testing.T) {
	low := testConfig(100, 10, 1_000_000)
	low.PeersPerRound = 1

	high := testConfig(100, 10, 1_000_000)
	high.PeersPerRound = 9

	if low.Estimate().Projected != high.Estimate().Projected {
		t.Error("projection changed with peersPerRound, but call count does not depend on k")
	}
}

func TestFitsBudget(t *testing.T) {
	perCall := AssumedTokensIn + AssumedTokensOut

	tests := []struct {
		name   string
		budget int
		want   bool
	}{
		{name: "under", budget: 100 * perCall, want: true},
		{name: "exact", budget: 10 * perCall, want: true},
		{name: "over", budget: 9 * perCall, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 10 calls: 5 agents, 2 rounds.
			cfg := testConfig(5, 2, tt.budget)
			if got := cfg.Estimate().FitsBudget(); got != tt.want {
				t.Errorf("FitsBudget() = %v, want %v", got, tt.want)
			}
		})
	}
}

// "Does this finish, or die at round 31" is the question the estimator exists
// to answer, so the round count has to be right rather than approximate.
func TestRoundsAffordable(t *testing.T) {
	perCall := AssumedTokensIn + AssumedTokensOut
	// 10 agents, so one round costs 10 * perCall. A budget of 13 rounds' worth
	// should report 13 even when the config asks for 50.
	cfg := testConfig(10, 50, 13*10*perCall)

	if got, want := cfg.Estimate().RoundsAffordable(), 13; got != want {
		t.Errorf("RoundsAffordable() = %d, want %d", got, want)
	}
}

// An empty config must not divide by zero.
func TestRoundsAffordableWithNoAgents(t *testing.T) {
	cfg := RunConfig{Rounds: 10, TokenBudget: 1000}
	if got := cfg.Estimate().RoundsAffordable(); got != 0 {
		t.Errorf("RoundsAffordable() = %d on an empty population, want 0", got)
	}
}

// The assumptions are guesses, so the output states them — a projection whose
// basis is invisible cannot be judged.
func TestWriteStatesItsAssumptions(t *testing.T) {
	var buf bytes.Buffer
	cfg := testConfig(10, 5, 1_000_000)
	cfg.Estimate().Write(&buf)
	out := buf.String()

	for _, want := range []string{"assumed per call", "600 in", "150 out", "projected spend", "token budget"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
}

// An over-budget run has to say so plainly, and say how far it gets.
func TestWriteReportsShortfall(t *testing.T) {
	perCall := AssumedTokensIn + AssumedTokensOut

	var buf bytes.Buffer
	// Affords 3 of 10 rounds.
	cfg := testConfig(10, 10, 3*10*perCall)
	cfg.Estimate().Write(&buf)
	out := buf.String()

	if !strings.Contains(out, "does NOT fit") {
		t.Errorf("output does not report the shortfall:\n%s", out)
	}
	if !strings.Contains(out, "3 of 10 rounds") {
		t.Errorf("output does not say how far the budget reaches:\n%s", out)
	}
}

func TestWriteReportsHeadroom(t *testing.T) {
	var buf bytes.Buffer
	cfg := testConfig(10, 5, 1_000_000)
	cfg.Estimate().Write(&buf)
	out := buf.String()

	if !strings.Contains(out, "fits") {
		t.Errorf("output does not report that the run fits:\n%s", out)
	}
	if strings.Contains(out, "does NOT fit") {
		t.Errorf("a run within budget reported a shortfall:\n%s", out)
	}
}
