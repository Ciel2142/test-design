# Coverage: fixture gradle kotlin project

Spec: docs/superpowers/specs/2026-08-30-gradle-fixture-design.md
Baseline: BASELINE_COMMIT · 0 passed, 0 failed, 0 skipped

## Verification

- runner-kind: gradle
- runner-root: .
- runner-command: ["gradle", "test", "--rerun"]
- report-globs: ["build/test-results/test/TEST-*.xml"]
- baseline-evidence: docs/superpowers/coverage/2026-08-30-gradle-fixture-baseline.json
- baseline-sha256: BASELINE_SHA256
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-08-30-gradle-fixture-coverage.md"]

## Scenarios

### Behavior: fixture suite outcomes

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | a backtick-named test binds and passes | OrderKtTest.`passes with backticks and spaces` passes | unit | assertion inverted | low |
| SC-02 | a failing test is recorded as failed | OrderKtTest.failsDeliberately fails | unit | seeded failure removed | low |
| SC-03 | a display-named test cannot bind an identity | OrderKtTest.displayNamed renders as free text in XML | unit | @DisplayName removed | low |
