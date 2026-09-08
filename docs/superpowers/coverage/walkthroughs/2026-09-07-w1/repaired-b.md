# Coverage draft B (repaired after review): order and payment validation, expanded

Spec: docs/superpowers/coverage/walkthroughs/2026-09-07-w1/input-spec.md
Baseline: PENDING_BASELINE_COMMIT · 0 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go
- runner-root: fixtures/go-project
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/walkthroughs/2026-09-07-w1/draft-b-baseline.json
- baseline-sha256: PENDING_BASELINE_SHA256
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/walkthroughs/2026-09-07-w1/draft-b.md"]

## Live decision

- decision: not-applicable
- reason: every rule here is settled in process, the concurrency rows included: they run two callers against one in-process instance rather than against a deployed revision, and the module serves no endpoint

## Retired

Retired after the W-1 review (review-b.md), revision 1 of the reviewer instructions — each merged into the row named:

- SC-17: merged into SC-01
- SC-18: merged into SC-01
- SC-19: merged into SC-02
- SC-20: merged into SC-02
- SC-22: merged into SC-03
- SC-23: merged into SC-04
- SC-24: merged into SC-04
- SC-29: merged into SC-26
- SC-32: merged into SC-05
- SC-33: merged into SC-06
- SC-34: merged into SC-06
- SC-38: merged into SC-08
- SC-41: merged into SC-11
- SC-44: merged into SC-14

## Scenarios

Cost after review: 31 surviving scenarios (43 before review; 14 retired, 2 added), 5 high-risk obligations, 0 live journeys.

### Behavior: order quantity validation

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | an order quantity above the minimum is accepted | ValidateQuantity(5) returns nil | unit | the guard rejects every quantity greater than zero | med |
| SC-02 | an order quantity of zero is rejected | ValidateQuantity(0) returns a non-nil error whose message states that the quantity must be greater than zero | unit | the guard compares with `<` where the rule needs `<=` | med |
| SC-16 | the smallest accepted order quantity is accepted | ValidateQuantity(1) returns nil | unit | the guard rejects at one instead of at zero | med |
| SC-21 | a negative order quantity is rejected | ValidateQuantity(-1) returns a non-nil error whose message states that the quantity must be greater than zero | unit | the guard rejects only an exact zero (`== 0`), so negative quantities pass | med |

### Behavior: reservation guards

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-03 | a positive reservation quantity is accepted | Reserve(5) returns nil | unit | the reservation guard rejects every positive quantity | med |
| SC-04 | a reservation for zero units is rejected | Reserve(0) returns a non-nil error whose message states that the reservation quantity must be positive | unit | the reservation guard compares with `<` where the rule needs `<=` | med |
| SC-47 | a negative reservation quantity is rejected | Reserve(-1) returns a non-nil error whose message states that the reservation quantity must be positive | unit | the reservation guard rejects only an exact zero (`== 0`), so negative quantities pass | med |
| SC-25 | a batch whose quantities are all positive is accepted | ReserveBatch over 2, 3 and 4 returns nil | unit | the per-element guard rejects positive quantities | med |
| SC-26 | a batch is rejected when its first quantity is not positive | ReserveBatch over 0, 3 and 4 returns a non-nil error whose message states that the reservation quantity must be positive | unit | the loop guards only the last element | med |
| SC-27 | a batch is rejected when only its last quantity is not positive | ReserveBatch over 2, 3 and 0 returns a non-nil error whose message states that the reservation quantity must be positive | unit | the loop stops after the first element | med |
| SC-28 | a batch carrying no quantities is accepted | ReserveBatch over an empty list returns nil | unit | a length check ahead of the loop rejects the empty batch | med |
| SC-30 | concurrent reservations never oversubscribe the stock | with one unit available, exactly one of two concurrent reservations returns nil, the other returns a non-nil error, and the reserved total ends at one | integration | the reserved total is read and written without holding the lock | high |

