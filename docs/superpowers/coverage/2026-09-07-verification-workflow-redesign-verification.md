# Verification: docs/superpowers/coverage/2026-09-07-verification-workflow-redesign-coverage.md

- Source: `docs/superpowers/coverage/2026-09-07-verification-workflow-redesign-coverage.md` (sha256 `69ade28c44a0645c13fb93bc6f7b16fa1313e2777675744422b4d45860c2b98d`)
- Head commit: `4421051232853bdf50a9f61154d3ce2fa18404aa`
- Baseline: `122401156dbf639afb5d7ce021e3a0ae2dbb0824` (ok)
- Run: `run-2026-09-11-0003`
- Verifier: `0.6.0` · Schema: `1`
- Approved by: щл ok
- **Verdict: PASS**

## Layer 1 — trace

Status: pass · Suite passed 1498 · failed 0 · skipped 2 · new failures 0

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
| SC-01 | HOLDS | rb-01 | dispatch-13 |
| SC-02 | HOLDS | batch-2 | dispatch-2 |
| SC-03 | HOLDS | batch-2 | dispatch-2 |
| SC-04 | HOLDS | batch-2 | dispatch-2 |
| SC-05 | HOLDS | rb-05 | dispatch-14 |
| SC-06 | HOLDS | batch-3 | dispatch-3 |
| SC-07 | HOLDS | rb-07 | dispatch-15 |
| SC-08 | HOLDS | batch-3 | dispatch-3 |
| SC-09 | HOLDS | batch-12 | dispatch-12 |
| SC-10 | HOLDS | batch-12 | dispatch-12 |
| SC-11 | HOLDS | batch-12 | dispatch-12 |
| SC-12 | HOLDS | batch-12 | dispatch-12 |
| SC-13 | HOLDS | batch-12 | dispatch-12 |
| SC-14 | HOLDS | rb-14 | dispatch-16 |
| SC-15 | HOLDS | batch-5 | dispatch-5 |
| SC-16 | HOLDS | rb-16 | dispatch-17 |
| SC-17 | HOLDS | batch-5 | dispatch-5 |
| SC-18 | HOLDS | batch-5 | dispatch-5 |
| SC-19 | HOLDS | batch-5 | dispatch-5 |
| SC-20 | HOLDS | batch-6 | dispatch-6 |
| SC-21 | HOLDS | rb-21 | dispatch-18 |
| SC-22 | HOLDS | batch-6 | dispatch-6 |
| SC-23 | HOLDS | batch-6 | dispatch-6 |
| SC-24 | HOLDS | batch-6 | dispatch-6 |
| SC-25 | HOLDS | batch-7 | dispatch-7 |
| SC-26 | HOLDS | rb-26 | dispatch-19 |
| SC-27 | HOLDS | batch-7 | dispatch-7 |
| SC-28 | HOLDS | batch-7 | dispatch-7 |
| SC-29 | HOLDS | rb-29 | dispatch-20 |
| SC-30 | HOLDS | batch-9 | dispatch-9 |
| SC-31 | HOLDS | batch-9 | dispatch-9 |
| SC-32 | HOLDS | rb-32 | dispatch-21 |
| SC-33 | HOLDS | rb-33 | dispatch-22 |
| SC-34 | HOLDS | batch-11 | dispatch-11 |
| SC-35 | HOLDS | rb-35 | dispatch-23 |
| SC-36 | HOLDS | batch-11 | dispatch-11 |

| Batch | Dispatch | Rows | Canary Row | Canary Kind | Weakening | Caught | Inherits From | Voided |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| batch-1 | dispatch-1 | SC-01 | SC-01 | synthesized | binding assertion loosened from next-id Next == 3 to Next >= 2 | true | - | false |
| batch-2 | dispatch-2 | SC-02, SC-03, SC-04, SC-05 | SC-05 | synthesized | blocking assertions (wantVerdict BLOCKED, wantReason) deleted, leaving only the echoed Layer2.Verdict state check | true | - | false |
| batch-3 | dispatch-3 | SC-06, SC-07, SC-08 | SC-07 | synthesized | exact ApplicableRows set equality replaced with a strings.Contains presence check | true | - | false |
| batch-4 | dispatch-4 | SC-09, SC-10, SC-11, SC-12, SC-13, SC-14 | SC-14 | synthesized | env and verdict assertions dropped, leaving a step-count shape check and a non-empty env check | false | - | true |
| batch-5 | dispatch-5 | SC-15, SC-16, SC-17, SC-18, SC-19 | SC-16 | synthesized | expected per-case verdict equality replaced with a non-empty presence check | true | - | false |
| batch-6 | dispatch-6 | SC-20, SC-21, SC-22, SC-23, SC-24 | SC-21 | synthesized | Load error after the interrupted replacement downgraded to t.Logf plus an early return, so every binding assertion is skipped | true | - | false |
| batch-7 | dispatch-7 | SC-25, SC-26, SC-27, SC-28 | SC-26 | synthesized | shared and unknown subtests' class equality and proofEveryRecordRefused replaced with presence checks | true | - | false |
| batch-8 | dispatch-8 | SC-29 | SC-29 | synthesized | out.Trace/out.Live freshness assertions and the resumed pending block deleted; trace repoCommit equality loosened to non-empty | true | - | false |
| batch-9 | dispatch-9 | SC-30, SC-31, SC-32 | SC-32 | synthesized | measured-commit equality assertions replaced with non-empty checks | true | - | false |
| batch-10 | dispatch-10 | SC-33 | SC-33 | synthesized | proofRefused and the counts/uncovered assertions dropped, leaving a decision-exists check | true | - | false |
| batch-11 | dispatch-11 | SC-34, SC-35, SC-36 | SC-35 | synthesized | layer4Applicable length assertion loosened to len(got) > 1 and the missing-refusal assertion guarded behind errors.As | true | - | false |
| batch-12 | dispatch-12 | SC-09, SC-10, SC-11, SC-12, SC-13, SC-14 | SC-14 | synthesized | allowlist equality, SC-1 verdict, step exit-status, canary-on-disk and empty-allowlist assertions all dropped; only a non-empty env and a non-empty runDir presence check remain | true | - | false |
| rb-01 | dispatch-13 | SC-01 | - | - | - | - | batch-1 | false |
| rb-05 | dispatch-14 | SC-05 | - | - | - | - | batch-2 | false |
| rb-07 | dispatch-15 | SC-07 | - | - | - | - | batch-3 | false |
| rb-14 | dispatch-16 | SC-14 | - | - | - | - | batch-12 | false |
| rb-16 | dispatch-17 | SC-16 | - | - | - | - | batch-5 | false |
| rb-21 | dispatch-18 | SC-21 | - | - | - | - | batch-6 | false |
| rb-26 | dispatch-19 | SC-26 | - | - | - | - | batch-7 | false |
| rb-29 | dispatch-20 | SC-29 | - | - | - | - | batch-8 | false |
| rb-32 | dispatch-21 | SC-32 | - | - | - | - | batch-9 | false |
| rb-33 | dispatch-22 | SC-33 | - | - | - | - | batch-10 | false |
| rb-35 | dispatch-23 | SC-35 | - | - | - | - | batch-11 | false |

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

