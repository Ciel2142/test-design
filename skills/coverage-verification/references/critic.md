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

Excluding test files from the diff is not tidiness. At the finishing-a-development-branch
seam the branch diff normally *contains the new tests*, so a diff that included them would
hand the critic the real source of every row alongside the pasted one — naming the
synthesized canary below by inspection, no disk read required. Layer 2's question is about
production code; test files in the diff add nothing and cost the canary.

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

### The canary

**Inject one canary per batch:** a test you already know is weak. A batch that verdicts
**the** canary healthy has all its verdicts voided and re-dispatches at a higher effort
tier. Without this, an all-`HOLDS` critic passes forever after the one-time acceptance check.

A critic that reads nothing and sacrifices a row escapes when it lands on the canary — ~1/N
per dispatch attempt, N the batch's row count (coin flip at N=2). Voided batches retry at a
higher tier, compounding against a persistent critic. A second canary would buy 1/C(N,2) at
one extra dispatch per batch and the rules needed to keep two canaries from identifying each
other.

Source each canary in this order:

1. **A naturally weak test** — one a prior run verdicted `WEAK` or `VACUOUS` **on its real
   source**. Preferred: nothing is fabricated and the expected verdict is already known. Two
   exclusions. A `WEAK`/`VACUOUS` earned by a weakened *copy* does not qualify: reusing it
   pastes the row's healthy real source, expects weakness, gets `HOLDS`, and voids the batch
   forever. And a row whose layer-1 status is `gap` or `ambiguity` cannot serve either —
   finalize forces `MISSING` on those rows and refuses any other verdict for them, so there
   is no weakness verdict for such a row to earn.
2. **A synthesized one.** Pick a row and paste, in place of its real test source, a copy
   weakened so it no longer binds to that row's `Verification mutation` — the binding
   assertion loosened, replaced with a presence check, or dropped. The correct verdict for
   the paste is `WEAK` or `VACUOUS`; a healthy verdict voids the batch. That row has now been
   verdicted on a copy rather than on its real source, so re-dispatch **that row alone** with
   its real source to get its actual verdict. One extra single-row dispatch per *synthesized*
   canary — at most one per batch, none when the canary is natural, which is verdicted on its
   real source already. Never a second full batch.

   Vary the weakening. `coverage-critic` is told to hunt for exactly the forms named above, so
   a batch in which one row and only one row asserts mere presence identifies itself. Weaken by
   loosening a boundary, asserting a mock instead of state, dropping to a shape check —
   whatever the row's real assertion makes natural.

   If a row cannot be weakened without the copy still binding — a golden-file or snapshot
   test, where the binding is the fixture and not an inline assertion — pick a different row.
   If none can, record `canaryUnavailableReason` on that batch, explaining the
   snapshot/fixture binding that makes weakening unavailable. Supply all real-source
   verdicts, no `canary`, and no `inheritsFrom`. Finalize records
   `critic-canary-unavailable` and PARTIAL (unless another finding blocks). Do not fake
   inherited evidence; this exception is not a healthy-canary claim.

Option 1 is unavailable on exactly the run where the guard matters most: a healthy first run
has no prior verdicts, so a rule that stops at option 1 leaves the rubber-stamp critic
unguarded in the common case. That is circular — the canary would require the signal layer 2
exists to produce.

Two properties of the dispatch make a synthetic canary work, and both are load-bearing:

- **Every row's source is pasted, not just the canary's.** A batch where the canary row
  arrives inline and the rest arrive as file paths announces which row is the canary.
- **The paste is authoritative** — `coverage-critic` verdicts the source it was given, not
  what it may find on disk. Stated in the agent, so re-reading the file gains nothing.

Never write the weakened copy to disk. It exists only in the dispatch prompt: a synthetic
test file in the working tree would make every later op refuse `source-not-reproducible`,
and would be indistinguishable from a real test to the trace binding.

**Every dispatch states whether it carries a canary.** The single-row re-dispatches above
carry none, and cannot — weakening their only row is what they exist to undo. Except for the explicit snapshot-only unavailable batch above, a canary-free
dispatch is legitimate only when it re-runs rows whose canary-bearing dispatch already
caught its canary; it inherits that batch's evidence and names it. Without this the rule
recurses: the re-dispatch needs a canary, synthesizing one needs another re-dispatch, forever.
Say which kind each dispatch is in the prompt, because `coverage-critic` is told a canary is
present and would otherwise conclude a healthy lone row must be the weak one.

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
- `isolated` — a test file exactly one prior group's bundle names, and no other prior group's
  bundle or reads names.
- `shared` — a production, build, dependency, data, or documentation change outside the
  coverage directory, or a test file more than one prior context names.
- `unknown` — a test file no prior manifest names at all.

A `shared` or `unknown` path refuses every prior review group and mutation row; an `isolated`
one refuses only its own group and that group's mutation rows.