### Behavior: payment amount and currency

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-05 | a positive payment amount is accepted | ValidateAmount(100) returns nil | unit | the amount guard rejects every positive amount | med |
| SC-06 | a payment amount of zero is rejected | ValidateAmount(0) returns a non-nil error whose message states that the amount must be greater than zero | unit | the amount guard compares with `<` where the rule needs `<=` | med |
| SC-07 | a supported currency code is accepted | ValidateCurrency("EUR") returns nil | unit | EUR is dropped from the supported set | med |
| SC-08 | an unsupported currency code is rejected | ValidateCurrency("JPY") returns a non-nil error whose message quotes JPY | unit | the unsupported branch returns nil | med |
| SC-31 | the smallest chargeable amount is accepted | ValidateAmount(1) returns nil | unit | the amount guard rejects everything below a minimum larger than one cent | med |
| SC-35 | a negative payment amount is rejected | ValidateAmount(-50) returns a non-nil error whose message states that the amount must be greater than zero | unit | the guard rejects only an exact zero (`== 0`), so negative amounts pass | med |
| SC-36 | a second supported currency code is accepted | ValidateCurrency("USD") returns nil | unit | USD is dropped from the supported set | med |
| SC-37 | a third supported currency code is accepted | ValidateCurrency("GBP") returns nil | unit | GBP is dropped from the supported set | med |
| SC-39 | a lowercase spelling of a supported code is rejected | ValidateCurrency("usd") returns a non-nil error whose message quotes usd | unit | the code is upper-cased before the comparison | med |
| SC-40 | an empty currency code is rejected | ValidateCurrency("") returns a non-nil error whose message quotes the empty code | unit | the empty code is added to the supported set | med |

### Behavior: charge idempotency

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-11 | a charge repeated under a key already seen for the account is refused | the second charge under that key returns a non-nil error and the account still holds exactly one charge for the key | unit | the seen-key lookup is skipped before the charge is recorded | high |
| SC-12 | charges under distinct idempotency keys are independent | two charges submitted under different keys both return nil and the account holds two charges | unit | the seen-key set is keyed by the charge amount instead of by the key | med |
| SC-42 | two charges submitted concurrently under one key settle exactly once | with two callers submitting the same key at the same time, one returns nil, the other returns a non-nil error, and the account holds exactly one charge for that key | integration | the seen-key check and the record are no longer applied as one atomic step | high |
| SC-43 | an idempotency key belongs to the account that used it | the same key submitted by two different accounts records one charge for each account | unit | the seen-key set is keyed by the key alone, without the account | high |

Risk rationale: SC-43 high — an account-blind key set refuses a second account's charge as a duplicate although it is a distinct payment, so a real payment is dropped, the payer is told the charge already exists, and nothing surfaces the loss until reconciliation.

### Behavior: retry backoff

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-14 | the delay before each retry follows the configured schedule | the delays observed before attempts 1, 2 and 3 equal the first three values of the configured schedule | unit | the backoff formula uses a base other than the configured one | med |
| SC-15 | retrying stops at the configured attempt limit | once the limit is reached the caller receives the last attempt's error and no further attempt is made | unit | the attempt counter is compared with `<` where the rule needs `<=` | med |
| SC-45 | a delay past the end of the schedule is capped | for an attempt beyond the schedule's length the observed delay equals the configured maximum and no more | unit | the cap is applied before the schedule value is computed | med |
| SC-48 | a schedule value above the configured maximum is capped | for an attempt whose schedule value exceeds the configured maximum the observed delay equals the maximum | unit | the cap is applied only past the end of the schedule | med |
| SC-46 | concurrent retry loops for one charge do not compound its attempts | with two retry loops running for the same charge, each observed delay still matches the schedule and the charge's attempt count ends at the limit rather than at twice the limit | integration | the attempt counter is shared between loops without synchronisation | high |
