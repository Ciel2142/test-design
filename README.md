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
3. **live** — the journeys the live decision names, plus every active `smoke` or `e2e` row, run against the real system
4. **mutation** — for high-risk rows, the named mutation is applied and the test must fail

## Supported runners

Go, Maven, and Gradle projects only — the trace op binds `covers:` tags structurally
through Go's parser and AST for Go, and a token scanner for Java and Kotlin. No other
language or test runner is supported.

## Prerequisites

A Go 1.26+ toolchain on the machine running either skill. The plugin's executable has no
third-party dependencies, so there is no build step and no binary to install — every op
runs as `go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier <op> …`. Inside a
Claude Code session that variable is usually empty in the Bash tool's shell; both skills
open with a step that resolves the plugin root from the skill directory or from
`~/.claude/plugins/installed_plugins.json` and pastes the literal path into every command.

## The verifier executable

Both skills drive every mechanical step — the repository snapshot, the disposable
worktrees, the suite runs, the tag binding, the live plan execution, the mutation
apply/evaluate cycle, and the verdict computation — through sixteen ops of one Go
executable:

| Op | What it does |
|---|---|
| `next-id` | allocates the next unused `SC-nn` for an area, scanning every coverage document and floor-rule table in the repository |
| `preflight` | validates a coverage source and its runner block, and computes the approval-gate summary, without touching the repository |
| `resume` | reports what a run's ledger has already answered and what it still owes; the one op that only reads, safe to call at any point |
| `baseline` | snapshots the repository at the baseline commit, runs the suite there, and publishes baseline evidence |
| `trace` | runs the suite in a disposable worktree, binds every `covers:` tag to its declaration, and joins each binding to the runner's records as layer-1 evidence |
| `reuse` | judges which of a prior run's review groups and mutation rows still match this run's trace and worktree, so the run can adopt them instead of re-dispatching |
| `record-review` | records a layer-2 review group's start and completion in the run's ledger, the evidence `finalize` later reads as the group's batch |
| `prepare-mutation` | resolves a row's bound test identity and opens a disposable worktree for layer 4 |
| `evaluate-mutation` | runs a mutated site's targeted test, requires it to fail, restores the source, and reruns it to require a pass |
| `record-mutation` | records a row's mutation as unappliable in the run's ledger when no site of its verification mutation applies as a code-level fault |
| `recover-mutation` | restores or recreates an interrupted mutation row's worktree so evaluation can continue |
| `discard-mutation-edit` | discards a mutator edit that failed confirmation, restoring the worktree so the same site can be re-dispatched |
| `prepare-live` | opens a disposable worktree and the run's live-profile descriptor for layer 3 |
| `execute-live` | validates and runs a live plan against the real system, capturing reduced transcripts |
| `record-attempt` | records an observed live or mutation refusal with verifier-stamped run, source, and commit identity |
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
  review-<groupId>.json                                          one layer-2 review group's verdicts
  mutation-unappliable-<rowId>.json                              a row whose verification mutation could not be applied
  reused-*                                                       an adopted record's own artifact, copied here and re-verified
```

## Install

Clone, then point a local marketplace at the clone:

```bash
git clone http://100.64.0.5:3000/service/test-design.git
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

## Verify the install

Restart Claude Code, then type `/test-design:` — the completion list must offer
`test-coverage-design` and `coverage-verification`. From any shell:

```bash
go version
R=$(grep -o '"installPath": *"[^"]*/test-design/[^"]*"' \
  ~/.claude/plugins/installed_plugins.json | head -1 | cut -d'"' -f4)
go run -C "$R" ./cmd/test-design-verifier
```

`go version` reports 1.26 or later, and the bare executable answers
`{"code":"unknown-operation","message":"no operation specified"}` — that is the plugin
compiling and the ops dispatcher running.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| `go run -C "": …` or an op that reports the plugin as the target | `CLAUDE_PLUGIN_ROOT` is empty in the shell. Run the resolution step at the top of the skill and paste the printed path. |
| `source-outside-repo` from `preflight` | `-source` was given relative to the plugin, or `-repo` was omitted. Pass `-repo "$TARGET_REPO"` and a path relative to it. |
| `/test-design:…` skills missing after `plugin update` | Updates apply on the next session. Restart Claude Code. |
| The Skill tool refuses `test-coverage-design` | Intended: both skills carry `disable-model-invocation`. Type the slash command yourself. |
| `dirty-repository` or `source-not-reproducible` | Commit or discard the change the message names. There is no force flag. |
| `normalization-failed: no fresh report file matched report-globs` from `baseline` or `trace` | The runner command failed before it wrote any report, typically a wrong `JAVA_HOME` or a missing wrapper. Run the exact `runner-command` by hand in a detached worktree at the baseline commit to see the real error. |
| `row-not-bound: row bound ambiguously` from `prepare-mutation` | Two tests carry `covers:` for one `high` row. Keep the tag on the test that flips under the mutation, cite the row from the other with a different word, and start a new run. |
| `foreign-diff` from `evaluate-mutation` | The mutator edited a template, YAML, XSD, build or test path. The row's fault is outside production source; amend the coverage source to name a fault in the code that consumes that file. |

## Developing

The verifier's end-to-end tests run real `go test` suites inside disposable worktrees, so
the command package needs longer than Go's default ten-minute per-package timeout:

```bash
make test        # go test -timeout 40m ./...
make check       # gofmt, go vet, and the fast packages only
```

## Further reading

- [docs/opt-in.md](docs/opt-in.md) — the per-feature opt-in and how it composes with `wave-planning`
- [docs/live-profile.md](docs/live-profile.md) — writing the committed live profile that a required live decision names
- [docs/upgrading-1.0.md](docs/upgrading-1.0.md) — moving a machine or repository from 0.x: global rule and hook removal, coverage-doc migration, dropped runners
