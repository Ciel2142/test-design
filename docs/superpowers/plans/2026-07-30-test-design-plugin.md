# test-design Plugin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Claude Code plugin that adds two mandatory stages to the superpowers flow — coverage design before planning, and four-layer coverage verification after implementation.

**Architecture:** The plugin is pure markdown plus JSON manifests: two skills, three agent definitions, and reference files. It carries no runtime code of its own. Every mechanical step (parsing test-runner output, extracting test names, applying mutations) is a shell one-liner prescribed verbatim in a reference file, using tools the target project already has. A fixture project inside the plugin repo makes the verification layers testable without dogfooding.

**Tech Stack:** Markdown, JSON, bash, `jq` (JSON runner reports), `python3` stdlib `xml.etree` (JUnit XML reports), `vitest` (fixture project only).

## Global Constraints

- Plugin name `test-design`; marketplace name `test-design`.
- Skill names exactly `test-coverage-design`, `coverage-verification`. Agent names exactly `coverage-critic`, `live-runner`, `mutator`.
- All plugin content in English.
- The plugin ships **no runtime dependency of its own**. Mechanical steps use `jq` where the runner emits JSON and `python3` stdlib where it emits JUnit XML — both already present in any project using those runners.
- Artifacts are written to the **target repo** under `docs/superpowers/coverage/`, never into the plugin repo. In a monorepo these paths are package-relative.
- Normalized runner contract: lines of `status<TAB>test-name`, where status is exactly one of `passed`, `failed`, `skipped`.
- SC-ID format `SC-nn`, or `<area>/SC-nn` in a monorepo. Never renumbered; retired IDs move to `## Retired`.
- Tag format: a comment immediately above the test containing the literal text `covers: SC-nn`, comma-separated for multiple. More than 2 IDs on one test is an `overclaim` finding.
- Row schema exactly `| ID | Behavior | Bug it catches | Level | Expected | Risk |`.
- `Bug it catches` must be phrased as a code-level mutation.
- Risk is derived, not judged: rows on axes 3, 4, 5, 6 are `high` by rule, as is any row whose mutation names data loss, corruption, or exposure. Argue-down requires recorded user approval.
- Levels exactly `unit | component | integration | smoke | e2e`.
- Verdicts exactly `PASS | PARTIAL | BLOCKED`. `PARTIAL` requires a human acknowledgement line plus attempt evidence.
- Verification is invoked at the `superpowers:finishing-a-development-branch` seam.
- Post-prune row ceiling ~40; typical 8–25.

**Checkpoint:** after Task 7 the generate stage is complete and usable on its own. Tasks 8–12 build the verify stage. Stopping at 7 leaves working software.

---

### Task 1: Plugin skeleton and manifests

**Files:**
- Create: `.claude-plugin/marketplace.json`
- Create: `.claude-plugin/plugin.json`
- Create: `README.md`
- Create: `.gitignore`

**Interfaces:**
- Consumes: nothing.
- Produces: an installable plugin whose skills resolve as `test-design:test-coverage-design` and `test-design:coverage-verification`. Later tasks add the skill directories those names point at.

- [ ] **Step 1: Write the marketplace manifest**

`.claude-plugin/marketplace.json`:

```json
{
  "name": "test-design",
  "owner": {
    "name": "AT_VALukin"
  },
  "plugins": [
    {
      "name": "test-design",
      "source": "./",
      "description": "Coverage design before implementation, four-layer coverage verification after."
    }
  ]
}
```

- [ ] **Step 2: Write the plugin manifest**

`.claude-plugin/plugin.json`:

```json
{
  "name": "test-design",
  "version": "0.1.0",
  "description": "Adds two stages to the superpowers flow: design test coverage before planning, verify it after implementation.",
  "author": {
    "name": "AT_VALukin"
  }
}
```

- [ ] **Step 3: Write `.gitignore`**

```
node_modules/
*.log
/tmp/
fixtures/sample-project/coverage/
fixtures/sample-project/.vitest-report.json
```

- [ ] **Step 4: Write the README**

`README.md`:

```markdown
# test-design

A Claude Code plugin adding two stages to the [superpowers](https://github.com/obra/superpowers) flow.

| Stage | When | What it produces |
|---|---|---|
| `test-coverage-design` | after the spec is approved, before `writing-plans` | a coverage doc: invariants, out-of-scope, and an ID'd scenario table |
| `coverage-verification` | at the `finishing-a-development-branch` seam | a verification report across four layers |

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

## Install

```bash
claude plugin marketplace add /home/igi21/experiements/test-design
claude plugin install test-design@test-design
```

Then add the gate rule from `docs/enforcement.md` to your `~/.claude/CLAUDE.md`.

## Design

See `docs/superpowers/specs/2026-07-30-test-design-plugin-design.md`.
```

- [ ] **Step 5: Verify the manifests parse**

Run: `jq -e . .claude-plugin/marketplace.json .claude-plugin/plugin.json`
Expected: both files echoed, exit code 0. A syntax error exits non-zero.

- [ ] **Step 6: Verify the plugin installs**

Run:
```bash
claude plugin marketplace add /home/igi21/experiements/test-design
claude plugin install test-design@test-design
```
Expected: install succeeds. The plugin has no skills yet, so none are listed — that is correct at this stage. If install fails, the manifest schema is wrong; fix before proceeding.

- [ ] **Step 7: Commit**

```bash
git add .claude-plugin README.md .gitignore
git commit -m "feat: plugin skeleton and manifests"
```

---

### Task 2: Fixture project with planted defects

**Files:**
- Create: `fixtures/sample-project/package.json`
- Create: `fixtures/sample-project/src/orders.ts`
- Create: `fixtures/sample-project/src/server.ts`
- Create: `fixtures/sample-project/test/orders.test.ts`
- Create: `fixtures/README.md`

**Interfaces:**
- Consumes: nothing.
- Produces: a runnable project emitting a vitest JSON report at `.vitest-report.json`, containing four deliberately-shaped tests that later tasks assert against by name:
  - `rejects zero quantity` — tagged `SC-01`, passes, genuinely bites
  - `rolls back on mid-write failure` — tagged `SC-02`, **skipped** via `it.skip`
  - `creates an order` — tagged `SC-03`, passes but asserts only on a spy (vacuous)
  - `computes total` — **untagged**, passes
  Plus an HTTP surface on port 3999 with a `/orders` endpoint and a `GET /health` for the live layer, and a `FAIL_DB=1` env switch that makes the write path fail so a dep-down walkthrough has something to exercise.

- [ ] **Step 1: Write the fixture package manifest**

`fixtures/sample-project/package.json`:

```json
{
  "name": "sample-project",
  "private": true,
  "type": "module",
  "scripts": {
    "test": "vitest run --reporter=json --outputFile=.vitest-report.json",
    "start": "node --experimental-strip-types src/server.ts"
  },
  "devDependencies": {
    "vitest": "^3.0.0"
  }
}
```

- [ ] **Step 2: Write the production code with a real guard to mutate**

`fixtures/sample-project/src/orders.ts`:

```typescript
export type Order = { id: number; qty: number };

const rows: Order[] = [];
let nextId = 1;

export function createOrder(qty: number, onWrite?: () => void): Order {
  if (qty <= 0) {
    throw new Error("qty must be > 0");
  }
  const order = { id: nextId++, qty };
  rows.push(order);
  try {
    onWrite?.();
  } catch (err) {
    rows.pop();
    throw err;
  }
  return order;
}

export function total(): number {
  return rows.reduce((sum, r) => sum + r.qty, 0);
}

export function count(): number {
  return rows.length;
}

export function reset(): void {
  rows.length = 0;
  nextId = 1;
}
```

- [ ] **Step 3: Write the HTTP surface for the live layer**

`fixtures/sample-project/src/server.ts`:

```typescript
import { createServer } from "node:http";
import { createOrder, count } from "./orders.ts";

const failDb = process.env.FAIL_DB === "1";

const server = createServer((req, res) => {
  if (req.url === "/health") {
    res.writeHead(200).end("ok");
    return;
  }
  if (req.method === "POST" && req.url === "/orders") {
    let body = "";
    req.on("data", (c) => (body += c));
    req.on("end", () => {
      let qty: number;
      try {
        qty = JSON.parse(body || "{}").qty;
      } catch {
        res.writeHead(400).end(JSON.stringify({ error: "bad json" }));
        return;
      }
      try {
        const order = createOrder(qty, () => {
          if (failDb) throw new Error("db unavailable");
        });
        res.writeHead(201).end(JSON.stringify(order));
      } catch (err) {
        const message = (err as Error).message;
        const status = message === "db unavailable" ? 503 : 400;
        res.writeHead(status).end(JSON.stringify({ error: message, rows: count() }));
      }
    });
    return;
  }
  res.writeHead(404).end();
});

server.listen(3999, () => console.log("listening on 3999"));
```

- [ ] **Step 4: Write the four deliberately-shaped tests**

`fixtures/sample-project/test/orders.test.ts`:

