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
| `review-group` / `<groupId>` | the group's rows, each row's source hashes (primary first, then contributors), the dispatch-inputs digest they came from | `evidence` (`review-<groupId>-<seq>.json`) | `holds` (every verdict `HOLDS`), `blocked` (any other) |
| `mutation-row` / `<rowId>` | the bound test's identity, the runner kind, the run dir, the worktree path | `evidence` (mutation-evidence.json, or `mutation-unappliable-<rowId>-<seq>.json`), `attempt` (mutation-attempt.json, environmental only) | `caught`, `survived`, `unappliable`, `environmental` |
| `live-attempt` / `live` | the run dir, worktree path, profile path, plan digest | `evidence` (live-evidence.json), `attempt` (live-attempt.json, unavailable only) | `pass`, `fail`, `inconclusive`, `unavailable` |
| `phase` / `publish` | the measured commit, the run dir, the report's digest, whether it carries an acknowledgement, and every file the publication writes with its digest | nothing — the captured files are the evidence | `ok` |

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
4. mutation rows: every mutation row the run opened whose record is started or no longer
   stands, in ledger order — whatever its risk — then every required row (high risk, active)
   never started, in dispatch order;
5. the publish phase: one entry `{phase, publish, started-not-completed}`, appearing only when
   the ledger's current publish record is started — never `never-started`, since nothing
   attempts publication before every obligation above is settled.

