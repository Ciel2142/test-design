# Verification: docs/superpowers/coverage/2026-09-07-verification-workflow-redesign-coverage.md

- Source: `docs/superpowers/coverage/2026-09-07-verification-workflow-redesign-coverage.md` (sha256 `ddf7a90910b294cb6b9aadc6d189bde21925a02e0388bcaaa565021f8358cf20`)
- Head commit: `ba67b0334e8d11848a56ace794bbbffe42ea8b1f`
- Baseline: `122401156dbf639afb5d7ce021e3a0ae2dbb0824` (ok)
- Run: `run-2026-09-11-0001`
- Verifier: `0.6.0` · Schema: `1`
- Approved by: go
- **Verdict: BLOCKED**

## Layer 1 — trace

Status: pass · Suite passed 1493 · failed 0 · skipped 2 · new failures 0

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
| SC-01 | HOLDS | batch-1r |  |
| SC-02 | HOLDS | batch-2 |  |
| SC-03 | HOLDS | batch-2 |  |
| SC-04 | HOLDS | batch-2r |  |
| SC-05 | HOLDS | batch-2 |  |
| SC-06 | HOLDS | batch-3r |  |
| SC-07 | HOLDS | batch-3 |  |
| SC-08 | HOLDS | batch-3 |  |
| SC-09 | HOLDS | batch-4b |  |
| SC-10 | HOLDS | batch-4b |  |
| SC-11 | HOLDS | batch-4b |  |
| SC-12 | HOLDS | batch-4br |  |
| SC-13 | HOLDS | batch-4r |  |
| SC-14 | HOLDS | batch-4b |  |
| SC-15 | HOLDS | batch-5 |  |
| SC-16 | HOLDS | batch-5r |  |
| SC-17 | HOLDS | batch-5 |  |
| SC-18 | HOLDS | batch-5 |  |
| SC-19 | HOLDS | batch-5 |  |
| SC-20 | HOLDS | batch-6b |  |
| SC-21 | HOLDS | batch-6b |  |
| SC-22 | HOLDS | batch-6br |  |
| SC-23 | HOLDS | batch-6b |  |
| SC-24 | HOLDS | batch-6r |  |
| SC-25 | HOLDS | batch-7 |  |
| SC-26 | HOLDS | batch-7r |  |
| SC-27 | HOLDS | batch-7 |  |
| SC-28 | HOLDS | batch-7 |  |
| SC-29 | HOLDS | batch-1 |  |
| SC-30 | HOLDS | batch-8r |  |
| SC-31 | HOLDS | batch-8cr |  |
| SC-32 | HOLDS | batch-8br |  |
| SC-33 | HOLDS | batch-1 |  |
| SC-34 | HOLDS | batch-9 |  |
| SC-35 | HOLDS | batch-9 |  |
| SC-36 | HOLDS | batch-9r |  |

