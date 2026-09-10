## Finalize

The request carries three fields beyond the shared envelope — everything else the report
needs comes from the run's own ledger, written by the ops that already ran:

```json
{
  "schemaVersion": "1",
  "repoRoot": "<captured absolute TARGET_REPO>",
  "coverageSource": "<repo-relative path>",
  "runID": "<run id>",
  "approval": "<the user's go-ahead, verbatim>",
  "freshness": {"result": "fresh", "stale": []},
  "acknowledgement": "<the user's PARTIAL acknowledgement, verbatim>"
}
```

`acknowledgement` is omitted on the first `finalize` of a run and carried only on the rerun
that answers a published `PARTIAL`. A request still carrying `traceEvidence`, `layer2`,
`layer3`, or `layer4` — the previous protocol's pasted-evidence fields — is refused
`envelope-not-accepted`, naming the field: every layer's evidence now comes from the run's
own ledger, recorded by the op that produced it, and this thread pastes none of it.

Run:

```text
go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier finalize -request -
```

`finalize` recovers its own prior publication if one is owed (below), re-checks the tree is
clean, reads the run's ledger for every completed phase, review group, live attempt and
mutation row, validates the bookkeeping, computes the verdict, and publishes the report and
the transcripts into the source repository. An adopting run and a fresh run are assembled
the same way; the only difference is which records carry a `reuse` reference.

