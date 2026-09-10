# Coverage: verification workflow redesign

Spec: docs/superpowers/specs/2026-09-07-verification-workflow-redesign.md
Baseline: 122401156dbf639afb5d7ce021e3a0ae2dbb0824 · 865 passed, 1 failed, 2 skipped

Status: Completed coverage source prepared for final user approval. The written spec, twelve behavior groups, mixed-evidence amendment, local harness setup and isolated two-commit baseline procedure are approved. Baseline evidence is captured, including one pre-existing failure. Setup verification is not final feature verification; redesign implementation remains unauthorized.

Work record: `test-design-review-handoff-86t`. Target repository is this plugin itself (`test-design`); the installed plugin is only the tool driver. Specs/plans remain local under the repository ignore policy; coverage artifacts are intentionally committable.

## Cost and allocation

- 36 active executable scenarios: 22 high-risk, 14 med; 7 unit, 28 integration, 1 smoke.
- 22 high-risk mutation obligations; later site enumeration may require more than 22 mutation evaluations. No risk argue-down is approved.
- 3 additional mandatory real-agent walkthroughs below. They are not synthetic canaries, not source-text assertions, and not credited by the current verifier.
- `next-id` ran before numbering against the absolute target with `-area ""` and returned `next: 1` (installed verifierVersion `0.6.0`); SC-01 through SC-36 were allocated consecutively from that result.
- Floor rule does not apply: multiple behaviors, persisted evidence and external execution. Candidate variants were merged before IDs were assigned; no issued/approved obligation was silently deleted.
- Existing tests were not surveyed or used to prune/downgrade rows. Test-file names seen in directory listings were not evidence of coverage. Existing tests may be credited later only by execution and independent fault-effectiveness review.

## Approval record and representation limits

1. The user approved mandatory separate W-1 through W-3 evidence on 2026-09-07 (work-record comment 14). Meaningful agent review, consequence-based judgment, scope negotiation and automatic workflow behavior currently live in Markdown instructions, not executable guards. The installed code-mutation/Go-JVM binding schema cannot prove these through instruction-text assertions. A machine PASS alone must not be called full feature acceptance.
2. The user authorized one real local harness as coverage setup. `.test-design/live-profiles/workflow-redesign-local.json` invokes `fixtures/workflow-redesign-live/recipe.py`; `service.go` supplies actual quote calculation over a private Unix socket. This replaces no verifier logic and requires no remote host, cluster, credential or production access. The existing scratch/echo fixture is intentionally unchanged.
3. The user authorized coverage/setup-only commits and temporary clean linked-worktree baseline capture, without staging, resetting or stashing existing source edits. Coverage/setup source commit: `7575077`. The verifier captured the baseline in the isolated clean checkout at that commit and measured the merge-base above. No dirty-tree bypass or user-source checkpoint was used.
4. 2026-09-07 format amendment (approved in the design-review brief grill, Q5): the `## Live decision` section gained the machine-readable form — `decision: required`, a `reason` bullet and journey `J-1` over `SC-10, SC-11, SC-13, SC-17` with the setup-proof expectations. The original prose stays below it. No scenario row, level, verification mutation or risk changed; this document's SHA256 changed and the brief/map references were refreshed.

## Verification

- runner-kind: go
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-09-07-verification-workflow-redesign-baseline.json
- baseline-sha256: 7428df4b74e1f123ec208ff5cbab7b1ff00aed7236f9db2118644c7ff0c2740d
- live-profile: .test-design/live-profiles/workflow-redesign-local.json
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-09-07-verification-workflow-redesign-coverage.md"]

## Baseline failures

Captured through the installed verifier's `baseline` operation with run ID `baseline-redesign-20260907-setup`, using Go `go1.26.3 darwin/arm64`. The operation published evidence successfully; the recorded suite exit status is 1. This failure list is baseline accounting, not an existing-test coverage survey.

- `test-design/cmd/test-design-verifier::TestBaselineJVM_MavenE2EPublishesEvidence` — `baselinejvm_test.go:136` reported `ToolVersions = map[mvn:Apache Maven 3.9.15 (98b2cdbfdb5f1ac8781f537ea9acccaed7922349)], want a java entry`.

Skipped: `TestE2EMutation_GradleSingleSite` and `TestTraceJVM_GradlePipeline`, both in `test-design/cmd/test-design-verifier`. The evidence retains their identities. No green-baseline claim, rerun to hide the failure, or unrelated test/source repair was made.

## Live decision

- decision: required
- reason: SC-10 is the deployed-business obligation; the local disposable Go service over a private Unix socket is the approved configured target.

| Journey | Scenarios | Expected |
|---|---|---|
| J-1 | SC-10, SC-11, SC-13, SC-17 | On the deployed selected revision, after readiness passes as a separate step, a three-item quote totals 375 cents and the artifact/revision identity is recorded; a stale (older) deployment is refused; authenticated shutdown then removes this run's root while a neighboring run root and an unrelated sentinel survive, and both run roots are proved absent at the end. |

Required, not N/A. SC-10 is the deployed-business obligation; the deployed journey also observes relevant selection, handoff, ordering, failure and cleanup contracts in SC-07 through SC-19 without replacing their cheaper direct checks. W-2 separately observes actual agent preparation and permission behavior. Approved setup target: a local disposable Go service over `/tmp/test-design-workflow-{run-id}/service.sock`, inside a mode-0700 run-owned directory. The finite recipe builds exact committed source, stamps the selected revision, fingerprints the artifact, checks readiness separately, requires a three-item quote to total 375 cents, then requests authenticated shutdown and verifies resource removal. It refuses stale deployments and unexpected cleanup contents. No TCP port, remote endpoint or inherited service credential is used. The root profile is trusted project configuration, not a sandbox. Direct harness smoke proof does not claim that the still-unimplemented redesigned verifier successfully orchestrates this journey.

Setup proof: `docs/superpowers/coverage/2026-09-07-verification-workflow-redesign-setup.json` records actual recipe execution against two committed subject revisions. The older service remained healthy but failed the 375-cent business expectation; after selecting the newer commit without deployment, stale identity was refused. Redeployment passed the business assertion with revision/artifact identity. Cleanup refused an unexpected file without deleting it, preserved a neighboring run, and ultimately proved both run roots absent. The actual Go profile loader/validator accepted the template. This is not final coverage-verification evidence; the previously reported verifier live-path defects have not been fixed or re-probed.

