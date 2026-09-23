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

Before any `go run -C`, capture the **absolute target repository** while still in
that repository, not in the plugin checkout:

```bash
TARGET_REPO=$(git rev-parse --show-toplevel)
```

Use that captured absolute value for every `repoRoot` and `-repo`. Source paths remain
relative to that target root; `-C` changes the process directory to the plugin and must
never make the plugin the verification target.

## Floor rule — when to skip this stage

If the feature has **≤2 behaviors and no persistence, no concurrency, and no external
dependency**, do not produce a separate document. Append a `## Test coverage` section to
the approved spec containing the scenario table, the `## Verification` block, the
`Baseline:` header line, a `## Live decision` section, and a `## Baseline failures`
section; present that amendment for approval, then use the planning handoff below. The
section follows the same schema, ID allocation, live decision, and two-commit baseline
protocol as a standalone coverage source — the floor rule only changes where it lives,
never what it must contain. A floor-rule feature has no external dependency, so its live
decision is normally `not-applicable` with its reason stated; it is still written down,
because `preflight` refuses a source without one as `live-decision-incomplete`.

Below that size the rows are the plan's test steps restated, and a separate doc costs two
human gates to re-derive them. Say which rule applied: "Floor rule applies — 2 behaviors,
no persistence. Appending the coverage source to the spec instead."

## Workflow

1. **Allocate the starting ID.** Before sweeping, run

   ```text
   go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier next-id -repo "$TARGET_REPO" -area <area>
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
5. **Derive risk** per [references/risk-derivation.md](references/risk-derivation.md), from
   the consequence of the failure and never from the subject matter; argue down only with
   recorded user approval.
6. **Assign level** per [references/level-assignment.md](references/level-assignment.md).
   Cheapest level that catches the mutation; axis 5 rows are `integration`+, axis 11 rows
   are `component`+.
7. **Add harness hints** for `integration`, `component`, `smoke`, and `e2e` rows.
8. **Record ruled-out axes** per behavior, each citing a spec fact or a code fact.
9. **Extract invariants** — properties holding across all scenarios, `INV-n`.
10. **List out-of-scope** items, each with a reason.
11. **Author the live decision** (see [below](#live-decision)). `not-applicable` states the
    reason layer 3 does not apply; `required` carries a journeys table whose rows name active
    IDs and a result observable on the deployed revision. Every active `smoke` or `e2e` row
    is a live obligation and appears in at least one journey.
12. **Get an independent scenario review** (see [below](#independent-scenario-review)):
    dispatch `scenario-reviewer` in `review` mode, resolve every finding it returns, then
    recheck exactly once. Nothing reaches the user before this.
13. **Write the `## Verification` block** (see [below](#verification-block)) naming the
    runner, its command, report globs, the baseline evidence path, and — when the live
    decision is `required` — the live profile.
