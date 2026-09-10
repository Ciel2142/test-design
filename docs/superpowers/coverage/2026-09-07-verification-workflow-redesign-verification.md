# Verification: docs/superpowers/coverage/2026-09-07-verification-workflow-redesign-coverage.md

- Source: `docs/superpowers/coverage/2026-09-07-verification-workflow-redesign-coverage.md` (sha256 `e43b07d54bbbe6c7f529a6e891d44bc7dbaa338f30e365dec5ceffb96f4e1566`)
- Head commit: `ff7a8a3965eb08ea3142a2752c63fa2306824371`
- Baseline: `122401156dbf639afb5d7ce021e3a0ae2dbb0824` (ok)
- Run: `run-2026-09-10-0001`
- Verifier: `0.6.0` · Schema: `1`
- Approved by: go ahead
use subagents and worktrees where possible, and cheap models if not necessary for expensive to execute
- **Verdict: BLOCKED**

## Layer 1 — trace

Status: blocked · Suite passed 1485 · failed 0 · skipped 2 · new failures 0

Findings: ambiguity 1

| Row | Level | Risk | Layer 1 | Scope/Name |
| --- | --- | --- | --- | --- |
| SC-01 | integration | med | bound | test-design/cmd/test-design-verifier TestNextIDReservesRetiredHighestAndTraceExcludesRetired |
| SC-02 | integration | med | bound | test-design/cmd/test-design-verifier TestTraceCreditsOneTestForThreeRows |
| SC-03 | integration | high | bound | test-design/cmd/test-design-verifier TestTraceCombinesTwoTestsForOneRow |
| SC-04 | integration | med | bound | test-design/cmd/test-design-verifier TestTraceWithholdsCreditForSkippedContributor |
| SC-05 | unit | med | bound | test-design/internal/report TestDeriveLayer2BlocksWeakVacuousMissing |
| SC-06 | unit | med | bound | test-design/internal/coverage TestValidateLiveDecisionIncompleteForms |
| SC-07 | unit | med | bound | test-design/internal/live TestApplicableRowsFollowRequiredJourneys |
| SC-08 | integration | med | bound | test-design/cmd/test-design-verifier TestLiveHandoff_GapRowExecutesAndStaysTraceGap |
| SC-09 | integration | med | bound | test-design/cmd/test-design-verifier TestLiveHandoff_ResolvedProfileAcceptedAndResourceRemoved |
| SC-10 | smoke | med | bound | test-design/cmd/test-design-verifier TestLiveTruth_DeployedRevisionJourney |
| SC-11 | integration | high | bound | test-design/cmd/test-design-verifier TestLiveTruth_ReadinessGatesBusinessAssertions |
| SC-12 | integration | high | bound | test-design/cmd/test-design-verifier TestLiveTruth_OwnedCleanupAfterFailedPhase |
| SC-13 | integration | high | bound | test-design/cmd/test-design-verifier TestLiveTruth_CleanupConfinedToRun |
| SC-14 | unit | high | bound | test-design/cmd/test-design-verifier TestExecuteLive_SanitizedEnvOmitsUnapprovedCredential |
| SC-15 | integration | high | bound | test-design/cmd/test-design-verifier TestLiveTruth_UnboundAssertionFailureBlocks |
| SC-16 | unit | high | bound | test-design/cmd/test-design-verifier TestExecuteLive_MergeVerdictKeepsEarlierFailure |
| SC-17 | integration | high | bound | test-design/cmd/test-design-verifier TestLiveTruth_EveryCleanupFailureRetained |
| SC-18 | integration | high | bound | test-design/cmd/test-design-verifier TestLiveTruth_IncompleteOutputIsNotProof |
| SC-19 | integration | med | bound | test-design/cmd/test-design-verifier TestLiveHandoff_FinalizeAcceptsRealLiveEvidence |
| SC-20 | integration | high | bound | test-design/cmd/test-design-verifier TestResume_SelectsOnlyValidatedCompletions |
| SC-21 | integration | high | bound | test-design/internal/runstate TestCheckpoint_InterruptedReplacementKeepsPriorCompletion |
| SC-22 | integration | high | bound | test-design/cmd/test-design-verifier TestResume_PreservesFailedOutcomes |
| SC-23 | integration | high | bound | test-design/cmd/test-design-verifier TestRestart_RecoversLiveStateBeforeRepeating |
| SC-24 | integration | high | bound | test-design/cmd/test-design-verifier TestRestart_RestoresMutationWorkspaceBeforeMeasuring |
| SC-25 | integration | med | bound | test-design/cmd/test-design-verifier TestReuse_MatchesCompleteInputsBeforeReuse |
| SC-26 | integration | high | bound | test-design/cmd/test-design-verifier TestReuse_IsolatedRepairInvalidatesOnlyItsGroup |
| SC-27 | integration | high | bound | test-design/cmd/test-design-verifier TestReuse_RejectsAlteredRetainedBytes |
| SC-28 | integration | high | bound | test-design/cmd/test-design-verifier TestReuse_PreservesOriginalProvenanceInReport |
| SC-29 | integration | high | bound | test-design/cmd/test-design-verifier TestReuse_NewRevisionGetsFreshSuiteAndLive |
| SC-30 | integration | high | bound | test-design/cmd/test-design-verifier TestPublication_RetriesFromRetainedEvidence |
| SC-31 | integration | high | bound | test-design/cmd/test-design-verifier TestPublication_RecoversOnlyProvenOwnArtifacts |
| SC-32 | integration | high | bound | test-design/cmd/test-design-verifier TestPublication_CommitKeepsMeasuredRevision |
| SC-33 | integration | med | bound | test-design/cmd/test-design-verifier TestReuse_LegacyRecordsAreHistoricalNotReusable |
| SC-34 | unit | med | bound | test-design/internal/report TestLayer2_CompleteReviewFinalizesWithoutCanaryBookkeeping |
| SC-35 | unit | high | bound | test-design/internal/report TestLayer4_HighRiskObligationSurvivesCanaryRemoval |
| SC-36 | integration | med | bound | test-design/cmd/test-design-verifier TestOrchestration_SummaryMatchesRetainedWork |

