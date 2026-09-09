## Run state

The executable keeps one ledger per verification run. Every phase, review group, mutation
row and live attempt in it is `started`, then `completed` with its outcome and its retained
artifact, or `superseded` with a reason. Whenever a run is picked back up — after a crash, a
killed process, or a deliberate pause — ask the ledger what remains before calling anything
else; do not re-derive it from memory or from the last op's own output.

### Where the ledger lives

`TEST_DESIGN_STATE_DIR` names the state root when set; otherwise it is
`os.UserCacheDir()/test-design-verifier`. A run's directory is
`<stateDir>/runs/<repoKey>/<runID>`, where `repoKey` is the first 16 hex digits of the
SHA-256 of the resolved repository root. A state dir that resolves inside the repository
under verification is refused `ledger-open-failed` before anything is created — the ledger
must never become part of what a run snapshots.

Every op that touches the run's ledger takes the run identity from the shared request body's
`runID`, matching the common request shown in [SKILL.md](../SKILL.md) — except `execute-live`,
whose request carries no `runID` at all: it reads the run ID out of the prepared
`live-state.json` and addresses the ledger under that, because that file is what names which
run the attempt belongs to.

### Records: kinds, statuses, outcomes

Four kinds, one current record per obligation:

| Kind / ID | Captured at start | Retained at completion | Outcome vocabulary |
|---|---|---|---|
| `phase` / `trace` | `runDir` | `evidence` (trace-evidence.json), `dispatchInputs` (dispatch-inputs.json) | `ok` |
| `review-group` / `<groupId>` | the group's rows, each row's source hashes (primary first, then contributors), the dispatch-inputs digest they came from | `evidence` (`review-<groupId>.json`) | `holds` (every verdict `HOLDS`), `blocked` (any other) |
| `mutation-row` / `<rowId>` | the bound test's identity, the runner kind, the run dir, the worktree path | `evidence` (mutation-evidence.json, or `mutation-unappliable-<rowId>.json`), `attempt` (mutation-attempt.json, environmental only) | `caught`, `survived`, `unappliable`, `environmental` |
| `live-attempt` / `live` | the run dir, worktree path, profile path, plan digest | `evidence` (live-evidence.json), `attempt` (live-attempt.json, unavailable only) | `pass`, `fail`, `inconclusive`, `unavailable` |

A record's status is `started`, `completed`, or `superseded`. `Complete` never rewrites an
outcome: a completed record is immutable until superseded, and a superseded record keeps its
own outcome, artifacts and timestamps — it is replaced, never edited. `blocked`, `survived`,
`unappliable`, `environmental`, `fail`, `inconclusive` and `unavailable` are the failed
outcomes: a completed record carrying one of them is an answer, not a gap, and `resume` never
lists it as pending.

### `resume`: run this before any other op

Run `resume` at every restart, before calling any other op for the run. It is the one op that
only reads — it opens no worktree, runs no suite, writes no record — so asking it is always
safe, including while another process holds the run's work.

It reports, per layer, whether the layer is settled (`trace`, `review.complete`,
`live` via `required`/`recoveryRequired`, `mutation.complete`) and, in `pending`, exactly what
is still owed. Call it with the shared request body (`schemaVersion`, `repoRoot`,
`coverageSource`, `runID`) — it takes no other field. Read `pending` top to bottom: it is
sectioned, not interleaved by ledger position, so a parent reads one layer at a time —

1. the trace phase, only while it is not yet complete;
2. every review group the run opened, in ledger order, followed by one entry with an empty
   `id` and reason `uncovered: <rows>` naming every active row no current group claims — this
   entry closes the review section, and appears only once trace is complete;
3. the live attempt, when the coverage source requires layer 3;
4. mutation rows: every required row (high risk, active) the run already has a record for, in
   ledger order, then every required row never started, in dispatch order.

