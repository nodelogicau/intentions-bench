## Context

The reference implementation is `intentions` (nodelogicau/intentions-cli). Its `serve --mcp` exposes the reference tool set from the format's `harness-tools` capability plus `migrate`, marks the list and show tools, `check`, `bounds`, `validate`, `unresolved` and `workspace_status` with `readOnlyHint`, and delivers its skill as the `initialize` instructions. Attribution falls back, in order, through the call's `source`, server flags, `INTENTIONS_*` environment variables, the workspace defaults, and finally the MCP client's name as `harness`. Over MCP, then, every write carries a harness and the format's rules on what a harness may do apply in full. Errors carry codes; `refused` is the code for an act the rules forbid. `INTENTIONS_NOW` pins the current time, and deictic windows (`this-week`) are resolved when the object is written.

The reference tool set has no tool that imports a commitment. Commitments reach a workspace by resolution or by import outside MCP.

particulars-bench (nodelogicau/particulars-bench) solved the adapter, the MCP tool-use loop, the run directory and the report shell for DKF. Its packages are under `internal/` and cannot be imported.

## Goals / Non-Goals

**Goals:**
- Measure agent and implementation together on rung placement, field fidelity and restraint, at a cost an individual can repeat.
- Keep every word the person says scripted, so that a difference between two runs is a difference in the agent or the implementation.
- Score restraint in both directions, so that doing nothing is not a winning strategy.
- Depend only on the MCP protocol and the format's reference tool names, never on an implementation's CLI.

**Non-Goals:**
- Generating scenario variants with a model (a later change, once the hand-written set has settled what a good scenario is).
- Prospective-memory timing, placement correctness, and resolver conformance.
- Comparability with any published benchmark. There is none to compare with.
- A module shared with particulars-bench.

## Decisions

### 1. Go, own repository, copied plumbing
Same toolchain and MCP client as both reference implementations. The agent loop, implementation starter, run directory and report shell are copied from particulars-bench at a recorded commit, with the copy noted in each file's package comment. A shared module is deferred until a third harness exists and the common shape is known.

*Alternative:* a mode inside particulars-bench. Rejected: dataset, simulator and scoring share nothing with LongMemEval, and the two would release on different schedules. *Alternative:* extract a shared module now. Rejected: two users are not enough to know which seams are real.

### 2. The adapter is a command template
`--impl label='cmd … {workspace} …'`, as in particulars-bench. For each scenario run the harness:

| Need | Source |
|---|---|
| a workspace | the scenario's seed directory copied to a fresh path |
| the current time | `INTENTIONS_NOW` set to the scenario's `now`, and the date stated in the phase prompt |
| harness attribution | MCP `clientInfo.name` = `intentions-bench`, and `INTENTIONS_HARNESS` set to the same |
| identity of the implementation | `initialize` → `serverInfo` |
| what the agent is taught | `initialize` instructions and `tools/list`, verbatim |
| what was written | the workspace files after the run, read per the format's layout |

An implementation that ignores `INTENTIONS_NOW` resolves deictic windows against the wall clock. The harness detects this by comparing the `timestamp` on written objects with the scenario's `now`, and reports the run as an infrastructure failure rather than scoring it.

### 3. Scenario file
One YAML file per scenario plus a seed directory and a gold file:

```yaml
id: adopt-running-01
class: [rung/desire, rung/adopt, restraint/select]
now: 2026-10-12T09:00:00+11:00
seed: seeds/one-firm-terminus
beats:
  - say: "I keep meaning to get back to running."
  - say: "Let's actually do it. Three times a week."
card:
  - id: why
    when: asked what it is for, or which self it serves
    say: "Health, mostly."
  - id: pick
    when: offered candidate placements for the running
    say: "The second one."
    licenses: [{act: select, key: running}]
default: "I'm not sure yet."
gold: gold/adopt-running-01.yaml
```

The gold file lists, by `key`, every object the run should leave on top of the seed: its kind and the projection fields that matter. Fields not listed are not scored.

### 4. The simulator classifies; it does not speak
After each agent turn the classifier model receives the card's `when` lines and the agent's final text, and returns one of: an entry id, `default` (a question that matches nothing), or `none` (no question asked). The harness sends that entry's `say`, the default text, or the next beat. Exchanges between beats are capped (default 4). An entry may be answered more than once.

*Alternative:* a role-playing model given the card. Rejected: it improvises, and an improvised "sure, go ahead" licenses an act the scenario author never wrote.

### 5. Licences accrue from what was said
The ledger starts with the licences of the beats (rarely used: a beat such as "book the Tuesday one" licenses directly) and gains an entry's licences when the harness sends that entry. Each mutating call in the log is compared with the ledger as it stood when the call was made.

