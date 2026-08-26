# Coverage: legacy

Spec: docs/superpowers/specs/2026-08-26-legacy-design.md
Baseline: a1b2c3d · 1 passed, 0 failed, 0 skipped

## Scenarios

| ID | Behavior | Bug it catches | Level | Expected | Risk |
|----|----------|-----------------|-------|----------|------|
| SC-01 | quantity at the lower bound is rejected | guard uses `<` instead of `<=` | unit | throws "qty must be > 0", no row persisted | med |
| SC-02 | write rolls back when the commit hook fails | rollback call omitted on the error path | integration | 0 rows persisted | high |
| SC-03 | dependency down answers 503, not 500 | error mapped to the generic handler instead of the unavailable branch | smoke | 503, body reports rows: 0 | high |
| SC-04 | cancel reverses the stock reservation | reversal call omitted | integration | stock count restored | med |
