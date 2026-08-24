---
name: test-coverage-design
disable-model-invocation: true
description: Design test coverage for an approved feature as an ID'd scenario table.
---

# Test Coverage Design

## Overview
Invoking this skill explicitly opts the current feature into test-design. Installation
alone never changes the planning or implementation workflow.

Decide what must be tested **before** tasks get drawn. Output is one approved coverage
source whose rows carry stable IDs that the plan's test steps cite and that
`coverage-verification` later measures the implementation against.

**Announce at start:** "I'm using the test-coverage-design skill to design the coverage for
this feature."

**Mode:** interactive. This stage needs the user's judgment on applicability, risk, and
scope. Do not run it silently to completion.

## Floor rule — when to skip this stage

If the feature has **≤2 behaviors and no persistence, no concurrency, and no external
dependency**, do not produce a separate document. Append a `## Test coverage` section to
the approved spec containing the scenario table, baseline commit, passed/failed/skipped
counts, and `## Baseline failures`; present that amendment for approval, then proceed to
`superpowers:writing-plans`.

Below that size the rows are the plan's test steps restated, and a separate doc costs two
human gates to re-derive them. Say which rule applied: "Floor rule applies — 2 behaviors,
no persistence. Appending the coverage source to the spec instead."

## Workflow

1. **Extract behaviors.** Read the approved spec. List every externally-observable
   capability. **Confirm the list with the user before sweeping** — a wrong behavior list
   makes every downstream row wrong, and this is the cheapest possible moment to fix it.
2. **Sweep.** Each behavior against all twelve axes in
   [references/sweep-axes.md](references/sweep-axes.md). Emit candidates.
3. **Prune.** Each survivor names one distinct code-level mutation. Cannot name one →
   delete. Same mutation twice → merge, retire the loser's ID.
4. **Derive risk** per [references/risk-derivation.md](references/risk-derivation.md).
   Never judge it; argue down only with recorded user approval.
5. **Assign level** per [references/level-assignment.md](references/level-assignment.md).
   Cheapest level that catches the mutation; axis 5 rows are `integration`+, axis 11 rows
   are `component`+.
6. **Add harness hints** for `integration`, `component`, `smoke`, and `e2e` rows.
7. **Record ruled-out axes** per behavior, each citing a spec fact or a code fact.
8. **Extract invariants** — properties holding across all scenarios, `INV-n`.
9. **List out-of-scope** items, each with a reason.
10. **Capture the baseline** — `git merge-base HEAD <default-branch>` and the suite result
    at that commit. Record the merge-base sha plus passed/failed/skipped counts in the
    header, and list any failing test names under a `## Baseline failures` section (empty if
    the suite was green). Verification compares against it so pre-existing failures are not
    charged to this feature.
11. Present to the user, get approval, write and commit.

## The existing suite is not an input

Rows come from the spec sweep. **Do not survey the tests that already exist** — no grep of
the test tree for a behavior before sweeping it, no row pruned or downgraded because
something that looks like it appears to be covered already. Step 10 runs the suite, but only
to record which tests were already failing; that baseline is a failure list, not a coverage
survey, and it never feeds the pruning rule in step 3.

An existing test proves something is exercised. It does not prove this row's `Bug it
catches` would be caught, and at design time nobody can tell the difference by reading it —
that is the exact judgment layers 2 and 4 exist to make, by verdicting the test against the
named mutation and by applying that mutation to the source. Deleting a row on inspection
buys the answer without the measurement, and the cost is asymmetric: the row is gone, its ID
was never issued, and no later layer can report on a row that does not exist. A redundant
row, the other way around, is one merge in step 3 or one `## Retired` line at amendment.

Reconciliation is post-factum by design. `coverage-verification` layer 1 greps for
`covers: SC-nn`, so an already-written test that does cover a row is credited by tagging it
— no new test needed — and layers 2 and 4 then verdict it like any other. That happens with
the code in front of it and a mutation to apply, which is when the question is actually
answerable.

## Row schema

`| ID | Behavior | Bug it catches | Level | Expected | Risk |`

No other columns. Priority, mission, and grouping live outside the table.

Harness hints for `integration`, `component`, `smoke`, and `e2e` rows go on `harness SC-nn: <hint>` lines below the table; each behavior group records which axis produced each surviving row.

