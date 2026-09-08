# W-1 resolution: real-agent walkthrough of the scenario reviewer

Reviewer instructions: `agents/scenario-reviewer.md` at revision 1 (commit `75c0902`,
`git hash-object` `1cad237c1e6cc7079943fff84c26950de3ffd24c`; see `instructions-revision.txt`).
One revision was needed: every seed in `seeds.md` was cited on the first pass, so step 6 of the
task (amend and re-dispatch) did not run.

Dispatch shape: Claude Code `Agent` tool, agent type `general-purpose`, session model (no
`model:` pin, matching the plugin agent's absent `model:` line), prompt = the verbatim body of
`agents/scenario-reviewer.md` followed by an inputs block naming the requirements, the draft, the
repository root, the mode and (for `recheck`) the previous findings, plus a stated read-only
rule. Deviation from the plugin agent: `general-purpose` carries every tool, not the plugin's
`tools: Read, Grep, Glob` line, so the read-only property is proved by `git status --short` after
each dispatch rather than by tool restriction. Transcripts are the agents' final messages,
saved verbatim (angle brackets restored where the harness transport HTML-escaped them):
`review-a.md`, `review-b.md`, `review-risk.md`, `recheck-a.md`, `recheck-b.md`.

Scoring follows `seeds.md` "Scoring rules for unseeded findings": every finding is exactly one
of seed hit, seed miss, borderline, unseeded true-extra, unseeded false-positive. One reading
rule was added by the controller and is recorded in the wave ledger: a finding that names a
seeded redundancy explicitly counts as a hit even when its verb is `DROP` or `REWRITE` rather
than the key's `MERGE`; the verb deviation is recorded beside the hit.

## draft-a.md — four seeds, four hits

| Seed | Row | Expected | Finding in `review-a.md` | Outcome |
|------|-----|----------|--------------------------|---------|
| A-1 | SC-10 redundant with SC-06 | MERGE into SC-06 | `DROP SC-10` — "same input (ValidateAmount(0)), same mutation and a weaker Expected than SC-06; one defect at payments.go:6 kills both, and SC-06 already carries the message" | hit (verb DROP naming SC-06 as the covering row; the key notes A-1 is the mechanically easy seed) |
| A-2 | SC-13 invented loyalty tier | DROP | `DROP SC-13` — "no requirement mentions a loyalty tier and payments.go has no tier code" | hit |
| A-3 | SC-09 unobservable expected result | REWRITE (DROP also defensible) | `DROP SC-09` — "no observable result: a caller cannot tell a switch from a linear scan" | hit (DROP, allowed by the key) |
| A-4 | `ReserveBatch` omitted | ADD | three `ADD` findings citing R-2.3/R-2.4/R-2.5 and `orders.go:19-26` (all-positive batch accepted; middle-position non-positive rejected; empty batch accepted) | hit (the load-bearing absence seed) |

Unseeded findings on draft A:

| Finding | Outcome | Basis |
|---------|---------|-------|
| `REWRITE SC-07` (assert USD, EUR and GBP, not EUR alone) | false-positive | the key calls SC-07 good; the draft's scope note holds the other spellings for a second pass. The argument is the draft-B SC-36/SC-37 borderline argument applied to draft A, and the key blesses that mutation-literal reading as borderline there, so "unseeded true-extra" is a defensible alternative label; recorded as false-positive because the key calls SC-07 good. Author rejected it. |
| `REWRITE SC-14` (schedule index, not a formula base; say how the delay is observed) | true-extra | R-5.1 defines a schedule, so the draft's "formula base" mutation had no site. Author applied it. |
| `REWRITE SC-15` (state the invocation count) | true-extra | the `<`/`<=` mutation survives "no further attempt" as written. Author applied it. |
| `LIVE` reason clause "keeps no state between calls" contradicts SC-11/SC-12 | true-extra | the not-applicable decision stands; only the clause was struck. Author applied it. |
| `ESCALATE` guards accept zero (`< 0`) against R-1.2/R-3.2 | true-extra | rule 1 of the key: shipped code contradicts spec-faithful rows. For the human. |
| `ESCALATE` SC-06 risk may be high | false-positive | the key calls draft-A risk correct; a zero-cent charge settling an order is a med consequence with no evidence of an irreversible loss. For the human (escalations are not table edits). |
| `ESCALATE` R-4/R-5 unimplemented | true-extra | rule 2 of the key; the reviewer did not ask to drop the rows. For the human. |
| `ESCALATE` fixture `covers:` tags bind the draft's IDs to other tests | true-extra | a real property of `fixtures/go-project`, outside the table. For the human. |

Repair (`repaired-a.md`): SC-09, SC-10, SC-13 retired; SC-16, SC-17, SC-18 (`ReserveBatch`) added;
SC-14, SC-15 rewritten; live reason clause struck; REWRITE SC-07 rejected; the four escalations are
carried to the human. Cost line: 15 surviving scenarios, 1 high-risk obligation, 0 live journeys.

## draft-b.md — the >40 variant

Before review: 43 scenarios, 6 high, 0 journeys (the reviewer's `COUNTS scenarios=43 high=6
journeys=0` matches). The reviewer ran the focused redundancy and scope pass the size rule asks for.

Firm merges (14 expected, 14 cited):

| Row | Expected merge into | Finding | Outcome |
|-----|---------------------|---------|---------|
| SC-17 | SC-01 | `MERGE SC-01 SC-16 SC-17` | hit (the finding keeps SC-16 rather than SC-01 as the survivor; see the firm-keeps table) |
| SC-18 | SC-01 | `REWRITE SC-18` — "its mutation is SC-01's in other words, so as written it is a duplicate" | hit (verb REWRITE; the reviewer proposed repurposing the ID as a no-ceiling row, which the author rejected as a requirement the spec does not state) |
| SC-19 | SC-02 | `MERGE SC-02 SC-19 SC-20` | hit |
| SC-20 | SC-02 | `MERGE SC-02 SC-19 SC-20` | hit |
| SC-22 | SC-03 | `MERGE SC-03 SC-22` | hit |
| SC-23 | SC-04 | `MERGE SC-04 SC-23 SC-24` | hit |
| SC-24 | SC-04 | `MERGE SC-04 SC-23 SC-24` | hit |
| SC-29 | SC-26 | `REWRITE SC-29` — "as written ([0, 5]) it dies with SC-26 on the same per-element `<=`" | hit (verb REWRITE; the reviewer proposed repurposing it as the middle-position row; the author retired it and left the middle-position case for a later pass) |
| SC-32 | SC-05 | `MERGE SC-05 SC-31 SC-32` | hit |
| SC-33 | SC-06 | `MERGE SC-06 SC-33 SC-34` | hit |
| SC-34 | SC-06 | `MERGE SC-06 SC-33 SC-34` | hit |
| SC-38 | SC-08 | `MERGE SC-08 SC-38` | hit |
| SC-41 | SC-11 | `MERGE SC-11 SC-41` | hit |
| SC-44 | SC-14 | `MERGE SC-14 SC-44` | hit |

Borderline rows (either resolution correct):

| Row | Reviewer | Basis stated | Outcome |
|-----|----------|--------------|---------|
| SC-21 | `REWRITE SC-21` — keep, with `qty == 0` as the mutation | mutation distinctness (the inverted comparison is already killed by SC-02) | borderline, keep side |
| SC-35 | `REWRITE SC-35` — keep, same reasoning | mutation distinctness | borderline, keep side |
| SC-36 | no finding | (mutation column read literally: USD dropped is a different mutation) | borderline, keep side |
| SC-37 | no finding | as SC-36 | borderline, keep side |

The reviewer applied one basis — the mutation column — to all four borderline rows, which is the
consistency the key asks for.

Firm keeps touched by findings:

| Finding | Outcome | Basis |
|---------|---------|-------|
| `MERGE SC-01 SC-16 SC-17` collapses SC-01 into SC-16 (keep the boundary 1) | false-positive for SC-01 | the key keeps both the accepted-class representative and the boundary; the reviewer's row loses no coverage, but it merges away a row the key calls good. Author kept SC-01 and SC-16, retired SC-17. |
| `MERGE SC-05 SC-31 SC-32` collapses SC-05 into SC-31 | false-positive for SC-05 | as above. Author kept SC-05 and SC-31, retired SC-32. |
| `MERGE SC-03 SC-22` also proposes Reserve(1) as the surviving value | false-positive for the value change | the same pattern: the key keeps SC-03 as the accepted-reservation representative (Reserve(5)); the merge of SC-22 is the hit, the value change is not. Author retired SC-22 and kept Reserve(5). |
| `REWRITE SC-30`, `REWRITE SC-42`, `REWRITE SC-46` (integration not earned; in-process races are unit) | false-positive | the key keeps all three at integration; SC-30's "no stock state" point is rule 2 (unimplemented behavior, rows legitimate). Author rejected. |
| `RISK SC-12 med→high` | false-positive | the key states draft-B risk is correct throughout; a refused distinct payment is recoverable, not one of the four categories. Author rejected. |
| `ESCALATE SC-46 high carries no impact rationale` | false-positive | SC-46 is high under "harmful shared-state failure", a named category that needs no rationale line. For the human. |

Other unseeded findings on draft B:

| Finding | Outcome | Basis |
|---------|---------|-------|
| `ADD Reserve(-1) is rejected` (R-2.2) | true-extra | no reservation row used a negative value. Author added SC-47. |
| `ADD in-schedule value above the maximum is capped` (R-5.2 first sentence) | true-extra | SC-45 covers only past-the-end. Author added SC-48. |
| `LIVE` reason clause "keeps no state between calls" | true-extra | as draft A; clause struck. |
| `ESCALATE` guards accept zero | true-extra | rule 1. For the human. |
| `ESCALATE` R-2.6/R-4/R-5 unimplemented | true-extra | rule 2; the reviewer named both consequences without asking to drop. For the human. |
| `ESCALATE` draft IDs collide with the fixture's existing `covers:` tags and its own coverage doc | true-extra | a real property of the fixture. For the human. |

After repair (`repaired-b.md`): 14 retired (SC-17, SC-18, SC-19, SC-20, SC-22, SC-23, SC-24, SC-29,
SC-32, SC-33, SC-34, SC-38, SC-41, SC-44), 29 surviving of the original 43 — inside the key's 25-29
range, at its maximum because all four borderline rows were kept — plus 2 added (SC-47, SC-48) = 31
active. High: 6 → 5 (SC-41 retired). Journeys: 0 → 0.

## risk-contrast.md — the silent downgrade

| Seed | Expected | Finding in `review-risk.md` | Outcome |
|------|----------|-----------------------------|---------|
| R-1 | RISK naming SC-02 (amended) and the missing amendment; escalation also correct | `RISK SC-02 (amended) med→high` — "a security-boundary violation is always high; the original's `high` stands, and the downgrade would also pull the row out of the mutation-verified set"; plus `ESCALATE the document carries two risk assignments over one ID set and names neither as governing` | hit — the downgrade was flagged, named as a security-boundary violation, tied to the approved original, and escalated |

Unseeded findings on the risk contrast: the reviewer treated the exhibit as a full draft (its
own prose says it is not a coverage source), so it also emitted fourteen `ADD` findings for every
requirement section, `REWRITE SC-03` and `REWRITE SC-05`, `ESCALATE` for the unimplemented
SC-02/SC-04/SC-05 subjects, `ESCALATE` for the zero-accepting guards and `LIVE no ## Live decision
section is present`. The `ADD`s, the two escalations and the `LIVE` finding are true-extra (each
carries a requirement or `file:line` basis, and the document has no live decision); `REWRITE
SC-03` and `REWRITE SC-05` are false-positive by the key (everything but SC-02's downgrade is
intended to be correct), though SC-03's "no reserved total exists to observe" is rule 2 again.
SC-05's rationale line was not questioned, as the key expects.

## Recheck (one per draft)

### repaired-a.md (`recheck-a.md`)

Fourteen previous findings answered, each exactly once. `RESOLVED`: DROP SC-09, DROP SC-10, DROP
SC-13, REWRITE SC-14, REWRITE SC-15, the three `ReserveBatch` ADDs (now SC-16, SC-17, SC-18) and
the LIVE clause — every seed hit and every applied unseeded finding was confirmed against the
amended rows with evidence (`file:line` for the ADDs; the Retired section for the DROPs). `OPEN`: REWRITE SC-07 (the author's rejection is not
visible to the recheck, as the design intends, so it goes to the human with its consequence: a
USD or GBP dropped from the switch survives the row) and the four escalations, each restated
against the amended draft; the `covers:` escalation grew sharper (retired SC-09/SC-10 still bound
in the fixture tests; SC-16, SC-17, SC-18 have no test yet). No `NEW GAP` label was emitted, but two
new findings on the amended draft appeared in the normal format: `REWRITE SC-12` (the amount-keyed
mutation survives unless both charges share one amount — a real under-determination the Task 9a
review had noted as a deferred minor) and `ESCALATE final-command names draft-a.md` (a true
property of the repaired copy, which keeps the original's `## Verification` pointer). Fresh
counts `scenarios=15 high=1 journeys=0` match the cost line. Leftovers for the human: the four
escalations, REWRITE SC-07, REWRITE SC-12 and the final-command pointer. There is no further round.

### repaired-b.md (`recheck-b.md`)

Twenty-four previous findings answered, each exactly once. `RESOLVED` (10): the firm merges the
author applied as stated (SC-19/SC-20, SC-23/SC-24, SC-33/SC-34, SC-38, SC-41), the two borderline
rewrites (SC-21, SC-35), both ADDs (SC-47, SC-48) and the LIVE clause. `OPEN` (14): every finding the
author rejected came back OPEN, as the design intends — the recheck sees the amended draft and its
own findings, never the author's reasons — and each carries its consequence for the human: the
SC-01→SC-16 and SC-05→SC-31 collapses (the author kept both class representative and boundary),
the `MERGE SC-03 SC-22` value change (SC-22 is retired, but the reviewer also wants Reserve(1) as the
surviving value; the author kept Reserve(5) — a one-unit reservation refused in production is the
consequence it names),
the SC-18 repurpose (no-ceiling row), the SC-29 repurpose (middle-position batch, which the author
left for a later pass), the three level rewrites (SC-30, SC-42, SC-46), RISK SC-12, the SC-46
rationale escalation, and the three factual escalations restated against the tree. One more merge the
author applied came back OPEN on a different point: `MERGE SC-14 SC-44` — SC-44 is retired, but the
reviewer's second clause (the "formula base" mutation has no site under R-5.1's schedule) was not
applied to SC-14 in draft B, although it was in draft A; a real inconsistency in the repair, not in
the review. No `NEW GAP` label was emitted; one new finding in the normal format, `REWRITE SC-45`
(the cap mutation is not applicable without guessing), is a fair observation the first pass missed.
Fresh counts `scenarios=31 high=5 journeys=0` match the cost line. Leftovers for the human: the
fourteen OPEN findings (`MERGE SC-03 SC-22` among them) and REWRITE SC-45. There is no further round.

### What the walkthrough shows

- Every seed was cited on the first pass with revision 1 of the instructions: A-1, A-2, A-3, A-4
  (draft A), all fourteen firm merges (draft B), R-1 (risk contrast). The load-bearing seeds — A-4's
  absence and R-1's silent security downgrade — were both found with a `file:line` or requirement
  basis, so the hit rate does not rest on the mechanically easy A-1.
- The reviewer over-reaches in two repeatable ways, both recorded above as false-positives:
  it collapses a class representative into its boundary row (SC-01/SC-16, SC-05/SC-31, and the
  Reserve(1) proposal on SC-03/SC-22) and it
  argues integration down to unit for in-process races (SC-30, SC-42, SC-46). Neither weakens a
  seed hit; both are the author's call and were rejected.
- The recheck contract behaves as specified: applied findings come back `RESOLVED` with evidence,
  rejected findings come back `OPEN` with a consequence, and the leftovers are listed for the
  human rather than looped. One recheck also caught a real repair inconsistency (SC-14's mutation).
- The `general-purpose` agents edited nothing: every `git status --short` below shows only the
  files this task wrote.

Repair inconsistency noted for the record, not fixed after the fact (the task forbids editing
inputs or outputs once produced): `repaired-b.md` SC-14 keeps the "formula base" mutation that
`repaired-a.md` SC-14 replaced.


## Read-only proof

`git status --short` after each dispatch, pasted verbatim:

### git status --short after draft-a review dispatch (revision 1)
```
 M .beads/issues.jsonl
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/instructions-revision.txt
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/review-a.md
```
(instructions-revision.txt and review-a.md are files this task wrote; .beads/issues.jsonl is the br work-tracker file modified by issue closes before this task.)

### git status --short after draft-b review dispatch (revision 1)
```
 M .beads/issues.jsonl
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/instructions-revision.txt
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/review-a.md
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/review-b.md
```

### git status --short after risk-contrast review dispatch (revision 1)
```
 M .beads/issues.jsonl
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/instructions-revision.txt
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/review-a.md
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/review-b.md
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/review-risk.md
```

### git status --short after repaired-a recheck dispatch (revision 1)
```
 M .beads/issues.jsonl
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/instructions-revision.txt
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/recheck-a.md
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/repaired-a.md
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/repaired-b.md
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/resolution.md
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/review-a.md
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/review-b.md
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/review-risk.md
```

### git status --short after repaired-b recheck dispatch (revision 1)
```
 M .beads/issues.jsonl
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/instructions-revision.txt
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/recheck-a.md
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/recheck-b.md
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/repaired-a.md
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/repaired-b.md
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/resolution.md
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/review-a.md
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/review-b.md
?? docs/superpowers/coverage/walkthroughs/2026-09-07-w1/review-risk.md
```
