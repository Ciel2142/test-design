---
name: coverage-verification
disable-model-invocation: true
description: Use only at the finishing-a-development-branch handoff when an explicit test-coverage-design opt-in produced approved SC rows
---

# Coverage Verification

Measure an explicitly approved coverage source across four layers. Each layer's
self-report is checked by something other than itself. The executable owns snapshots,
worktrees, real runners, structural binding, live execution, mutation restore checks,
evidence, and verdicts. This thread owns approval, production-diff freshness, critic
canaries and dispatch, and the semantic check of each mutation diff.

**Announce:** "I'm using the coverage-verification skill to verify coverage for this feature."
**When:** only at the `superpowers:finishing-a-development-branch` handoff, after final
review inside `superpowers:subagent-driven-development`, for an explicit design opt-in.
No approved source means no invocation or report: leave normal finishing untouched.

## Invocation and target identity

```text
/test-design:coverage-verification <coverage-source>
```

The source is a repo-relative dedicated coverage document or an approved spec with an
inline floor-rule SC table and Verification block. Go 1.26+ is required; no committed
binary or separate build step is needed. Before any `go run -C`, capture the **absolute
target repository** while still in that repository (not the plugin checkout):

```bash
TARGET_REPO=$(git rev-parse --show-toplevel)
```

Keep that captured value throughout this run. Every request uses its literal absolute
value as `repoRoot`; every `-repo` uses `"$TARGET_REPO"`. `-C` changes the process directory
to the plugin, so a relative repo root would verify the plugin instead of the target.
The common request is:

```json
{"schemaVersion":"1","repoRoot":"<captured absolute TARGET_REPO>","coverageSource":"<repo-relative source>","runID":"<run id>"}
```

Every request-body op uses `go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier <op> -request -`
with that JSON on stdin and phase-specific extra fields. `preflight` and `next-id` take
flags instead. Success is JSON on stdout; refusal is `{code,message}` on stderr, exit 1.
Never retry an unclassified refusal: read [finalize/recovery](references/finalize-recovery.md).

## Approval gate — stop here first

Only `preflight` may run before approval: it parses/validates/counts but runs no suite,
opens no worktree, and dispatches nothing. Pass the source relative to the target root:

```text
go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier preflight -source <coverage-source> -repo "$TARGET_REPO"
```

A refusal ends the invocation: report code and message verbatim; do not present a gate.
On success ask the user, on an actual user turn, to approve the run. State the branch
(read with `git -C "$TARGET_REPO" rev-parse --abbrev-ref HEAD`) and these preflight fields:
`source`, `sourceSha256`, `runner.kind/root/command/reportGlobs`, `layers.layer1` through
`layer4`, `highRiskCount`, `liveProfile` when required, and the three subagents
`coverage-critic`, `live-runner`, `mutator`. No invented values.

Anything except an actual go-ahead ends the run; silence is not approval. Preserve the
reply verbatim as finalize's `approval`. Mint `run-YYYY-MM-DD-nnnn` only at approval.
A rerun after a fix requires another gate, a new run ID, and all four layers.

## State router — mandatory reads at each phase

| State | Required action before proceeding | Next |
|---|---|---|
| Restart or resume | Run `resume` and read [run state](references/run-state.md) in full before repeating any op | Wherever `resume` reports work still owed |
| Approved | Run trace below; consume its immutable dispatch payload; classify freshness | Critic |
| Critic | **Read [critic phase](references/critic.md) in full before dispatch** | Live |
| Live | **Read [live phase](references/live.md) in full before applicability/prepare/dispatch** | Mutation |
| Mutation | **Read [mutation phase](references/mutation.md) in full before any mutator dispatch** | Finalize |
| Finalize or refusal/recovery | **Read [finalize/recovery](references/finalize-recovery.md) in full before publishing or retrying** | Finish only on PASS or acknowledged PARTIAL |

Do not eagerly load the later phases. A linked phase is mandatory when its state is reached,
not optional background. No phase's completion is itself permission to finish the branch.

## Layer 1 — trace and freshness

```text
go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier trace -request -
```

Trace snapshots the target, runs its declared suite in a disposable HEAD worktree, binds
`covers:` structurally (Go AST; Java/Kotlin token scanner), joins normalized real runner
records, compares baseline, disposes the worktree and verifies the original snapshot.

Read `evidencePath` (traceEvidence for later ops), `dispatchPath`, `runDir`, `repoCommit`,
`baselineStatus`, `findingCounts`, and `rowAccounting {activeRows,withResult,unaccounted}`.
A baseline other than `ok` does not stop later layers; finalize accounts for it as PARTIAL.

Read the `dispatchPath` JSON once. Its envelope carries `runId`, `repoCommit`, `source`,
`sourceSha256`; each row carries `rowId`, `behavior`, `expected`, `level`, `risk`,
`verificationMutation`, `status`, `scope`, `testName`, `file`, `line`, `endLine`,
`testSource`, `testSourceSha256`. Use these facts for all agent dispatches. The executable
read each uniquely bound range once: paste `testSource` exactly, including tag and all
assertions. Do not reread/re-search tests or reconstruct rows from the coverage source.
Gap/ambiguous rows have no fabricated source; dispatch `no source found` for them.

Findings: `gap` = untagged/not run; `ambiguity` = not uniquely bindable/normalizable;
`drift` = unknown non-retired ID; `overclaim` = more than two active IDs on one test;
`red` = newly failed against baseline. Finalize can add `red-runner` for nonzero runner
exit without a normalized report. Use the executable's findings and row statuses unchanged.

Read the branch's **production-only diff** and compare every externally observable change
against dispatch rows' behavior groups. The parent supplies finalize's semantic freshness:

```json
{"result":"fresh","stale":[]}
```

or `{"result":"stale","stale":[{"hunk":"internal/orders/guard.go:41-58","behavior":"partial-refund path"}]}`.
Stale requires named hunks and behaviors and blocks; amend the approved source, not the verdict.

## Run-wide invariants

- One approved source, commit, source hash, and run ID across every layer; never re-stamp
  or combine evidence from different runs. Version 1.1.0 retains schemaVersion `"1"`;
  old live/mutation evidence or attempts missing identity must be regenerated, not inferred.
- All four layers remain required subject only to runtime-derived applicability. Gaps stay
  visible; a missing environment is an evidenced attempt, never a fabricated success.
- Every critic paste is complete and authoritative; only a synthetic canary's prompt copy
  is weakened. Never write it to disk. Exclude test files from the production diff.
- Every batch needs a caught canary, valid inheritance for a synthetic real-source retry,
  or an explicit canary-unavailable reason yielding PARTIAL. Natural canary verdicts count.
- Only the executable runs live commands and targeted tests, restores mutations and checks
  restore success. Parent must confirm the applied production diff is the row's named fault.
- Report the executable's verdict and reasons verbatim. PARTIAL needs a user acknowledgement
  after its published reasons; BLOCKED cannot finish. Never commit to reuse an old trace.

## Completion

Approval, all row/layer accounting, canary audit, real evidence, mutation semantic checks,
and freshness must reach finalize. Preserve the report and transcripts in the target repo.
PASS or acknowledged PARTIAL returns to `superpowers:finishing-a-development-branch`;
BLOCKED requires repair and a full freshly approved run. The final phase owns exact
publication, acknowledgement recovery, artifact paths and refusal handling.
