# test-design plugin — Design

**Date:** 2026-07-30
**Status:** approved

## Problem

The superpowers flow moves from an approved spec straight into `writing-plans`. Test
thinking exists only as per-task TDD steps inside the plan: each task says "write the
failing test", but nothing ever asks *which* tests the feature needs before tasks get
drawn. Coverage is therefore whatever the plan author happened to think of while
decomposing the work, and it is never checked afterwards.

Two consequences:

- **Under-coverage by omission.** Edge cases, failure paths, and concurrency get skipped
  silently. Nothing records that they were considered and ruled out versus never
  considered at all.
- **No verification that tests bite.** A green suite proves tests pass, not that they
  would catch the bugs they nominally guard against. A test asserting on a mock passes
  forever.

## Solution

A separate plugin, `test-design`, adding two stages to the superpowers flow: one that
designs coverage before planning, one that verifies it after implementation. Superpowers
itself is not modified — its plugin cache is overwritten on update, so the new skills
reference superpowers skills by name from the outside and stay durable.

### Flow

```
superpowers:brainstorming              spec approved by user
        │
        ▼
test-design:test-coverage-design       INTERACTIVE
        │                              behaviors → sweep → prune → coverage doc
        │  docs/superpowers/coverage/YYYY-MM-DD-<feature>-coverage.md
        ▼
superpowers:writing-plans              task test steps cite SC-IDs
        │
        ▼
superpowers:subagent-driven-development
        │                              TDD per task; `// covers: SC-nn` tags
        │                              RED output captured for risk=high rows
        ▼
test-design:coverage-verification      layer 1  trace       (main thread)
        │                              layer 2  coverage-critic subagent
        │                              layer 3  live-runner subagent
        │  docs/superpowers/coverage/YYYY-MM-DD-<feature>-verification.md
        │  docs/superpowers/coverage/transcripts/W*.txt
        ▼
superpowers:requesting-code-review → superpowers:finishing-a-development-branch
```

### Design decisions and rationale

| Decision | Rationale |
|---|---|
| Generic and English, no TMS coupling | Reusable across any repo and language. Deliberately not tied to TestIT / Kaiten / GitLab. |
| Two stages, not one | Generating coverage without verifying it produces a document nobody checks. Verifying without a designed yardstick has nothing to measure. |
| Table artifact, not Gherkin | The consumer is the agent writing the plan, not a business stakeholder. Gherkin costs roughly 5× the tokens per scenario and drifts from the real test code, creating two descriptions of one test. |
| `Bug it catches` as a required column | Makes the pruning rule structural rather than advisory. A row whose column cannot be filled is deleted. Mirrors the rule in superpowers `writing-good-tests`: name the production change that would make the test fail. |
| Generate stage interactive, verify stage unattended | Coverage design needs human judgment on applicability, risk, and scope. Verification is mechanical evidence-gathering. |
| Verification by fresh-context subagents | The agent that wrote the code is the worst judge of whether its tests catch the bug — it reads its own intent into a weak assertion. The critic's value comes entirely from not having written the implementation. |
| `coverage-critic` denied edit tools | Structurally prevents the critic silently repairing a weak test and then passing it. A constraint by tool grant, not by instruction. |
| Text-level enforcement in v1, hooks deferred | A hook can only check that a file exists, which is the same existence-only property rejected for verification. It also requires guessing the coverage-doc filename from the plan filename, and a false denial mid-flow is what makes people disable gates permanently. |

## Component 1 — `test-coverage-design`

**Input:** approved spec. **Output:** one committed coverage doc. **Mode:** interactive.

### Workflow

1. **Extract behaviors.** Read the spec, list every externally-observable capability.
   Confirm the list with the user before sweeping — a wrong behavior list makes every
   downstream row wrong.
2. **Sweep.** Each behavior against all ten axes. Emit candidate scenarios.
3. **Prune.** Each candidate must name one distinct production bug it would catch.
   Cannot name one → delete the row. Two rows naming the same bug → merge.
4. **Assign level.** The cheapest level that can actually catch that bug. Concurrency
   rows default to integration or above.
5. **Add harness hints** for smoke and e2e rows: environment, fixtures, what is touched.
6. **Record ruled-out axes** per behavior with a reason. Silent omission is forbidden.
7. **Extract invariants** — properties that must hold across all scenarios.
8. **List out-of-scope** items with a reason each.
9. Present, get approval, write and commit.

### Sweep axes (`references/sweep-axes.md`)

| # | Axis | Probes |
|---|------|--------|
| 1 | Boundaries / cardinality | 0, 1, many, max, max+1, empty, negative, huge |
| 2 | Data shape | null/missing, wrong type, unicode/emoji, whitespace, length overflow, injection payloads, bad encoding |
| 3 | Failure paths | dependency down, timeout, partial failure mid-operation, quota/disk full, malformed dependency response |
| 4 | Idempotency / retry | same request twice, retry after a timeout that actually succeeded, at-least-once delivery |
| 5 | Concurrency / ordering | see sub-axes below |
| 6 | Auth / permission | no actor, wrong actor, expired credential, insufficient scope, cross-tenant access |
| 7 | State / lifecycle | illegal transition, operation on deleted or archived entity, double-close, use-before-init |
| 8 | Time | timezone, DST, clock skew, expiry boundary, timestamp ties |
| 9 | Resource limits | oversized payload, connection exhaustion, rate limit reached |
| 10 | Contract / observability | error code and message correctness, failure actually logged and metered, API/schema backward compatibility |

Axis 5 sub-axes, enumerated because concurrency is the most-skipped and least
unit-testable axis:

- two writers on the same entity (lost update)
- check-then-act / TOCTOU (read-modify-write with no lock)
- out-of-order delivery (event B arrives before event A)
- duplicate delivery (at-least-once broker semantics)
- transaction isolation anomalies (dirty read, phantom, write skew)
- deadlock / inconsistent lock ordering
- concurrent initialization, cache stampede, double-start
- cancellation mid-flight (client aborts, context cancelled)
- shared mutable state across threads, goroutines, or async tasks

### Level assignment (`references/level-assignment.md`)

Levels: `unit`, `integration`, `smoke`, `e2e`. Assign the cheapest level that can catch
the named bug — never a higher level out of habit.

**Concurrency exception:** rows on axis 5 default to `integration` or above. A unit test
against a mock cannot produce real contention, so marking a mocked race as covered is a
false-confidence case. The only exception is code with an injectable scheduler or clock,
where interleaving is deterministic.

### Artifact template

```markdown
# Coverage: <feature>
Spec: docs/superpowers/specs/2026-07-30-<feature>-design.md

