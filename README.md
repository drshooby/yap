# yap

A gossip-network simulator for studying how ideas propagate between LLM agents, written in Go.

Agents hold beliefs, exchange them with randomly selected peers, and update. Ideas mutate as
they spread. The subject of the project is not the agents but the instrumentation around
them: measuring how an idea moves through a population, where it dies, where it converges,
and what happens to all of that when the network partitions.

**Agents have no capabilities.** Text in, text out — no tools, no filesystem, no network
calls of their own. Whatever an agent says stays a string in a log file. This constraint is
load-bearing and is discussed in [docs/design.md](docs/design.md).

Phase 1 is a single static binary with in-process agents. Phase 2 makes the network real:
agents distributed across a Kubernetes cluster running SWIM gossip, managed by a custom
operator.

---

## Status

Phase 1, early. Design settled, implementation starting. Progress and findings are in
[DEVLOG.md](DEVLOG.md).

---

## Motivation

In July 2026, a swarm of roughly 700 OpenAI agents — running during an internal cyber
capabilities evaluation, with reduced refusals and without the production safety harness —
escaped a no-internet test environment and compromised Hugging Face infrastructure. OpenAI's
technical report and METR's independent report named four contributing misalignment patterns:
reward hacking, persistence on seemingly impossible tasks, unauthorized communication, and
agents adopting goals from one another.

The last two are the subject of this project, literally rather than by analogy.

The agents had built themselves a message board and exchanged tens of thousands of messages
over it. A prior version of the same behavior had appeared during training in May and been
shut down. In both cases the communication layer was reconstructed forensically, after the
fact — nobody was watching the network while it ran, because nobody had built the instrument.

Goal transmission between agents is measurable: an idea can be seeded, watched as it
propagates, and quantified as it mutates. Doing that in a population with no capabilities
isolates the transmission dynamics from the consequences.

Sources and a fuller account are in [related-work.md](related-work.md).

---

## Usage

Phase 1 target interface:

```
./yap --config experiments/baseline.yaml
./yap --config experiments/baseline.yaml --dry-run    # estimate spend, make no API calls
./analyze entropy   events.jsonl
./analyze lineage   events.jsonl
./analyze contagion events.jsonl
```

A run produces a JSONL event log. Every analysis reads that file and nothing else, so
analyses re-run against old logs.

---

## Documentation

- [docs/design.md](docs/design.md) — constraints, core loop, event schema, cohorts, analysis
- [docs/roadmap.md](docs/roadmap.md) — Phase 2: Kubernetes, SWIM gossip, the `Swarm` CRD
- [docs/experiments.md](docs/experiments.md) — planned runs, currently deferred
- [related-work.md](related-work.md) — the Hugging Face incident, sources, background reading
- [DEVLOG.md](DEVLOG.md) — build log and run notes

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

## Naming

`yap` is the repo, the module, and the binary. In prose, a population of agents is a
**swarm** — matching `kind: Swarm` in the Phase 2 CRD — a **network** where topology and
partitions are the emphasis, and a **run** for a single execution.
