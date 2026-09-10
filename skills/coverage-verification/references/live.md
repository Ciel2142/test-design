## Layer 3 — live

Layer 3 covers the rows the coverage source's `## Live decision` makes applicable: with
`decision: required`, every active row a journey names plus every active `smoke` or `e2e`
row; with `decision: not-applicable`, none. A journeyed `unit`, `component` or
`integration` row is as applicable as a `smoke` row and keeps the level it was written
with. Read the live decision from the source, and the row facts from the trace dispatch
payload, **before** preparing or planning. Layer 3 is skipped entirely only when
`prepare-live` refuses `no-applicable-rows`; every other applicable row is planned, run and
judged, the layer-1 gap/ambiguity rows among them.

When reviewing a profile's schema or limits, **read [live profiles](../../../docs/live-profile.md)**.
Optional `timeoutSeconds` defaults to 120, `cleanupTimeoutSeconds` to 30, and
`maxOutputBytes` to 16777216; zero selects the default and negatives are refused.
Cleanup has its own deadline; timeout or output exhaustion is not passing evidence.

### 1. Prepare

```text
go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier prepare-live -request -
```

with the shared request body. On success it returns `runID`, `worktreePath`, `runDir`,
`profilePath`, and `descriptor` — the live profile with `{run-id}` substituted throughout.

`prepare-live` validates the source's `## Live decision` before it decides applicability: a
missing or invalid section is `live-decision-incomplete`, and no worktree or run dir is
created. That is a defect in the
coverage source, not an outcome — amend the source and rerun.

Two refusals are outcomes rather than errors:

- `no-applicable-rows` — the live decision is `not-applicable`. Nothing is recorded for
  layer 3 at all; continue straight to mutation. The report reads layer 3 as `n/a` from the
  coverage source's live decision, not skipped and not partial.
- `profile-unavailable` — the profile is missing, unreadable, or fails validation.
  `prepare-live` now records that attempt itself, completing `live-attempt/live` with outcome
  `unavailable` before it creates a run dir or worktree — refusing `live-attempt-completed`
  first when a completed record already stands without `repeat.reason`. The record is in the
  ledger; finalize reads it. Continue the run; finalize records `live-profile-unavailable` and
  the verdict becomes `PARTIAL`.

### 2. Dispatch [live-runner](../../../agents/live-runner.md)

Give it the `worktreePath`, the `descriptor` and its `profilePath`, the journeys the live
decision names (id, scenarios, expected), the applicable row facts with their levels from
`dispatchPath`, the run ID, and the layer-1 gap set read from the trace evidence's findings,
marked "bind these too; they have no automated test".

Its entire final message is a `live.Plan` JSON document — no prose, no fence. Write that
message to `<runDir>/plan.json` byte-for-byte unchanged. Do not repair a plan by hand: the
plan is the agent's product, and editing it makes the evidence describe a run nobody
designed.

Two other replies are legitimate:

- `N/A` — legitimate only when the live decision is `not-applicable`, and that case never
  reaches this dispatch: `prepare-live` refuses `no-applicable-rows` first. If you handed
  the agent an applicable row, the reply contradicts it: re-dispatch quoting those row
  facts, never omit their evidence.
- A refusal naming the row whose recipe cannot be expressed through declared commands.
  This ends the **whole live layer**, not just that row: per-row continuation is not
  supported. Record the actual reply verbatim using the shared request plus `layer: 3`
  and nonempty `refusal`:

  ```text
  go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier record-attempt -request -
  ```

  The record is in the ledger; finalize reads it. Continue to mutation.
  The executable stamps the source/commit/run identity; never hand-write an envelope,
  invent a refusal, or report the other live rows as executed. Retain the prepared
  `worktreePath` and `runDir` for controller-owned disposal if preparation already ran.

### 3. Execute

```text
go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier execute-live -request -
```

The request adds three fields to the shared body: `runDir`, `plan` (the absolute path to
`<runDir>/plan.json`), and `traceEvidence` (the absolute `evidencePath` from layer 1). The op
validates the plan against the profile, starts the environment, runs the steps, reduces the
transcripts, runs cleanup unconditionally, disposes the worktree, and writes
`live-evidence.json`.

Read `evidencePath` — the record is in the ledger; finalize reads it — plus `rowVerdicts`,
`unboundFailures`, `cleanupOK`, and `cleanupFailed` for the running summary. Every applicable row
carries `LIVE-PASS`, `LIVE-FAIL`, or `LIVE-INCONCLUSIVE`; a gap row gets the journey's
truthful outcome like any other, with `layer1Gap: true` recorded beside its verdict. The
layer-1 gap still blocks the branch on layer 1's own account. `cleanupOK` and `cleanupFailed`
are not a strict inverse — see "Outcomes" below for the third state neither covers.

**The reduced transcripts inside the evidence are the record; the agent's summary is not.**
Nothing here needs re-checking by hand: the verdicts come from the op that ran the commands,
not from the agent that proposed them.

A plan that fails validation is refused by name — `shell-invocation-rejected`,
`undeclared-command`, `undeclared-endpoint`, `inexpressible-predicate`,
`missing-row-predicates`, `unknown-row`, `unbound-row`. A plan `DecodePlan` cannot read at
all — malformed JSON, a `schemaVersion` other than `"1"`, no steps, or a step missing its
`expectation` or `command` — is `plan-decode-failed`, quoting the decoder's own message. Each
is a defect in the agent's plan, not in the repository: re-dispatch `live-runner` quoting the
refusal message so it designs to the rule it broke.

### Outcomes

The evidence's JSON names are the contract; read them by field, not by summary:

- `rowVerdicts[].verdict` is the worst outcome among a row's bound steps: `LIVE-PASS`,
  `LIVE-FAIL`, or `LIVE-INCONCLUSIVE` when a bound step never completed or the walkthrough never
  reached a step the row binds. Earlier and later failures merge the same way regardless of
  order: once a row is `LIVE-FAIL` a later `LIVE-PASS` on another bound step cannot clear it. A
  health phase that passes proves readiness only and never yields a row verdict by itself; plan
  steps run only when both `start` and `health` passed, so a bound row no completed plan step
  reached is `LIVE-INCONCLUSIVE`.
- `unboundFailures` lists every completed command whose predicates failed but that reached no row
  verdict: a `start` or `health` command, a plan `step` with no `scBindings`, a `plan-cleanup` step
  with no `scBindings`, or a `plan-cleanup` step whose bindings could not apply because `start` or
  `health` failed and the walkthrough never ran. Each entry names `phase`
  (`start | health | step | plan-cleanup`), a 1-based `index` within that phase, and
  `expectation` — the planner's own text, passed through the same redaction barrier as a
  transcript and replaced with the literal `[withheld by redaction barrier]` on a hit. The array
  is always present, empty when there is nothing to report.
- `cleanupOK`, `cleanupFailed`, and neither are a tri-state over the profile's own `cleanup`
  commands only — a plan's `cleanup` steps are judged like any other plan step, bound or
  unbound, per the `unboundFailures` bullet above.
  `cleanupFailed` is true the moment any profile cleanup command completes with a known failure —
  a nonzero exit, or the configured last command completing with its `cleanupEvidence` unheld —
  and that failure is never cleared by a later command succeeding or masked by another command
  failing to complete. `cleanupOK` is true only when every command completed and the last one's
  `cleanupEvidence` held; it is mutually exclusive with `cleanupFailed`. Neither flag is true when
  no completed command failed but at least one never completed (a deadline, a cancellation, the
  output cap, or a capture error): its cause lives in `executionErrors`, and cleanup is unproven
  rather than vindicated.
- `executionErrors` mixes three causes, not one: a command that did not complete (timeout,
  signal, cancellation, output-cap overflow, or a capture error) as `<phase>[<index>]: <cause>`;
  the raw stdout/stderr/combined output of any command that started — whether or not it went on
  to complete — that could not be SHA-256'd, as `hash <path>: <err>` (this can co-occur with that
  same step's own non-completion entry; it does not imply the step has a result); and a completed
  command whose transcript the redaction barrier withheld, also as `<phase>[<index>]: <cause>`.
  The first and third forms differ in what else the run records. A step that did not complete has
  no reduced entry in its phase's step list and no `unboundFailures` entry: a non-completion is
  not a completed, failed predicate. A step whose transcript was withheld has no reduced entry
  either, but it did complete and its predicates were still judged: it settles the rows it binds
  as any other step would, and an unbound one whose predicates failed is listed in
  `unboundFailures` — carrying `[withheld by redaction barrier]` in place of its `expectation`
  when that text is what the barrier caught, and the planner's own text when the value was in the
  command or the output instead.
- Phase results — `startSteps[].result`, `healthSteps[].result`, and the profile-cleanup entries
  inside `cleanupSteps[].result` — are `pass` iff the command completed with exit 0 (start,
  health, and every profile cleanup command carry this predicate implicitly, and the configured
  last cleanup command additionally carries `cleanupEvidence`); a completed nonzero exit is
  `fail`, and a command that never completed has no reduced step at all.

Finalize turns these into layer-3 reasons: a non-empty `unboundFailures` yields
`live-unbound-assertion-failed` (BLOCKED), detailing every `phase[index] expectation` entry;
`cleanupFailed` yields `live-cleanup-failed` and the neither-state yields `live-cleanup-missing`,
both PARTIAL; a non-empty `executionErrors` yields `live-execution-errors` (PARTIAL); and a row
verdict of `LIVE-FAIL` or `LIVE-INCONCLUSIVE` is BLOCKED for that row. These are independent —
several can fire on the same run — and none of them can turn a run's verdict into a pass.

**Restart.** A prior live-attempt record for this run — completed or left started by an
interruption — makes `execute-live` run the profile's own `cleanup` commands as recovery
before any `start` command: the resources that attempt owns are exactly what those commands
remove, and a fresh start over a half-live environment would judge rows against something
nobody designed. `recovery` — `{performed, ok, steps}` in both the evidence and the output —
is absent from a first attempt and present on every later one; `performed` is always `true`
there, `ok` says whether the commands proved removal, and `steps` carries their reduced
transcripts. A failed recovery is a completed attempt with ledger outcome `inconclusive`: no
fresh start at all — no `start`, `health`, plan or plan-cleanup command runs — every
applicable row is `LIVE-INCONCLUSIVE`, the normal end-of-run cleanup is skipped, and
`executionErrors` gains a `recovery` entry naming the cause. A completed attempt, by either
path, is refused `live-attempt-completed` without `repeat.reason`; with one, recovery runs
again first. Prefer `execute-live` (which recovers) over a layer-3 `record-attempt` while the
profile is still usable: `record-attempt` completes the layer `unavailable` without running
recovery at all, leaving the environment exactly as the interruption left it. See
[run state](run-state.md) for the ledger mechanics.

Artifact identity has no dedicated evidence field: the deploy recipe stamps and checks the
repository revision itself, refusing to run against a stale checkout on its own account, and the
verifier only preserves what the recipe printed — each step's `decisiveOutput` carries the
recipe's own deploy line, readiness line, or matched business line, with context, whichever the
step's predicates picked out. The recipe's lines are the record; the verifier adds no
interpretation of its own.

