# test-design

A Claude Code plugin adding two stages to the [superpowers](https://github.com/obra/superpowers) flow.

| Stage | When | What it produces |
|---|---|---|
| `test-coverage-design` | after explicit opt-in, before `writing-plans` | a coverage source: invariants, out-of-scope, and an ID'd scenario table |
| `coverage-verification` | at the `finishing-a-development-branch` seam | a verification report across four layers |

The plugin is passive. Installing it does not prompt, amend planning, or run either stage.
After a spec is approved, invoking `/test-design:test-coverage-design` opts that feature
into both coverage design and final coverage verification. Features that do not opt in
follow the normal Superpowers flow without waiver markers or warnings.

## Why

The superpowers flow goes from spec straight to plan. Test thinking exists only as
per-task TDD steps, so nothing ever asks which tests the feature needs before tasks get
drawn — and nothing later checks that the tests written actually catch the bugs they
nominally guard against. A green suite proves tests pass, not that they bite.

## The four verification layers

1. **trace** — every scenario has a tagged test that the runner reports as *executed and passed*
2. **adversarial read** — a fresh subagent judges whether each test would fail if the named bug returned
3. **live** — smoke and e2e rows plus derived happy and non-happy walkthroughs run against the real system
4. **mutation** — for high-risk rows, the named mutation is applied and the test must fail

## Supported runners

Go, Maven, and Gradle projects only — the trace op binds `covers:` tags structurally
through Go's parser and AST for Go, and a token scanner for Java and Kotlin. No other
language or test runner is supported.

## Prerequisites

A Go 1.26+ toolchain on the machine running either skill. The plugin's executable has no
third-party dependencies, so there is no build step and no binary to install — every op
runs as `go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier <op> …`.

## The verifier executable

Both skills drive every mechanical step — the repository snapshot, the disposable
worktrees, the suite runs, the tag binding, the live plan execution, the mutation
apply/evaluate cycle, and the verdict computation — through nine ops of one Go
executable:

| Op | What it does |
|---|---|
| `next-id` | allocates the next unused `SC-nn` for an area, scanning every coverage document and floor-rule table in the repository |
| `preflight` | validates a coverage source and its runner block, and computes the approval-gate summary, without touching the repository |
| `baseline` | snapshots the repository at the baseline commit, runs the suite there, and publishes baseline evidence |
| `trace` | runs the suite in a disposable worktree, binds every `covers:` tag to its declaration, and joins each binding to the runner's records as layer-1 evidence |
| `prepare-mutation` | resolves a row's bound test identity and opens a disposable worktree for layer 4 |
| `evaluate-mutation` | runs a mutated site's targeted test, requires it to fail, restores the source, and reruns it to require a pass |
| `prepare-live` | opens a disposable worktree and the run's live-profile descriptor for layer 3 |
| `execute-live` | validates and runs a live plan against the real system, capturing reduced transcripts |
| `finalize` | validates the evidence from all four layers, computes the verdict, and publishes the report |

## Lifecycle vocabulary

- **TDD RED** — the initial test run against untouched production code fails, as expected.
- **ALREADY-GREEN** — that same run passes; the row returns to the parent for
  reconciliation rather than counting as TDD RED.
- **Verification mutation** — the coverage source's per-row code-level fault; informs test
  design but is applied only by final verification, never during implementation.
- **MUTATION-CAUGHT** — the row's targeted test failed with the mutation applied and
  passed again after restore, for every site the mutation could be applied at.

## Artifacts

```text
.test-design/live-profiles/<name>.json                          declared live-environment surface for layer 3
docs/superpowers/coverage/YYYY-MM-DD-<feature>-coverage.md       the approved coverage source
docs/superpowers/coverage/YYYY-MM-DD-<feature>-baseline.json     baseline evidence `baseline` publishes
docs/superpowers/coverage/YYYY-MM-DD-<feature>-verification.json the canonical verification report `finalize` publishes
docs/superpowers/coverage/YYYY-MM-DD-<feature>-verification.md   that report rendered to Markdown
docs/superpowers/coverage/transcripts/<run-id>/                  every evidence document one verification run produced
```

## Install

Clone, then point a local marketplace at the clone:

```bash
git clone http://192.168.1.34:3000/service/test-design.git
claude plugin marketplace add ./test-design
claude plugin install test-design@test-design
```

`claude plugin marketplace add` takes the path to the clone, so substitute wherever you put
it. The remote above is LAN-only; any clone of this repo works the same way.

Installing gives you the skills and agents only. It registers no hook and requires no
global `CLAUDE.md` rule. Ordinary plan writes receive no warning or denial.

Both skills are invoked explicitly, never automatically: `/test-design:test-coverage-design`
after a feature's spec is approved, and `/test-design:coverage-verification
<coverage-source>` at the `finishing-a-development-branch` handoff. See
[docs/opt-in.md](docs/opt-in.md) for the resulting handoffs and `wave-planning`
composition.
