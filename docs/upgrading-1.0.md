# Upgrading from 0.x to 1.0

1.0 replaces the skill-driven, hand-run verification of 0.5 with a Go executable that owns
every mechanical step. The skills and agents still exist under the same names, but what they
ask of you, what they produce, and what they accept as input all changed. This page lists
what to do on a machine or repository that used 0.5.

## Changes to the machine

**Remove the `## Test design gate` rule from `~/.claude/CLAUDE.md`.** 0.5 required a global
rule that amended `superpowers:brainstorming` to ask about coverage at the spec seam. 1.0 is
opt-in only: nothing prompts, and the rule now does harm by asking a question the plugin no
longer expects an answer to. Delete the whole section; `docs/enforcement.md`, which
documented it, is gone.

**The warning hook is gone.** 0.5 registered `hooks/coverage-doc-warning.sh`, which fired on
plan writes citing missing SC rows, and `measure-skip-rate.sh`. 1.0 registers no hook. If you
copied either into a personal hook configuration, remove it — nothing in 1.0 emits the
`Coverage: waived (user decision, YYYY-MM-DD)` line it looked for.

**Install a Go 1.26+ toolchain.** Every op runs as
`go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier <op> …`. There is no build step
and no committed binary, but `go` must be on `PATH` wherever a skill runs. Capture the
absolute target root before changing directories, not from the plugin checkout:

```bash
TARGET_REPO=$(git rev-parse --show-toplevel)
```

Every request's `repoRoot` is that absolute value, and every `-repo` uses
`"$TARGET_REPO"`; source paths are relative to that target, not the plugin.

**Update the plugin** so `CLAUDE_PLUGIN_ROOT` points at a 1.0 checkout. The marketplace is
the local clone, so pull it first:

```bash
git -C ./test-design pull
claude plugin marketplace update test-design
claude plugin update test-design@test-design
```

Restart Claude Code afterwards; `plugin update` applies on the next session.

## Changes to what is supported

| | 0.5 | 1.0 |
|---|---|---|
| Runners | vitest/jest, go, pytest, maven/gradle, any JUnit XML | **Go, Maven, Gradle only** |
| Tag binding | text matching on runner output (`runner-output.md`) | structural: Go parser/AST; Java/Kotlin token scanner |
| Suite execution | by the agent, in the working tree | by the executable, in a disposable worktree |
| Mutation | agent edits and reverts source | `mutator` edits a worktree; executable evaluates and restores |
| Live | agent runs commands directly | `live-runner` returns a plan; executable runs it under a committed profile |
| Report | Markdown written by the agent | `finalize` publishes canonical JSON plus rendered Markdown |

A repository whose suite is vitest or pytest cannot opt in under 1.0. There is no fallback
path; every op refuses any other `runner-kind` as `unsupported-runner-kind`.

## Changes to an existing coverage source

A 0.5 coverage doc is refused by 1.0's parser until migrated. Three things changed:

1. **Column order.** The scenario table header is now
   `| ID | Behavior | Expected | Level | Verification mutation | Risk |`. 0.5 put the
   mutation column third and `Expected` fifth. Move the cells; the text does not change.
2. **A `## Verification` block is required.** It carries the runner configuration the
   executable needs (`runner-kind`, `runner-root`, `runner-command`, `report-globs`,
   `baseline-evidence`, `baseline-sha256`, `final-command`, and `live-profile` when any
   row is `smoke` or `e2e`). 0.5 had no machine-readable equivalent; the block is written
   from how the suite is actually run today.
3. **Baseline is captured by the `baseline` op**, not by hand, and takes two commits — the
   first so the op has a committed source to snapshot, the second to record the SHA-256
   and counts it printed.

The step-by-step procedure, including what to do when two old documents collide on an SC
ID, is the conditional
[migration reference](../skills/test-coverage-design/references/migration.md). The done
condition is a successful

```text
go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier preflight -source <path> -repo "$TARGET_REPO"
```

`covers: SC-nn` tags in test source are unchanged and do not need touching, unless the
migration renumbers rows to resolve a collision.

## Changes to smoke and e2e rows

A 0.5 doc with `smoke` or `e2e` rows described the live walkthrough in prose and the agent
ran it. 1.0 runs layer 3 only under a committed live profile at
`.test-design/live-profiles/<name>.json`, named by the `## Verification` block. Without one,
layer 3 is `PARTIAL` with attempt evidence and the run does not reach `PASS`. Writing a
profile is a project decision, reviewed like build configuration; see
[live-profile.md](live-profile.md).

## Changes to how a run ends

A 0.5 run ended when the agent wrote its report. A 1.0 run ends when `finalize` publishes
`<feature>-verification.json` and `.md` under `docs/superpowers/coverage/` plus every
evidence document under `transcripts/<run-id>/`. Two consequences worth knowing before the
first run:

- **`PARTIAL` needs an acknowledgement.** `finalize` publishes a `PARTIAL` report with
  `acknowledged: false`. It is not a finished gate until rerun with the user's actual
  acknowledgement after publication. Keep HEAD unchanged: preserve scratch evidence,
  then restore/remove only the exact generated publication files before that rerun.
  Committing the first report moves HEAD and requires a new full verification run.
  Follow the mandatory [finalize/recovery procedure](../skills/coverage-verification/references/finalize-recovery.md).
- **A dirty tree refuses.** There is no `--force`. Resolve user/source changes without
  deleting unrelated work; a changed commit requires fresh evidence. Generated PARTIAL
  publication recovery is the exact-file procedure linked above, not a blanket discard.

## 1.1 evidence hardening

Version 1.1.0 retains schemaVersion `"1"` and the existing baseline schema. Live/mutation
evidence and attempts now require `repoCommit`, `source`, and `sourceSha256`, in addition
to run and row identities. Regenerate old unbound evidence with a freshly approved full
run; do not fill missing fields from current HEAD or rename/re-stamp old evidence.
The executable's `record-attempt` operation records actual agent/environment refusals;
controllers no longer hand-write attempt envelopes.

## What did not change

- The two entry points: `/test-design:test-coverage-design` after spec approval,
  `/test-design:coverage-verification <coverage-source>` at the
  `finishing-a-development-branch` handoff.
- The four layers and their meaning.
- The row schema's semantics: one behavior per row, a `Verification mutation` that is a
  code-level fault rather than a negated expectation, `Risk` derived per
  `references/risk-derivation.md`.
- SC IDs, once allocated, stay fixed. `next-id` now allocates them, scanning every coverage
  document in the repository, so a new feature never reuses an old feature's number.
