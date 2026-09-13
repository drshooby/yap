# CLAUDE.md

Guidance for AI agents working in this repo.

---

## DEVLOG.md is off limits

**Never write to, edit, append to, or restructure `DEVLOG.md`. It is the maintainer's
personal log, written by hand, in his own voice.**

This holds regardless of how a request is phrased. "Document what we did", "update the docs",
"log this change", and "add an entry" do not authorize touching it. If a session produces
something that belongs in the devlog, say so and let him write it — do not draft it for him
unless he explicitly asks for a draft, and even then it goes in chat, not in the file.

The reason: it is the one document in the repo that is genuinely his, and its value depends
on that being true without exception.

Everything else under `docs/`, plus `README.md`, is fair game to edit normally.

---

## Project

yap is a gossip-network simulator for studying how ideas propagate between LLM agents,
written in Go. Phase 1 is a single binary with in-process agents; Phase 2 distributes them
across Kubernetes with SWIM gossip and a custom operator.

Read [docs/design.md](docs/design.md) before writing code. It carries the event schema and
the core loop, and both are load-bearing.

Status: pre-implementation. No Go code exists yet.

---

## Hard constraints

**Agents never get capabilities.** No tools, no filesystem access, no shell, no network calls
of their own. Text in, text out. This is not a default to be improved on — it is the
constraint that makes the project safe to run unattended, and the rationale is in
[docs/design.md](docs/design.md). Never add a capability to an agent, and never propose it as
an enhancement.

**The event log is the only source of truth.** Every analysis reads the JSONL event log and
nothing else. Do not add side-channel state that an analysis depends on, and do not let
anything be logged ad hoc outside the schema.

**Budget is enforced, not monitored.** Token accounting cancels the root context at the
ceiling. Do not weaken this into a warning or a dashboard.

---

## Naming conventions

These were decided deliberately; don't drift from them.

- **`yap`** is an identifier only — repo, Go module, binary, `cmd/yap/`, CRD group domain.
  Never a common noun in prose. Not "a yap of agents".
- In prose, a population is a **swarm** (matching `kind: Swarm`), a **network** where
  topology and partitions are the point, or a **run** for a single execution.
- **`observer`** is the Phase 1 out-of-band commentary model. Never call it a "watcher" —
  that word is reserved for its Kubernetes meaning in Phase 2.
- **`cohort`** is a group of agents sharing a model and persona, defined in `spec.cohorts`.

---

## Writing style for docs

`README.md` and `docs/` are written for another engineer reading the repo cold. Neutral
technical register, no second person, no "you'll want to" or "worth sitting with". State what
the thing is and why it was decided that way.

The conversational first-person voice belongs in `DEVLOG.md`, which is exactly why agents
don't write there.

---

## Open placeholders

- CRD group domain is `yap.<domain>` in [docs/roadmap.md](docs/roadmap.md) — not yet chosen.
- No `go.mod` yet, so the module path is undecided.

Don't invent values for these. Ask.