Status: pass · Applicable 22 · Caught 22 · Survived 0 · Unappliable 0 · N/A 0

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
| SC-22 | MUTATION-CAUGHT | - |
| SC-23 | MUTATION-CAUGHT | - |
| SC-24 | MUTATION-CAUGHT | - |
| SC-25 | N/A | - |
| SC-26 | MUTATION-CAUGHT | - |
| SC-27 | MUTATION-CAUGHT | - |
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

- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/live-evidence.json` (sha256 `3913a0fa8fea016c1592b6addedcb777f90a82871f75c2bec9eb77f9c1a6d07c`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-03.json` (sha256 `161bbe8bc38b0df1a65fe1205710a7350ec5021747173e3cce688afb2489ee58`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-11.json` (sha256 `b251324e91b4d767490a69a152346a718e9a89cbb2326ae9cca089cbf6d0ac40`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-12.json` (sha256 `69e9e10bcce1216c5eff1144162f900923c1763aeef151e6199b551bdd3f91bd`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-13.json` (sha256 `dfd5a4e790ae915af332655e7d2ac93819a0fcd46895ef0f180046bb5268ff4f`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-14.json` (sha256 `af77cc782652ae51bf77a9092571cfd4dcb7fb9a2d05d8dcaced387658353def`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-15.json` (sha256 `6b9b1622ef41bf11debee414dcde3c34a22f004d81091c3dc612c236d502893a`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-16.json` (sha256 `c2040e110e6c27be296bb4734ce0ac62533bf1fe186699cded03a249dd87ace7`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-17.json` (sha256 `16817aae22ea824aa33254406b5eae11ee37136a426e3f5fa6e04efc2ab45127`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-18.json` (sha256 `f0a3c30324a88d5b6a0053056f6a389142349795da398116d14db9c4323e1cf5`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-20.json` (sha256 `b09a892e757c842c3aa78f4cca14a6c091ce6e26a261ae1443693cedb84140ea`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-21.json` (sha256 `db5bdb6a9c45218431436b20ac4563c4762dbb3c2782ffb00639f81bfc61176b`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-22.json` (sha256 `498a4a678cf54784484119c80e011db5dbb640013529ae87d197a04882874709`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-23.json` (sha256 `237bf13fbea0f38e60468edccaa06635b20774a0522b031233f38801e841023c`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-24.json` (sha256 `0be3c65e6e5a4298226fd257bfc0948528f386c52b38c177369dec9dc20e4367`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-26.json` (sha256 `94a6fd8a106068b08bd326143678a02993418dc17664221c5f699c132d8a9ba2`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-27.json` (sha256 `378230703e5c796f0b1a3a5a3bba34c62b2dd36b10b63e2f1e60158136d76bdf`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-28.json` (sha256 `3d8e089587c8e625df3083cc61e03426bd46175e16beb71f40db4c520cdd62f1`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-29.json` (sha256 `43db315a246c5f211bf3a0f3abd0faf2e0e3bc7c321143bea677f0ab484af2e6`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-30.json` (sha256 `87c9cc77b1fc5573e2827df392183543bc782143871f0dec1b0f8af2bc8d2dbb`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-31.json` (sha256 `f8aec1e9ce469cb4c08667b06c0b3c3be286297fc631a1d69dc63402f6006a58`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-32.json` (sha256 `c84616f94e2e171b473ce36e396bfb176f1837bb971cf2186bd7a0ad45dd0fe6`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/mutation-SC-35.json` (sha256 `12e05299db91f4371e7b4128d1cc630873b09a644d3182d59be9121444b1a998`)
- `docs/superpowers/coverage/transcripts/run-2026-09-11-0003/trace-evidence.json` (sha256 `789b956168b7e81c126babaaa2fbd80cd1d33e09573eab9a372ff954f09bf29c`)

## What must happen before this branch can finish

Nothing — verdict PASS.