14. **Capture the baseline**, in two commits (see [below](#baseline-capture)).
15. Present to the user: the cost summary line, then every finding still `OPEN` or
    `NEW GAP` with its consequence. Get approval, write and commit.

## Existing tests are candidates, not proof

Read the implementation, the tests, the fixtures and the test infrastructure while you draft.
Knowing what a test asserts and what a harness can already reach makes rows sharper and
harness hints real, and the scenario reviewer grounds its own findings in that same tree.

What the reading may not do is decide the table. Rows come from the approved requirements: a
behavior the requirements owe gets a row whether or not something that looks like it already
exists. No row is pruned, merged away, or downgraded on the grounds that a test exists — at
design time nobody can tell by reading whether that test would catch this row's
`Verification mutation`, and the cost of guessing wrong is asymmetric: the row is gone, its
ID was never issued, and no later layer can report on a row that does not exist. A redundant
row, the other way around, is one merge during pruning or one `## Retired` line at amendment.

An existing test earns credit exactly as a new one does, and only that way: it carries a
`covers:` tag, it executes in the verification run, and layer 2's critic verdicts it against
the row it claims. Tagging an already-written test is the whole cost of crediting it — no new
test is needed — and the judgment then happens with the code in front of the critic and a
mutation to apply, which is when the question is actually answerable.

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
`.test-design/live-profiles/<name>.json`, repo-relative. It is required whenever the
[live decision](#live-decision) is `required` — that decision is what makes layer 3
applicable, and layer 3 needs an approved profile to dispatch against — and absent when the
decision is `not-applicable`. Without it, `preflight` refuses `target-missing`.

Runner rules:

- **Go**: `runner-command` defaults to `["go", "test", "-count=1", "-json", "./..."]`.
  `-count=1` is mandatory — a cached result did not execute during this run and cannot prove
  a tagged test ran. A multi-module repository needs one coverage source's runner block per
  affected `go.mod`; module boundaries are never guessed across them. Add the single token
  `-race` when an active row's mutation is shared memory read or written from more than one
  goroutine — it needs cgo on the build machine, and once present it carries unchanged into
  layer 4's mutation run, so the baseline must be captured with that same command. Write a
  timeout as one token (`-timeout=30m`), never as two: two non-flag tokens make the package
  pattern ambiguous.
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

## Live decision

One section states whether layer 3 applies to this feature and what it runs. The verifier
parses it, and `preflight` refuses `live-decision-incomplete` when it is absent or
incomplete, so this is a schema, not prose:

```text
## Live decision

- decision: required            (or: not-applicable)
- reason: <text>                (mandatory for not-applicable; optional otherwise)

| Journey | Scenarios | Expected |
|---|---|---|
| J-1 | SC-10, SC-11 | <observable result on the deployed selected revision> |
```

- `decision` takes exactly `required` or `not-applicable`.
- `reason` is mandatory for `not-applicable` — say what about this feature puts nothing on a
  deployed surface — and optional for `required`.
- The journeys table belongs to a `required` decision and must be absent from a
  `not-applicable` one.
- `Journey` is `J-n`. `Scenarios` is a comma-separated list of active IDs from this document.
  `Expected` names a result observable on the deployed selected revision — a response, a
  stored record, a delivered message — never internal state nobody can see from outside.
- Non-bullet, non-table prose inside the section is ignored by the parser. The table header
  must read exactly `Journey | Scenarios | Expected`; any other header is ignored, and a
  table carrying only a header row counts as no table at all. Both read as `journeys-missing`
  rather than as a pointer at the malformed table.

`preflight` names exactly one code for the first fault it finds:

| Code | Cause |
|---|---|
| `missing` | no `## Live decision` section |
| `reason-missing` | `not-applicable` without a reason |
| `contradiction` | `not-applicable` while an active row is `smoke` or `e2e`, or `not-applicable` with a journeys table |
| `journeys-missing` | `required` with no usable journeys table |
| `journey-unknown-row` | a journey cites an ID that is not an active row of this document |
| `live-row-unjourneyed` | an active `smoke` or `e2e` row appears in no journey |
| `target-missing` | `required` while `## Verification` carries no `live-profile` |

A `smoke` or `e2e` row is a live obligation for as long as it is active: no journey may leave
one out, and a decision of `not-applicable` cannot stand next to one. Journeys may also name
`unit` or `integration` rows they happen to exercise on the way through — that costs the row
nothing and does not relabel it.

## Independent scenario review

The author of a table is its worst judge. Before the user sees the draft, hand it to
[`scenario-reviewer`](../../agents/scenario-reviewer.md), which reads the repository but
holds no edit tool: it returns findings and never touches the table. A reviewer that can
repair a row will repair it and then pass it, which destroys the signal.

Dispatch it with four things — the approved requirements, the draft coverage source, the
repository root, and `mode: review`. It answers with one block per finding — `MERGE`, `DROP`,
`REWRITE`, `ADD`, `RISK <id> <from>→<to>`, `LIVE`, `ESCALATE` — and one closing line,
measured on the draft as handed over, before any finding is applied:

```text
COUNTS scenarios=<n> high=<n> journeys=<n>
```

Resolve every finding yourself. Each one ends in exactly one of three states: **applied**,
**rejected** with the reason it does not hold, or **escalated** to the user with the concrete
consequence of settling it the other way. Only a factual or product-scope disagreement is
escalated; ordinary cleanup is applied or rejected.

Then dispatch it once more with the amended draft and its own previous findings, and
`mode: recheck`. It answers each previous finding `RESOLVED`, `OPEN`, or `NEW GAP`, and
reviews the amended rows. A finding you rejected comes back `OPEN` — the recheck reads the
amended draft and its own previous findings, never your reasons — so expect to carry it, with
your reason and its concrete consequence, to the user. **There is exactly one recheck.**
Anything still `OPEN` or `NEW GAP` after it goes to the user at approval, each with its
concrete consequence; the user decides, and there is no further round.

Record the round in a `## Review record` section of the coverage source:

- the reviewer instruction revision — the commit SHA of `agents/scenario-reviewer.md` and its
  `git hash-object agents/scenario-reviewer.md` digest — so a later reader knows which
  instructions produced these findings;
- the findings as returned;
- the resolution of each: applied, rejected with the reason, or escalated.

Nothing else. No scenario-signature table: the record says what was reviewed and what came of
it, not a digest of every row.

At approval, show the user what the table commits them to, mirroring the reviewer's counts:

```text
Cost: <n> surviving scenarios, <n> high-risk obligations, <n> live journeys
```

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
   {"schemaVersion":"1","repoRoot":"<captured absolute TARGET_REPO>","coverageSource":"<repo-relative path>","runID":"<id>"}
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

The smallest set of rows naming distinct mutations. A typical feature lands at 8–25 rows
**after pruning**.

More than forty surviving rows triggers a focused redundancy and scope review: look hardest
at near-duplicate rows, and at rows tracing to no approved requirement. Forty is neither a
cap nor a target — a feature that genuinely owes fifty distinct behaviors gets fifty rows —
and splitting the table across two documents reduces no cost, it only hides it.

## Amendment

Implementation invalidates pre-implementation docs routinely: a new failure mode surfaces,
an error code changes, a sub-feature is dropped. Without an amend path, verification
measures the code against a stale yardstick and passes.

Amendments **append** rows with new IDs — allocated from `next-id`, the same as any new row
— and move superseded ones to `## Retired`, with user approval. Never renumber, never
silently edit a row an implementer already tagged against.

A `MUTATION-SURVIVED` site names a defect the current suite cannot catch: repair it as an
amendment, not a comment. Broaden the row's `Verification mutation` to name the surviving
site, or append a new row carrying it, then re-verify. Explaining the survivor away in prose
is never the repair.

## Migration — only for an old source

If the parser identifies an old header or registry collision, **read
[migration](references/migration.md) in full before editing that source**. A new source
does not need migration context.

## Definition of done

- [ ] Starting ID allocated from `next-id` before any row was numbered; start at one
      only when the allocator returns `next: 1`
- [ ] Behavior list confirmed by the user before sweeping
- [ ] No row pruned or downgraded on the grounds that a test already exists
- [ ] Every behavior group has a `Ruled out:` line citing spec or code facts
- [ ] Axis 5 rule-outs name the absent shared resource
- [ ] Every `Verification mutation` is a mutation, not a negated `Expected`
- [ ] Risk derived from consequence; every `high` outside the four always-`high`
      consequences carries its `Risk rationale:` line; every argue-down carries a recorded
      approval line
- [ ] Axis 5 rows `integration`+; axis 11 rows `component`+
- [ ] `-race` present in `runner-command` when an active row's mutation is a shared-memory
      race across goroutines
- [ ] Harness hints on `integration`, `component`, `smoke`, `e2e` rows
- [ ] Invariants and out-of-scope both present
- [ ] `## Live decision` complete: `not-applicable` carries its reason; `required` carries
      journeys naming active IDs, observable results, and every active `smoke`/`e2e` row
- [ ] `scenario-reviewer` dispatched in `review` mode and rechecked exactly once
- [ ] `## Review record` present: instruction revision, findings, one resolution each
- [ ] Cost summary line shown to the user, with any finding still `OPEN` or `NEW GAP`
- [ ] `## Verification` block present, `live-profile` set when the live decision is
      `required`
- [ ] Baseline captured through the two-commit protocol; `Baseline:` line and
      `baseline-sha256` carry the printed sha and counts, not `pending`
- [ ] Artifact committed to the target repo, not the plugin repo

## Next stage

If the user's installed workflow requires wave planning, hand off first to
`wave-planning`; otherwise proceed directly to `superpowers:writing-plans`. This plugin
does not install or mandate a wave-planning dependency. In either path, require the
eventual plan to cite SC-IDs in each task's test steps, so every row maps to a task and
an uncovered row is visible as a plan gap. This opt-in also commits the feature to
`coverage-verification` at the `finishing-a-development-branch` handoff.

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

A test may cite any number of active IDs; the count alone is never a finding. What each cited
row needs is its own identifiable assertion inside that test — layer 2's critic verdicts
every cited row separately, against the assertion that claims it, so a row resting on nothing
nameable fails there rather than at the tag.

Credit runs the other way too: several tests may contribute to one row, and every tagged site
that binds to it is one of its contributors. Each contributor carries its own execution
evidence. A contributor that was skipped, or whose test never ran, is that row's `gap`; a
contributor that failed is that row's `red` — whatever the row's other contributors did.

Citing SC-IDs in the plan alone is not enough: `coverage-verification` layer 1 binds tags in
test source, not the plan, so an untagged test reads as a `gap` even when the plan cited the
row correctly.