## Layer 2 — adversarial read

Status: pass · Applicable 36 · Holds 36 · Weak 0 · Vacuous 0 · Missing 0

| Row | Layer 2 | Batch | Dispatch |
| --- | --- | --- | --- |
| SC-01 | HOLDS | batch-1-real | dispatch-1-real |
| SC-02 | HOLDS | batch-2 | dispatch-2 |
| SC-03 | HOLDS | batch-2 | dispatch-2 |
| SC-04 | HOLDS | batch-2 | dispatch-2 |
| SC-05 | HOLDS | batch-2-real | dispatch-2-real |
| SC-06 | HOLDS | batch-3 | dispatch-3 |
| SC-07 | HOLDS | batch-3-real | dispatch-3-real |
| SC-08 | HOLDS | batch-3 | dispatch-3 |
| SC-09 | HOLDS | batch-4 | dispatch-4 |
| SC-10 | HOLDS | batch-4 | dispatch-4 |
| SC-11 | HOLDS | batch-4 | dispatch-4 |
| SC-12 | HOLDS | batch-4 | dispatch-4 |
| SC-13 | HOLDS | batch-4 | dispatch-4 |
| SC-14 | HOLDS | batch-4-real | dispatch-4-real |
| SC-15 | HOLDS | batch-5 | dispatch-5 |
| SC-16 | HOLDS | batch-5-real | dispatch-5-real |
| SC-17 | HOLDS | batch-5 | dispatch-5 |
| SC-18 | HOLDS | batch-5 | dispatch-5 |
| SC-19 | HOLDS | batch-5 | dispatch-5 |
| SC-20 | HOLDS | batch-6 | dispatch-6 |
| SC-21 | HOLDS | batch-6-real | dispatch-6-real |
| SC-22 | HOLDS | batch-6 | dispatch-6 |
| SC-23 | HOLDS | batch-6 | dispatch-6 |
| SC-24 | HOLDS | batch-6 | dispatch-6 |
| SC-25 | HOLDS | batch-7 | dispatch-7 |
| SC-26 | HOLDS | batch-7 | dispatch-7 |
| SC-27 | HOLDS | batch-7-real | dispatch-7-real |
| SC-28 | HOLDS | batch-7 | dispatch-7 |
| SC-29 | HOLDS | batch-8-real | dispatch-8-real |
| SC-30 | HOLDS | batch-9-real | dispatch-9-real |
| SC-31 | HOLDS | batch-9 | dispatch-9 |
| SC-32 | HOLDS | batch-9 | dispatch-9 |
| SC-33 | HOLDS | batch-10-real | dispatch-10-real |
| SC-34 | HOLDS | batch-11 | dispatch-11 |
| SC-35 | HOLDS | batch-11-real | dispatch-11-real |
| SC-36 | HOLDS | batch-11 | dispatch-11 |

