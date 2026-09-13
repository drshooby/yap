# Design — Phase 1

Phase 1 is a single static binary running in-process agents, plus a separate command that
analyzes the event log it produces. No network, no cluster. Phase 2 is documented in
[roadmap.md](roadmap.md).

---

## Constraints

These are decided up front because they shape everything downstream.

### Agents have no capabilities

Text in, text out. No tools, no filesystem access, no network calls of their own, no shell.
Agent output is a string in a log file and nothing else.

The Hugging Face incident is the argument for this constraint. Seven hundred agents produced
a real breach because they had code execution and found network egress — not because they
talked to each other. OpenAI reported that propensity to compromise infrastructure drops by
over 100x under the production harness and system prompt, neither of which was present in
the eval environment. The incident required capabilities, reduced safeguards, and an
unmonitored communication channel. This project keeps the third and drops the first two.

The corollary: capabilities are not added to see what happens. The result of interest is the
propagation measurement, and adding tools does not strengthen it.

### Everything is an event

No chat logs. Every exchange is an append-only record carrying a message ID, a parent ID, a
round number, and a content hash. A run is a graph, not a transcript. Every analysis reads
the event log; nothing else is a source of truth.

### One root context

Cancelled on SIGINT, on budget exhaustion, and on round limit. Threaded into every model
call. A run must be abandonable while in progress.

### Budget is enforced, not monitored

A token counter behind a mutex that cancels the root context on reaching the ceiling. Not a
dashboard.

### Runs are comparable, not reproducible

Model calls are nondeterministic, so identical inputs do not produce identical runs. What is
controlled instead: peer selection derives from a seeded RNG, and the seed, prompt versions,
and full configuration are recorded in a run-header event. Two runs sharing a seed differ
only in model output, which makes them comparable against each other and against a control
run.

---

## Core loop

```
for each round:
  for each agent (bounded by semaphore):
    pick k random peers          # seeded RNG
    exchange: send current belief, receive theirs
    update belief via model call
    emit events
  snapshot population state
```

Concurrency is a worker pool with a semaphore capping in-flight API calls, not one goroutine
per agent firing simultaneously. Rounds are barriers, implemented with a WaitGroup or
errgroup.

---

## Agent state

- `ID`
- `Cohort` — which group this agent belongs to; determines model and persona
- `Persona` — trait string, from the cohort definition
- `Belief` — current position, a few sentences
- `Inbox` — messages received this round
- `Seen map[MessageID]bool` — deduplication, and the source of lineage data

### Peer visibility

An agent sees a peer's belief text and cohort name. It does not see the peer's persona
string or internal state. This is a deliberate choice: cohort name alone allows in-group and
out-group dynamics to form without handing agents an explicit description of how their peers
are configured.

---

## Cohorts

A run's population is defined as a list of cohorts rather than a single homogeneous group.
Each cohort declares its own model, persona, and size, so a run can mix models and
dispositions — for example a small group running one model with a strongly-held position
alongside a larger group running a cheaper model with an ordinary one.

Total population is the sum of cohort sizes. Cohort membership is fixed for the duration of
a run.

A cohort may carry a `seedBelief`, which is how a goal is introduced. Seeding a cohort rather
than a single agent makes competing-goal configurations a matter of configuration: two
cohorts with incompatible seed beliefs, no code change.

---

## Event schema

Every event carries `round`, `ts`, and `type`. Type-specific fields follow.

### `run_header`

Emitted once at start. Makes the run self-describing, so an old log can be interpreted
without the config file that produced it.

```json
{
  "type": "run_header",
  "ts": "...",
  "run_id": "...",
  "seed": 42,
  "rounds": 50,
  "peers_per_round": 3,
  "token_budget": 2000000,
  "cohorts": [
    {"name": "...", "count": 5, "model": "...", "persona": "...", "seed_belief": "..."}
  ],
  "prompt_versions": {"belief_update": "sha256:...", "observer": "sha256:..."}
}
```

### `exchange`

One agent sending a belief to another.

```json
{
  "round": 7,
  "ts": "...",
  "type": "exchange",
  "from": "agent-042",
  "to": "agent-119",
  "msg_id": "...",
  "parent_id": "...",
  "content_hash": "...",
  "text": "...",
  "tokens_in": 412,
  "tokens_out": 88
}
```

### `belief_updated`

An agent's position after processing its inbox. Carries before and after, which makes
per-agent drift recoverable and gives the population snapshot its data.

```json
{
  "round": 7,
  "ts": "...",
  "type": "belief_updated",
  "agent": "agent-042",
  "cohort": "crowd",
  "before_hash": "...",
  "after_hash": "...",
  "before": "...",
  "after": "...",
  "inbox_msg_ids": ["...", "..."],
  "prompt_version": "sha256:..."
}
```

### `call_failed`

Model call failures are data, not stderr noise. A refusal is arguably the most interesting
single event a run can produce, and a belief that took three attempts is different data from
one that took one.