Each entry's `reason` is one of `never-started`, `started-not-completed`, or the reason a
completed record no longer stands (`stale`, `inputs-changed`, or a `Verify` failure such as
`artifact-missing:<role>`). A completed record that fails artifact verification, has changed
inputs, or is stale is reported with status `invalid`, the reason as the discriminator — but
`records[]` keeps the record's own status of `completed`: the ledger records what happened,
and what happened never changes; only whether it still counts can.

`review.uncovered` lists the active rows of the current dispatch inputs that no current group
claims, in dispatch order — the order the trace phase produced them, not ledger order.

`mutation.rows` lists every row the current dispatch requires (high risk, active) plus any
other row a current ledger record exists for, whether or not the dispatch still names it as
required — a row the run recorded once never drops out of the report. A row with no ledger
record reports `status: "none"`. Only a required row can appear in `pending` — a
bookkeeping-only record for a row that stopped being required never blocks a restart. A
started row's `worktreePath` is read back from `mutation-state.json` in the row's run dir, not
from the record's own captured inputs: `recover-mutation` rewrites that file when it restores
or recreates the worktree, so a parent re-dispatching the mutator gets the workspace that
exists now, not the one `prepare-mutation` opened.

`counts` tallies the whole ledger: `started`, `completed`, `failed` (completed records with a
failed outcome), `superseded`, `invalid` (completed records that no longer stand), and
`pending` (the length of the `pending` list above).

Output (fields relevant here; `records[]` carries the raw ledger too):

```json
{
  "runId": "run-2026-09-07-0001",
  "ledgerPath": "<stateDir>/runs/<repoKey>/run-2026-09-07-0001/ledger.json",
  "initialized": true,
  "stale": false,
  "trace": {"status": "completed", "outcome": "ok", "evidencePath": "...", "dispatchPath": "...", "runDir": "...", "reason": ""},
  "review": {"complete": false, "groups": [{"id": "batch-1", "rows": ["SC-07"], "status": "completed", "outcome": "holds", "evidencePath": "...", "reason": ""}], "uncovered": ["SC-09"]},
  "live": {"required": true, "status": "started", "recoveryRequired": true},
  "mutation": {"complete": false, "rows": [{"rowId": "SC-07", "status": "started", "worktreePath": "...", "completedSites": ["site-1"], "interruptedSite": "site-2"}]},
  "pending": [
    {"kind": "review-group", "id": "", "reason": "uncovered: SC-09"},
    {"kind": "mutation-row", "id": "SC-07", "reason": "started-not-completed"}
  ],
  "counts": {"started": 1, "completed": 2, "failed": 0, "superseded": 0, "invalid": 0, "pending": 2}
}
```

### Recording review groups: `record-review`

Call it with `status: "started"` before dispatching a group's critics, and again with
`status: "completed"` once every critic in that group has answered — around every critic
dispatch, never partway through one.

- `started` fixes the group's rows and the input identity they are judged against: each row's
  primary source hash first, then every contributor's, taken from the current trace record's
  dispatch inputs. Request: `groupId`, `status: "started"`, `rows` (every row the group
  covers).
- `completed` writes the group's evidence and records the outcome. Request: `groupId`,
  `status: "completed"`, `verdicts` (one `{rowId, verdict, reason}` per row of the group). The
  written evidence (`review-<groupId>.json`) retains `rows`, `inputs` (the same input identity
  captured at `started`) and `verdicts` — it carries no canary field at all; the canary's own
  accounting stays in finalize's `layer2.batches[]`, never in the ledger's review record.

Merged into the shared request body:

```json
{"groupId": "batch-1", "status": "started", "rows": ["SC-07", "SC-09"]}
```

```json
{"groupId": "batch-1", "status": "completed",
 "verdicts": [{"rowId": "SC-07", "verdict": "HOLDS", "reason": ""}, {"rowId": "SC-09", "verdict": "HOLDS", "reason": ""}]}
```