| Column | Rule |
|---|---|
| `ID` | `SC-nn`, or `<area>/SC-nn` in a monorepo where `<area>` is the package the grep and suite scope to. Never renumbered. |
| `Behavior` | What is under test, one clause |
| `Bug it catches` | A **code-level mutation**: wrong operator, missing guard, swapped argument, absent await, inverted condition, omitted rollback |
| `Level` | `unit \| component \| integration \| smoke \| e2e` |
| `Expected` | The observable contract, specific enough to check mechanically |
| `Risk` | `low \| med \| high`, derived |

### `Bug it catches` must be a mutation, not a negation

<Good>
`guard uses < instead of <=` · `rollback call omitted on the error path` ·
`uniqueness check runs outside the transaction (TOCTOU)` · `await dropped on the publish call`
</Good>

<Bad>
`qty=0 is accepted` · `the write is not rolled back` · `a duplicate order is created`
</Bad>

The bad forms are the `Expected` column negated. That phrasing auto-fills for every row,
which makes the pruning rule vacuous — and it gives layer 4's mutator nothing to apply.
Every row must name something a person could actually type into the source.

## Artifact

Write to the **repository being worked on**, never into the plugin repo:

`<repo>/docs/superpowers/coverage/YYYY-MM-DD-<feature>-coverage.md`

In a monorepo where packages own their docs, the path is package-relative and IDs carry the
`<area>/` prefix.

See [references/example-coverage-doc.md](references/example-coverage-doc.md) for a filled
artifact. Use it for structure, never as the scope of a new feature.

## Size discipline

Smallest set naming distinct mutations. Typical feature: 8–25 rows **after pruning**. If
more than roughly forty survive, the spec covers too much — return to the decomposition
check in `superpowers:brainstorming` rather than writing a plan nobody finishes.

## Amendment

Implementation invalidates pre-implementation docs routinely: a new failure mode surfaces,
an error code changes, a sub-feature is dropped. Without an amend path, verification
measures the code against a stale yardstick and passes.

Amendments **append** rows with new IDs and move superseded ones to `## Retired`, with user
approval. Never renumber, never silently edit a row an implementer already tagged against.

## Definition of done

- [ ] Behavior list confirmed by the user before sweeping
- [ ] No row pruned or downgraded on the grounds that a test already exists
- [ ] Every behavior group has a `Ruled out:` line citing spec or code facts
- [ ] Axis 5 rule-outs name the absent shared resource
- [ ] Every `Bug it catches` is a mutation, not a negated `Expected`
- [ ] Risk derived by rule; every argue-down carries a recorded approval line
- [ ] Axis 5 rows `integration`+; axis 11 rows `component`+
- [ ] Harness hints on `integration`, `component`, `smoke`, `e2e` rows
- [ ] Invariants and out-of-scope both present
- [ ] Baseline commit recorded in the header
- [ ] Artifact committed to the target repo, not the plugin repo

## Next stage

**REQUIRED SUB-SKILL:** `superpowers:writing-plans`. Tell it to cite SC-IDs in each task's
test steps, so every row maps to a task and an uncovered row is visible as a plan gap.
This opt-in also commits the feature to `coverage-verification` at the
`finishing-a-development-branch` handoff.

Also tell it to require, on every implemented test, a comment immediately above it
containing the literal text `covers: SC-nn` — comma-separated for multiple IDs. More than 2
IDs on one test is an `overclaim` finding at verification: the test must then carry a
distinct, separately-nameable assertion per cited row, and layer 2 verdicts each row against
its own. Two is where that stops being routine — a row and its boundary case legitimately
share one test; a third row usually means the rows should have been merged in the source, or the
test split. `overclaim` blocks the branch while **unresolved**, and it is resolved either by
layer 2 verdicting every cited row against a named assertion, or by merging or splitting so
the tag drops to two. Do not read it as advisory. Only that substring is
load-bearing, so any comment syntax works (`//`, `#`, `--`, etc.). Citing SC-IDs in the plan
alone is not enough: `coverage-verification` layer 1 greps test source for this tag, not the
plan, so an untagged test reads as a `gap` even when the plan cited the row correctly.