| Batch | Dispatch | Rows | Canary Row | Canary Kind | Weakening | Caught | Inherits From | Voided |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| batch-1 | dispatch-1 | SC-01 | SC-01 | synthesized | boundary loosened: `unprefixed.Next != 3` replaced with `unprefixed.Next < 2`, so a NextID that ignores the retired SC-02 (returning 2) still passes | true | - | false |
| batch-1-real | dispatch-1-real | SC-01 | - | - | - | - | batch-1 | false |
| batch-2 | dispatch-2 | SC-02, SC-03, SC-04, SC-05 | SC-05 | synthesized | blocking-outcome assertions dropped: wantVerdict(BLOCKED) and wantReason(layer 2, SC-03, verdict) removed; the copy asserts only the recorded Layer2.Verdict string (copied from the batch regardless of the blocking branch) plus a non-existent Layer2.Reason field, so removing the weak/vacuous/missing blocking branch still passes | true | - | false |
| batch-2-real | dispatch-2-real | SC-05 | - | - | - | - | batch-2 | false |
| batch-3 | dispatch-3 | SC-06, SC-07, SC-08 | SC-07 | synthesized | exact expected-set string comparison replaced by strings.Contains(got, "SC-2/smoke") containment for every positive case, so a smoke/e2e-only gate that ignores journeys still passes | true | - | false |
| batch-3-real | dispatch-3-real | SC-07 | - | - | - | - | batch-3 | false |
| batch-4 | dispatch-4 | SC-09, SC-10, SC-11, SC-12, SC-13, SC-14 | SC-14 | synthesized | absence assertions replaced by presence assertions: the step reads the allowlisted PATH (exit 0) instead of the withheld canary (exit 1), the exact `len(run.env) == 1` check becomes a ContainsFunc presence check for PATH=, and the empty-allowlist case is dropped, so returning the complete parent environment still passes | true | - | false |
| batch-4-real | dispatch-4-real | SC-14 | - | - | - | - | batch-4 | false |
| batch-5 | dispatch-5 | SC-15, SC-16, SC-17, SC-18, SC-19 | SC-16 | synthesized | order-sensitive table cases removed: [fail,pass], [fail,inconclusive] and [inconclusive,pass] replaced by [pass,pass], [pass,inconclusive] and [fail], so every expected verdict equals the last step's verdict and a last-write-wins merge still passes | true | - | false |
| batch-5-real | dispatch-5-real | SC-16 | - | - | - | - | batch-5 | false |
| batch-6 | dispatch-6 | SC-20, SC-21, SC-22, SC-23, SC-24 | SC-21 | synthesized | interruption seam removed: the checkpointWrite override that fails mid-write and the interrupted Start g2 are deleted, leaving only happy-path persistence assertions that a direct truncating write also satisfies | true | - | false |
| batch-6-real | dispatch-6-real | SC-21 | - | - | - | - | batch-6 | false |
| batch-7 | dispatch-7 | SC-25, SC-26, SC-27, SC-28 | SC-27 | synthesized | tamper steps removed: no artifact is truncated, deleted or rewritten between adoption and publication and no refusal (artifact-digest-mismatch / artifact-missing / reused-artifact-changed) is asserted; the copy asserts only untampered adoption, file presence and a transcript digest that trivially equals the recorded one, so trusting path/envelope without re-hashing still passes | true | - | false |
| batch-7-real | dispatch-7-real | SC-27 | - | - | - | - | batch-7 | false |
| batch-8 | dispatch-8 | SC-29 | SC-29 | synthesized | trace/live freshness assertions dropped: out.Trace/out.Live never-adopted checks, the resumed live-attempt pending check, and the BLOCKED verdict with red / LIVE-FAIL reasons are removed, leaving only review/mutation adoption decisions and headCommit checks that a reuse adopting prior trace or live results still satisfies | true | - | false |
| batch-8-real | dispatch-8-real | SC-29 | - | - | - | - | batch-8 | false |
| batch-9 | dispatch-9 | SC-30, SC-31, SC-32 | SC-30 | synthesized | retry path removed: the destination-only repair and the second finalize run are deleted along with the Recovered / Publication completed / verification-records-unchanged assertions, so the copy asserts only the refusal shape and never observes whether the retry repeats verification work | true | - | false |
| batch-9-real | dispatch-9-real | SC-30 | - | - | - | - | batch-9 | false |
| batch-10 | dispatch-10 | SC-33 | SC-33 | synthesized | refusal assertions dropped: proofRefused(...) with the legacy-no-manifest / manifest-version-unsupported reason and the Refused/Reused counts check replaced by a bare presence check that a decision for G-ship exists, so an admission that synthesizes a manifest and adopts the legacy record still passes | true | - | false |
| batch-10-real | dispatch-10-real | SC-33 | - | - | - | - | batch-10 | false |
| batch-11 | dispatch-11 | SC-34, SC-35, SC-36 | SC-35 | synthesized | obligation assertions dropped: the layer4Applicable(active, nil) high-risk check and the "missing" subtest (nil Mutations must refuse missing-verdict naming SC-02) are removed; only the survived/caught outcome subtests remain, which pass with explicitly supplied mutation entries even when r.Risk == "high" no longer makes the row applicable | true | - | false |
| batch-11-real | dispatch-11-real | SC-35 | - | - | - | - | batch-11 | false |

