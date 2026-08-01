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
   normalized `status<TAB>scope<TAB>name` contract. Never let raw suite output land in this
   context — a failing suite in a large repo is tens of thousands of tokens.
5. **Join tags to runner results** on **both** the tag's file and its name, per
   [references/runner-output.md](references/runner-output.md) ("Joining a tag to its row").
   Never join on the name alone: names repeat across files, so a name-only join can bind a
   tag to a different file's passing result while its own test is skipped.
6. **Compare against the baseline** — the passed/failed/skipped counts and the
   `## Baseline failures` list recorded in the coverage doc header.
7. **Read the production diff for this branch.** Report any touched behavior with no SC row
   as `stale`.

Findings:

| Finding | Condition |
|---|---|
| `gap` | Row has no tag, **or** its tagged test is reported `skipped`, **or** the test is absent from the run entirely, **or** the join is ambiguous. Report the last as `gap (ambiguous)` with its candidates, per [references/runner-output.md](references/runner-output.md) — it means the tag could not be pinned to one test, which is a different repair from a missing one |
| `drift` | A tag cites an SC-ID that is neither active nor in `## Retired` |
| `overclaim` | One test carries more than 2 SC-IDs. Each cited row must then be verdicted separately by layer 2 against a named assertion |
| `stale` | The diff touches a behavior with no SC row — trigger to amend the doc |
| `red` | A test failing now that passed at baseline. Pre-existing failures are not this feature's `red` |

A tag above a skipped test is the cheapest possible false pass: it satisfies a grep and
leaves the suite green. Step 4 exists solely to close it. Never substitute grep plus exit
code.

## Layer 2 — adversarial read

Dispatch `coverage-critic`, one agent per behavior group, groups in parallel.

Give each: the group's rows, **each row's test source pasted inline** with its file path,
and **the production diff with test files excluded**. A row whose tagged test does not exist
is still included, with `no source found at <path>` in place of the paste — that is what
lets the critic return `MISSING` and lets the reconciliation below run at all.

Excluding test files from the diff is not tidiness. At the finishing-a-development-branch
seam the branch diff normally *contains the new tests*, so a diff that included them would
hand the critic the real source of every row alongside the pasted one — naming the
synthesized canary below by inspection, no disk read required. Layer 2's question is about
production code; test files in the diff add nothing and cost the canary.

Withholding the diff would leave the critic unable to answer
its own question, since "would this fail if the mutation were applied" is a claim about
production code. Its honesty comes from having no edit tools, not from having no
information.

**Inject one canary per batch:** a test you already know is weak. A batch that verdicts its
canary `HOLDS` has all its verdicts voided and re-dispatches at a higher effort tier.
Without this, an all-`HOLDS` critic passes forever after the one-time acceptance check.

Source the canary in this order:

1. **A naturally weak test** — one layer 1 flagged, or one a prior run verdicted `WEAK` or
   `VACUOUS` **on its real source**. Preferred: nothing is fabricated and the expected
   verdict is already known. A `WEAK`/`VACUOUS` earned by a weakened *copy* does not
   qualify: reusing it as a natural canary pastes the row's healthy real source, expects
   weakness, gets `HOLDS`, and voids the batch forever.
2. **A synthesized one.** Pick any row in the batch and paste, in place of its real test
   source, a copy weakened so it no longer binds to that row's `Bug it catches` — the
   binding assertion replaced with a truthiness or `toBeDefined` check, or dropped. The
   correct verdict for the paste is `WEAK` or `VACUOUS`; `HOLDS` voids the batch. That row
   has now been verdicted on a copy rather than on its real source, so re-dispatch **that
   row alone** with its real source to get its actual verdict. One extra single-row
   dispatch per batch, not a second full batch.

   Vary the weakening. `coverage-critic` is told to hunt for exactly the forms named above,
   so a batch in which one row and only one row asserts `toBeDefined()` identifies itself.
   Weaken by loosening a boundary, asserting a mock instead of state, dropping to a shape
   check — whatever the row's real assertion makes natural.

   If the row cannot be weakened without the copy still binding — a golden-file or
   snapshot test, where the binding is the fixture and not an inline assertion — pick a
   different row. If no row in the batch can be weakened, say so in the report and skip
   batch-voiding for that batch. That fallback is now the narrow case it was always meant
   to be, not the default.

