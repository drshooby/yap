# Devlog

Newest first. Build notes and run results, interleaved.

---

## 2026-09-13 — Design session: docs restructure, cohorts, schema additions

Started with a README that had grown to ~320 lines and was doing the job of four documents.
Split it: short README, `docs/design.md` for Phase 1 architecture, `docs/roadmap.md` for
Phase 2, `docs/experiments.md` for the runs. `related-work.md` unchanged. This devlog is new.

**Experiments deferred.** E1–E5 stay fully documented but stop being commitments. The
apparatus is the thing being built; what to point it at gets decided once it runs. The
analyses are *not* deferred — they constrain the event schema, so deferring them would mean
regenerating every log produced before they landed.

**Cohorts replace the homogeneous population.** The old CRD had `agents`, `personaSet`, and
`model` as flat fields, which quietly assumed every agent was identical. It couldn't express
something as basic as a small group on one model with a strong position alongside a larger
group on a cheaper model. Now `spec.cohorts` is a list, each with its own count, model,
persona, and optional `seedBelief`. `agents` is gone — population is the sum of counts, which
also removes a validation failure mode where the two could disagree.

Knock-on benefit: "does cohort diversity help or hurt contagion?" goes from unanswerable to a
config change, and per-cohort adoption rates fall out of the contagion analysis for free.

**CRD renamed `Yap` → `Swarm`.** Briefly considered naming the kind `Cohort`, which would
have produced `kind: Cohort` containing `cohorts: [...]` — recursive-looking and confusing to
read cold. `Swarm` is the run; cohorts live inside it.

**Schema additions.** Five gaps found reading the old README with fresh eyes, three of them
schema-level and therefore cheap now and expensive later:

- `run_header` event carrying RNG seed, full config, and prompt version hashes. Runs still
  aren't reproducible — the model varies — but they become *comparable*, which is the
  property actually needed.
- `call_failed` as a typed event. Refusals, timeouts, rate limits, malformed responses. A
  refusal is arguably the most interesting thing a run can produce and it was previously
  going to stderr.
- `belief_updated` carrying before and after, so per-agent trajectory over N rounds is
  recoverable rather than just the current position.

Two operational: dry-run cost estimation before spending money, and E0 as a control run.
The control matters more than it first appears — LLM populations drift toward agreeable
centrist text on their own, so a convergence curve in a seeded run means nothing without the
unseeded curve beside it.

**Adoption measurement.** Settled on marker-based tracking for now: a distinctive invented
token in the seed belief, presence checked against the lineage graph. Deterministic, free,
re-runnable. Known limitation is that it measures transmission of a string rather than an
idea, and drops exactly the reformulation case that's most interesting. Judge model documented
as an extension point, pointed only at agents where the marker is absent but lineage shows
exposure — that's the reformulation set and a small fraction of the population, so it bounds
the cost. Not building it until there are runs to argue about and the rubric can be written
against real output.

**Kubernetes: control plane vs. data plane.** Worth writing down because it wasn't obvious.
Using the API server as a distributed state machine is tempting for round coordination — pods
report completion into status, controller advances `currentRound`, pods watch. It works, and
it defeats the purpose: the API server stays reachable through any NetworkPolicy applied
between pods, so it would partition the gossip plane while leaving coordination intact. That
engineers away the exact behavior E4 exists to observe. Budget aggregation, lifecycle, and
chaos scheduling are fine there. Rounds and exchanges stay pod-to-pod.

Also flagged that `memberlist` gossips *membership*, not payloads — its user-message facility
is best-effort and small, so it can't carry belief text. Discovery and failure detection only,
with a separate connection for exchanges. That decision changes what Phase 2 is, so it comes
first. Six open mechanics recorded in the roadmap rather than guessed at.

A `Partition` CRD is parked as a maybe — declarative chaos, fault windows recorded into the
same event log as everything else. Two CRDs is a lot of Kubernetes ahead of any evidence.

**Naming.** `yap` stays as repo, module, and binary. It had also been used as a common noun
throughout the prose — "a yap of 700 agents" — which read as flippant about an incident with
subpoenas attached. Prose now uses swarm/network/run; the joke lands once, in the title.

**Renamed the Phase 1 observer.** Was "watcher", which collides with the Kubernetes sense of
the word that Phase 2 uses constantly. Now `observer`.

Next: event schema and writer, before any agent code.
