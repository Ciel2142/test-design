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

## Prerequisite

ID allocation, source validation, and baseline capture run through a Go executable
committed to the plugin, invoked as:

```text
go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier <op> …
```

This requires a Go 1.26+ toolchain on the machine running the skill. The module has no
third-party dependencies, so no separate build step or committed binary is needed.

## Floor rule — when to skip this stage

If the feature has **≤2 behaviors and no persistence, no concurrency, and no external
dependency**, do not produce a separate document. Append a `## Test coverage` section to
the approved spec containing the scenario table, the `## Verification` block, the
`Baseline:` header line, and a `## Baseline failures` section; present that amendment for
approval, then proceed to `superpowers:writing-plans`. The section follows the same
schema, ID allocation, and two-commit baseline protocol as a standalone coverage source —
the floor rule only changes where it lives, never what it must contain.

Below that size the rows are the plan's test steps restated, and a separate doc costs two
human gates to re-derive them. Say which rule applied: "Floor rule applies — 2 behaviors,
no persistence. Appending the coverage source to the spec instead."

## Workflow

1. **Allocate the starting ID.** Before sweeping, run

   ```text
   go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier next-id -repo . -area <area>
   ```

   Always pass `-area`, even as the empty string for a single-project repo (`-area ""`):
   the flag must appear on the command line for its value to count as set, and the area you
   name is the same prefix your rows will carry (`<area>/SC-nn`, or bare `SC-nn` when
   `<area>` is empty). Omitting `-area` instead lists every area the repo-wide registry
   already knows about, which is empty for a repo's first coverage document and does not
   name the area you are about to write into. The output is `{"nextIds":[{"kind":"sc",
   "area":"<area>","next":<n>}]}` — `next` is the first unused number for that area, scanned
   across every existing coverage document and every spec carrying a floor-rule table,
   old-header documents included. Number every new row from `next`, incrementing locally;
   never start at 1 if `next` is not 1.
2. **Extract behaviors.** Read the approved spec. List every externally-observable
   capability. **Confirm the list with the user before sweeping** — a wrong behavior list
   makes every downstream row wrong, and this is the cheapest possible moment to fix it.
3. **Sweep.** Each behavior against all twelve axes in
   [references/sweep-axes.md](references/sweep-axes.md). Emit candidates.
4. **Prune.** Each survivor names one distinct code-level mutation. Cannot name one →
   delete. Same mutation twice → merge, retire the loser's ID.
5. **Derive risk** per [references/risk-derivation.md](references/risk-derivation.md).
   Never judge it; argue down only with recorded user approval.
6. **Assign level** per [references/level-assignment.md](references/level-assignment.md).
   Cheapest level that catches the mutation; axis 5 rows are `integration`+, axis 11 rows
   are `component`+.