Read from the output: `verdict`, `acknowledged`, `reasons[]` — each `{layer, rowId, code,
detail}` — `jsonPath`, `markdownPath`, `transcriptsDir`, `runDir`, `measuredCommit` (the
revision the report measures), `recovered[]` (the own files this call put back before
republishing, each `{path, action: restored|removed}`, an empty array when nothing needed
it), and `work` (`{fresh, reused, failed, incomplete}` counts — the full breakdown is the
published report's `## Work` section, under [Artifact](#artifact) below). Report the verdict
and the reasons as the executable stated them. Do not re-derive a verdict, re-word a reason,
or summarize a blocking reason into something softer.

### What finalize reads, and what stops it before it reads anything

`finalize` asks the same question `resume` does before assembling anything:

- No ledger at all for this run ID, or a ledger whose trace phase never completed →
  `trace-incomplete`. Nothing is created under the state root by this refusal; run `trace`
  for this run ID (or finish an interrupted one) first.
- Any other obligation `resume` still lists pending — a review group, the live attempt, a
  mutation row — → `run-incomplete`, the message listing each entry exactly as `resume`
  would: `<kind> <id>: <reason>`. Answer every entry it names and rerun finalize; nothing is
  published while one is owed.

Once nothing is owed, `finalize` reads the run's current completed records and publishes
each one's document as a transcript under this name:

| Record | Published as |
|---|---|
| `phase/trace` | `trace-evidence.json` |
| `review-group/<id>`, fresh | `review-<id>.json` |
| `review-group/<id>`, adopted | `reused-review-<id>.json` |
| `live-attempt/live`, a journey ran | `live-evidence.json` |
| `live-attempt/live`, the environment refused | `live-attempt.json` |
| `mutation-row/<id>`, caught or survived | `mutation-<id>.json` |
| `mutation-row/<id>`, caught or survived, adopted | `reused-mutation-<id>.json` |
| `mutation-row/<id>`, environmental | `mutation-attempt-<id>.json` |
| `mutation-row/<id>`, environmental, adopted | `reused-mutation-attempt-<id>.json` |
| `mutation-row/<id>`, unappliable | `mutation-unappliable-<id>.json` |
| `mutation-row/<id>`, unappliable, adopted | `reused-mutation-unappliable-<id>.json` |

`resume` and `finalize` read one ledger with one set of rules, so they always agree about
what a run still owes; a divergence between them is a defect in one of the two, never a
bookkeeping fix available to this thread.

### Publication and `phase/publish`

Every publication — successful, partial, or retried — is recorded in the run's own ledger as
a `phase/publish` record: the measured revision, the run directory, the report's digest,
whether it carries an acknowledgement, and every file the publication writes with the digest
of the bytes it writes. The record names no artifacts of its own; those files are the record.
It never enters the report: `finalize` never refuses `run-incomplete` because of it — finalize
is about to redo it — and it never appears in `## Work`. `resume` does report an open
publication, in its own `pending[]` list (`{phase, publish, started-not-completed}`) and its
`publication` section, exactly like any other unfinished obligation.

A publication is complete only once that record completes. A `finalize` that fails before
writing anything, or after writing the report but before every file lands, leaves the record
started — a partially written report set — and changes nothing else: no trace, review, live
or mutation record is started, superseded, or invalidated by a publication failure or its
retry. Retry is `finalize` again, over the same request: the report is recomputed from the
same ledger evidence (only `finishedAt` differs), never copied from the failed attempt's run
directory.

### Recovering this run's own artifacts

Before the snapshot gate runs, `finalize` recovers exactly the files its own current
`phase/publish` record names — proven by digest against that record, path by path, never by
directory name, filename pattern, or timestamp. Every dirty entry in the tree is verified
before any of them is touched:

- A path the record does not name, or one it names whose working-tree state recovery does
  not undo (a deletion, a rename, a staged addition, or a conflict — recovery only ever
  undoes a modification or an untracked file) → `source-not-reproducible`, naming it. This is
  any dirt the run did not itself publish; commit or discard it, then rerun finalize.
- A path the record does name, whose bytes are unreadable or do not digest to what the
  record recorded → `publication-unverifiable`, naming it. This is the one case recovery
  refuses even though the path is its own: the bytes are not what this run wrote, so
  discarding them would not be recovery. Restore or commit the file by hand, or discard the
  edit, then rerun finalize.

Only once every dirty entry verifies is anything touched: a path tracked at HEAD is restored
with `git checkout -- <path>`, one path at a time; an untracked one is removed; and
`transcripts/<run-id>/` goes with the last file removed from it — removing an empty
directory is all that ever happens to one, so an ignored sentinel, or any file the record
does not name, keeps the directory (and refuses the run under the rule above instead).
`recovered[]` names each file recovery touched with its action, `restored` or `removed`, an
empty array when the tree was already clean. A run with no publication ever started, over a
dirty tree, also refuses `source-not-reproducible`: nothing is recorded, so nothing can be
proved this run's own.

This is the executable's job alone **for a rerun of the same run**. Do not `git checkout`,
remove, or restore a published file by hand before rerunning finalize — rerunning it is the
recovery step; hand intervention can only turn a provable own file into one recovery can no
longer verify. Once the run is abandoned there is nothing left to recover with, and the
prohibition lifts: after `head-moved`, commit or discard the old run's published files by
hand before tracing the new run (see the `head-moved` row below).

### PARTIAL acknowledgement, in this order

1. **Finalize publishes first**, with `acknowledged: false`. The report exists and says on
   its face that the acknowledgement is pending; there is no un-published PARTIAL to
   negotiate over.
2. **Ask on an actual user turn**, quoting the `PARTIAL` reasons the output gave. An
   agent-composed acknowledgement does not count.
3. Then one of two paths finishes it:
   - **Rerun `finalize`** with the same request plus `acknowledgement` set to the user's
     actual reply, verbatim. The executable recovers its own published files itself — the
     recovery above — before it republishes; this thread touches nothing in the tree first.
   - **Or commit the first report, then rerun.** Commit exactly the files the first
     publication wrote — `jsonPath`, `markdownPath`, and the report's `transcripts[]` paths —
     and nothing else. A commit that touches only this run's own publication paths does not
     move the measured revision: `finalize` and `resume` keep reporting the trace commit, not
     the commit that published over it. Rerun `finalize` with the acknowledgement at the new
     HEAD; there is nothing left for recovery to do, since the publication is now the
     committed state rather than a dirty one.

Either path ends the same way: an acknowledged report, republished, naming the same measured
revision as the first.

### `head-moved`

A commit that is not exactly this run's own publication paths moves the measured revision:
`finalize` refuses `head-moved`, naming the offending path and both commits — the one the run
was opened at and the current HEAD. Mint a new run ID and repeat every layer at the current
HEAD, reusing the approval line verbatim when the scope is unchanged (the repair-rerun rule
in SKILL.md's approval gate); an expanded scope needs a new gate.

## Refusals

Every code below is a refusal the executable can return to this thread. Read the code first;
the message names the specifics.

| Code | Where | What this thread does next |
|---|---|---|
| `unsupported-schema-version` | any op | The request's `schemaVersion` is not `"1"`. Fix the request; this is a defect in the document this thread built, never in the repository |
| `envelope-not-accepted` | `finalize` | The request still carries `traceEvidence`, `layer2`, `layer3`, or `layer4` — the message names which. Every layer's evidence now comes from the run's own ledger; drop the field rather than filling it in |
| `source-not-reproducible` | `trace`, `prepare-live`, `execute-live`, `evaluate-mutation`, `finalize` | The working tree carries dirt this run did not itself publish. Resolve only the changes the message names; if any commit or source changed, restart with fresh approval and run ID. At `finalize` this gate runs after its own recovery step, so it never names this run's own unrecovered PARTIAL publication |
| `dirty-repository` | `baseline`, `prepare-mutation` | Same condition, named at the ops that must start from a pristine tree. Commit or discard, then re-run. Never work around it |
| `snapshot-mismatch` | `execute-live`, `evaluate-mutation`, `finalize` | The repository changed while the op was running. This does **not** claim corruption. At `execute-live` and `evaluate-mutation`: commit or discard the concurrent change, then re-run the run from a fresh run ID. At `finalize` nothing was published and the `phase/publish` record stays started: discard the concurrent change and rerun `finalize` over the same request, under the same run ID. Once that change is committed it is `head-moved` instead |
| `ledger-changed` | `finalize` | The run's own ledger moved between the read the report was assembled from and the transaction that opens the publication — another caller repeated an obligation and answered it differently, which the snapshot gate cannot see because the ledger is not in the repository. The message names what differs. Nothing was published and no publish record was written; an earlier publication of this run was already taken back by the recovery step above, so rerun `finalize` over the same request — it assembles the report from the ledger as it now stands and publishes it |
| `source-changed-during-verification` | `trace` | The same condition seen at the end of the trace run. It does **not** claim corruption. Commit or discard the concurrent change and re-run |
| `head-moved` | `finalize` | HEAD is no longer the commit this run was opened at, for a reason other than this run's own publication — the message names the offending path and both commits. Mint a new run ID and repeat every layer at the current HEAD, reusing the approval line verbatim when the scope is unchanged. If the old run left an uncommitted publication in the tree, commit or discard those files by hand before tracing the new run: `head-moved` is decided before recovery, so the old run can no longer take them back, and the new run's `trace` refuses `source-not-reproducible` while they sit there |
| `trace-incomplete` | `finalize` | This run has no ledger, or its trace phase never completed. Run `trace` for this run ID — or finish an interrupted one — before finalizing |
| `run-incomplete` | `finalize` | Some other obligation `resume` still lists pending, named `<kind> <id>: <reason>` per entry. Answer every entry and rerun finalize; nothing was published. One reason cannot be answered: `phase trace: stale` means the coverage source changed under the run, so no obligation of this run is about the source as it now reads — start a new run at a new run ID |
| `stale-evidence` (source hash mismatch) | `finalize` | An evidence document's `sourceSha256` does not match the coverage source's current hash. The coverage source changed mid-run — mint a new run ID and repeat every layer against the source as it now reads |
| `missing-verdict` | `finalize` | An active row has no layer-2 verdict, a layer-3-applicable row has none, a layer-4-applicable row has no mutation entry, or a layer-1 gap row's recorded verdict is not `MISSING`. The first three should never happen once `resume` reports nothing pending — treat it as a defect in `resume` or `finalize`, not a request this thread can fix. The last is a critic dispatch that disagreed with layer 1: reconcile per [critic phase](critic.md) and repeat that group's review |
| `publication-unverifiable` | `finalize` | An own path from a prior publication of this run has bytes that are not what this run wrote — the message names it. Restore or commit the file by hand, or discard the edit, then rerun; recovery will not discard bytes it cannot prove are its own |
| `recovery-failed` | `finalize` | An internal failure: recovery could not complete a change it had already proven safe — a trackedness question git could not answer, or a checkout or removal that failed partway through applying its plan. Not a bookkeeping defect in this request or evidence of tampering; repair the repository by hand per the message and rerun finalize |
| `profile-unavailable` | `prepare-live` | The live profile is missing or invalid. `prepare-live` has already recorded the refused attempt in the ledger (`live-attempt/live`, outcome `unavailable`), its document written into the run dir (`runs/<run-id>/live-attempt.json`) and the worktree disposed; the `live-attempt-completed` gate is the one that refuses before anything is created. The record is in the ledger, finalize reads it, and the run continues toward `live-profile-unavailable` → `PARTIAL` |
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
is not `bound`; row-less layer-1 findings — `ambiguity` and `drift`, which name no active
row; a layer-2 `WEAK`, `VACUOUS`, or `MISSING`; a layer-3 `LIVE-FAIL` or `LIVE-INCONCLUSIVE`;
a completed live command whose predicates failed but bound no row
(`live-unbound-assertion-failed`); a repository that changed during the live or mutation run;
`invalid-verdict` at layer 2, 3, or 4, when a row's verdict value falls outside that layer's
closed set; and a layer-4 `MUTATION-SURVIVED` or `MUTATION-UNAPPLIABLE`.

**`PARTIAL`** — no blocking reason, but at least one layer could not deliver a full answer.
The executable's causes, exhaustively: `baseline-<status>` when `baselineStatus` was not
`ok`; `live-profile-unavailable`; `live-cleanup-failed` and `live-cleanup-missing`;
`live-execution-errors`; `redaction-barrier` when a live transcript could not be reduced
without redacting evidence; and `mutation-environmental`.

A `PARTIAL` is not final until it is acknowledged. The published report says `Acknowledgement
pending` on its face until the acknowledgement re-run replaces it.

**`PASS`** — no reason at either severity. Every applicable row carried a verdict from every
applicable layer, and every non-applicable layer was `n/a` for a reason the report names.

## Degradation

| Situation | What the run does |
|---|---|
| No active live-applicable row (live decision `not-applicable`) | `prepare-live` refuses `no-applicable-rows`; nothing is recorded for layer 3 at all; the report reads layer 3 as `n/a` from the live decision |
| Live profile missing or invalid | `prepare-live` records the refused attempt itself (`live-attempt/live`, outcome `unavailable`), writing its document into the run dir (`runs/<run-id>/live-attempt.json`) and disposing the worktree; the `live-attempt-completed` gate is what refuses before anything is created. The record is in the ledger, finalize reads it; reason `live-profile-unavailable` → `PARTIAL` |
| Live cleanup failed, or recorded no success | `live-cleanup-failed` / `live-cleanup-missing` → `PARTIAL` |
| A completed live command's predicates failed but bound no row (an unbound `start`, `health`, plan, or plan-cleanup step) | `unboundFailures` non-empty → `live-unbound-assertion-failed` → `BLOCKED` |
| A live transcript cannot be reduced without redacting evidence | `redaction-barrier` on that row → `PARTIAL` |
| Baseline evidence absent or unverifiable | `trace` reports `baselineStatus` `missing` or `invalid: …`; the run continues; `baseline-<status>` → `PARTIAL` |
| `evaluate-mutation` refuses for environmental reasons | Row recorded `environmental` with an attempt document; `mutation-environmental` → `PARTIAL` |
| The row's `Verification mutation` cannot be applied as a source edit | Row recorded `unappliable`; `MUTATION-UNAPPLIABLE` → `BLOCKED`, and the repair is a coverage-source amendment |
| Row is a layer-1 gap | Layer 4 is never dispatched for it; finalize derives `MUTATION-N/A (layer 1 gap)`. Layer 3 is unaffected by the gap: an applicable gap row is planned, run and judged exactly like any other row, marked `layer1Gap: true` beside its verdict — only layer 1's own finding still blocks the branch |
| Coverage source missing | Do not invoke; the feature did not opt in |

## Artifact

`finalize` publishes into the coverage source's own directory, named from its stem. Both
conventional dedicated coverage sources and approved inline floor-rule sources are accepted
by the runtime:

```text
<dir>/<stem>-verification.json     the canonical report document
<dir>/<stem>-verification.md       report.RenderMarkdown over that document
<dir>/transcripts/<run-id>/        every document this run's records name, under the names below
```

Every transcript name `finalize` can write:

```text
trace-evidence.json                          the trace phase
review-<groupId>.json                        a fresh layer-2 review group's verdicts
reused-review-<groupId>.json                 an adopted layer-2 review group's verdicts
live-evidence.json                           a live journey that ran
live-attempt.json                            a live attempt the environment refused
mutation-<rowId>.json                        a fresh caught or survived mutation row
reused-mutation-<rowId>.json                 an adopted caught or survived mutation row
mutation-attempt-<rowId>.json                a fresh environmental mutation row
reused-mutation-attempt-<rowId>.json         an adopted environmental mutation row
mutation-unappliable-<rowId>.json            a fresh row whose mutation could not be applied
reused-mutation-unappliable-<rowId>.json     an adopted row whose mutation could not be applied
```

Every one of these is hashed in the report's `transcripts[]` list and named again in the
report's `## Work` section, below, against the obligation it proves; nothing about this run
lives only in a scratch directory. All of it is published as tracked repository content in a
single commit-ready change.

### The `## Work` section

The published report's `## Work` section is the run's own accounting of every obligation it
answered: one line `Fresh N · Reused N · Failed N · Incomplete N`, then a row per obligation —
its kind (`phase`, `review-group`, `live-attempt`, `mutation-row`), its ID, the rows it
covers, whether it is `fresh` or `reused`, its outcome, its standing (`ok`, `failed`, or
`incomplete`), and the transcript that proves it, by path. A reused item's row is followed by
a bullet naming the run and sequence it was adopted from and the basis it was judged on. Read
it to see, at a glance, what this run actually did versus what it adopted from a prior one —
never re-derive this accounting from the transcripts by hand.

For the rendered shape, see
[example-verification.md](example-verification.md) — the exact output of
`report.RenderMarkdown` over a `BLOCKED` run.

## Evidence compatibility

Current generated evidence uses verifierVersion `"1.2.0"` and schemaVersion `"1"`.
Live/mutation evidence and attempts require `repoCommit`, `source`, and `sourceSha256`
as well as their existing run/row identity spellings. Old unbound evidence is refused;
regenerate it in a newly approved full run. Never infer missing fields or hand-stamp old
JSON. Existing baseline schema remains usable; this is not a baseline migration. A batch
document carrying the retired `canary`, `inheritsFrom`, `voided`, or `canaryUnavailableReason`
fields decodes with them ignored: nothing about evidence already on disk needs migrating for
their removal.
