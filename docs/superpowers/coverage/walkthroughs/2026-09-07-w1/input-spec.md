# Order and payment validation — approved requirements

Subject: the `orders` and `payments` packages of `fixtures/go-project`.
Status: approved 2026-09-07.

This document states what a caller can observe. Transport, storage, reporting and the
shape of the code are out of scope; every requirement below is written so that a caller
holding only the public functions can tell whether it holds.

## 1. Order quantity validation

- R-1.1 An order quantity of one or more is accepted: validating it yields no error.
- R-1.2 An order quantity of zero or less is rejected. The rejection carries a message
  stating that the quantity must be greater than zero.

## 2. Reservation guards

- R-2.1 A reservation for a positive quantity is accepted.
- R-2.2 A reservation for zero or fewer units is rejected. The rejection carries a message
  stating that the reservation quantity must be positive.
- R-2.3 A batch reservation is accepted when every quantity in the batch is positive.
- R-2.4 A batch reservation is rejected when any one of its quantities is not positive,
  wherever in the batch that quantity sits — first, last or in the middle. The rejection
  carries the same message as R-2.2.
- R-2.5 A batch that carries no quantities at all is accepted; there is nothing in it to
  reject.
- R-2.6 Reservations made concurrently against the same stock never oversubscribe it: the
  reserved total never exceeds what was available, and a caller that cannot be satisfied is
  told so rather than silently succeeding.

## 3. Payment amount and currency

- R-3.1 A payment amount of one cent or more is accepted.
- R-3.2 A payment amount of zero or less is rejected. The rejection carries a message
  stating that the amount must be greater than zero.
- R-3.3 The currency codes USD, EUR and GBP are accepted, and only in that exact uppercase
  spelling.
- R-3.4 Any other currency code is rejected, including an empty code and a lowercase
  spelling of a supported one. The rejection quotes the code it refused.

## 4. Charge idempotency

- R-4.1 A charge is identified by the idempotency key the caller supplied together with the
  account it is charged to. The rules below are written in those terms.
- R-4.2 A charge submitted under a key already seen for that account is refused, and the
  account still holds exactly one charge for that key.
- R-4.3 Charges submitted under distinct keys are independent: each one is recorded.
- R-4.4 An idempotency key belongs to the account that used it. The same key submitted by
  two different accounts records one charge for each of them.
- R-4.5 Two charges submitted concurrently under one key settle exactly once: one caller is
  refused and exactly one charge is recorded.

## 5. Retry backoff

- R-5.1 The delay before retry attempt n is the nth value of the configured schedule.
- R-5.2 No delay is longer than the configured maximum; past the end of the schedule the
  maximum is used.
- R-5.3 Retrying stops at the configured attempt limit. The caller then receives the last
  error from the final attempt.
- R-5.4 Retry loops running concurrently for one charge each observe the scheduled delays,
  and the attempt limit counts that charge's attempts once rather than once per loop.
