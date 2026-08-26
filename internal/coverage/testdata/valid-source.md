# Coverage: order creation

Spec: docs/superpowers/specs/2026-07-30-order-creation-design.md
Baseline: a1b2c3d · 1 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go-test-json
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-07-30-order-creation-baseline.json
- baseline-sha256: 3b1f2a9c
- live-profile: .test-design/live-profiles/local.json
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-07-30-order-creation-coverage.md"]

## Retired

- SC-04 merged into SC-03 (same mutation: uniqueness check outside the transaction)

## Scenarios

### Behavior: create an order

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | quantity at the lower bound is rejected | throws "qty must be > 0", no row persisted | unit | guard uses `<` instead of `<=` | med |
| SC-02 | write rolls back when the commit hook fails | 0 rows persisted, INV-1 holds | integration | rollback call omitted on the error path | high |

harness SC-02: in-process store, write hook throws on invocation

### Behavior: cancel an order

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| orders/SC-03 | dependency down answers 503, not 500 | 503, body reports rows: 0 | smoke | error mapped to the generic handler instead of the unavailable branch | high |

### Baseline table (not a scenario table)

| Test | Status |
|------|--------|
| not-an-id | should-be-ignored |
