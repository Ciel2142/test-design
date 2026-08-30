# Coverage: basic

Spec: docs/superpowers/specs/2026-01-01-basic-design.md
Baseline: a1b2c3d · 5 passed, 0 failed, 0 skipped

## Retired

- SC-04 merged into SC-02 (same mutation)

## Scenarios

### Behavior: create a widget

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | widget name is required | throws "name required" | unit | guard removed | med |
| SC-02 | widget accepts a name | creates widget | unit | condition inverted | med |
| SC-03 | widget rejects negative qty | throws "qty must be positive" | unit | boundary flipped | med |
| SC-05 | widget rejects duplicate id | throws "duplicate id" | unit | check removed | med |
