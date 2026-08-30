# Coverage: fixture go project

Spec: docs/superpowers/specs/2026-08-30-fixture-design.md
Baseline: BASELINE_COMMIT · 0 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-08-30-fixture-baseline.json
- baseline-sha256: BASELINE_SHA256
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-08-30-fixture-coverage.md"]

## Retired

- SC-90 superseded by SC-02 (same production bug shape, kept as a drift-detection fixture)

## Scenarios

### Behavior: order quantity validation

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | quantity at the valid lower bound is accepted | ValidateQuantity(5) returns nil | unit | guard rejects any qty > 0 | low |
| SC-02 | quantity of zero is rejected | ValidateQuantity(0) returns an error | unit | guard uses `<` instead of `<=` | med |
| SC-03 | backorder quantity policy is defined | ValidateQuantity documents the backorder exception | unit | rule intentionally not yet implemented | low |
| SC-04 | quantity ceiling is enforced | ValidateQuantity rejects quantities above the configured maximum | unit | ceiling check omitted | med |
| SC-09 | duplicate test name isolation across packages | orders and payments may each define TestSameName without collision | unit | import-path scoping omitted from binding | low |

### Behavior: payment validation

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-05 | payment amount of zero is rejected | ValidateAmount(0) returns an error | unit | guard uses `<` instead of `<=` | med |
| SC-06 | payment currency support is validated | ValidateCurrency accepts only supported ISO codes | unit | unsupported code check omitted | low |
| SC-07 | payment idempotency key is validated | duplicate idempotency keys are rejected | unit | dedupe check omitted | high |
| SC-08 | payment retry backoff follows schedule | backoff calculation follows the configured schedule | unit | backoff formula uses wrong base | med |
