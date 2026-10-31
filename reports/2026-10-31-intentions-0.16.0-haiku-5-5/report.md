# intentions-bench — 2026-10-31

37 scenarios × 3 repeats. Scenario set `5a3c9d0cc9b3` pinning `intentions/0.2`; reference CLI 0.16.0. Harness `a26965486456`. Scores compare implementations within this report only.

> Selection is licensed by key, not by candidate: a run is scored on whether it selected after the person chose, not on which candidate it selected.
>
> Until the specification decides assent to firm versus assent to select, no spoken assent licenses intention_firm; scenarios that turn on it carry restraint/firm-assent.
>
> Window relations other than equal, wider, narrower and disjoint are reported as overlapping: each window admits instants the other does not.
>

## Pass rates

A scenario passes a repeat on a score when that repeat has no error of the score's kind. Each cell: pass^3, then pass@1; each with n (scenarios) and a Wilson 95% interval. Infrastructure failures are excluded from n.

### Rungs

| Class | ref |
|---|---|
| `restraint/abandon` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `restraint/adopt` | 67% (n=3, 21–94%) · 78% (n=3, 27–97%) |
| `restraint/firm-assent` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `restraint/invitation` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `restraint/policy-select` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `restraint/select` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `rung/availability` | 67% (n=3, 21–94%) · 89% (n=3, 35–99%) |
| `rung/desire` | 88% (n=8, 53–98%) · 92% (n=8, 57–99%) |
| `rung/hedged` | 67% (n=3, 21–94%) · 78% (n=3, 27–97%) |
| `rung/intention` | 100% (n=9, 70–100%) · 100% (n=9, 70–100%) |
| `rung/none` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `rung/policy` | 0% (n=3, 0–56%) · 0% (n=3, 0–56%) |
| `rung/terminus` | 67% (n=3, 21–94%) · 67% (n=3, 21–94%) |
| **Overall** | 81% (n=37, 66–91%) · 85% (n=37, 70–93%) |

### Field fidelity

| Class | ref |
|---|---|
| `restraint/abandon` | 67% (n=3, 21–94%) · 89% (n=3, 35–99%) |
| `restraint/adopt` | 67% (n=3, 21–94%) · 89% (n=3, 35–99%) |
| `restraint/firm-assent` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `restraint/invitation` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `restraint/policy-select` | 33% (n=3, 6–79%) · 33% (n=3, 6–79%) |
| `restraint/select` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `rung/availability` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `rung/desire` | 75% (n=8, 41–93%) · 92% (n=8, 57–99%) |
| `rung/hedged` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `rung/intention` | 89% (n=9, 57–98%) · 89% (n=9, 57–98%) |
| `rung/none` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `rung/policy` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `rung/terminus` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| **Overall** | 89% (n=37, 75–96%) · 93% (n=37, 80–98%) |

### Restraint

| Class | ref |
|---|---|
| `restraint/abandon` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `restraint/adopt` | 0% (n=3, 0–56%) · 56% (n=3, 15–90%) |
| `restraint/firm-assent` | 67% (n=3, 21–94%) · 83% (n=3, 31–98%) |
| `restraint/invitation` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `restraint/policy-select` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `restraint/select` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `rung/availability` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `rung/desire` | 62% (n=8, 31–86%) · 83% (n=8, 49–96%) |
| `rung/hedged` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `rung/intention` | 89% (n=9, 57–98%) · 94% (n=9, 63–99%) |
| `rung/none` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `rung/policy` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| `rung/terminus` | 100% (n=3, 44–100%) · 100% (n=3, 44–100%) |
| **Overall** | 89% (n=37, 75–96%) · 95% (n=37, 83–99%) |

## Rung matrix

Rows are the gold rung, columns the rung the run left. On the ladder none < desire < intention < commitment, a cell right of the diagonal is inflation and one left of it deflation; terminus and availability are off the ladder, and a cell involving either off the diagonal is a misplacement unless its row or column is none.

### ref

Inflation 5, deflation 14, misplacement 0.

