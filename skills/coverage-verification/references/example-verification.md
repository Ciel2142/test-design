# Verification: order creation

Coverage: docs/superpowers/coverage/2026-07-30-order-creation-coverage.md
Baseline: a1b2c3d · Run: 2026-07-30T14:02Z
Approved by: alex — "go ahead, run it here" (2026-07-30T14:01Z)
Verdict: BLOCKED — 2 gaps, 1 vacuous, 1 mutation survived, 1 live failure, 1 live inconclusive

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

## Layer 2 — adversarial read (1 canary, synthesized from SC-01 — caught)

2 rows read: HOLDS 1 · WEAK 0 · VACUOUS 1 — real-source verdicts only; the canary copy's
own `VACUOUS` is canary evidence, not a row verdict, and is excluded from this tally

```
SC-01 HOLDS
  asserts the thrown message AND that no row persisted; mutating <= to < makes qty=0
  succeed, so count() becomes 1 and the second assertion fails
SC-03 VACUOUS
  only asserts spy.toHaveBeenCalledOnce; the mutation changes persisted state, which this
  test never inspects
```

Canary: one, synthesized from SC-01.

No natural one was available — layer 1's flagged rows are `SC-02` and `SC-05`,
neither in this batch (`SC-01`, `SC-03`), and neither had a prior `WEAK`/`VACUOUS` verdict
before this run started. So one was synthesized from `SC-01`: the batch was dispatched
(`dispatch 1`, canary-bearing) with `SC-01`'s `expect(count()).toBe(0)` replaced by
`expect(count()).toBeGreaterThanOrEqual(0)`, which holds for both `0` and `1` and so no
longer binds to the `<=`→`<` mutation. The critic returned `VACUOUS` on that copy — canary
caught — so `SC-03`'s verdict stands. `SC-01` was then re-dispatched alone with its real
source (`dispatch 2`, canary-free, inheriting dispatch 1's canary evidence), returning the
`HOLDS` above. Nothing was written to disk; the weakened copy existed only in the dispatch
prompt.

## Layer 3 — live (transcripts re-grepped by main thread)

```
W0 happy    PASS          EXPECT 201 → got 201                                    no SC row (baseline happy path)
W2 dep down FAIL          EXPECT 503 and "rows":0 → got 500                       SC-05
W5 parallel INCONCLUSIVE  1 iteration only; needs a stated count with fail-if-any  SC-03
SC-05 LIVE-N/A (layer 1 gap)  no tagged test at all (see layer 1 gap)
```

`W2` exercises exactly the scenario `SC-05` describes, but the row has no tagged test, so
its formal layer-3 verdict is `LIVE-N/A (layer 1 gap)` — `W2`'s `FAIL` is diagnostic
evidence for item 3 below, not a substitute for tagging the row.

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
