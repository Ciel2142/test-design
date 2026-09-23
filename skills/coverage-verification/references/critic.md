## Layer 2 — adversarial read

Dispatch [coverage-critic](../../../agents/coverage-critic.md), one agent per behavior group, groups in parallel. Give each: the
group's rows, **each row's test source pasted inline** with its file path, and **the
production diff with test files excluded**.

### The paste comes from dispatchPath, never from a search

Use the immutable dispatch payload read in layer 1: each row's `testSource` is the
exact unique bound range (`file`, `line`, `endLine`), with `testSourceSha256` identifying
those bytes. Paste it directly; never re-search by name or reread disk to replace it.
Use `behavior`, `expected`, `verificationMutation`, `level`, `risk` and `status` from
that same payload. A gap or ambiguity has no fabricated source: dispatch it anyway
with `no source found`, optionally naming the recorded binding site. The critic can
then return `MISSING`, which agrees with layer 1 rather than hiding an untested row.

**Paste it verbatim and complete** — the whole test body, every assertion, unedited. A
dispatcher that drops the binding assertion earns a `VACUOUS` on a healthy row.

When a test genuinely will not fit, elide **only** code outside the bound range — shared
setup, fixture builders, imports — and name each cut inline where it was made:
`// elided: setUp at OrderServiceTest.java:12-40`. Never elide an assertion, nor an arrange
step the assertions read from. If the body alone still will not fit, dispatch that row by
itself rather than trimming it.

A critic reason naming a truncated or unmarked-incomplete paste is a **dispatch defect**:
re-paste that row complete and re-dispatch it, and do not record the verdict it came with.

Excluding test files from the diff is not tidiness. Layer 2's question — would this row's
pasted test fail if its verification mutation were applied to the production code — is a
claim about production code alone. The test source needed to answer it is already pasted in
full and verbatim from the dispatch payload; test files in the diff add nothing to answering
that question.

Withholding the diff would leave the critic unable to answer its own question, since "would
this fail if the mutation were applied" is a claim about production code. Its honesty comes
from having no edit tools, not from having no information.

### The bundle and the manifest

`record-review started` takes `policy`: the critic's name and the absolute paths of the
instruction files the parent dispatches it with. It returns `bundle` — the whole test files
the group's rows are pasted from, each with the rows it serves — captured from what is on
disk at HEAD, never from a digest the parent supplies.

The critic's own `reads:` line (see the agent's output format) is relayed at completion as
`reads` on `record-review completed`. An explicitly empty list says the critic read nothing
beyond its pasted bundle; leaving the field out says nobody recorded what it read at all —
the difference between a manifest a later run can reuse and one it cannot.

A manifest is incomplete for any of: `policy-unrecorded` (no `policy` given), `policy-path-
not-absolute:<path>` or `policy-unreadable:<path>` (a bad instruction path), `head-moved` or
`tree-dirty:<path>` (the repository moved under the group), `reads-unrecorded` (no `reads`
relayed at completion), or `unidentifiable-read:<path>` (a read outside the repository,
untracked at HEAD, or escaping it with `..`). None of this blocks the run: a review's verdict
stands whatever its manifest says. The only consequence of an incomplete manifest is that the
group cannot be adopted by a later `reuse`.

### Reuse across runs

Before dispatching any critic, run `test-design-verifier reuse` once — after `trace`, before
the first batch — with `priorRunId` naming the run to stand on and the same `policy` this
run's critics will dispatch under. Read its `review[]` and `mutation[]` decisions and its
`changedPaths[]`; dispatch only the rows `resume` still lists `pending` — a record `reuse`
adopted needs no critic, and one it refused is owed exactly like any other pending
obligation.

`changedPaths[]` classes every tracked path that differs since the prior commit:

- `exempt` — the coverage source's own directory, where a run publishes its report and
  transcripts.
- `isolated` — a test file exactly one prior group's bundle names, absent from every other
  prior group's bundle or reads.
- `shared` — a production, build, dependency, data, or documentation change outside the
  coverage directory, or a test file more than one prior context names, or a test file that
  appears in exactly one group's reads but in no group's bundle.
- `unknown` — a test file no prior manifest names at all.

Ownership is read from every prior group's recorded manifest, including a group that was
started but never completed or whose artifact no longer verifies: a change to a file such a
group was handed is still attributed to it, even though the group itself cannot be adopted.

A `shared` or `unknown` path refuses every prior review group and mutation row; an `isolated`
one refuses only its own group and that group's mutation rows.

An adopted record keeps the prior outcome, failure included: a failed group or row comes back
failed, and asking for it again is a `repeat`, carrying `repeat.reason`, exactly like
repeating any other completed obligation — reuse never erases a prior failure by silently
re-asking it. `trace` and the live attempt are never reuse candidates: they always dispatch
fresh against this run's own commit, whatever `reuse` adopted. A mutation row is compared
against the toolchain of the run that actually executed it — the trace the prior run recorded
immediately before the row, not its latest — never the toolchain of the review whose manifest
it borrows as context. A record the prior run itself adopted is judged against the manifest it
carries from its original producer, and the reference it earns names that producer
(`priorRunId`, `priorSeq`, artifact) while `basis.priorCommit` names the run it was compared
against and the ledger detail names the run it was taken from.

`reuse` reads the repository the way `trace` and `finalize` do: an uncommitted change refuses
the whole call `source-not-reproducible` before any record is judged.

`reuse` refuses per record, never the whole call. One line per reason:

- `missing-prior-run-id`, `prior-run-is-current` — the request itself is malformed.
- `trace-incomplete`, `stale-evidence` — this run has no completed, verified trace yet, or its
  identity moved since.
- `prior-ledger-missing`, `prior-ledger-invalid` — the named run has no ledger, or it will not
  load.
- `prior-source-mismatch`, `prior-commit-unreachable` — the prior run verified a different
  document, or names a commit this repository does not have or cannot diff against HEAD.
- `prior-not-completed` — the prior record never finished.
- `legacy-no-manifest` — the record predates manifest capture; never reconstructed.
- `record-unreadable` — the record's captured inputs will not decode; nothing about it can be
  told, so it is neither legacy nor adoptable.
- `artifact-missing:evidence`, `artifact-unreadable:evidence`,
  `artifact-digest-mismatch:evidence` — the prior artifact is missing, will not decode, or no
  longer digests to what the ledger recorded.
- `manifest-version-unsupported` — the manifest's own version is not one this run reads.
- `manifest-incomplete:<reason>` — the manifest recorded one of its own incompleteness
  reasons.
- `row-inactive:<row>` — the row is retired, renumbered, or absent from this run's trace.
- `module-changed:<path>`, `unknown-ownership:<path>` — the `shared`/`unknown` widening above.
- `isolated-change:<path>` — the group's own bundle changed.
- `claim-missing:<row>`, `claim-changed:<row>` — the row's approved claim is unreadable now,
  or reads differently.
- `context-missing:<path>`, `context-changed:<path>` — a bundled or read file is unreadable
  now, or reads differently.
- `runner-changed`, `toolchain-changed` — the runner identity differs (review and mutation),
  or the toolchain does (mutation only).
- `toolchain-unrecorded` — the mutation row's own executing toolchain cannot be established:
  the row carries no adopted manifest, the trace record preceding it in the prior ledger is
  missing or no longer verifies, or either that toolchain or this run's own is empty — two
  failed version probes are not the same toolchain (mutation only).
- `policy-unspecified`, `policy-changed` — this run named no `policy` for review, or the
  critic or its instructions differ from the prior review's (review only).
- `policy-path-not-absolute:<path>`, `policy-unreadable:<path>` — this run's own
  `policy.instructions` names a relative or unreadable file, so nothing could be compared
  against it (review only).
- `sources-changed:<row>` — this run's trace binds the row to different per-row sources than
  the group was judged against (review only).
- `status-changed:<row>` — this run's trace binds the row to a different status than the
  prior run's trace did; compared only when the prior run's own trace document is still on
  disk and still verifies, so a cleaned run directory or a repeated trace leaves the question
  unasked rather than refused (review only).
- `identity-unrecorded` — a mutation row was completed without ever being prepared against a
  bound test (a `record-attempt` with no `prepare-mutation`), so it has no identity to compare
  (mutation only).
- `identity-changed` — a mutation row's own test identity differs from what this run's trace
  now binds (mutation only).
- `no-review-context` — no prior current review group both verifies and completely covers the
  row (mutation only).
- `already-recorded`, `row-already-grouped:<row>` — this run's own ledger already holds the
  record, or the row under another group.

### Bookkeeping

Every batch is recorded through `record-review` (see [run state](run-state.md)); `finalize`
reads it from the ledger as the group's layer-2 batch. The parent never pastes batches.

Verdicts are `HOLDS`, `WEAK`, `VACUOUS`, or `MISSING`; anything else is refused
`invalid-verdict`. `WEAK`, `VACUOUS`, and `MISSING` all block.

### What finalize enforces

- **Exactly one verdict per active row.** None is `missing-verdict`; two is
  `duplicate-verdict`.
- **A layer-1 `gap` or `ambiguity` row must carry `MISSING`.** Any other verdict for it is
  `missing-verdict`.
- **`runId` equals this run's ID** on every batch, or `stale-evidence`.
- **Every row named anywhere in a batch is an active row** of the source, or `unknown-row`.

### Reconciling `MISSING`

`MISSING` means the row's tagged test does not exist. On a row layer 1 already reported as
`gap` or `ambiguity`, that is the expected verdict and the two layers agree. On a row layer 1
reported as bound, they contradict each other: consult that row's `testSource` and binding metadata in
the immutable dispatch payload. A nonempty bound source means a dispatch defect —
re-paste that exact payload and re-dispatch that row rather than recording the `MISSING`.

### Suspect rows from calibration

`finalize`'s report carries a `calibration` block comparing each high-risk row's layer-2
verdict against its layer-4 outcome. When a batch held `HOLDS` on a row whose mutation then
survived, calibration names that row under `holdsSurvived` and names every other row the same
batch also verdicted `HOLDS` under `suspectRows` — the batch's read of this behavior group is
now in doubt, and those rows never got their own mutation to confirm it independently.

Do not let a suspect row's prior `HOLDS` stand on `reuse`'s say-so alone. A new group ID
naming the suspect row is refused `row-already-grouped`: the calibration block names only
rows, never a group ID, so find the suspect row's batch in the report's rows table — its
`Batch` column names the group that verdicted it — and reuse that same `groupId`. Start it
again through `record-review` (`status: started`) with the suspect row alone in `rows` and
`repeat` carrying a `reason` naming the calibration finding — for example, "batch batch-2 held
SC-02 HOLDS and its mutation survived; re-confirming SC-03 independently" — then dispatch a
fresh critic for it and complete the group as usual. That `Start` supersedes the whole prior
group, not only the suspect row: the batch's other rows, no longer claimed by any current
group, sit uncovered until a later `record-review started` regroups them, over this `groupId`
or a new one.