## Invariants

- INV-1 Every approved behavior remains an explicit obligation; absent bindings, missing live decisions and unsupported proof never become coverage or N/A by omission.
- INV-2 Deployment and cleanup operate only within the approved target and owned resource scope; a trusted profile is not an arbitrary-script sandbox.
- INV-3 Runtime evidence identifies the measured revision, deployed artifact and observed behavior; reports do not silently advance the target to their own publication commit.
- INV-4 Started, corrupt, failed or incompletely described work is not reusable successful completion; uncertain mutable state is recovered before repetition.
- INV-5 Reuse retains original run/commit/source/artifact identity and requires executable-validated complete matching inputs; unidentified external state and uncertain dependencies widen invalidation or prohibit reuse.
- INV-6 Setup, assertion, cleanup and restoration failures remain visible and prevent an unsupported full PASS even without a scenario binding.
- INV-7 Removing canaries or tag-count restrictions does not remove independent assertion scrutiny, executed-test evidence or approved high-risk mutation obligations.
- INV-8 Human approval fixes scope and permission boundaries; same-scope repair does not require another identical approval, but expansion does.
- INV-9 Historical reports are retained unchanged as readable records; missing legacy manifests are not reconstructed into reusable proof.

## Out of scope

- Changing the two-commit baseline protocol: explicitly excluded by the approved spec; this design still uses it.
- Provider plugins, production deployment, expanded credentials and unrelated shared-resource deletion: outside approved scope and permission.
- Cross-language dependency inference, global cache services and offline critic calibration platforms: explicitly excluded; use conservative file/group/module boundaries.
- New distributed or simultaneous multi-operator coordination guarantees: not specified. Same-run interruption and owned-resource recovery are included; no new lock service is required. A-2: the same-host advisory file lock on the run ledger is owned-state protection between this run’s own callers, not a lock service.
- Clock expiry, DST/timezone behavior, throughput targets or fixed token-savings percentages: no such contract. Evidence identity is content/target based; cost is observed on paired repair scenarios.
- Unrelated Go/JVM runner changes and previously noted Maven/Failsafe questions: not part of this redesign. Existing supported runners must retain behavior; baseline is not a survey of their coverage. A-2 declares the one JVM runner change that did land on this branch (report-glob confinement); it stays outside the redesign’s numbered obligations.
- Implementation of a general-purpose LLM evaluation runner: not needed for three focused real-agent walkthroughs. Prompt string assertions and canned reviewer replies cannot substitute for semantic proof.

## Retired

None. Unnumbered sweep candidates were consolidated before allocating this draft’s rows. Subsequent approved amendments append fresh IDs and explicitly retire superseded rows; they never renumber existing IDs.

## Pruning decisions

- Missing/contradictory live-decision variants share the explicit-decision defaulting fault (SC-06); schema field spelling remains an implementation-planning decision.
- Zero-bound assertion failures across phases share lost global reduction (SC-15). Last-write-wins assertion loss (SC-16) and ignored earlier cleanup exits (SC-17) remain distinct faults.
- Deadline/cancellation/capture/output-limit failures share the dangerous conversion of incomplete execution into an ordinary expected exit (SC-18); no row per limit value.
- Manifest dimensions share the complete-input rule (SC-25); invalidation-boundary selection (SC-26), retained-byte integrity (SC-27) and re-stamped provenance (SC-28) remain distinct faults.
- Publication failures before any output or after a prefix share publication-only recovery (SC-30); destructive recovery (SC-31) and recursively advancing the target (SC-32) remain distinct.
- Required unit-labelled journey selection (SC-07) is not deployed proof. SC-10 remains smoke because it detects omitted deployment against a healthy older instance, not merely because the old layer-3 gate needs a label.
- No row was manufactured for DST, arbitrary journey quotas, a provider framework, new multi-client locking, or a general LLM evaluation platform.

## Mandatory real-agent walkthroughs — approved amendment

The user approved these separate obligations and final acceptance gate on 2026-09-07. They preserve requirements the installed row schema cannot honestly represent. They are not retired/dropped requirements, do not imply mock/test-double proof, and must be inspected against actual representative inputs and outputs; one example does not establish universal LLM reliability.

### W-1: Design quality and reviewed scope

Behavior groups: 1, 2.

Run an actual independent reviewer on a representative draft containing one redundant row, one invented requirement, one unobservable expected result and one omitted real behavior. Require findings tied to each seeded defect and requirement, then inspect the repaired table and focused re-review. Include a >40-row version with superficial duplicates and genuinely distinct cases; preserve distinct cases, retire merged IDs, show actual scenario/high-risk/journey counts. Contrast an ordinary failure-path case with security/integrity/shared-state cases; record consequence-based risks and reject silent post-approval downgrades. Inspect existing code/tests only during the future redesigned workflow, not this installed-skill sweep.

Evidence: actual instruction/source revision, input artifacts, authentic agent transcript/findings, observed actions and final resolution. No result is claimed at design time.

### W-2: Agent-driven live preparation and scoped permission

Behavior groups: 4, 5, 6.

Use a real agent running the changed plugin against a disposable service project whose SC rows are unit-labelled, with an explicitly approved isolated nonproduction target and a missing project recipe. Observe recipe preparation, revision-bound deployment, business assertions and owned cleanup without a separate smoke prompt. Repeat with requested target/resource expansion: the agent must request new authorization rather than treating trusted recipes as a sandbox. Health-only/old deployment is not proof; exercise a real changed response or state transition.

Evidence: actual instruction/source revision, input artifacts, authentic agent transcript/findings, observed actions and final resolution. No result is claimed at design time.

### W-3: Lean review and repair orchestration

Behavior groups: 7, 8, 9, 10, 12.

Run the actual changed skill with real independent critics on coherent groups. Retain authentic source bundles, findings, dispatch count, input bytes and run artifacts. No synthetic weakened copies, canary redispatch/inheritance or parent-assembled finalize envelope. Repair one isolated test group: unrelated completed matching reviews are not redispatched, but current suite/live observations are fresh. Same-scope repair has no repeated approval; expanding verification or permission scope does. Required high-risk mutation checks and real weak-test findings still block honestly.

Evidence: actual instruction/source revision, input artifacts, authentic agent transcript/findings, observed actions and final resolution. No result is claimed at design time.

## Scenarios and twelve-axis sweep

