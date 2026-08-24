---
name: coverage-verification
description: Use only at the finishing-a-development-branch handoff when an explicit test-coverage-design opt-in produced approved SC rows
---

# Coverage Verification

## Overview

Measure the implementation against its approved coverage source across four layers. The
source is either a dedicated coverage doc or an inline SC table in the approved spec.
Each layer's self-report is checked by something other than itself.

**Announce at start:** "I'm using the coverage-verification skill to verify coverage for
this feature."

**When:** only at the `superpowers:finishing-a-development-branch` handoff for a feature
whose explicit `test-coverage-design` invocation produced an approved coverage source. Not
before `requesting-code-review` — that skill runs *inside*
`superpowers:subagent-driven-development` as its final review, so no seam exists there.
Consequence: a `BLOCKED` verdict arrives after SDD's own reviews have run.

**Mode:** approval-gated, then unattended. `PARTIAL` additionally requires a human
acknowledgement.

**Do not run** if the feature has no approved coverage source. Absence means the feature
did not opt in; leave the normal finishing flow untouched. Check the source after the gate
below, as the first act of layer 1. There is no yardstick without it, and a report with no
yardstick is worse than no report.

## Approval gate

**Stop before doing anything.** Announce, say what the run will cost, and wait for the user
to reply on an actual user turn. No coverage-source read, no tag grep, no suite run, no
dispatch before that reply. Every invocation carries the gate, including a re-run after a
`BLOCKED` fix.

The gate exists because of where this skill fires. At the
`superpowers:finishing-a-development-branch` seam the thread has just carried an entire
implementation, so verification starts with a context window that is already nearly full —
and layer 2 pastes whole test bodies into that same thread, one row at a time. An auto-start
spends what is left of the window on a run the user may have wanted in a fresh session, and
a run that exhausts the window mid-flight leaves evidence that is partial and unreportable:
the layers that did complete are not a verdict, and nothing about the ones that did not is
recoverable without re-running them.

State in the request: the branch, the coverage source path this run expects, and that layers
2–4 dispatch subagents. Do not state a row count — the source has not been read, and a number
invented to make the request concrete claims work that has not happened.

Anything other than a go-ahead ends the run. Silence, a topic change, and an unrelated
instruction are not approval.

The report records the approval verbatim in its header, on the same footing as a `PARTIAL`
acknowledgement — a run whose header cannot say who let it start is not auditable as
gated.

## Layer 1 — trace (main thread)

1. **Parse the coverage source.** Collect active SC-IDs, retired SC-IDs, and each row's
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
   `## Baseline failures` list recorded in the coverage source header.
7. **Read the production diff for this branch.** Report any touched behavior with no SC row
   as `stale`.

Findings:

| Finding | Condition |
|---|---|
| `gap` | Row has no tag, **or** its tagged test is reported `skipped`, **or** the test is absent from the run entirely, **or** the join is ambiguous. Report the last as `gap (ambiguous)` with its candidates, per [references/runner-output.md](references/runner-output.md) — it means the tag could not be pinned to one test, which is a different repair from a missing one |
| `drift` | A tag cites an SC-ID that is neither active nor in `## Retired` |
| `overclaim` | One test carries more than 2 SC-IDs. Each cited row must then be verdicted separately by layer 2 against a named assertion |
| `stale` | The diff touches a behavior with no SC row — trigger to amend the coverage source |
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

**Read each paste at the `file:name` binding layer 1 already resolved** — not from a fresh
grep for the test name. Names repeat across files; the scope-restricted join in layer 1 is
what settles which one a tag means, and re-looking it up here throws that away and can hand
the critic a different file's test under this row's ID. A row layer 1 reported as
`gap (ambiguous)` has no resolved binding and so has no paste: it is already blocking, and
arrives as `no source found at <path>` like any other missing row.

**Paste it verbatim and complete** — the whole test body, every assertion, unedited. Step 4's
"never let raw suite output land in this context" is about a runner's failure output, not
about the tests under review, and does not license trimming a test to fit. A dispatcher that
drops the binding assertion earns a `VACUOUS` on a healthy row, and because the paste is
authoritative the critic cannot notice.

When a test genuinely will not fit, elide **only** code outside the test body — shared
`beforeEach`, fixture builders, imports — and name each cut inline where it was made:
`// elided: beforeEach at orders.test.ts:12-40`. Never elide an assertion, nor an arrange
step the assertions read from. If the body alone still will not fit, dispatch that row by
itself rather than trimming it.

A critic reason naming a truncated or unmarked-incomplete paste is a **dispatch defect**:
re-paste that row complete and re-dispatch it, and do not record the verdict it came with.

Excluding test files from the diff is not tidiness. At the finishing-a-development-branch
seam the branch diff normally *contains the new tests*, so a diff that included them would
hand the critic the real source of every row alongside the pasted one — naming the
synthesized canary below by inspection, no disk read required. Layer 2's question is about
production code; test files in the diff add nothing and cost the canary.

Withholding the diff would leave the critic unable to answer
its own question, since "would this fail if the mutation were applied" is a claim about
production code. Its honesty comes from having no edit tools, not from having no
information.

**Inject one canary per batch:** a test you already know is weak. A batch that verdicts
**the** canary `HOLDS` has all its verdicts voided and re-dispatches at a higher effort tier.
Without this, an all-`HOLDS` critic passes forever after the one-time acceptance check.

A critic that reads nothing and sacrifices a row escapes when it lands on the canary — ~1/N
per dispatch attempt, N the batch's row count (coin flip at N=2). Voided batches retry at a
higher tier, compounding against a persistent critic. The second canary bought 1/C(N,2) at one
extra dispatch per batch and the rules needed to keep two canaries from identifying each other.