## Layer 3 — live

Status: pass · Applicable 1 · Pass 1 · Fail 0 · Inconclusive 0 · N/A 0

| Row | Layer 3 |
| --- | --- |
| SC-01 | N/A |
| SC-02 | N/A |
| SC-03 | N/A |
| SC-04 | N/A |
| SC-05 | N/A |
| SC-06 | N/A |
| SC-07 | N/A |
| SC-08 | N/A |
| SC-09 | N/A |
| SC-10 | LIVE-PASS |
| SC-11 | N/A |
| SC-12 | N/A |
| SC-13 | N/A |
| SC-14 | N/A |
| SC-15 | N/A |
| SC-16 | N/A |
| SC-17 | N/A |
| SC-18 | N/A |
| SC-19 | N/A |
| SC-20 | N/A |
| SC-21 | N/A |
| SC-22 | N/A |
| SC-23 | N/A |
| SC-24 | N/A |
| SC-25 | N/A |
| SC-26 | N/A |
| SC-27 | N/A |
| SC-28 | N/A |
| SC-29 | N/A |
| SC-30 | N/A |
| SC-31 | N/A |
| SC-32 | N/A |
| SC-33 | N/A |
| SC-34 | N/A |
| SC-35 | N/A |
| SC-36 | N/A |

## Layer 4 — mutation

Status: blocked · Applicable 22 · Caught 15 · Survived 5 · Unappliable 2 · N/A 0

