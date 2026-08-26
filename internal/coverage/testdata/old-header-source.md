# Coverage: order creation

Spec: docs/superpowers/specs/2026-07-30-order-creation-design.md
Baseline: a1b2c3d · 1 passed, 0 failed, 0 skipped

## Scenarios

| ID | Behavior | Bug it catches | Level | Expected | Risk |
|----|----------|-----------------|-------|----------|------|
| SC-01 | quantity at the lower bound is rejected | guard uses `<` instead of `<=` | unit | throws "qty must be > 0", no row persisted | med |
