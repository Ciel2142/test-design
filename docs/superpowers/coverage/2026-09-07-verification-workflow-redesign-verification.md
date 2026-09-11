# Verification: docs/superpowers/coverage/2026-09-07-verification-workflow-redesign-coverage.md

- Source: `docs/superpowers/coverage/2026-09-07-verification-workflow-redesign-coverage.md` (sha256 `ddf7a90910b294cb6b9aadc6d189bde21925a02e0388bcaaa565021f8358cf20`)
- Head commit: `b2d1b229fe98deb6a70689b6fd4ee0ef7abd23b1`
- Baseline: `122401156dbf639afb5d7ce021e3a0ae2dbb0824` (ok)
- Run: `run-2026-09-11-0002`
- Verifier: `0.6.0` · Schema: `1`
- Approved by: go
- **Verdict: BLOCKED**

## Layer 1 — trace

Status: pass · Suite passed 1496 · failed 0 · skipped 2 · new failures 0

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
| SC-01 | HOLDS | rb-01 | dispatch-rb-01 |
| SC-02 | HOLDS | batch-2 | dispatch-2 |
| SC-03 | HOLDS | batch-2 | dispatch-2 |
| SC-04 | HOLDS | batch-2 | dispatch-2 |
| SC-05 | HOLDS | rb-05 | dispatch-rb-05 |
| SC-06 | HOLDS | batch-3 | dispatch-3 |
| SC-07 | HOLDS | rb-07 | dispatch-rb-07 |
| SC-08 | HOLDS | batch-3 | dispatch-3 |
| SC-09 | HOLDS | batch-4 | dispatch-4 |
| SC-10 | HOLDS | batch-4 | dispatch-4 |
| SC-11 | HOLDS | batch-4 | dispatch-4 |
| SC-12 | HOLDS | batch-4 | dispatch-4 |
| SC-13 | HOLDS | batch-4 | dispatch-4 |
| SC-14 | HOLDS | rb-14 | dispatch-rb-14 |
| SC-15 | HOLDS | batch-5 | dispatch-5 |
| SC-16 | HOLDS | rb-16 | dispatch-rb-16 |
| SC-17 | HOLDS | batch-5 | dispatch-5 |
| SC-18 | HOLDS | batch-5 | dispatch-5 |
| SC-19 | HOLDS | batch-5 | dispatch-5 |
| SC-20 | HOLDS | batch-6 | dispatch-6 |
| SC-21 | HOLDS | rb-21 | dispatch-rb-21 |
| SC-22 | HOLDS | batch-6 | dispatch-6 |
| SC-23 | HOLDS | batch-6 | dispatch-6 |
| SC-24 | HOLDS | batch-6 | dispatch-6 |
| SC-25 | HOLDS | batch-12 | dispatch-12 |
| SC-26 | HOLDS | rb-26 | dispatch-rb-26 |
| SC-27 | HOLDS | batch-12 | dispatch-12 |
| SC-28 | HOLDS | rb-28 | dispatch-rb-28 |
| SC-29 | HOLDS | rb-29 | dispatch-rb-29 |
| SC-30 | HOLDS | batch-9 | dispatch-9 |
| SC-31 | HOLDS | batch-9 | dispatch-9 |
| SC-32 | HOLDS | rb-32 | dispatch-rb-32 |
| SC-33 | HOLDS | rb-33 | dispatch-rb-33 |
| SC-34 | HOLDS | batch-11 | dispatch-11 |
| SC-35 | HOLDS | rb-35 | dispatch-rb-35 |
| SC-36 | HOLDS | batch-11 | dispatch-11 |