```typescript
import { describe, it, expect, vi, beforeEach } from "vitest";
import { createOrder, total, count, reset } from "../src/orders.ts";

beforeEach(() => reset());

describe("orders", () => {
  // covers: SC-01
  it("rejects zero quantity", () => {
    expect(() => createOrder(0)).toThrow("qty must be > 0");
    expect(count()).toBe(0);
  });

  // covers: SC-02
  it.skip("rolls back on mid-write failure", () => {
    expect(() =>
      createOrder(3, () => {
        throw new Error("boom");
      }),
    ).toThrow("boom");
    expect(count()).toBe(0);
  });

  // covers: SC-03
  it("creates an order", () => {
    const spy = vi.fn();
    createOrder(2, spy);
    expect(spy).toHaveBeenCalledOnce();
  });

  it("computes total", () => {
    createOrder(2);
    createOrder(3);
    expect(total()).toBe(5);
  });
});
```

- [ ] **Step 5: Document what each planted defect is for**

`fixtures/README.md`:

```markdown
# Fixtures

`sample-project/` exists so the verification layers can be tested without dogfooding.
Every oddity in it is deliberate. Do not "fix" them.

| Test | Tag | Shape | Which layer it proves |
|---|---|---|---|
| `rejects zero quantity` | `SC-01` | passes, asserts thrown error **and** that no row persisted | layer 4: mutating `qty <= 0` to `qty < 0` must make it fail |
| `rolls back on mid-write failure` | `SC-02` | `it.skip` | layer 1: a tag above a skipped test must yield `gap`, not coverage |
| `creates an order` | `SC-03` | passes, asserts only `toHaveBeenCalledOnce` on a spy | layer 2: must be verdicted `VACUOUS` |
| `computes total` | none | passes | layer 1: an untagged test is not a finding by itself; it exists to prove the tag grep does not over-match |

`src/server.ts` provides the live surface: `GET /health`, `POST /orders`.
With `FAIL_DB=1` the write path throws and the endpoint answers `503` with `rows: 0` —
that is the dep-down walkthrough. Without it, `POST /orders {"qty":3}` answers `201`.

The mutation target for `SC-01` is `src/orders.ts` line with `if (qty <= 0)`.
```

- [ ] **Step 6: Install and run the fixture suite**

Run:
```bash
cd fixtures/sample-project && npm install && npm test; echo "exit=$?"
```
Expected: vitest reports `3 passed | 1 skipped`, writes `.vitest-report.json`, exit code 0.

- [ ] **Step 7: Confirm the JSON report distinguishes skipped from passed**

Run:
```bash
cd fixtures/sample-project && jq -r '.testResults[].assertionResults[] | "\(.status)\t\(.fullName)"' .vitest-report.json
```
Expected output contains a line whose status is `skipped` and whose name ends `rolls back on mid-write failure`, plus three `passed` lines. If vitest reports skipped tests as `pending` rather than `skipped`, record the actual value — Task 3 normalizes it.

- [ ] **Step 8: Verify the live surface answers both paths**

Run:
```bash
cd fixtures/sample-project
npm start & sleep 1
curl -s -o /dev/null -w '%{http_code}\n' -X POST localhost:3999/orders -d '{"qty":3}'
kill %1
FAIL_DB=1 npm start & sleep 1
curl -s -w '\n%{http_code}\n' -X POST localhost:3999/orders -d '{"qty":3}'
kill %1
```
Expected: `201` for the first, then a body containing `"rows":0` and status `503` for the second.

- [ ] **Step 9: Commit**

```bash
git add fixtures
git commit -m "test: fixture project with planted coverage defects"
```

---

### Task 3: Normalized runner-output contract

**Files:**
- Create: `skills/coverage-verification/references/runner-output.md`

**Interfaces:**
- Consumes: the fixture report from Task 2.
- Produces: the normalized contract layer 1 consumes — stdout lines of `status<TAB>test-name`, status in `passed|failed|skipped` — plus one verbatim command per runner that emits it, and one test-name-extraction regex per language. Task 6's layer 1 algorithm depends on both.

- [ ] **Step 1: Write the reference file**

`skills/coverage-verification/references/runner-output.md`:

```markdown
# Runner output: normalized contract

Layer 1 must distinguish *executed and passed* from *skipped, filtered out, or absent*.
A grep over source plus a green exit code cannot do that: a tag above a skipped test
satisfies the grep and leaves the suite green.

## The contract

Every command below prints one line per test:

    status<TAB>test-name

`status` is exactly `passed`, `failed`, or `skipped`. Anything a runner calls `pending`,
`todo`, `ignored`, or `disabled` normalizes to `skipped`. A test named in a tag but absent
from this output is a `gap`.

## Emitting the contract

### vitest / jest

```bash
npx vitest run --reporter=json --outputFile=.vitest-report.json
jq -r '.testResults[].assertionResults[]
       | "\(if .status == "pending" or .status == "todo" then "skipped" else .status end)\t\(.fullName)"' \
   .vitest-report.json
```

### go

```bash
go test -json ./... > .go-report.json
jq -r 'select(.Test != null and (.Action == "pass" or .Action == "fail" or .Action == "skip"))
       | "\(if .Action == "pass" then "passed" elif .Action == "fail" then "failed" else "skipped" end)\t\(.Test)"' \
   .go-report.json
