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
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-07-30-order-creation-coverage.md"]

## Scenarios

### Behavior: create an order

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | quantity at the lower bound is rejected | throws "qty must be > 0", no row persisted | unit | guard uses `<` instead of `<=` | med |

### Behavior: create an order, retried

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | duplicate of the row above | throws "qty must be > 0", no row persisted | unit | guard uses `<` instead of `<=` | med |
