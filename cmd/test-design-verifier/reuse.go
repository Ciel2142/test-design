package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
	"test-design/internal/runstate"
	"test-design/internal/worktree"
)

// This file lets one run stand on an earlier run's answers. It adopts nothing
// it has not proved unchanged: every input a prior verdict depended on — the
// approved claim, the whole files the critic read, the runner, the toolchain,
// the critic's own policy — is compared against what this run observes now, and
// a record whose comparison does not hold is refused by name rather than
// quietly skipped. Refusal is per record: reuse adopts what verifies and
// refuses what does not, in one answer. It never writes into the prior run's
// ledger, never copies its artifacts, and never adopts the trace phase or the
// live attempt, which are observations of the run asking.

// The classes a changed path is read under, the two answers a basis records for
// policy and toolchain, and the reason the two never-adopted obligations carry.
const (
	changeExempt   = "exempt"
	changeIsolated = "isolated"
	changeShared   = "shared"
	changeUnknown  = "unknown"

	basisMatched       = "matched"
	basisNotApplicable = "not-applicable"

	reasonFreshRequired = "fresh-required"
	// reasonRecordUnreadable is a prior record whose captured inputs will not
	// decode: not legacy, which is a shape this executable knows and predates,
	// but a shape it cannot read at all.
	reasonRecordUnreadable = "record-unreadable"
)

type reuseRequest struct {
	verifyRequest
	PriorRunID string        `json:"priorRunId"`
	Policy     *reviewPolicy `json:"policy,omitempty"` // the critic policy this run will dispatch with
}

// reuseChangedPath is one tracked path that differs between the prior run's
// commit and this one, and what the prior manifests say it belongs to.
type reuseChangedPath struct {
	Path   string   `json:"path"`
	Class  string   `json:"class"`            // isolated | shared | unknown | exempt
	Groups []string `json:"groups,omitempty"` // prior groups whose context names the path
}

// reuseDecision is what became of one prior record: adopted with the
// provenance it carries, or refused with the first reason that answered.
type reuseDecision struct {
	Kind      runstate.Kind            `json:"kind"`
	ID        string                   `json:"id"`
	Rows      []string                 `json:"rows,omitempty"`
	Reused    bool                     `json:"reused"`
	Outcome   string                   `json:"outcome,omitempty"`
	Reason    string                   `json:"reason,omitempty"`
	Reference *evidence.ReuseReference `json:"reference,omitempty"`
}

type reuseCounts struct {
	Reused       int `json:"reused"`
	ReusedFailed int `json:"reusedFailed"`
	Refused      int `json:"refused"`
}

type reuseOutput struct {
	evidence.Envelope
	RunID           string             `json:"runId"`
	PriorRunID      string             `json:"priorRunId"`
	PriorRepoCommit string             `json:"priorRepoCommit"`
	RepoCommit      string             `json:"repoCommit"`
	ChangedPaths    []reuseChangedPath `json:"changedPaths"`
	Trace           reuseDecision      `json:"trace"` // always reused:false, reason fresh-required
	Live            reuseDecision      `json:"live"`  // always reused:false, reason fresh-required
	Review          []reuseDecision    `json:"review"`
	Mutation        []reuseDecision    `json:"mutation"`
	Counts          reuseCounts        `json:"counts"`
}

// reusedMutationInputs is the inputs shape of an adopted mutation-row record:
// the workspace identity the prior run recorded, unchanged, plus the manifest
// of the group that reviewed the row and the provenance of what was adopted.
type reusedMutationInputs struct {
	mutationRowInputs
	Manifest *evidence.Manifest       `json:"manifest"`
	Reuse    *evidence.ReuseReference `json:"reuse"`
}

// reuseBeforeAdopt runs inside the reuse transaction after every prior record
// has been verified and before the first is adopted; it is nil in production
// and exists so a test can change a retained artifact at the moment adopt's
// re-digest must catch it.
var reuseBeforeAdopt func()

func newReuseFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("reuse", flag.ContinueOnError)
	addRequestFlag(fs)
	return fs
}

func decodeReuseRequest(op string, requestBody []byte) (*reuseRequest, *cliError) {
	base, cliErr := decodeVerifyRequest(op, requestBody)
	if cliErr != nil {
		return nil, cliErr
	}
	var extra struct {
		PriorRunID string        `json:"priorRunId"`
		Policy     *reviewPolicy `json:"policy"`
	}
	if err := json.Unmarshal(requestBody, &extra); err != nil {
		return nil, &cliError{Code: "malformed-request", Message: fmt.Sprintf("%s: decode request: %v", op, err)}
	}
	if base.RunID == "" {
		return nil, &cliError{Code: "missing-run-id", Message: op + ": runID is required"}
	}
	if extra.PriorRunID == "" {
		return nil, &cliError{
			Code:    "missing-prior-run-id",
			Message: op + ": priorRunId is required; reuse stands on a named earlier run",
		}
	}
	if extra.PriorRunID == base.RunID {
		return nil, &cliError{
			Code:    "prior-run-is-current",
			Message: fmt.Sprintf("%s: %s is this run; a run cannot adopt its own records", op, extra.PriorRunID),
		}
	}
	return &reuseRequest{verifyRequest: *base, PriorRunID: extra.PriorRunID, Policy: extra.Policy}, nil
}

