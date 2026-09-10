# Verification: docs/superpowers/coverage/2026-09-07-verification-workflow-redesign-coverage.md

- Source: `docs/superpowers/coverage/2026-09-07-verification-workflow-redesign-coverage.md` (sha256 `b45e5ac06a5fb6693dc407c21a5c12357795853236189519f009cc680d63d3a2`)
- Head commit: `dee1832206b4b5df8db5ef62f5194e8d310043f6`
- Baseline: `122401156dbf639afb5d7ce021e3a0ae2dbb0824` (ok)
- Run: `run-2026-09-10-0002`
- Verifier: `0.6.0` · Schema: `1`
- Approved by: proceed
- **Verdict: BLOCKED**

## Layer 1 — trace

Status: pass · Suite passed 1491 · failed 0 · skipped 2 · new failures 0

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
| SC-01 | HOLDS | batch-1 | dispatch-1 |
| SC-02 | HOLDS | batch-2 | dispatch-2 |
| SC-03 | HOLDS | batch-2 | dispatch-2 |
| SC-04 | HOLDS | batch-2 | dispatch-2 |
| SC-05 | HOLDS | batch-2r | dispatch-4 |
| SC-06 | HOLDS | batch-3 | dispatch-3 |
| SC-07 | HOLDS | batch-3r | dispatch-5 |
| SC-08 | HOLDS | batch-3 | dispatch-3 |
| SC-09 | HOLDS | batch-4b | dispatch-11 |
| SC-10 | HOLDS | batch-4b | dispatch-11 |
| SC-11 | HOLDS | batch-4br | dispatch-13 |
| SC-12 | HOLDS | batch-4r | dispatch-22 |
| SC-13 | HOLDS | batch-4b | dispatch-11 |
| SC-14 | HOLDS | batch-4b | dispatch-11 |
| SC-15 | HOLDS | batch-5 | dispatch-8 |
| SC-16 | HOLDS | batch-5r | dispatch-10 |
| SC-17 | HOLDS | batch-5 | dispatch-8 |
| SC-18 | HOLDS | batch-5 | dispatch-8 |
| SC-19 | HOLDS | batch-5 | dispatch-8 |
| SC-20 | HOLDS | batch-6 | dispatch-9 |
| SC-21 | HOLDS | batch-6 | dispatch-9 |
| SC-22 | HOLDS | batch-6r | dispatch-12 |
| SC-23 | HOLDS | batch-6 | dispatch-9 |
| SC-24 | HOLDS | batch-6 | dispatch-9 |
| SC-25 | HOLDS | batch-7b | dispatch-16 |
| SC-26 | HOLDS | batch-7br | dispatch-18 |
| SC-27 | HOLDS | batch-7b | dispatch-16 |
| SC-28 | HOLDS | batch-7r | dispatch-23 |
| SC-29 | HOLDS | batch-1 | dispatch-1 |
| SC-30 | HOLDS | batch-8b | dispatch-20 |
| SC-31 | HOLDS | batch-8br | dispatch-21 |
| SC-32 | HOLDS | batch-8r | dispatch-24 |
| SC-33 | HOLDS | batch-1r | dispatch-6 |
| SC-34 | HOLDS | batch-9 | dispatch-17 |
| SC-35 | HOLDS | batch-9r | dispatch-19 |
| SC-36 | HOLDS | batch-9 | dispatch-17 |