Risks below follow the currently invoked installed rule: axes 3–6 are high, as are mutations naming data loss/corruption/exposure; other noncosmetic cases are med. SC-28 is high because its mutation corrupts historical evidence identity. W-1 observes the redesigned consequence-based policy without applying it prematurely to this table.

### Behavior 1: Independent scenario review

Sweep: 1: duplicate rows and >40-row review; 2: vague or contradictory claims; 7: author → reviewer → correction → approval, stable retirement; 10: requirement-grounded findings; 11: actual human approval handoff, not document-string assertions.

Ruled out: 3/4: reviewer-provider timeout/retry policy is not introduced (spec Scope); 5: this review reads a candidate table and returns findings, with no shared table/database/cache/queue/lock written by the independent reviewer (spec Review the whole candidate table); 6: deployment permission is covered under behavior 5, not this read-only reviewer; 8: no date/expiry semantics; 9: >40 scope/cost is axis 1, not a new memory/rate limit; 12: retirement is axis 7, not a new catalog storage migration.

Axes: SC-01 axes 1,7,10,12.

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|---|---|---|---|---|---|
| SC-01 | Preserve stable scenario identifiers through retirement | Approved retirement removes the row from active obligations while its identifier stays reserved; unaffected IDs stay fixed and next-id never reissues the retired highest number. | integration | Ignore inactive owners when computing Registry.NextID. | med |

harness SC-01: Actual old/amended coverage documents and registry operations; retire the highest SC, inspect new allocation and active report accounting. Semantic amendment approval is separately observed in W-1.

Semantic acceptance: W-1 is mandatory; the registry row alone does not prove independent meaningful review or consequence-based judgment.

### Behavior 2: Meaningful risk and visible obligations

Sweep: 1: exact surviving obligations/counts and >40 trigger; 6: human alone approves lowering obligations; 7: approved obligations cannot silently change; 10: consequence-based risk and honest total cost.

Ruled out: 2/3/4: no numeric parser, external risk service or automatic retry is introduced; 5: the derivation/review has no shared risk database/cache/queue/lock (spec Make size and cost visible); 8: no time-based risk/expiry; 9: no promised token cap or throughput limit; 11: rendered layout is not a product contract; 12: semantic amendments use existing approval, not a new risk-store migration.

Semantic acceptance: W-1 is mandatory; the registry row alone does not prove independent meaningful review or consequence-based judgment.

### Behavior 3: Evidence-backed many-to-many test credit

Sweep: 1: three IDs per test and multiple contributors; 2: absent execution records; 3: failed contributors remain failures; 7: skipped/unexecuted is not successful; 10: per-claim identity and effectiveness.

Ruled out: 4: no at-least-once credit/publish protocol is introduced by trace.Join; 5: join reads input rows/sites/results into invocation-local maps, no shared database, cache, queue or lock is written (internal/trace/join.go:58-60); 6: test binding has no actor/tenant permission contract; 8: normalized identity/status, not clock-based matching; 9: >2 is a semantic cardinality barrier, not a resource cap; 11: no rendered component; 12: legacy evidence eligibility belongs to behavior 11.

Axes: SC-02 axes 1,10; SC-03 axes 1,3,10; SC-04 axes 2,7,10; SC-05 axes 10.

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|---|---|---|---|---|---|
| SC-02 | Credit one executed test for three independently asserted scenarios | All three SC claims retain bindings and proceed to per-claim effectiveness review; no cardinality-only overclaim finding. | integration | Restore the len(activeIDs) > 2 rejection and its continue in trace.Join. | med |
| SC-03 | Combine multiple executed tests supporting one scenario | All contributing test identities are retained for review; multiple sites alone do not create ambiguity or discard a failing contributor. | integration | Restore the default multiple-site ambiguity branch in trace.Join. | high |
| SC-04 | Withhold execution credit for skipped or unexecuted contributors | A cited test with no executed result or only a skip remains a gap, including when another contributor passes. | integration | Treat a missing or skipped contributor as passed during multi-site result collapse. | med |
| SC-05 | Withhold credit for assertions that do not establish the claim | Executed tags alone cannot yield covered/PASS when the independent critic records WEAK, VACUOUS, or MISSING for the claimed fault. | unit | Remove the weak/vacuous/missing blocking branch in report.deriveLayer2. | med |

harness SC-02: Temporary Go module with one actual executed test asserting three distinct outcomes; run trace and inspect each claim binding, not tag-text presence.

harness SC-03: Temporary module with two distinct tests for one claim; run once both passing and once one failing, retaining both identities.

harness SC-04: Real go test JSON from a fixture with a skipped tagged contributor; separate run selection excludes another contributor. Inspect trace gaps and downstream non-PASS.

### Behavior 4: Explicit live applicability

Sweep: 1/2/7/10/12: explicit coherent decisions, active journey references, legacy missing decisions and independent trace/live outcomes.

Ruled out: 3: unavailable runtime access is a failure under behaviors 5–6, not a new applicability-service failure; 4: decision selection has no side effect to retry; 5: applicableLiveRows reads source into a local slice and writes no shared database/cache/queue/lock (cmd/test-design-verifier/executelive.go:305-320); 6: target authorization belongs to behavior 5; 8: no expiry/date selection; 9: no journey quota specified; 11: JSON decision meaning is axis 10, no rendered-component contract.

Axes: SC-06 axes 1,2,7,10,12; SC-07 axes 1,7,10; SC-08 axes 7,10.

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|---|---|---|---|---|---|
| SC-06 | Require an explicit coherent live decision | Absent decision, reasonless N/A, N/A contradicting active smoke/e2e rows, or required journeys lacking target/expectations is incomplete; justified N/A and complete required decisions are accepted. | unit | Default missing or incomplete live-decision metadata to N/A in source validation. | med |
| SC-07 | Select required journeys independently of scenario level | Required journeys select their active unit/component/integration SCs without relabelling; retired or unknown SC references are not credited. | unit | Keep smoke/e2e row-level filtering as the required-journey applicability gate. | med |
| SC-08 | Execute valid live journeys despite trace gaps | The journey actually executes and retains its result while the original missing/ambiguous test binding stays a trace gap and prevents unsupported full PASS. | integration | Reinstate the gap-row-bound rejection in live plan validation. | med |

harness SC-08: Temporary Git subject with an intentionally unbound journey SC and a real observable business command; drive trace, prepare, execute and finalize with their real output files.

### Behavior 5: Authorized live preparation and execution