| Row | Layer 4 | Surviving Sites |
| --- | --- | --- |
| SC-01 | N/A | - |
| SC-02 | N/A | - |
| SC-03 | MUTATION-CAUGHT | - |
| SC-04 | N/A | - |
| SC-05 | N/A | - |
| SC-06 | N/A | - |
| SC-07 | N/A | - |
| SC-08 | N/A | - |
| SC-09 | N/A | - |
| SC-10 | N/A | - |
| SC-11 | MUTATION-CAUGHT | - |
| SC-12 | MUTATION-CAUGHT | - |
| SC-13 | MUTATION-UNAPPLIABLE | - |
| SC-14 | MUTATION-CAUGHT | - |
| SC-15 | MUTATION-CAUGHT | - |
| SC-16 | MUTATION-CAUGHT | - |
| SC-17 | MUTATION-CAUGHT | - |
| SC-18 | MUTATION-SURVIVED | site-2 |
| SC-19 | N/A | - |
| SC-20 | MUTATION-SURVIVED | site-1, site-2, site-5, site-7, site-8 |
| SC-21 | MUTATION-CAUGHT | - |
| SC-22 | MUTATION-SURVIVED | site-5 |
| SC-23 | MUTATION-CAUGHT | - |
| SC-24 | MUTATION-CAUGHT | - |
| SC-25 | N/A | - |
| SC-26 | MUTATION-CAUGHT | - |
| SC-27 | MUTATION-SURVIVED | site-4 |
| SC-28 | MUTATION-CAUGHT | - |
| SC-29 | MUTATION-CAUGHT | - |
| SC-30 | MUTATION-UNAPPLIABLE | - |
| SC-31 | MUTATION-SURVIVED | site-1, site-4 |
| SC-32 | MUTATION-CAUGHT | - |
| SC-33 | N/A | - |
| SC-34 | N/A | - |
| SC-35 | MUTATION-CAUGHT | - |
| SC-36 | N/A | - |

## Transcripts

- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/live-evidence.json` (sha256 `68c5ddf301771e5750d6ce782e2ecf971ad448b2adef686fd5976434e4ced765`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-03.json` (sha256 `fa1dff087933286bc2c01420af30a45f5c9e587d00e3d217832a72dd09033df2`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-11.json` (sha256 `cf8543a90c7dc7757825007888bfbbdb302b1d6fbd3c82dad939b39b0ed6bb4c`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-12.json` (sha256 `f97901b63084dc94b107bab21dd274f31da4a9523e5de7a614d5037a27404dc3`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-14.json` (sha256 `4cb29d5106005002039aa16bd3e92a3fc2b3645b68c93b8ffc9e5b11eead4e86`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-15.json` (sha256 `a5557b2213f2072cc89aa3f387406b298658bcd96ba297a24897b471fbe5e220`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-16.json` (sha256 `8aeb2c5e777e9a8e074fa9d647de40bbea961183445780fcc43f19ab18f5cea4`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-17.json` (sha256 `14504cb0b294cf1a0dc5ff698202ab272de67cd414edea0792052d49ce744c41`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-18.json` (sha256 `7236696617dd2c930aaedf15df98d18286bbf41a22dc8266e0caf94991638408`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-20.json` (sha256 `feef57d2c336cbbb3cd2e09da64398234c5d9b12ca6a13946c5dba80282902a0`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-21.json` (sha256 `17428684417b202687129e2a13bde79479226e10a3afc5d9dee30f90480384e6`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-22.json` (sha256 `c46daf09d9d0edebe136ed7bbeecf8d21996e1d9dddd3439a1a7743df09c3544`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-23.json` (sha256 `62043864e101f1ddd00373937c81ea6fb4d516c7eb0b95aa46fcf67885f5b522`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-24.json` (sha256 `705bf9994dded9400010070961772456f71adfee013b2c5482e2e30170f79f68`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-26.json` (sha256 `3ff1664d6d72ea5429d56fb1b26feddaca763c87b2b80c8e6152d435afe7d376`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-27.json` (sha256 `858d7c6030e578faf16465a19d73daff18e1136dd88a8f94164153ce9df72a25`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-28.json` (sha256 `ccdf1c342cd5b5c096e5382a2fef6570a8d4e51375c49296a9092b9304a01a88`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-29.json` (sha256 `f6c190c1727807ad7be16ac9a35ed0f7d15493efd5a4b1070655c3303a399b32`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-31.json` (sha256 `0367f8de77638b49c0ac0cc1662bec34e598a06935591e11a3d4ac056cd1d6cb`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-32.json` (sha256 `5fea644e43522ef34daa25b118fe9fde0b3649a7a9539f3c5dc5e3da2ec6eb00`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/mutation-SC-35.json` (sha256 `2519b07d4fcc34089627c060b3db7c657e58cb5b20b490a15ddebcdf122d6d63`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0001/trace-evidence.json` (sha256 `0df36f43608cf82cc02803a6cd963e1d803cdac4f5d22a86ebdeb3802ae15c2a`)

## What must happen before this branch can finish

- [freshness] [] stale — the coverage source is stale against the spec: internal/jvmrun/jvmrun.go:29-71: JVM report-glob confinement: confineGlob now validates every path segment and pattern validity, refusing globs previously accepted; ReportDirs removed; internal/jvmrun/jvmrun.go:110-214: JVM report collection: no longer RemoveAll's report directories, drops the mtime cutoff, walks under os.Root, refuses symlinked or non-regular report paths (report-glob-escapes-root) and errors on unreadable directories — runner behavior the source declares out of scope and unchanged; cmd/test-design-verifier/finalize.go:978-995: artifactStem accepts any non-empty .md basename, so finalize publishes for a source not named *-coverage.md instead of refusing source-name-unconventional; internal/live/transcript.go:29-51: redaction barrier now also scans the plan step's expectation text and substitutes [withheld by redaction barrier]; source says existing redaction remains unchanged; internal/subprocess/run.go:1-99: verifier's own subprocess policy: 30-minute suite timeout and 30-second git utility timeout with process-group kill, returned as infrastructure errors; internal/live/plan.go:86-94: live plan validation refuses a bound step with no predicates (missing-row-predicates); cmd/test-design-verifier/preflight.go:64-72: preflight resolves a relative --source against --repo instead of refusing source-outside-repo; internal/runstate/lock.go:1-45: ledger reads/writes take shared/exclusive flock on ledger.lock, a concurrent-parent safety guarantee the source scopes out; cmd/test-design-verifier/publication.go:359-430: finalize fingerprints the ledger and refuses ledger-changed when another caller altered it between assembly and publication; internal/runstate/runstate.go:110-123: run ledger location: TEST_DESIGN_STATE_DIR override, default under the user cache dir; cmd/test-design-verifier/recordreview.go:104-151: record-review refuses invalid-group-id for a group id that is not a plain name; cmd/test-design-verifier/recordreview.go:443-470: a scenario row may belong to only one current review group: row-already-grouped refusal at record-review and reuse
- [1] [] ambiguity — internal/tags/extract_test.go:17: comment is inside a function body, not immediately above a test function
- [4] [SC-13] MUTATION-UNAPPLIABLE — The row's Verification mutation (broaden the approved project recipe cleanup selector from the exact run resource to a shared prefix) applies only in fixtures/workflow-redesign-live/recipe.py:154, a Python fixture, not in any production .go file; the mutator enumerated exactly that one site and applied it (root = /tmp/test-design-workflow-<run_id stripped of trailing digits>), but a Go-runner row cannot be evaluated on a non-.go diff, so the site was not confirmed and evaluate-mutation was not called. Coverage-design defect: the mutation is phrased against the trusted recipe, outside the verifier's mutation surface.
- [4] [SC-18] MUTATION-SURVIVED — the tagged test did not fail at site-2
- [4] [SC-20] MUTATION-SURVIVED — the tagged test did not fail at site-1; site-2; site-5; site-7; site-8
- [4] [SC-22] MUTATION-SURVIVED — the tagged test did not fail at site-5
- [4] [SC-27] MUTATION-SURVIVED — the tagged test did not fail at site-4
- [4] [SC-30] MUTATION-UNAPPLIABLE — Confirmation failed at site-2 (cmd/test-design-verifier/resume.go:308, staleForRun): the applied diff `if false && measured == id.RepoCommit` removes the exemption for a HEAD that moved only because this run's own report was committed, so resume judges staleness against publication HEAD — that is SC-32's fault (publication commit advancing the measurement target), not SC-30's named mutation (routing a publication *failure* through all-layer invalidation instead of publication-only recovery). Sites 1 (pendingRefusal no longer skips the owed publish phase) and 3 (pendingObligations re-owes every completed record while a publication is started) did match and were caught; site-2 was evaluated before this thread's confirmation rejected it and survived, so the row's evidence at test-design-verifier-1175555114/runs/run-2026-09-10-0001/mutation-evidence.json is not used.
- [4] [SC-31] MUTATION-SURVIVED — the tagged test did not fail at site-1; site-4
