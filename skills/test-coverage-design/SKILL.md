---
name: test-coverage-design
description: Use after a spec is approved and before writing an implementation plan - designs the test coverage the feature needs, as an ID'd scenario table that the plan and later verification both cite
---

# Test Coverage Design

## Overview

Decide what must be tested **before** tasks get drawn. Output is one coverage doc whose
rows carry stable IDs that the plan's test steps cite and that `coverage-verification`
later measures the implementation against.

**Announce at start:** "I'm using the test-coverage-design skill to design the coverage for
this feature."

**Mode:** interactive. This stage needs the user's judgment on applicability, risk, and
scope. Do not run it silently to completion.

## Floor rule — when to skip this stage

If the feature has **≤2 behaviors and no persistence, no concurrency, and no external
dependency**, do not produce a separate document. Append the scenario table inline to the
spec at brainstorming's spec-review gate and proceed to `superpowers:writing-plans`.

Below that size the rows are the plan's test steps restated, and a separate doc costs two
human gates to re-derive them. Say which rule applied: "Floor rule applies — 2 behaviors,
no persistence. Appending the table to the spec instead."

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
6. **Add harness hints** for `component`, `smoke`, and `e2e` rows.
7. **Record ruled-out axes** per behavior, each citing a spec fact or a code fact.
8. **Extract invariants** — properties holding across all scenarios, `INV-n`.
9. **List out-of-scope** items, each with a reason.
10. **Capture the baseline** — `git merge-base HEAD <default-branch>` and the suite result
    at that commit. Verification compares against it so pre-existing failures are not
    charged to this feature.
11. Present to the user, get approval, write and commit.

## Row schema

`| ID | Behavior | Bug it catches | Level | Expected | Risk |`

No other columns. Priority, mission, and grouping live outside the table.

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
- [ ] Every behavior group has a `Ruled out:` line citing spec or code facts
- [ ] Axis 5 rule-outs name the absent shared resource
- [ ] Every `Bug it catches` is a mutation, not a negated `Expected`
- [ ] Risk derived by rule; every argue-down carries a recorded approval line
- [ ] Axis 5 rows `integration`+; axis 11 rows `component`+
- [ ] Harness hints on `component`, `smoke`, `e2e` rows
- [ ] Invariants and out-of-scope both present
- [ ] Baseline commit recorded in the header
- [ ] Artifact committed to the target repo, not the plugin repo

## Next stage

**REQUIRED SUB-SKILL:** `superpowers:writing-plans`. Tell it to cite SC-IDs in each task's
test steps, so every row maps to a task and an uncovered row is visible as a plan gap.