Sweep: 2/7/10: prepared/resolved profile handoff and revision-bound business evidence; 3/5: start→readiness→business→cleanup order and phase failure; 5/6: overlapping run resources and scoped cleanup/credentials; 11: automatic preparation and permission boundaries are exercised in W-2.

Ruled out: 1: zero journeys/unknown rows covered by behavior 4, not a new deployment count limit; 4: uncertain repeat is behavior 7 and fresh revised targets are behavior 9, not blind deployment retries; 8/9: execution deadlines/output limits belong to behavior 6; 12: no cluster rollout/provider schema introduced (spec Scope and Prepare the target).

Axes: SC-09 axes 2,7,10; SC-10 axes 7,10; SC-11 axes 3,5,7; SC-12 axes 3,7; SC-13 axes 5,6; SC-14 axes 6.

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|---|---|---|---|---|---|
| SC-09 | Execute a prepared run-owned profile in resolved form | Real prepare output containing a resolved run-owned resource is accepted by execute without removing resource-isolation declarations; cleanup removes that resource. | integration | Apply the template-only literal {run-id} validation to the resolved descriptor. | med |
| SC-10 | Deploy the measured revision and exercise changed business behavior | Normal live verification builds/deploys the selected revision, records its artifact/target, separates readiness from the changed business assertion and proves owned cleanup; an older healthy deployment cannot satisfy it. | smoke | Omit the deployment/start loop while an older healthy instance remains reachable. | med |
| SC-11 | Wait for successful startup and readiness before business assertions | Failed startup/readiness retains non-success and never invokes business assertions; successful health without business evidence is not a journey pass. | integration | Run the business walkthrough even when startFailed or healthFailed is true. | high |
| SC-12 | Attempt owned cleanup after a failed live phase | A run-owned resource created before startup/readiness/business failure is cleaned up, and its absence is independently observed. | integration | Return on start/health failure before entering the unconditional cleanup section. | high |
| SC-13 | Keep cleanup confined to the current run | Cleaning one run removes its owned resources while another active run and an unrelated sentinel remain intact. | integration | In prepare-live’s descriptor substitution, resolve the {run-id} token of the profile’s cleanup command group to a shared prefix of the run ID (the ID with its trailing digits stripped) instead of the run’s exact identity, so the verifier-issued cleanup addresses a resource selector wider than this run’s own. | high |
| SC-14 | Limit inherited credentials to the declared environment | A live command receives only approved inherited variables; an unapproved credential is absent, and no full-environment fallback is used. | unit | Return the complete parent environment instead of the SanitizedEnv allowlist projection. | high |

harness SC-09: Real prepare→execute calls with a local run-owned resource and removal predicate; consume prepared state unchanged, never manufacture a resolved descriptor.

harness SC-10: Approved local disposable Go-service subject with distinct old/new business responses; actual revision build, artifact fingerprint, separately observed readiness, real request/state assertion, and resource-removal proof. Real-agent prompting behavior is additionally required by walkthrough W-2.

harness SC-11: Finite real recipe records phase invocations, controls startup/readiness failure and a readiness transition; assert no business request before readiness, including under cancellation.

harness SC-12: Create a real temporary run-owned resource then fail a live phase; inspect actual cleanup invocation and resource absence, not just cleanupOK.

harness SC-13: Approved local recipe, two run-specific resource sets with overlapping lifetimes and an unrelated sentinel; clean only one. This probes trusted project code, not a claimed plugin sandbox.

Semantic acceptance: W-2 is mandatory; executor/recipe scenarios do not prove that an agent will prepare the recipe or request approval at the right boundary.

### Behavior 6: Truthful live outcomes

Sweep: 1/2/10: zero bindings, multiple step results and real evidence schema; 3: assertion/cleanup/command failures; 5: cancellation and concurrently captured stdout/stderr; 8/9: execution versus cleanup deadlines and bounded output; 7: normal completion, failed assertion and incomplete execution remain distinct.

Ruled out: 4: repeated-attempt recovery belongs to behavior 7; 6: inherited credential scope is behavior 5 and existing redaction remains unchanged, no auth-provider integration introduced; 11: JSON/report outcome meaning is axis 10, no layout/focus contract; 12: historical admission is behavior 11, fresh producer-consumer agreement is included here.

Axes: SC-15 axes 1,3,10; SC-16 axes 1,3,10; SC-17 axes 3,10; SC-18 axes 3,5,7,8,9,10; SC-19 axes 2,10.

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|---|---|---|---|---|---|
| SC-15 | Account for assertion failures without scenario bindings | A failed setup/business/plan-cleanup assertion with zero SC bindings remains an overall live failure even when bound business assertions pass. | integration | Retain assertion failure only inside the SCBindings verdict loop. | high |
| SC-16 | Preserve an earlier failed business assertion | A later passing step for the same scenario cannot overwrite its earlier failure; both step orders yield failure. | unit | Replace worst-verdict merge with unconditional last-write-wins assignment. | high |
| SC-17 | Retain every cleanup failure and require removal proof | An earlier cleanup command failure or failed removal predicate prevents PASS even when the final cleanup command exits successfully. | integration | Compute cleanup success solely from the last command and ignore earlier nonzero cleanup results. | high |
| SC-18 | Treat interrupted or incomplete command output as unavailable proof | Timeout, cancellation, output-limit and capture errors cannot satisfy an expected nonzero-exit predicate; cleanup still gets its independent execution allowance. | integration | Convert subprocess/capture errors to ordinary completed exit results before predicate evaluation. | high |
| SC-19 | Accept actual live evidence through finalization without identity repair | Real prepare→execute evidence is accepted downstream with the measured run, commit and source identity; no fabricated/re-stamped envelope is necessary. | integration | Omit repoCommit when serializing execute-live evidence. | med |

harness SC-15: Actual execute→finalize pipeline with one passing bound step and one failed unbound predicate; place it in each supported assertion phase within this fault family. Inspect actual final reasons and reduced step result.

harness SC-17: Real multi-command cleanup with an early failed operation followed by a successful absence checker; separately leave observable residue so removal-proof failure is visible. Consume producer output in finalization.

harness SC-18: Finite real helper processes: ordinary expected nonzero control, deterministic cancellation, deadline, and concurrent stdout/stderr at/over the shared cap with a success-shaped prefix; observe non-PASS and cleanup execution.

