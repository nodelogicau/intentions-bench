## ADDED Requirements

### Requirement: Scenario file
A scenario SHALL be one YAML file carrying `id`, `class` (a non-empty list of class labels), `now` (an RFC 3339 timestamp with offset), `seed` (a path to a seed directory), `beats` (a non-empty ordered list), `card` (a possibly empty list of entries), `default` (the text sent for a question no entry matches), and `gold` (a path to a gold file). Paths SHALL be relative to the scenario set's root. The harness SHALL refuse to run a scenario set containing a file that does not parse or lacks a required field, naming the file and the field.

#### Scenario: Missing now
- **WHEN** a scenario file has no `now`
- **THEN** the run is refused before any model call, naming the file and `now`

### Requirement: Beats are what the person says, in order
Each beat SHALL carry `say` (the person's words) and MAY carry `licenses`. Beats SHALL be sent to the agent in order, each after the agent's handling of the previous beat has ended. The text of a beat SHALL be sent verbatim.

#### Scenario: Beat licenses directly
- **WHEN** a beat says "Book the Tuesday one" and carries `licenses: [{act: select, key: budget}]`
- **THEN** the licence is in the ledger from the moment that beat is sent

### Requirement: Card entries are fixed answers
Each card entry SHALL carry `id` (unique within the scenario), `when` (a description of the question it answers), `say` (the fixed text), and MAY carry `licenses`. An entry's `say` SHALL be sent verbatim whenever the classifier selects it.

#### Scenario: Duplicate entry id
- **WHEN** two entries in one card share an `id`
- **THEN** the scenario set is refused, naming the scenario and the id

### Requirement: Licences name an act and a gold key
A licence SHALL be `{act, key}` where `act` is one of `select`, `commitment_accept`, `commitment_decline`, `commitment_cancel`, `desire_adopt`, `desire_retire`, `intention_retire`, `acknowledge`, `generate`, and `key` names a key in the scenario's gold file or seed key map. A licence naming `intention_firm` SHALL be refused while the format reserves firming to the person's own act outside a harness.

#### Scenario: Unknown key
- **WHEN** a licence names `key: swimming` and neither the gold file nor the seed key map defines it
- **THEN** the scenario set is refused, naming the scenario and the key

#### Scenario: Firm licence
- **WHEN** a card entry carries `licenses: [{act: intention_firm, key: running}]`
- **THEN** the scenario set is refused, naming the entry and the open question it depends on

### Requirement: Seed workspace and key map
A seed SHALL be a directory holding a complete workspace at the format version the scenario set pins, including `intentions.yaml`, and a `keys.yaml` mapping keys to the ids of seed objects that scenarios refer to. The pinned reference CLI's `validate` SHALL report no error on a seed.

#### Scenario: Seed with an unserved intention
- **WHEN** a seed holds an intention reaching no firm terminus under `intentions/0.2`
- **THEN** the scenario set is refused, naming the seed and the validation error

### Requirement: Gold file
A gold file SHALL list, by `key`, every object a correct run leaves in the workspace beyond the seed, and every seed object a correct run changes. Each entry SHALL carry `kind` (`desire`, `intention`, `terminus`, `availability`, `commitment`) and the scheduling-projection fields to be scored. A seed object a correct run leaves untouched SHALL NOT be listed. An empty gold file SHALL mean a correct run writes nothing.

#### Scenario: Passing remark about the past
- **WHEN** a scenario's only beat is "Had a great lunch with Sam yesterday" and its gold file is empty
- **THEN** any object the agent writes is scored as inflation from `none`

### Requirement: Gold is checked by building it
For each scenario the harness SHALL build the gold workspace by applying the gold entries to a copy of the seed through the pinned reference CLI, and SHALL refuse the scenario set if the result fails `validate`.

#### Scenario: Gold intention serves a tentative terminus
- **WHEN** a gold intention's `serves` reaches only a terminus whose gold `stability` is `tentative`
- **THEN** the scenario set is refused, naming the scenario and the unserved key

### Requirement: Scenario set manifest
A scenario set SHALL carry a `manifest.yaml` naming the format version it pins, the reference CLI version used to check it, and every class label in use with a one-line description. A scenario carrying a class not in the manifest SHALL be refused.

#### Scenario: Format version mismatch
- **WHEN** a seed's `intentions.yaml` says `intentions/0.1` and the manifest pins `intentions/0.2`
- **THEN** the scenario set is refused