A repeated `trace` whose dispatch inputs are byte-identical to the ones a group was started
against leaves the group valid: `dispatch-inputs-changed` (at completion) and `resume`'s
`inputs-changed` (at read time) fire only when the recorded sources or the dispatch-inputs
digest actually differ, never merely because the phase ran again.

### Recording an unappliable row: `record-mutation`

Use it once the mutator, or your own confirmation of its edit, says no site of a row's
verification mutation applies as a code-level fault. The row must already be prepared for
this run (`prepare-mutation` ran for it); the op writes `mutation-unappliable-<rowId>.json`,
naming the prepared run dir and worktree, and completes the row with outcome `unappliable`.

Merged into the shared request body — `rowId`, `outcome: "unappliable"`, `reason` (why no
site applies, required, never blank):

```json
{"rowId": "SC-07", "outcome": "unappliable", "reason": "the mutation is a negated expectation, not a code fault"}
```

### Recovering an interrupted mutation site: `recover-mutation`

Call it only after `evaluate-mutation` refused `site-interrupted`, or after `resume` reported
the row's `interruptedSite` set — never on a healthy workspace. On one with no interruption to
recover from it restores the worktree to the prepared commit anyway, which discards whatever
the mutator's last applied-but-not-yet-evaluated edit put there.

It restores an attached worktree in place, or prunes and recreates one that vanished, then
requires an empty ignored-inclusive `git status` and HEAD at the prepared commit before it
clears the interruption marker. Completed sites in the row's state are kept; the interrupted
site's own capture directory is left as evidence and is overwritten only by that site's next
evaluation. A refused recovery (`restore-failed`, `recreate-failed`, `residue-remains`,
`wrong-revision`) never rewrites the state file — the marker stays set, the row stays pending,
and the abandoned worktree stays in place.

