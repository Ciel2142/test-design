# Coverage: retired-row

Spec: docs/superpowers/specs/2026-08-26-retired-row-design.md
Baseline: a1b2c3d · 2 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go-test-json
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-08-26-retired-row-baseline.json
- baseline-sha256: 9f8e7d6c
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-08-26-retired-coverage.md"]

## Retired

- SC-02 superseded by the runner contract; the row stays for traceability

## Scenarios

### Behavior: place an order

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | order total includes tax | total = subtotal + tax | unit | tax addition omitted | med |
| SC-02 | checkout completes end to end | order confirmed | e2e | confirmation step removed | high |
