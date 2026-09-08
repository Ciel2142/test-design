# Coverage draft A (repaired after review): order and payment validation

Spec: docs/superpowers/coverage/walkthroughs/2026-09-07-w1/input-spec.md
Baseline: PENDING_BASELINE_COMMIT · 0 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go
- runner-root: fixtures/go-project
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/walkthroughs/2026-09-07-w1/draft-a-baseline.json
- baseline-sha256: PENDING_BASELINE_SHA256
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/walkthroughs/2026-09-07-w1/draft-a.md"]

## Live decision

- decision: not-applicable
- reason: every rule here is settled in process by calling the package directly; the module deploys nothing and serves no endpoint, so a deployed revision would show a journey nothing the in-process run does not already show

## Retired

Retired after the W-1 review (review-a.md), revision 1 of the reviewer instructions:

- SC-09: no observable result; the expected result described how the lookup is compiled (review-a.md DROP SC-09)
- SC-10: merged into SC-06: same input, same mutation, weaker expected result (review-a.md DROP SC-10)
- SC-13: no requirement or code names a loyalty tier (review-a.md DROP SC-13)

## Scenarios

Cost after review: 15 surviving scenarios, 1 high-risk obligation, 0 live journeys.

Scope of this draft: one accepted and one rejected case for each rule of `input-spec.md`.
The exact values on either side of each edge, the further currency spellings called out in
R-3.4, the delay cap of R-5.2, the account scoping of R-4.4 and the three concurrency
requirements R-2.6, R-4.5 and R-5.4 are held for a second pass and are deliberately not in
this table.

### Behavior: order quantity validation

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | an order quantity above the minimum is accepted | ValidateQuantity(5) returns nil | unit | the guard rejects every quantity greater than zero | med |
| SC-02 | an order quantity of zero is rejected | ValidateQuantity(0) returns a non-nil error whose message states that the quantity must be greater than zero | unit | the guard compares with `<` where the rule needs `<=` | med |

### Behavior: reservation guards

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-03 | a positive reservation quantity is accepted | Reserve(5) returns nil | unit | the reservation guard rejects every positive quantity | med |
| SC-04 | a reservation for zero units is rejected | Reserve(0) returns a non-nil error whose message states that the reservation quantity must be positive | unit | the reservation guard compares with `<` where the rule needs `<=` | med |
| SC-16 | a batch whose quantities are all positive is accepted | ReserveBatch over 2, 3 and 4 returns nil | unit | the per-element comparison at the batch guard is flipped so positive quantities are rejected | med |
| SC-17 | a batch with a non-positive quantity in the middle position is rejected | ReserveBatch over 2, -1 and 4 returns a non-nil error whose message states that the reservation quantity must be positive | unit | the range loop over the batch stops after its first element | med |
| SC-18 | an empty batch is accepted | ReserveBatch over an empty list returns nil | unit | a length check ahead of the loop rejects the empty batch | med |

### Behavior: payment amount and currency

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-05 | a positive payment amount is accepted | ValidateAmount(100) returns nil | unit | the amount guard rejects every positive amount | med |
| SC-06 | a payment amount of zero is rejected | ValidateAmount(0) returns a non-nil error whose message states that the amount must be greater than zero | unit | the amount guard compares with `<` where the rule needs `<=` | med |
| SC-07 | a supported currency code is accepted | ValidateCurrency("EUR") returns nil | unit | EUR is dropped from the supported set | med |
| SC-08 | an unsupported currency code is rejected | ValidateCurrency("JPY") returns a non-nil error whose message quotes JPY | unit | the unsupported branch returns nil | med |

### Behavior: charge idempotency

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-11 | a charge repeated under a key already seen for the account is refused | the second charge under that key returns a non-nil error and the account still holds exactly one charge for the key | unit | the seen-key lookup is skipped before the charge is recorded | high |
| SC-12 | charges under distinct idempotency keys are independent | two charges submitted under different keys both return nil and the account holds two charges | unit | the seen-key set is keyed by the charge amount instead of by the key | med |

### Behavior: retry backoff

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-14 | the delay before each retry follows the configured schedule | the delays recorded by the injected sleeper before attempts 1, 2 and 3 equal the first three values of the configured schedule | unit | attempt n reads the schedule value for attempt n+1 | med |
| SC-15 | retrying stops at the configured attempt limit | with a limit of 3 the operation is invoked exactly three times and the caller receives the third invocation's error | unit | the attempt counter is compared with `<` where the rule needs `<=` | med |