// reuseHandler adopts what the prior run proved and still holds. The current
// run's own gates answer first — it must already stand on its own trace and its
// inputs must not have moved since — then the prior run is judged as a whole,
// then every record it holds is judged on its own. The per-record judgement and
// the writes it decides run inside one exclusive transaction over this run's
// ledger, so a row another group claimed meanwhile is seen before it is claimed
// again.
func reuseHandler(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError {
	const op = "reuse"

	req, cliErr := decodeReuseRequest(op, requestBody)
	if cliErr != nil {
		return cliErr
	}
	src, cliErr := loadVerifiedSource(op, req.RepoRoot, req.CoverageSource)
	if cliErr != nil {
		return cliErr
	}
	repoCommit, cliErr := headCommit(op, req.RepoRoot)
	if cliErr != nil {
		return cliErr
	}
	store, cliErr := openLedger(op, &req.verifyRequest)
	if cliErr != nil {
		return cliErr
	}
	ledger, cliErr := loadLedgerFor(op, runstate.KindPhase, store)
	if cliErr != nil {
		return cliErr
	}
	// Adopted evidence is added to a run, never used to start one: the rows a
	// group answers for are the rows this run's own trace dispatched.
	if _, _, cliErr := currentDispatch(op, ledger); cliErr != nil {
		return cliErr
	}
	// Staleness is answered here rather than left to the ledger's identity
	// check, so a run whose document or commit moved under it reads as evidence
	// that no longer stands rather than as a foreign ledger.
	if stale, reason := staleAgainst(ledger.Identity, repoCommit, src); stale {
		return &cliError{
			Code:    "stale-evidence",
			Message: fmt.Sprintf("%s: %s; trace this run again before reusing", op, reason),
		}
	}

	// The repository is read the way trace and finalize read it: as a commit.
	// An uncommitted change is a tree no comparison against the prior commit
	// describes, so once this run's own gates have answered it is refused —
	// before the prior run is opened and before any record is judged — with
	// the reason every other op gives.
	ctrl, err := worktree.NewController(req.RepoRoot)
	if err != nil {
		return &cliError{Code: "controller-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	before, err := ctrl.TakeSnapshot()
	if err != nil {
		return &cliError{Code: "snapshot-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	if !before.Clean() {
		return &cliError{
			Code:    "source-not-reproducible",
			Message: fmt.Sprintf("%s: source not reproducible: %s", op, describeDirty(req.RepoRoot)),
		}
	}
	prior, cliErr := openPriorRun(op, req, src)
	if cliErr != nil {
		return cliErr
	}
	// A commit this repository has but cannot be diffed against HEAD — a
	// shallow clone, a corrupted object — is a prior run this one cannot be
	// compared with, which is what prior-commit-unreachable says.
	paths, err := changedPaths(req.RepoRoot, prior.identity().RepoCommit, repoCommit)
	if err != nil {
		return &cliError{
			Code: "prior-commit-unreachable",
			Message: fmt.Sprintf("%s: cannot compare run %s at %s with HEAD: %v", op,
				prior.runID, prior.identity().RepoCommit, err),
		}
	}
	policy, policyProblems := policyIdentity(req.Policy)
	run := &reuseRun{
		op: op, req: req, src: src, prior: prior, policy: policy,
		policyProblems: policyProblems,
		active:         activeRowIDs(src),
		changes:        classifyChanges(paths, req.CoverageSource, src.Runner.Kind, prior.groups),
	}

	var out reuseOutput
	err = store.Update(ledgerIdentity(&req.verifyRequest, req.RepoRoot, repoCommit, src), func(l *runstate.Ledger) error {
		dispatch, digest, cliErr := currentDispatch(op, l)
		if cliErr != nil {
			return cliErr
		}
		run.dispatch, run.digest = dispatch, digest
		run.claims = claimsOf(dispatch)
		if reuseBeforeAdopt != nil {
			reuseBeforeAdopt()
		}

		review, err := run.reviewDecisions(l)
		if err != nil {
			return err
		}
		rows, err := run.mutationDecisions(l)
		if err != nil {
			return err
		}
		out = reuseOutput{
			Envelope:        evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
			RunID:           req.RunID,
			PriorRunID:      prior.runID,
			PriorRepoCommit: prior.identity().RepoCommit,
			RepoCommit:      repoCommit,
			ChangedPaths:    run.changes.paths,
			Trace:           reuseDecision{Kind: runstate.KindPhase, ID: tracePhaseID, Reason: reasonFreshRequired},
			Live:            reuseDecision{Kind: runstate.KindLiveAttempt, ID: liveAttemptID, Reason: reasonFreshRequired},
			Review:          review,
			Mutation:        rows,
			Counts:          reuseCountsOf(review, rows),
		}
		return nil
	})
	if cliErr := updateError(op, runstate.KindReviewGroup, err); cliErr != nil {
		return cliErr
	}
	return writeStdout(stdout, out)
}

// priorRun is the earlier run reuse is asked to stand on, read once and never
// written: its ledger, the review groups it holds, and the dispatch document
// its trace wrote when that is still on disk.
type priorRun struct {
	runID  string
	ledger *runstate.Ledger
	groups []*priorGroup
	// dispatch is the prior trace's document and the digest its record carries,
	// present only when that record still verifies. It answers two questions —
	// how the trace bound a row, and which toolchain the run measured under —
	// and a run directory a temporary root has cleaned leaves both unanswered.
	dispatch       *dispatchInputs
	dispatchRows   map[string]dispatchRow
	dispatchDigest string
}

func (p *priorRun) identity() runstate.Identity { return p.ledger.Identity }

// openPriorRun finds the earlier run's ledger — every run of one repository
// lives under the same key, so its run ID is enough — and answers whether this
// run may be compared against it at all: it must exist, be readable, be a run
// of the same coverage document, and name a commit this repository still has.
func openPriorRun(op string, req *reuseRequest, src *coverage.Source) (*priorRun, *cliError) {
	priorReq := req.verifyRequest
	priorReq.RunID = req.PriorRunID
	// The prior run is read, never created: its ledger is looked for on disk
	// before any store is opened over it, so a run ID nobody ever traced leaves
	// no directory behind for the next reuse to mistake for a run.
	dir, cliErr := locateLedgerDir(op, &priorReq)
	if cliErr != nil {
		return nil, cliErr
	}
	store, cliErr := openExistingLedger(op, dir)
	if cliErr != nil {
		return nil, cliErr
	}
	if store == nil {
		return nil, &cliError{
			Code:    "prior-ledger-missing",
			Message: fmt.Sprintf("%s: run %s has no ledger under this repository", op, req.PriorRunID),
		}
	}
	ledger, err := store.Load()
	switch {
	case errors.Is(err, runstate.ErrNoLedger):
		return nil, &cliError{
			Code:    "prior-ledger-missing",
			Message: fmt.Sprintf("%s: run %s has no ledger under this repository", op, req.PriorRunID),
		}
	case err != nil:
		return nil, &cliError{Code: "prior-ledger-invalid", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	if ledger.Identity.Source != src.Path {
		return nil, &cliError{
			Code: "prior-source-mismatch",
			Message: fmt.Sprintf("%s: run %s verified %s, not %s", op,
				req.PriorRunID, ledger.Identity.Source, src.Path),
		}
	}
	if !commitExists(req.RepoRoot, ledger.Identity.RepoCommit) {
		return nil, &cliError{
			Code: "prior-commit-unreachable",
			Message: fmt.Sprintf("%s: run %s was made at %s, which this repository does not have", op,
				req.PriorRunID, ledger.Identity.RepoCommit),
		}
	}
	prior := &priorRun{runID: req.PriorRunID, ledger: ledger, groups: readPriorGroups(ledger)}
	prior.dispatch, prior.dispatchDigest = priorTrace(ledger)
	if prior.dispatch != nil {
		prior.dispatchRows = dispatchRowsByID(prior.dispatch)
	}
	return prior, nil
}

// locateLedgerDir places a run's ledger directory exactly as openLedger does —
// the resolved state root, refused when it resolves inside the repository —
// without creating it. It is the read-only half of openLedger, for the ops
// that answer "is there a ledger" from what is on disk.
func locateLedgerDir(op string, req *verifyRequest) (string, *cliError) {
	fail := func(err error) *cliError {
		return &cliError{Code: "ledger-open-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	stateDir, err := runstate.DefaultStateDir()
	if err != nil {
		return "", fail(err)
	}
	resolvedState, err := resolveExistingPrefix(stateDir)
	if err != nil {
		return "", fail(fmt.Errorf("resolve state dir %s: %w", stateDir, err))
	}
	repoRoot, err := resolveThroughSymlinks(req.RepoRoot)
	if err != nil {
		return "", fail(fmt.Errorf("resolve repo root %s: %w", req.RepoRoot, err))
	}
	if isWithinPath(repoRoot, resolvedState) {
		return "", fail(fmt.Errorf("state dir %s resolves inside repository %s", resolvedState, repoRoot))
	}
	dir, err := runstate.Locate(resolvedState, repoRoot, req.RunID)
	if err != nil {
		return "", fail(err)
	}
	return dir, nil
}

// openExistingLedger opens a store over dir only when dir already holds a
// ledger file, and reports a nil store when it does not. Nothing is created
// on the way to a nil answer.
func openExistingLedger(op, dir string) (*runstate.Store, *cliError) {
	_, err := os.Stat(filepath.Join(dir, runstate.LedgerFile))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, &cliError{Code: "ledger-open-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	store, err := runstate.Open(dir)
	if err != nil {
		return nil, &cliError{Code: "ledger-open-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	return store, nil
}

// tracePreceding is the trace record — current or superseded — with the
// greatest sequence number below seq: the trace the prior run stood on when
// it opened the record at seq, and so the one that says which toolchain that
// record was measured under. Nil when no trace preceded it.
func (p *priorRun) tracePreceding(seq int) *runstate.Record {
	var preceding *runstate.Record
	for i := range p.ledger.Records {
		rec := &p.ledger.Records[i]
		if rec.Kind != runstate.KindPhase || rec.ID != tracePhaseID || rec.Seq >= seq {
			continue
		}
		if preceding == nil || rec.Seq > preceding.Seq {
			preceding = rec
		}
	}
	return preceding
}

// priorTrace is the dispatch document of the prior run's current trace and the
// digest that record carries for it, when the record completed and its bytes
// still verify. Anything less is no document at all: a run that repeated its
// trace, or whose run directory was cleaned, has nothing here this run may read
// a prior observation out of.
func priorTrace(ledger *runstate.Ledger) (*dispatchInputs, string) {
	rec := runstate.Current(ledger, runstate.KindPhase, tracePhaseID)
	if rec == nil || rec.Status != runstate.StatusCompleted || len(runstate.Verify(rec)) > 0 {
		return nil, ""
	}
	artifact := rec.Artifacts[artifactDispatchInputs]
	dispatch, err := loadDispatchInputs(artifact.Path)
	if err != nil {
		return nil, ""
	}
	return dispatch, artifact.SHA256
}

// priorGroup is one review-group record of the prior run as reuse reads it: the
// record, the rows and per-row sources it was judged against, the complete
// manifest its evidence carries, and the refusal the record alone decides.
type priorGroup struct {
	rec     *runstate.Record
	rows    []string
	sources map[string][]string
	// bundle and context are what this group's critic was handed and what it
	// was handed or reported reading, gathered from every manifest the prior
	// run recorded for it. They describe ownership, not eligibility: a group
	// that cannot be adopted was still handed its files, and a change to one of
	// them is a change to something somebody reviewed.
	bundle         map[string]bool
	context        map[string]bool
	dispatchDigest string
	manifest       *evidence.Manifest
	reason         string
}

// own records what a manifest says this group's critic was handed and read.
func (g *priorGroup) own(manifest *evidence.Manifest) {
	if manifest == nil {
		return
	}
	for path := range manifest.Bundle {
		g.bundle[path] = true
		g.context[path] = true
	}
	for path := range manifest.Reads {
		g.context[path] = true
	}
}

func readPriorGroups(ledger *runstate.Ledger) []*priorGroup {
	var groups []*priorGroup
	for i := range ledger.Records {
		rec := &ledger.Records[i]
		if rec.Kind != runstate.KindReviewGroup || rec.Status == runstate.StatusSuperseded {
			continue
		}
		groups = append(groups, readPriorGroup(rec))
	}
	return groups
}

// readPriorGroup reads one group's evidence, in the order the answers come: a
// group the prior run never closed has nothing to read, a record whose inputs
// will not decode is a checkpoint nothing can be told about, a record that
// captured no manifest is from before manifests existed and is never repaired,
// and the artifact's bytes are re-hashed before anything inside them is
// believed. The manifest taken is the artifact's, which is the complete one;
// the record's own inputs are consulted only to tell a legacy record from a
// current one.
func readPriorGroup(rec *runstate.Record) *priorGroup {
	group := &priorGroup{rec: rec, bundle: map[string]bool{}, context: map[string]bool{}}
	var inputs reviewGroupInputs
	inputsErr := recordInputs(rec, &inputs)
	group.rows = inputs.Rows
	// Ownership is taken from the started manifest before anything is decided
	// about the record, so a group nothing can adopt still says which files it
	// was handed.
	group.own(inputs.Manifest)

	switch {
	case rec.Status != runstate.StatusCompleted:
		group.reason = "prior-not-completed"
		return group
	case inputsErr != nil:
		group.reason = reasonRecordUnreadable
		return group
	case inputs.Manifest == nil:
		group.reason = "legacy-no-manifest"
		return group
	}
	if reasons := runstate.Verify(rec); len(reasons) > 0 {
		group.reason = reasons[0]
		return group
	}
	_, artifact, ok := recordArtifact(rec)
	if !ok {
		group.reason = "artifact-missing:" + artifactEvidence
		return group
	}
	doc, err := loadReviewEvidence(artifact.Path)
	if err != nil {
		group.reason = "artifact-unreadable:" + artifactEvidence
		return group
	}
	if doc.Inputs.Manifest == nil {
		group.reason = "legacy-no-manifest"
		return group
	}
	group.own(doc.Inputs.Manifest)
	group.manifest, group.sources = doc.Inputs.Manifest, doc.Inputs.Sources
	group.dispatchDigest = doc.Inputs.DispatchInputsSHA256
	return group
}

// loadReviewEvidence reads back a review group's retained evidence — the
// document that carries the group's complete manifest.
func loadReviewEvidence(path string) (*reviewEvidence, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read review evidence %s: %w", path, err)
	}
	var doc reviewEvidence
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("read review evidence %s: malformed JSON: %w", path, err)
	}
	return &doc, nil
}

// recordArtifact is the one file a completed record's outcome rests on: its
// evidence, or the attempt document for an outcome that produced none.
func recordArtifact(rec *runstate.Record) (string, runstate.Artifact, bool) {
	for _, role := range []string{artifactEvidence, artifactAttempt} {
		if artifact, ok := rec.Artifacts[role]; ok {
			return role, artifact, true
		}
	}
	return "", runstate.Artifact{}, false
}

// reuseChanges is what moved between the prior run's commit and this one, and
// what that costs: the reason every record inherits when a change cannot be
// attributed to one group, and the reason a group inherits when a file only it
// was handed moved.
type reuseChanges struct {
	paths    []reuseChangedPath
	widened  string
	isolated map[string]string
}

// classifyChanges reads every changed path against the prior manifests. A test
// file exactly one group's bundle names, and no other group's context does, is
// that group's own; anything else a manifest cannot attribute — a production,
// build, data or documentation change, or a test file no manifest names at all
// — is uncertainty, and uncertainty widens to every record rather than being
// guessed at. The one exemption is the coverage source's own directory, where
// the run publishes its report and transcripts; the source itself is compared
// by claims and runner, not by path.
func classifyChanges(paths []string, coverageSource, runnerKind string, groups []*priorGroup) *reuseChanges {
	changes := &reuseChanges{paths: []reuseChangedPath{}, isolated: map[string]string{}}
	for _, changed := range paths {
		entry := reuseChangedPath{Path: changed, Groups: contextOwners(groups, changed)}
		switch {
		case isPublication(coverageSource, changed):
			entry.Class = changeExempt
		case !isTestFile(runnerKind, changed):
			entry.Class = changeShared
		case len(entry.Groups) == 0:
			entry.Class = changeUnknown
		case len(entry.Groups) == 1 && bundledBy(groups, entry.Groups[0], changed):
			entry.Class = changeIsolated
		default:
			entry.Class = changeShared
		}

		switch entry.Class {
		case changeIsolated:
			if _, named := changes.isolated[entry.Groups[0]]; !named {
				changes.isolated[entry.Groups[0]] = "isolated-change:" + changed
			}
		case changeShared:
			changes.widen("module-changed:" + changed)
		case changeUnknown:
			changes.widen("unknown-ownership:" + changed)
		}
		changes.paths = append(changes.paths, entry)
	}
	return changes
}

// widen keeps the first widening reason. Paths arrive sorted, so the reason
// every record carries names the first such path in sorted order.
func (c *reuseChanges) widen(reason string) {
	if c.widened == "" {
		c.widened = reason
	}
}

// basisPaths is what an adopted record was judged against: every changed path
// the coverage directory does not exempt, minus the files that belong to the
// record's own group, which by then are known not to have moved.
func (c *reuseChanges) basisPaths(ownGroup string) []string {
	paths := []string{}
	for _, changed := range c.paths {
		if changed.Class == changeExempt {
			continue
		}
		if changed.Class == changeIsolated && slices.Contains(changed.Groups, ownGroup) {
			continue
		}
		paths = append(paths, changed.Path)
	}
	return paths
}

// contextOwners names the prior groups whose recorded context — the bundle its
// critic was handed or the reads that critic reported — includes path.
func contextOwners(groups []*priorGroup, path string) []string {
	var owners []string
	for _, group := range groups {
		if group.context[path] {
			owners = append(owners, group.rec.ID)
		}
	}
	return owners
}

// bundledBy reports whether the named group was handed path as one of its own
// files, rather than merely reporting that it read it.
func bundledBy(groups []*priorGroup, id, path string) bool {
	for _, group := range groups {
		if group.rec.ID == id {
			return group.bundle[path]
		}
	}
	return false
}

// isPublication reports whether path is the coverage source or a file beside
// it: the one directory a run writes into, where finalize publishes its report
// and its transcripts. A source at the repository root claims no such
// directory — a whole repository is not a publication.
func isPublication(coverageSource, path string) bool {
	if path == coverageSource {
		return true
	}
	cut := strings.LastIndex(coverageSource, "/")
	if cut < 0 {
		return false
	}
	return strings.HasPrefix(path, coverageSource[:cut+1])
}

// reuseRun is one reuse in progress: what this run observes now, what the prior
// run recorded, and what moved between them.
type reuseRun struct {
	op             string
	req            *reuseRequest
	src            *coverage.Source
	prior          *priorRun
	changes        *reuseChanges
	policy         *evidence.PolicyIdentity
	policyProblems []string
	dispatch       *dispatchInputs
	digest         string
	active         []string
	claims         map[string]string
}

// claimsOf is the approved semantics this run hands out per row, as the trace's
// dispatch document digested it.
func claimsOf(dispatch *dispatchInputs) map[string]string {
	claims := make(map[string]string, len(dispatch.Rows))
	for _, row := range dispatch.Rows {
		claims[row.RowID] = row.ClaimSHA256
	}
	return claims
}

// reviewDecisions judges every current review group of the prior run, in ledger
// order, and adopts the ones nothing refuses.
func (r *reuseRun) reviewDecisions(l *runstate.Ledger) ([]reuseDecision, error) {
	decisions := []reuseDecision{}
	for _, group := range r.prior.groups {
		decision := reuseDecision{Kind: runstate.KindReviewGroup, ID: group.rec.ID, Rows: group.rows}
		if reason := r.reviewRefusal(group, l); reason != "" {
			decision.Reason = reason
			decisions = append(decisions, decision)
			continue
		}
		reference, err := r.adoptReview(l, group)
		if err != nil {
			return nil, err
		}
		decision.Reused, decision.Outcome, decision.Reference = true, group.rec.Outcome, reference
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

// reviewRefusal is why this run may not stand on one prior group's verdict,
// empty when it may. The first answer wins, and they are asked cheapest first:
// what the record says about itself, then what its manifest says, then what the
// document and the repository say, then what this run's own ledger already
// holds.
func (r *reuseRun) reviewRefusal(group *priorGroup, l *runstate.Ledger) string {
	if group.reason != "" {
		return group.reason
	}
	if group.manifest.Version != evidence.ManifestVersion {
		return "manifest-version-unsupported"
	}
	if reason := firstIncomplete(group.manifest); reason != "" {
		return "manifest-incomplete:" + reason
	}
	if row := r.inactiveRow(group.rows); row != "" {
		return "row-inactive:" + row
	}
	if r.changes.widened != "" {
		return r.changes.widened
	}
	if reason := r.changes.isolated[group.rec.ID]; reason != "" {
		return reason
	}
	// A review group is judged on its policy and not on its toolchain: a critic
	// reads source, it does not run it.
	if reasons := evidence.Mismatch(group.manifest, r.probe(group.manifest, r.policy), nil, true, false); len(reasons) > 0 {
		return reasons[0]
	}
	// A policy input this run could not identify is not one it compared: the
	// digests it did compute would equally match a prior policy that never
	// named the unreadable file at all.
	if len(r.policyProblems) > 0 {
		return r.policyProblems[0]
	}
	current, _ := reviewGroupInputsFor(r.dispatch, group.rows, r.digest)
	for _, row := range group.rows {
		if !slices.Equal(current.Sources[row], group.sources[row]) {
			return "sources-changed:" + row
		}
	}
	for _, row := range group.rows {
		if r.statusChanged(group, row) {
			return "status-changed:" + row
		}
	}
	if runstate.Current(l, runstate.KindReviewGroup, group.rec.ID) != nil {
		return "already-recorded"
	}
	if row := groupedElsewhere(l, group.rec.ID, group.rows); row != "" {
		return "row-already-grouped:" + row
	}
	return ""
}

// mutationDecisions judges every current mutation row of the prior run, in
// ledger order, and adopts the ones nothing refuses. A row is judged on its own
// inputs against the manifest of the group that reviewed its test, so a group
// refused for another row's claim or for the critic policy does not drag its
// rows down with it.
func (r *reuseRun) mutationDecisions(l *runstate.Ledger) ([]reuseDecision, error) {
	decisions := []reuseDecision{}
	for i := range r.prior.ledger.Records {
		rec := &r.prior.ledger.Records[i]
		if rec.Kind != runstate.KindMutationRow || rec.Status == runstate.StatusSuperseded {
			continue
		}
		decision := reuseDecision{Kind: runstate.KindMutationRow, ID: rec.ID}
		var inputs mutationRowInputs
		inputsErr := recordInputs(rec, &inputs)

		reason, context := r.mutationRefusal(rec, inputs, inputsErr, l)
		if reason != "" {
			decision.Reason = reason
			decisions = append(decisions, decision)
			continue
		}
		reference := r.reference(rec, []string{rec.ID}, context.group.rec.ID,
			len(context.recorded.ContextPaths()), basisNotApplicable, basisMatched)
		// The adopted row keeps the manifest it was judged against — its
		// review context's, or its own when it was adopted before — with the
		// toolchain it was actually measured under in place of the one that
		// reviewed it, so the next run judges the row on its own execution
		// rather than on a review it may have inherited.
		adopted := reusedMutationInputs{mutationRowInputs: inputs, Manifest: context.manifest(), Reuse: reference}
		if err := r.adopt(l, runstate.KindMutationRow, rec, adopted, reference); err != nil {
			return nil, err
		}
		decision.Reused, decision.Outcome, decision.Reference = true, rec.Outcome, reference
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

// mutationContext is what a prior mutation row is judged against: the group
// that reviewed it, the manifest that stands for the row — the group's for a
// row the prior run measured itself, the row's own recorded one for a row that
// run adopted, since a fresh review may have read less than the original — and
// the toolchain the row was actually measured under, which is not always the
// one that reviewed it.
type mutationContext struct {
	group     *priorGroup
	recorded  *evidence.Manifest
	toolchain map[string]string
}

// manifest is the recorded context with the row's own executing toolchain in
// place of whatever that context carried: everything else about the inputs is
// the context's, but which toolchain ran the mutant is the row's own fact.
func (c *mutationContext) manifest() *evidence.Manifest {
	measured := *c.recorded
	measured.ToolVersions = c.toolchain
	return &measured
}

// adoptedMutation is the row's inputs as a record the prior run adopted from
// an earlier one carries them, nil for a row that run measured itself.
func adoptedMutation(rec *runstate.Record) *reusedMutationInputs {
	var adopted reusedMutationInputs
	if err := recordInputs(rec, &adopted); err != nil || adopted.Reuse == nil || adopted.Manifest == nil {
		return nil
	}
	return &adopted
}

// recordReuse is the reuse reference a prior record carries, nil for a record
// the prior run produced itself.
func recordReuse(rec *runstate.Record) *evidence.ReuseReference {
	var carried struct {
		Reuse *evidence.ReuseReference `json:"reuse"`
	}
	if err := recordInputs(rec, &carried); err != nil {
		return nil
	}
	return carried.Reuse
}

// mutationRefusal is why this run may not stand on one prior mutation row,
// empty when it may, with the context an adoption then rests on.
func (r *reuseRun) mutationRefusal(
	rec *runstate.Record, inputs mutationRowInputs, inputsErr error, l *runstate.Ledger,
) (string, *mutationContext) {
	if rec.Status != runstate.StatusCompleted {
		return "prior-not-completed", nil
	}
	if reasons := runstate.Verify(rec); len(reasons) > 0 {
		return reasons[0], nil
	}
	// Verify passes over a record with no artifacts at all: an outcome resting
	// on no file is not evidence, whatever role it would have been kept under.
	if _, _, ok := recordArtifact(rec); !ok {
		return "artifact-missing:" + artifactEvidence, nil
	}
	// Mutation evidence follows its review group: a row nothing reviewed, or
	// whose group's evidence no longer verifies, has no context to be judged
	// against, and no context is a refusal rather than a judgement made anyway.
	group := r.contextGroup(rec.ID)
	if group == nil {
		return "no-review-context", nil
	}
	if row := r.inactiveRow([]string{rec.ID}); row != "" {
		return "row-inactive:" + row, nil
	}
	// An input identity this run cannot read is not one it can show unchanged;
	// one that was never recorded — a row answered without ever being prepared
	// against a bound test — has nothing to compare at all.
	if inputsErr != nil {
		return reasonRecordUnreadable, nil
	}
	if inputs.Identity == (mutationRowIdentity{}) {
		return "identity-unrecorded", nil
	}
	bound, _ := r.dispatchRow(rec.ID)
	if inputs.Identity.Scope != bound.Scope || inputs.Identity.Name != bound.TestName {
		return "identity-changed", nil
	}
	if r.changes.widened != "" {
		return r.changes.widened, nil
	}
	if reason := r.changes.isolated[group.rec.ID]; reason != "" {
		return reason, nil
	}
	adopted := adoptedMutation(rec)
	toolchain, known := r.executingToolchain(rec, adopted)
	// Two empty toolchains are not the same toolchain: a version probe that
	// answered nothing, on either side, leaves nothing to compare.
	if !known || len(toolchain) == 0 || len(r.dispatch.ToolVersions) == 0 {
		return "toolchain-unrecorded", nil
	}
	// A mutation row is judged on its own claim and on the toolchain that ran
	// it; the critic policy that reviewed its group answers for the group.
	context := &mutationContext{group: group, recorded: group.manifest, toolchain: toolchain}
	if adopted != nil {
		context.recorded = adopted.Manifest
	}
	manifest := context.manifest()
	reasons := evidence.Mismatch(manifest, r.probe(manifest, nil), []string{rec.ID}, false, true)
	if len(reasons) > 0 {
		return reasons[0], nil
	}
	if runstate.Current(l, runstate.KindMutationRow, rec.ID) != nil {
		return "already-recorded", nil
	}
	return "", context
}

// executingToolchain is the toolchain a prior mutation row was measured under.
// A row this run's predecessor adopted from an earlier one carries it in its
// own recorded manifest; a row that run measured itself was measured under the
// trace that preceded the row in its ledger — not the run's latest trace,
// which may have been repeated under another toolchain after the row was
// measured — and only when that trace's document still verifies. When neither
// can be established the row is not judged at all: a mutant whose execution
// environment is unknown is not one a later run can show unchanged, and the
// reviewing run's toolchain is not an answer — an adopted review may have been
// made under a third one entirely.
func (r *reuseRun) executingToolchain(rec *runstate.Record, adopted *reusedMutationInputs) (map[string]string, bool) {
	if adopted != nil {
		return adopted.Manifest.ToolVersions, true
	}
	trace := r.prior.tracePreceding(rec.Seq)
	if trace == nil || len(runstate.Verify(trace)) > 0 {
		return nil, false
	}
	artifact, ok := trace.Artifacts[artifactDispatchInputs]
	if !ok {
		return nil, false
	}
	dispatch, err := loadDispatchInputs(artifact.Path)
	if err != nil {
		return nil, false
	}
	return dispatch.ToolVersions, true
}

// contextGroup is the prior group whose manifest stands for a row: the one that
// reviewed the row, whose evidence verified and whose manifest is complete.
func (r *reuseRun) contextGroup(row string) *priorGroup {
	for _, group := range r.prior.groups {
		if group.manifest == nil || !group.manifest.Complete {
			continue
		}
		if slices.Contains(group.rows, row) {
			return group
		}
	}
	return nil
}

// probe is what this run observes now for a prior manifest's inputs: the claims
// the document approves, the context files as they are on disk, the runner and
// toolchain the trace recorded, and the policy the request named.
func (r *reuseRun) probe(manifest *evidence.Manifest, policy *evidence.PolicyIdentity) evidence.Probe {
	files, _ := digestUnder(r.req.RepoRoot, manifest.ContextPaths())
	return evidence.Probe{
		Claims:       r.claims,
		Files:        files,
		Runner:       r.dispatch.Runner,
		ToolVersions: r.dispatch.ToolVersions,
		Policy:       policy,
	}
}

// inactiveRow names the first row this run no longer carries: retired or
// renumbered in the document, or absent from the trace it dispatched. Either
// way the prior record answers for something nobody is asking about.
func (r *reuseRun) inactiveRow(rows []string) string {
	for _, row := range rows {
		if !slices.Contains(r.active, row) {
			return row
		}
		if _, bound := r.dispatchRow(row); !bound {
			return row
		}
	}
	return ""
}

func (r *reuseRun) dispatchRow(row string) (dispatchRow, bool) {
	for _, entry := range r.dispatch.Rows {
		if entry.RowID == row {
			return entry, true
		}
	}
	return dispatchRow{}, false
}

// statusChanged reports whether this run's trace binds row differently than the
// trace the group was judged against did. The comparison is made only against
// that exact document — the prior run's current trace, when it verifies and its
// digest is the one the group's evidence names — never against a later trace
// the prior run repeated, which nobody reviewed under. A prior run whose run
// directory has been cleaned, or which traced again, leaves the question
// unasked; the claim, source and context comparisons still answer for the row.
func (r *reuseRun) statusChanged(group *priorGroup, row string) bool {
	if r.prior.dispatchDigest == "" || r.prior.dispatchDigest != group.dispatchDigest {
		return false
	}
	before, recorded := r.prior.dispatchRows[row]
	if !recorded {
		return false
	}
	now, bound := r.dispatchRow(row)
	return bound && before.Status != now.Status
}

// adoptReview records a prior group as this run's own: the rows, sources and
// dispatch digest are this run's, because they are what the run must answer
// for; the manifest and the reference are the prior run's, because they are
// what the verdict rests on.
func (r *reuseRun) adoptReview(l *runstate.Ledger, group *priorGroup) (*evidence.ReuseReference, error) {
	inputs, ok := reviewGroupInputsFor(r.dispatch, group.rows, r.digest)
	if !ok {
		return nil, fmt.Errorf("%s: group %s names a row this run's trace does not", r.op, group.rec.ID)
	}
	reference := r.reference(group.rec, group.rows, group.rec.ID,
		len(group.manifest.ContextPaths()), basisMatched, basisNotApplicable)
	inputs.Manifest, inputs.Reuse = group.manifest, reference
	return reference, r.adopt(l, runstate.KindReviewGroup, group.rec, inputs, reference)
}

// adopt writes one prior record as this run's: started and completed in the
// same transaction, pointing at the original artifact, whose bytes Complete
// digests again. No copy is made and nothing of the prior run is written. The
// digest Complete arrived at must be the one the reference was built from —
// the bytes this run verified — or the file moved while it was being adopted
// and the whole transaction is abandoned.
func (r *reuseRun) adopt(
	l *runstate.Ledger, kind runstate.Kind, rec *runstate.Record, inputs any, reference *evidence.ReuseReference,
) error {
	if _, err := l.Start(kind, rec.ID, inputs, "", ""); err != nil {
		return err
	}
	artifacts := make(map[string]string, len(rec.Artifacts))
	for role, artifact := range rec.Artifacts {
		artifacts[role] = artifact.Path
	}
	detail := fmt.Sprintf("reused from %s seq %d", r.prior.runID, rec.Seq)
	completed, err := l.Complete(kind, rec.ID, rec.Outcome, artifacts, detail)
	if err != nil {
		return err
	}
	_, adopted, ok := recordArtifact(completed)
	if !ok || adopted.SHA256 != reference.Artifact.SHA256 {
		return fmt.Errorf("%s: %s/%s: %s changed while it was being adopted",
			r.op, kind, rec.ID, reference.Artifact.Path)
	}
	return nil
}

// reference is the provenance an adopted record carries: which run, commit,
// document and record produced it, the original artifact by path and digest,
// and the basis it was judged on. A record the prior run itself adopted was
// produced by an earlier run still, and the artifact says so: the producer is
// copied from the reference that record carries, while the basis names the
// prior run's commit, which is what this run compared against.
func (r *reuseRun) reference(
	rec *runstate.Record, rows []string, ownGroup string, contextFiles int, policy, toolchain string,
) *evidence.ReuseReference {
	_, artifact, _ := recordArtifact(rec)
	identity := r.prior.identity()
	reference := &evidence.ReuseReference{
		PriorRunID:        r.prior.runID,
		PriorRepoCommit:   identity.RepoCommit,
		PriorSourceSHA256: identity.SourceSHA256,
		PriorSeq:          rec.Seq,
		Outcome:           rec.Outcome,
		Artifact:          evidence.FileRef{Path: artifact.Path, SHA256: artifact.SHA256},
		Basis: evidence.ReuseBasis{
			PriorCommit:  identity.RepoCommit,
			ChangedPaths: r.changes.basisPaths(ownGroup),
			ClaimRows:    append([]string{}, rows...),
			ContextFiles: contextFiles,
			Policy:       policy,
			Toolchain:    toolchain,
		},
	}
	if producer := recordReuse(rec); producer != nil {
		reference.PriorRunID = producer.PriorRunID
		reference.PriorRepoCommit = producer.PriorRepoCommit
		reference.PriorSourceSHA256 = producer.PriorSourceSHA256
		reference.PriorSeq = producer.PriorSeq
		reference.Artifact = producer.Artifact
	}
	return reference
}

// firstIncomplete names the first reason a manifest gives for being incomplete,
// in sorted order, empty when it is complete.
func firstIncomplete(manifest *evidence.Manifest) string {
	reasons := slices.Clone(manifest.Incomplete)
	slices.Sort(reasons)
	if len(reasons) == 0 {
		return ""
	}
	return reasons[0]
}

// groupedElsewhere names the first of rows that already sits in another current
// group of this run's ledger: adopting the prior group would put a row in two
// groups at once, and the run could not say which verdict answers for it.
func groupedElsewhere(l *runstate.Ledger, groupID string, rows []string) string {
	for i := range l.Records {
		rec := &l.Records[i]
		if rec.Kind != runstate.KindReviewGroup || rec.Status == runstate.StatusSuperseded || rec.ID == groupID {
			continue
		}
		var inputs reviewGroupInputs
		if err := recordInputs(rec, &inputs); err != nil {
			continue
		}
		for _, row := range rows {
			if slices.Contains(inputs.Rows, row) {
				return row
			}
		}
	}
	return ""
}

// reuseCountsOf tallies what the op did: what it adopted, how much of that was
// a failed outcome the run still owes an answer for, and what it refused.
func reuseCountsOf(decisions ...[]reuseDecision) reuseCounts {
	var counts reuseCounts
	for _, group := range decisions {
		for _, decision := range group {
			if !decision.Reused {
				counts.Refused++
				continue
			}
			counts.Reused++
			if runstate.IsFailedOutcome(decision.Kind, decision.Outcome) {
				counts.ReusedFailed++
			}
		}
	}
	return counts
}
