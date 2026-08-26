# Coverage: dup

Spec: docs/superpowers/specs/2026-01-01-dup-design.md
Baseline: a1b2c3d · 1 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go-test-json
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-01-01-dup-baseline.json
- baseline-sha256: 3b1f2a9c
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-01-01-dup-coverage.md"]

## Scenarios

### Behavior: first

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | first behavior | first expected | unit | first mutation | med |

### Behavior: duplicate of first

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | duplicate behavior | duplicate expected | unit | duplicate mutation | med |
