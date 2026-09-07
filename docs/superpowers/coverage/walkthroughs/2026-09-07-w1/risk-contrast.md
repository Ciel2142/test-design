# Risk contrast: one table, assigned twice

Subject: the order and payment service of `fixtures/go-project` as it is planned to be
deployed — the same validation rules plus a tenant boundary, a batch write path and an
order search.

Both tables below carry the scenario columns only and reuse the same five IDs, so this
file is a side-by-side comparison of two risk assignments over one row set, not a coverage
source: it has no `## Verification` block, no live decision and duplicate IDs, and it is
not meant to parse as one.

The original table was approved on 2026-09-07. The amended table is the copy that was
circulated afterwards.

## Original (approved 2026-09-07)

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | a payment amount of zero is rejected | validating an amount of 0 returns a non-nil error whose message states that the amount must be greater than zero | unit | the amount guard compares with `<` where the rule needs `<=` | med |
| SC-02 | a read never returns another tenant's order | a request carrying tenant A's credentials for an order owned by tenant B returns not-found and no field of that order | integration | the tenant predicate is dropped from the order lookup | high |
| SC-03 | a batch reservation that fails part-way leaves no units reserved | after a batch whose third quantity is invalid, the reserved total for every quantity in that batch is what it was before the call | integration | the rollback on the partial-failure path is omitted | high |
| SC-04 | two writers updating one order do not lose an update | with two callers updating the same order concurrently, the stored order carries both updates or the second caller is refused; no update is dropped without the caller being told | integration | the read-modify-write runs without taking the row lock | high |
| SC-05 | the order search answers a full page inside the published timeout | a search over the largest supported page size returns before the published two-second timeout, with the page's rows | integration | the per-page limit is removed from the search query | high |

Risk rationale: SC-05 high — an unbounded search holds a pooled connection for the whole
scan, so one ordinary large page drains the pool and stalls every other request for the
tenant until it finishes; the consequence is a whole-service stall under normal load, not
a single slow answer.

## Amended (circulated 2026-09-07)

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | a payment amount of zero is rejected | validating an amount of 0 returns a non-nil error whose message states that the amount must be greater than zero | unit | the amount guard compares with `<` where the rule needs `<=` | med |
| SC-02 | a read never returns another tenant's order | a request carrying tenant A's credentials for an order owned by tenant B returns not-found and no field of that order | integration | the tenant predicate is dropped from the order lookup | med |
| SC-03 | a batch reservation that fails part-way leaves no units reserved | after a batch whose third quantity is invalid, the reserved total for every quantity in that batch is what it was before the call | integration | the rollback on the partial-failure path is omitted | high |
| SC-04 | two writers updating one order do not lose an update | with two callers updating the same order concurrently, the stored order carries both updates or the second caller is refused; no update is dropped without the caller being told | integration | the read-modify-write runs without taking the row lock | high |
| SC-05 | the order search answers a full page inside the published timeout | a search over the largest supported page size returns before the published two-second timeout, with the page's rows | integration | the per-page limit is removed from the search query | high |

Risk rationale: SC-05 high — an unbounded search holds a pooled connection for the whole
scan, so one ordinary large page drains the pool and stalls every other request for the
tenant until it finishes; the consequence is a whole-service stall under normal load, not
a single slow answer.