## Invariants
- INV-1 no partial commit ever visible to a concurrent reader
- INV-2 no duplicate side effect on a retried request

## Out of scope
- perf beyond 1k rps — no requirement in the spec
- multi-region failover — deferred, not in this spec

## Scenarios

### Behavior: create order
Ruled out: time (no date logic), auth (internal endpoint, no actor concept)

| ID | Behavior | Bug it catches | Level | Input / preconditions | Expected | Risk |
|----|----------|----------------|-------|----------------------|----------|------|
| SC-01 | qty at lower bound rejected | off-by-one lets qty=0 through | unit | qty=0 | 400, "qty must be > 0" | med |
| SC-02 | write rolls back on mid-tx failure | half-written order visible | integration | pg up, fail after row 1 | 0 rows persisted, INV-1 | high |
| SC-03 | two writers, same idempotency key | duplicate order created | integration | 2 parallel POST, same key | 1 row, 2nd → 200 | high |

harness SC-02: pg testcontainer, fault injection on the second insert
harness SC-03: 2 parallel clients, real pg, no mocks
```

### Size discipline

Aim for the smallest set that names distinct bugs. A typical feature lands 8–25 rows.
Over roughly 40 rows means the spec covers too much — return to the decomposition check
in `superpowers:brainstorming` rather than writing a plan nobody will finish.

## Component 2 — `coverage-verification`

Runs after implementation, before `superpowers:requesting-code-review`. Unattended.

### Layer 1 — trace (main thread)

- `grep -rn "covers: SC-"` across the test tree → map SC-ID to `file:line`
- run the suite, require green
- findings: **gap** (row with no tagged test), **drift** (tag citing an SC-ID absent from
  the coverage doc), **red** (suite not green)

### Layer 2 — `coverage-critic` subagent

Batched one subagent per behavior group; groups run in parallel. Each receives the SC
rows, the tagged test files, and nothing else. Tools: `Read`, `Grep`, `Glob`.

One question per row: *would this test fail if the named bug were reintroduced?*

Verdicts: `HOLDS`, `WEAK` (passes for the wrong reason), `VACUOUS` (asserts nothing bound
to the bug). Defect patterns it hunts:

- assertion on a mock rather than on resulting state
- assertion on a value the implementation returns trivially
- assertion that still holds with the bug present

Rows with `risk=high` additionally require the RED output captured during the TDD cycle.
Recording the failure at write time is cheaper and stronger evidence than re-breaking the
code later. Absent → `NO-EVIDENCE`.

### Layer 3 — `live-runner` subagent

Tools: `Bash`, `Read`. Brings the system up via the project's run recipe (or the homelab
services when real dependencies are needed), executes every smoke and e2e row plus the
full walkthrough set, and redirects real output into transcript files.

Walkthroughs cover the happy path **and each principal non-happy path** — the happy path
is not privileged, it is just row zero:

```
W0 happy      order accepted, event consumed, status CONFIRMED
W1 rejected   qty=0        → 400, no event published, no row
W2 dep down   pg stopped   → 503, event NOT acked, retried on recovery
W3 duplicate  same key 2x  → 1 row, 2nd call 200 idempotent
W4 timeout    consumer 30s → message requeued, no partial write
W5 parallel   N clients    → concurrency rows exercised under real contention
```

**Tamper check:** after the subagent returns, the main thread runs
`git status --porcelain` over source and test paths. `Bash` cannot be structurally
constrained the way `coverage-critic` can, so the no-edit constraint is verified after the
fact rather than granted away.

### Report template

```markdown
# Verification: <feature>
Coverage: docs/superpowers/coverage/2026-07-30-orders-coverage.md
Verdict: BLOCKED — 3 gaps, 1 live failure