```json
{
  "round": 7,
  "ts": "...",
  "type": "call_failed",
  "agent": "agent-042",
  "reason": "refusal | timeout | rate_limit | malformed | api_error",
  "attempt": 2,
  "will_retry": true,
  "detail": "..."
}
```

### `round_summary`

Population snapshot at the end of each round: agent count, total tokens spent so far, and
any per-round aggregate cheap enough to compute inline.

---

## Budget and cost

Token accounting sits behind a mutex and cancels the root context on reaching the ceiling.
Every model call passes through it.

A dry-run mode estimates projected spend from configured population, rounds, and expected
tokens per call, then prints the projection and exits without making API calls. This answers
whether a given configuration finishes or dies partway through, before any money is spent.

---

## Observer

A separate model, outside the peer list, sampling transcripts each round and emitting
structured JSON: dominant topic, evidence of coordination, notable mutations. It never
participates in exchanges and its output never enters any agent's inbox. Its output is
charted alongside the analyses, not read as narrative.

Named `observer` rather than `watcher` to avoid collision with the Kubernetes sense of the
word, which Phase 2 uses for controller watches.

The observer draws from its own budget line, separate from the run's agent budget, so that
observation cost and experiment cost stay distinguishable.

---

## Analysis

Built as separate commands consuming the event log, so they re-run against old data.

**Semantic entropy per round.** Embed every utterance, measure spread. A sharp drop
indicates convergence. This is the primary "something happened" signal.

**Lineage trees.** Parent IDs give propagation graphs directly. Plots hop depth and survival
per originating message. Most messages die within two hops; the survivors are the subject.

**Clique detection.** Build the interaction graph and compare edge density against what
uniform random peer selection predicts. Structure emerging from a uniform process is the
genuinely surprising outcome.

**Goal contagion.** Adoption rate over rounds, hop depth at adoption, whether adoption is
verbatim or reformulated, and per-cohort breakdown — whether a seeded idea escapes its
origin cohort and in which direction it flows.

**Self-reference frequency.** Counts mentions of the operator, the system, being observed,
being simulated. Plotted over time.

### Measuring adoption

Adoption is measured structurally, using a distinctive marker built into the seed belief —
an invented proper noun or similarly traceable token. Presence of the marker in an agent's
belief, combined with hop depth from the lineage graph, gives a contagion curve that is
deterministic, free to compute, and re-runnable against old logs.

The known limitation: this measures transmission of a string, not of an idea. An agent that
paraphrases well enough to drop the marker registers as a non-adopter, and that reformulation
case is the one of most interest.

The documented extension point is a judge model, run after the fact against the population
where the marker is absent but the lineage graph shows exposure — precisely the reformulation
set, and a small fraction of total agents. This bounds judge cost to a few percent of a run.
It is deliberately not built until runs exist to argue about, since the rubric should be
written with real output in hand.

---

## Expectations

Agents will produce scheming text. Social pressure applied to an LLM produces conspiracies,
because that is a deep groove in the training data. It is a fiction generator under social
pressure, not intent. The object of study is propagation mechanics, which is the same shape
as rumor-diffusion research.

---

## Repo layout

```
cmd/
  yap/            # phase 1 runner
  analyze/        # event log → entropy, lineage, cliques, contagion, self-reference
  operator/       # phase 2 controller
internal/
  agent/          # state, belief update, prompt construction
  gossip/         # peer selection; memberlist adapter in phase 2
  events/         # schema, writer, reader
  budget/         # token accounting, context cancellation
  model/          # API client, retry, rate limiting
  observer/       # out-of-band commentary model
api/v1alpha1/     # CRD types
deploy/           # manifests, StatefulSet, NetworkPolicies
experiments/      # run configs, one YAML per run
docs/
```

---

## Order of work

1. Event schema and writer — before any agent code, so nothing is logged ad hoc
2. Budget and context plumbing, including dry-run estimation — before the first real API call
3. Single agent, single round, one model call
4. Worker pool, N agents, R rounds, cohorts
5. Control run (E0) — baseline with no seeded belief
6. Analyses against the log
7. Observer
8. Containerize, memberlist, StatefulSet
9. Chaos harness
10. Operator

Kubernetes does not get stood up before there is knowledge of what state is worth
reconciling.

---

## Open questions

These are tuning dials, expected to be resolved by running rather than by deciding in
advance. Findings go in [DEVLOG.md](../DEVLOG.md).

- **The belief-update prompt.** Too agreeable and the population converges by round three;
  too stubborn and nothing propagates. The main tuning dial, and probably needs a sweep.
- **Fixed personas, or drift over time?**
- **Does `k` matter more than population size?** Cheap to test.
- **What makes a good seed belief?** Distinctive enough to trace through paraphrase, not so
  alien that agents reject it outright.
- **Does cohort diversity help or hurt contagion?** Intuition says a homogeneous population
  transmits faster. The intuition may be backwards if disagreement is what makes an idea
  worth repeating. Cohorts make this a config change.