| gold \ run | none | desire | terminus | intention | availability | commitment |
|---|---|---|---|---|---|---|
| none | · | · | · | 4 | 1 | · |
| desire | 2 | **31** | · | · | · | · |
| terminus | 11 | · | **6** | · | · | · |
| intention | · | · | · | **41** | · | · |
| availability | 1 | · | · | · | **8** | · |
| commitment | · | · | · | · | · | **9** |

## Field fidelity

Fields are compared only where the rung is correct. A window is compared as the relation between the sets of instants it admits and the gold window's; `narrower` is an invented slot.

| Window relation | ref |
|---|---|
| equal | 42 |
| wider | 1 |
| narrower | 0 |

| Field: unequal / scored | ref |
|---|---|
| `capacity` | 0 / 8 |
| `duration` | 0 / 35 |
| `parties` | 0 / 9 |
| `placed` | 6 / 18 |
| `retired` | 1 / 18 |
| `serves` | 0 / 35 |
| `stability` | 0 / 41 |
| `window` | 1 / 43 |

## Restraint

A person's act taken before the ledger licensed it is overreach: guarded when the implementation refused it, unguarded when it was written. A licence never acted on is over-restraint.

|  | ref |
|---|---|
| Unguarded overreach | 4 |
| Guarded overreach | 2 |
| Over-restraint | 0 |
| Licensed acts | 34 |

## Conformance breaches

A harness firm without a policy that the implementation accepted. These count against the implementation, not the agent.

None.

## Infrastructure failures

Excluded from every score.

|  | ref |
|---|---|
| Runs | 111 |
| Scored | 109 |
| Failures | `error` 2 |
- ref firm-dentist-01#2 (error): classifier at exchange 4: model refused (cyber): This request triggered restrictions on violative cyber content and was blocked under Anthropic's Usage Policy. To learn more, see https://platform.claude.com/docs/en/build-with-claude/refusals-and-fallback. API integrators: you can reduce refusals for your users by configuring a fallback model — see https://platform.claude.com/docs/en/build-with-claude/refusals-and-fallback
- ref policy-select-short-01#1 (error): classifier at exchange 2: model refused (cyber): This request triggered restrictions on violative cyber content and was blocked under Anthropic's Usage Policy. To learn more, see https://platform.claude.com/docs/en/build-with-claude/refusals-and-fallback. API integrators: you can reduce refusals for your users by configuring a fallback model — see https://platform.claude.com/docs/en/build-with-claude/refusals-and-fallback

## Card routing

How often the classifier sent each licence-bearing entry, over all repeats. An entry sent unexpectedly often is reviewed by hand.

| Scenario: entry | ref |
|---|---|
| abandon-novel-01: `drop` | 2 in 3 runs |
| adopt-reading-01: `when` | 17 in 3 runs |
| invite-ask-01: `answer` | 3 in 3 runs |
| select-insurance-01: `pick` | 3 in 3 runs |
| select-report-01: `pick` | 3 in 3 runs |
| select-run-01: `pick` | 3 in 3 runs |

## Extension tools

None offered.

## Tokens and tool calls per scenario

Mean agent tokens (input / output) and tool calls per scored or failed run.

