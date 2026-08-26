# Orders design

Floor-rule feature: the scenario table lives in the spec, not a separate
coverage document.

## Scenarios

### Behavior: place an order

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| orders/SC-01 | order total includes tax | total = subtotal + tax | unit | tax addition omitted | med |
| orders/SC-02 | cancelled order releases stock | stock count restored | integration | restore call omitted | high |
