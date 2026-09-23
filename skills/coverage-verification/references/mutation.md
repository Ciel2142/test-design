## Layer 4 — mutation

Layer 4 covers each active `risk=high` row whose layer-1 status is bound. Rows whose layer-1
status is `gap` or `ambiguity` are **never** dispatched: there is no executable test to
mutate, and finalize derives `MUTATION-N/A (layer 1 gap)` for them itself. Nothing refuses a
mutation record for such a row while it is being recorded; `finalize` refuses the whole run
`mutation-on-gap-row` when it computes the report over one, and the repair is a new run
without that row's mutation work.

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
and makes no edit on this call. Pass that whole list, unchanged, as `sites` on the row's first
`evaluate-mutation` call in §3 below — the op registers it there, and the row cannot be
answered by fewer sites than it names.

If it returns `unappliable` with a reason instead of a list, record the row as

```json
{"rowId": "SC-07", "outcome": "unappliable", "reason": "<the agent's reason, verbatim>"}
```

and move on. Finalize turns that into `MUTATION-UNAPPLIABLE`, which blocks: it is a
coverage-design defect — the row's `Verification mutation` was phrased as a negated
expectation rather than a code-level fault. Report it as such and never let the mutator
invent a substitute. `finalize` publishes the record as `mutation-unappliable-<rowId>.json`.

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

   The request adds `runDir`, `site`, `lastSite`, and — on the row's first call for this run —
   `sites` (the mutator's full enumerated list, verbatim) to the shared body. The op runs the
   row's targeted test, requires it to fail, restores the source inside the worktree, reruns
   the same target, requires it to pass, and records both outcomes under the site label.
   Output: `site`, `outcome`, and `diff {file, hunks}`; on the call that completes the row it
   also returns the aggregated `sites[]` and the `evidencePath` of the row's mutation evidence.

The row completes — evidence written, worktree disposed, ledger record completed — exactly
when every site named in `sites` has been evaluated, whether or not that call also set
`lastSite: true`. `lastSite` is only an assertion: send `true` on what you believe is the row's
final site, and a `true` sent while another registered site still has no outcome is refused
`sites-remaining`, leaving the row started and the worktree untouched. Sending `false` or
omitting it on what turns out to be the last remaining site still completes the row.

The first call for a row with no registered list and no `sites` is refused `missing-sites`. A
`sites` value is refused `invalid-sites` when a label is empty, contains `/` or `\`, repeats
another label, or does not include `site` itself. `sites` sent on a later call is compared to
the row's registered list as a set — order does not matter — and refused
`enumeration-changed` if it disagrees; once registered, evaluating a `site` outside that list
is refused `unknown-site`. Every one of these refusals happens before any run or state
change, so a corrected retry costs nothing.

Finalize derives `MUTATION-CAUGHT` when every enumerated site's targeted test failed with the
mutation applied and passed again after restore, and `MUTATION-SURVIVED`, naming each
surviving site, otherwise. One survivor blocks; no number of caught sites elsewhere offsets it.
Fresh mutation evidence that carries `enumeratedSites` is refused `mutation-sites-incomplete`
if its own evaluated sites do not match that list; evidence written before this field existed,
or evidence a run reused from an earlier one, is accepted as before.

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
`mutant-build-failed` is not in this list — see below. Record the actual code and message
verbatim in `refusal`, using the shared request plus `layer: 4` and this `rowID`:

```text
go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier record-attempt -request -
```

The op returns a source/commit-stamped `evidencePath` and completes the row in the ledger with
outcome `environmental` in the same call; the record is in the ledger, finalize reads it —
never hand-write or re-stamp an attempt.

A completed `environmental` record with no attempt artifact is refused
`missing-attempt-evidence`, and the attempt document is loaded through `report.LoadAttempt`,
which refuses anything without the envelope. The row becomes `MUTATION-N/A (environmental)`
and the run yields `PARTIAL`.

A mutated-phase run that never compiled is refused `mutant-build-failed` instead of
`zero-selection`, quoting the compiler detail, and leaves the `Started` marker in place
(`Phase: "mutated"`) exactly as any other refusal does. Run `recover-mutation` to restore the
worktree, then dispatch `mutator` once more for the same site, giving it the compiler output.
A second `mutant-build-failed` on that same site means the mutation cannot be applied: record
the row `unappliable` through `record-mutation`, exactly once, with the compiler output as
`reason` — the same one-entry-per-row rule the confirmation failure above follows. A
clean-phase build failure is unrelated and still surfaces as `clean-run-failed`.

On the JVM side there is no separate build-failure code: a mutated run is checked for a
compiler error in the runner output before anything is recorded, and that surfaces as
`normalization-failed`.

### Restart

If a run is picked back up, `resume` names every started mutation row with its
`completedSites` and its `interruptedSite` — read [run state](run-state.md) in full before
repeating any op here. `evaluate-mutation` refuses `site-interrupted` for that row until
`recover-mutation` has restored or recreated its workspace: nothing is measured on a worktree
of unknown cleanliness. Completed sites are kept either way — recovery never re-measures a
site the row already answered. `recover-mutation` never touches the row's registered `sites`
either: resending a different list afterward is still refused `enumeration-changed`, and the
label `interruptedSite` names stays a member of it. A refused recovery leaves the row exactly
where it was: pending, and the abandoned worktree in place, not removed on your account.