Source each canary in this order:

1. **A naturally weak test** — one layer 1 flagged, or one a prior run verdicted `WEAK` or
   `VACUOUS` **on its real source**. Preferred: nothing is fabricated and the expected
   verdict is already known. A `WEAK`/`VACUOUS` earned by a weakened *copy* does not
   qualify: reusing it as a natural canary pastes the row's healthy real source, expects
   weakness, gets `HOLDS`, and voids the batch forever.
2. **A synthesized one.** Pick a row and paste, in place of its real test source, a copy
   weakened so it no longer binds to that row's `Bug it catches` — the binding assertion
   replaced with a truthiness or `toBeDefined` check, or dropped. The correct verdict for
   the paste is `WEAK` or `VACUOUS`; `HOLDS` voids the batch. That row has now been
   verdicted on a copy rather than on its real source, so re-dispatch **that row alone**
   with its real source to get its actual verdict. One extra single-row dispatch per
   *synthesized* canary — at most one per batch, none when the canary is natural, which is
   verdicted on its real source already. Never a second full batch.

   Vary the weakening. `coverage-critic` is told to hunt for exactly the forms named above,
   so a batch in which one row and only one row asserts `toBeDefined()` identifies itself.
   Weaken by loosening a boundary, asserting a mock instead of state, dropping to a shape
   check — whatever the row's real assertion makes natural.

   If a row cannot be weakened without the copy still binding — a golden-file or snapshot
   test, where the binding is the fixture and not an inline assertion — pick a different
   row. If none can, say so in the report and skip batch-voiding for that batch. That
   fallback is the narrow case it was always meant to be, not the default.

Option 1 is unavailable on exactly the run where the guard matters most: a healthy first
run has no flagged test and no prior verdicts, so a rule that stops at option 1 leaves the
rubber-stamp critic unguarded in the common case. That is circular — the canary would
require the signal layer 2 exists to produce.

Two properties of the dispatch make a synthetic canary work, and both are load-bearing:

- **Every row's source is pasted, not just the canary's.** A batch where the canary row
  arrives inline and the rest arrive as file paths announces which row is the canary.
- **The paste is authoritative** — `coverage-critic` verdicts the source it was given, not
  what it may find on disk. Stated in the agent, so re-reading the file gains nothing.

Never write the weakened copy to disk. Layer 2 has no tamper-check protocol, and a
synthetic test file in the working tree is indistinguishable from a real one to every later
layer.

**Every dispatch states whether it carries a canary.** The single-row re-dispatches above
carry none, and cannot — weakening their only row is what they exist to undo. A canary-free
dispatch is legitimate exactly when it re-runs rows whose canary-bearing dispatch already
caught its canary; it inherits that batch's evidence and names it. Without this the rule
recurses: the re-dispatch needs a canary, synthesizing one needs another re-dispatch, forever.
Say which kind each dispatch is in the prompt, because `coverage-critic` is told a canary is
present and would otherwise conclude a healthy lone row must be the weak one.

The report names the row that served as the canary for each batch, whether it was natural or
synthesized, and for a synthesized one, the weakening applied — a claimed catch with no
named row and no named weakening is not auditable. It also names any batch that carried
none, and why.

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

`MUTATION-UNAPPLIABLE` is a coverage-design defect — the row's `Bug it catches` was not
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
voided, the re-dispatches it spawned are voided with it — their critic was never
canary-tested.

## Degradation

| Situation | Behavior |
|---|---|
| No `smoke`/`e2e` rows | Layer 3 `N/A` |
| No run recipe, or dependencies unavailable | Layer 3 `SKIPPED` with attempt evidence → `PARTIAL` |
| Suite exists but cannot execute here | Layers 1 and 4 trace-only → `PARTIAL` with attempt evidence |
| No test suite at all (legacy repo) | Scope layer 1's gap set to rows whose behavior the diff touches. Do not hard-block on rows the feature never went near |
| Pure refactor | The source is the invariant set plus named characterization tests; layers 2–4 `N/A`; layer 1 requires zero new failures against baseline |
| Coverage source missing | Do not invoke; the feature did not opt in |

## Artifact

`<repo>/docs/superpowers/coverage/YYYY-MM-DD-<feature>-verification.md`, transcripts under
`<repo>/docs/superpowers/coverage/transcripts/`.

Format: [references/example-verification.md](references/example-verification.md).

## Definition of done

- [ ] Approval obtained on a user turn before any layer ran, recorded verbatim in the
      report header
- [ ] Layer 1 ran in full, consuming machine-readable runner output
- [ ] Every active row has a verdict from every applicable layer
- [ ] Each canary-bearing layer-2 batch names its canary, natural or synthesized, and
      reports it caught — with the weakening named if synthesized, and the reason stated if
      the batch carried none. A canary-free dispatch names the canaried batch whose evidence
      it inherits
- [ ] Every layer-2 paste was taken from layer 1's resolved `file:name` binding and pasted
      whole, with any elision outside the test body named inline
- [ ] Layer 3 transcripts re-grepped by this thread, not trusted from the summary
- [ ] Tamper check run before and after both layer 3 and layer 4
- [ ] Every `risk=high` row has a layer 4 verdict
- [ ] `PARTIAL` carries both the acknowledgement line and attempt evidence
- [ ] Report committed to the target repo

## Next stage

Verdict `PASS` or acknowledged `PARTIAL` → continue with
`superpowers:finishing-a-development-branch`.

Verdict `BLOCKED` → report the findings, fix, re-run. Do not finish the branch.