| Batch | Dispatch | Rows | Canary Row | Canary Kind | Weakening | Caught | Inherits From | Voided |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| batch-1 | dispatch-1 | SC-01 | SC-01 | synthesized | binding assertion loosened from next-id Next == 3 to Next >= 2 | true | - | false |
| batch-2 | dispatch-2 | SC-02, SC-03, SC-04, SC-05 | SC-05 | synthesized | blocking assertions (wantVerdict BLOCKED, wantReason) deleted, leaving only the echoed Layer2.Verdict state check | true | - | false |
| batch-3 | dispatch-3 | SC-06, SC-07, SC-08 | SC-07 | synthesized | exact ApplicableRows set equality replaced with a strings.Contains presence check | true | - | false |
| batch-4 | dispatch-4 | SC-09, SC-10, SC-11, SC-12, SC-13, SC-14 | SC-14 | synthesized | env and verdict assertions dropped, leaving a step-count shape check and a non-empty env check | true | - | false |
| batch-5 | dispatch-5 | SC-15, SC-16, SC-17, SC-18, SC-19 | SC-16 | synthesized | expected per-case verdict equality replaced with a non-empty presence check | true | - | false |
| batch-6 | dispatch-6 | SC-20, SC-21, SC-22, SC-23, SC-24 | SC-21 | synthesized | Load error after the interrupted replacement downgraded to t.Logf plus an early return, so every binding assertion is skipped | true | - | false |
| batch-7 | dispatch-7 | SC-25, SC-26, SC-27, SC-28 | SC-28 | synthesized | prior-run identity equality on the reuse reference and on the published copies replaced with non-empty presence checks | false | - | true |
| batch-8 | dispatch-8 | SC-29 | SC-29 | synthesized | out.Trace/out.Live freshness assertions and the resumed pending block deleted; trace repoCommit equality loosened to non-empty | true | - | false |
| batch-9 | dispatch-9 | SC-30, SC-31, SC-32 | SC-32 | synthesized | measured-commit equality assertions replaced with non-empty checks | true | - | false |
| batch-10 | dispatch-10 | SC-33 | SC-33 | synthesized | proofRefused and the counts/uncovered assertions dropped, leaving a decision-exists check | true | - | false |
| batch-11 | dispatch-11 | SC-34, SC-35, SC-36 | SC-35 | synthesized | layer4Applicable length assertion loosened to len(got) > 1 and the missing-refusal assertion guarded behind errors.As | true | - | false |
| batch-12 | dispatch-12 | SC-25, SC-26, SC-27, SC-28 | SC-26 | synthesized | shared and unknown subtests' class equality and proofEveryRecordRefused replaced with presence checks | true | - | false |
| rb-01 | dispatch-rb-01 | SC-01 | - | - | - | - | batch-1 | false |
| rb-05 | dispatch-rb-05 | SC-05 | - | - | - | - | batch-2 | false |
| rb-07 | dispatch-rb-07 | SC-07 | - | - | - | - | batch-3 | false |
| rb-14 | dispatch-rb-14 | SC-14 | - | - | - | - | batch-4 | false |
| rb-16 | dispatch-rb-16 | SC-16 | - | - | - | - | batch-5 | false |
| rb-21 | dispatch-rb-21 | SC-21 | - | - | - | - | batch-6 | false |
| rb-26 | dispatch-rb-26 | SC-26 | - | - | - | - | batch-12 | false |
| rb-29 | dispatch-rb-29 | SC-29 | - | - | - | - | batch-8 | false |
| rb-32 | dispatch-rb-32 | SC-32 | - | - | - | - | batch-9 | false |
| rb-33 | dispatch-rb-33 | SC-33 | - | - | - | - | batch-10 | false |
| rb-35 | dispatch-rb-35 | SC-35 | - | - | - | - | batch-11 | false |
| rb-28 | dispatch-rb-28 | SC-28 | - | - | - | - | batch-12 | false |

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

Status: blocked · Applicable 22 · Caught 18 · Survived 4 · Unappliable 0 · N/A 0

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
| SC-20 | MUTATION-SURVIVED | site-1 |
| SC-21 | MUTATION-CAUGHT | - |
| SC-22 | MUTATION-SURVIVED | site-1 |
| SC-23 | MUTATION-CAUGHT | - |
| SC-24 | MUTATION-CAUGHT | - |
| SC-25 | N/A | - |
| SC-26 | MUTATION-CAUGHT | - |
| SC-27 | MUTATION-SURVIVED | site-1 |
| SC-28 | MUTATION-SURVIVED | site-2 |
| SC-29 | MUTATION-CAUGHT | - |
| SC-30 | MUTATION-CAUGHT | - |
| SC-31 | MUTATION-CAUGHT | - |
| SC-32 | MUTATION-CAUGHT | - |
| SC-33 | N/A | - |
| SC-34 | N/A | - |
| SC-35 | MUTATION-CAUGHT | - |
| SC-36 | N/A | - |