| Batch | Dispatch | Rows | Canary Row | Canary Kind | Weakening | Caught | Inherits From | Voided |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| batch-1 | dispatch-1 | SC-01, SC-29, SC-33 | SC-33 | synthesized | dropped the proofRefused/counts assertions and the resume Uncovered loop, leaving only a presence check that some decision exists for G-ship | true | - | false |
| batch-2 | dispatch-2 | SC-02, SC-03, SC-04, SC-05 | SC-05 | synthesized | dropped the BLOCKED verdict and reason-detail assertions and loosened the SC-03 Layer2.Verdict==HOLDS check to a non-empty Reason check | true | - | false |
| batch-3 | dispatch-3 | SC-06, SC-07, SC-08 | SC-07 | synthesized | removed the two journey-widening cases: required journeys widening past smoke and e2e, and retired/unknown journey references not being credited | true | - | false |
| batch-2r | dispatch-4 | SC-05 | - | - | - | - | batch-2 | false |
| batch-3r | dispatch-5 | SC-07 | - | - | - | - | batch-3 | false |
| batch-1r | dispatch-6 | SC-33 | - | - | - | - | batch-1 | false |
| batch-4 | dispatch-7 | SC-09, SC-10, SC-11, SC-12, SC-13, SC-14 | SC-12 | synthesized | removed both requireOwnedCleanup calls | false | - | true |
| batch-5 | dispatch-8 | SC-15, SC-16, SC-17, SC-18, SC-19 | SC-16 | synthesized | removed the inconclusive step definition and three ordering cases, so no case pairs a failure or an inconclusive with a success | true | - | false |
| batch-6 | dispatch-9 | SC-20, SC-21, SC-22, SC-23, SC-24 | SC-22 | synthesized | dropped every recorded-outcome assertion: the blocked and survived checks, the outcome column of the table, counts.Failed==3, and the superseded-outcome check, leaving status-only checks | true | - | false |
| batch-5r | dispatch-10 | SC-16 | - | - | - | - | batch-5 | false |
| batch-4b | dispatch-11 | SC-09, SC-10, SC-11, SC-12, SC-13, SC-14 | SC-11 | synthesized | removed the two subtests that assert a nonzero-exit start command and a nonzero-exit health command stop the run before the next phase | true | - | false |
| batch-6r | dispatch-12 | SC-22 | - | - | - | - | batch-6 | false |
| batch-4br | dispatch-13 | SC-11 | - | - | - | - | batch-4b | false |
| batch-7 | dispatch-14 | SC-25, SC-26, SC-27, SC-28 | SC-28 | synthesized | dropped the batch.RunID prior-run assertion and every published-copy identity assertion on the reused review and mutation artifacts | false | - | true |
| batch-8 | dispatch-15 | SC-30, SC-31, SC-32 | SC-32 | synthesized | loosened the measuredCommit and headCommit equality checks to non-empty checks, dropped publication.measuredCommit, and narrowed the refusal-message check to one substring | false | - | true |
| batch-7b | dispatch-16 | SC-25, SC-26, SC-27, SC-28 | SC-26 | synthesized | removed the production and unknown subtests, the two that assert shared and unknown ownership widen invalidation | true | - | false |
| batch-9 | dispatch-17 | SC-34, SC-35, SC-36 | SC-35 | synthesized | removed the layer4Applicable high-risk assertion and the missing subtest that expects a refusal naming SC-02 | true | - | false |
| batch-7br | dispatch-18 | SC-26 | - | - | - | - | batch-7b | false |
| batch-9r | dispatch-19 | SC-35 | - | - | - | - | batch-9 | false |
| batch-8b | dispatch-20 | SC-30, SC-31, SC-32 | SC-31 | synthesized | removed the ignored-sentinel-survives and tracked-restored-untracked-removed subtests | true | - | false |
| batch-8br | dispatch-21 | SC-31 | - | - | - | - | batch-8b | false |
| batch-4r | dispatch-22 | SC-12 | - | - | - | - | batch-4b | false |
| batch-8r | dispatch-24 | SC-32 | - | - | - | - | batch-8b | false |
| batch-7r | dispatch-23 | SC-28 | - | - | - | - | batch-7b | false |

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

Status: blocked · Applicable 22 · Caught 16 · Survived 6 · Unappliable 0 · N/A 0

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
| SC-20 | MUTATION-SURVIVED | site-2 |
| SC-21 | MUTATION-SURVIVED | site-2 |
| SC-22 | MUTATION-SURVIVED | site-1, site-3 |
| SC-23 | MUTATION-CAUGHT | - |
| SC-24 | MUTATION-CAUGHT | - |
| SC-25 | N/A | - |
| SC-26 | MUTATION-SURVIVED | site-2 |
| SC-27 | MUTATION-SURVIVED | site-4 |
| SC-28 | MUTATION-CAUGHT | - |
| SC-29 | MUTATION-CAUGHT | - |
| SC-30 | MUTATION-SURVIVED | site-3 |
| SC-31 | MUTATION-CAUGHT | - |
| SC-32 | MUTATION-CAUGHT | - |
| SC-33 | N/A | - |
| SC-34 | N/A | - |
| SC-35 | MUTATION-CAUGHT | - |
| SC-36 | N/A | - |

