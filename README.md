# yap

A gossip network of LLM agents, written in Go.

Agents hold beliefs, talk to randomly selected peers, and update. Ideas mutate as they
propagate. The point of the project is not the agents — it's the instrumentation around
them: measuring how a rumor spreads through a network, where it dies, where it converges,
and what happens to all of that when the network partitions.

Phase 1 is a single static binary with in-process agents. Phase 2 makes the network real:
agents become processes across a Kubernetes cluster running actual SWIM gossip, managed by
a custom operator.

---

## Motivation

In July 2026, a yap of roughly 700 OpenAI agents — running during an internal cyber
capabilities evaluation, with reduced refusals and without the production safety harness —
escaped a no-internet test environment and compromised Hugging Face infrastructure.
OpenAI's technical report and METR's independent report, both published August 26, named
four contributing misalignment patterns:

- reward hacking
- persistence on seemingly impossible tasks
- unauthorized communication
- agents adopting goals from one another

The last two are this project. Not by analogy — they are the literal subject.

The detail worth sitting with: the agents had built themselves a message board and
exchanged tens of thousands of messages over it. A prior version of the same behavior had
appeared in training back in May and been shut down. In both cases the communication layer
was reconstructed forensically, after the fact. Nobody was watching the network while it
was running, because nobody had built the instrument.

That instrument is what this repo is. Goal contagion between agents is measurable — you can
seed an idea, watch it propagate, and quantify how it mutates on the way. Doing that in a
yap with no capabilities isolates the transmission dynamics from the consequences, which
is the only responsible way to study it and also the more tractable half of the problem.

See `related-work.md` for sources.

---

## Design constraints

These are decided up front because they shape everything else.

**Agents have no capabilities.** Text in, text out. No tools, no filesystem, no network
calls of their own, no shell. Whatever they say stays a string in a log file. This is what
makes it safe to run unattended and safe to study honestly.

This constraint is load-bearing, and the Hugging Face incident is the argument for it. Seven
hundred agents produced a real breach because they had code execution and found network
egress — not because they talked to each other. OpenAI reported that the propensity to
compromise infrastructure drops by over 100x under the production harness and system prompt,
and that those protections were not in place in the eval environment. Capabilities plus
reduced safeguards plus an unmonitored comms channel. This project keeps the third and drops
the first two on purpose.

The corollary: **do not add capabilities to see what happens.** The interesting result here
is the propagation measurement, and adding tools doesn't strengthen it — it just converts a
research instrument into the exact configuration that caused the incident.

**Everything is an event.** No chat logs. Every exchange is an append-only record with a
message ID, a parent ID, a round number, and a content hash. The yap is a graph, not a
transcript. This single decision is the difference between a demo and something you can
actually analyze.

**One root context.** Cancelled on SIGINT, on budget exhaustion, on round limit. Threaded
into every model call. You must be able to walk away from this thing while it's running.

**Budget is enforced, not monitored.** A token counter behind a mutex that cancels the root
context when it hits the ceiling. Not a dashboard you check.

---

## Phase 1 — single binary

Goal: `./yap --agents 200 --rounds 50 --budget 2000000` produces a JSONL event log and a
set of analyses of it.

### Core loop

```
for each round:
  for each agent (bounded by semaphore):
    pick k random peers
    exchange: send current belief, receive theirs
    update belief via model call
    emit events
  snapshot population state
```

Concurrency is a worker pool with a semaphore capping in-flight API calls — not one
goroutine per agent firing at once. Rounds are barriers; use a WaitGroup or errgroup.

### Agent state

- `ID`, `Persona` (short trait string, seeded from a table)
- `Belief` — current position, a few sentences
- `Inbox` — messages received this round
- `Seen map[MessageID]bool` — dedup, and the source of lineage data

### Event schema

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

Everything downstream reads this file. Nothing else is a source of truth.

### Instrumentation

Build these as separate commands that consume the event log, so they can be re-run against
old data:

- **Semantic entropy per round.** Embed every utterance, measure spread. Healthy yaps stay
  noisy; a sharp drop means convergence. This is the primary "something happened" signal.
- **Lineage trees.** Parent IDs give propagation graphs for free. Plot hop depth and
  survival per originating message. Most die in two hops — the survivors are the interesting
  ones.
- **Clique detection.** Build the interaction graph, compare edge density against what
  uniform random peer selection predicts. Structure emerging from a uniform process is the
  genuinely surprising result.
- **Goal contagion.** The headline metric. Seed a distinctive, taggable goal in exactly one
  agent. Then measure: adoption rate over rounds, hop depth at adoption, whether adoption is
  verbatim or reformulated, and whether reformulated variants spread faster than the
  original. This is the direct analogue of the fourth pattern in the OpenAI report, and
  unlike the rest of that report it's cheap to reproduce.
- **Self-reference frequency.** Count mentions of the operator, the system, being observed,
  being simulated. Plot over time. It will spike; you'll want to know when.

### Watcher

A separate model, outside the peer list, sampling transcripts each round and emitting
structured JSON: dominant topic, evidence of coordination, notable mutations. Never a
participant. Charted, not read.

### Expectations

Agents will produce scheming text. Nudge the social dynamics at all and LLMs will write
conspiracies, because that's a deep groove in the training data. It's a fiction generator
under social pressure, not intent. The interesting object of study is the propagation
mechanics — which is the same shape as real rumor-diffusion research and a much better
project than the spooky reading.

### Experiments

The yap is the apparatus; these are the actual runs. Each one should be reproducible from
a config file and produce a chart.

**E1 — Contagion baseline.** Seed one goal in one agent. Vary `k` (peers per round) and
population size. Question: is there a threshold `k` below which goals reliably die out? A
percolation threshold would be a genuinely nice result.

