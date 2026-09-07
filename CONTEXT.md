# test-design

Claude Code plugin plus Go verifier that turns an approved feature spec into a reviewed scenario table, then proves each scenario with bound tests, independent review, deployed-system journeys and mutation checks.

## Language

### Coverage source

**Coverage source**:
The Markdown document holding a feature's scenario table, verification settings, live decision, retired IDs and review record.
_Avoid_: coverage doc, table, spec

**Scenario**:
One `SC-nn` row: an observable requirement, a plausible consequential failure, a level, a verification mutation and a risk.
_Avoid_: case, test case, row (in prose)

**Retired scenario**:
A scenario removed from active obligations by an approved amendment; its ID stays reserved and is never reissued.
_Avoid_: deleted row, dropped row

**Verification mutation**:
The single named production fault a scenario's test must detect.
_Avoid_: bug, canary

**Risk**:
`high` or `med` per scenario, set by consequence (security boundary, data loss/corruption, irreversible side effect, harmful shared state) or an explicit impact rationale, never by axis membership alone.

**Live decision**:
The coverage source's declaration that deployed verification is `required` (with journeys) or `not-applicable` (with a reason).
_Avoid_: live flag, smoke decision

**Journey**:
One `J-n` deployed exercise in a live decision: the scenarios it exercises and its observable expected result on the selected revision.
_Avoid_: walkthrough, smoke test, e2e

**Live profile**:
Trusted project-owned execution configuration for one approved non-production target. Not a sandbox.
_Avoid_: provider, environment config

**Review record**:
Coverage-source section holding the scenario reviewer's instruction revision, findings and their resolutions.

### Trace and credit

**Site**:
One `covers:` tag bound to the test declaration directly beneath it.
_Avoid_: tag location, annotation

**Binding**:
The link between a site and one active scenario it cites.

**Contributor**:
A bound site recorded inside a scenario's row result together with its own execution status.
_Avoid_: source, binding (when execution status matters)

**Primary contributor**:
The contributor that fills a row result's legacy single-test fields; first by file then line.

**Row result**:
The credit record for a scenario with at least one executed contributor; its status collapses over executed contributors (failed over skipped over passed).
_Avoid_: binding result, credit

**Finding**:
A trace discrepancy of kind `gap`, `red`, `drift` or `ambiguity`, per contributor where it names a site.
_Avoid_: overclaim (no longer produced)

**Gap**:
A scenario, or one of its contributors, with no executed passing evidence: uncited, unexecuted or skipped.

### Review agents

**Scenario reviewer**:
The design-stage independent agent that judges whether draft scenarios deserve to exist, before human approval. Read-only; returns findings only.
_Avoid_: critic, design critic

**Coverage critic**:
The implementation-stage independent agent that judges whether a bound test would fail under its verification mutation.
_Avoid_: reviewer

**Recheck**:
The scenario reviewer's single second pass over the amended draft plus its previous findings; leftovers go to the human, not to another pass.
_Avoid_: re-review, second round

### Plugin validation

**Walkthrough**:
A `W-n` real-agent exercise of the plugin's own workflow on seeded representative inputs; evidence is the verbatim transcript and resolution.
_Avoid_: journey, demo

**Wave**:
One implementation slice of a feature with its own brief, ownership boundaries and closing evidence.

**Layer**:
One of the four verification stages: 1 trace binding, 2 critic verdict, 3 live journeys, 4 mutation evaluation.