Option 1 is unavailable on exactly the run where the guard matters most: a healthy first
run has no flagged test and no prior verdicts, so a rule that stops at option 1 leaves the
rubber-stamp critic unguarded in the common case. That is circular — the canary would
require the signal layer 2 exists to produce.

Two things make the synthetic canary work, and both are load-bearing:

- **Every row's source is pasted, not just the canary's.** A batch where one row arrives
  inline and the rest arrive as file paths announces which row is the canary.
- **The paste is authoritative** — `coverage-critic` verdicts the source it was given, not
  what it may find on disk. Stated in the agent, so re-reading the file gains nothing.

Never write the weakened copy to disk. Layer 2 has no tamper-check protocol, and a
synthetic test file in the working tree is indistinguishable from a real one to every later
layer.

**Every dispatch states whether it carries a canary.** The single-row re-dispatch above does
not, and cannot — weakening its only row is what it exists to undo. A canary-free dispatch is
legitimate exactly when it re-runs rows whose canary-bearing dispatch already caught its
canary; it inherits that batch's evidence and names it. Without this the rule recurses: the
re-dispatch needs a canary, synthesizing one needs another re-dispatch, forever. Say which
kind each dispatch is in the prompt, because `coverage-critic` is told a canary is present
and would otherwise conclude a healthy lone row must be the weak one.

The report names which row served as the canary for each batch, whether it was natural or
synthesized, and for a synthesized one, the weakening applied — a claimed catch with no
named row and no named weakening is not auditable.

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

Each walkthrough line names the SC-IDs it exercises, so a reader can tell which row a
`W2 dep down` line verdicts. A `smoke` or `e2e` row layer 1 already reported as `gap` has no
tagged test to bind a walkthrough result to; record it as `LIVE-N/A (layer 1 gap)` instead
of treating an unrelated walkthrough's result as that row's verdict.

## Layer 4 — mutation (`risk=high` rows only)

Dispatch `mutator`. Same tamper-check protocol as layer 3, before and after. Before
dispatch, the dispatcher also confirms the working tree is clean — `git status --porcelain`
empty apart from untracked files under `transcripts/`, which layer 3 already wrote — since
the mutator's own revert (`git checkout --`) cannot distinguish its mutation from
pre-existing uncommitted work.

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
`stale`, `red`, `WEAK`, `VACUOUS`, `MUTATION-SURVIVED`, live `FAIL`, live `INCONCLUSIVE`.

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

Every row verdict records which run **and which dispatch** produced it, so a report cannot
silently combine fresh subset results with stale inherited verdicts and present the mixture
as `PASS`. Dispatch granularity matters inside a single run: a canary'd row carries a verdict
from the weakened copy and another from the re-dispatch of its real source, and a reader who
cannot tell them apart cannot tell a planted weakness from a real finding. When a batch is
voided, the re-dispatch it spawned is voided with it — its critic was never canary-tested.

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
- [ ] Each canary-bearing layer-2 batch names its canary, natural or synthesized, and
      reports it caught — with the weakening named if it was synthesized. A canary-free
      dispatch names the canaried batch whose evidence it inherits
- [ ] Layer 3 transcripts re-grepped by this thread, not trusted from the summary
- [ ] Tamper check run before and after both layer 3 and layer 4
- [ ] Every `risk=high` row has a layer 4 verdict
- [ ] `PARTIAL` carries both the acknowledgement line and attempt evidence
- [ ] Report committed to the target repo

## Next stage

Verdict `PASS` or acknowledged `PARTIAL` → continue with
`superpowers:finishing-a-development-branch`.

Verdict `BLOCKED` → report the findings, fix, re-run. Do not finish the branch.
