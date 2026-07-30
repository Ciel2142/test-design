---
name: coverage-verification
description: Use at the finishing-a-development-branch handoff - verifies every coverage scenario has a test that actually bites, across trace, adversarial read, live execution, and mutation
---

# Coverage Verification

## Overview

Measure the implementation against the coverage doc across four layers. Each layer's
self-report is checked by something other than itself.

**Announce at start:** "I'm using the coverage-verification skill to verify coverage for
this feature."

**When:** at the `superpowers:finishing-a-development-branch` handoff. Not before
`requesting-code-review` — that skill runs *inside*
`superpowers:subagent-driven-development` as its final review, so no seam exists there.
Consequence: a `BLOCKED` verdict arrives after SDD's own reviews have run.

**Mode:** unattended, except that `PARTIAL` requires a human acknowledgement.

**Refuse to run** if the coverage doc is missing. There is no yardstick, and a report with
no yardstick is worse than no report.

## Layer 1 — trace (main thread)

1. **Parse the coverage doc.** Collect active SC-IDs, retired SC-IDs, and each row's
   `Level` and `Risk`.
2. **Grep the tags,** scoped to the affected package:
   `grep -rn "covers: SC-" <test-dir>`
3. **Resolve each tag to a test name** using the language patterns in
   [references/runner-output.md](references/runner-output.md).
4. **Run the suite with machine-readable output, redirected to a file.** Parse only the
   normalized `status<TAB>name` contract. Never let raw suite output land in this context —
   a failing suite in a large repo is tens of thousands of tokens.
5. **Join tags to runner results** by suffix match on the test name.
6. **Compare against the baseline** suite result recorded in the coverage doc header.

Findings:

| Finding | Condition |
|---|---|
| `gap` | Row has no tag, **or** its tagged test is reported `skipped`, **or** the test is absent from the run entirely |
| `drift` | A tag cites an SC-ID that is neither active nor in `## Retired` |
| `overclaim` | One test carries more than 2 SC-IDs. Each cited row must then be verdicted separately by layer 2 against a named assertion |
| `stale` | The diff touches a behavior with no SC row — trigger to amend the doc |
| `red` | A test failing now that passed at baseline. Pre-existing failures are not this feature's `red` |

A tag above a skipped test is the cheapest possible false pass: it satisfies a grep and
leaves the suite green. Step 4 exists solely to close it. Never substitute grep plus exit
code.

## Layer 2 — adversarial read

Dispatch `coverage-critic`, one agent per behavior group, groups in parallel.

Give each: the group's rows, the tagged test files, and **the production diff**. Withholding
the diff would leave the critic unable to answer its own question, since "would this fail if
the mutation were applied" is a claim about production code. Its honesty comes from having
no edit tools, not from having no information.

**Inject one canary per batch:** a test you already know is weak. A batch that verdicts its
canary `HOLDS` has all its verdicts voided and re-dispatches at a higher effort tier.
Without this, an all-`HOLDS` critic passes forever after the one-time acceptance check.

The canary must be drawn from a test already known weak for this run — one layer 1 flagged,
or one a prior run verdicted `WEAK` or `VACUOUS`. If no such test exists for this batch, say
so in the report and skip the batch-voiding rule for that batch rather than manufacturing an
unrelated canary.

Verdicts: `HOLDS`, `WEAK`, `VACUOUS`. `WEAK` and `VACUOUS` are blocking.

A critic may also return `SC-nn MISSING` when a row's tagged test does not exist. That is
not a fourth verdict — fold it into layer 1's `gap` set and reconcile: if layer 1 did not
already report that row as a `gap`, the tag resolution or the name join is broken, and layer
1 must be re-run before the report is trusted.

## Layer 3 — live

Dispatch `live-runner` for the `smoke` and `e2e` rows plus derived walkthroughs.

**Before dispatch,** record `git rev-parse HEAD`, `git stash list`, and a hash of
`git diff`. **After it returns,** compare all three. `git status --porcelain` alone would
miss a runner that commits, stashes, or edits and reverts.