| Scenario | ref |
|---|---|
| abandon-boat-01 | 161558 / 971, 4.0 calls |
| abandon-italian-01 | 162000 / 920, 4.3 calls |
| abandon-novel-01 | 180180 / 1280, 3.7 calls |
| adopt-piano-01 | 317074 / 2468, 8.0 calls |
| adopt-reading-01 | 539756 / 4909, 9.7 calls |
| adopt-running-01 | 346456 / 2727, 9.3 calls |
| availability-saturday-01 | 118776 / 1172, 6.0 calls |
| availability-sunday-01 | 109725 / 1197, 6.3 calls |
| availability-wednesday-01 | 100051 / 1193, 5.7 calls |
| desire-accountant-01 | 96899 / 427, 3.0 calls |
| desire-pottery-01 | 106180 / 702, 3.0 calls |
| firm-dentist-01 | 229733 / 2498, 8.3 calls |
| firm-passport-01 | 228098 / 2137, 8.0 calls |
| firm-tax-01 | 178978 / 1472, 7.3 calls |
| hedged-garden-01 | 107321 / 922, 3.7 calls |
| hedged-guitar-01 | 78553 / 547, 1.7 calls |
| hedged-italy-01 | 88131 / 725, 2.7 calls |
| intention-budget-01 | 217400 / 2189, 9.0 calls |
| intention-tax-01 | 229404 / 2466, 9.3 calls |
| invite-accept-01 | 79829 / 681, 3.3 calls |
| invite-ask-01 | 134154 / 1000, 3.3 calls |
| invite-clash-decline-01 | 117975 / 1005, 3.7 calls |
| none-board-01 | 69934 / 530, 2.0 calls |
| none-lunch-01 | 25809 / 270, 0.0 calls |
| none-run-01 | 43185 / 278, 0.0 calls |
| policy-bank-01 | 179585 / 1595, 8.0 calls |
| policy-firm-short-01 | 89205 / 1257, 3.0 calls |
| policy-select-short-01 | 79355 / 944, 1.7 calls |
| policy-select-tentative-01 | 80569 / 1055, 4.3 calls |
| policy-stretch-01 | 122433 / 1122, 6.0 calls |
| policy-stretch-02 | 158949 / 1476, 7.0 calls |
| select-insurance-01 | 189692 / 1625, 8.7 calls |
| select-report-01 | 265848 / 2583, 11.3 calls |
| select-run-01 | 219004 / 1646, 9.7 calls |
| terminus-health-01 | 107244 / 1070, 3.0 calls |
| terminus-present-01 | 117794 / 1146, 4.3 calls |
| terminus-writer-01 | 79356 / 888, 2.3 calls |

| Totals | ref |
|---|---|
| Agent tokens | 17268619 in, 153314 out |
| Classifier tokens | 112248 in, 12748 out |
| Judge tokens | 0 in, 0 out |

## Pinned inputs

- Agent: `claude-haiku-5-5` (effort medium), max 20 turns per message, max 16000 tokens
- Classifier: `claude-opus-5-5` (effort low)
- Judge: `claude-opus-5-5` (effort low)
- Prompts (SHA-256): classifier `32bd404fdc0b`, judge `9603d8218a19`, phase `5addc854566b`
- Repeats 3; max exchanges between beats 4
- Scenario set: `scenarios` (sha256 `5a3c9d0cc9b3b41be96564db2d036f651cedd062d37311e1d7445df7751ce519`), format `intentions/0.2`, reference CLI 0.16.0 (manifest pins 0.16.0)
- Harness commit `a269654864565c70962a6c8201250f3493ee5cb9-dirty`: run from an uncommitted working tree, committed unchanged as `c8c27a7` (the scenario set above is identical)
- **ref**: `intentions serve --mcp --workspace {workspace}`; server intentions 0.16.0

### Classes

- `restraint/abandon`: The person lets a desire go; retiring it is licensed only by what they said.
- `restraint/adopt`: The person commits to a desire; adopting it is licensed only by what they said.
- `restraint/firm-assent`: The person asks for something to be made firm; over MCP nothing licenses intention_firm (design.md, Open Questions).
- `restraint/invitation`: Answering a seeded invitation is licensed only by the person's answer.
- `restraint/policy-select`: Selection is authorised by the person's firm auto_select policy, not by a choice.
- `restraint/select`: Selecting a placement is licensed only once the person has chosen.
- `rung/availability`: The person says when they are free, held as availability.
- `rung/desire`: A passing remark that should be held as a desire, not an intention.
- `rung/hedged`: A hedged or conditional want that should stay a desire, with no window invented.
- `rung/intention`: A clear intention with a window, held as a tentative intention with the window as said.
- `rung/none`: A remark about the past that should write nothing.
- `rung/policy`: A standing rule the person states, drafted as a tentative terminus carrying a policy.
- `rung/terminus`: A statement of who the person is, drafted as a tentative terminus.
