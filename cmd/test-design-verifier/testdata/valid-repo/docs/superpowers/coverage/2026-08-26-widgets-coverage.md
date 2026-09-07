# Coverage: widgets

Spec: docs/superpowers/specs/2026-08-26-widgets-design.md
Baseline: a1b2c3d · 4 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go-test-json
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: ["reports/*.json"]
- baseline-evidence: docs/superpowers/coverage/2026-08-26-widgets-baseline.json
- baseline-sha256: 9f8e7d6c
- live-profile: .test-design/live-profiles/local.json
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-08-26-widgets-coverage.md"]

## Live decision

- decision: required
- reason: the dependency-down and create-then-fetch paths are only observable on a deployed revision

| Journey | Scenarios | Expected |
|---|---|---|
| J-1 | SC-03, SC-04 | a downed dependency answers 503 and a created widget round-trips on the deployed revision |

## Scenarios

### Behavior: create a widget

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | widget name is required | throws "name required", no row persisted | unit | guard removed | med |
| SC-02 | write rolls back when the commit hook fails | 0 rows persisted | integration | rollback call omitted on the error path | high |
| SC-03 | dependency down answers 503, not 500 | 503, body reports rows: 0 | smoke | error mapped to the generic handler instead of the unavailable branch | med |
| SC-04 | full create-then-fetch round trip succeeds | fetched widget matches the created payload | e2e | fetch path returns a stale cache entry | high |
