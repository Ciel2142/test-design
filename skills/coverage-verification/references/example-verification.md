# Verification: docs/superpowers/coverage/2026-07-30-order-creation-coverage.md

- Source: `docs/superpowers/coverage/2026-07-30-order-creation-coverage.md` (sha256 `9f2b6c1e4a7d8f0351c6a9e2d4b7f8091a2c3d4e5f60718293a4b5c6d7e8f901`)
- Head commit: `4c1d90ab7e2f5638c0a1b2d3e4f5061728394a5b`
- Baseline: `a1b2c3d4e5f60718293a4b5c6d7e8f9012345678` (ok)
- Run: `run-2026-07-30-0001`
- Verifier: `1.2.0` · Schema: `1`
- Approved by: alex — "go ahead, run it here" (2026-07-30T14:01Z)
- **Verdict: BLOCKED**

## Freshness

Result: fresh · Baseline: `a1b2c3d4e5f60718293a4b5c6d7e8f9012345678` → Head commit: `4c1d90ab7e2f5638c0a1b2d3e4f5061728394a5b`

| Path | Basis |
| --- | --- |
| internal/orders/create.go | adds an input-validation check; does not change the create behavior scenarios test |

## Layer 1 — trace

Status: blocked · Suite passed 12 · failed 0 · skipped 0 · new failures 0

Findings: gap 1

| Row | Level | Risk | Layer 1 | Scope/Name |
| --- | --- | --- | --- | --- |
| SC-01 | unit | high | bound | test-design/internal/orders TestRejectsZeroQuantity |
| SC-02 | unit | med | bound | test-design/internal/orders TestPersistsAcceptedOrder |
| SC-03 | integration | high | bound | test-design/internal/orders TestUniquenessUnderRetry |
| SC-04 | smoke | med | bound | test-design/internal/orders TestCreateOrderSmoke |
| SC-05 | e2e | high | gap |  |

## Layer 2 — adversarial read

Status: blocked · Applicable 5 · Holds 3 · Weak 0 · Vacuous 1 · Missing 1

| Row | Layer 2 | Batch |
| --- | --- | --- |
| SC-01 | HOLDS | batch-2 |
| SC-02 | VACUOUS | batch-1 |
| SC-03 | HOLDS | batch-1 |
| SC-04 | HOLDS | batch-1 |
| SC-05 | MISSING | batch-1 |

| Batch | Run | Rows | Reused from |
| --- | --- | --- | --- |
| batch-1 | run-2026-07-30-0001 | SC-01, SC-02, SC-03, SC-04, SC-05 | - |
| batch-2 | run-2026-07-30-0001 | SC-01 | - |

## Layer 3 — live

Status: blocked · Applicable 2 · Pass 1 · Fail 1 · Inconclusive 0 · N/A 0

| Row | Layer 3 |
| --- | --- |
| SC-01 | N/A |
| SC-02 | N/A |
| SC-03 | N/A |
| SC-04 | LIVE-FAIL |
| SC-05 | LIVE-PASS |

## Layer 4 — mutation

Status: blocked · Applicable 3 · Caught 1 · Survived 1 · Unappliable 0 · N/A 1

| Row | Layer 4 | Surviving Sites |
| --- | --- | --- |
| SC-01 | MUTATION-CAUGHT | - |
| SC-02 | N/A | - |
| SC-03 | MUTATION-SURVIVED | site-2 |
| SC-04 | N/A | - |
| SC-05 | MUTATION-N/A (layer 1 gap) | - |

Calibration: Comparable 2 · Agree 1 · Holds→Survived SC-03 · Flagged→Caught - · Suspect SC-04

## Work

Fresh 6 · Reused 0 · Failed 3 · Incomplete 0

| Kind | ID | Rows | Source | Outcome | Standing | Artifact |
| --- | --- | --- | --- | --- | --- | --- |
| phase | trace | - | fresh | ok | ok | `docs/superpowers/coverage/transcripts/run-2026-07-30-0001/trace-evidence.json` |
| review-group | batch-1 | SC-02, SC-03, SC-04, SC-05 | fresh | blocked | failed | `docs/superpowers/coverage/transcripts/run-2026-07-30-0001/review-batch-1.json` |
| review-group | batch-2 | SC-01 | fresh | holds | ok | `docs/superpowers/coverage/transcripts/run-2026-07-30-0001/review-batch-2.json` |
| live-attempt | live | - | fresh | fail | failed | `docs/superpowers/coverage/transcripts/run-2026-07-30-0001/live-evidence.json` |
| mutation-row | SC-01 | SC-01 | fresh | caught | ok | `docs/superpowers/coverage/transcripts/run-2026-07-30-0001/mutation-SC-01.json` |
| mutation-row | SC-03 | SC-03 | fresh | survived | failed | `docs/superpowers/coverage/transcripts/run-2026-07-30-0001/mutation-SC-03.json` |

## Transcripts

- `docs/superpowers/coverage/transcripts/run-2026-07-30-0001/live-evidence.json` (sha256 `1c9a0f3b8d27465e0a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f6071`)
- `docs/superpowers/coverage/transcripts/run-2026-07-30-0001/mutation-SC-01.json` (sha256 `2d0b1e4c9a38576f1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f607182`)
- `docs/superpowers/coverage/transcripts/run-2026-07-30-0001/mutation-SC-03.json` (sha256 `3e1c2f5d0b49687a2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293`)
- `docs/superpowers/coverage/transcripts/run-2026-07-30-0001/review-batch-1.json` (sha256 `5a3c9e0f1b2d4768c0a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f607`)
- `docs/superpowers/coverage/transcripts/run-2026-07-30-0001/review-batch-2.json` (sha256 `6b4dae1023c5879d1c2d3e4f5061728394a5b6c7d8e9f0a1b2c3d4e5f6071829`)
- `docs/superpowers/coverage/transcripts/run-2026-07-30-0001/trace-evidence.json` (sha256 `4f2d3a6e1c507a8b3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f6071829304`)

## What must happen before this branch can finish

- [1] [SC-05] gap — no test cites SC-05
- [2] [SC-02] VACUOUS — only asserts that the repository save method was called; the row's Verification mutation changes persisted state, which this test never inspects
- [2] [SC-05] MISSING — no source found at internal/orders/create_test.go; the row has no tagged test
- [3] [SC-04] LIVE-FAIL — the live walkthrough failed
- [4] [SC-03] MUTATION-SURVIVED — the tagged test did not fail at site-2