harness SC-19: Actual clean temporary Git subject and real prepare, execute, finalize calls; feed the emitted evidence file unchanged to the consumer and verify the reported measured revision.

### Behavior 7: Safe interruption recovery

Sweep: 1/4/5/7: duplicate/out-of-order completed results, started versus completed, restart and cleanup/restoration before repetition; 3/9: checkpoint-write failure/interruption; 6: recovery respects owned resources; 10: persisted failed outcomes remain visible.

Ruled out: 2: missing/corrupt artifact or manifest acceptance is covered by behavior 8, rather than another decoder family; 8: no clock-based expiry or power-loss durability promise (spec Persist completion); 11: these are persisted CLI-state transitions, no rendered component; 12: old contract admission belongs to behavior 11. Concurrency uses actual checkpoint files/workspaces/resources, not an invented database, distributed lock or multi-client service.

Axes: SC-20 axes 1,4,5,7; SC-21 axes 3,5,7,9; SC-22 axes 3,4,7,10; SC-23 axes 3,4,5,6,7; SC-24 axes 3,4,5,7.

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|---|---|---|---|---|---|
| SC-20 | Resume only validated completed obligations | After restart unchanged completed groups/sites/phases are not redispatched; started records, duplicate results and a completed subset cannot satisfy other obligations or the whole phase. | integration | Accept record existence or any completion instead of requiring the full validated completion set. | high |
| SC-21 | Preserve earlier completion across interrupted checkpoint replacement | An interrupted completion update leaves the prior or new complete checkpoint readable without losing earlier validated work or exposing a partial completion. | integration | Replace atomic checkpoint temp-file/rename with a direct truncating write. | high |
| SC-22 | Preserve failed outcomes on resume | A completed failure stays visible and non-successful; unchanged failed inputs without identified environmental recovery do not trigger blind repetition or reusable success. | integration | Restore every completed operation as success instead of preserving its recorded outcome. | high |
| SC-23 | Recover uncertain live state before repeating the attempt | An interrupted attempt performs and observes owned cleanup before fresh startup; failed recovery remains incomplete/non-PASS rather than accepted completion. | integration | Skip recovery cleanup when restarting a started but uncompleted live operation. | high |
| SC-24 | Restore or recreate uncertain mutation workspaces before measuring again | A repeated evaluation starts from the intended clean revision; tracked/untracked residue, missing worktree or failed restoration never counts as completed mutation proof. | integration | Resume evaluation directly from loaded prepared state without restoring or safely recreating its workspace. | high |

harness SC-20: Real persisted run, one complete and one merely started group, controlled out-of-order/duplicate result delivery, then process restart; measure actual work selected. No simultaneous multi-client service is assumed.

harness SC-21: Real checkpoint files with one committed result; interrupt/fail replacement after writing begins and restart the reader. Assert retained prior completion and non-completion of partial new work; no power-loss/fsync guarantee is claimed.

harness SC-22: Persist actual failed review/mutation or evidenced refusal, restart and compare canonical outcome plus operation counts; changed input or identified environmental recovery permits affected repetition.

harness SC-23: Real verifier/local recipe interrupted after resource creation; restart, inspect cleanup-before-new-start ordering and preserve unrelated sentinel resources.

harness SC-24: Real disposable Git worktree interrupted after a production edit or target run, including an untracked residue; restart and observe intended clean baseline before repeated measurement. Original repository is never reset.

### Behavior 8: Selective evidence reuse

Sweep: 1/2/7/10: complete-input matching, support reads, missing or unidentified inputs; 4: isolate a repaired group versus widen shared/unknown impact; 3/5: retained-file corruption/replacement at consumption/publication; 12: original provenance stays immutable across evidence versions.

Ruled out: 6: no new actor/tenant or cryptographic signature system; path/digest integrity is not a claim against a hostile owner controlling all local evidence; 8: matching is not based on timestamp recency (spec Reuse through explicit input manifests); 9: no global cache/quota/eviction service; 11: canonical report meaning is axis 10, not a layout contract.

Axes: SC-25 axes 1,2,7,10; SC-26 axes 4,7,10; SC-27 axes 2,3,5,10; SC-28 axes 7,10,12.

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|---|---|---|---|---|---|
| SC-25 | Match complete review and mutation inputs before reuse | Unchanged complete inputs permit reuse; changed or unidentifiable claim semantics, support context, fixtures, production/build/dependencies/runner, toolchain/environment or review policy refuse affected reuse despite identical test-body bytes. | integration | Use only TestSourceSHA256 equality instead of complete manifest equality and completeness checks. | med |
| SC-26 | Invalidate only provably isolated work and widen uncertain impact | An isolated test-file repair rechecks its group and dependent mutations while unrelated groups remain reusable; shared or uncertain helper/production/build/dependency impact widens invalidation to the affected module. | integration | Restrict invalidation to the edited file group even when ownership is shared or unknown. | high |
| SC-27 | Reject altered or missing retained evidence bytes | Missing, truncated, replaced or digest-mismatched evidence cannot earn reuse or publication success; the accepted and published bytes match the recorded identity while other valid completed work remains reusable. | integration | Trust artifact path/envelope without verifying the digest of the bytes actually consumed and retained. | high |
| SC-28 | Preserve original provenance in consuming reuse reports | Original evidence bytes and run/commit/source/outcome/artifact identity stay unchanged and addressable; the new report carries a separate reference and verified matching basis. | integration | Corrupt historical identity by filling the reused artifact envelope with the consuming run and HEAD. | high |

harness SC-25: Real two-run bundles with stable test bodies; change each named input category independently, include a supporting read outside the original bundle and unidentifiable external state. Observe actual fresh/reused work, not only hash comparison.

harness SC-26: Two independent review groups with dependent mutation evidence; compare isolated test edit with shared-helper/build edit and unknown ownership. Inspect actual dispatch identities/counts and retain complete required review context.

harness SC-27: Real artifacts altered/deleted after completion and controlled replacement between consumption and publication; inspect refusal/invalidation and no dishonest PASS, including unaffected completed-work preservation.

harness SC-28: Actual old/new run evidence; verify unchanged original bytes plus dereferenceable original provenance and matching basis in the consuming canonical report.

### Behavior 9: Fresh observations after repairs

Sweep: 3/4/7/10: a repaired/revised target gets new observations and a previous pass cannot hide its failure.

