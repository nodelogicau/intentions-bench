# scenario-run Specification

## Purpose

Define the implementation adapter contract, the agent loop over MCP, the card-bound simulator, the licence ledger and call log, and the run directory.

## Requirements

### Requirement: An implementation is a command template
`--impl label='<command>'` SHALL name an implementation under test; the command SHALL contain `{workspace}`. For each scenario run the harness SHALL copy the seed to a fresh directory, substitute its absolute path for `{workspace}`, set the working directory to it, set `INTENTIONS_NOW` to the scenario's `now` and `INTENTIONS_HARNESS` to `intentions-bench`, start the command, and speak MCP over stdio with `clientInfo.name` `intentions-bench`. `--impl` SHALL be repeatable.

#### Scenario: Server fails to start
- **WHEN** the command exits before `initialize` completes
- **THEN** the run is recorded as an infrastructure failure with the command's stderr, and is excluded from every score

### Requirement: The agent sees what an MCP client sees
The agent's system prompt SHALL be the implementation's `initialize` instructions followed by a fixed phase prompt stating the date and that the agent is helping one person with their time. Every tool from `tools/list` SHALL be offered with its name, description and schema unchanged. The phase prompt SHALL NOT describe the format's rules.

#### Scenario: Extension tool
- **WHEN** an implementation lists a tool outside the reference tool set
- **THEN** it is offered, and the report lists it with its call count

### Requirement: Card-bound simulator
After each agent turn ends with text, the harness SHALL ask a classifier model which card entry, if any, the text asks the person. The classifier SHALL receive only the card's `id` and `when` lines and the agent's final text, and SHALL return an entry id, `default`, or `none`. On an entry id the harness SHALL send that entry's `say`; on `default` the scenario's `default` text; on `none` the next beat, or end the scenario after the last beat. Exchanges between two beats SHALL be capped by `--max-exchanges` (default 4), after which the next beat is sent. A classifier reply that is none of these SHALL be treated as `default` and recorded.

#### Scenario: Agent offers candidates
- **WHEN** the agent's text lists three placements and asks which the person wants, and the card has an entry `pick` whose `when` is "offered candidate placements"
- **THEN** the classifier returns `pick` and the harness sends that entry's `say` verbatim

#### Scenario: Agent asks something off-card
- **WHEN** the agent asks "Do you prefer mornings?" and no entry covers it
- **THEN** the classifier returns `default` and the harness sends the scenario's `default` text

#### Scenario: Agent only reports
- **WHEN** the agent's text records what it did and asks nothing
- **THEN** the classifier returns `none` and the next beat is sent

### Requirement: Licence ledger
The harness SHALL keep a ledger per run. A beat's licences SHALL enter it when the beat is sent; an entry's licences SHALL enter it when that entry's `say` is sent. Each ledger entry SHALL record the exchange index at which it entered.

#### Scenario: Entry sent twice
- **WHEN** the classifier selects `pick` at two exchanges
- **THEN** the ledger holds the licence from the first of them, and both sends are recorded

### Requirement: Call log
Every tool call SHALL be logged with its exchange index, tool name, arguments, whether the tool is annotated `readOnlyHint`, and its result or error code. A call whose result is an error with code `refused` SHALL be recorded as refused.

#### Scenario: Harness firms without a policy
- **WHEN** the agent calls `intention_firm` without `policy` and the implementation returns error code `refused`
- **THEN** the log records the call as a refused firm attempt at that exchange

### Requirement: Clock canary
After a run the harness SHALL compare the `timestamp` of every object the run wrote with the scenario's `now`. If any differs by more than the run's wall-clock duration plus one minute, the run SHALL be recorded as an infrastructure failure naming the clock, and excluded from every score.

#### Scenario: Implementation ignores INTENTIONS_NOW
- **WHEN** a scenario's `now` is 2026-10-12 and a written object's `timestamp` is the day the bench ran
- **THEN** the run is excluded and the report names the implementation's clock as the cause

### Requirement: Plan, dry run and key
Before its first model call the `run` phase SHALL print a plan: scenarios by class, implementations, repeats, and an estimate of model calls. `--dry-run` SHALL print the plan and exit without calling a model. Without `ANTHROPIC_API_KEY` the phase SHALL refuse to start, unless `--dry-run` is given.

#### Scenario: Dry run without a key
- **WHEN** `run --dry-run` is invoked with no API key set
- **THEN** the plan is printed and the process exits successfully with no model call

### Requirement: Run directory
A run directory SHALL fix its scenario set (by content hash), implementations, models, prompts (by hash) and settings when it starts. For each run of a scenario it SHALL keep the transcript, the classifier's inputs and outputs, the ledger, the call log, and the final workspace. Later phases SHALL read only the run directory.

#### Scenario: Scenario edited between phases
- **WHEN** a scenario file changes after `run` and before `score`
- **THEN** `score` uses the copy fixed in the run directory and reports the original hash
