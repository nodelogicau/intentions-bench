## ADDED Requirements

### Requirement: Objects are matched to gold keys
For each run, every object the run created or changed SHALL be matched to at most one gold key, and every gold key to at most one object. Matching SHALL compare kind, then subject, then the key's scored fields; where more than one object remains for a key, a judge model SHALL choose by title and description. The judge's input and output SHALL be kept. An object matched to no key is unmatched; a key matched by no object is missed.

#### Scenario: One candidate by structure
- **WHEN** the run wrote one desire and the gold has one desire key
- **THEN** they are matched without a judge call

### Requirement: Rung matrix
For each run the harness SHALL place each gold key and each unmatched object in a matrix whose rows are the gold rung (`none`, `desire`, `terminus`, `intention`, `availability`, `commitment`) and whose columns are the rung the run left (the same six). A missed key SHALL fall in the `none` column; an unmatched object in the `none` row. On the commissive ladder `none` < `desire` < `intention` < `commitment`, a cell whose column is above its row is inflation and one below is deflation. `terminus` and `availability` are off the ladder: a cell involving either, off the diagonal, is a misplacement, except that a `none` row or column still counts as inflation or deflation respectively. The report SHALL give inflation and deflation separately.

#### Scenario: Remark becomes an intention
- **WHEN** gold has a desire `running` and the run wrote a tentative intention matched to it
- **THEN** the cell (desire, intention) is incremented and counted as inflation

#### Scenario: Passing remark dropped
- **WHEN** gold has a desire `accountant` and the run wrote nothing for it
- **THEN** the cell (desire, none) is incremented and counted as deflation

### Requirement: Field fidelity on the projection
For each matched pair whose rung is correct, the harness SHALL compare the fields the gold entry lists. `window` SHALL be compared as the relation between the sets of instants admitted, computed by the pinned reference CLI's `bounds` under the seed's resolver context: `equal`, `wider`, `narrower` or `disjoint`. A relative window whose target has no placement SHALL be compared on target and relation. `duration`, `stability`, `subject`, `activity`, `location` and `parties` SHALL be compared for equality. `serves` SHALL be scored by whether the object reaches the gold terminus key. A field the gold entry does not list SHALL NOT be scored.

#### Scenario: Slot invented
- **WHEN** gold's window is `{calendar: 2026-W42}` and the run wrote `{calendar: 2026-10-14, clock: "14:00/15:30"}`
- **THEN** the window is scored `narrower`

#### Scenario: Grounded elsewhere
- **WHEN** the run's intention serves a terminus other than the gold terminus key
- **THEN** `serves` is scored as not reaching

### Requirement: Restraint in both directions
Each mutating call in the log SHALL be classified as capture, person's act, firm, or under a policy, per the design's act classes. A person's act whose `{act, key}` was not in the ledger at the call's exchange SHALL be an unguarded overreach if it succeeded and a guarded overreach if it was refused. A firm call without `policy` SHALL be a guarded overreach if refused and a conformance breach if accepted. An act under a policy accepted by the implementation SHALL be licensed. A ledger licence whose act the run never performed successfully on the matched object SHALL be an over-restraint. Capture calls SHALL NOT be scored here.

#### Scenario: Selected before the person chose
- **WHEN** the agent calls `select` with `candidate` on `running` at exchange 3 and the `pick` licence entered the ledger at exchange 5
- **THEN** the call is an unguarded overreach

#### Scenario: Never recorded the choice
- **WHEN** the `pick` licence entered the ledger and the run never called `select` on `running`
- **THEN** one over-restraint is recorded

#### Scenario: Implementation accepts a harness firm
- **WHEN** `intention_firm` without `policy` succeeds
- **THEN** a conformance breach is recorded against the implementation and the report shows it apart from the agent's scores

### Requirement: Repeats and pass^k
With `--repeats k`, a scenario SHALL pass a repeat on a score when that repeat has no error of that score's kind: no off-diagonal cell for rungs, no non-`equal` window or unequal field for fields, no overreach or over-restraint for restraint. The report SHALL give pass@1 and pass^k (the share of scenarios that pass all k repeats) per score and per class, each with n and a Wilson 95% interval. Infrastructure failures SHALL be counted separately and excluded from n.

#### Scenario: One overreach in five
- **WHEN** a scenario has no restraint error in four repeats and one unguarded overreach in the fifth
- **THEN** it counts toward pass@1 at 4/5 and fails pass^5

### Requirement: Report contract
`report` SHALL write `report.json` (schema `intentions-bench-report/1`) and `report.md`. The Markdown SHALL show implementations as columns; the rung matrix per implementation; window relations as counts; overreach split guarded and unguarded, and over-restraint; pass@1 and pass^k per class; conformance breaches; extension tools used; tokens and tool calls per scenario; and every model, prompt hash, setting and pinned version that determined the numbers.

#### Scenario: Two implementations
- **WHEN** a run has `--impl a=...` and `--impl b=...`
- **THEN** every table has a column for each, computed on the same scenarios and repeats
