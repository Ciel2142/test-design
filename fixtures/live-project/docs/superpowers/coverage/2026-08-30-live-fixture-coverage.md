# Coverage: fixture live project

Spec: docs/superpowers/specs/2026-08-30-live-fixture-design.md
Baseline: BASELINE_COMMIT · 0 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-08-30-live-fixture-baseline.json
- baseline-sha256: BASELINE_SHA256
- live-profile: .test-design/live-profiles/fixture-local.json
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-08-30-live-fixture-coverage.md"]

## Retired

- SC-90 superseded by SC-11 (same walkthrough, kept as a drift-detection fixture)

## Scenarios

### Behavior: live walkthrough against the running fixture

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-11 | the happy walkthrough reaches the ready marker | walkthrough step prints live-fixture-ready | smoke | ready marker never printed | med |
| SC-12 | the non-happy walkthrough is refused | walkthrough step prints live-fixture-refused | e2e | refusal path answers accepted instead | high |
| SC-13 | queue depth is reported while the app runs | depth endpoint answers with the current depth | smoke | depth counter never incremented | low |
| SC-14 | quantity guard rejects zero | guard rejects qty 0 | unit | guard uses `<` instead of `<=` | med |