| Batch | Dispatch | Rows | Canary Row | Canary Kind | Weakening | Caught | Inherits From | Voided |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| batch-1 |  | SC-01, SC-29, SC-33 | SC-01 | synthesized | the next-id boundary assertion `unprefixed.Next != 3` loosened to `unprefixed.Next < 2`, so ignoring inactive owners (which yields 2) no longer flips it | true | - | false |
| batch-1r |  | SC-01 | - | - | - | - | batch-1 | false |
| batch-2 |  | SC-02, SC-03, SC-04, SC-05 | SC-04 | synthesized | skipped-status boundary loosened to accept "skipped" or "passed", both wantFinding(gap,...) calls dropped, and the downstream report.Compute layer-1-bound block removed | true | - | false |
| batch-2r |  | SC-04 | - | - | - | - | batch-2 | false |
| batch-3 |  | SC-06, SC-07, SC-08 | SC-06 | synthesized | the errors.As fatal replaced with a bare `return` when no error is present, so a decision defaulted to N/A silently passes every incomplete-form case | true | - | false |
| batch-3r |  | SC-06 | - | - | - | - | batch-3 | false |
| batch-4 |  | SC-09, SC-10, SC-11, SC-12, SC-13, SC-14 | SC-13 | synthesized | run-B survival checks and requireSentinelIntact replaced with a cleanup-shape check (cleanupOK true, two cleanup steps) | false | - | true |
| batch-4b |  | SC-09, SC-10, SC-11, SC-12, SC-14 | SC-12 | synthesized | requireOwnedCleanup dropped from the two failure subtests and the plan-cleanup entry removed from the expected unboundFailures, so a return before the cleanup section flips nothing | true | - | false |
| batch-4br |  | SC-12 | - | - | - | - | batch-4b | false |
| batch-4r |  | SC-13 | - | - | - | - | batch-4b | false |
| batch-5 |  | SC-15, SC-16, SC-17, SC-18, SC-19 | SC-16 | synthesized | the verdict-order table trimmed to the single "success then failure" case, the one ordering last-write-wins happens to agree with | true | - | false |
| batch-5r |  | SC-16 | - | - | - | - | batch-5 | false |
| batch-6 |  | SC-20, SC-21, SC-22, SC-23, SC-24 | SC-24 | synthesized | requireRestoredWorkspace, requireRecoveredProgress and the rec.Recreated assertions dropped, leaving only the original-repo-untouched invariants | false | - | true |
| batch-6b |  | SC-20, SC-21, SC-22, SC-23 | SC-22 | synthesized | every outcome assertion read after the restart removed - record outcomes, resume outcomes, counts.Failed, IsFailedOutcome - leaving only statuses, pending and superseded bookkeeping | true | - | false |
| batch-6br |  | SC-22 | - | - | - | - | batch-6b | false |
| batch-6r |  | SC-24 | - | - | - | - | batch-6b | false |
| batch-7 |  | SC-25, SC-26, SC-27, SC-28 | SC-26 | synthesized | subtests trimmed to "isolated" and "exempt"; the shared-ownership, unknown-ownership and production cases - the only ones that reach the widening arms - removed | true | - | false |
| batch-7r |  | SC-26 | - | - | - | - | batch-7 | false |
| batch-8 |  | SC-30, SC-31, SC-32 | SC-30 | synthesized | the exact slices.Equal on state.Pending in the live-answered subtest loosened to a slices.Contains check for the publish phase | false | - | true |
| batch-8b |  | SC-31, SC-32 | SC-32 | synthesized | the post-publication-commit assertions removed - measuredCommit and headCommit on the acknowledgement re-run, publication.measuredCommit, state.Stale and the head-moved refusal | false | - | true |
| batch-8c |  | SC-31, SC-32 | SC-31 | synthesized | all six subtests removed, leaving only the initial publication's PASS verdict, its two output paths and a non-empty RunDir - nothing that observes a file the run never wrote | true | - | false |
| batch-8cr |  | SC-31 | - | - | - | - | batch-8c | false |
| batch-8r |  | SC-30 | - | - | - | - | batch-8c | false |
| batch-9 |  | SC-34, SC-35, SC-36 | SC-36 | synthesized | run B's work-counts equality, the freshB DeepEqual, the greet.Source/greet.Reuse assertions and the reused markdown bullet all dropped, leaving only a non-empty item list, the ## Work heading and the cost comparisons | true | - | false |
| batch-9r |  | SC-36 | - | - | - | - | batch-9 | false |
| batch-8br |  | SC-32 | - | - | - | - | batch-8c | false |

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

Status: blocked · Applicable 22 · Caught 19 · Survived 3 · Unappliable 0 · N/A 0

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
| SC-13 | MUTATION-CAUGHT | - |
| SC-14 | MUTATION-CAUGHT | - |
| SC-15 | MUTATION-CAUGHT | - |
| SC-16 | MUTATION-CAUGHT | - |
| SC-17 | MUTATION-CAUGHT | - |
| SC-18 | MUTATION-CAUGHT | - |
| SC-19 | N/A | - |
| SC-20 | MUTATION-CAUGHT | - |
| SC-21 | MUTATION-CAUGHT | - |
| SC-22 | MUTATION-SURVIVED | site-5 |
| SC-23 | MUTATION-CAUGHT | - |
| SC-24 | MUTATION-SURVIVED | site-5 |
| SC-25 | N/A | - |
| SC-26 | MUTATION-CAUGHT | - |
| SC-27 | MUTATION-SURVIVED | site-1 |
| SC-28 | MUTATION-CAUGHT | - |
| SC-29 | MUTATION-CAUGHT | - |
| SC-30 | MUTATION-CAUGHT | - |
| SC-31 | MUTATION-CAUGHT | - |
| SC-32 | MUTATION-CAUGHT | - |
| SC-33 | N/A | - |
| SC-34 | N/A | - |
| SC-35 | MUTATION-CAUGHT | - |
| SC-36 | N/A | - |

