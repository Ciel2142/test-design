# Coverage: orders

Spec: docs/superpowers/specs/2026-08-26-orders-design.md
Baseline: a1b2c3d · 3 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go-test-json
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-08-26-orders-baseline.json
- baseline-sha256: 3b1f2a9c
- live-profile: .test-design/live-profiles/local.json
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-08-26-orders-coverage.md"]

## Live decision

- decision: required
- reason: the downed-dependency path is only observable on a deployed revision

| Journey | Scenarios | Expected |
|---|---|---|
| J-1 | orders/SC-05 | a downed dependency answers 503 on the deployed revision |

## Retired

- orders/SC-07 duplicate of orders/SC-05, folded in

## Scenarios

### Behavior: create an order

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | quantity at the lower bound is rejected | throws "qty must be > 0", no row persisted | unit | guard uses `<` instead of `<=` | med |

### Behavior: cancel an order

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| orders/SC-05 | dependency down answers 503, not 500 | 503, body reports rows: 0 | smoke | error mapped to the generic handler instead of the unavailable branch | high |
| orders/SC-06 | cancel reverses the stock reservation | stock count restored | integration | reversal call omitted | med |
