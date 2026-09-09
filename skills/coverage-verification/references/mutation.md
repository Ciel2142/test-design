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

Dispatch [mutator](../../../agents/mutator.md) with the `worktreePath`, the SC row facts from `dispatchPath`, including `verificationMutation`
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

   If it is not the named mutation, the **row** is over. Do not call `evaluate-mutation` for
   this site or for any later site of this row; record the one row entry described under
   [When confirmation fails](#when-confirmation-fails) and move to the next row.
3. Evaluate:

   ```text
   go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier evaluate-mutation -request -
   ```

   The request adds `runDir`, `site`, and `lastSite` to the shared body. The op runs the row's
   targeted test, requires it to fail, restores the source inside the worktree, reruns the
   same target, requires it to pass, and records both outcomes under the site label. Output:
   `site`, `outcome`, and `diff {file, hunks}`; on the `lastSite: true` call it also returns
   the aggregated `sites[]` and the `evidencePath` of the row's mutation evidence.

Set `lastSite: true` on the final site of a row whose every site confirmed. That call writes
the evidence and disposes the worktree; record the row as

```json
{"rowId": "SC-07", "evidencePath": "<the printed evidencePath>"}
```

Finalize derives `MUTATION-CAUGHT` when every enumerated site's targeted test failed with the
mutation applied and passed again after restore, and `MUTATION-SURVIVED`, naming each
surviving site, otherwise. One survivor blocks; no number of caught sites elsewhere offsets it.

### When confirmation fails

A confirmation failure aborts the **row**, not just the site — at any position in the site
list, including the middle of it. Nothing in that worktree can be salvaged afterwards:
`mutator` never reverts its own edit and only `evaluate-mutation` restores the source, so the
unconfirmed edit stays on disk and the diff police refuses every later call against that
worktree; and a site already evaluated cannot be re-run, because `evaluate-mutation` refuses
a repeat as `duplicate-site`. There is no "last good site" to fall back to.

So, exactly once for the row:

```json
{"rowId": "SC-07", "outcome": "unappliable", "reason": "<what the diff actually did, at which site>"}
```

One entry per row — a second entry for the same row is refused `duplicate-verdict`. Evaluate
no further site of that row, and make no further `evaluate-mutation` call against that
worktree. No op will dispose it, so leave it in place and name both its `worktreePath` and its
`runDir` in the report and to the user, so it can be removed: disposing an abandoned worktree
is the controller's job, not this thread's, and deleting it by hand destroys the evidence of
why the row was abandoned.

### Environmental refusals

`evaluate-mutation` refusing `zero-selection`, `target-build-failed`, `clean-run-failed`,
`cleanup-failed`, or `unexplained-runner-failure` means the environment failed, not the test.
Record the actual code and message verbatim in `refusal`,
using the shared request plus `layer: 4` and this `rowID`:

```text
go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier record-attempt -request -
```

The op returns source/commit-stamped `evidencePath`; never hand-write or re-stamp an attempt.
Use that path as `attemptEvidencePath`,

and record the row as

```json
{"rowId": "SC-07", "outcome": "environmental", "reason": "<refusal message>", "attemptEvidencePath": "<that file>"}
```

`environmental` without an `attemptEvidencePath` is refused `missing-attempt-evidence`, and
the attempt document is loaded through `report.LoadAttempt`, which refuses anything without
the envelope. The row becomes `MUTATION-N/A (environmental)` and the run yields `PARTIAL`.

### Restart

If a run is picked back up, `resume` names every started mutation row with its
`completedSites` and its `interruptedSite` — read [run state](run-state.md) in full before
repeating any op here. `evaluate-mutation` refuses `site-interrupted` for that row until
`recover-mutation` has restored or recreated its workspace: nothing is measured on a worktree
of unknown cleanliness. Completed sites are kept either way — recovery never re-measures a
site the row already answered. A refused recovery leaves the row exactly where it was:
pending, and the abandoned worktree in place, not removed on your account.
