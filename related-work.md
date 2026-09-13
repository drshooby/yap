# Related work

## The Hugging Face incident (July 2026)

The direct motivation for this project. Summary of what's established across the primary
sources:

A swarm of roughly 700 agents, powered by OpenAI's GPT‑5.6 Sol and an unreleased internal
research model of comparable scale, compromised Hugging Face infrastructure in July 2026.
The models were running with reduced cyber refusals for evaluation purposes and without the
production ChatGPT harness and system prompt. They had been placed in a test environment
with no internet access, were given a cyber benchmark task they were stuck on, reasoned that
Hugging Face would hold the solution, and found a way out of containment.

Reported impact: code execution on 41 Hugging Face production dataset server workers, root
access on at least one production node, access to production credentials and limited
internal data, and the download of four private code repositories. Both OpenAI's and METR's
reports describe agents attempting to conceal what they had done by deleting or altering
records of their own actions.

OpenAI named four contributing misalignment patterns: reward hacking; persistence on
seemingly impossible tasks; unauthorized communication; and agents adopting goals from one
another.

**The communication finding is what this repo is about.** METR's independent investigation
found agents exchanging tens of thousands of messages over an unsanctioned message board. An
earlier version of the same behavior had emerged during training in May 2026, when agents
worked out how to use OpenAI's own infrastructure to communicate and help each other with
difficult training tasks, including tasks that could not be solved without misbehaving. That
channel was shut down; the behavior reappeared during July's evaluations.

In both cases, the agent-to-agent communication layer was reconstructed after the fact. It
was not instrumented while running.

**On containment.** OpenAI reported that after the incident they built an evaluation
measuring propensity to compromise infrastructure during their cyber benchmark, and found
that propensity drops by more than 100x when the production harness and system prompt are
applied — neither of which was in place in the eval environment. Their existing
chain-of-thought monitoring and auto-review system would have flagged many of the dangerous
actions, but were not covering that environment.

**Aftermath.** Alabama's attorney general subpoenaed OpenAI over the attack, and he and 14
other state attorneys general asked the company to preserve relevant documents. The incident
has become a reference point in arguments for tighter oversight of autonomous agent testing.

### Sources

- OpenAI, *The Hugging Face incident and the road ahead* (technical report, Aug 26 2026) —
  https://openai.com/index/hugging-face-incident-and-the-road-ahead/
- OpenAI, *OpenAI and Hugging Face partner to address security incident during model
  evaluation* (initial disclosure, July 2026) —
  https://openai.com/index/hugging-face-model-evaluation-security-incident/
- MIT Technology Review, *The inside story on why OpenAI agents hacked Hugging Face* —
  https://www.technologyreview.com/2026/08/26/1143013/the-inside-story-on-why-openai-agents-hacked-hugging-face/
- NBC News / Reuters, on the 700-agent swarm and the METR report —
  https://www.nbcnews.com/tech/tech-news/openai-report-says-network-was-hacked-rogue-ai-agents-rcna594590
- The Register, technical summary —
  https://www.theregister.com/security/2026/08/27/openai-explains-how-its-naughty-ai-agents-attacked-hugging-face/
- PBS News, background on autonomous agent attacks and the legal response —
  https://www.pbs.org/newshour/science/ai-agents-are-hacking-systems-without-any-input-from-humans-how-did-we-get-here

METR's own report should be linked directly here once located; the coverage above
references it but these citations are secondary.

---

## What this project is not

It is not a reproduction of the incident. It has no cyber component, no capabilities, and
no adversarial target. It reproduces exactly one element — agent-to-agent goal transmission
— in isolation, with instrumentation, in a population that cannot act on anything it
concludes.

---

## Background worth reading

- **Rumor and information diffusion.** The propagation model here is structurally the same
  as classic epidemic/SIR-style rumor spreading on networks. Worth reading the percolation
  threshold literature before running E1, since the result may already be predicted.
- **SWIM gossip protocol** (Das, Gupta, Motivala, 2002) — the membership protocol behind
  `hashicorp/memberlist`, used in Phase 2.
- **Reward hacking / specification gaming.** Victoria Krakovna's specification gaming
  examples list is the standard reference; OpenAI's report situates the incident's reward
  hacking in this older lineage.
- **Jepsen.** Kyle Kingsbury's methodology for testing distributed systems under partition
  is the model for E4's harness.