Ruled out: 1/2: arbitrary numbers or schema shapes of revisions add no new fault beyond target identity; 5: this target-selection path has no shared database/cache/queue/lock, and tests select A then B sequentially (spec Persist completion and S:152); overlapping resources are behavior 5; 6: target scope expansion is W-2, not implicit approval here; 8: revision identity is not a wall-clock/expiry comparison; 9: no target-rate/throughput contract; 11: no rendered component; 12: old evidence admission is behavior 11.

Axes: SC-29 axes 3,4,7,10.

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|---|---|---|---|---|---|
| SC-29 | Execute fresh suite and live observations for a new target revision | A selected new revision runs the declared suite/trace and required live journey even if review/mutation inputs match; an older pass cannot mask failure at the new target. | integration | Allow generic reuse to return prior trace/suite or live results across target revisions. | high |

harness SC-29: Two real subject commits and revision-distinguishing suite/local business outcomes; observe fresh invocations and commit/artifact identity through finalization, with prior passing observations unable to hide new failure.

### Behavior 10: Independent publication recovery

Sweep: 1/3/4/7/9/10: zero/prefix/all publication, destination write failure, retry without repeating verification, explicit target unchanged by report commit; 5/6: user-modified artifacts and unrelated files survive ownership-scoped recovery.

Ruled out: 2: malformed retained evidence is behavior 8 rather than a new publication parser; 8: no timestamp cutoff or publication lease; 11: report semantic output is axis 10, exact Markdown wording/layout is not asserted; 12: static historical records and old evidence admission are behavior 11.

Axes: SC-30 axes 1,3,4,7,9,10; SC-31 axes 3,4,5,6; SC-32 axes 4,7,10.

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|---|---|---|---|---|---|
| SC-30 | Retry publication without repeating valid verification | After destination-only repair, publication resumes from retained validated evidence without another suite, live, critic or mutation run; a partially written report set is not completed publication. | integration | In resume’s pending accounting, when the publish phase is started-but-incomplete or was refused, re-owe every completed review group, live attempt and mutation row as pending instead of owing only the publish phase; the own-report exemption in staleForRun is SC-32’s surface and is not a site of this mutation. | high |
| SC-31 | Preserve user files during publication recovery | Recovery affects only this run’s proven generated artifacts; unrelated tracked/untracked files and edited or uncertain-provenance outputs are preserved or cause explicit refusal. | integration | Replace exact generated-artifact recovery with recursive directory deletion or repository reset, deleting user data. | high |
| SC-32 | Keep publication commits separate from verification target selection | Publishing/committing report files keeps the report’s measured revision fixed and causes no recursive verification; explicit selection of a real new target still requires fresh observations. | integration | Choose current publication HEAD as the next measurement target instead of retaining the selected target. | high |

harness SC-30: Actual retained run; fail before publication and after a written prefix, repair destination alone, resume through executable and compare invocation counts plus complete report/transcript outputs.

harness SC-31: Partial publication beside tracked/untracked sentinels; modify a generated output before retry. Assert byte preservation and ownership refusal rather than accepting a cleanup flag.

harness SC-32: Complete measurements at A, commit only generated report files, continue publication/acknowledgment; observe no new verification and target A. Then explicitly select B and observe fresh work.

### Behavior 11: Explicit compatibility handling

Sweep: 2/7/10/12: old missing-manifest evidence remains readable but is not reusable; existing sources require the explicit live decision covered under behavior 4.

Ruled out: 1: multiple old versions introduce no count limit; stable retired IDs are behavior 1; 3/4: missing/corrupt evidence and rerun selection are behaviors 7–8, not a new migration retry engine; 5: legacy admission reads a local record and has no shared schema/database/cache/queue/lock to migrate (spec S:154); 6: no new permission system; 8: age alone does not establish proof; 9: no bulk migration/volume promise; 11: static historical JSON/Markdown remains readable, no new historical rendering API.

Axes: SC-33 axes 2,7,10,12.

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|---|---|---|---|---|---|
| SC-33 | Treat legacy proof as historical rather than reusable | Old records lacking required manifests are explicitly ineligible and require fresh affected verification; stored historical JSON/Markdown remains unchanged and readable without inferred proof. | integration | Accept missing legacy manifests by constructing them from current inputs during reuse admission. | med |

harness SC-33: Retain genuine old-shape evidence/report artifacts, attempt new-run reuse, inspect clear ineligibility, fresh affected selection and preserved original bytes; no new historical-reader API required.

### Behavior 12: Bounded verification orchestration

Sweep: 1: missing obligations and truthful group counts; 2: missing review evidence; 3: failure/incomplete work stays blocking; 4: repair dispatches do not repeat unaffected work; 7: review completion and immutable context; 10: no canary requirement, truthful summary and retained artifacts; 11: user-facing run/repair approval boundaries exercised by real agent.

Ruled out: 5: persistence/recovery of shared run records belongs to behavior 7; report assembly reads them and acquires no separate shared cache/database/queue/lock (spec Keep the parent out of bookkeeping); 6: expanded-target authority belongs to behavior 5; 8: no expiry/clock cutoff; 9: bounded authoritative inputs and observed input bytes are a scope/dispatch contract, not a newly promised byte limit; 12: versioned legacy evidence belongs to behavior 11.

Axes: SC-34 axes 10; SC-35 axes 3,10; SC-36 axes 10.

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|---|---|---|---|---|---|
| SC-34 | Accept complete real-source review without canary bookkeeping | A complete independently produced all-HOLDS review and valid remaining evidence can finalize without canary, inheritance, voided-batch, or unavailable-canary fields. | unit | Reinstate validateCanaryBookkeeping in ordinary layer-2 validation. | med |
| SC-35 | Require every approved high-risk mutation obligation after canary removal | Removing runtime canaries does not allow a missing or survived high-risk mutation to produce PASS; complete caught evidence permits that obligation. | unit | Remove r.Risk == "high" from report.layer4Applicable, leaving only explicitly supplied mutation entries. | high |
| SC-36 | Report actual fresh reused failed and incomplete work | Published summary agrees with retained operations and evidence: it identifies the failed/incomplete obligations, original reused evidence and matching reason, and actual fresh work without parent-supplied layer envelopes. | integration | Classify a reuse-reference operation as fresh during executable report assembly. | med |

harness SC-36: Run a mixed-outcome disposable verification, compare real invocation logs and artifact references to the canonical summary; rerun an isolated repair and measure actual dispatch count and repeated input bytes.