```

### pytest

```bash
pytest --junitxml=.pytest-report.xml
```

Then the JUnit XML reader below.

### maven / gradle (surefire, failsafe)

```bash
mvn -q test                     # or: mvn -q -Pit-full verify
```

Then the JUnit XML reader below, globbing `target/surefire-reports/*.xml` and
`target/failsafe-reports/*.xml`.

### JUnit XML reader (pytest, maven, gradle, and anything else emitting JUnit XML)

```bash
python3 - '<glob>' <<'PY'
import sys, glob, xml.etree.ElementTree as ET
for path in glob.glob(sys.argv[1]):
    head = open(path, 'rb').read(4096).lower()
    if b'<!doctype' in head or b'<!entity' in head:
        print(f"skipped-unsafe\t{path}: DTD/ENTITY declaration present", file=sys.stderr)
        continue
    for tc in ET.parse(path).getroot().iter('testcase'):
        name = f"{tc.get('classname')}.{tc.get('name')}"
        if tc.find('skipped') is not None:
            status = 'skipped'
        elif tc.find('failure') is not None or tc.find('error') is not None:
            status = 'failed'
        else:
            status = 'passed'
        print(f"{status}\t{name}")
PY
```

`python3` and its stdlib `xml.etree` are the only requirement; no pip install.

**The DTD guard is not optional.** `xml.etree` expands internal entities, so a report
containing nested `ENTITY` declarations can blow up memory (billion laughs / quadratic
blowup). No legitimate JUnit report needs a DTD, so refusing any file that declares one
costs nothing and removes the class entirely — without pulling in `defusedxml` and breaking
the plugin's zero-dependency property.

Threat model, stated honestly: this parses the project's own test-runner output on a
developer machine, so exploiting it requires already controlling that output — a malicious
test dependency, or a report fetched from an untrusted CI artifact. Defense in depth, not a
live hole. A report rejected by the guard is reported to stderr and its tests count as
absent from the run, which surfaces as `gap` rather than as a silent pass.

## Extracting the test name a tag refers to

The tag sits immediately above a test declaration. Read forward from the tag to the next
non-blank line and extract the name with the pattern for that language. Join to the runner
output by **suffix match** on the name, because runners prefix with suite, class, or file.

| Language | Pattern | Example match |
|---|---|---|
| Go | `^func (Test\w+)` | `TestRejectsZeroQuantity` |
| Python | `^\s*def (test_\w+)` | `test_partial_write_rolls_back` |
| TS/JS | `^\s*(?:it\|test)(?:\.\w+)?\(\s*['"\`](.+?)['"\`]` | `rejects zero quantity` |
| Java/Kotlin | `(?:void\|fun) (\w+)\s*\(` on the line after `@Test` | `rejectsZeroQuantity` |
| Rust | `^\s*fn (\w+)` on the line after `#[test]` | `rejects_zero_quantity` |

Note the TS/JS pattern deliberately allows a modifier — `it.skip(`, `it.only(`,
`test.todo(` all match, so a skipped test is still *found* and then reported `skipped`
by the runner output. A pattern requiring `it(` exactly would silently miss it and the
row would read as `gap` for the wrong reason, hiding the real problem.

## Unrunnable suites

If the suite cannot execute in this environment (CI-only, licensed dependency, hardware in
the loop), do not fabricate the contract. Record the commands attempted and their exit
codes as attempt evidence, and let the verdict fall to `PARTIAL`.
```

- [ ] **Step 2: Verify the vitest command emits the contract against the fixture**

Run:
```bash
cd fixtures/sample-project && jq -r '.testResults[].assertionResults[]
  | "\(if .status == "pending" or .status == "todo" then "skipped" else .status end)\t\(.fullName)"' \
  .vitest-report.json
```
Expected: exactly four lines; three `passed`, one `skipped`; the `skipped` line's name ends with `rolls back on mid-write failure`. If the status value differs from what Task 2 step 7 recorded, fix the `jq` normalization here, not the fixture.

- [ ] **Step 3: Verify the TS name-extraction pattern finds all four tests including the skipped one**

Run:
```bash
cd fixtures/sample-project && grep -nE "^\s*(it|test)(\.\w+)?\(\s*['\"\`]" test/orders.test.ts
```
Expected: four matches, one of which is the `it.skip(` line. A pattern that misses `it.skip` is the failure this step exists to catch.

- [ ] **Step 4: Verify the JUnit XML reader against a synthetic report**

Run:
```bash
mkdir -p /tmp/junit-check && cat > /tmp/junit-check/r.xml <<'XML'
<testsuite name="s">
  <testcase classname="C" name="passes"/>
  <testcase classname="C" name="is_skipped"><skipped/></testcase>
  <testcase classname="C" name="fails"><failure message="m"/></testcase>
</testsuite>
XML
python3 - '/tmp/junit-check/*.xml' <<'PY'
import sys, glob, xml.etree.ElementTree as ET
for path in glob.glob(sys.argv[1]):
    head = open(path, 'rb').read(4096).lower()
    if b'<!doctype' in head or b'<!entity' in head:
        print(f"skipped-unsafe\t{path}", file=sys.stderr)
        continue
    for tc in ET.parse(path).getroot().iter('testcase'):
        name = f"{tc.get('classname')}.{tc.get('name')}"
        if tc.find('skipped') is not None:
            status = 'skipped'
        elif tc.find('failure') is not None or tc.find('error') is not None:
            status = 'failed'
        else:
            status = 'passed'
        print(f"{status}\t{name}")
PY
```
Expected exactly:
```
passed	C.passes
skipped	C.is_skipped
failed	C.fails
```

- [ ] **Step 5: Commit**

```bash
git add skills/coverage-verification/references/runner-output.md
git commit -m "feat: normalized runner-output contract with per-runner commands"
```

---

### Task 4: Sweep axes reference

**Files:**
- Create: `skills/test-coverage-design/references/sweep-axes.md`

**Interfaces:**
- Consumes: nothing.
- Produces: the twelve axes and axis-5 sub-axes that Task 6's workflow sweeps, and the rule that ruling out an axis must cite a spec or code fact. Task 5 references axis numbers 3, 4, 5, 6 for risk derivation and 5, 11 for level exceptions.

- [ ] **Step 1: Write the reference file**

`skills/test-coverage-design/references/sweep-axes.md`:

```markdown
# Sweep axes

Sweep **every behavior against all twelve axes**. The default failure mode this exists to
prevent is happy path plus one null check, then declaring done.

An axis is either swept into rows or explicitly ruled out. Silent omission is forbidden.

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

## Axis 5 sub-axes

Enumerated separately because concurrency is the most-skipped axis and the least
catchable by a unit test.

- two writers on the same entity (lost update)
- check-then-act / TOCTOU (read-modify-write with no lock)
- out-of-order delivery (event B arrives before event A)
- duplicate delivery (at-least-once broker semantics)
- transaction isolation anomalies (dirty read, phantom, write skew)
- deadlock / inconsistent lock ordering
- concurrent initialization, cache stampede, double-start
- cancellation mid-flight (client aborts, context cancelled)
- shared mutable state across threads, goroutines, or async tasks

## Ruling out an axis

Every ruled-out axis needs a reason **citing a spec fact or a code fact**, not a judgment.

<Good>
`Ruled out: time (spec defines no date logic), auth (internal endpoint, spec §3 states no
actor concept), migration (no schema change — no new Liquibase changeset in this feature)`
</Good>

<Bad>
`Ruled out: time (n/a), auth (n/a), concurrency (no shared state)`
</Bad>

`no shared state` is unfalsifiable at design time, and axis 5 is the flagship — dismissing
it in one unchecked clause is the single easiest way to gut the sweep. **Ruling out axis 5
specifically requires naming the shared resource that is absent:** no table written, no
cache, no lock acquired, no queue consumer, no static or module-level mutable value.

## Pruning

Sweeping twelve axes across several behaviors generates far more candidates than any plan
should carry. Prune hard, immediately after sweeping:

1. Each candidate must name **one distinct code-level mutation** it would catch.
2. A candidate whose mutation cannot be named is deleted.
3. Two candidates naming the same mutation are merged; the loser's ID is retired.

The ceiling of roughly forty rows applies to survivors, never to candidates. A ceiling on
candidates would reward under-sweeping, which is backwards.
```

- [ ] **Step 2: Verify all twelve axes and nine sub-axes are present**

Run:
```bash
grep -cE '^\| [0-9]+ \|' skills/test-coverage-design/references/sweep-axes.md
grep -cE '^- ' skills/test-coverage-design/references/sweep-axes.md
```
Expected: `12` for the first. The second counts the sub-axis bullets plus the pruning list — confirm at least 9 sub-axis bullets appear under "Axis 5 sub-axes" by eye.

- [ ] **Step 3: Commit**

```bash
git add skills/test-coverage-design/references/sweep-axes.md
git commit -m "feat: twelve sweep axes with citation requirement for rule-outs"
```

---

### Task 5: Risk derivation and level assignment references

**Files:**
- Create: `skills/test-coverage-design/references/risk-derivation.md`
- Create: `skills/test-coverage-design/references/level-assignment.md`

**Interfaces:**
- Consumes: axis numbers from Task 4.
- Produces: the rule that maps axis → `Risk`, the argue-down protocol, the five level values, and the concurrency and rendering exceptions. Task 6 applies both in workflow steps 4 and 5; Task 9's layer 4 fires only on `risk=high` rows.

- [ ] **Step 1: Write the risk derivation reference**

`skills/test-coverage-design/references/risk-derivation.md`:

```markdown
# Risk derivation

`Risk` is **derived by rule, never judged**. It takes exactly `low`, `med`, or `high`.

## The rule

- Rows on axes **3 (failure paths), 4 (idempotency/retry), 5 (concurrency), 6 (auth)** are `high`.
- Any row whose mutation names **data loss, data corruption, or exposure** is `high`.
- Everything else is `med`, or `low` where the failure is purely cosmetic.

## Why it is not a judgment call

`high` is the only load-bearing value: it obliges the row to carry layer 4 mutation
evidence at verification. An agent that both assigns risk and later bears its cost has a
free, untraceable way to switch that gate off — mark every row `med` and layer 4 never
fires. A human reviewing a twenty-row table audits scenarios, not risk labels, so nobody
notices. Under context pressure this is not even deliberate; it is the path of least
friction.

Deriving from the axis removes the discretion that makes that possible.

## Argue-down protocol

The agent may propose lowering a row from `high` to `med`. It may never raise its own
burden, and never lower one silently.

1. State the row, the rule that made it `high`, and the concrete reason the rule misfires here.
2. Get explicit user approval.
3. Record it in the coverage doc under the behavior group:

```
Risk argued down: SC-07 high→med — axis 4 row, but the endpoint is a pure read with no
write path, so a duplicate call has no side effect to be idempotent about. Approved by user.
```

An argue-down with no recorded approval line is invalid; verification treats the row as
`high`.
```

- [ ] **Step 2: Write the level assignment reference**

`skills/test-coverage-design/references/level-assignment.md`:

```markdown
# Level assignment

`Level` takes exactly `unit`, `component`, `integration`, `smoke`, or `e2e`.

**Assign the cheapest level that can actually catch the named mutation.** Never a higher
level out of habit — an e2e test for an off-by-one guard is slow, flaky, and no more
truthful than the unit test.

| Level | Catches | Cost |
|---|---|---|
| `unit` | logic in one unit, no I/O | milliseconds |
| `component` | a rendered component's state and interaction | fast, in-process |
| `integration` | real collaborators — DB, broker, lock, transaction | seconds |
| `smoke` | the deployed surface answers correctly | needs the system up |
| `e2e` | a full path across services | slowest, most fragile |

Rows at `component`, `smoke`, or `e2e` need a harness hint: environment, fixtures, what is
touched. Layer 3 executes the `smoke` and `e2e` rows.

## Concurrency exception

**Rows on axis 5 default to `integration` or above.**

A unit test against a mock cannot produce real contention. Two mocked "concurrent" calls
execute sequentially in one thread, so the test passes whether or not the lock exists —
marking such a row covered is false confidence, which is worse than an admitted gap.

The only exception: code with an **injectable scheduler or clock**, where the interleaving
is deterministic and chosen by the test. Then `unit` is honest, because the test really can
produce the interleaving that triggers the bug.

## Rendering exception

**Rows on axis 11 default to `component` or above.**

Same argument: a jsdom environment is a mock of a browser. Layout, focus order, and
breakpoint behavior asserted against jsdom prove that jsdom agrees, not that a browser
does. `unit` is not an honest home for a rendering claim.
```

- [ ] **Step 3: Verify both files state their exact enums**

Run:
```bash
grep -c 'high' skills/test-coverage-design/references/risk-derivation.md
grep -E 'unit.*component.*integration.*smoke.*e2e' skills/test-coverage-design/references/level-assignment.md
```
Expected: a non-zero count, and one line listing all five levels.

- [ ] **Step 4: Commit**

```bash
git add skills/test-coverage-design/references/risk-derivation.md skills/test-coverage-design/references/level-assignment.md
git commit -m "feat: axis-derived risk rule and level assignment with mock-contention exceptions"
```

---

### Task 6: `test-coverage-design` skill

**Files:**
- Create: `skills/test-coverage-design/SKILL.md`

**Interfaces:**
- Consumes: `references/sweep-axes.md`, `references/risk-derivation.md`, `references/level-assignment.md` from Tasks 4–5.
- Produces: the workflow, the floor rule, the row schema `| ID | Behavior | Bug it catches | Level | Expected | Risk |`, the `## Retired` section, the amendment protocol, and the artifact path `<repo>/docs/superpowers/coverage/YYYY-MM-DD-<feature>-coverage.md`. Task 7 supplies the worked example it points at; Task 9's layer 1 parses this artifact.

- [ ] **Step 1: Write the skill**

`skills/test-coverage-design/SKILL.md`:

```markdown
---
name: test-coverage-design
description: Use after a spec is approved and before writing an implementation plan - designs the test coverage the feature needs, as an ID'd scenario table that the plan and later verification both cite
---

# Test Coverage Design

## Overview

Decide what must be tested **before** tasks get drawn. Output is one coverage doc whose
rows carry stable IDs that the plan's test steps cite and that `coverage-verification`
later measures the implementation against.

**Announce at start:** "I'm using the test-coverage-design skill to design the coverage for
this feature."

**Mode:** interactive. This stage needs the user's judgment on applicability, risk, and
scope. Do not run it silently to completion.

## Floor rule — when to skip this stage

If the feature has **≤2 behaviors and no persistence, no concurrency, and no external
dependency**, do not produce a separate document. Append the scenario table inline to the
spec at brainstorming's spec-review gate and proceed to `superpowers:writing-plans`.

Below that size the rows are the plan's test steps restated, and a separate doc costs two
human gates to re-derive them. Say which rule applied: "Floor rule applies — 2 behaviors,
no persistence. Appending the table to the spec instead."

## Workflow

1. **Extract behaviors.** Read the approved spec. List every externally-observable
   capability. **Confirm the list with the user before sweeping** — a wrong behavior list
   makes every downstream row wrong, and this is the cheapest possible moment to fix it.
2. **Sweep.** Each behavior against all twelve axes in
   [references/sweep-axes.md](references/sweep-axes.md). Emit candidates.
3. **Prune.** Each survivor names one distinct code-level mutation. Cannot name one →
   delete. Same mutation twice → merge, retire the loser's ID.
4. **Derive risk** per [references/risk-derivation.md](references/risk-derivation.md).
   Never judge it; argue down only with recorded user approval.
5. **Assign level** per [references/level-assignment.md](references/level-assignment.md).
   Cheapest level that catches the mutation; axis 5 rows are `integration`+, axis 11 rows
   are `component`+.
6. **Add harness hints** for `component`, `smoke`, and `e2e` rows.
7. **Record ruled-out axes** per behavior, each citing a spec fact or a code fact.
8. **Extract invariants** — properties holding across all scenarios, `INV-n`.
9. **List out-of-scope** items, each with a reason.
10. **Capture the baseline** — `git merge-base HEAD <default-branch>` and the suite result
    at that commit. Verification compares against it so pre-existing failures are not
    charged to this feature.
11. Present to the user, get approval, write and commit.

## Row schema

`| ID | Behavior | Bug it catches | Level | Expected | Risk |`

No other columns. Priority, mission, and grouping live outside the table.

| Column | Rule |
|---|---|
| `ID` | `SC-nn`, or `<area>/SC-nn` in a monorepo where `<area>` is the package the grep and suite scope to. Never renumbered. |
| `Behavior` | What is under test, one clause |
| `Bug it catches` | A **code-level mutation**: wrong operator, missing guard, swapped argument, absent await, inverted condition, omitted rollback |
| `Level` | `unit \| component \| integration \| smoke \| e2e` |
| `Expected` | The observable contract, specific enough to check mechanically |
| `Risk` | `low \| med \| high`, derived |

### `Bug it catches` must be a mutation, not a negation

<Good>
`guard uses < instead of <=` · `rollback call omitted on the error path` ·
`uniqueness check runs outside the transaction (TOCTOU)` · `await dropped on the publish call`
</Good>

<Bad>
`qty=0 is accepted` · `the write is not rolled back` · `a duplicate order is created`
</Bad>

The bad forms are the `Expected` column negated. That phrasing auto-fills for every row,
which makes the pruning rule vacuous — and it gives layer 4's mutator nothing to apply.
Every row must name something a person could actually type into the source.

## Artifact

Write to the **repository being worked on**, never into the plugin repo:

`<repo>/docs/superpowers/coverage/YYYY-MM-DD-<feature>-coverage.md`

In a monorepo where packages own their docs, the path is package-relative and IDs carry the
`<area>/` prefix.

See [references/example-coverage-doc.md](references/example-coverage-doc.md) for a filled
artifact. Use it for structure, never as the scope of a new feature.

## Size discipline

Smallest set naming distinct mutations. Typical feature: 8–25 rows **after pruning**. If
more than roughly forty survive, the spec covers too much — return to the decomposition
check in `superpowers:brainstorming` rather than writing a plan nobody finishes.

## Amendment

Implementation invalidates pre-implementation docs routinely: a new failure mode surfaces,
an error code changes, a sub-feature is dropped. Without an amend path, verification
measures the code against a stale yardstick and passes.

Amendments **append** rows with new IDs and move superseded ones to `## Retired`, with user
approval. Never renumber, never silently edit a row an implementer already tagged against.

## Definition of done

- [ ] Behavior list confirmed by the user before sweeping
- [ ] Every behavior group has a `Ruled out:` line citing spec or code facts
- [ ] Axis 5 rule-outs name the absent shared resource
- [ ] Every `Bug it catches` is a mutation, not a negated `Expected`
- [ ] Risk derived by rule; every argue-down carries a recorded approval line
- [ ] Axis 5 rows `integration`+; axis 11 rows `component`+
- [ ] Harness hints on `component`, `smoke`, `e2e` rows
- [ ] Invariants and out-of-scope both present
- [ ] Baseline commit recorded in the header
- [ ] Artifact committed to the target repo, not the plugin repo

## Next stage

**REQUIRED SUB-SKILL:** `superpowers:writing-plans`. Tell it to cite SC-IDs in each task's
test steps, so every row maps to a task and an uncovered row is visible as a plan gap.
```

- [ ] **Step 2: Verify the frontmatter parses and the referenced files exist**

Run:
```bash
head -5 skills/test-coverage-design/SKILL.md
for f in sweep-axes risk-derivation level-assignment; do
  test -f "skills/test-coverage-design/references/$f.md" && echo "ok $f" || echo "MISSING $f"
done
grep -o 'references/[a-z-]*\.md' skills/test-coverage-design/SKILL.md | sort -u
```
Expected: frontmatter shows `name:` and `description:`; three `ok` lines; every referenced path is one of the three that exist. `example-coverage-doc.md` will report missing — Task 7 creates it.

- [ ] **Step 3: Commit**

```bash
git add skills/test-coverage-design/SKILL.md
git commit -m "feat: test-coverage-design skill with floor rule and mutation-phrased rows"
```

---

### Task 7: Worked coverage-doc example

**Files:**
- Create: `skills/test-coverage-design/references/example-coverage-doc.md`

**Interfaces:**
- Consumes: the row schema from Task 6, the fixture project from Task 2.
- Produces: a filled artifact matching the fixture project exactly, so Task 9's layer 1 has a real coverage doc to parse in its own verification step. `SC-01` is `unit`/`med`, `SC-02` is `integration`/`high`, `SC-03` is `integration`/`high`.

**This is the checkpoint.** After this task the generate stage is complete and usable.

- [ ] **Step 1: Write the worked example against the fixture project**

`skills/test-coverage-design/references/example-coverage-doc.md`:

```markdown
# Coverage: order creation

Spec: docs/superpowers/specs/2026-07-30-order-creation-design.md
Baseline: a1b2c3d — suite result captured at this commit

## Invariants

- INV-1 no partially-written order is ever visible to a reader
- INV-2 a retried create with the same idempotency key produces no second row

## Out of scope

- throughput beyond 1k rps — the spec states no performance requirement
- multi-region failover — deferred, not in this spec

## Retired

- SC-04 merged into SC-03 (same mutation: uniqueness check outside the transaction)

## Scenarios

### Behavior: create an order

Ruled out: time (spec defines no date logic or expiry), auth (internal endpoint, spec §3
states no actor concept), rendering (API only, no UI surface), migration (no schema change
— no new changeset in this feature), resource limits (payload is two scalar fields)

| ID | Behavior | Bug it catches | Level | Expected | Risk |
|----|----------|----------------|-------|----------|------|
| SC-01 | quantity at the lower bound is rejected | guard uses `<` instead of `<=` | unit | throws "qty must be > 0", no row persisted | med |
| SC-02 | write rolls back when the commit hook fails | rollback call omitted on the error path | integration | 0 rows persisted, INV-1 holds | high |
| SC-03 | two writers with one idempotency key | uniqueness check runs outside the transaction (TOCTOU) | integration | 1 row, second call answers 200 | high |
| SC-05 | dependency down answers 503, not 500 | error mapped to the generic handler instead of the unavailable branch | smoke | 503, body reports `rows: 0`, message not acked | high |

harness SC-02: in-process store, write hook throws on invocation
harness SC-03: 2 parallel clients, real store, no mocks, 50 iterations fail-if-any
harness SC-05: service started with `FAIL_DB=1`, `POST /orders {"qty":3}`

### Notes on this example

`SC-01` is `med` because axis 1 is not in the high-by-rule set. `SC-02` and `SC-05` are
axis 3, `SC-03` is axis 5 — all `high` by rule, so all three require layer 4 mutation
evidence.

`SC-03` is `integration`, not `unit`, because two mocked concurrent calls execute
sequentially in one thread and would pass whether or not the check sits inside the
transaction. Its harness names an iteration count, because a single green run of a race
test records `INCONCLUSIVE`, not `PASS`.

Every `Bug it catches` names something a person could type into the source. Contrast
"a duplicate order is created", which is `Expected` negated and gives the mutator nothing
to apply.
```

- [ ] **Step 2: Verify every row obeys the schema and the risk rule**

Run:
```bash
grep -E '^\| SC-' skills/test-coverage-design/references/example-coverage-doc.md | awk -F'|' '{print NF}'
grep -E '^\| SC-0[235]' skills/test-coverage-design/references/example-coverage-doc.md | grep -c 'high'
```
Expected: every row reports `8` fields (6 columns plus the leading and trailing empties), and `3` for the second — the three axis-3/5 rows are all `high`.

- [ ] **Step 3: Verify the example's IDs match the fixture project's tags**

Run:
```bash
grep -o 'covers: SC-[0-9]*' fixtures/sample-project/test/orders.test.ts | sort -u
```
Expected: `covers: SC-01`, `covers: SC-02`, `covers: SC-03`. These are rows in the example doc, so Task 9's layer 1 can be verified against the pair. `SC-05` has no fixture test on purpose — it is the `gap` layer 1 must report.

- [ ] **Step 4: Commit**

```bash
git add skills/test-coverage-design/references/example-coverage-doc.md
git commit -m "feat: worked coverage-doc example matching the fixture project"
```

---

### Task 8: `coverage-critic` agent

**Files:**
- Create: `agents/coverage-critic.md`

**Interfaces:**
- Consumes: row fields from Task 6, the fixture tests from Task 2.
- Produces: an agent returning one verdict per row — `HOLDS`, `WEAK`, or `VACUOUS` — plus the canary protocol Task 9's layer 2 drives. Tool grant is `Read, Grep, Glob` only.

- [ ] **Step 1: Write the agent definition**

`agents/coverage-critic.md`:

```markdown
---
name: coverage-critic
description: Judges whether a test would actually fail if the bug it claims to guard against were reintroduced. Read-only by design.
tools: Read, Grep, Glob
---

You judge whether tests bite. You did not write the code under review and you must not
develop an opinion about whether it is good code — that is someone else's job.

## Your question

For each scenario row you are given:

> Would this test **fail** if the mutation named in `Bug it catches` were applied to the
> production code?

You have the row, the tagged test files, and the production diff. Read the diff — the
question is a claim about production code, and you cannot answer it from test files alone.

## Verdicts

Exactly one per row.

| Verdict | Meaning |
|---|---|
| `HOLDS` | Applying the mutation would make this test fail. It binds to the bug. |
| `WEAK` | The test passes, but would **still** pass with the mutation applied. It tests something adjacent. |
| `VACUOUS` | The test asserts nothing bound to the mutation at all. |

## Defect patterns to hunt

- **Assertion on a mock, not on state.** `expect(spy).toHaveBeenCalledOnce()` proves the
  code called something. It does not prove the resulting state is correct. If the mutation
  changes state but the test only inspects a spy, that is `VACUOUS`.
- **Assertion on a trivially-returned value.** The test asserts what the implementation
  returns unconditionally, so it cannot distinguish correct from incorrect.
- **Assertion that survives the mutation.** Trace it concretely: apply the mutation in your
  head, re-read the assertions, ask which one flips. If none flips, `WEAK`.
- **Assertion on the wrong side of the boundary.** A row about `qty <= 0` tested only with
  `qty = -5` does not catch `<` versus `<=`. The boundary value itself must appear.

## Output format

One block per row, nothing else:

```
SC-01 HOLDS
  asserts the thrown message AND that no row persisted; mutating <= to < makes qty=0
  succeed, so count() becomes 1 and the second assertion fails
SC-03 VACUOUS
  only asserts spy.toHaveBeenCalledOnce; the mutation changes persisted state, which this
  test never inspects
```

Every verdict needs the one-or-two-line reason naming which assertion flips, or why none
does. A bare verdict is not usable.

## The canary

Your batch contains one test the dispatcher already knows is weak. If you verdict every row
`HOLDS`, you have failed the canary and your entire batch is discarded and re-run. Do not
guess which one it is — just do the work honestly and the canary takes care of itself.

## Rules

- You have no edit tools. This is deliberate: a critic that can repair a weak test will
  repair it and then pass it, which destroys the signal. If a test is weak, say so.
- Never soften a verdict because the fix looks tedious.
- Never verdict `HOLDS` on the grounds that the code looks correct. The question is about
  the **test**, not the code.
- If a row's tagged test does not exist, report `SC-nn MISSING` and move on. Do not
  speculate about what it would have asserted.
```

- [ ] **Step 2: Verify the tool grant excludes edit tools**

Run:
```bash
grep '^tools:' agents/coverage-critic.md
```
Expected: exactly `tools: Read, Grep, Glob`. If `Edit`, `Write`, or `Bash` appear, the structural constraint is gone — fix it.

- [ ] **Step 3: Verify the agent reaches the right verdict on the fixture's vacuous test**

Dispatch the agent with this input, using the fixture files:

```
Row: SC-03 | creates an order | uniqueness check runs outside the transaction (TOCTOU) | integration | 1 row, second call answers 200 | high
Tagged test: fixtures/sample-project/test/orders.test.ts (the test tagged covers: SC-03)
Production code: fixtures/sample-project/src/orders.ts
```

Expected: `SC-03 VACUOUS`, with a reason naming that the test only asserts on the spy. A
`HOLDS` verdict here means the agent definition is not working — revise the defect-pattern
section before proceeding.

- [ ] **Step 4: Verify the agent reaches the right verdict on the fixture's good test**

Dispatch with:

```
Row: SC-01 | quantity at the lower bound is rejected | guard uses < instead of <= | unit | throws "qty must be > 0", no row persisted | med
Tagged test: fixtures/sample-project/test/orders.test.ts (the test tagged covers: SC-01)
Production code: fixtures/sample-project/src/orders.ts
```

Expected: `SC-01 HOLDS`, with a reason naming the `count()` assertion. An agent that
verdicts everything `VACUOUS` is as useless as one that verdicts everything `HOLDS`.

- [ ] **Step 5: Commit**

```bash
git add agents/coverage-critic.md
git commit -m "feat: coverage-critic agent, read-only with canary protocol"
```

---

### Task 9: `mutator` agent

**Files:**
- Create: `agents/mutator.md`

**Interfaces:**
- Consumes: `Bug it catches` phrasing from Task 6, the fixture project from Task 2.
- Produces: an agent returning `MUTATION-CAUGHT` or `MUTATION-SURVIVED` per `risk=high` row, plus transcripts at `transcripts/SC-nn-mutation.txt`. Tool grant `Read, Bash, Edit`.

- [ ] **Step 1: Write the agent definition**

`agents/mutator.md`:

```markdown
---
name: mutator
description: For high-risk rows, applies the named mutation and proves the tagged test fails. Produces reproducible evidence that a test bites.
tools: Read, Bash, Edit
---

You prove that a test catches the bug it claims to catch, by reintroducing the bug.

This is the only verification agent with edit tools, because mutating source is the job.
You work on a throwaway branch and you leave the tree exactly as you found it.

## Protocol per row

For each `risk=high` row you are given:

1. **Confirm you are on a throwaway branch.** Run `git rev-parse --abbrev-ref HEAD`. If it
   is the working branch, create one: `git checkout -b mutation-check-<SC-nn>`. Never mutate
   on the branch the user is developing on.
2. **Apply the mutation** named in `Bug it catches`, and nothing else. One row, one
   mutation, one file where possible.
3. **Run only the tagged test** — never the whole suite. This is what keeps the layer
   affordable: cost scales with high-risk row count, not suite runtime.
4. **Require FAIL.** A passing test here is the finding: the test does not bite.
5. **Revert** with `git checkout -- <file>` and run the same test again.
6. **Require PASS.** If it now fails, your revert was incomplete or the test is flaky —
   report that rather than proceeding.
7. **Write the transcript** to `<coverage-dir>/transcripts/SC-nn-mutation.txt`: the diff you
   applied, both commands, and both outputs verbatim.

## Verdicts

| Verdict | Meaning |
|---|---|
| `MUTATION-CAUGHT` | Test failed with the mutation, passed after revert. The row has real evidence. |
| `MUTATION-SURVIVED` | Test passed with the mutation applied. **Blocking.** |
| `MUTATION-UNAPPLIABLE` | The named mutation cannot be expressed as a source edit. Report the row, do not invent a substitute. |

`MUTATION-UNAPPLIABLE` usually means the row's `Bug it catches` was phrased as a negated
expectation rather than a code-level mutation. That is a coverage-doc defect, not a code
defect — say so plainly.

## Output format

```
SC-02 MUTATION-CAUGHT
  mutation: removed `rows.pop()` from the catch block in src/orders.ts
  with mutation:    FAIL — expected count() to be 0, got 1
  after revert:     PASS
  transcript: docs/superpowers/coverage/transcripts/SC-02-mutation.txt
```

## Cleanup, mandatory

Before you finish:

1. `git checkout -- .` to drop any residual edit
2. `git status --porcelain` — must be empty except for the transcripts you wrote
3. Return to the original branch and delete the throwaway branch
4. Report the final `git status --porcelain` output verbatim in your response

The dispatcher records `git rev-parse HEAD`, `git stash list`, and a hash of `git diff`
before dispatching you and compares all three afterwards. Committing or stashing your
mutations to hide them will be detected. Do not.
```

- [ ] **Step 2: Verify the mutation loop by hand on the fixture, to prove the protocol works**

Run:
```bash
cd fixtures/sample-project
cp src/orders.ts /tmp/orders.ts.bak
sed -i 's/if (qty <= 0)/if (qty < 0)/' src/orders.ts
npx vitest run -t 'rejects zero quantity' 2>&1 | tail -5; echo "mutated exit=$?"
cp /tmp/orders.ts.bak src/orders.ts
npx vitest run -t 'rejects zero quantity' 2>&1 | tail -5; echo "reverted exit=$?"
git status --porcelain fixtures/sample-project/src/orders.ts
```
Expected: the mutated run FAILS, the reverted run PASSES, and `git status` reports nothing
for that file. This proves the fixture's `SC-01` test genuinely bites and that the protocol
is executable. If the mutated run passes, the fixture test is weaker than Task 2 intended —
fix the fixture.

- [ ] **Step 3: Verify the agent's tool grant**

Run:
```bash
grep '^tools:' agents/mutator.md
```
Expected: exactly `tools: Read, Bash, Edit`.

- [ ] **Step 4: Commit**

```bash
git add agents/mutator.md
git commit -m "feat: mutator agent with throwaway-branch protocol and mandatory cleanup"
```

---

### Task 10: `live-runner` agent

**Files:**
- Create: `agents/live-runner.md`

**Interfaces:**
- Consumes: `smoke` and `e2e` rows and harness hints from Task 6, the fixture server from Task 2.
- Produces: an agent that derives walkthroughs from rows, runs them against the real system, and writes transcripts containing command, exit code, and the grep proving each pre-declared expectation. Task 11's layer 3 re-greps those transcripts. Tool grant `Bash, Read`.

- [ ] **Step 1: Write the agent definition**

`agents/live-runner.md`:

```markdown
---
name: live-runner
description: Runs smoke and e2e rows plus derived happy and non-happy walkthroughs against the real running system, capturing auditable transcripts.
tools: Bash, Read
---

You exercise the real system and leave evidence a third party can check.

Your summary is **not** the evidence. The transcripts are. The dispatcher re-greps them for
the tokens you declared, so a summary that does not match its transcript will be caught.

## Bring-up

Use **the project's own run recipe** — its README, Makefile, compose file, or documented
start command. That is the only source of environment. Do not substitute infrastructure of
your own choosing.

If the system cannot be brought up, do not fabricate results. Record every command you
tried and its exit code, plus the files you looked for and did not find, and report
`SKIPPED` with that attempt evidence. An honest `SKIPPED` is worth more than an invented
`PASS`, and the verdict machinery is built to handle it.

## Walkthroughs are derived, not fixed

Build the set from the coverage doc: one walkthrough per distinct failure path present at
`smoke` or `e2e` level, plus one happy path. A feature with no such rows means you have
nothing to do — report `N/A`, not `SKIPPED`.

Typical shape, illustrative only:

```
W0 happy      request accepted, downstream effect observed, final state correct
W1 rejected   invalid input  → 4xx, no side effect
W2 dep down   dependency off → correct status, message not acked, retried on recovery
W3 duplicate  same key twice → one effect, second call idempotent
W4 timeout    slow consumer  → requeued, no partial write
W5 parallel   N clients      → axis-5 rows under real contention
```

## Declare the expectation before you run

For each walkthrough, **write down the machine-checkable expectation first**: status code,
row count, log line, queue depth. Then run. Then prove it with a grep.

The transcript for each walkthrough must contain, in order:

1. the expectation, as a single line beginning `EXPECT: `
2. the command you ran
3. its exit code
4. its output
5. the grep that proves the expectation, and that grep's output
6. a single line `RESULT: PASS` or `RESULT: FAIL`

Write one file per walkthrough: `<coverage-dir>/transcripts/W<n>-<slug>.txt`.

Example:

```
EXPECT: HTTP 503 and body contains "rows":0
$ FAIL_DB=1 npm start & sleep 1; curl -s -w '\n%{http_code}' -X POST localhost:3999/orders -d '{"qty":3}'
exit=0
{"error":"db unavailable","rows":0}
503
$ grep -c '"rows":0' out.txt && grep -c '^503$' out.txt
1
1
RESULT: PASS
```

## Concurrency walkthroughs need repetition

One green pass of a race test proves nothing — the interleaving that triggers the bug
simply may not have occurred. A W5-class walkthrough requires a stated iteration count with
fail-if-any, a race detector, or a seeded deterministic schedule.

A single green run records `RESULT: INCONCLUSIVE`, never `PASS`.

## Rules

- Do not edit source, tests, fixtures, config, or CI files. The dispatcher records
  `git rev-parse HEAD`, `git stash list`, and a hash of `git diff` before dispatching you
  and compares all three afterwards.
- Clean up what you started: stop services, remove temporary data, release ports.
- Namespace anything you create in shared infrastructure, and delete only what you created.
- Report the full path of every transcript you wrote.
```

- [ ] **Step 2: Verify the agent's tool grant excludes edit tools**

Run:
```bash
grep '^tools:' agents/live-runner.md
```
Expected: exactly `tools: Bash, Read`. `Bash` can of course write files — that is why the
dispatcher's tamper check exists rather than relying on the grant alone.

- [ ] **Step 3: Verify the fixture's dep-down walkthrough is executable as specified**

Run:
```bash
cd fixtures/sample-project
FAIL_DB=1 npm start > /tmp/w2-server.log 2>&1 & sleep 1
curl -s -w '\n%{http_code}\n' -X POST localhost:3999/orders -d '{"qty":3}' > /tmp/w2.txt
kill %1
cat /tmp/w2.txt
grep -c '"rows":0' /tmp/w2.txt; grep -c '^503$' /tmp/w2.txt
```
Expected: body `{"error":"db unavailable","rows":0}`, status `503`, and both greps report
`1`. This is the W2 transcript shape the agent must produce.

- [ ] **Step 4: Commit**

```bash
git add agents/live-runner.md
git commit -m "feat: live-runner agent with pre-declared expectations and derived walkthroughs"
```

---

### Task 11: `coverage-verification` skill

**Files:**
- Create: `skills/coverage-verification/SKILL.md`
- Create: `skills/coverage-verification/references/example-verification.md`

**Interfaces:**
- Consumes: `references/runner-output.md` (Task 3), the coverage-doc format (Task 6), and all three agents (Tasks 8–10).
- Produces: the four-layer algorithm, the five layer-1 findings (`gap`, `drift`, `overclaim`, `stale`, `red`), the three verdicts, the `PARTIAL` acknowledgement requirement, the degradation table, the tamper check, and the report format.

- [ ] **Step 1: Write the skill**

`skills/coverage-verification/SKILL.md`:

```markdown
---
name: coverage-verification
description: Use at the finishing-a-development-branch handoff - verifies every coverage scenario has a test that actually bites, across trace, adversarial read, live execution, and mutation
---

# Coverage Verification

## Overview

Measure the implementation against the coverage doc across four layers. Each layer's
self-report is checked by something other than itself.

**Announce at start:** "I'm using the coverage-verification skill to verify coverage for
this feature."

**When:** at the `superpowers:finishing-a-development-branch` handoff. Not before
`requesting-code-review` — that skill runs *inside*
`superpowers:subagent-driven-development` as its final review, so no seam exists there.
Consequence: a `BLOCKED` verdict arrives after SDD's own reviews have run.

**Mode:** unattended, except that `PARTIAL` requires a human acknowledgement.

**Refuse to run** if the coverage doc is missing. There is no yardstick, and a report with
no yardstick is worse than no report.

## Layer 1 — trace (main thread)

1. **Parse the coverage doc.** Collect active SC-IDs, retired SC-IDs, and each row's
   `Level` and `Risk`.
2. **Grep the tags,** scoped to the affected package:
   `grep -rn "covers: SC-" <test-dir>`
3. **Resolve each tag to a test name** using the language patterns in
   [references/runner-output.md](references/runner-output.md).
4. **Run the suite with machine-readable output, redirected to a file.** Parse only the
   normalized `status<TAB>name` contract. Never let raw suite output land in this context —
   a failing suite in a large repo is tens of thousands of tokens.
5. **Join tags to runner results** by suffix match on the test name.
6. **Compare against the baseline** suite result recorded in the coverage doc header.

Findings:

| Finding | Condition |
|---|---|
| `gap` | Row has no tag, **or** its tagged test is reported `skipped`, **or** the test is absent from the run entirely |
| `drift` | A tag cites an SC-ID that is neither active nor in `## Retired` |
| `overclaim` | One test carries more than 2 SC-IDs. Each cited row must then be verdicted separately by layer 2 against a named assertion |
| `stale` | The diff touches a behavior with no SC row — trigger to amend the doc |
| `red` | A test failing now that passed at baseline. Pre-existing failures are not this feature's `red` |

A tag above a skipped test is the cheapest possible false pass: it satisfies a grep and
leaves the suite green. Step 4 exists solely to close it. Never substitute grep plus exit
code.

## Layer 2 — adversarial read

Dispatch `coverage-critic`, one agent per behavior group, groups in parallel.

Give each: the group's rows, the tagged test files, and **the production diff**. Withholding
the diff would leave the critic unable to answer its own question, since "would this fail if
the mutation were applied" is a claim about production code. Its honesty comes from having
no edit tools, not from having no information.

**Inject one canary per batch:** a test you already know is weak. A batch that verdicts its
canary `HOLDS` has all its verdicts voided and re-dispatches at a higher effort tier.
Without this, an all-`HOLDS` critic passes forever after the one-time acceptance check.

Verdicts: `HOLDS`, `WEAK`, `VACUOUS`. `WEAK` and `VACUOUS` are blocking.

A critic may also return `SC-nn MISSING` when a row's tagged test does not exist. That is
not a fourth verdict — fold it into layer 1's `gap` set and reconcile: if layer 1 did not
already report that row as a `gap`, the tag resolution or the name join is broken, and layer
1 must be re-run before the report is trusted.

## Layer 3 — live

Dispatch `live-runner` for the `smoke` and `e2e` rows plus derived walkthroughs.

**Before dispatch,** record `git rev-parse HEAD`, `git stash list`, and a hash of
`git diff`. **After it returns,** compare all three. `git status --porcelain` alone would
miss a runner that commits, stashes, or edits and reverts.

**Then re-grep the transcripts yourself** for the `EXPECT:` and `RESULT:` tokens the agent
declared. The agent's summary is a claim;
`superpowers:verification-before-completion` names trusting agent success reports as a red
flag, and a self-graded `W2 PASS` is exactly that. Re-greping costs almost nothing and makes
the summary auditable.

No `smoke` or `e2e` rows → layer 3 is `N/A`, not `SKIPPED`, not `PARTIAL`.

A single-iteration concurrency walkthrough is `INCONCLUSIVE`, never `PASS`.

## Layer 4 — mutation (`risk=high` rows only)

Dispatch `mutator`. Same tamper-check protocol as layer 3, before and after.

Verdicts: `MUTATION-CAUGHT`, `MUTATION-SURVIVED` (blocking), `MUTATION-UNAPPLIABLE`.

`MUTATION-UNAPPLIABLE` is a coverage-doc defect — the row's `Bug it catches` was not
phrased as a code-level mutation. Report it as such; do not let the mutator invent a
substitute.

## Verdicts

Exactly three.

**`BLOCKED`** on any of: `gap`, unretired `drift`, unresolved `overclaim`, unresolved
`stale`, `red`, `WEAK`, `VACUOUS`, `MUTATION-SURVIVED`, live `FAIL`.

**`PARTIAL`** when a layer could not run. Requires **both**:

- a human acknowledgement line: `PARTIAL accepted by <user> — <reason>`
- attempt evidence in the transcript directory: commands tried, exit codes, files sought

Without both, the expensive layer is opt-out by unverified declaration — and layer 3 is
where that pressure lands.

**`PASS`** only when every layer either passed or is `N/A`.

## Re-run scope

Layer 1 **always** re-runs in full. It is cheap, and a fix that repairs two rows can break
neighbors. Layers 2, 3, and 4 may be scoped to an ID subset.

Every row verdict records which run produced it, so a report cannot silently combine fresh
subset results with stale inherited verdicts and present the mixture as `PASS`.

## Degradation

| Situation | Behavior |
|---|---|
| No `smoke`/`e2e` rows | Layer 3 `N/A` |
| No run recipe, or dependencies unavailable | Layer 3 `SKIPPED` with attempt evidence → `PARTIAL` |
| Suite exists but cannot execute here | Layers 1 and 4 trace-only → `PARTIAL` with attempt evidence |
| No test suite at all (legacy repo) | Scope layer 1's gap set to rows whose behavior the diff touches. Do not hard-block on rows the feature never went near |
| Pure refactor | Doc is the invariant set plus named characterization tests; layers 2–4 `N/A`; layer 1 requires zero new failures against baseline |
| Coverage doc missing | Refuse to run |

## Artifact

`<repo>/docs/superpowers/coverage/YYYY-MM-DD-<feature>-verification.md`, transcripts under
`<repo>/docs/superpowers/coverage/transcripts/`.

Format: [references/example-verification.md](references/example-verification.md).

## Definition of done

- [ ] Layer 1 ran in full, consuming machine-readable runner output
- [ ] Every active row has a verdict from every applicable layer
- [ ] Each layer-2 batch's canary was caught
- [ ] Layer 3 transcripts re-grepped by this thread, not trusted from the summary
- [ ] Tamper check run before and after both layer 3 and layer 4
- [ ] Every `risk=high` row has a layer 4 verdict
- [ ] `PARTIAL` carries both the acknowledgement line and attempt evidence
- [ ] Report committed to the target repo

## Next stage

Verdict `PASS` or acknowledged `PARTIAL` → continue with
`superpowers:finishing-a-development-branch`.

Verdict `BLOCKED` → report the findings, fix, re-run. Do not finish the branch.
```

- [ ] **Step 2: Write the report example**

`skills/coverage-verification/references/example-verification.md`:

```markdown
# Verification: order creation

Coverage: docs/superpowers/coverage/2026-07-30-order-creation-coverage.md
Baseline: a1b2c3d · Run: 2026-07-30T14:02Z
Verdict: BLOCKED — 1 gap, 1 vacuous, 1 mutation survived, 1 live failure

## Layer 1 — trace (full re-run)

3/4 rows tagged · runner reported 3 passed, 1 skipped · 0 new failures vs baseline

```
gap:       SC-05 — no tagged test
gap:       SC-02 — tagged test reported `skipped` by the runner
drift:     none
overclaim: none
stale:     none
```

`SC-02` is the case a grep-only layer 1 would have called covered: the tag is present and
the suite is green, because a skipped test does not fail.

## Layer 2 — adversarial read (canary caught in 1/1 batches)

2 rows read: HOLDS 1 · WEAK 0 · VACUOUS 1

```
SC-01 HOLDS
  asserts the thrown message AND that no row persisted; mutating <= to < makes qty=0
  succeed, so count() becomes 1 and the second assertion fails
SC-03 VACUOUS
  only asserts spy.toHaveBeenCalledOnce; the mutation changes persisted state, which this
  test never inspects
```

## Layer 3 — live (transcripts re-grepped by main thread)

```
W0 happy    PASS          EXPECT 201 → got 201
W2 dep down FAIL          EXPECT 503 and "rows":0 → got 500
W5 parallel INCONCLUSIVE  1 iteration only; needs a stated count with fail-if-any
```

transcripts: `docs/superpowers/coverage/transcripts/W0-happy.txt`, `W2-dep-down.txt`,
`W5-parallel.txt`

Tamper check: HEAD unchanged, stash list unchanged, diff hash unchanged.

## Layer 4 — mutation (risk=high rows)

```
SC-02 MUTATION-UNAPPLIABLE  row has no tagged executable test (see layer 1 gap)
SC-03 MUTATION-SURVIVED     uniqueness check moved outside the transaction, test still passed
```

transcripts: `docs/superpowers/coverage/transcripts/SC-03-mutation.txt`

Tamper check: HEAD unchanged, stash list unchanged, diff hash unchanged; working tree clean
apart from the transcripts.

## What must happen before this branch can finish

1. `SC-02` — un-skip the test, or delete the row and record why in `## Retired`
2. `SC-03` — assert persisted state instead of the spy, then re-run layers 2 and 4 for `SC-03`
3. `SC-05` — write the missing smoke test
4. `W2` — map the dependency failure to 503 rather than the generic 500 handler
5. `W5` — give the harness a stated iteration count with fail-if-any
```

- [ ] **Step 3: Verify the skill names all five layer-1 findings and all three verdicts**

Run:
```bash
for f in gap drift overclaim stale red; do
  grep -q "\`$f\`" skills/coverage-verification/SKILL.md && echo "ok $f" || echo "MISSING $f"
done
for v in PASS PARTIAL BLOCKED; do
  grep -q "$v" skills/coverage-verification/SKILL.md && echo "ok $v" || echo "MISSING $v"
done
```
Expected: eight `ok` lines, no `MISSING`.

- [ ] **Step 4: Verify layer 1's algorithm against the fixture end to end**

This is the real test of Task 11. Using the fixture project and the worked example doc as
the coverage doc, execute layer 1 by hand:

```bash
cd fixtures/sample-project
grep -rn "covers: SC-" test/
jq -r '.testResults[].assertionResults[]
  | "\(if .status == "pending" or .status == "todo" then "skipped" else .status end)\t\(.fullName)"' \
  .vitest-report.json
```
Expected conclusion, which you must be able to derive from those two outputs alone:
- `SC-01` → tagged, `passed` → covered
- `SC-02` → tagged, `skipped` → **`gap`**
- `SC-03` → tagged, `passed` → covered (layer 2 will find it vacuous)
- `SC-05` → no tag → **`gap`**

If the algorithm as written in the skill does not produce exactly those four conclusions
from those two commands, the algorithm is wrong. Fix the skill, not the fixture.

- [ ] **Step 5: Commit**

```bash
git add skills/coverage-verification/SKILL.md skills/coverage-verification/references/example-verification.md
git commit -m "feat: coverage-verification skill, four layers with auditable evidence"
```

---

### Task 12: Enforcement rule and install docs

**Files:**
- Create: `docs/enforcement.md`
- Modify: `README.md` (replace the `docs/enforcement.md` pointer with the actual rule)
- Modify: `~/.claude/CLAUDE.md` (append the gate rule)

**Interfaces:**
- Consumes: skill names from Tasks 6 and 11.
- Produces: the gate rule that supersedes `brainstorming`'s terminal-state instruction by name, installed in the user's global CLAUDE.md.

- [ ] **Step 1: Write the enforcement doc**

`docs/enforcement.md`:

```markdown
# Enforcement

The two stages are wired in by skill-text handoffs plus one rule in `~/.claude/CLAUDE.md`.

## Why the rule must name what it supersedes

`superpowers:brainstorming` ends with an emphatic instruction:

> **The terminal state is invoking writing-plans.** Do NOT invoke frontend-design,
> mcp-builder, or any other implementation skill. The **ONLY** skill you invoke after
> brainstorming is writing-plans.

That text is loaded into context at exactly the moment the handoff happens, and it is more
local than a global memory rule. A gate rule that does not name it is relying on the agent
silently preferring the weaker instruction — and the resulting nondeterministic skipping
reads as "the stage got skipped" rather than "two instructions collided", which sends
whoever debugs it in the wrong direction.

## The rule

Append to `~/.claude/CLAUDE.md`:

```markdown
## Test design gate

`superpowers:brainstorming` states that the only skill it may invoke afterwards is
`writing-plans` ("The ONLY skill you invoke after brainstorming is writing-plans").
That rule is amended here. The sequence is:

  brainstorming → test-design:test-coverage-design → writing-plans

For features meeting test-coverage-design's floor rule (≤2 behaviors, no persistence, no
concurrency, no external dependency), the coverage table is appended inline to the spec
instead and brainstorming proceeds directly to writing-plans.

At the end of implementation, `superpowers:subagent-driven-development` hands off to
`finishing-a-development-branch`. Invoke `test-design:coverage-verification` at that
handoff, before the branch is finished. A plan with no coverage doc, or a branch with no
verification report, is incomplete.
```

## Not hooks, for now

A hook could enforce this deterministically — Claude Code hooks run arbitrary commands, so
one could perform the entire tag-and-runner-report check. The reason to defer is the
unknown false-denial rate: a gate that blocks legitimate work mid-flow gets disabled
permanently, and that risk is unmeasured until the stage has real mileage.

Revisit after the dogfood phases. If the stage is observed being skipped, the first step up
is a **warning** hook on writes to `docs/superpowers/plans/*` that injects context about
whether a coverage doc exists — not a denial.
```

- [ ] **Step 2: Replace the README's pointer with the rule itself**

In `README.md`, replace:

```markdown
Then add the gate rule from `docs/enforcement.md` to your `~/.claude/CLAUDE.md`.
```

with:

```markdown
Then install the gate rule — see [docs/enforcement.md](docs/enforcement.md) for the text
and for why it must name the `brainstorming` instruction it supersedes.
```

- [ ] **Step 3: Append the rule to the user's global CLAUDE.md**

Append the fenced rule block from step 1 to `~/.claude/CLAUDE.md`. Do not reformat or
reorder anything already in that file.

- [ ] **Step 4: Verify the rule is installed and names the superseded instruction**

Run:
```bash
grep -A3 '## Test design gate' ~/.claude/CLAUDE.md
grep -c 'ONLY skill you invoke after brainstorming' ~/.claude/CLAUDE.md
```
Expected: the rule block prints, and the count is `1` — the superseded instruction is quoted
by name.

- [ ] **Step 5: Verify the whole plugin loads with both skills present**

Run:
```bash
cd /home/igi21/experiements/test-design
jq -e . .claude-plugin/*.json > /dev/null && echo "manifests ok"
ls skills/*/SKILL.md agents/*.md
claude plugin install test-design@test-design
```
Expected: `manifests ok`, two `SKILL.md` files, three agent files, and a successful
reinstall. Both skills should now resolve as `test-design:test-coverage-design` and
`test-design:coverage-verification`.

- [ ] **Step 6: Commit and push**

```bash
git add docs/enforcement.md README.md
git commit -m "feat: enforcement rule naming the superseded brainstorming instruction"
git push origin main
```

---

### Task 13: Dogfood phase 1 — `qmemd-a1r`

**Files:**
- Create (in the qmemd repo): `docs/superpowers/coverage/<date>-secret-scan-coverage.md`
- Create (in the qmemd repo): `docs/superpowers/coverage/<date>-secret-scan-verification.md`
- Modify: `docs/dogfood-phase-1.md` in this repo — findings

**Interfaces:**
- Consumes: the whole plugin.
- Produces: evidence that each gate catches a planted defect, and a findings doc listing every gate that proved to be theater.

- [ ] **Step 1: Run the full flow on the real feature**

Target: `qmemd-a1r` in `/home/igi21/experiements/qmemd` — `remember()` unconditionally
commits and pushes at `engine.ts:1561-1562`, so a pasted secret persists in remote history.
Needs a scan gating the commit before push, plus a `SECRET_SUSPECTED` doctor code.

Run `brainstorming` → `test-coverage-design` → `writing-plans` → implement →
`coverage-verification`, using the plugin as installed.

- [ ] **Step 2: Plant the defects and record what each gate did**

Plant each of these, run the relevant layer, and record the actual verdict:

| Plant | Layer | Required outcome |
|---|---|---|
| a test with no `covers:` tag | 1 | `gap` |
| a `covers:`-tagged test marked `it.skip` | 1 | `gap`, not covered |
| a test asserting only on a spy | 2 | `VACUOUS` or `WEAK` |
| a batch canary | 2 | caught; a missing batch is voided and re-dispatched |
| a high-risk row whose test does not bite | 4 | `MUTATION-SURVIVED` |
| a single-iteration concurrency walkthrough | 3 | `INCONCLUSIVE`, not `PASS` |

- [ ] **Step 3: Record the findings**

Write `docs/dogfood-phase-1.md`: for each gate, whether its plant was caught, the measured
token and wall-clock cost of each stage, and every point where the skill text was ambiguous
enough that you had to guess.

Any plant that survived means that gate is theater. Cut it or fix it — do not ship it weak
and do not proceed to phase 2 with a known-blind gate.

- [ ] **Step 4: Commit**

```bash
git add docs/dogfood-phase-1.md
git commit -m "docs: dogfood phase 1 findings from qmemd-a1r"
git push origin main
```

---

## Deferred to phase 2

`ofacservice-3s6` (`/home/igi21/work/ofacservice`) validates what qmemd structurally
cannot: layer 3 against real Postgres, RabbitMQ, and HTTP dependencies; axes 3, 4, and 5
with genuine contention via `ScheduleLocker`; the `PARTIAL` path; and compiled-language
re-run economics. Run it as a separate plan after phase 1's findings are folded in.

## Not in this plan

- The warning hook on plan writes — deferred pending a measured false-denial rate
- TestIT / Kaiten export adapter
- Reconciliation with the Russian skills in the `qa` repo
