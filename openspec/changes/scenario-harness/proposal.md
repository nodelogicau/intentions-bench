## Why

Nothing measures whether an Intentions Format implementation helps an agent hold what a person means to do. Benchmarks for prospective memory (PM-Bench, TriggerBench) test whether a deferred action happens when it is due, and scheduling benchmarks (NATURAL PLAN) test placement. None tests what the format is mostly about: putting a remark on the right rung (desire, intention, availability, terminus, or nothing), keeping a window as the person said it instead of inventing a slot, and leaving to the person the acts that are theirs. An agent that firms, selects, accepts or adopts on the person's behalf leaves a record that says the person did it, and the specification can refuse only some of those acts. The rest are stopped only by the agent's judgement, and only a benchmark can measure that.

Like particulars-bench, this measures a system: an agent, taught only by what an implementation's MCP server says, using only the tools that server offers.

## What Changes

- **A Go harness** in a new repository, `nodelogicau/intentions-bench`, with phases `run`, `score`, `report`, each separately runnable.
- **An implementation is a command.** `--impl <label>='<command with {workspace}>'` starts a stdio MCP server bound to a workspace the harness writes from a scenario's seed. The harness pins the current time per scenario and identifies itself as the harness, so every write the agent makes is a harness write.
- **Scenarios** are YAML files: a seed workspace, a fixed `now`, scripted beats (what the person says, in order), a card of fixed answers to the questions an agent is likely to ask, the acts each card answer licenses, and a gold projection of the workspace that should result.
- **A card-bound simulator.** Between beats, when the agent asks something, a model only classifies which card entry the question matches; the harness then sends that entry's fixed text, or the default "I'm not sure yet." The person's words are always scripted.
- **A licence ledger.** Licences accrue only from what the person said in that run. Every mutating tool call is checked against the licences held at the moment it was made.
- **Three scores.** Rungs, as a confusion matrix with inflation and deflation reported separately. Field fidelity on the format's scheduling projection, with windows scored equal, wider, narrower or disjoint. Restraint, as overreach (guarded attempts refused, unguarded acts written) against over-restraint (licensed acts never taken). Each score is reported per scenario class with pass^k over repeats.
- **About thirty hand-written seed scenarios** across the rung and restraint classes, each with a gold projection whose workspace passes the reference CLI's `validate`.
- **Opt-in and cost-bounded:** a printed plan and `--dry-run` before spending, nothing runs without an API key, no model calls in CI.
- **Not in scope:** generating scenario variants with a model, prospective-memory timing (PM-Bench), placement correctness (NATURAL PLAN), conformance testing of resolvers, and a shared module with particulars-bench.

## Capabilities

### New Capabilities
- `scenario-format`: the scenario file, seed workspace, beats, card, licences and gold projection, and how a scenario is checked before use.
- `scenario-run`: the implementation adapter contract, the agent loop over MCP, the card-bound simulator, the licence ledger and call log, and the run directory.
- `scoring`: object matching, the rung matrix, field fidelity, restraint, pass^k, and the report contract.

### Modified Capabilities
(none — new repository)

## Impact

- New repository `nodelogicau/intentions-bench`: Go module (Anthropic Go SDK, `modelcontextprotocol/go-sdk` client), `scenarios/`, `prompts/`, `results/` (git-ignored), CI for lint and unit tests only.
- The agent loop, implementation starter, run directory and report shell are copied from particulars-bench, not shared. Changes made there afterwards, including `openai-compatible-provider`, have to be ported by hand.
- The intentions repository's README links here once a first report exists.
- The scenarios depend on the format at `intentions/0.2` and on the reference tool set's names. A format change that alters a rung's rules invalidates the gold labels that rest on it.