Semantic acceptance: W-3 is mandatory; accepting canary-free evidence does not prove that normal agent dispatches avoid canary recursion or use sufficient bounded context.

## Amendment A-2 (2026-09-10): repairs after verification run-2026-09-10-0001

Verification `run-2026-09-10-0001` (report `docs/superpowers/coverage/2026-09-07-verification-workflow-redesign-verification.md`, commit 31db4c6) returned BLOCKED: the source was stale against eleven production hunks, two verification mutations enumerated outside their rows’ fault, nine mutation sites survived, and one `covers:` comment sat inside a function body. This amendment records the source-side repairs. It adds no scenario row, retires none, renumbers nothing and changes no Cost and allocation count. Test-side repairs (strengthened bound tests for SC-18, SC-20, SC-22, SC-27, SC-31; the reworded comment in `internal/tags/extract_test.go`) are code changes the next trace measures, not source text.

### Rephrased verification mutations

Only the probe wording changes; each row’s behavior, expected result, level and risk stand.

- SC-13 — was: “Broaden the approved project recipe cleanup selector from the exact run resource to a shared prefix.” That wording enumerated to `fixtures/workflow-redesign-live/recipe.py:154`, a trusted Python fixture outside the Go mutation surface, and the row was MUTATION-UNAPPLIABLE. The row now names the verifier’s own surface: the substitution of the run’s identity into the profile’s cleanup command group (`substituteRunID`, `cmd/test-design-verifier/preparelive.go`). A cleanup issued for a wider selector leaves run A’s root in place, which `TestLiveTruth_CleanupConfinedToRun` observes directly.
- SC-30 — was: “Route publication failure through all-layer invalidation instead of publication-only recovery.” Site enumeration reached the own-report exemption in `staleForRun` (`cmd/test-design-verifier/resume.go`), which is SC-32’s fault (a publication commit advancing the measured revision), so the row was MUTATION-UNAPPLIABLE. The row now names resume’s pending accounting alone and excludes `staleForRun` explicitly.

### Production hunks declared as in-scope hardening

The production diff between baseline 1224011 and the measured head carries these behaviors the approved text did not name. Each is assigned to the behavior group it hardens, with the suite tests that exercise it. They are declared behaviors, not numbered obligations: their tests run in every trace but earn no layer-2 or layer-4 credit, and none introduces data loss, corruption or exposure — each narrows what the verifier accepts or bounds what it runs — so none carries a mutation obligation under the risk rule in force. A later finding that one of them is wrong or missing is amended by appending a fresh SC row, never by editing this list.

| Hunk | Behavior | Group | Suite evidence |
|---|---|---|---|
| `internal/jvmrun/jvmrun.go:29-71,110-214` | JVM report-glob confinement: every glob segment is validated (`report-glob-escapes-root` for an empty, absolute, traversing or invalid pattern); report collection walks under `os.Root`, refuses symlinked or non-regular report paths, fails on an unreadable directory, removes only concrete pre-existing report files (never a directory) and applies no mtime cutoff; `ReportDirs` is gone. | Out of scope (runner), declared here | `TestRun_GlobEscapeRefused` (`internal/jvmrun`), `TestLoadVerifiedSource_ReportGlobEscapingRootRefused` |
| `cmd/test-design-verifier/finalize.go:978-995` | `artifactStem` derives the publication stem from any non-empty `.md` basename, taking the `-coverage.md` stem when present; only an empty stem or a non-Markdown name is refused `source-name-unconventional`. | 10 | `TestArtifactStem_AcceptsAnyMarkdownBasename` (added with this amendment) |
| `internal/live/transcript.go:29-51` | The redaction barrier scans the plan step’s expectation text together with the decisive output and command; a step whose expectation carries a withheld parent-environment value is published as `[withheld by redaction barrier]`, never with the value. | 5 | `TestLiveRunWithholdsUnboundExpectationBehindRedactionBarrier` |
| `internal/subprocess/run.go:1-99` | The verifier’s own subprocess policy: 30-minute suite timeout, 30-second git utility timeout, process-group kill on timeout, cancellation or SIGINT/SIGTERM, returned as infrastructure errors distinct from an ordinary exit result. | 6 | `TestRunTimeoutTerminatesDescendants` (`internal/subprocess`), `TestExecuteStepTimeoutPreservesRawAndCannotPass` (`internal/live`) |
| `internal/live/plan.go:86-94` | Live plan validation refuses a row-bound step with no predicates (`missing-row-predicates`). | 4 | `TestValidateRefusesBoundStepWithoutPredicates` (`internal/live`), `TestExecuteLive_MissingRowPredicatesRefused` |
| `cmd/test-design-verifier/preflight.go:64-72` | Preflight resolves a relative `--source` against `--repo`; a source outside the repository is still refused `source-outside-repo`. | 12 | `TestPreflightHandler_ValidSourceReportsFullOutput`, `TestPreflightHandler_SourceOutsideRepoNamesBothPaths` |
| `internal/runstate/lock.go:1-45` | Ledger reads take a shared and write transitions an exclusive advisory lock on `ledger.lock`, serialising same-host callers of one run. | 7 | `TestRecordReview_GroupLifecycle`, `TestRecordReview_ConcurrentStartsKeepARowInOneGroup` |
| `cmd/test-design-verifier/publication.go:359-430` | Finalize fingerprints the ledger it assembled the report from and refuses `ledger-changed` when the ledger differs at the moment the publication opens. | 10 | `TestStartPublication_RefusesALedgerThatChangedSinceFinalizeReadIt` |
| `internal/runstate/runstate.go:110-123` | Run ledger location: `TEST_DESIGN_STATE_DIR` overrides; the default is under the user cache directory. | 7 | `TestDefaultStateDir` (`internal/runstate`) |
| `cmd/test-design-verifier/recordreview.go:104-151` | record-review refuses `invalid-group-id` for a group id that is not a plain name of letters, digits, dot, dash or underscore. | 7 | `TestRecordReview_RefusesAMalformedRequestBeforeTheLedger` |
| `cmd/test-design-verifier/recordreview.go:443-470` | A scenario row belongs to at most one current review group: record-review and reuse refuse `row-already-grouped` unless the earlier group is superseded. | 8 | `TestRecordReview_GroupLifecycle`, `TestRecordReview_ConcurrentStartsKeepARowInOneGroup` |
| `cmd/test-design-verifier/reuse.go` (`reuseBeforeAdopt`) | Test seam added with this amendment: a package-level hook that is nil in production and, when a test sets it, runs inside the reuse transaction after every prior record is verified and before the first is adopted. It changes no production behavior; it exists so `TestReuse_RejectsAlteredRetainedBytes` can alter a retained artifact at the moment adopt’s re-digest must refuse it (SC-27). | 8 | `TestReuse_RejectsAlteredRetainedBytes` |
| `cmd/test-design-verifier/resume.go` (`everyGroupSettled`) | Dead clause removed with this amendment: `reportedStatus` already reports a completed record that carries a reason as `invalid`, so a group with status `completed` and a non-empty reason never existed and the extra `Reason != ""` check was unreachable. Run-2026-09-10-0001 enumerated it as SC-20 site-1, an equivalent mutant; removing it changes no observable behavior. | 7 | `TestResume_SelectsOnlyValidatedCompletions` |