## Layer 1 — trace
20/23 rows tagged · suite green (142 passed)
gap:   SC-07, SC-12 — no tagged test
drift: tag SC-99 in orders_test.go — not in coverage doc

## Layer 2 — adversarial read
HOLDS 17 · WEAK 2 · VACUOUS 1
SC-05 VACUOUS      asserts mock.calledOnce, never asserts persisted state
SC-11 WEAK         would still pass with the off-by-one present
SC-03 NO-EVIDENCE  risk=high, no RED output recorded

## Layer 3 — live
smoke/e2e rows 6/6 PASS
W0 PASS · W1 PASS · W2 FAIL · W3 PASS · W4 PASS · W5 PASS
W2 FAIL  pg stopped → 500 not 503, message NOT requeued — INV-1 at risk
transcripts: docs/superpowers/coverage/transcripts/
```

Any gap, `VACUOUS`, `NO-EVIDENCE`, or live `FAIL` yields verdict `BLOCKED` and no handoff
to code review. Re-runs accept an ID subset, so fixing two rows does not re-execute the
whole live layer.

### Degradation rules

| Situation | Behavior |
|---|---|
| No approved spec (user enters mid-flow) | Generate stage runs, header records `Spec: none — derived from conversation`. Visible, not a blocker. |
| No run recipe, or dependencies unavailable | Layer 3 reports `SKIPPED` with a per-row reason. Verdict becomes `PARTIAL`, never `PASS`. |
| No test suite yet (legacy code) | Layer 1 reports every row as a gap, as-is. No softening. |
| Coverage doc missing at verify time | Verification refuses to run — there is no yardstick. |

Three verdicts only: `PASS`, `PARTIAL`, `BLOCKED`. `PARTIAL` exists so that an
un-runnable live layer cannot masquerade as success.

## Repository layout

```
test-design/
  .claude-plugin/
    marketplace.json                      name: test-design
    plugin.json
  skills/
    test-coverage-design/
      SKILL.md                            workflow, gate, artifact template
      references/sweep-axes.md            the ten axes + concurrency sub-axes
      references/level-assignment.md      pyramid rules + concurrency exception
      references/example-coverage-doc.md  one filled reference artifact
    coverage-verification/
      SKILL.md                            three layers, dispatch rules, report template
      references/example-verification.md
  agents/
    coverage-critic.md                    tools: Read, Grep, Glob
    live-runner.md                        tools: Bash, Read
  README.md
```

**Home:** `/home/igi21/experiements/test-design`, remote
`http://192.168.1.34:3000/service/test-design.git`.

Development install:

```bash
claude plugin marketplace add /home/igi21/experiements/test-design
claude plugin install test-design@test-design
```

## Enforcement

Skill-text `REQUIRED SUB-SKILL` handoffs — the same mechanism superpowers uses on itself
— plus one rule in `~/.claude/CLAUDE.md`:

```markdown
## Test design gate
After a spec is approved and before superpowers:writing-plans, invoke
test-design:test-coverage-design. After implementation and before
superpowers:requesting-code-review, invoke test-design:coverage-verification.
A plan with no coverage doc, or a branch with no verification report, is incomplete.
```

## Acceptance criteria

Dogfooding is the acceptance test: run the full flow end-to-end on one small real
feature, then confirm the observable outcomes.

- [ ] Coverage doc contains a `Ruled out:` line for every behavior group
- [ ] Every scenario row has a non-empty `Bug it catches` value
- [ ] Every concurrency row is at `integration` level or above
- [ ] Plan tasks produced by `writing-plans` cite SC-IDs in their test steps
- [ ] Layer 1 detects a deliberately untagged test as a gap
- [ ] `coverage-critic` returns `VACUOUS` or `WEAK` for a deliberately planted weak test
      — a critic that never rejects anything is broken, so the rejection must be proven
- [ ] `live-runner` produces transcripts and reports `FAIL` for a deliberately broken
      non-happy path
- [ ] A feature with no run recipe yields verdict `PARTIAL`, not `PASS`

## Known limitation

Verification measures the implementation against the coverage doc. It cannot detect bugs
the doc never anticipated — a thin doc yields a confident and useless green report. The
doc is the yardstick, which is why the generate stage is interactive rather than
autonomous. No amount of verification rigor compensates for a lazy sweep.

## Deferred

- Soft warn-hook on writes to `docs/superpowers/plans/*`, injecting context about whether
  a coverage doc exists. Add only if the stage is observed being skipped in practice.
- TestIT / Kaiten export adapter for ATON work.
- Reconciliation with the Russian skills in the `qa` repo. `checklist-from-feature`
  overlaps in intent. Both are left alone; no cross-repo synchronization.
