---
name: coverage-verification
disable-model-invocation: true
description: Use only at the finishing-a-development-branch handoff when an explicit test-coverage-design opt-in produced approved SC rows
---

# Coverage Verification

## Overview

Measure the implementation against its approved coverage source across four layers. The
source is either a dedicated coverage doc or an inline SC table in the approved spec — the
document `test-coverage-design` produced, whose IDs were allocated by
`test-design-verifier next-id` and whose baseline evidence was captured by
`test-design-verifier baseline`. Each layer's self-report is checked by something other
than itself.

Every mechanical act of this run belongs to the verifier executable: it takes the
repository snapshot, creates and disposes the worktrees, runs the suite, binds `covers:`
tags to declarations, executes the live plan, applies and evaluates mutations, and computes
the verdict. This thread's work is the judgment the executable cannot do — classifying the
production diff for freshness, composing layer-2 batches and their canaries, confirming
that a mutation diff is the mutation the row named, and carrying the three agent dispatches.
Where this skill says "read", it means read a field of an op's output; it never means
re-deriving a result the executable already produced.

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
did not opt in; leave the normal finishing flow untouched. There is no yardstick without it,
and a report with no yardstick is worse than no report.

## Prerequisites and invocation

This skill is invoked explicitly and only explicitly:

```text
/test-design:coverage-verification <coverage-source>
```

`<coverage-source>` is the repo-relative path to the approved document. The frontmatter sets
`disable-model-invocation: true`, so nothing else can start this run.

Every op runs through the plugin's Go executable:

```text
go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier <op> -request -
```

with the request document on stdin. A Go 1.26+ toolchain on the machine running the skill is
a prerequisite; the module has no third-party dependencies, so there is no build step and no
committed binary.

Seven ops take that shared request body:

```json
{"schemaVersion": "1", "repoRoot": ".", "coverageSource": "<repo-relative path>", "runID": "<run id>"}
```

`schemaVersion` is `"1"`; any other value is refused `unsupported-schema-version` before the
op does anything. Per-op extra fields are named at each layer below. `preflight` and
`next-id` are the two ops driven by command-line flags rather than a request body.