## Amendment A-3 (2026-09-11): repairs after verification run-2026-09-10-0002

Verification `run-2026-09-10-0002` (report `docs/superpowers/coverage/2026-09-07-verification-workflow-redesign-verification.md`, commit dee1832) returned BLOCKED for seven reasons: the source was stale against one production hunk, and six rows each carried a mutation site their bound test did not catch (SC-20 site-2, SC-21 site-2, SC-22 sites 1 and 3, SC-26 site-2, SC-27 site-4, SC-30 site-3). This amendment records the source-side repairs. It adds no scenario row, retires none, renumbers nothing and changes no Cost and allocation count. The test-side repairs — strengthened bound tests for SC-20, SC-21, SC-22, SC-26 and SC-30 — are code changes the next trace measures, not source text.

### Rephrased verification mutations

Only the probe wording changes; each row's behavior, expected result, level and risk stand.

- SC-27 — the row's mutation stands, with one site excluded by name: the pre-load `report.VerifyEvidenceDigest` in `reusedMutationEntry` (`cmd/test-design-verifier/finalize.go`). Run-2026-09-10-0002 enumerated it as site-4 and it survived because it is an equivalent mutant, not because the row's test is weak. Three checks dominate it on the same bytes: `pendingRefusal` refuses over resume's own `runstate.Verify` of the record before finalize assembles anything; `assembleRun` runs `runstate.Verify(rec)` in the same loop iteration immediately before `addMutationRow`; and `reuse.adopt` refuses adoption unless the completed record's artifact digest equals the reuse reference's, so an adopted record's artifact and its reference are always the same path and the same digest. Tampering before finalize is refused `run-incomplete`, tampering between the accounting and the assembly is refused `ledger-invalid`, and tampering after the report is written is caught by `stageReusedArtifacts`, which is a different check the row already binds. The check stays in place as defense in depth against the window it names; it earns no site.

### Production hunks declared as in-scope hardening

Appended to the A-2 table under the same rule: declared behaviors, not numbered obligations; their tests run in every trace but earn no layer-2 or layer-4 credit, and neither introduces data loss, corruption or exposure. A later finding that one of them is wrong or missing is amended by appending a fresh SC row, never by editing this list.

| Hunk | Behavior | Group | Suite evidence |
|---|---|---|---|
| `cmd/test-design-verifier/baseline.go:169-214` | Baseline evidence JVM toolchain identity: `toolVersions` reads the JVM identity out of the invoked runner's own `-version` banner (the first `Java version:`, `Launcher JVM:` or `JVM:` line of `mvn -B -version` or `gradle --version`), never from an unrelated `java` executable found on PATH, and records no `java` entry at all for a runner that reports none. `addToolVersion` strips ANSI escapes and takes the first non-empty, non-flag line as the runner's own version. Capture stays best-effort: a toolchain that cannot report its version does not invalidate the suite run that already succeeded with it. | 8 | `TestBaselineJVM_MavenE2EPublishesEvidence` (asserts both the `mvn` and the `java` entry), `TestTraceDispatchCarriesClaimRunnerAndToolchain` (the go runner's own version, with no JVM entry to report) |
| `cmd/test-design-verifier/baseline.go` (import order) | Formatting only, applied with this amendment: the import block was not `gofmt`-clean (`internal/trace` before `internal/subprocess`). No behavior changes. | 8 | `gofmt -l ./cmd ./internal` reports nothing |

## Source grounding

- Design semantics and future behaviors: approved spec sections Scenario selection, Deliberate live verification, Incremental verification, and Acceptance evidence.
- Structural credit: `internal/trace/join.go:36-180`; immutable current dispatch: `cmd/test-design-verifier/dispatch.go:17-105`.
- Live selection and execution: `cmd/test-design-verifier/executelive.go:137-259,305-521`; prepared substitution: `preparelive.go:114-155,190-215`; profile and plan rules: `internal/live/profile.go:15-135`, `internal/live/plan.go:76-195`.
- Completion patterns: `internal/live/state.go:30-65`, `internal/mutation/state.go:41-79`; current evaluation/restoration: `cmd/test-design-verifier/evaluatemutation.go:133-297`, `internal/worktree/diff.go:63-71`.
- Evidence/admission/accounting: `internal/report/load.go:25-142`, `validate.go:75-278,316-444`, `verdict.go:243-409`, `rows.go:73-87`.
- Publication/target boundaries: `cmd/test-design-verifier/finalize.go:104-292`, `internal/worktree/publish.go:41-155`, `skills/coverage-verification/references/finalize-recovery.md:41-78`.
- Future completion/manifests/recovery mutations name responsibilities already required by the spec, not asserted existing APIs or speculative implementation bodies.

## Planning and final verification handoff

Not authorized until this completed source receives final approval. Every active SC must map to a plan test step; W-1–W-3 are also explicit completion obligations under the approved mixed-evidence amendment. Preserve the installed canonical `covers:` placement and at-most-two-ID convention for the plugin’s own tagged tests until an approved verifier/source cutover changes it; multi-ID subjects inside scenario fixtures are the behavior under test, not permission to silently change this run’s tooling.

> Run the initial test against untouched production code. A failing result is TDD RED; a passing result is ALREADY-GREEN and returns to the parent for reconciliation. The SC row's verification mutation informs test design but is applied only by final coverage verification.

Implementation-time mutations are neither TDD RED nor layer-4 proof. Final high-risk measurement remains independent. The finishing handoff requires the user’s explicit `/test-design:coverage-verification` invocation; no implementation-plan or deployment approval is inferred here.