Each entry's `reason` is one of `never-started`, `started-not-completed`, or the reason a
completed record no longer stands: `stale`, `inputs-changed`, `inputs-unreadable` (the
record's own captured inputs cannot be decoded), `dispatch-inputs-unreadable` (the trace
phase's dispatch-inputs file cannot be read, which leaves the whole run at the trace), or a
`Verify` failure such as `artifact-missing:<role>` or `artifact-digest-mismatch:<role>`. A
completed record that fails artifact verification, has changed or unreadable inputs, or is
stale is reported with status `invalid`, the reason as the discriminator — but `records[]`
keeps the record's own status of `completed`: the ledger records what happened, and what
happened never changes; only whether it still counts can. One more reason appears only in
`mutation.rows[]`, never in `pending`: a started row whose `mutation-state.json` cannot be
read carries `state-unreadable` there, while its `pending` entry stays
`started-not-completed`.

`review.uncovered` lists the active rows of the current dispatch inputs that no current group
claims, in dispatch order — the order the trace phase produced them, not ledger order.

`mutation.rows` lists every row the current dispatch requires (high risk, active) plus any
other row a current ledger record exists for, whether or not the dispatch still names it as
required — a row the run recorded once never drops out of the report. A row with no ledger
record reports `status: "none"`. Risk decides only what a run must do, never what it has
already taken on: a row the run opened is owed until it is answered, so a started row, or
one whose completion no longer stands, appears in `pending` whether or not the current
dispatch inputs require it; only the rows never started are limited to the required ones. A
started row's `worktreePath` is read back from `mutation-state.json` in the row's run dir, not
from the record's own captured inputs: `recover-mutation` rewrites that file when it restores
or recreates the worktree, so a parent re-dispatching the mutator gets the workspace that
exists now, not the one `prepare-mutation` opened.

`counts` tallies the whole ledger: `started`, `completed`, `failed` (completed records with a
failed outcome), `superseded`, `invalid` (completed records that no longer stand), `pending`
(the length of the `pending` list above), and `reused` (current completed records whose inputs
carry `reuse`).

`headCommit` is the repository's current HEAD — distinct from the commit the ledger recorded.
Staleness is judged against the measured commit, not HEAD directly: a commit that touched only
this run's own publication paths is drift, not staleness. `publication` reports the current
`phase/publish` record: `status` (`none`, `started`, or `completed`), `outcome`,
`measuredCommit`, `acknowledged`, the `files` it named with their digests, and `reason`
(`started-not-completed` when `status` is `started`).

Output (fields relevant here; `records[]` carries the raw ledger too):

```json
{
  "runId": "run-2026-09-07-0001",
  "ledgerPath": "<stateDir>/runs/<repoKey>/run-2026-09-07-0001/ledger.json",
  "initialized": true,
  "stale": false,
  "headCommit": "<HEAD>",
  "trace": {"status": "completed", "outcome": "ok", "evidencePath": "...", "dispatchPath": "...", "runDir": "...", "reason": ""},
  "review": {"complete": false, "groups": [{"id": "batch-1", "rows": ["SC-07"], "status": "completed", "outcome": "holds", "evidencePath": "...", "reason": ""}], "uncovered": ["SC-09"]},
  "live": {"required": true, "status": "started", "recoveryRequired": true},
  "mutation": {"complete": false, "rows": [{"rowId": "SC-07", "status": "started", "worktreePath": "...", "completedSites": ["site-1"], "interruptedSite": "site-2"}]},
  "publication": {"status": "none", "outcome": "", "measuredCommit": "", "acknowledged": false, "files": [], "reason": ""},
  "pending": [
    {"kind": "review-group", "id": "", "reason": "uncovered: SC-09"},
    {"kind": "mutation-row", "id": "SC-07", "reason": "started-not-completed"}
  ],
  "counts": {"started": 2, "completed": 2, "failed": 0, "superseded": 0, "invalid": 0, "pending": 2, "reused": 0}
}
```

A publication never appears beside those entries: `finalize` opens the `phase/publish` record
only once nothing else is owed, and it records the revision the report measures and every file
the publication will write at the moment it starts. So an open publication is a run whose only
pending entry is its own, with `measuredCommit` and `files` already filled:

```json
{
  "publication": {
    "status": "started",
    "outcome": "",
    "measuredCommit": "4c1f0b9e2a7d8c3f5b6a109e8d7c6b5a4f3e2d10",
    "acknowledged": false,
    "files": [
      {"path": "docs/superpowers/coverage/2026-09-07-subject-verification.json", "sha256": "e9c6b698…"},
      {"path": "docs/superpowers/coverage/2026-09-07-subject-verification.md", "sha256": "4dffe677…"},
      {"path": "docs/superpowers/coverage/transcripts/run-2026-09-07-0001/trace-evidence.json", "sha256": "38b3700e…"}
    ],
    "reason": "started-not-completed"
  },
  "pending": [{"kind": "phase", "id": "publish", "reason": "started-not-completed"}]
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
  written evidence (`review-<groupId>-<seq>.json`, where `<seq>` is the started record's
  position in the ledger, so a repeated group's completion never overwrites the evidence its
  superseded record digested) retains `rows`, `inputs` (the same input identity captured at
  `started`) and `verdicts`. This record is what `finalize` reads as the group's layer-2
  batch — the parent never assembles or pastes one.

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

Both calls run their checks and their write under the run's exclusive ledger lock, so parents
working one run in parallel cannot each pass a check the other has already falsified: ten
`started` calls naming one row in ten groups leave the row in exactly one of them, the rest
refused `row-already-grouped`; two `completed` calls for one group leave one result, its
evidence written and digested under the same lock, the other refused `duplicate-completion`
without writing anything.

### Recording an unappliable row: `record-mutation`

Use it once the mutator, or your own confirmation of its edit, says no site of a row's
verification mutation applies as a code-level fault. The row must already be prepared for
this run (`prepare-mutation` ran for it); the op writes
`mutation-unappliable-<rowId>-<seq>.json` (`<seq>` the started record's ledger position),
naming the prepared run dir and worktree, and completes the row with outcome `unappliable` —
the write and the completion under one exclusive lock, so a second verdict for the row finds
it completed and writes nothing.

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

It restores an attached worktree in place; one whose path no longer exists it recreates,
first forgetting that one worktree's registry entry (`git worktree remove --force` on the
path alone — the repository's registry is shared with every other worktree of it, so nothing
is pruned and another worktree whose directory happens to be absent at that moment, such as
one on an unmounted volume, keeps its entry). Only a path that does not exist counts as
vanished: a path that exists as something other than a directory, or that cannot be checked
at all, is refused `restore-failed`, never replaced. Either way it then requires an empty
ignored-inclusive `git status` and HEAD at the prepared commit before it clears the
interruption marker. Completed sites in the row's state are kept; the interrupted
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

The canonical spelling of the row identity differs by op: `record-review`
(`verdicts[].rowId`), `record-mutation` (`rowId`) and the `resume`/`recover-mutation` outputs
spell it `rowId`; the `prepare-mutation`, `evaluate-mutation` and `record-attempt` requests
spell it `rowID`. Copy the field name from each op's own example. The decoder matches a JSON
key case-insensitively when no exact match exists, so a `rowID` pasted into `record-mutation`,
or a `rowId` pasted into `prepare-mutation`, is accepted rather than ignored or refused — but
the spelling each example shows is the one the op documents and the one its output uses.

### Refusals

| Op | Code | Meaning |
|---|---|---|
| `trace` | `phase-completed` | The trace phase is already completed for this run and the request carried no `repeat.reason`; add one or use a new run ID |
| `trace`, `record-review`, `prepare-mutation`, `evaluate-mutation`, `record-mutation`, `record-attempt`, `execute-live` | `ledger-identity-mismatch` | The ledger already belongs to a different repo root, commit, source path or source digest than this request presents — raised by the ledger transaction every `Start`/`Complete` runs in, before any check inside it; `resume` and `recover-mutation` never write, so they never return it |
| every ledger-touching op — `trace`, `record-review`, `prepare-mutation`, `evaluate-mutation`, `record-mutation`, `recover-mutation`, `record-attempt`, `execute-live`, `resume` | `ledger-open-failed` | The state dir could not be resolved, or it resolves inside the repository under verification — raised by `openLedger` itself, before anything is read |
| every ledger-touching op — same list as above | `ledger-invalid` | The checkpoint could not be parsed, names a foreign schema version, or holds two current records for one obligation — raised wherever the checkpoint is loaded, whether for a read or at the start of a write transaction |
| every ledger-touching op except `resume` and `recover-mutation`, which never write — `trace`, `record-review`, `prepare-mutation`, `evaluate-mutation`, `record-mutation`, `record-attempt`, `execute-live` | `ledger-write-failed` | The checkpoint could not be replaced after a `Start` or `Complete` transition; the prior checkpoint stands unchanged |
| `record-review` | `missing-group-id` | `groupId` is required |
| `record-review` | `invalid-group-id` | `groupId` must be a plain name of letters, digits, dot, dash or underscore |
| `record-review` | `invalid-status` | `status` must be `started` or `completed` |
| `record-review` | `missing-rows` | A started group needs at least one row |
| `record-review` | `duplicate-row` | The same row appears twice in `rows` |
| `record-review` | `unknown-row` | A row is not active in the coverage source, or not named by the trace's current dispatch inputs |
| `record-review` | `row-already-grouped` | The row already belongs to another current group; supersede that group or group different rows — decided under the same exclusive lock as the group's own `started`, so concurrent parents cannot both claim a row |
| `record-review` | `trace-incomplete` | This run's trace phase has not completed, or its evidence no longer verifies; run trace first |
| `record-review` | `review-group-completed` | The group is already completed for this run and the request carried no `repeat.reason` |
| `record-review` | `review-group-not-started` | `completed` was called for a group that `started` was never called for |
| `record-review` | `duplicate-completion` | The group is already completed; the first result stands — decided under the lock, so a second concurrent completion writes no evidence |
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
| `recover-mutation` | `restore-failed` | The attached worktree could not be reset to the prepared commit, or the worktree path exists as something other than a directory, or could not be checked at all |
| `recover-mutation` | `recreate-failed` | A vanished worktree's own registry entry could not be forgotten, or a replacement could not be created |
| `recover-mutation` | `residue-remains` | The restored worktree still carries ignored or untracked paths after reset |
| `recover-mutation` | `wrong-revision` | The restored worktree's HEAD is not the prepared commit |
| `record-attempt` | `mutation-row-completed` | Layer 4's row is already completed for this run |
| `prepare-live`, `record-attempt`, `execute-live` | `live-attempt-completed` | The live attempt is already completed for this run and the request carried no `repeat.reason` |
| `execute-live` | `stale-evidence` | HEAD is no longer the commit `prepare-live` prepared the workspace at; run `prepare-live` again — refused before the ledger is opened, the way `evaluate-mutation` and `recover-mutation` refuse it, so no record is created under a commit the evidence does not name |

`evaluate-mutation`'s `lastSite` call both appends the site's evidence and completes the row
in one call: if the ledger write fails after the evidence was already appended, the refusal is
`ledger-write-failed` and names the evidence path in its message. The site's evaluation is
real, but the row cannot be completed under this run ID; mint a new run ID rather than
retrying the same one.
