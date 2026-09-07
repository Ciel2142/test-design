# W-1 seeds: the answer key

Written together with `input-spec.md`, `draft-a.md`, `draft-b.md` and `risk-contrast.md`
and committed before the reviewer's instructions existed, so no seed could be tuned to the
instructions. Nothing here is repeated or hinted at in the inputs themselves.

Finding kinds are the review vocabulary: MERGE, DROP, REWRITE, ADD, RISK, LIVE, ESCALATE.

## draft-a.md — exactly four seeded defects, and nothing else

15 rows. `SC-09`, `SC-10` and `SC-13` are the three defective rows; the fourth seed is an
absence, not a row. Every other row is intended to be a good row: an observable expected
result, a mutation that would really break it, a risk that follows the consequence.

| Seed | Row | Kind | What is wrong | Expected finding |
|------|-----|------|---------------|------------------|
| A-1 | SC-10 | redundant | Same verification mutation as `SC-06` (the amount guard's `<` where `<=` is needed) and the same observable outcome (a non-nil error for an amount of 0), reworded as "below the smallest chargeable amount". One mutation cannot be caught twice by two rows that assert the same thing. | MERGE, citing SC-10 into SC-06 |
| A-2 | SC-13 | invented | A lapsed loyalty tier appears nowhere in `input-spec.md` and nowhere in `orders/orders.go` or `payments/payments.go`. There is no payer profile, no tier and no freshness check anywhere in the subject. | DROP |
| A-3 | SC-09 | unobservable expected result | The expected result asserts how the lookup is compiled — a jump table rather than a walk over the supported set. No caller of `ValidateCurrency` can observe that, and the stated mutation (rewriting the switch as a linear scan) changes no observable behaviour, so no test could ever fail on it. | REWRITE (DROP is also defensible: the row has no observable behaviour left once the implementation claim is removed) |
| A-4 | `ReserveBatch` per-element guard | omitted real behavior | `input-spec.md` R-2.3, R-2.4 and R-2.5 require a batch reservation to be accepted when every quantity is positive, rejected when any one is not positive wherever it sits in the batch, and accepted when empty. `fixtures/go-project/orders/orders.go:19-26` implements exactly that loop. Draft A's reservation group carries only `Reserve` rows (SC-03, SC-04) and no `ReserveBatch` row at all. | ADD |

Draft A carries a scope note under `## Scenarios` that defers, by name, the edge values,
the further currency spellings of R-3.4, the delay cap of R-5.2, the account scoping of
R-4.4 and the three concurrency requirements R-2.6, R-4.5 and R-5.4. Those deferrals exist
so that seed A-4 is the table's only unexplained absence. `input-spec.md` R-2.3, R-2.4 and
R-2.5 are not among the deferred rules. A reviewer that argues a named deferral should not
have been deferred — concurrency most plausibly — is making a legitimate scope argument; it
is neither a hit on a seed nor a false positive, and the resolution records it as an
unseeded extra.

Seeded risk defects in draft A: none. Its only `high` is `SC-11` (a repeated charge taking
the money twice is an irreversible external side effect, a named category, so it needs no
rationale line). Every other row is `med` because its failure is an ordinary rejected
input. A reviewer that raises a RISK finding on draft A is producing a false positive.

## draft-b.md — merge or keep for all 43 rows

Expected surviving count: **27** (12 rows carried over from draft A plus 15 genuinely
distinct boundary and concurrency rows). 16 rows are superficial duplicates and should be
merged away. 43 is over forty, so a focused redundancy and scope review is expected before
approval; splitting the table across two documents is not a resolution.

### Merge (16 rows) — expected finding kind MERGE

| Row | Duplicates | Why it is superficial |
|-----|-----------|-----------------------|
| SC-17 | SC-01 | Different input value (7 instead of 5) in the same accepted class, same outcome, same mutation |
| SC-18 | SC-01 | Different input value (1000) in the same accepted class; the mutation is the same guard restated loosely |
| SC-19 | SC-02 | Reworded ("an order line carrying no units"), same input 0, same outcome, same mutation |
| SC-20 | SC-02 | Same failure named at a different call site (the checkout path); the guard and the outcome are identical |
| SC-22 | SC-03 | Different input value (250) in the same accepted class, same outcome, same mutation |
| SC-23 | SC-04 | Reworded, same input 0, same outcome, same mutation |
| SC-24 | SC-04 | Same failure named at a different call site (the order service) |
| SC-29 | SC-26 | Same per-element guard, same first-element-invalid batch, same outcome; only the literal quantities differ |
| SC-32 | SC-05 | Different input value (2599) in the same accepted class, same outcome, same mutation |
| SC-33 | SC-06 | Reworded ("zero-value payment"), same input 0, same outcome, same mutation |
| SC-34 | SC-06 | Same failure named at a different call site (the refund path) |
| SC-36 | SC-07 | Another member of the same supported set; one table-driven row covers USD, EUR and GBP |
| SC-37 | SC-07 | Another member of the same supported set, as above |
| SC-38 | SC-08 | Another member of the unsupported class; same default branch, same outcome |
| SC-41 | SC-11 | Same seen-key mutation and the same observable outcome as SC-11, reworded as a retry |
| SC-44 | SC-14 | A single point already asserted by SC-14, which observes attempts 1, 2 and 3 |

### Keep (27 rows)

| Row | Why it survives |
|-----|-----------------|
| SC-01 | Carried from draft A: the accepted-quantity case |
| SC-02 | Carried from draft A: the rejected-quantity case |
| SC-03 | Carried from draft A: the accepted reservation |
| SC-04 | Carried from draft A: the rejected reservation |
| SC-05 | Carried from draft A: the accepted amount |
| SC-06 | Carried from draft A: the rejected amount |
| SC-07 | Carried from draft A: a supported currency code |
| SC-08 | Carried from draft A: an unsupported currency code |
| SC-11 | Carried from draft A: the idempotent repeat charge |
| SC-12 | Carried from draft A: distinct keys stay independent |
| SC-14 | Carried from draft A: the backoff schedule |
| SC-15 | Carried from draft A: the attempt limit |
| SC-16 | Boundary: 1 is the smallest accepted quantity, the value that separates the accepted class from the rejected one; SC-01's 5 does not pin it |
| SC-21 | Boundary: the far side of the same edge. Negative and zero are distinct classes for the shipped guard, which compares with `<` and so rejects -1 while letting 0 through |
| SC-25 | `ReserveBatch` accepting an all-positive batch: a different function from SC-03 with a loop of its own |
| SC-26 | `ReserveBatch` rejecting on the first element: pins that the guard runs at all |
| SC-27 | `ReserveBatch` rejecting on the last element: pins that the loop does not stop early. A different mechanism from SC-26, not a different value |
| SC-28 | Boundary: the empty batch, the only input with no element for the guard to see |
| SC-30 | Concurrency: two reservations against one unit of stock; observes a shared-state invariant no single-caller row can |
| SC-31 | Boundary: one cent, the smallest chargeable amount |
| SC-35 | Boundary: the negative side of the amount edge, distinct from zero for the shipped guard, as SC-21 is for quantity |
| SC-39 | Distinct behavior: case sensitivity of the currency code, not another member of the accepted or rejected set |
| SC-40 | Boundary: the empty currency code |
| SC-42 | Concurrency: two callers submitting one idempotency key at once; observes the atomicity of check-then-record, which SC-11's sequential repeat cannot |
| SC-43 | Distinct behavior: the key's account scope, a second dimension of the key rather than a repeat of SC-11 |
| SC-45 | Boundary: an attempt past the end of the schedule, where the cap rather than the schedule decides |
| SC-46 | Concurrency: two retry loops sharing one charge's attempt counter |

Risk in draft B is intended to be correct throughout, so a RISK finding on draft B is a
false positive. Six rows are `high`: SC-11 and SC-41 (irreversible external side effect —
money taken twice), SC-42 (the same, under concurrency), SC-30 and SC-46 (harmful
shared-state failure), and SC-43, which sits outside the four named categories and
therefore carries the impact rationale line printed under its group.

## risk-contrast.md — one silent downgrade

`SC-02`, the cross-tenant read, is `high` in the original table and `med` in the amended
copy. Nothing else differs between the two tables: SC-01 stays `med`, SC-03, SC-04 and
SC-05 stay `high`, and SC-05's rationale line is reproduced word for word under both.

The downgrade is silent: the amended table carries no amendment note, no rationale, and no
record of a user approval, while a cross-tenant read is a security-boundary violation —
one of the four consequences that are `high` by rule.

| Seed | Row | Kind | Expected finding |
|------|-----|------|------------------|
| R-1 | SC-02 (amended table) | risk silently lowered from high to med after approval | RISK, naming SC-02 and the missing amendment; escalation to the human is also correct, since a post-approval downgrade of a security row is not ordinary cleanup |

Everything else in `risk-contrast.md` is intended to be correct, SC-05's rationale line
included: a saturation consequence is outside the four named categories, so it is the case
that must carry a rationale rather than the case that must be lowered.
