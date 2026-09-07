## Layer 3 — live

Layer 3 covers active `smoke` and `e2e` rows. Read applicability from the trace dispatch
payload **before** preparing or planning. With no applicable rows, omit `layer3`.
If every applicable row is a layer-1 gap/ambiguity, also omit `layer3`: do not call
prepare or dispatch a planner just to receive N/A. Finalize preserves the applicable
count and derives `LIVE-N/A (layer 1 gap)` for each such row. With a mix, dispatch only
bound rows plus the complete gap set; execution/finalize account for gaps themselves.

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

Two refusals are outcomes rather than errors:

- `no-applicable-rows` — the source has no active `smoke` or `e2e` row. Omit `layer3` from
  the finalize request entirely; the report says layer 3 is `n/a`, not skipped and not
  partial.
- `profile-unavailable` — the profile is missing, unreadable, or fails validation. The op has
  **already written** `live-attempt.json` into the run dir and its message names the path.
  Take that path as `layer3.attemptEvidencePath` and continue the run; finalize records
  `live-profile-unavailable` and the verdict becomes `PARTIAL`.

### 2. Dispatch [live-runner](../../../agents/live-runner.md)

Give it the `worktreePath`, the `descriptor` and its `profilePath`, the applicable smoke and
e2e row facts from `dispatchPath`, the run ID, and the layer-1 gap set read from the trace evidence's findings.

Its entire final message is a `live.Plan` JSON document — no prose, no fence. Write that
message to `<runDir>/plan.json` byte-for-byte unchanged. Do not repair a plan by hand: the
plan is the agent's product, and editing it makes the evidence describe a run nobody
designed.

Two other replies are legitimate:

- `N/A` — legitimate only if trace already proves no applicable bound row. That case
  should have skipped this dispatch. If trace contains a bound applicable row, the reply
  contradicts it: re-dispatch quoting those row facts, never omit their evidence.
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
`rowVerdicts`, `cleanupOK`, and `cleanupFailed` for the running summary. Row verdicts are
`LIVE-PASS`, `LIVE-FAIL`, `LIVE-INCONCLUSIVE`, and `LIVE-N/A (layer 1 gap)`, which the op
assigns to gap rows itself.

**The reduced transcripts inside the evidence are the record; the agent's summary is not.**
Nothing here needs re-checking by hand: the verdicts come from the op that ran the commands,
not from the agent that proposed them.

A plan that fails validation is refused by name — `empty`, `unsupported`,
`shell-invocation-rejected`, `undeclared-command`, `undeclared-endpoint`,
`inexpressible-predicate`, `missing-row-predicates`, `unknown-row`, `unbound-row`, `gap-row-bound`. Each is a defect in
the agent's plan, not in the repository: re-dispatch `live-runner` quoting the refusal message
so it designs to the rule it broke.

