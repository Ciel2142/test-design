## Finalize

Assemble one request from the four layers:

```json
{
  "schemaVersion": "1",
  "repoRoot": "<captured absolute TARGET_REPO>",
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
row is `smoke` or `e2e`, or all applicable rows are layer-1 gaps/ambiguities.
In the latter case finalize derives per-row N/A and retains applicability accounting. `layer4.rows[]` holds one entry per row layer 4 covered.

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
3. **Preserve the scratch artifacts and remove only this publication from the target.**
   Keep the returned `runDir` (including canonical report JSON/Markdown) and the original
   trace/live/mutation/attempt paths used by the request. The first publication dirtied
   the tree. From its `jsonPath`, `markdownPath`, and canonical report's `transcripts[]`
   paths, identify the exact generated files. Restore only those files already tracked
   at the measured HEAD; remove only those generated files that were untracked there.
   Never delete a whole transcripts directory or discard unrelated/user changes.
   If publication was edited or provenance is uncertain, stop rather than remove it.
4. **Keep HEAD unchanged and re-run `finalize`** with the original request and evidence
   paths plus `acknowledgement` set to the user's actual reply, verbatim. The repository
   must be clean again. The report is republished with acknowledgement; do not promise
   byte identity for regenerated timestamps. Commit only after this acknowledged report.

Committing the first report moves HEAD and makes its trace stale: it is **not** an
acknowledgement recovery path. If HEAD moved, preserve the artifacts and start all four
layers with a new run ID and approval. No dirty-tree bypass is available.

## Refusals

Every code below is a refusal the executable can return to this thread. Read the code first;
the message names the specifics.

| Code | Where | What this thread does next |
|---|---|---|
| `unsupported-schema-version` | any op | The request's `schemaVersion` is not `"1"`. Fix the request; this is a defect in the document this thread built, never in the repository |
| `source-not-reproducible` | `trace`, `prepare-live`, `execute-live`, `evaluate-mutation`, `finalize` | The working tree is dirty. Resolve only the changes the message names. If any commit/source changes, restart with fresh approval and run ID. For this run's generated `PARTIAL` publication use the exact-artifact recovery above, keeping HEAD unchanged |
| `dirty-repository` | `baseline`, `prepare-mutation` | Same condition, named at the ops that must start from a pristine tree. Commit or discard, then re-run. Never work around it |
| `snapshot-mismatch` | `execute-live`, `evaluate-mutation`, `finalize` | The repository changed while the op was running. This does **not** claim corruption: commit or discard the concurrent change, then re-run the run from a fresh run ID |
| `source-changed-during-verification` | `trace` | The same condition seen at the end of the trace run. It does **not** claim corruption. Commit or discard the concurrent change and re-run |
| `head-moved` | `finalize` | HEAD is no longer the commit the trace ran against. A verification describes one commit: mint a new run ID and repeat every layer at the current HEAD |
| `stale-evidence` (run-ID mismatch) | `finalize` | Some evidence or batch names a different run ID than the request. A bookkeeping defect in the request this thread assembled — fix the request document, never the evidence, and never re-stamp a run ID |
| `stale-evidence` (source hash mismatch) | `finalize` | An evidence document's `sourceSha256` does not match the coverage source's current hash. The coverage source changed mid-run — mint a new run ID and repeat every layer against the source as it now reads |
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
is not `bound`; row-less layer-1 findings — `drift` and `overclaim` on tags that cite no
active row; a layer-2 `WEAK`, `VACUOUS`, or `MISSING`; a layer-3 `LIVE-FAIL` or
`LIVE-INCONCLUSIVE`; a repository that changed during the live or mutation run;
`invalid-verdict` at layer 2, 3, or 4, when a row's verdict value falls outside that layer's
closed set; and a layer-4 `MUTATION-SURVIVED` or `MUTATION-UNAPPLIABLE`.

**`PARTIAL`** — no blocking reason, but at least one layer could not deliver a full answer.
The executable's causes, exhaustively: `baseline-<status>` when `baselineStatus` was not `ok`;
`live-profile-unavailable`; `live-cleanup-failed` and `live-cleanup-missing`;
`live-execution-errors`; `critic-canary-unavailable`; `redaction-barrier` when a live transcript could not be reduced
without redacting evidence; and `mutation-environmental`.

A `PARTIAL` is not final until it is acknowledged. The published report says `Acknowledgement
pending` on its face until the acknowledgement re-run replaces it.

**`PASS`** — no reason at either severity. Every applicable row carried a verdict from every
applicable layer, and every non-applicable layer was `n/a` for a reason the report names.

## Degradation

| Situation | What the run does |
|---|---|
| No active `smoke`/`e2e` row | `prepare-live` refuses `no-applicable-rows`; `layer3` is absent from the finalize request and the report's layer 3 is `n/a` |
| Every applicable live row is a layer-1 gap | Skip prepare/planning; omit `layer3`; finalize derives N/A with the applicable count named |
| Live profile missing or invalid | `layer3.attemptEvidencePath` from `live-attempt.json`; reason `live-profile-unavailable` → `PARTIAL` |
| Live cleanup failed, or recorded no success | `live-cleanup-failed` / `live-cleanup-missing` → `PARTIAL` |
| A live transcript cannot be reduced without redacting evidence | `redaction-barrier` on that row → `PARTIAL` |
| Baseline evidence absent or unverifiable | `trace` reports `baselineStatus` `missing` or `invalid: …`; the run continues; `baseline-<status>` → `PARTIAL` |
| `evaluate-mutation` refuses for environmental reasons | Row recorded `environmental` with an attempt document; `mutation-environmental` → `PARTIAL` |
| The row's `Verification mutation` cannot be applied as a source edit | Row recorded `unappliable`; `MUTATION-UNAPPLIABLE` → `BLOCKED`, and the repair is a coverage-source amendment |
| Row is a layer-1 gap | No layer-3 or layer-4 dispatch; finalize derives `LIVE-N/A (layer 1 gap)` and `MUTATION-N/A (layer 1 gap)` |
| Coverage source missing | Do not invoke; the feature did not opt in |

## Artifact

`finalize` publishes into the coverage source's own directory, named from its stem. Both conventional dedicated coverage sources and approved
inline floor-rule sources are accepted by the runtime:

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
[example-verification.md](example-verification.md) — the exact output of
`report.RenderMarkdown` over a `BLOCKED` run.


## Evidence compatibility

Current generated evidence uses verifierVersion `"1.1.0"` and schemaVersion `"1"`.
Live/mutation evidence and attempts require `repoCommit`, `source`, and `sourceSha256`
as well as their existing run/row identity spellings. Old unbound evidence is refused;
regenerate it in a newly approved full run. Never infer missing fields or hand-stamp old
JSON. Existing baseline schema remains usable; this is not a baseline migration.