Success is one JSON document on stdout. A refusal is `{"code": "...", "message": "..."}` on
stderr with exit status 1. Read the code and act on it per [Refusals](#refusals); never
re-run an op past a refusal you have not classified.

## Approval gate

**Stop before doing anything else.** `preflight` is the only op allowed to run before
approval. It parses the source, validates the runner block, and counts the rows each layer
will cover; it opens no worktree, runs no suite, and dispatches nothing:

```text
go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier preflight -source <coverage-source> -repo <repo-root>
```

If `preflight` refuses, the gate is not presented at all. Report the refusal verbatim — its
code and its message — and stop. There is nothing to approve, because nothing has been
validated; `parse-source-failed` in particular means the document is not a coverage source
this verifier can measure against.

On success, state in the approval request — every value read from `preflightOutput`, none
invented:

- `source` and `sourceSha256`, the document and revision this run measures against;
- the branch the source repository is on;
- `runner.kind`, `runner.root`, `runner.command`, and `runner.reportGlobs`;
- `layers.layer1` through `layers.layer4`, the row count each layer will cover;
- `highRiskCount`, the rows layer 4 will mutate;
- `liveProfile`, when `liveProfileRequired` is true;
- that layers 2, 3, and 4 dispatch subagents — `coverage-critic`, `live-runner`, `mutator`.

Then wait for the user to reply on an actual user turn. Every invocation carries the gate,
including a re-run after a `BLOCKED` fix.

The gate exists because of where this skill fires. At the
`superpowers:finishing-a-development-branch` seam the thread has just carried an entire
implementation, so verification starts with a context window that is already nearly full —
and layer 2 pastes whole test bodies into that same thread, one row at a time. An auto-start
spends what is left of the window on a run the user may have wanted in a fresh session, and
a run that exhausts the window mid-flight leaves evidence that is partial and unreportable:
the layers that did complete are not a verdict, and nothing about the ones that did not is
recoverable without re-running them.

Anything other than a go-ahead ends the run. Silence, a topic change, and an unrelated
instruction are not approval.

Record the go-ahead **verbatim**. It becomes the `approval` field of the finalize request —
`finalize` refuses `missing-approval` without it — and it is printed in the published
report's header. A run whose header cannot say who let it start is not auditable as gated.

## Run identity

Mint one run ID at approval, in the form `run-YYYY-MM-DD-nnnn`, and send it as `runID` in
every request of this run. Each layer stamps its evidence with it, and `finalize` refuses
`stale-evidence` when any batch, live document, or mutation document names a different run.

So a re-run is a new run ID and a full repeat of all four layers. Subset re-runs do not
exist here by construction: layer-1 evidence from one run cannot be combined with layer-4
evidence from another, and no bookkeeping trick makes it possible. Fix what `BLOCKED`
reported, then start again at the gate with a fresh run ID.

## Layer 1 — trace

```text
go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier trace -request -
```

with the shared request body. The op snapshots the repository, creates a disposable worktree
at HEAD, runs the source's own `runner-command` there, binds every `covers:` tag to its
declaration structurally — Go's parser and AST for Go, the token scanner for Java and Kotlin
— joins each binding to the runner's normalized records, compares the run against the
baseline evidence, disposes the worktree, and re-compares the snapshot.

Read from its output:

| Field | Use |
|---|---|
| `evidencePath` | Absolute path to `trace-evidence.json`. It is `traceEvidence` in both the `execute-live` and `finalize` requests, and the source of every layer-2 paste |
| `runDir` | Where this run's artifacts live; quote it to the user if the run has to be abandoned |
| `baselineStatus` | `ok`, `missing`, or `invalid: <reason>` |
| `findingCounts` | Findings by kind, for the approval-free summary you give the user as the run proceeds |
| `rowAccounting` | `{activeRows, withResult, unaccounted}`; `unaccounted` rows are gaps |
| `repoCommit` | The commit this verification describes |

The finding vocabulary is the executable's, and this skill uses no other names for these
conditions:

| Finding | Meaning |
|---|---|
| `gap` | No test cites the row, or its bound test did not run (skipped, or absent from the report) |
| `ambiguity` | The tag could not be pinned to exactly one declaration, or the runner emitted an identity the normalizer cannot express. Different repair from a missing test: the tag exists but binds to nothing definite |
| `drift` | A tag cites an ID that is neither active nor retired in the registry |
| `overclaim` | One test cites more than two active IDs. Each cited row must then be verdicted separately by layer 2 against its own named assertion |
| `red` | A test failing here that passed at baseline. Pre-existing failures are not this feature's `red` |

`finalize` adds one more of its own: `red-runner`, when the runner exited non-zero and
produced no normalized report at all.

**A `baselineStatus` other than `ok` is not a stop condition.** The run continues; finalize
turns it into a `baseline-<status>` reason and a `PARTIAL` verdict. Stopping here would
discard three layers of evidence over one missing comparison file.

### The one semantic act in layer 1

Everything above is the executable's. Exactly one judgment is this thread's: **freshness**.
Read the branch's production diff and classify it against the coverage source's behavior
groups. A hunk that changes an externally-observable behavior no row describes makes the
source stale — the yardstick no longer covers the code, and a verdict measured against it
would pass by omission.

The result is the `freshness` field of the finalize request:

```json
{"result": "fresh", "stale": []}
```

or

```json
{"result": "stale", "stale": [{"hunk": "internal/orders/guard.go:41-58", "behavior": "partial-refund path"}]}
```

`stale` with an empty list is refused `invalid-freshness`; a result outside `fresh|stale` is
refused the same way; omitting the field is `missing-freshness`. A `stale` result blocks the
branch and the repair is a coverage-source amendment, not a re-run.

## Layer 2 — adversarial read

Dispatch `coverage-critic`, one agent per behavior group, groups in parallel. Give each: the
group's rows, **each row's test source pasted inline** with its file path, and **the
production diff with test files excluded**.

### The paste comes from the trace evidence, never from a search

Each entry of `rowResults[]` in the trace evidence carries `rowId`, `scope`, `name`,
`status`, and the binding site: `file`, `line`, `endLine`. `line` is the line of the bound
`covers:` tag comment, which is contiguous with the declaration; `endLine` is the last line
of the test declaration. Read `file` lines `line` through `endLine` with the Read tool and
paste exactly that range — tag line included — as the row's source.

Unbound rows carry zero in all three fields. They are dispatched anyway, with
`no source found at <path>` in place of the paste: that is what lets the critic return
`MISSING` and lets the layer-1 reconciliation below run at all.

Never re-find a test by searching for its name. Names repeat across files, and the structural
binding the trace op already resolved is what settles which file a tag means. A fresh lookup
throws that away and can hand the critic a different file's test under this row's ID — and
because the paste is authoritative, the critic cannot notice.

**Paste it verbatim and complete** — the whole test body, every assertion, unedited. A
dispatcher that drops the binding assertion earns a `VACUOUS` on a healthy row.

When a test genuinely will not fit, elide **only** code outside the range you read — shared
setup, fixture builders, imports — and name each cut inline where it was made:
`// elided: setUp at OrderServiceTest.java:12-40`. Never elide an assertion, nor an arrange
step the assertions read from. If the body alone still will not fit, dispatch that row by
itself rather than trimming it.

A critic reason naming a truncated or unmarked-incomplete paste is a **dispatch defect**:
re-paste that row complete and re-dispatch it, and do not record the verdict it came with.

Excluding test files from the diff is not tidiness. At the finishing-a-development-branch
seam the branch diff normally *contains the new tests*, so a diff that included them would
hand the critic the real source of every row alongside the pasted one — naming the
synthesized canary below by inspection, no disk read required. Layer 2's question is about
production code; test files in the diff add nothing and cost the canary.

Withholding the diff would leave the critic unable to answer its own question, since "would
this fail if the mutation were applied" is a claim about production code. Its honesty comes
from having no edit tools, not from having no information.

### The canary

**Inject one canary per batch:** a test you already know is weak. A batch that verdicts
**the** canary healthy has all its verdicts voided and re-dispatches at a higher effort
tier. Without this, an all-`HOLDS` critic passes forever after the one-time acceptance check.

A critic that reads nothing and sacrifices a row escapes when it lands on the canary — ~1/N
per dispatch attempt, N the batch's row count (coin flip at N=2). Voided batches retry at a
higher tier, compounding against a persistent critic. A second canary would buy 1/C(N,2) at
one extra dispatch per batch and the rules needed to keep two canaries from identifying each
other.

Source each canary in this order:

1. **A naturally weak test** — one a prior run verdicted `WEAK` or `VACUOUS` **on its real
   source**. Preferred: nothing is fabricated and the expected verdict is already known. Two
   exclusions. A `WEAK`/`VACUOUS` earned by a weakened *copy* does not qualify: reusing it
   pastes the row's healthy real source, expects weakness, gets `HOLDS`, and voids the batch
   forever. And a row whose layer-1 status is `gap` or `ambiguity` cannot serve either —
   finalize forces `MISSING` on those rows and refuses any other verdict for them, so there
   is no weakness verdict for such a row to earn.
2. **A synthesized one.** Pick a row and paste, in place of its real test source, a copy
   weakened so it no longer binds to that row's `Verification mutation` — the binding
   assertion loosened, replaced with a presence check, or dropped. The correct verdict for
   the paste is `WEAK` or `VACUOUS`; a healthy verdict voids the batch. That row has now been
   verdicted on a copy rather than on its real source, so re-dispatch **that row alone** with
   its real source to get its actual verdict. One extra single-row dispatch per *synthesized*
   canary — at most one per batch, none when the canary is natural, which is verdicted on its
   real source already. Never a second full batch.

   Vary the weakening. `coverage-critic` is told to hunt for exactly the forms named above, so
   a batch in which one row and only one row asserts mere presence identifies itself. Weaken by
   loosening a boundary, asserting a mock instead of state, dropping to a shape check —
   whatever the row's real assertion makes natural.

   If a row cannot be weakened without the copy still binding — a golden-file or snapshot
   test, where the binding is the fixture and not an inline assertion — pick a different row.
   If none can, say so in the report and skip batch-voiding for that batch. That fallback is
   the narrow case it was always meant to be, not the default.

Option 1 is unavailable on exactly the run where the guard matters most: a healthy first run
has no prior verdicts, so a rule that stops at option 1 leaves the rubber-stamp critic
unguarded in the common case. That is circular — the canary would require the signal layer 2
exists to produce.

Two properties of the dispatch make a synthetic canary work, and both are load-bearing:

- **Every row's source is pasted, not just the canary's.** A batch where the canary row
  arrives inline and the rest arrive as file paths announces which row is the canary.
- **The paste is authoritative** — `coverage-critic` verdicts the source it was given, not
  what it may find on disk. Stated in the agent, so re-reading the file gains nothing.

Never write the weakened copy to disk. It exists only in the dispatch prompt: a synthetic
test file in the working tree would make every later op refuse `source-not-reproducible`,
and would be indistinguishable from a real test to the trace binding.

**Every dispatch states whether it carries a canary.** The single-row re-dispatches above
carry none, and cannot — weakening their only row is what they exist to undo. A canary-free
dispatch is legitimate exactly when it re-runs rows whose canary-bearing dispatch already
caught its canary; it inherits that batch's evidence and names it. Without this the rule
recurses: the re-dispatch needs a canary, synthesizing one needs another re-dispatch, forever.
Say which kind each dispatch is in the prompt, because `coverage-critic` is told a canary is
present and would otherwise conclude a healthy lone row must be the weak one.

### Bookkeeping

Record one `report.Batch` document per dispatch. These become `layer2.batches[]` of the
finalize request, and they are the report's audit trail:

```json
{
  "batchId": "batch-1",
  "dispatchId": "dispatch-1",
  "runId": "<run id>",
  "rows": ["SC-01", "SC-02", "SC-03"],
  "canary": {"rowId": "SC-01", "kind": "synthesized", "weakening": "...", "caught": true},
  "inheritsFrom": "",
  "voided": false,
  "verdicts": [{"rowId": "SC-02", "verdict": "HOLDS", "reason": "..."}]
}
```

Verdicts are `HOLDS`, `WEAK`, `VACUOUS`, or `MISSING`; anything else is refused
`invalid-verdict`. `WEAK`, `VACUOUS`, and `MISSING` all block. `canary.kind` is `natural` or
`synthesized`, and `weakening` is required and non-empty for a synthesized one.

The rules finalize enforces, each worth getting right the first time:

- **Exactly one non-voided verdict per active row.** None is `missing-verdict`; two is
  `duplicate-verdict`; a row verdicted only by voided batches is `voided-verdict`.
- **A layer-1 `gap` or `ambiguity` row must carry `MISSING`.** Any other verdict for it is
  `missing-verdict`.
- **The canary-bearing batch records no verdict for its own canary row.** The weakened copy's
  verdict is canary evidence — it lives in `canary.caught` — not a row verdict. The row's real
  verdict comes from its single-row re-dispatch, and a synthesized canary without that
  separate real-source batch is refused `canary-bookkeeping`.
- **A canary whose batch did not catch it must void that batch.** `caught: false` with
  `voided` unset is `uncaught-canary-not-voided`.
- **A canary-free batch must name `inheritsFrom`**, and it must name a batch whose canary was
  caught; anything else is `canary-bookkeeping`.
- **`runId` equals this run's ID** on every batch, or `stale-evidence`.
- **Every row named anywhere in a batch is an active row** of the source, or `unknown-row`.

When a canary is not caught, record `caught: false` and `voided: true`, then re-dispatch that
batch's rows at a higher effort tier as a **new** batch carrying its own canary. Do not point
the re-dispatch at the voided batch with `inheritsFrom`: voiding is transitive, so a batch
inheriting from a voided one is voided with it and its verdicts would not count either.
`inheritsFrom` exists for the canary-free single-row re-dispatch, and for that alone.

The report names the row that served as the canary for each batch, whether it was natural or
synthesized, and for a synthesized one the weakening applied — a claimed catch with no named
row and no named weakening is not auditable. It also names any batch that carried none, and
why.

### Reconciling `MISSING`

`MISSING` means the row's tagged test does not exist. On a row layer 1 already reported as
`gap` or `ambiguity`, that is the expected verdict and the two layers agree. On a row layer 1
reported as bound, they contradict each other: re-read that row's `file`, `line`, `endLine` in
the trace evidence. Non-zero values mean the paste was wrong, which is a dispatch defect —
re-paste from the recorded range and re-dispatch that row rather than recording the `MISSING`.

## Layer 3 — live

Layer 3 covers the active `smoke` and `e2e` rows.

### 1. Prepare

```text
go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier prepare-live -request -
```

with the shared request body. On success it returns `runID`, `worktreePath`, `runDir`,
`profilePath`, and `descriptor` — the live profile with `{run-id}` substituted throughout.

Two refusals are outcomes rather than errors:

- `no-applicable-rows` — the source has no active `smoke` or `e2e` row. Omit `layer3` from
  the finalize request entirely; the report says layer 3 is `n/a`, not skipped and not
  partial.
- `profile-unavailable` — the profile is missing, unreadable, or fails validation. The op has
  **already written** `live-attempt.json` into the run dir and its message names the path.
  Take that path as `layer3.attemptEvidencePath` and continue the run; finalize records
  `live-profile-unavailable` and the verdict becomes `PARTIAL`.

### 2. Dispatch `live-runner`

Give it the `worktreePath`, the `descriptor` and its `profilePath`, the applicable smoke and
e2e rows, the run ID, and the layer-1 gap set read from the trace evidence's findings.

Its entire final message is a `live.Plan` JSON document — no prose, no fence. Write that
message to `<runDir>/plan.json` byte-for-byte unchanged. Do not repair a plan by hand: the
plan is the agent's product, and editing it makes the evidence describe a run nobody
designed.

Two other replies are legitimate:

- `N/A` — no applicable row can be bound, because every one of them is a layer-1 gap. Treat
  it as `no-applicable-rows` above.
- A refusal naming the row whose recipe cannot be expressed through the profile's declared
  commands. Write it as an attempt document under the run dir and use it as
  `layer3.attemptEvidencePath`:

  ```json
  {"schemaVersion": "1", "verifierVersion": "0.6.0", "runID": "<run id>", "profile": "<profilePath>", "refusal": "<the agent's reason, verbatim>"}
  ```

  Finalize loads it with `report.LoadLiveAttempt`, which refuses a document without the
  envelope, `runID`, or `refusal`.

### 3. Execute

```text
go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier execute-live -request -
```

The request adds three fields to the shared body: `runDir`, `plan` (the absolute path to
`<runDir>/plan.json`), and `traceEvidence` (the absolute `evidencePath` from layer 1). The op
validates the plan against the profile, starts the environment, runs the steps, reduces the
transcripts, runs cleanup unconditionally, disposes the worktree, and writes
`live-evidence.json`.

Read `evidencePath` — it is `layer3.evidencePath` in the finalize request — plus
`rowVerdicts`, `cleanupOK`, and `cleanupFailed` for the running summary. Row verdicts are
`LIVE-PASS`, `LIVE-FAIL`, `LIVE-INCONCLUSIVE`, and `LIVE-N/A (layer 1 gap)`, which the op
assigns to gap rows itself.

**The reduced transcripts inside the evidence are the record; the agent's summary is not.**
Nothing here needs re-checking by hand: the verdicts come from the op that ran the commands,
not from the agent that proposed them.

A plan that fails validation is refused by name — `empty`, `unsupported`,
`shell-invocation-rejected`, `undeclared-command`, `undeclared-endpoint`,
`inexpressible-predicate`, `unknown-row`, `unbound-row`, `gap-row-bound`. Each is a defect in
the agent's plan, not in the repository: re-dispatch `live-runner` quoting the refusal message
so it designs to the rule it broke.

## Layer 4 — mutation

Layer 4 covers each active `risk=high` row whose layer-1 status is bound. Rows whose layer-1
status is `gap` or `ambiguity` are **never** dispatched: there is no executable test to
mutate, finalize derives `MUTATION-N/A (layer 1 gap)` for them itself, and supplying an entry
for one is refused `mutation-on-gap-row`.

### 1. Prepare the row's worktree

```text
go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier prepare-mutation -request -
```

The request adds `rowID` to the shared body. Output: `rowID`, `identity {scope, name}`,
`runnerKind`, `worktreePath`, `runDir`. It refuses `row-not-found` for an ID that is retired
or absent, `row-not-bound` when no tag in the worktree binds the row, and `dirty-repository`
when the tree is not clean.

### 2. Enumerate the sites

Dispatch `mutator` with the `worktreePath`, the SC row including its `Verification mutation`
text, and the resolved `identity`. It returns the list of sites where that mutation applies
and makes no edit on this call.

If it returns `unappliable` with a reason instead of a list, record the row as

```json
{"rowId": "SC-07", "outcome": "unappliable", "reason": "<the agent's reason, verbatim>"}
```

and move on. Finalize turns that into `MUTATION-UNAPPLIABLE`, which blocks: it is a
coverage-design defect — the row's `Verification mutation` was phrased as a negated
expectation rather than a code-level fault. Report it as such and never let the mutator
invent a substitute.

### 3. Apply one site, confirm it, evaluate it

Per site, in the order the agent listed them:

1. Dispatch `mutator` again naming exactly one site from its own list. It applies that edit
   inside the worktree and returns the site label, the file, and a one-line description.
2. **Confirm the diff yourself** before evaluating:

   ```text
   git -C <worktreePath> diff
   ```

   Read it and confirm it is the row's named verification mutation at that site.
   `evaluate-mutation` polices the diff structurally — one file, one hunk, no test, build,
   config, `docs/`, `.test-design/`, or `testdata/` path — but structure cannot tell a
   mutation that reproduces the row's fault from an unrelated one-line edit that happens to
   pass the same checks. This confirmation is the only place that judgment happens.

   If it is not the named mutation, do not call `evaluate-mutation` for this site. Record the
   row `unappliable` with what the diff actually did as the reason.
3. Evaluate:

   ```text
   go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier evaluate-mutation -request -
   ```

   The request adds `runDir`, `site`, and `lastSite` to the shared body. The op runs the row's
   targeted test, requires it to fail, restores the source inside the worktree, reruns the
   same target, requires it to pass, and records both outcomes under the site label. Output:
   `site`, `outcome`, and `diff {file, hunks}`; on the `lastSite: true` call it also returns
   the aggregated `sites[]` and the `evidencePath` of the row's mutation evidence.

Set `lastSite: true` on the final site. That call writes the evidence and disposes the
worktree; record the row as

```json
{"rowId": "SC-07", "evidencePath": "<the printed evidencePath>"}
```

Finalize derives `MUTATION-CAUGHT` when every enumerated site's targeted test failed with the
mutation applied and passed again after restore, and `MUTATION-SURVIVED`, naming each
surviving site, otherwise. One survivor blocks; no number of caught sites elsewhere offsets it.

### When confirmation leaves no valid site

The worktree is disposed only by an `evaluate-mutation` call with `lastSite: true`, so make
that call for the last site you were able to confirm. If no confirmable site remains — the
first site failed confirmation and it was the only one — there is nothing valid left to
evaluate. Record the row `unappliable` with the reason, leave the worktree in place, and name
both the `worktreePath` and the `runDir` in the report and to the user: disposing an abandoned
worktree is the controller's job, not this thread's, and deleting it by hand destroys the
evidence of why the row was abandoned.

### Environmental refusals

`evaluate-mutation` refusing `zero-selection`, `target-build-failed`, `clean-run-failed`,
`cleanup-failed`, or `unexplained-runner-failure` means the environment failed, not the test.
Write the refusal as an attempt document under the run dir:

```json
{"schemaVersion": "1", "verifierVersion": "0.6.0", "runID": "<run id>", "rowID": "SC-07", "code": "<refusal code>", "message": "<refusal message, verbatim>"}
```

and record the row as

```json
{"rowId": "SC-07", "outcome": "environmental", "reason": "<refusal message>", "attemptEvidencePath": "<that file>"}
```

`environmental` without an `attemptEvidencePath` is refused `missing-attempt-evidence`, and
the attempt document is loaded through `report.LoadAttempt`, which refuses anything without
the envelope. The row becomes `MUTATION-N/A (environmental)` and the run yields `PARTIAL`.

## Finalize

Assemble one request from the four layers:

```json
{
  "schemaVersion": "1",
  "repoRoot": ".",
  "coverageSource": "<repo-relative path>",
  "runID": "<run id>",
  "approval": "<the user's go-ahead, verbatim>",
  "traceEvidence": "<absolute evidencePath from trace>",
  "freshness": {"result": "fresh", "stale": []},
  "layer2": {"batches": []},
  "layer3": {"evidencePath": "<absolute evidencePath from execute-live>"},
  "layer4": {"rows": []}
}
```

`layer2.batches[]` holds every batch of layer 2, voided ones included — the voided batches are
what make the canary claim auditable. `layer3` carries `evidencePath` **or**
`attemptEvidencePath`, never both (`malformed-request`), and is absent entirely when no active
row is `smoke` or `e2e`. `layer4.rows[]` holds one entry per row layer 4 covered.

Run:

```text
go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier finalize -request -
```

The op re-checks the tree is clean, re-checks that HEAD is still the commit the trace ran
against, loads every evidence document named in the request, validates the bookkeeping,
computes the verdict, and publishes the report and the transcripts into the source repository.

Read from the output: `verdict`, `acknowledged`, `reasons[]` — each
`{layer, rowId, code, detail}` — `jsonPath`, `markdownPath`, `transcriptsDir`, and `runDir`.
Report the verdict and the reasons as the executable stated them. Do not re-derive a verdict,
re-word a reason, or summarize a blocking reason into something softer.

### PARTIAL acknowledgement, in this order

1. **Finalize publishes first**, with `acknowledged: false`. The report exists and says on its
   face that the acknowledgement is pending; there is no un-published PARTIAL to negotiate over.
2. **Ask on an actual user turn**, quoting the `PARTIAL` reasons the output gave. An
   agent-composed acknowledgement does not count.
3. **Commit or discard the first report before re-running.** The publish left the working tree
   dirty, and `finalize` — like every other run op — refuses `source-not-reproducible` on a
   dirty tree. This step is the fix for that refusal; there is no flag that skips it.
4. **Re-run `finalize`** with the same request plus `acknowledgement` set to the user's reply,
   verbatim. The second report is byte-identical to the first apart from `finishedAt` and the
   acknowledgement it now carries.

## Refusals

Every code below is a refusal the executable can return to this thread. Read the code first;
the message names the specifics.

| Code | Where | What this thread does next |
|---|---|---|
| `unsupported-schema-version` | any op | The request's `schemaVersion` is not `"1"`. Fix the request; this is a defect in the document this thread built, never in the repository |
| `source-not-reproducible` | `trace`, `prepare-live`, `execute-live`, `evaluate-mutation`, `finalize` | The working tree is dirty. Commit or discard the change the message names, then re-run the op. After a `PARTIAL` publish this is expected — see the acknowledgement order above |
| `dirty-repository` | `baseline`, `prepare-mutation` | Same condition, named at the ops that must start from a pristine tree. Commit or discard, then re-run. Never work around it |
| `snapshot-mismatch` | `execute-live`, `evaluate-mutation`, `finalize` | The repository changed while the op was running. This does **not** claim corruption: commit or discard the concurrent change, then re-run the run from a fresh run ID |
| `source-changed-during-verification` | `trace` | The same condition seen at the end of the trace run. It does **not** claim corruption. Commit or discard the concurrent change and re-run |
| `head-moved` | `finalize` | HEAD is no longer the commit the trace ran against. A verification describes one commit: mint a new run ID and repeat every layer at the current HEAD |
| `stale-evidence` | `finalize` | Some batch, live document, or mutation document names a different run ID than the request. A bookkeeping defect in the request this thread assembled — fix the request document, never the evidence, and never re-stamp a run ID |
| `missing-verdict` | `finalize` | An active row has no layer-2 verdict, a layer-3 applicable row is absent from `rowVerdicts`, a layer-4 applicable row has no entry, or a layer-1 gap row carries something other than `MISSING`. Add the missing entry to the request; do not invent a verdict for a row nothing verdicted |
| `voided-verdict` | `finalize` | A row is verdicted only by voided batches. Its re-dispatch never happened or was never recorded: re-dispatch that row and add the batch |
| `uncaught-canary-not-voided` | `finalize` | A batch records `caught: false` without `voided: true`. Set `voided: true` and add the higher-tier re-dispatch batch |
| `profile-unavailable` | `prepare-live` | The live profile is missing or invalid. `live-attempt.json` is already written: use it as `layer3.attemptEvidencePath` and continue; the verdict becomes `PARTIAL` |
| `zero-selection` | `evaluate-mutation` | The targeted test selector matched nothing in the worktree. Record the row `environmental` with the attempt document and continue |
| `row-not-bound` | `prepare-mutation` | No `covers:` tag binds the row in the worktree. Reconcile against layer 1: a row layer 1 called bound that is unbound here means the trace evidence and this worktree disagree, and the run must restart at a new run ID |
| `unexplained-runner-failure` | `trace`, `evaluate-mutation` | The runner exited non-zero with no failed test and no runner failure. From `trace` it stops the run — the suite command in the `## Verification` block does not work here, and the stderr file in the run dir says why. From `evaluate-mutation` record the row `environmental` and continue |

A code not in this table is a defect in this thread's request or in the environment, not a
verdict: report it verbatim with its message and the run dir, and stop rather than retrying
blind.

## Verdicts

Exactly three, and `finalize` decides which.

**`BLOCKED`** — any blocking reason. The executable raises them for: a `stale` freshness
result; `red-runner`; new failures against the baseline; any active row whose layer-1 status
is not `bound`; a layer-2 `WEAK`, `VACUOUS`, or `MISSING`; a layer-3 `LIVE-FAIL` or
`LIVE-INCONCLUSIVE`; a repository that changed during the live or mutation run; and a layer-4
`MUTATION-SURVIVED` or `MUTATION-UNAPPLIABLE`.

**`PARTIAL`** — no blocking reason, but at least one layer could not deliver a full answer.
The executable's causes, exhaustively: `baseline-<status>` when `baselineStatus` was not `ok`;
`live-profile-unavailable`; `live-cleanup-failed` and `live-cleanup-missing`;
`live-execution-errors`; `redaction-barrier` when a live transcript could not be reduced
without redacting evidence; and `mutation-environmental`.

A `PARTIAL` is not final until it is acknowledged. The published report says `Acknowledgement
pending` on its face until the acknowledgement re-run replaces it.

**`PASS`** — no reason at either severity. Every applicable row carried a verdict from every
applicable layer, and every non-applicable layer was `n/a` for a reason the report names.

## Degradation

| Situation | What the run does |
|---|---|
| No active `smoke`/`e2e` row | `prepare-live` refuses `no-applicable-rows`; `layer3` is absent from the finalize request and the report's layer 3 is `n/a` |
| Every applicable live row is a layer-1 gap | `live-runner` replies `N/A`; layer 3 is `n/a` with the applicable count named |
| Live profile missing or invalid | `layer3.attemptEvidencePath` from `live-attempt.json`; reason `live-profile-unavailable` → `PARTIAL` |
| Live cleanup failed, or recorded no success | `live-cleanup-failed` / `live-cleanup-missing` → `PARTIAL` |
| A live transcript cannot be reduced without redacting evidence | `redaction-barrier` on that row → `PARTIAL` |
| Baseline evidence absent or unverifiable | `trace` reports `baselineStatus` `missing` or `invalid: …`; the run continues; `baseline-<status>` → `PARTIAL` |
| `evaluate-mutation` refuses for environmental reasons | Row recorded `environmental` with an attempt document; `mutation-environmental` → `PARTIAL` |
| The row's `Verification mutation` cannot be applied as a source edit | Row recorded `unappliable`; `MUTATION-UNAPPLIABLE` → `BLOCKED`, and the repair is a coverage-source amendment |
| Row is a layer-1 gap | No layer-3 or layer-4 dispatch; finalize derives `LIVE-N/A (layer 1 gap)` and `MUTATION-N/A (layer 1 gap)` |
| Coverage source missing | Do not invoke; the feature did not opt in |

## Artifact

`finalize` publishes into the coverage source's own directory, named from its stem — a source
that does not end in `-coverage.md` is refused `source-name-unconventional`:

```text
<dir>/<stem>-verification.json     the canonical report document
<dir>/<stem>-verification.md       report.RenderMarkdown over that document
<dir>/transcripts/<run-id>/        every evidence document this run produced
```

The transcripts directory holds the trace evidence, the live evidence or live attempt, and one
mutation or mutation-attempt document per layer-4 row, each hashed in the report's
`transcripts[]` list. All of it is published as tracked repository content in a single commit-
ready change; nothing about this run lives only in a scratch directory.

For the rendered shape, see
[references/example-verification.md](references/example-verification.md) — the exact output of
`report.RenderMarkdown` over a `BLOCKED` run.

## Definition of done

- [ ] Approval obtained on a user turn before any op but `preflight` ran, and carried verbatim
      as the finalize request's `approval`
- [ ] One run ID minted at approval and sent in every request of this run
- [ ] Layer 1 ran through the `trace` op; freshness classified by this thread and recorded as
      `fresh` or `stale` with hunks and behaviors named
- [ ] Every layer-2 paste taken from the trace evidence's `file`, `line`, `endLine`, pasted
      whole, with any elision outside that range named inline
- [ ] Each canary-bearing batch names its canary, its kind, its weakening if synthesized, and
      whether it was caught; each canary-free batch names the caught-canary batch it inherits
      from; each uncaught canary voided its batch and spawned a higher-tier re-dispatch
- [ ] Layer 3 ran through `prepare-live` and `execute-live`, or `layer3` is absent or carries
      an attempt path for a named reason
- [ ] Every bound `risk=high` row has mutation evidence or a recorded `unappliable` /
      `environmental` outcome, and every applied diff was confirmed against the row's named
      mutation before `evaluate-mutation` ran
- [ ] `finalize` returned a verdict; its `reasons[]` are reported as it stated them
- [ ] A `PARTIAL` carries an acknowledgement obtained on a user turn, after the first report
      was committed or discarded
- [ ] The published report and transcripts are committed to the target repository

## Next stage

Verdict `PASS` or acknowledged `PARTIAL` → continue with
`superpowers:finishing-a-development-branch`.

Verdict `BLOCKED` → report the reasons the executable gave, fix them, and re-run from the gate
with a new run ID. Do not finish the branch.
