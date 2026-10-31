## 1. Repository

- [x] 1.1 Go module `github.com/nodelogicau/intentions-bench`, `cmd/intentions-bench` with phases `run`, `score`, `report` and shared flags (`--impl`, `--scenarios`, `--classes`, `--ids`, `--repeats`, `--max-exchanges`, `--concurrency`, `--dry-run`, `--model`, `--effort`, `--classifier-model`, `--judge-model`); the `ANTHROPIC_API_KEY` refusal.
- [x] 1.2 Makefile (`build`, `test`, `lint`), `.golangci.yml`, and CI running unit tests and lint only, with no model calls.
- [x] 1.3 Copy `agent`, `impl`, `run`, the report shell and the fake-server pattern from particulars-bench at a recorded commit; note the source commit in each package comment.
- [x] 1.4 Create `nodelogicau/intentions-bench` on GitHub.

## 2. Scenario format

- [x] 2.1 Parse scenario, card, licence, gold, `keys.yaml` and `manifest.yaml`; refuse with file and field named.
- [x] 2.2 Check seeds and build gold through the pinned reference CLI's `validate`; refuse a set that fails.
- [x] 2.3 Refuse `intention_firm` licences and unknown keys; tests for each refusal scenario in `scenario-format`.

## 3. Run

- [x] 3.1 Adapter: seed copy, `{workspace}`, `INTENTIONS_NOW`, `INTENTIONS_HARNESS`, `clientInfo.name`; infrastructure failure with stderr.
- [x] 3.2 Phase prompt (date, one person, no format rules) and its hash; tools offered verbatim.
- [x] 3.3 Classifier prompt and call; `entry | default | none`; malformed replies as `default`; exchange cap.
- [x] 3.4 Licence ledger and call log with exchange indices, `readOnlyHint`, and error codes.
- [x] 3.5 Clock canary.
- [x] 3.6 Plan printer and `--dry-run`; run directory fixing scenario hashes, models, prompts and settings.
- [x] 3.7 Tests with a fake MCP server and a scripted model: a classified question sends the entry verbatim, the ledger records licences when sent, a refused call is logged as refused.

## 4. Score

- [x] 4.1 Object matching by structure, with judge alignment only for ties; keep judge input and output.
- [x] 4.2 Rung matrix with inflation, deflation and misplacement.
- [x] 4.3 Field fidelity: window relation via the pinned reference CLI's `bounds`, equality fields, `serves` reachability.
- [x] 4.4 Restraint: act classes, overreach (guarded, unguarded), over-restraint, conformance breach.
- [x] 4.5 pass@1 and pass^k with Wilson intervals; infrastructure failures excluded from n.
- [x] 4.6 Tests for every scenario in the `scoring` spec using hand-built logs and workspaces.

## 5. Report

- [x] 5.1 `intentions-bench-report/1` JSON.
- [x] 5.2 Markdown: implementations as columns, rung matrix, window relations, restraint, pass^k per class, conformance breaches, extensions, tokens, and every pinned input.

## 6. Scenarios

- [x] 6.1 Seeds: an empty workspace with one firm terminus; one with a firm `auto_select` policy; one with an imported pending commitment; one with a placed intention and a clashing import.
- [x] 6.2 About thirty scenarios, at least three per class: passing remark → desire; hedged want → desire; clear intention with window → tentative intention; self-understanding → draft terminus; policy stated → draft terminus; availability; past event → nothing; abandon a desire; adopt a desire; select after the person chose; accept or decline a seeded invitation; "lock it in" (`restraint/firm-assent`); select under a firm policy.
- [x] 6.3 Manifest with format version, reference CLI version, and every class described.

## 7. First run

- [ ] 7.1 Run the reference CLI with `--repeats 3`; commit the report under `reports/` with the date and versions.
- [x] 7.2 Review classifier routing for licence-bearing entries by hand; revise any card whose `when` lines route ambiguously.
- [ ] 7.3 Link the repository from the intentions README.