## Transcripts

- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/live-evidence.json` (sha256 `09df664bf1d342445650606bfbe3d8a558777a17c22ec815e83fe50db2b231d7`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-03.json` (sha256 `cd05f766d611ba425d59f4373c2b765120a3ee4d1eb1a8d2beb42cb9c522bc55`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-11.json` (sha256 `f99609d395fe9337a202119490deb273b7eda88a521761909c240dcae33a0ed5`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-12.json` (sha256 `81eaf96b3c2ddafe466e8f8e9f5ea307827ae0ed9834fcf0905ed6dee29b3af5`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-13.json` (sha256 `3daa30e57ee05a0e272adf95178b6e5a924c88adc46a830801fc29735f11b2be`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-14.json` (sha256 `d901f1a93f5ba914c1c906ef355136b8cb8481a3dc9326f4ff064742a7febf73`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-15.json` (sha256 `bda1b59fe2ca2e55f70163dc3100825a9266ca9755bcf93622da19921dca5cba`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-16.json` (sha256 `75aa44cd652e194f813068333c3c156df6847968f50ee7d3770ae5db432c0aab`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-17.json` (sha256 `d495c03ede4e690fda7b395a3439ed300820acdb2d99b5c8504619857c1e4ef3`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-18.json` (sha256 `a7a4495d55e1a11cea46ce7afd4068c4ed829f8958f3d20d9e4188ddea8671e2`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-20.json` (sha256 `816741b80c0a99ddd900b737b512cafbc2761827454ab67ab1a8145aa0f0caa6`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-21.json` (sha256 `c9bdae96572dca2320a447e7ed81484aca80291e615ddd39a3a300dc08067eed`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-22.json` (sha256 `4e147ac42097b2c6e72b39d47b5f851a8e95f7aae4e76f8c37c28f7fc05740f6`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-23.json` (sha256 `b7ec273cf84c1d649fc29c9c1be714dda064f99b1a5ad2db20a49e47f06b5957`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-24.json` (sha256 `6450c0646cda3e407c352da771ba020bd569704cccc703393eef4568a3d4e88c`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-26.json` (sha256 `6b52fd2cf00f6c3a3e98fbe19283794806c1e8adac705215cba698e8a9479b46`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-27.json` (sha256 `906df511f8807ed3094fe43b548d81d04d516a400c3e11998b4c70f06129120e`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-28.json` (sha256 `519c44223e83c2dafbbdbf789ca9f65af274ed5045a79f0368d72897e65ddad5`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-29.json` (sha256 `701f0d40816506b504ea21a660fe504fa52fdaa8330a664a38ef9ac093e24cd7`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-30.json` (sha256 `688b10795fd0ae37f3097d68b02a415f524f6320e7b3e36bff8498f7e6230cd1`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-31.json` (sha256 `ceb95700a534abe5986adde88b10d59f650c3d81e999fa12636af879f2ef4c36`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-32.json` (sha256 `dc5f83da0f5b2e9d175aa10978b7ff76eb6ee9afe92dc2fe4743ed80c2f6fc54`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/mutation-SC-35.json` (sha256 `46c4687aa9fd96fcff59cb75977fdac50a60f199a64bd5ff7e39274c0fd14340`)
- `docs/superpowers/coverage/transcripts/run-2026-09-10-0002/trace-evidence.json` (sha256 `fab5d98dd0aa5197b69e8d4109e7af7ecf21c5b5043516b70b9f8c014aa3bade`)

## What must happen before this branch can finish

- [freshness] [] stale — the coverage source is stale against the spec: cmd/test-design-verifier/baseline.go:169-214: baseline evidence JVM toolchain identity
- [4] [SC-20] MUTATION-SURVIVED — the tagged test did not fail at site-2
- [4] [SC-21] MUTATION-SURVIVED — the tagged test did not fail at site-2
- [4] [SC-22] MUTATION-SURVIVED — the tagged test did not fail at site-1; site-3
- [4] [SC-26] MUTATION-SURVIVED — the tagged test did not fail at site-2
- [4] [SC-27] MUTATION-SURVIVED — the tagged test did not fail at site-4
- [4] [SC-30] MUTATION-SURVIVED — the tagged test did not fail at site-3
