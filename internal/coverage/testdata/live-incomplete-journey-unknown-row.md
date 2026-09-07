# Coverage: live decision fixture

Spec: docs/superpowers/specs/2026-09-07-live-decision-design.md
Baseline: a1b2c3d · 1 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go-test-json
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/live-decision-baseline.json
- baseline-sha256: 3b1f2a9c
- live-profile: .test-design/live-profiles/local.json
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/live-decision-coverage.md"]

## Live decision

- decision: required

| Journey | Scenarios | Expected |
|---|---|---|
| J-1 | SC-02, SC-99 | the order appears in the deployed ledger |

## Scenarios

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | quantity at the lower bound is rejected | throws "qty must be > 0", no row persisted | unit | guard uses `<` instead of `<=` | med |
| SC-02 | checkout completes on the deployed revision | order row present in the deployed ledger | e2e | commit skipped on the success path | high |