## Transcripts

- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/live-evidence.json` (sha256 `e7b4982499dec0dea7d50a143f95cdfc2e2325f81e753f0bf401e1ed16e992a8`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-03.json` (sha256 `5c2717092678f170c3ceaad9f1342ef7da8d0844361c9c2ae74ca45176f97f2f`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-11.json` (sha256 `757c95bc7efa4cd58584826e970f292a714a086472f30675fcbecd2f4e508618`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-12.json` (sha256 `c6e6b1378d977caf1e654ac8b22420898f02783457064972cfa80fcfabd8457d`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-13.json` (sha256 `07b25f27757c85a34df4b9e472179ea06147d1979110cd0da4692923c065ddcf`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-14.json` (sha256 `6cc63119fcdde3be3240a9047360d861f48dd41a42fd92ce4b2076bc5c763042`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-15.json` (sha256 `14227677b0138aebd46674603b381864412a16c3f8a5a94135db401eac92f974`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-16.json` (sha256 `83ab34b9cc3584e2cbaaae42f4624f12ae67b1347f26ddf5522529197b0154f2`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-17.json` (sha256 `013db1fe44cf250ca5a0f1b847e1ec4e03d378a3331bd4f8a6ced00b9e62cb54`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-18.json` (sha256 `7abda9364b3fc230957335226735b4c1eb15cdafb1be854f0e9c530218f7a079`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-20.json` (sha256 `976125dfbedb42aa1a0ead281053a7713a77aa9197837d6b807bde2ed8904d7d`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-21.json` (sha256 `4a94f050fa2371d706ba53d6b3a84e12dc1f0c026bb4c8971c02336c3779ad78`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-22.json` (sha256 `e3b05851f4b96117e4a399e4a2bd36a263c7ef992cff60e3e484161dc8c3eb48`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-23.json` (sha256 `856fa655ee3dae97a1453097f0746aebfdb28f032e67f39f9cf92c20dfe36ce8`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-24.json` (sha256 `892de8424aa0256c40fd911eec799d5e8f79f9b785acd45907b4136b8d025381`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-26.json` (sha256 `e74dae892494327ec25ad39ee289d9dda100a1a5bc45805a9ffef59306383e8b`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-27.json` (sha256 `6accd14bcb4c5d321695486deaf92098b76e9a4839a9fe5698671d20f9a05964`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-28.json` (sha256 `0a0cf5c0a846fdae68e89825d48e4d2963d8dae128dc529db1c6c27275964b00`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-29.json` (sha256 `bfb65ee80984aa4c31bd62f1ec4c558504db8fb31a18f58e8af9a39f87aa1c15`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-30.json` (sha256 `68114c91cf394546b721bff991f543ce78d1549932d5ef52f0f4e3b28a0621f0`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-31.json` (sha256 `420252a6aaa2b066840456fd25507d5ccf592b9878c55dfb9a1aa8b2082c257c`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-32.json` (sha256 `a14a14f3b778fdcc4edd824865f44d6c7d4fb3dc3dc8bbfa8f7a8121a8b3b7a9`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/mutation-SC-35.json` (sha256 `43a6b490f67baf173786abb020d1faa7774001ad8cc79fdc270dca500f2af433`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0002/trace-evidence.json` (sha256 `fb2544058fbcbcbc82fa1dc3e996fb73c9c6eb4f093166a7f473938d6fd22cf4`)

## What must happen before this branch can finish

- [4] [SC-20] MUTATION-SURVIVED — the tagged test did not fail at site-1
- [4] [SC-22] MUTATION-SURVIVED — the tagged test did not fail at site-1
- [4] [SC-27] MUTATION-SURVIVED — the tagged test did not fail at site-1
- [4] [SC-28] MUTATION-SURVIVED — the tagged test did not fail at site-2
