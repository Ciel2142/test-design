# Coverage: fixture maven project

Spec: docs/superpowers/specs/2026-08-30-maven-fixture-design.md
Baseline: BASELINE_COMMIT · 0 passed, 0 failed, 0 skipped

## Verification

- runner-kind: maven
- runner-root: .
- runner-command: ["mvn", "-B", "test"]
- report-globs: ["target/surefire-reports/TEST-*.xml"]
- baseline-evidence: docs/superpowers/coverage/2026-08-30-maven-fixture-baseline.json
- baseline-sha256: BASELINE_SHA256
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-08-30-maven-fixture-coverage.md"]

## Scenarios

### Behavior: fixture suite outcomes

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | a passing test is recorded as passed | OrdersTest.passesDeliberately passes | unit | assertion inverted | low |
| SC-02 | a failing test is recorded as failed | OrdersTest.failsDeliberately fails | unit | seeded failure removed | low |
| SC-03 | a disabled test is recorded as skipped | OrdersTest.skippedDeliberately is skipped | unit | @Disabled removed | low |
| SC-04 | a nested class test binds by Outer$Inner | OuterTest$InnerTest.nestedPasses passes | unit | nesting flattened | low |
| SC-05 | parameterized invocations collapse to one family | ParamTest.allPositive passes for all values | unit | one invocation seeded to fail | low |
| SC-06 | an uncited row is reported as a gap | no test cites SC-06 | unit | deliberately uncited | low |
| SC-07 | quantity validation guard rejects non-positive input | Orders.isValidQuantity rejects qty <= 0 | unit | guard uses `<` instead of `<=` | high |
