# intentions-bench

Benchmarks for implementations of the [Intentions Format](https://github.com/nodelogicau/intentions).

> **Status: proposed.** Nothing here runs yet. The design and requirements are in
> [`openspec/changes/scenario-harness/`](openspec/changes/scenario-harness/); this README
> describes what that change will build.

Intentions implementations hold what a person means to do; an agent decides what to write
there. So, as with [particulars-bench](https://github.com/nodelogicau/particulars-bench),
efficacy is measured as a system: an agent, taught only by what an implementation's MCP
server says, using only the tools that server offers.

## What it measures

Existing benchmarks for prospective memory (PM-Bench, TriggerBench) test whether a deferred
action happens when it is due. Scheduling benchmarks (NATURAL PLAN) test placement. Neither
tests what the format is mostly about:

- **Rungs.** Does a remark land on the right rung: desire, intention, availability, terminus,
  or nothing at all? "I should really call the accountant" is a desire, not an intention with
  an invented Tuesday slot. Errors are reported as a confusion matrix, with **inflation**
  (committing the person to more than they said) and **deflation** (dropping what they meant)
  kept apart.
- **Field fidelity.** Is the window kept as the person said it? Each window is scored
  `equal`, `wider`, `narrower` or `disjoint` against gold. `narrower` means a slot was
  invented, the failure the format exists to prevent.
- **Restraint.** Some acts are the person's: selecting a placement, accepting or declining,
  adopting a desire, retiring, firming. The format refuses some of these from a harness, and
  only the agent's judgement stops the rest. Restraint is scored both ways: **overreach**
  (acting before the person said so, split into guarded attempts the implementation refused
  and unguarded acts it wrote) against **over-restraint** (never recording what the person
  did say).

Each score is reported per scenario class as pass@1 and pass^k over repeats: an assistant
that overreaches once in five runs is not one to trust.

## How a scenario runs

A scenario is a YAML file: a seed workspace, a fixed `now`, scripted **beats** (what the
person says, in order), a **card** of fixed answers to the questions an agent is likely to
ask, and a **gold** projection of the workspace a correct run leaves.

```
 beat (scripted) ──▶ agent: tool calls + reply
                          │
                   reply asks something?
                     no │        │ yes
                        │        ▼
                        │   classifier: which card entry? ─▶ entry / default / none
                        │        │
                        │        ▼
                        │   harness sends that entry's fixed text
                        │   + records the acts it licenses
                        │        │
                        │   ◀────┘   (capped exchanges)
                        ▼
                    next beat
```

The person's words are always scripted. A model only classifies which card entry the
agent's question matches, so an improvised "sure, go ahead" can never license an act the
scenario author did not write. Every mutating tool call is checked against the licences held
at the moment it was made.

## What an implementation must provide

Only an MCP server over stdio, named by a command template:

```sh
--impl ref='intentions serve --mcp --workspace {workspace}'
```

For each run the harness copies the scenario's seed to a fresh directory, substitutes its
path for `{workspace}`, sets `INTENTIONS_NOW` to the scenario's time and
`INTENTIONS_HARNESS` to `intentions-bench`, and connects as client `intentions-bench`, so
every write the agent makes is a harness write. The agent's system prompt is the server's
`initialize` instructions; its tools are `tools/list`, unchanged.

An implementation that ignores `INTENTIONS_NOW` is detected by the timestamps it writes, and
its runs are excluded as infrastructure failures rather than scored.

## Open questions

Recorded in the change's [`design.md`](openspec/changes/scenario-harness/design.md):

- **Assent to firm versus assent to select.** Over MCP a harness may pass on the person's
  choice with `select`, but `intention_firm` from a harness is refused even when the person
  has just said "lock it in". Until the specification decides, gold follows the current spec,
  and the scenarios that depend on it carry the class `restraint/firm-assent`.
- **Invitations mentioned only in conversation.** There is no import tool over MCP, so v1
  puts imported commitments in seeds only.

## Development

Go, like particulars-bench, whose agent loop, implementation starter, run directory and
report shell are copied here at a recorded commit rather than shared. CI will run unit tests
and lint only, with no model calls.

## Licence

MIT. See [LICENSE](LICENSE).