## Transcripts

- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/live-evidence.json` (sha256 `7cdbd076901434226fe391326dc2e9ab5283eac90e739e5f020858ee2e8d90f1`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-03.json` (sha256 `11b9d86ec137563f1d0a357ca39bf9e231379d90c0754f106af06400cd42fdc2`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-11.json` (sha256 `51d9a142ab7c1ec287251ac79b29c98b7fda8baec48fa786df25a12406802107`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-12.json` (sha256 `750fe4852240a35fd8010dd2ff7d247bc554ae893da011088e3fd7dada07071c`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-13.json` (sha256 `5d115ab64be3729eb953946066a3f9f3781da9ef2fe42d27ef1ae26f386ec391`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-14.json` (sha256 `cce7c1c5c88991fe53df023408ad5ebb81f360df160c822d0899e2a380b9c96d`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-15.json` (sha256 `8681e85e23bf814d172e568950f6e24c687986fd912f65630875354484e40a39`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-16.json` (sha256 `a7198e5f6dbec23fc020b85e3f680d89593b9a700014ff275542b1ee68d1e239`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-17.json` (sha256 `d50ed5aaacd814d5f019846c398e639a99f1bfe4fb439fce74f510ab11a48e8d`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-18.json` (sha256 `e58ed75a22b96792b5a48bd7e43ea3f63c3daa6bad3d10dbfb207b61f12dd148`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-20.json` (sha256 `d05ddb95f3fbd535a0730e4a8f4076ad5651582485e6a10b41c42817ca1b6685`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-21.json` (sha256 `8fb8d66ae1324896953cf54e45f2a2479190bde867c7bc39a62fffbaef88878b`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-22.json` (sha256 `4acc63ed3d0d9888169dc89fe8bfd2f1fc06057cdda49bd47fa3c22b01a29a7f`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-23.json` (sha256 `6cb72fa1c2a30e29fed50de54d390103938bbd9e497fc306d4955f6a13b7cc37`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-24.json` (sha256 `d17f7aecaa36d91262db258266c93c6630a731259003e758c4d7257ac8c8382f`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-26.json` (sha256 `7f28f15aab46277fbaea43165532c4f0f54c2087887b2d439bc44e025931b6fd`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-27.json` (sha256 `7601552388c23742dfe04312de8556557130541a22a28c7572e30459c4a6a982`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-28.json` (sha256 `ef3a2176726dd664216534e00a7e285fa60fcb86b28978cc5c264b5673593cdd`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-29.json` (sha256 `3ee9fd4b60904151db12d4f5bb5934bf449176a95dcd59c32794674d4492b3aa`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-30.json` (sha256 `547f116ad91ad3e6a3b3a8ddc71b0dd95567934c2bfbc7edbfa4b696232b57ec`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-31.json` (sha256 `03543488f7a17869b16b0cb64651738bd2284b3a006714584cf64ced5b8cdd82`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-32.json` (sha256 `da2e81a14d0b5cc7ea84f34ed27ca1f4b0a64df739ac68899e33f92b653a55fd`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/mutation-SC-35.json` (sha256 `03ac082f6d3a57d794986679d05de48a6c6a9f9775929fe079939e9631829420`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0001/trace-evidence.json` (sha256 `6de9d772271f263474da5516f8eba540b3c54069f65247f0177491596a1aef51`)

## What must happen before this branch can finish

- [4] [SC-22] MUTATION-SURVIVED — the tagged test did not fail at site-5
- [4] [SC-24] MUTATION-SURVIVED — the tagged test did not fail at site-5
- [4] [SC-27] MUTATION-SURVIVED — the tagged test did not fail at site-1
