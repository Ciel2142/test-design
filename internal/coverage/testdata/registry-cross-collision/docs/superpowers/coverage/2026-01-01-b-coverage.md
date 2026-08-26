# Coverage: b

Spec: docs/superpowers/specs/2026-01-01-b-design.md
Baseline: a1b2c3d · 1 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go-test-json
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-01-01-b-baseline.json
- baseline-sha256: 3b1f2a9c
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-01-01-b-coverage.md"]

## Scenarios

### Behavior: b

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | b behavior | b expected | unit | b mutation | med |
