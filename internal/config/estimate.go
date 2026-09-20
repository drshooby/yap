package config

import (
	"fmt"
	"io"
	"text/tabwriter"
)

// Claude Note:
// Assumed per-call token counts. These are guesses, not measurements: a belief
// update sends a persona, the agent's current belief, and k peer beliefs, then
// returns one belief. They exist so a run's cost is knowable before it starts,
// and the estimate prints them so the number can be judged rather than trusted.
//
// Replace them with figures from a real run once E0 has produced one.
const (
	AssumedTokensIn  = 600
	AssumedTokensOut = 150
)

// Estimate is a projection of what a run will cost, derived from the config
// alone. Every field is reported so the arithmetic is checkable.
type Estimate struct {
	Population    int
	Rounds        int
	Calls         int
	TokensPerCall int
	Projected     int
	Budget        int
}

// Estimate projects the token cost of a run.
//
// One model call per agent per round: an agent reads its inbox and writes one
// belief, whatever k is. Peers affect the size of a call, not the number of
// them, and that is already folded into the assumed input tokens.
func (c *RunConfig) Estimate() Estimate {
	pop := c.Population()
	calls := pop * c.Rounds
	perCall := AssumedTokensIn + AssumedTokensOut

	return Estimate{
		Population:    pop,
		Rounds:        c.Rounds,
		Calls:         calls,
		TokensPerCall: perCall,
		Projected:     calls * perCall,
		Budget:        c.TokenBudget,
	}
}

// FitsBudget reports whether the projection completes within the ceiling.
func (e Estimate) FitsBudget() bool {
	return e.Projected <= e.Budget
}

// RoundsAffordable is how many rounds the budget covers at the projected rate,
// which answers "does this finish, or die partway" directly.
func (e Estimate) RoundsAffordable() int {
	perRound := e.Population * e.TokensPerCall
	if perRound == 0 {
		return 0
	}
	return e.Budget / perRound
}

// Write prints the projection, including the assumptions behind it.
func (e Estimate) Write(w io.Writer) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	fmt.Fprintf(tw, "population\t%d agents\n", e.Population)
	fmt.Fprintf(tw, "rounds\t%d\n", e.Rounds)
	fmt.Fprintf(tw, "model calls\t%d\n", e.Calls)
	fmt.Fprintf(tw, "assumed per call\t%d tokens (%d in, %d out)\n",
		e.TokensPerCall, AssumedTokensIn, AssumedTokensOut)
	fmt.Fprintf(tw, "projected spend\t%d tokens\n", e.Projected)
	fmt.Fprintf(tw, "token budget\t%d\n", e.Budget)
	tw.Flush()

	fmt.Fprintln(w)
	if e.FitsBudget() {
		headroom := e.Budget - e.Projected
		fmt.Fprintf(w, "fits, with %d tokens (%d%%) to spare\n",
			headroom, percent(headroom, e.Budget))
		return
	}

	affordable := e.RoundsAffordable()
	fmt.Fprintf(w, "does NOT fit: the budget covers about %d of %d rounds\n", affordable, e.Rounds)
	fmt.Fprintf(w, "raise tokenBudget to ~%d, or cut rounds to %d\n", e.Projected, affordable)
}

func percent(part, whole int) int {
	if whole == 0 {
		return 0
	}
	return part * 100 / whole
}
