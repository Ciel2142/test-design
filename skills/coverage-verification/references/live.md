## Layer 3 — live

Layer 3 covers the rows the coverage source's `## Live decision` makes applicable: with
`decision: required`, every active row a journey names plus every active `smoke` or `e2e`
row; with `decision: not-applicable`, none. A journeyed `unit`, `component` or
`integration` row is as applicable as a `smoke` row and keeps the level it was written
with. Read the live decision from the source, and the row facts from the trace dispatch
payload, **before** preparing or planning. Omit `layer3` only when `prepare-live` refuses
`no-applicable-rows`; every other applicable row is planned, run and judged, the layer-1
gap/ambiguity rows among them.

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

- `no-applicable-rows` — the live decision is `not-applicable`. Omit `layer3` from
  the finalize request entirely; the report says layer 3 is `n/a`, not skipped and not
  partial.
- `profile-unavailable` — the profile is missing, unreadable, or fails validation. The op has
  **already written** `live-attempt.json` into the run dir and its message names the path.
  Take that path as `layer3.attemptEvidencePath` and continue the run; finalize records
  `live-profile-unavailable` and the verdict becomes `PARTIAL`.

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

  Use its `evidencePath` as `layer3.attemptEvidencePath` and continue to mutation.
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

Read `evidencePath` — it is `layer3.evidencePath` in the finalize request — plus
`rowVerdicts`, `cleanupOK`, and `cleanupFailed` for the running summary. Every applicable row
carries `LIVE-PASS`, `LIVE-FAIL`, or `LIVE-INCONCLUSIVE`; a gap row gets the journey's
truthful outcome like any other, with `layer1Gap: true` recorded beside its verdict. The
layer-1 gap still blocks the branch on layer 1's own account.

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