7. **Add harness hints** for `integration`, `component`, `smoke`, and `e2e` rows.
8. **Record ruled-out axes** per behavior, each citing a spec fact or a code fact.
9. **Extract invariants** — properties holding across all scenarios, `INV-n`.
10. **List out-of-scope** items, each with a reason.
11. **Write the `## Verification` block** (see [below](#verification-block)) naming the
    runner, its command, report globs, the baseline evidence path, and — when any row is
    `smoke` or `e2e` — the live profile.
12. **Capture the baseline**, in two commits (see [below](#baseline-capture)).
13. Present to the user, get approval, write and commit.

## The existing suite is not an input

Rows come from the spec sweep. **Do not survey the tests that already exist** — no
inspection of the test tree for a behavior before sweeping it, no row pruned or downgraded
because something that looks like it appears to be covered already. Step 12 runs the
baseline suite, but only to record which tests were already failing; that baseline is a
failure list, not a coverage survey, and it never feeds the pruning rule in step 4.

An existing test proves something is exercised. It does not prove this row's `Verification
mutation` would be caught, and at design time nobody can tell the difference by reading it —
that is the exact judgment layers 2 and 4 exist to make, by verdicting the test against the
named mutation and by applying that mutation to the source. Deleting a row on inspection
buys the answer without the measurement, and the cost is asymmetric: the row is gone, its ID
was never issued, and no later layer can report on a row that does not exist. A redundant
row, the other way around, is one merge in step 4 or one `## Retired` line at amendment.

Reconciliation is post-factum by design. `coverage-verification` layer 1 binds each `covers:
SC-nn` tag to its declaration through Go's parser and AST, or the JVM token scanner, never
by searching for the literal tag text — so an already-written test that does cover a row is
credited by tagging it, no new test needed, and layers 2 and 4 then verdict it like any
other. That happens with the code in front of it and a mutation to apply, which is when the
question is actually answerable.

## Row schema

`| ID | Behavior | Expected | Level | Verification mutation | Risk |`

No other columns. `Expected` precedes the mutation because implementation is driven by
observable behavior, not by the fault that will later probe it. Priority, mission, and
grouping live outside the table.

Harness hints for `integration`, `component`, `smoke`, and `e2e` rows go on `harness SC-nn: <hint>` lines below the table; each behavior group records which axis produced each surviving row.

| Column | Rule |
|---|---|
| `ID` | `SC-nn`, or `<area>/SC-nn` in a monorepo where `<area>` is the package the runner and suite scope to. Allocated from `next-id`; never renumbered. |
| `Behavior` | What is under test, one clause |
| `Expected` | The observable contract, specific enough to check mechanically |
| `Level` | `unit \| component \| integration \| smoke \| e2e` |
| `Verification mutation` | A **code-level mutation**: wrong operator, missing guard, swapped argument, absent call, inverted condition, omitted rollback |
| `Risk` | `low \| med \| high`, derived |

### `Verification mutation` must be a mutation, not a negation

<Good>
`guard uses < instead of <=` · `rollback call omitted on the error path` ·
`uniqueness check runs outside the transaction (TOCTOU)` · `await dropped on the publish call`
</Good>

<Bad>
`qty=0 is accepted` · `the write is not rolled back` · `a duplicate order is created`
</Bad>

The bad forms are the `Expected` column negated. That phrasing auto-fills for every row,
which makes the pruning rule vacuous — and it gives the layer-4 mutator nothing to apply.
Every row must name something a person could actually type into the source. Only the final
mutator applies it, after the completed implementation is green, in a disposable worktree —
design time only names the fault, it never introduces it.

## Verification block

Every coverage source carries a `## Verification` bullet list, machine-parsed by the
verifier. Required keys:

| Key | Value |
|---|---|
| `runner-kind` | `go`, `maven`, or `gradle` |
| `runner-root` | Path, relative to the repo root, of the affected module or project |
| `runner-command` | The suite command as a JSON array of strings |
| `report-globs` | JSON array of report-file globs, relative to `runner-root`; may be `[]` for `go`, required non-empty for `maven`/`gradle` |
| `baseline-evidence` | Repo-relative path where `baseline` publishes its evidence file |
| `baseline-sha256` | SHA-256 the verifier prints after `baseline` runs; `pending` until then |
| `final-command` | The exact explicit final-verification command, as a JSON array: `["/test-design:coverage-verification", "<repo-relative path to this document>"]` |

Optional key `live-profile` names a committed profile at
`.test-design/live-profiles/<name>.json`, repo-relative. It is required whenever any row is
`smoke` or `e2e` — those levels are what makes layer 3 applicable — and absent otherwise.

Runner rules:

- **Go**: `runner-command` defaults to `["go", "test", "-count=1", "-json", "./..."]`.
  `-count=1` is mandatory — a cached result did not execute during this run and cannot prove
  a tagged test ran. A multi-module repository needs one coverage source's runner block per
  affected `go.mod`; module boundaries are never guessed across them.
- **Maven / Gradle**: the command names the project's wrapper first (`./mvnw`, `./gradlew`),
  falling back to an installed toolchain only when the project explicitly uses one. Gradle's
  command must include `--rerun` (Gradle 7.6+) or `--rerun-tasks` — the verifier refuses a
  command missing both. `report-globs` is required and non-empty for both — freshness is
  proven by the report files the command produces, not by the exit code alone.

Example block, Go runner, no live profile:

```text
## Verification

- runner-kind: go
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-08-31-orders-baseline.json
- baseline-sha256: pending
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-08-31-orders-coverage.md"]
```

Changing runner kind, root, command, or report globs after approval is a coverage-source
amendment requiring user approval; when the change alters which tests run or how their
identities are emitted, baseline evidence must be regenerated at the same baseline commit
before final verification.

## Baseline capture

The `Baseline:` header line and the `## Verification` block's `baseline-sha256` key are
filled in **two commits**, because the `baseline` op requires the coverage source to already
be committed — it snapshots the repository, runs the suite in a disposable worktree at the
merge-base commit, and publishes evidence back into the working tree, all gated on a clean
tree — while the SHA-256 and counts it prints are themselves the op's output and so cannot
be known before that commit exists:

1. Compute `git merge-base HEAD <default-branch>` and write it on the `Baseline:` header
   line: `Baseline: <sha>`.
2. Write the `## Verification` block with `baseline-sha256: pending`.
3. **Commit the source.** The tree must be clean for step 4; this is the first commit, not
   the final one.
4. Run:

   ```text
   go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier baseline -request -
   ```

   with this JSON document on stdin:

   ```json
   {"schemaVersion":"1","repoRoot":".","coverageSource":"<repo-relative path>","runID":"<id>"}
   ```

   `baseline` refuses `dirty-repository` on an uncommitted tree — step 3 is the fix for that
   refusal, never a `--force`. On success its output carries `evidencePath`,
   `evidenceSha256`, and `counts{passed,failed,skipped}`.
5. Write the printed `evidenceSha256` onto `baseline-sha256` and the printed counts onto the
   `Baseline:` line (`Baseline: <sha> · <passed> passed, <failed> failed, <skipped> skipped`).
   List any failing test names under `## Baseline failures` (empty, with a line saying the
   suite was green, if `counts.failed` is 0).
6. **Commit again.** This second commit is the one that carries real evidence; the first
   exists only so `baseline` had a committed source to run against.

Verification compares the current run against this baseline, so a pre-existing failure is
not charged to this feature.

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

Amendments **append** rows with new IDs — allocated from `next-id`, the same as any new row
— and move superseded ones to `## Retired`, with user approval. Never renumber, never
silently edit a row an implementer already tagged against.

## Migration

A document whose scenario table still puts its mutation-naming column third and `Expected`
fifth — the pre-registry five-column order — is refused by the verifier's parser, which
names the old header it found in its error and reports that document as unparseable.
Migrating it:

1. **Rename and reorder the column.** Move the old third column — the one naming the
   mutation each row demonstrates — to fifth position and rename it `Verification
   mutation`; move `Expected` from fifth to third. The result is `| ID | Behavior |
   Expected | Level | Verification mutation | Risk |`, cell contents unchanged apart from
   the move — a row's mutation text does not need rewriting, only relocating.
2. **Add the `## Verification` block.** An old document captured its baseline by hand and
   carries no machine-readable runner configuration; add the block described
   [above](#verification-block), sourcing `runner-kind`, `runner-root`, and
   `runner-command` from however the project's suite is actually run today.
3. **Resolve a registry collision, if one exists.** The repository ID registry harvests IDs
   from both header forms so an unmigrated document's IDs stay visible to `next-id`, but two
   documents defining the same ID is a hard failure. If migration surfaces a pre-registry
   collision, renumber the newer document's rows once, in the same change as the rename,
   updating every `covers:` tag in its test source to match. This is the one exception to
   "never renumbered" — it applies only to a collision inherited from before the registry
   existed; every ID allocated through `next-id` from here on stays fixed.
4. **Prove acceptance.** Run:

   ```text
   go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier preflight -source <path> -repo .
   ```

   A successful `preflightOutput` — not a refusal — is the migration's done condition.

## Definition of done

- [ ] Starting ID allocated from `next-id` before any row was numbered; no row numbered
      from one
- [ ] Behavior list confirmed by the user before sweeping
- [ ] No row pruned or downgraded on the grounds that a test already exists
- [ ] Every behavior group has a `Ruled out:` line citing spec or code facts
- [ ] Axis 5 rule-outs name the absent shared resource
- [ ] Every `Verification mutation` is a mutation, not a negated `Expected`
- [ ] Risk derived by rule; every argue-down carries a recorded approval line
- [ ] Axis 5 rows `integration`+; axis 11 rows `component`+
- [ ] Harness hints on `integration`, `component`, `smoke`, `e2e` rows
- [ ] Invariants and out-of-scope both present
- [ ] `## Verification` block present, `live-profile` set when any row is `smoke`/`e2e`
- [ ] Baseline captured through the two-commit protocol; `Baseline:` line and
      `baseline-sha256` carry the printed sha and counts, not `pending`
- [ ] Artifact committed to the target repo, not the plugin repo

## Next stage

**REQUIRED SUB-SKILL:** `superpowers:writing-plans`. Tell it to cite SC-IDs in each task's
test steps, so every row maps to a task and an uncovered row is visible as a plan gap.
This opt-in also commits the feature to `coverage-verification` at the
`finishing-a-development-branch` handoff.

Hand it this lifecycle contract, verbatim, for the implementer of every task carrying an
SC-tagged test:

> Run the initial test against untouched production code. A failing result is TDD RED; a
> passing result is ALREADY-GREEN and returns to the parent for reconciliation. The SC row's
> verification mutation informs test design but is applied only by final coverage
> verification.

An implementation-time mutation may never be cited as TDD RED or as layer-4 evidence — only
the final mutator's `MUTATION-CAUGHT` result counts, and it runs independently of any
earlier experiment.

Also tell it to require, on every implemented test, a comment carrying one or more canonical
IDs after the literal text `covers:`, comma-separated for multiple IDs (`covers: SC-01`,
`covers: orders/SC-01, orders/SC-02`). Placement is exact, not "somewhere nearby":

- **Go** — immediately above the `TestXxx` function:

  ```go
  // covers: SC-01
  func TestRejectsZeroQuantity(t *testing.T) {
  ```

- **Java** — after all annotations, immediately above the method declaration:

  ```java
  @Test
  // covers: SC-01
  void rejectsZeroQuantity() {
  ```

- **Kotlin** — after all annotations, immediately above the function declaration:

  ```kotlin
  @Test
  // covers: SC-01
  fun rejectsZeroQuantity() {
  ```

The verifier binds a tag to its declaration structurally — Go's parser and AST for Go,
a token scanner for Java and Kotlin — never by searching for the tag's literal text, so the
placement rule above is exact, not advisory: a tag separated from its declaration by
executable syntax, or above an unsupported declaration form, does not bind.

A test may cite at most two active IDs without an `overclaim` finding. More than 2 IDs on
one test is an `overclaim` finding at verification: the test must then carry a distinct,
separately-nameable assertion per cited row, and layer 2 verdicts each row against its own.
Two is where that stops being routine — a row and its boundary case legitimately share one
test; a third row usually means the rows should have been merged in the source, or the test
split. `overclaim` blocks the branch while **unresolved**, and it is resolved either by
layer 2 verdicting every cited row against a named assertion, or by merging or splitting so
the tag drops to two. Do not read it as advisory.

Citing SC-IDs in the plan alone is not enough: `coverage-verification` layer 1 binds tags in
test source, not the plan, so an untagged test reads as a `gap` even when the plan cited the
row correctly.