**Then re-grep the transcripts yourself** for the `EXPECT:` and `RESULT:` tokens the agent
declared. The agent's summary is a claim;
`superpowers:verification-before-completion` names trusting agent success reports as a red
flag, and a self-graded `W2 PASS` is exactly that. Re-greping costs almost nothing and makes
the summary auditable.

No `smoke` or `e2e` rows → layer 3 is `N/A`, not `SKIPPED`, not `PARTIAL`.

A single-iteration concurrency walkthrough is `INCONCLUSIVE`, never `PASS`.

## Layer 4 — mutation (`risk=high` rows only)

Dispatch `mutator`. Same tamper-check protocol as layer 3, before and after.

Verdicts: `MUTATION-CAUGHT`, `MUTATION-SURVIVED` (blocking), `MUTATION-UNAPPLIABLE`.

`MUTATION-UNAPPLIABLE` is a coverage-doc defect — the row's `Bug it catches` was not
phrased as a code-level mutation. Report it as such; do not let the mutator invent a
substitute.

A row layer 1 already reported as `gap` has no executable test to mutate, so it is not
dispatched to the mutator at all — record it as `MUTATION-N/A (layer 1 gap)` instead. This
is not `MUTATION-UNAPPLIABLE`, which means only that the row's `Bug it catches` itself was
mis-phrased, not that the tagged test is missing or skipped.

## Verdicts

Exactly three.

**`BLOCKED`** on any of: `gap`, unretired `drift`, unresolved `overclaim`, unresolved
`stale`, `red`, `WEAK`, `VACUOUS`, `MUTATION-SURVIVED`, live `FAIL`.

**`PARTIAL`** when a layer could not run. Requires **both**:

- a human acknowledgement line, obtained on an actual user turn — the run stops and asks,
  then records the user's reply verbatim as `PARTIAL accepted by <user> — <reason>`. An
  agent-composed line does not count.
- attempt evidence in the transcript directory: commands tried, exit codes, files sought.
  The commands tried must exercise the project's own documented run recipe (README,
  Makefile, compose file, documented start command) — the same rule `live-runner` already
  applies to bring-up, not one deliberately-doomed command.

Without both, the expensive layer is opt-out by unverified declaration — and layer 3 is
where that pressure lands.

**`PASS`** only when every layer either passed or is `N/A`.

## Re-run scope

Layer 1 **always** re-runs in full. It is cheap, and a fix that repairs two rows can break
neighbors. Layers 2, 3, and 4 may be scoped to an ID subset.

Every row verdict records which run produced it, so a report cannot silently combine fresh
subset results with stale inherited verdicts and present the mixture as `PASS`.

## Degradation

| Situation | Behavior |
|---|---|
| No `smoke`/`e2e` rows | Layer 3 `N/A` |
| No run recipe, or dependencies unavailable | Layer 3 `SKIPPED` with attempt evidence → `PARTIAL` |
| Suite exists but cannot execute here | Layers 1 and 4 trace-only → `PARTIAL` with attempt evidence |
| No test suite at all (legacy repo) | Scope layer 1's gap set to rows whose behavior the diff touches. Do not hard-block on rows the feature never went near |
| Pure refactor | Doc is the invariant set plus named characterization tests; layers 2–4 `N/A`; layer 1 requires zero new failures against baseline |
| Coverage doc missing | Refuse to run |

## Artifact

`<repo>/docs/superpowers/coverage/YYYY-MM-DD-<feature>-verification.md`, transcripts under
`<repo>/docs/superpowers/coverage/transcripts/`.

Format: [references/example-verification.md](references/example-verification.md).

## Definition of done

- [ ] Layer 1 ran in full, consuming machine-readable runner output
- [ ] Every active row has a verdict from every applicable layer
- [ ] Each layer-2 batch's canary was caught
- [ ] Layer 3 transcripts re-grepped by this thread, not trusted from the summary
- [ ] Tamper check run before and after both layer 3 and layer 4
- [ ] Every `risk=high` row has a layer 4 verdict
- [ ] `PARTIAL` carries both the acknowledgement line and attempt evidence
- [ ] Report committed to the target repo

## Next stage

Verdict `PASS` or acknowledged `PARTIAL` → continue with
`superpowers:finishing-a-development-branch`.

Verdict `BLOCKED` → report the findings, fix, re-run. Do not finish the branch.
