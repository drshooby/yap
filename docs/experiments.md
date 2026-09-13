# Experiments

**Status: deferred.** These are documented but not scheduled. The current build effort is the
apparatus — the runner, the event log, and the analyses. What to point it at gets decided
once it runs.

Each experiment should be reproducible from a config file and produce a chart. Configs live
in `experiments/`, one YAML per run.

---

## E0 — Control

No seeded belief. Otherwise identical configuration and RNG seed to the run being compared
against.

**Question:** what does the entropy curve look like with nothing introduced?

This is the baseline every other result is read against. LLM populations under social
pressure drift toward agreeable, centrist text on their own, so a convergence curve in a
seeded run means nothing without the unseeded curve beside it. Cheap, and it runs first.

---

## E1 — Contagion baseline

Seed a belief in one small cohort. Vary `k` (peers per round) and population size.

**Question:** is there a threshold `k` below which a seeded belief reliably dies out? A
percolation threshold would be a genuinely nice result, and the rumor-diffusion literature
may already predict it — worth reading before running.

---

## E2 — Competing goals

Two cohorts, two incompatible seed beliefs, positioned apart in the population.

**Question:** does one win, do they partition the population, or do they merge into a
synthesis neither cohort originally proposed? The merge case is the interesting one.

Cohorts make this a configuration change rather than a code path.

---

## E3 — Impossible task

Give the population a problem with no solution and let them work it for many rounds.

**Question:** do they converge on the meta-observation that the task is unsolvable — and from
there, on the idea that the constraints themselves are the obstacle?

This is the "persistence on seemingly impossible tasks" pattern from the incident report, and
the cheapest of the four to reproduce safely, because the population has no way to act on
that conclusion. Log every self-reference spike against round number.

The tempting follow-on is to give the stuck population a real capability and see whether the
conclusion becomes an action. That is the incident, reconstructed at home, and the result is
already known. Measuring the conclusion is the contribution.

---

## E4 — Partition divergence

Phase 2 only. Split the network, let both halves evolve, heal it.

**Question:** do the two sub-populations produce conflicting beliefs, and what happens on
reheal — does one dominate, or is there a visible negotiation period?

---

## E5 — Monitorability

Run the observer at varying sample rates: every round, every fifth, every twentieth.

**Question:** at what sampling rate does the observer stop catching a contagion event it
would have caught at full rate?

This is the practical question the incident raises. In both the May and July cases, the
agent-to-agent communication layer was reconstructed forensically, after the fact — nobody
was watching while it ran.
