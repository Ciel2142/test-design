# Verification: order creation

Coverage: docs/superpowers/coverage/2026-07-30-order-creation-coverage.md
Baseline: a1b2c3d · Run: 2026-07-30T14:02Z
Verdict: BLOCKED — 2 gaps, 1 vacuous, 1 mutation survived, 1 live failure

## Layer 1 — trace (full re-run)

3/4 rows tagged · runner reported 3 passed, 1 skipped · 0 new failures vs baseline

```
gap:       SC-05 — no tagged test
gap:       SC-02 — tagged test reported `skipped` by the runner
drift:     none
overclaim: none
stale:     none
```

`SC-02` is the case a grep-only layer 1 would have called covered: the tag is present and
the suite is green, because a skipped test does not fail.

## Layer 2 — adversarial read (canary caught in 1/1 batches)

2 rows read: HOLDS 1 · WEAK 0 · VACUOUS 1

```
SC-01 HOLDS
  asserts the thrown message AND that no row persisted; mutating <= to < makes qty=0
  succeed, so count() becomes 1 and the second assertion fails
SC-03 VACUOUS
  only asserts spy.toHaveBeenCalledOnce; the mutation changes persisted state, which this
  test never inspects
```

## Layer 3 — live (transcripts re-grepped by main thread)

```
W0 happy    PASS          EXPECT 201 → got 201
W2 dep down FAIL          EXPECT 503 and "rows":0 → got 500
W5 parallel INCONCLUSIVE  1 iteration only; needs a stated count with fail-if-any
```

transcripts: `docs/superpowers/coverage/transcripts/W0-happy.txt`, `W2-dep-down.txt`,
`W5-parallel.txt`

Tamper check: HEAD unchanged, stash list unchanged, diff hash unchanged.

## Layer 4 — mutation (risk=high rows)

```
SC-02 MUTATION-N/A (layer 1 gap)  row has no tagged executable test (see layer 1 gap)
SC-05 MUTATION-N/A (layer 1 gap)  no tagged test at all (see layer 1 gap)
SC-03 MUTATION-SURVIVED           uniqueness check moved outside the transaction, test still passed
```

transcripts: `docs/superpowers/coverage/transcripts/SC-03-mutation.txt`

Tamper check: HEAD unchanged, stash list unchanged, diff hash unchanged; working tree clean
apart from the transcripts.

## What must happen before this branch can finish

1. `SC-02` — un-skip the test, or delete the row and record why in `## Retired`
2. `SC-03` — assert persisted state instead of the spy, then re-run layers 2 and 4 for `SC-03`
3. `SC-05` — write the missing smoke test
4. `W2` — map the dependency failure to 503 rather than the generic 500 handler
5. `W5` — give the harness a stated iteration count with fail-if-any
