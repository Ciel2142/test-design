# Coverage: y

Spec: docs/superpowers/specs/2026-08-26-y-design.md
Baseline: a1b2c3d · 1 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go-test-json
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-08-26-y-baseline.json
- baseline-sha256: 3b1f2a9c
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-08-26-y-coverage.md"]

## Live decision

- decision: not-applicable
- reason: this document has no smoke or e2e rows

## Scenarios

### Behavior: y

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-03 | y behavior | y expected | unit | y mutation | med |
