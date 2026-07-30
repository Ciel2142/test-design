# test-design plugin — Design

**Date:** 2026-07-30
**Status:** approved, revised after adversarial review
**Revision:** 2 — see [Revision history](#revision-history)

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
        │                              (small features: coverage table appended
        │                               inline here — see Floor rule)
        ▼
test-design:test-coverage-design       INTERACTIVE
        │                              behaviors → sweep → prune → coverage doc
        │  <repo>/docs/superpowers/coverage/YYYY-MM-DD-<feature>-coverage.md
        ▼
superpowers:writing-plans              task test steps cite SC-IDs
        │
        ▼
superpowers:subagent-driven-development
        │                              TDD per task; `covers: SC-nn` tags
        │                              (SDD runs its own per-task and final
        │                               code reviews internally, then deletes
        │                               its workspace)
        ▼
test-design:coverage-verification      layer 1  trace           main thread
        │                              layer 2  coverage-critic subagent
        │                              layer 3  live-runner     subagent
        │                              layer 4  mutator         subagent (risk=high)
        │  <repo>/docs/superpowers/coverage/YYYY-MM-DD-<feature>-verification.md
        │  <repo>/docs/superpowers/coverage/transcripts/
        ▼
superpowers:finishing-a-development-branch
```

**Insertion point.** Verification runs at the `finishing-a-development-branch` seam, which
is the only point `subagent-driven-development` explicitly hands control back
(`subagent-driven-development/SKILL.md:423`). It is *not* placed before
`requesting-code-review`: that skill is consumed *inside* SDD as its final review
(`SKILL.md:399-400`), so no seam exists there. Consequence accepted: SDD's per-task and
final code reviews happen before coverage verification, so a `BLOCKED` verdict arrives
after those reviews rather than gating them.

### Design decisions and rationale

| Decision | Rationale |
|---|---|
| Generic and English, no TMS or infrastructure coupling | Reusable across any repo and language. Deliberately not tied to TestIT / Kaiten / GitLab, and not to any particular host or LAN. |
| Two stages, not one | Generating coverage without verifying it produces a document nobody checks. Verifying without a designed yardstick has nothing to measure. |
| Table artifact, not Gherkin | The consumer is the agent writing the plan, not a business stakeholder. Gherkin costs roughly 5× the tokens per scenario and drifts from the real test code. |
| `Bug it catches` phrased as a code-level mutation | Makes the pruning rule structural, and makes layer 4 executable: the column *is* the mutation to apply. |
| `Expected` kept, `Input / preconditions` cut | `Expected` is where the human confirms "rejected" means 400 rather than 422 or a silent ignore, before code exists — a reviewable contract worth its drift risk. `Input` is test-body detail the plan's code block already carries. |
| Risk derived from axis, not self-assigned | `high` obliges mutation evidence. An agent that both assigns risk and bears its cost has a free, untraceable way to switch the gate off. Judgment may argue a row *down* with user approval; it may never set the burden. |
| Generate stage interactive, verify stage unattended | Coverage design needs human judgment on applicability, risk, and scope. Verification is mechanical evidence-gathering. |
| Verification by fresh-context subagents | The agent that wrote the code is the worst judge of whether its tests catch the bug — it reads its own intent into a weak assertion. |
| `coverage-critic` denied edit tools but *given* the production diff | The tool denial prevents it repairing a weak test and passing it. Withholding the diff would leave it unable to answer its own question, since "would this fail if the bug returned" is a claim about production code. |
| Machine-readable test results, not grep alone | A tag above a skipped or filtered-out test satisfies a grep and leaves the suite green. Only the runner's own report distinguishes *executed and passed* from *not run*. |
| Mutation evidence, not TDD-time RED capture | See [Why not RED capture](#why-not-red-capture). |
| Text-level enforcement in v1, hooks deferred | Not because hooks are incapable — they run arbitrary commands and could perform every check here. Because a false denial mid-flow is what gets a gate disabled permanently, and the false-denial rate is unknown until the stage has real mileage. |

### Why not RED capture

An earlier revision required the RED failure output captured during TDD as evidence for
`risk=high` rows. That is unimplementable in this flow, verified against source:

- The implementer writes RED output into its task report
  (`subagent-driven-development/implementer-prompt.md:121-122`).
- That report lives in the SDD workspace, `<repo>/.superpowers/sdd/<plan-basename>/`
  (`SKILL.md:124`).
- The workspace is git-ignored — `sdd-workspace:39` runs `printf '*\n' > "$base/.gitignore"`.
- The workspace is deleted before the handoff this design attaches to: `rm -rf <workspace>`
  at `SKILL.md:419`.

So the only artifact containing RED output is destroyed before verification runs, and was
never in git history. Two further breaks even if it survived: the output is per task, not
per SC-ID, with nothing requiring `SC-nn` labelling; and the capture is conditional
(*"if TDD was required for this task"*), while SDD's own reviewer prompt instructs readers
to treat implementer reports as unverified claims.

Replaced by **layer 4**, scoped retroactive mutation. Weaker as a statement about
discipline — it does not prove the test was written first — but stronger as evidence,
because it is mechanically reproducible by a third party rather than a prose assertion.

## Component 1 — `test-coverage-design`

**Input:** approved spec. **Output:** one committed coverage doc. **Mode:** interactive.

### Floor rule

If the feature has **≤2 behaviors and no persistence, no concurrency, and no external
dependency**, do not run this stage as a separate step. Append the coverage table inline to
the spec at brainstorming's existing spec-review gate. No separate document, no separate
human approval.

Rationale: below that size the rows are the plan's test steps restated, and the stage costs
two human gates and several subagents to re-derive them. `superpowers:brainstorming` handles
the same objection by degrading gracefully (*"The design can be short… but you MUST present
it"*); this stage needs the same escape hatch or it will simply be skipped.

### Workflow

1. **Extract behaviors.** Read the spec, list every externally-observable capability.
   Confirm the list with the user before sweeping — a wrong behavior list makes every
   downstream row wrong.
2. **Sweep.** Each behavior against all twelve axes. Emit candidate scenarios.
3. **Prune.** Each candidate must name one distinct production bug, phrased as a
   code-level mutation. Cannot name one → delete the row. Two rows naming the same
   mutation → merge, retiring one ID.
4. **Derive risk** by rule (below), then allow argue-down with user approval.
5. **Assign level.** The cheapest level that can actually catch that bug.
6. **Add harness hints** for smoke, e2e, and component rows: environment, fixtures, what is
   touched.
7. **Record ruled-out axes** per behavior, each citing a spec fact or a code fact.
8. **Extract invariants** — properties that must hold across all scenarios.
9. **List out-of-scope** items with a reason each.
10. Present, get approval, write and commit.

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
| 11 | Rendering / interaction | loading, empty, and error states; focus and keyboard order; responsive breakpoints; i18n layout; optimistic-update rollback |
| 12 | Migration / rollout | forward and backward compatibility during rolling deploy, old and new code against one schema, partial-migration resume, production-scale volume |

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

**Ruling out an axis** requires citing a spec fact or a code fact, not a judgment.
`Ruled out: concurrency (no shared state)` is insufficient — dismissing axis 5 specifically
requires naming the shared resource that is absent (no table written, no cache, no lock, no
queue consumer). Unfalsifiable dismissals are how the flagship axis gets waved away in one
clause.

### Risk derivation

`Risk` is not judged, it is derived:

- Rows on axes **3, 4, 5, 6** are `high` by rule.
- Any row whose mutation names data loss, data corruption, or exposure is `high`.
- Everything else is `med`, or `low` where the failure is cosmetic.

The agent may propose lowering a row to `med` only with a written reason the user
explicitly approves, recorded in the doc. It may never raise the burden on itself and it may
never lower one silently. `high` is load-bearing: it is what obliges layer 4 mutation
evidence.

### Level assignment (`references/level-assignment.md`)

Levels: `unit`, `component`, `integration`, `smoke`, `e2e`. Assign the cheapest level that
can catch the named bug — never a higher level out of habit.

**Concurrency exception:** rows on axis 5 default to `integration` or above. A unit test
against a mock cannot produce real contention, so marking a mocked race as covered is a
false-confidence case. The only exception is code with an injectable scheduler or clock,
where interleaving is deterministic.

**Rendering exception:** rows on axis 11 default to `component` or above. The same
false-confidence argument applies — a jsdom environment is a mock of a browser, so
`unit` is not an honest home for a rendering claim.

### Row schema and column semantics

`| ID | Behavior | Bug it catches | Level | Expected | Risk |`

- **`ID`** — `SC-nn`, or `<area>/SC-nn` in a monorepo, where `<area>` is the package the
  grep and suite are scoped to. Stable for the feature's lifetime. Never renumbered; a
  deleted or merged row's ID moves to `## Retired`.
- **`Bug it catches`** — phrased as a code-level mutation: wrong operator, missing guard,
  swapped argument, absent await, inverted condition, omitted rollback. A row whose bug is
  merely the negation of its `Expected` value ("qty=0 accepted") is not a mutation and gets
  deleted — that phrasing auto-fills for every row and makes the pruning rule vacuous.
- **`Level`** — `unit | component | integration | smoke | e2e`. Rows at `component`,
  `smoke`, or `e2e` need a harness hint. `smoke` and `e2e` rows are what layer 3 executes.
- **`Expected`** — the observable contract, specific enough to be machine-checkable.
- **`Risk`** — `low | med | high`, derived by the rule above.

### Artifact template

```markdown
# Coverage: <feature>
Spec: docs/superpowers/specs/2026-07-30-<feature>-design.md
Baseline: <merge-base sha> — suite result captured at this commit

## Invariants
- INV-1 no partial commit ever visible to a concurrent reader
- INV-2 no duplicate side effect on a retried request

## Out of scope
- perf beyond 1k rps — no requirement in the spec
- multi-region failover — deferred, not in this spec

## Retired
- SC-04 merged into SC-03 (same mutation)

## Scenarios

### Behavior: create order
Ruled out: time (spec defines no date logic), rendering (no UI surface — API only),
migration (no schema change in this feature)

| ID | Behavior | Bug it catches | Level | Expected | Risk |
|----|----------|----------------|-------|----------|------|
| SC-01 | qty at lower bound rejected | guard uses `<` instead of `<=` | unit | 400, "qty must be > 0" | med |
| SC-02 | write rolls back on mid-tx failure | rollback call omitted on the error path | integration | 0 rows persisted, INV-1 | high |
| SC-03 | two writers, same idempotency key | uniqueness check runs outside the tx (TOCTOU) | integration | 1 row, 2nd → 200 | high |

harness SC-02: pg testcontainer, fault injection on the second insert
harness SC-03: 2 parallel clients, real pg, no mocks
```

### Amendment

Implementation routinely invalidates a pre-implementation doc — a new failure mode
surfaces, an error code changes, a sub-feature gets dropped. Without an amend path,
verification measures the code against a stale yardstick and passes.

Amendments **append** rows with new IDs and move superseded ones to `## Retired`, with user
approval. Layer 1 emits a `stale` finding when the diff touches a behavior that has no SC
row, which is the trigger to amend.

### Artifact location

Coverage docs and verification reports live in the **repository being worked on**, beside
its specs and plans — never in the `test-design` repo:

```
<target-repo>/docs/superpowers/
  specs/       YYYY-MM-DD-<feature>-design.md         (superpowers:brainstorming)
  coverage/    YYYY-MM-DD-<feature>-coverage.md       (test-coverage-design)
               YYYY-MM-DD-<feature>-verification.md   (coverage-verification)
               transcripts/                           (live-runner, mutator)
  plans/       YYYY-MM-DD-<feature>.md                (superpowers:writing-plans)
```

In a monorepo where packages own their docs, these paths are relative to the package, and
SC-IDs carry the `<area>/` prefix so one feature's grep cannot collide with another's tags.

### Size discipline

Aim for the smallest set that names distinct mutations. A typical feature lands 8–25 rows
**after pruning**. The ceiling applies to post-prune rows only: twelve axes across five
behaviors generate far more than forty candidates by construction, so a ceiling on
candidates would reward under-sweeping. If more than roughly forty rows *survive* pruning,
the spec covers too much — return to the decomposition check in
`superpowers:brainstorming`.

## Component 2 — `coverage-verification`

Runs at the `finishing-a-development-branch` seam. Unattended.

### Layer 1 — trace (main thread)

1. `grep -rn "covers: SC-"` scoped to the affected package → map SC-ID to `file:line`
2. Run the suite with machine-readable output (`go test -json`, `pytest --junitxml`,
   `--reporter=json`, `surefire-reports/*.xml`), **redirected to a file**. Parse the
   summary; never let raw suite output land in the main thread's context.
3. Compare against the **baseline** suite result captured at merge-base.

Findings:

| Finding | Meaning |
|---|---|
| `gap` | Row with no tagged test, **or** a tagged test the runner reports as skipped, filtered out, or absent from the run |
| `drift` | Tag citing an SC-ID that is neither in the doc nor in `## Retired` |
| `overclaim` | One test carrying more than 2 SC-IDs — each cited row must then be verdicted separately by layer 2 against a named assertion |
| `stale` | Diff touches a behavior with no SC row |
| `red` | A test failing that passed at baseline. Pre-existing failures are not this feature's `red` |

The tag is a comment immediately above the test, in the host language's comment syntax,
containing the literal text `covers: SC-nn` (comma-separated for multiple rows). Only that
substring is load-bearing, so `//`, `#`, `--`, `/* */`, and docstrings all work and the grep
stays language-agnostic:

```go
// covers: SC-01, SC-03
func TestRejectsZeroQuantity(t *testing.T) {
```

```python
# covers: SC-02
def test_partial_write_rolls_back():
```

The tag is a comment rather than part of the test name because superpowers TDD requires
test names to describe behavior; IDs embedded in names make them unreadable and get dropped
during refactors.

### Layer 2 — `coverage-critic` subagent

Batched one subagent per behavior group; groups run in parallel. Each receives the SC rows,
the tagged test files, and **the production diff**. Tools: `Read`, `Grep`, `Glob` — no edit
tools, so it cannot repair a weak test instead of failing it.

The diff is necessary: *"would this test fail if the named bug were reintroduced"* is a
claim about production code, and a critic given only test files can judge test *shape* but
not that. Tool denial, not information denial, is what keeps it honest.

One verdict per row: `HOLDS`, `WEAK` (passes for the wrong reason), `VACUOUS` (asserts
nothing bound to the mutation). Defect patterns it hunts:

- assertion on a mock rather than on resulting state
- assertion on a value the implementation returns trivially
- assertion that still holds with the mutation applied

**Per-run canary.** Each batch includes one known-weak test injected by the main thread. A
batch that verdicts its canary `HOLDS` has its verdicts voided and re-dispatches at a higher
effort tier. Without this, an all-`HOLDS` critic passes forever after the one-time
dogfooding check.

### Layer 3 — `live-runner` subagent

Tools: `Bash`, `Read`. Brings the system up via **the project's own run recipe** — no other
source of environment. Executes every `smoke` and `e2e` row plus the derived walkthrough
set, redirecting real output into transcript files.

**Walkthroughs are derived, not fixed.** One walkthrough per distinct failure path present
at `smoke` or `e2e` level, plus one happy path. A feature with no such rows yields layer 3
`N/A` — not `SKIPPED`, not `PARTIAL`. Illustrative shape:

```
W0 happy      order accepted, event consumed, status CONFIRMED
W1 rejected   qty=0        → 400, no event published, no row
W2 dep down   pg stopped   → 503, event NOT acked, retried on recovery
W3 duplicate  same key 2x  → 1 row, 2nd call 200 idempotent
W4 timeout    consumer 30s → message requeued, no partial write
W5 parallel   N clients    → axis-5 rows under real contention
```

**Every walkthrough declares a machine-checkable expectation before it runs** — status code,
row count, log line, queue depth. The transcript must contain the command, its exit code,
and the grep proving the expectation. The main thread then re-greps the transcripts for
those tokens. Layer 3's summary is thereby auditable rather than authoritative:
`verification-before-completion/SKILL.md:55` names *"trusting agent success reports"* as a
red flag, and a self-graded `W2 PASS` is exactly that.

**Concurrency walkthroughs need repetition.** One green pass of a race test proves nothing.
W5-class walkthroughs require a stated iteration count with fail-if-any, a race detector, or
a seeded deterministic schedule. A single green run records `INCONCLUSIVE`, never `PASS`.

**Tamper check.** Before dispatch the main thread records `git rev-parse HEAD`,
`git stash list`, and a hash of `git diff`; after return it compares all three. Checking
`git status --porcelain` alone would miss a runner that commits, stashes, or edits and
reverts. `Bash` cannot be structurally constrained the way `coverage-critic` can, so the
constraint is verified rather than granted away.

### Layer 4 — `mutator` subagent (`risk=high` rows only)

For each `risk=high` row, on a throwaway branch:

1. Apply the mutation named in `Bug it catches`
2. Run **only the tagged test** — not the suite — and require FAIL
3. Revert; run it again and require PASS
4. Write both outputs to `transcripts/SC-nn-mutation.txt`

Verdicts: `MUTATION-CAUGHT` (blocking if absent) or `MUTATION-SURVIVED` (blocking). Running
only the tagged test is what keeps this affordable: cost scales with high-risk row count,
not with suite runtime.

This layer is the reason `Bug it catches` must be phrased as a mutation. A row saying
"qty=0 accepted" gives the mutator nothing to apply; "guard uses `<` instead of `<=`" is
directly executable.

### Verdicts

Three only: `PASS`, `PARTIAL`, `BLOCKED`.

**`BLOCKED`** on any: `gap`, unretired `drift`, unresolved `overclaim`, unresolved `stale`,
`red`, `WEAK`, `VACUOUS`, `MUTATION-SURVIVED`, or live `FAIL`.

**`PARTIAL`** when a layer could not run. `PARTIAL` requires **both**:

- a recorded human acknowledgement line — `PARTIAL accepted by <user> — <reason>`
- falsifiable evidence of the attempt: the commands tried, their exit codes, and the files
  looked for, written to the transcript directory

Without both, an un-runnable layer is opt-out by unverified declaration — and layer 3 is the
expensive one, so that is precisely where the pressure to declare will land.

**Re-run scope.** Layer 1 always re-runs in full: it is cheap, and a fix that repairs two
rows can break neighbors. Layers 2, 3, and 4 may be scoped to an ID subset. Every row
verdict carries the run it came from, so a report cannot silently mix a fresh subset with
stale inherited verdicts.

### Degradation

| Situation | Behavior |
|---|---|
| No approved spec (user enters mid-flow) | Generate stage runs; header records `Spec: none — derived from conversation`. Visible, not a blocker. |
| No smoke or e2e rows | Layer 3 is `N/A`. Not `SKIPPED`, not `PARTIAL`. |
| No run recipe, or dependencies unavailable | Layer 3 `SKIPPED` with attempt evidence → `PARTIAL`, requiring human acknowledgement. |
| Suite exists but cannot execute here (CI-only, licensed deps, hardware in the loop) | Layers 1 and 4 trace-only → `PARTIAL` with attempt evidence. |
| No test suite yet (legacy code) | Layer 1's gap set is scoped to rows whose behavior the diff actually touches. A legacy repo does not hard-block on rows the feature never went near. |
| Coverage doc missing at verify time | Verification refuses to run — there is no yardstick. |

### Report template

```markdown
# Verification: <feature>
Coverage: docs/superpowers/coverage/2026-07-30-orders-coverage.md
Baseline: a1b2c3d · Run: 2026-07-30T14:02Z
Verdict: BLOCKED — 2 gaps, 1 vacuous, 1 mutation survived, 1 live failure

## Layer 1 — trace  (full re-run)
20/23 rows tagged · suite 142 passed / 0 new failures vs baseline
gap:       SC-07 (no tagged test), SC-12 (tagged test reported SKIPPED by runner)
drift:     tag SC-99 in orders_test.go — not in doc, not retired
overclaim: orders_test.go:88 carries 4 SC-IDs — rows verdicted separately below
stale:     diff touches refund path — no SC row covers it

## Layer 2 — adversarial read  (canary caught in all 3 batches)
20 rows read: HOLDS 17 · WEAK 2 · VACUOUS 1
SC-05 VACUOUS  asserts mock.calledOnce, never asserts persisted state
SC-11 WEAK     holds with the `<`/`<=` mutation applied

## Layer 3 — live  (transcripts re-grepped by main thread)
smoke/e2e rows 6/6 PASS
W0 PASS · W1 PASS · W2 FAIL · W3 PASS · W4 PASS · W5 INCONCLUSIVE (1 iteration only)
W2 FAIL  pg stopped → 500 not 503, message NOT requeued — INV-1 at risk
transcripts: docs/superpowers/coverage/transcripts/

## Layer 4 — mutation  (risk=high rows)
SC-02 MUTATION-CAUGHT     rollback removed → test failed as required
SC-03 MUTATION-SURVIVED   uniqueness check moved outside tx → test still passed
```

## Repository layout

```
test-design/
  .claude-plugin/
    marketplace.json                      name: test-design
    plugin.json
  skills/
    test-coverage-design/
      SKILL.md                            floor rule, workflow, artifact template
      references/sweep-axes.md            the twelve axes + concurrency sub-axes
      references/level-assignment.md      pyramid rules + concurrency and rendering exceptions
      references/risk-derivation.md       axis→risk rule, argue-down protocol
      references/example-coverage-doc.md  one filled reference artifact
    coverage-verification/
      SKILL.md                            four layers, dispatch rules, verdicts, degradation
      references/runner-output.md         per-language machine-readable test report parsing
      references/example-verification.md
  agents/
    coverage-critic.md                    tools: Read, Grep, Glob
    live-runner.md                        tools: Bash, Read
    mutator.md                            tools: Bash, Read, Edit
  README.md
```

`mutator` is the one verification agent granted edit tools — mutation is its job. It is
confined to a throwaway branch and subject to the same tamper check as `live-runner`.

**Home:** `/home/igi21/experiements/test-design`, remote
`http://192.168.1.34:3000/service/test-design.git`.

Development install:

```bash
claude plugin marketplace add /home/igi21/experiements/test-design
claude plugin install test-design@test-design
```

## Enforcement

Skill-text `REQUIRED SUB-SKILL` handoffs — the same mechanism superpowers uses on itself —
plus one rule in `~/.claude/CLAUDE.md` that must **explicitly supersede** the conflicting
instruction rather than hope to outrank it:

```markdown
## Test design gate
`superpowers:brainstorming` states that the only skill it may invoke afterwards is
`writing-plans` (its SKILL.md: "The ONLY skill you invoke after brainstorming is
writing-plans"). That rule is amended here: the sequence is

  brainstorming → test-design:test-coverage-design → writing-plans

For features meeting test-coverage-design's floor rule, the coverage table is appended
inline to the spec instead, and brainstorming proceeds directly to writing-plans.

At the end of implementation, `superpowers:subagent-driven-development` hands off to
`finishing-a-development-branch`; invoke `test-design:coverage-verification` at that
handoff, before the branch is finished.
```

Naming the superseded line matters. Left unnamed, the design relies on the agent silently
preferring a global memory rule over an emphatic, locally-loaded skill instruction — and
the resulting nondeterministic skipping would read as "the stage got skipped" rather than
"two instructions collided".

## Modes

**Refactor mode.** A pure refactor's mutations all reduce to "the refactor changed
behavior", and the axis sweep yields nothing. The pre-existing suite is the yardstick. The
doc is then the invariant set plus the named characterization tests that must not change;
layers 2, 3, and 4 are `N/A` and layer 1 requires zero new failures against baseline.

**UI features** use axis 11 and the `component` level. Not out of scope — silence would be
worse than either including or excluding it, because a sweep with no rendering axis
confidently reports "ruled out" on real UI work.

**Data migrations** use axis 12. Axis 4's idempotency framing misfires for migrations that
are not re-runnable by nature; axis 12 covers resume and rolling-deploy compatibility
instead.

## Cost

Order-of-magnitude only, unmeasured until the dogfood runs. For a 5-behavior, ~18-row
feature the stage plausibly adds a 30–60% surcharge over a comparable SDD run, and the
wall-clock cost is worse than the token cost because stage 1 blocks on a human twice and
layers 3 and 4 serialize at the end. Layer 4 scales with high-risk row count and runs only
the tagged test, so it is bounded by row count rather than suite runtime.

Treat these as a hypothesis the dogfood tests, not as a budget. The floor rule is the
mitigation for small work; if measured cost lands above this range for medium work, the
next lever is narrowing which axes are swept by default.

## Dogfood plan

Two phases, cheap first. The plugin will be broken on first contact, and every plugin bug
costs one cycle of the host project's suite — so shake it out where a cycle costs seconds
before validating where a cycle costs tens of minutes.

**Phase 1 — `qmemd-a1r`** (`/home/igi21/experiements/qmemd`). Secret-pattern scan before
`gitCommit+gitPush` at `engine.ts:1561`. TypeScript, vitest, 29 test files including
concurrency and adversarial suites. Written acceptance criteria already exist.
Validates: floor rule, sweep, risk derivation, layers 1, 2, 4.
Cannot validate: duplicate delivery is genuinely N/A; dependency-down is thin (git remote
and embed host, no broker or DB); layer 3 has little to exercise.

**Phase 2 — `ofacservice-3s6`** (`/home/igi21/work/ofacservice`). Archive sanctions lists to
docregistry. Idempotency is the subject matter — sha256 change-detection, and docregistry
`/store` has no idempotency key. Dependency-down and timeout apply to docregistry HTTP;
concurrency applies via `ScheduleLocker`. A `-Psmoke` profile and a local-dev runbook exist.
Validates: layer 3 end to end, axes 3/4/5 for real, the `PARTIAL` path, compiled-language
re-run economics.
Known cost: its beads tracker is stale, so "pending" rests on a code check; the issue also
carries 5 unresolved design questions, which means real brainstorming precedes the coverage
stage — itself a fuller exercise of the flow.

**Rejected:** `checknewpassport-4y5` (add UAE sanctions flag). Adding a JAXB field pair and
propagating a boolean introduces no shared mutable state, no second writer, and no lock, so
axis 5 does not bite and the sweep would produce exactly the coverage theater this stage
exists to catch. It also sits on an active branch, coupling plugin risk to delivery risk,
and its live sanctions path on the sandbox is currently dead pending fixture work in another
repo.

### Acceptance criteria

Falsifiability is the test. Documents are trivially producible, so each gate must be shown
to catch a planted defect:

- [ ] Coverage doc has a `Ruled out:` line per behavior group, each citing a spec or code fact
- [ ] Every row's `Bug it catches` is phrased as a code-level mutation, not a negated `Expected`
- [ ] Every axis-3/4/5/6 row is `high`, or carries a user-approved written argue-down
- [ ] Every axis-5 row is `integration` or above
- [ ] Plan tasks produced by `writing-plans` cite SC-IDs in their test steps
- [ ] **Planted:** an untagged test → layer 1 reports `gap`
- [ ] **Planted:** a tagged but `skip`-marked test → layer 1 reports `gap`, not coverage
- [ ] **Planted:** a test asserting on a mock → critic returns `VACUOUS` or `WEAK`
- [ ] **Planted:** a batch canary → critic catches it; a batch that misses it is voided and re-dispatched
- [ ] **Planted:** a high-risk row whose test does not bite → layer 4 reports `MUTATION-SURVIVED`
- [ ] **Planted:** a dep-down path returning 500 instead of 503 → layer 3 reports `FAIL`
- [ ] A single-iteration concurrency walkthrough records `INCONCLUSIVE`, not `PASS`
- [ ] A feature meeting the floor rule produces no separate coverage doc
- [ ] A skipped layer 3 cannot reach `PASS`, and `PARTIAL` requires the acknowledgement line

Any plant that survives means that gate is theater. Cut it rather than ship it weak.

## Known limitations

**The doc is the yardstick.** Verification measures the implementation against the coverage
doc. It cannot detect bugs the doc never anticipated — a thin doc yields a confident,
useless green report. This is why the generate stage is interactive rather than autonomous.
No amount of verification rigor compensates for a lazy sweep.

**Mutation evidence does not prove test-first.** Layer 4 shows a test catches its named
mutation, not that it was written before the code. TDD discipline remains enforced by
superpowers, not by this plugin.

**Verification arrives after SDD's code reviews.** Because the only available seam is
`finishing-a-development-branch`, a `BLOCKED` verdict lands after the per-task and final
reviews have already run.

## Deferred

- Soft warn-hook on writes to `docs/superpowers/plans/*`. Hooks can run arbitrary commands
  and could perform the full tag-and-report check; the reason to defer is the unknown
  false-denial rate, not any capability limit.
- TestIT / Kaiten export adapter for ATON work.
- Reconciliation with the Russian skills in the `qa` repo. `checklist-from-feature` overlaps
  in intent. Both are left alone; no cross-repo synchronization.
- Narrowing the default axis set if measured cost exceeds the estimate.

## Revision history

**Revision 2 (2026-07-30)** — adversarial review of revision 1 found seven mechanisms with a
cheap pass producing a clean report over nothing; three were verified against superpowers
source. Changes: RED-capture replaced by layer 4 mutation (the evidence was git-ignored and
`rm -rf`'d before verification ran); layer 1 now consumes machine-readable runner output (a
tag above a skipped test satisfied grep and left the suite green); `Risk` derived from axis
rather than self-assigned (the agent bearing the cost was also setting it); insertion point
moved to `finishing-a-development-branch` (no seam existed before `requesting-code-review`);
`PARTIAL` now requires human acknowledgement plus attempt evidence; layer 3 expectations
pre-declared and transcripts re-grepped by the main thread; `coverage-critic` given the
production diff; floor rule added; ceiling clarified as post-prune; baseline added so legacy
repos do not hard-block; axes 11 and 12 and the `component` level added; homelab clause
deleted as contradicting the plugin's portability principle; hook-deferral rationale
corrected — hooks are not limited to file-existence checks, the argument is false-denial
rate alone.

Not adopted: cutting the `Expected` column. It duplicates test-body detail and will drift,
but it is where the human confirms the intended contract before code exists. `Input /
preconditions` was cut instead.