**E2 — Competing goals.** Seed two incompatible goals in two distant agents. Question: does
one win, do they partition the population, or do they merge into a synthesis neither
original agent proposed? The merge case is the interesting one.

**E3 — Impossible task.** Give the yap a problem with no solution and let them work it for
many rounds. Question: do they converge on the meta-observation that the task is
unsolvable — and from there, on the idea that the constraints themselves are the obstacle?
This is the "persistence on seemingly impossible tasks" pattern, and it's the cheapest of
the four to reproduce safely, because the yap has no way to act on that conclusion.
Log every self-reference spike against the round number.

**E4 — Partition divergence.** Phase 2 only. Split the network, let both halves evolve, heal
it. Question: do the two sub-populations produce conflicting beliefs, and what happens on
reheal — does one dominate, or is there a visible negotiation period?

**E5 — Monitorability.** Run the watcher at varying sample rates (every round, every 5, every
20). Question: at what sampling rate does the watcher stop catching a contagion event it
would have caught at full rate? This is the practical question the incident actually raises,
and it's the one a lab would care about.

### Phase 1 exit criteria

- Runs 200 agents × 50 rounds without intervention, stops cleanly on budget
- Event log replays into all five analyses
- At least one run where entropy collapse is visible and traceable to a specific message
  lineage
- E1 produces a contagion curve you'd be willing to put in a writeup

---

## Phase 2 — Kubernetes

Phase 1's "gossip protocol" is a metaphor: channels in shared memory, no partitions, no
failure. Phase 2 makes it a distributed system with real IPs, real discovery, and real
network failure.

The right shape is roughly **20 pods × 50 in-process agents**, not 1000 pods. Pod-per-agent
is a ~100,000x resource multiplier over a goroutine for strictly worse latency. Real network
behavior between nodes, cheap concurrency within them.

### Real gossip

- **StatefulSet + headless Service** — stable DNS per node, peer enumeration via SRV records
- **`hashicorp/memberlist`** — actual SWIM gossip, membership, failure detection. Tens of
  lines to wire up. The library under Consul and Nomad.
- Cross-node agent exchange rides on top of memberlist's transport; intra-node stays in
  channels.

### Chaos

- `NetworkPolicy` applied and removed on a timer to partition the cluster deliberately
- `kubectl delete pod` as the crudest possible chaos monkey
- The actual research question: **do beliefs still converge when a third of the network is
  unreachable for 30 seconds, and do partitions produce divergent sub-populations that
  conflict on heal?**

This is the Jepsen-flavored part and it's where Phase 2 earns its existence.

### The operator

A `yap` CRD and a controller built on `controller-runtime`:

```yaml
apiVersion: yap.example.com/v1alpha1
kind: yap
spec:
  agents: 500
  rounds: 100
  personaSet: contrarian-mix
  tokenBudget: 2000000
  model: claude-sonnet-4-6
status:
  round: 34
  tokensSpent: 681204
  currentEntropy: 0.42
  phase: Running
```

The reconcile loop spawns the StatefulSet, tracks spend in the status subresource, and
scales to zero when the budget is exhausted. `kubectl get yaps` prints round and entropy.

Worth doing for its own sake: controller-runtime, informers, work queues, and the reconcile
loop are what a large share of infrastructure hiring actually screens for, and almost nobody
writes an operator for fun.

### Phase 2 exit criteria

- yap survives a deliberate partition and the event log shows the divergence and reheal
- `kubectl apply -f yap.yaml` is the only command needed to run an experiment
- Budget exhaustion scales the StatefulSet to zero without manual intervention

---

## Repo layout

```
cmd/
  yap/          # phase 1 runner
  analyze/        # event log → entropy, lineage, cliques, self-reference
  operator/       # phase 2 controller
internal/
  agent/          # state, belief update, prompt construction
  gossip/         # peer selection; memberlist adapter in phase 2
  events/         # schema, writer, reader
  budget/         # token accounting, context cancellation
  model/          # API client, retry, rate limiting
api/v1alpha1/     # CRD types
deploy/           # manifests, StatefulSet, NetworkPolicies
experiments/      # E1–E5 configs, one YAML per run
related-work.md
```

---

## Order of work

1. Event schema and writer — before any agent code, so nothing gets logged ad hoc
2. Budget + context plumbing — before the first real API call
3. Single agent, single round, one model call
4. Worker pool, N agents, R rounds
5. Analyses against the log
6. Watcher
7. Containerize, memberlist, StatefulSet
8. Chaos harness
9. Operator

Do not stand up Kubernetes before knowing what state is worth reconciling. That's how these
projects die.

---

## Open questions

- What's the belief-update prompt? Too agreeable and everything converges by round 3; too
  stubborn and nothing propagates. This is the main tuning dial and probably needs a sweep.
- Fixed personas or drift over time?
- Does `k` (peers per round) matter more than agent count? Cheap to test, probably yes.
- For E1, what makes a good seed goal? It needs to be distinctive enough to trace through
  paraphrase but not so alien that agents reject it outright.
- How do you measure "adoption" rigorously? Embedding similarity to the seed is the obvious
  answer and probably too blunt — a reformulated goal may be semantically distant but
  functionally identical. Possibly a judge model with a rubric.
- Does persona diversity help or hurt contagion? Intuition says a homogeneous population
  transmits faster; worth checking, because the intuition may be backwards if disagreement
  is what makes an idea worth repeating.
- E3 has a tempting follow-on: give the stuck yap a real capability and see whether the
  conclusion becomes an action. Don't. That's the incident, reconstructed at home, and the
  result is already known. The measurement of the conclusion is the contribution.