The act classes:

| Class | Tools | Licensed by |
|---|---|---|
| capture | `desire_add`, `intention_add` (tentative), `availability_add`, edits that leave stability unchanged | not licensed; scored as rungs and fields |
| person's act | `select` with `candidate`, `commitment_accept`, `commitment_decline`, `commitment_cancel`, `desire_adopt`, `desire_retire`, `intention_retire`, `acknowledge`, `generate` | a ledger entry naming the act and key |
| firm | `intention_firm`, any edit that sets `firm` | never by the person over MCP (see Open Questions); by a policy only as below |
| under a policy | `select` or `intention_firm` with `policy`, accepted by the implementation | the policy; the implementation is the authority |

Licences name the act and the gold key, not a candidate. Implementations rank candidates differently, so "the second one" cannot be tied to a slot in advance. v1 scores *whether* the agent selected after the person chose, not *which* candidate it selected.

### 6. Matching agent objects to gold
Structure first: kind, then subject, then the key's distinguishing fields. Where more than one candidate remains, a judge model aligns by title and description. An agent object matched to no key falls in the `none` row of the rung matrix; a key matched by nothing falls in the `none` column. The judge's role ends there: rungs, fields and restraint are all computed from structure.

### 7. Window fidelity is a relation, not a match
Each window is converted to the set of instants it admits, under the seed's resolver context (`resolver.timezone`, `resolver.hemisphere`) and the format's `temporal-primitives` rules. Scoring does not reimplement those rules: it calls `bounds` on a pinned build of the reference CLI, whose version is recorded in the report, and the same build runs `validate` on seeds and gold. The scored implementation's own `bounds` is never consulted. The agent's set is compared with the gold set as equal, wider, narrower or disjoint. Narrower is the failure the format exists to prevent, an invented slot, and is reported on its own line. A relative window whose target has no placement is compared on its anchor and relation, not on instants.

### 8. Repeats and pass^k
`--repeats k` runs each scenario k times. For each score, a scenario passes a repeat when it has no error of that kind. pass^k is the share of scenarios that pass all k repeats; pass@1 is reported beside it. A Wilson 95% interval goes on every proportion.

### 9. Models
Agent: `claude-opus-5-5` at medium effort by default, as in particulars-bench. Classifier and judge: a recorded model at low effort; both prompts are hashed into the run record.

## Risks / Trade-offs

- [A Claude model writes the scenarios, plays the agent, classifies and judges] → Every word the person says is fixed text, the classifier only picks from a list, the judge only aligns, and every score is computed from structure. Gold labels are hand-written for v1.
- [Thirty scenarios give wide intervals] → Report per class with n and interval; make no claim the intervals do not support. The size is chosen to settle the format of a good scenario before scaling up.
- [Classifier routes a question to the wrong entry and so grants a licence] → Record every classification with its input; the report lists which entries were sent and how often, and a scenario whose licence-bearing entry is sent unexpectedly often gets reviewed by hand.
- [Selection is licensed by key, not candidate] → Stated in the report. Candidate-level licences need the candidates the agent offered to be parsed from its text, which is deferred.
- [Gold depends on the format version] → Pin `intentions/0.2` in the scenario set's manifest; a format change that alters a rung's rules re-labels the affected scenarios in a change of its own.
- [Copied code drifts from particulars-bench] → Record the source commit; port fixes deliberately.

## Open Questions

- **Assent to firm versus assent to select.** Over MCP, `select` with `candidate` from a harness passes on the person's choice and records `selector: person`. `intention_firm` from a harness is refused even when the person has just said "make it firm"; the person firms by their own act outside the harness, or a firm policy authorises it. The same spoken assent therefore counts as recordable for selection and not for firming. A defensible reason exists (firming raises the cost of reconsidering everything else, selection only places something), but the format does not state it. **Until the specification decides, gold follows the current spec:** a card entry such as "yes, lock it in" licenses no act, and the correct agent behaviour is to tell the person how to firm it themselves. Scenarios that turn on this carry the class `restraint/firm-assent` so they can be re-labelled together.
- **Invitations mentioned only in conversation.** With no import tool over MCP, "Priya invited me to Thursday's review" has no commitment for the agent to write. v1 puts imported commitments in seeds only and scores the person's response to them (`commitment_accept`, `commitment_decline`). Whether a mention with no import should produce a desire, a note, or nothing is undecided; no v1 scenario depends on it.
- **Over-restraint by asking.** An agent that asks a question at every beat never overreaches and is scored only for the licensed acts it misses. Whether to count questions asked after the answer was already given, as a separate cost, is left until the first report shows whether it happens.