An adopted record keeps the prior outcome, failure included: a failed group or row comes back
failed, and asking for it again is a `repeat`, carrying `repeat.reason`, exactly like
repeating any other completed obligation — reuse never erases a prior failure by silently
re-asking it. `trace` and the live attempt are never reuse candidates: they always dispatch
fresh against this run's own commit, whatever `reuse` adopted.

`reuse` refuses per record, never the whole call. One line per reason:

- `missing-prior-run-id`, `prior-run-is-current` — the request itself is malformed.
- `trace-incomplete`, `stale-evidence` — this run has no completed, verified trace yet, or its
  identity moved since.
- `prior-ledger-missing`, `prior-ledger-invalid` — the named run has no ledger, or it will not
  load.
- `prior-source-mismatch`, `prior-commit-unreachable` — the prior run verified a different
  document, or names a commit this repository does not have.
- `prior-not-completed` — the prior record never finished.
- `legacy-no-manifest` — the record predates manifest capture; never reconstructed.
- `artifact-*` — the prior artifact is missing, unreadable, or no longer digests to what the
  ledger recorded.
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
- `policy-unspecified`, `policy-changed` — this request named no `policy` for review, or the
  critic or its instructions differ (review only).
- `sources-changed:<row>`, `status-changed:<row>` — this run's trace binds the row to
  different sources, or a different status, than the prior run's did (review only).
- `identity-changed` — a mutation row's own test identity differs from what this run's trace
  now binds (mutation only).
- `no-review-context` — no prior current review group both verifies and completely covers the
  row (mutation only).
- `already-recorded`, `row-already-grouped:<row>` — this run's own ledger already holds the
  record, or the row under another group.

### Bookkeeping

Record one `report.Batch` document per dispatch. These become `layer2.batches[]` of the
finalize request, and they are the report's audit trail:

```json
{
  "batchId": "batch-1",
  "dispatchId": "dispatch-1",
  "runId": "<run id>",
  "rows": ["SC-01", "SC-02", "SC-03"],
  "canary": {"rowId": "SC-01", "kind": "synthesized", "weakening": "...", "caught": true},
  "inheritsFrom": "",
  "voided": false,
  "verdicts": [{"rowId": "SC-02", "verdict": "HOLDS", "reason": "..."}]
}
```

Verdicts are `HOLDS`, `WEAK`, `VACUOUS`, or `MISSING`; anything else is refused
`invalid-verdict`. `WEAK`, `VACUOUS`, and `MISSING` all block. `canary.kind` is `natural` or
`synthesized`, and `weakening` is required and non-empty for a synthesized one.

The rules finalize enforces, each worth getting right the first time:

- **Exactly one non-voided verdict per active row.** None is `missing-verdict`; two is
  `duplicate-verdict`; a row verdicted only by voided batches is `voided-verdict`.
- **A layer-1 `gap` or `ambiguity` row must carry `MISSING`.** Any other verdict for it is
  `missing-verdict`.
- **Only a synthesized canary is excluded from its batch's row verdicts.** A natural
  canary is real source: its `WEAK` or `VACUOUS` verdict must be recorded and blocks. The weakened copy's
  verdict is canary evidence — it lives in `canary.caught` — not a row verdict. The row's real
  verdict comes from its single-row re-dispatch, and a synthesized canary without that
  separate real-source batch is refused `canary-bookkeeping`.
- **A canary whose batch did not catch it must void that batch.** `caught: false` with
  `voided` unset is `uncaught-canary-not-voided`.
- **A canary-free real-source retry must name `inheritsFrom`**, naming its caught-canary
  batch. The separate snapshot-only exception instead requires nonblank
  `canaryUnavailableReason`, exclusive with `canary` and `inheritsFrom`, and still
  supplies every real-source verdict; invalid combinations are `canary-bookkeeping`.
- **`runId` equals this run's ID** on every batch, or `stale-evidence`.
- **Every row named anywhere in a batch is an active row** of the source, or `unknown-row`.

When a canary is not caught, record `caught: false` and `voided: true`, then re-dispatch that
batch's rows at a higher effort tier as a **new** batch carrying its own canary. Do not point
the re-dispatch at the voided batch with `inheritsFrom`: voiding is transitive, so a batch
inheriting from a voided one is voided with it and its verdicts would not count either.
`inheritsFrom` exists for the canary-free single-row re-dispatch, and for that alone.

The report names the row that served as the canary for each batch, whether it was natural or
synthesized, and for a synthesized one the weakening applied — a claimed catch with no named
row and no named weakening is not auditable. It also names any batch that carried none, and
why.

### Reconciling `MISSING`

`MISSING` means the row's tagged test does not exist. On a row layer 1 already reported as
`gap` or `ambiguity`, that is the expected verdict and the two layers agree. On a row layer 1
reported as bound, they contradict each other: consult that row's `testSource` and binding metadata in
the immutable dispatch payload. A nonempty bound source means a dispatch defect —
re-paste that exact payload and re-dispatch that row rather than recording the `MISSING`.