Merged into the shared request body — `runDir` (the row's prepared run directory, absolute):

```json
{"runDir": "<the row's run dir>"}
```

Output: `rowId`, `worktreePath` (the surviving or recreated path), `recreated`,
`completedSites`, `interruptedSite` (now `""`).

### `repeat.reason`

`trace`, `prepare-mutation`, `record-review started`, `execute-live` and `record-attempt` each
accept `repeat: {"reason": "<why>"}` in the shared request body to reopen an obligation this
run already completed: the current record is superseded with `detail` set to the reason, and a
fresh record starts. It is legitimate for exactly one thing — an identified environmental
recovery, such as a runner that crashed for reasons since fixed, or a rebuilt workspace — and
never to retry an unchanged failure hoping for a different answer; a completed failure is an
answer, not a retry queue. A changed input calls for a new run ID, never a repeat. An empty
reason is refused `missing-repeat-reason`; omitting `repeat` against a completed obligation is
refused `<kind>-completed`. `resume`'s `records[]` keeps the superseded record exactly as it
stood — status `superseded`, its original outcome, and the repeat reason as its `detail` —
alongside the new one, so nothing about what actually happened is lost when it is superseded.

### Request field spelling

The newer `record-review` (`verdicts[].rowId`) and `record-mutation` (`rowId`) requests and
outputs spell the row identity `rowId`; `prepare-mutation`, `evaluate-mutation` and
`record-attempt` requests still spell it `rowID`. Copy the field name from each op's own
example rather than from a neighboring request — a `rowID` pasted into `record-mutation`, or a
`rowId` pasted into `prepare-mutation`, is silently ignored, not refused.

### Refusals

| Op | Code | Meaning |
|---|---|---|
| `trace` | `phase-completed` | The trace phase is already completed for this run and the request carried no `repeat.reason`; add one or use a new run ID |
| `trace`, `resume` | `ledger-identity-mismatch` | The ledger already belongs to a different repo root, commit, source path or source digest than this request presents |
| `trace`, `resume` | `ledger-open-failed` | The state dir could not be resolved, or it resolves inside the repository under verification |
| `trace`, `resume` | `ledger-invalid` | The checkpoint could not be parsed, names a foreign schema version, or holds two current records for one obligation |
| `trace` | `ledger-write-failed` | The checkpoint could not be replaced; the prior checkpoint stands unchanged |
| `record-review` | `missing-group-id` | `groupId` is required |
| `record-review` | `invalid-group-id` | `groupId` must be a plain name of letters, digits, dot, dash or underscore |
| `record-review` | `invalid-status` | `status` must be `started` or `completed` |
| `record-review` | `missing-rows` | A started group needs at least one row |
| `record-review` | `duplicate-row` | The same row appears twice in `rows` |
| `record-review` | `unknown-row` | A row is not active in the coverage source, or not named by the trace's current dispatch inputs |
| `record-review` | `row-already-grouped` | The row already belongs to another current group; supersede that group or group different rows |
| `record-review` | `trace-incomplete` | This run's trace phase has not completed, or its evidence no longer verifies; run trace first |
| `record-review` | `review-group-completed` | The group is already completed for this run and the request carried no `repeat.reason` |
| `record-review` | `review-group-not-started` | `completed` was called for a group that `started` was never called for |
| `record-review` | `duplicate-completion` | The group is already completed; the first result stands |
| `record-review` | `missing-verdict` | A row of the group has no verdict in the completion |
| `record-review` | `duplicate-verdict` | The same row appears twice in `verdicts` |
| `record-review` | `verdict-outside-group` | A verdict names a row that is not one of the group's own |
| `record-review` | `invalid-verdict` | A verdict value is not one of `HOLDS`, `WEAK`, `VACUOUS`, `MISSING` |
| `record-review` | `dispatch-inputs-changed` | The group was judged against dispatch inputs the run no longer hands out; regroup against the current trace |
| `prepare-mutation`, `evaluate-mutation`, `record-mutation`, `recover-mutation` | `mutation-row-completed` | The row is already completed for this run (and, for `prepare-mutation`, the request carried no `repeat.reason`) |
| `evaluate-mutation` | `site-interrupted` | The workspace carries an uncleared interruption marker; run `recover-mutation` before measuring another site |
| `evaluate-mutation`, `record-mutation`, `recover-mutation` | `mutation-row-not-started` | The row was never prepared for this run |
| `record-mutation` | `missing-row-id` | `rowId` is required |
| `record-mutation` | `unknown-row` | The row is not active in the coverage source |
| `record-mutation` | `invalid-outcome` | The only outcome this op may record is `unappliable`; a caught or survived verdict comes from `evaluate-mutation` |
| `record-mutation` | `missing-reason` | An `unappliable` row needs a reason; a blank one is not evidence |
| `recover-mutation` | `missing-run-dir` | `runDir` is required |
| `recover-mutation` | `run-dir-not-absolute` | `runDir` must be an absolute path |
| `recover-mutation` | `run-dir-not-found` | `runDir` does not exist |
| `recover-mutation` | `state-load-failed` | `mutation-state.json` in `runDir` could not be read |
| `recover-mutation` | `stale-evidence` | The prepared state's run, source or HEAD no longer matches the request; prepare again |
| `recover-mutation` | `restore-failed` | The attached worktree could not be reset to the prepared commit |
| `recover-mutation` | `recreate-failed` | A missing worktree could not be pruned and recreated |
| `recover-mutation` | `residue-remains` | The restored worktree still carries ignored or untracked paths after reset |
| `recover-mutation` | `wrong-revision` | The restored worktree's HEAD is not the prepared commit |
| `record-attempt` | `mutation-row-completed` | Layer 4's row is already completed for this run |
| `record-attempt`, `execute-live` | `live-attempt-completed` | The live attempt is already completed for this run and the request carried no `repeat.reason` |

`evaluate-mutation`'s `lastSite` call both appends the site's evidence and completes the row
in one call: if the ledger write fails after the evidence was already appended, the refusal is
`ledger-write-failed` and names the evidence path in its message. The site's evaluation is
real, but the row cannot be completed under this run ID; mint a new run ID rather than
retrying the same one.
