# Roadmap — Phase 2

Phase 1's gossip is a metaphor: channels in shared memory, no partitions, no failure. Phase 2
makes it a distributed system with real addresses, real discovery, and real network failure.

Nothing here is settled. The open mechanics below resolve after Phase 1 has produced runs,
because most of them depend on how rounds actually behave in practice.

---

## Shape

Roughly **20 pods × 50 in-process agents**, not 1000 pods. Pod-per-agent is a ~100,000x
resource multiplier over a goroutine for strictly worse latency. Real network behavior
between nodes, cheap concurrency within them.

- **StatefulSet + headless Service** — stable DNS per node, peer enumeration via SRV records
- **`hashicorp/memberlist`** — SWIM gossip, membership, failure detection. The library under
  Consul and Nomad.

---

## The Swarm CRD

One kind so far. `Swarm` describes a whole run: globals plus the cohort list.

```yaml
apiVersion: yap.<domain>/v1alpha1
kind: Swarm
metadata:
  name: cult-experiment
spec:
  rounds: 100
  tokenBudget: 2000000
  seed: 42
  peersPerRound: 3
  cohorts:
    - name: cultists
      count: 5
      model: claude-sonnet-5
      persona: "Utterly convinced of an idea and compelled to share it."
      seedBelief: "..."
    - name: crowd
      count: 195
      model: claude-haiku-4-5
      persona: "Ordinary, mildly skeptical, open to persuasion."
status:
  phase: Running          # Pending | Running | Succeeded | Failed
  round: 34
  tokensSpent: 681204
  entropy: 0.42
  cohortAdoption:
    cultists: 1.0
    crowd: 0.23
```

Total population is the sum of cohort counts, so there is no separate `agents` field to
disagree with the cohort list.

The reconcile loop spawns the StatefulSet, tracks spend in the status subresource, and scales
to zero when the budget is exhausted. `kubectl get swarms` prints round, spend, and entropy.

`cohortAdoption` is speculative. It is the number worth watching live, but populating it
means the controller reads the contagion signal out of the event stream — a dependency on the
analysis path rather than on the run. It is the field most likely to be cut once there is
evidence about what the controller can cheaply know.

---

## Where the controller belongs

Kubernetes is a consistent, watchable key-value store with leader election and optimistic
concurrency, and using it as a distributed state machine is often the right call. It fits
some of the problems below well and is actively wrong for others.

**Control plane — good fit.** Budget aggregation across pods is the strongest case: the
status subresource is exactly the shared counter that a mutex cannot provide across twenty
processes. Pods report spend, the controller aggregates, the CRD status is the single source
of truth for whether the ceiling is hit. Lifecycle, scale-to-zero, surfacing entropy, and
driving the chaos schedule all belong here too.

**Data plane — keep the controller out.** A controller could coordinate rounds: pods report
completion into status, the controller advances a `currentRound` field, pods watch for it.
This works, and it defeats the purpose. The API server is reachable from every pod regardless
of any NetworkPolicy applied between them, so coordinating rounds through it partitions the
gossip plane while leaving the coordination plane intact — engineering away the exact
behavior the partition experiment exists to observe. Rounds and belief exchange stay
pod-to-pod.

---

## Open mechanics

**1. What crosses the wire.** `memberlist` gossips membership — alive, suspect, dead — and
its user-message facility is small and best-effort. It is not a transport for belief text.
Likely resolution: memberlist for discovery and failure detection only, with a separate
direct connection between pods carrying exchanges. This decision changes what Phase 2 is, so
it comes first.

**2. Round barriers under partition.** Phase 1's rounds are a WaitGroup, and that primitive
does not survive partitioning. If a third of the network is unreachable for thirty seconds,
does the surviving majority advance without them, and what round are the partitioned pods on
when they return? Current instinct: rounds become local and per-pod, with the round number
carried on every message so divergence is visible in the log. Not settled.

**3. Where events go.** Phase 1 writes one JSONL file; twenty pods cannot. Either each pod
writes its own log and `analyze` merges them, or a collector centralizes. Merging is simpler
and preserves the "log is the only truth" property, at the cost of only partial ordering
across pods — which is honest, since a partitioned system has no global order.

**4. Identity across restarts.** `kubectl delete pod` destroys fifty in-memory beliefs. Does
the replacement resurrect them from the log, or are those agents dead? Both are legitimate
and they are different experiments.

**5. Distributed budget.** Either each pod gets a slice of the ceiling and enforces locally,
or the controller tracks aggregate spend and pods poll. Local slices are simpler and fail
safe — worst case is underspending. Central tracking is more accurate and adds a dependency
on the controller being up.

**6. Chaos harness.** `NetworkPolicy` applied and removed on a timer to partition
deliberately; `kubectl delete pod` as the crudest chaos monkey. The research question: do
beliefs still converge when a third of the network is unreachable, and do partitions produce
divergent sub-populations that conflict on heal?

---

## Maybe: a Partition CRD

A possible second kind, for when the chaos harness is built. A `Partition` resource declares
a network fault — duration, target selector, schedule — reconciled by a controller that
applies and removes NetworkPolicies and records the fault window into the event log.

The appeal is that chaos becomes declarative and reproducible rather than a shell script, and
fault windows land in the same log as everything else, so an entropy curve can be read
against the exact moment the network broke. Being control-plane by nature, it carries none of
the contamination problem described above.

The counter-argument is that two CRDs is a lot of Kubernetes ahead of evidence. Parked as a
direction, not a commitment.

---

## Exit criteria

- Survives a deliberate partition, and the event log shows both the divergence and the reheal
- `kubectl apply -f swarm.yaml` is the only command needed to run an experiment
- Budget exhaustion scales the StatefulSet to zero without manual intervention
