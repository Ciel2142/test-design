# Coverage: basic

Spec: docs/superpowers/specs/2026-01-01-basic-design.md
Baseline: a1b2c3d · 2 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go-test-json
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-01-01-basic-baseline.json
- baseline-sha256: 3b1f2a9c
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-01-01-basic-coverage.md"]

## Retired

- SC-04 merged into SC-02 (same mutation)

## Scenarios

### Behavior: create a widget

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | widget name is required | throws "name required" | unit | guard removed | med |
| SC-02 | widget id is generated | id is a UUID | unit | id generation stubbed to constant | low |
