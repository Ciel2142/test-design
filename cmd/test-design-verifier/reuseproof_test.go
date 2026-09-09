package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"test-design/internal/evidence"
	"test-design/internal/runstate"
)

// This file proves the reuse contract over the real ops rather than over the
// pieces they are built from: a subject repository, a first run that traces it
// and records real review and mutation evidence, and second runs that trace the
// subject again and ask reuse what still stands. Nothing here reaches into the
// ledger except where the proof is about a ledger written before this wave.
//
// The fixture's own rows all share test files across packages, so no grouping
// of them can show an isolated change. The subject therefore adds a shipping
// package no other row cites: one test file, one fixture file the critic reads,
// and two rows nothing else contributes to.

// The three groups the proofs partition the subject's active rows into. Every
// active row belongs to exactly one of them, so a run that adopts all three
// owes no review of its own.
var (
	proofOrdersRows = []string{"SC-01", "SC-02", "SC-03", "SC-04", "SC-08", "SC-09", "SC-10"}
	proofPayRows    = []string{"SC-05", "SC-06", "SC-07"}
	proofShipRows   = []string{"SC-11", "SC-12"}
)

const (
	// shippingTestFile is the one file the shipping group's critic is handed,
	// and shippingReadFile the one supporting file it reports reading.
	shippingTestFile = "shipping/shipping_test.go"
	shippingReadFile = "shipping/testdata/rates.txt"
	shippingSrcFile  = "shipping/shipping.go"
	// shippingNotesFile is a second supporting file the critic reports reading,
	// placed inside the coverage directory reuse exempts from path
	// classification: a change to it is never widened, so the only thing that
	// can refuse a record over it is the digest the manifest recorded.
	shippingNotesFile = "docs/superpowers/coverage/2026-09-07-shipping-rates-notes.md"

	// shippingGuardRefuses and shippingGuardAccepts are the two bodies of
	// Rate's empty-zone guard: the committed one, and the mutant that accepts
	// an empty zone. They differ in one line, so a mutant built from the
	// second is a single-hunk diff against the guard.
	shippingGuardRefuses = "\t\treturn 0, fmt.Errorf(\"shipping zone must not be empty\")"
	shippingGuardAccepts = "\t\treturn 7, nil"

	shippingRates = "eu=7\n"
	shippingNotes = "# shipping rates\n\nThe eu zone is the only published zone; its rate is 7.\n"

	shippingTestSource = `package shipping

import (
	"os"
	"strings"
	"testing"
)

// covers: SC-11
func TestRateKnownZone(t *testing.T) {
	published, err := os.ReadFile("testdata/rates.txt")
	if err != nil {
		t.Fatalf("read testdata/rates.txt: %v", err)
	}
	if !strings.Contains(string(published), "eu=7") {
		t.Fatalf("testdata/rates.txt = %q, want it to publish eu=7", published)
	}
	rate, err := Rate("eu")
	if err != nil {
		t.Fatalf("Rate(eu) = %v, want nil", err)
	}
	if rate != 7 {
		t.Fatalf("Rate(eu) = %d, want 7", rate)
	}
}

// covers: SC-12
func TestRateRejectsEmptyZone(t *testing.T) {
	if _, err := Rate(""); err == nil {
		t.Fatal("Rate(\"\") = nil, want an error")
	}
}
`

	shippingCoverageTable = `
### Behavior: shipping

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-11 | a published zone quotes its rate | Rate("eu") returns 7 | unit | eu rate returns wrong value | med |
| SC-12 | an empty shipping zone is rejected | Rate("") returns an error | unit | empty zone accepted | high |
`
)

// shippingSource is shipping.go with guard as the body of Rate's empty-zone
// branch, so the committed file and its mutant differ in that line alone.
func shippingSource(guard string) string {
	return `package shipping

import "fmt"

// Rate answers the shipping rate for zone: 7 for the one published zone, and
// an error for an empty zone or one nobody publishes a rate for.
func Rate(zone string) (int, error) {
	if zone == "" {
` + guard + `
	}
	if zone == "eu" {
		return 7, nil
	}
	return 0, fmt.Errorf("unsupported shipping zone %q", zone)
}
`
}

// reuseFixture is the subject the reuse proofs run over: the go fixture
// repository plus a shipping package no other coverage row cites, and the two
// rows that cite it. SC-11 is measured through a fixture file the critic reads
// beside the test; SC-12 is high risk, so the run owes it a mutation.
func reuseFixture(t *testing.T) string {
	t.Helper()
	return fixtureRepoWith(t, func(repo string) {
		writeRepoFile(t, repo, shippingSrcFile, shippingSource(shippingGuardRefuses))
		writeRepoFile(t, repo, shippingTestFile, shippingTestSource)
		writeRepoFile(t, repo, shippingReadFile, shippingRates)
		writeRepoFile(t, repo, shippingNotesFile, shippingNotes)
		rewriteFixtureDoc(t, repo, func(doc string) string { return doc + shippingCoverageTable })
	})
}

// completeGroupWith opens groupID under policy and closes it with one sound
// verdict per row and the critic's own account of what it read, through the
// real record-review op both times.
func completeGroupWith(
	t *testing.T, repo, runID, groupID string,
	dispatch dispatchInputs, policy *reviewPolicy, reads *[]string, rows ...string,
) recordReviewOutput {
	t.Helper()
	startReviewGroupWith(t, repo, runID, groupID, policy, rows...)
	return completeReviewGroupWith(t, repo, runID, groupID, dispatch, reads, rows...)
}

// proofMutateShippingRate rewrites the prepared worktree's shipping.go so
// Rate's empty-zone guard carries guard.
func proofMutateShippingRate(t *testing.T, worktreePath, guard string) {
	t.Helper()
	writeRepoFile(t, worktreePath, shippingSrcFile, shippingSource(guard))
}

// proofCaughtRow drives one high-risk row through the real prepare-mutation and
// evaluate-mutation ops with a mutant its bound test catches, leaving the row
// completed against evidence the run wrote.
func proofCaughtRow(t *testing.T, repo, runID, rowID, site string, mutate func(t *testing.T, worktreePath string)) {
	t.Helper()
	prep := preparedMutation(t, repo, runID, rowID)
	mutate(t, prep.WorktreePath)
	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, runID, prep.RunDir, site, true))
	if code != exitOK {
		t.Fatalf("evaluate-mutation %s exit code = %d, want %d; stderr=%s", rowID, code, exitOK, stderr)
	}
	var out evaluateMutationOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("evaluate-mutation stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if out.Outcome != "caught" {
		t.Fatalf("%s outcome = %q, want caught; the mutant must break the row's bound test", rowID, out.Outcome)
	}
}

// proofResetTo puts the subject back on commit, discarding everything a
// subtest committed or left behind, so the next subtest starts where the prior
// run's evidence was recorded.
func proofResetTo(t *testing.T, repo, commit string) {
	t.Helper()
	mustGit(t, repo, "reset", "--hard", "-q", commit)
	mustGit(t, repo, "clean", "-fdq")
}

// proofTraceAndReuse traces a fresh run over the subject as it is now and asks
// it to stand on priorRunID, holding the two obligations reuse never adopts.
func proofTraceAndReuse(t *testing.T, repo, runID, priorRunID string, policy *reviewPolicy) reuseOutput {
	t.Helper()
	tracedDispatch(t, repo, runID)
	out := runReuse(t, repo, runID, priorRunID, policy)
	if out.Trace.Reused || out.Trace.Reason != reasonFreshRequired {
		t.Errorf("trace = %+v, want it never adopted, reason %s", out.Trace, reasonFreshRequired)
	}
	if out.Live.Reused || out.Live.Reason != reasonFreshRequired {
		t.Errorf("live = %+v, want it never adopted, reason %s", out.Live, reasonFreshRequired)
	}
	return out
}

// proofRefused holds a refused decision to its reason, to carrying no
// provenance, and to not claiming to have been reused.
func proofRefused(t *testing.T, label string, got reuseDecision, want string) {
	t.Helper()
	if got.Reused || got.Reason != want || got.Reference != nil {
		t.Errorf("%s = %+v, want it refused %q with no reference", label, got, want)
	}
}

// proofReused holds an adopted decision to carrying the prior outcome and the
// provenance of what it stood on.
func proofReused(t *testing.T, label string, got reuseDecision, wantOutcome string) {
	t.Helper()
	if !got.Reused || got.Reason != "" || got.Reference == nil {
		t.Fatalf("%s = %+v, want it reused with a reference and no reason", label, got)
	}
	if got.Outcome != wantOutcome {
		t.Errorf("%s outcome = %q, want the prior run's %q", label, got.Outcome, wantOutcome)
	}
}

// proofDispatchedRows names the active rows in the order the run's trace
// dispatched them, which is the order resume reports uncovered rows in.
func proofDispatchedRows(dispatch dispatchInputs, except ...string) []string {
	rows := []string{}
	for _, row := range dispatch.Rows {
		if !slices.Contains(except, row.RowID) {
			rows = append(rows, row.RowID)
		}
	}
	return rows
}

// proofEveryRecordRefused holds a widening change to what widening means: every
// prior record of the subject — its three review groups and its three mutation
// rows — refused with the one reason naming the path. The count is asserted
// before the reasons, so a reuse that emitted no decisions at all fails here
// rather than passing an empty loop.
func proofEveryRecordRefused(t *testing.T, out reuseOutput, want string) {
	t.Helper()
	if len(out.Review) != 3 || len(out.Mutation) != 3 {
		t.Fatalf("decisions = %d review, %d mutation, want the subject's 3 and 3",
			len(out.Review), len(out.Mutation))
	}
	for _, decision := range append(slices.Clone(out.Review), out.Mutation...) {
		proofRefused(t, string(decision.Kind)+"/"+decision.ID, decision, want)
	}
	if got := (reuseCounts{Refused: 6}); out.Counts != got {
		t.Errorf("counts = %+v, want %+v: widening adopts nothing", out.Counts, got)
	}
}

// proofPendingIs holds resume's pending list to exactly want, in any order.
func proofPendingIs(t *testing.T, label string, got, want []resumePending) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s pending = %+v, want exactly %+v", label, got, want)
		return
	}
	for _, entry := range want {
		if !slices.Contains(got, entry) {
			t.Errorf("%s pending = %+v, want %+v among them", label, got, entry)
		}
	}
}

// TestReuse_MatchesCompleteInputsBeforeReuse is the whole of what a manifest is
// for. One run records a group under a named critic policy with the fixture
// file its critic read, and a mutation row measured under a named toolchain. A
// second run at the same commit adopts both, with a basis naming what it
// checked. Then, one at a time, each input the verdict rested on moves — the
// row's approved claim, the file the critic read, the production it reviewed,
// the build, the runner, the toolchain, the critic's own instructions — and the
// record that depended on it is refused by name while the one that did not is
// still adopted. A group whose critic reads were never recorded, or that read
// something outside the repository, is refused even where nothing moved at all.
// covers: SC-25
func TestReuse_MatchesCompleteInputsBeforeReuse(t *testing.T) {
	repo := reuseFixture(t)
	policy := reusePolicy(t)
	const priorRunID = "reuse-proof-inputs-prior"

	priorDispatch := tracedDispatch(t, repo, priorRunID)

	started := startReviewGroupWith(t, repo, priorRunID, "G-ship", policy, proofShipRows...)
	wantBundle := []reviewBundleFile{{
		Path:   shippingTestFile,
		SHA256: proofDigest(t, filepath.Join(repo, filepath.FromSlash(shippingTestFile))),
		Rows:   proofShipRows,
	}}
	if len(started.Bundle) != 1 || started.Bundle[0].Path != wantBundle[0].Path ||
		started.Bundle[0].SHA256 != wantBundle[0].SHA256 || !slices.Equal(started.Bundle[0].Rows, wantBundle[0].Rows) {
		t.Errorf("started bundle = %+v, want %+v", started.Bundle, wantBundle)
	}
	if started.ManifestComplete {
		t.Error("started manifestComplete = true, want false: a critic's reads are not known until it answers")
	}
	if !slices.Equal(started.ManifestIncomplete, []string{"reads-unrecorded"}) {
		t.Errorf("started manifestIncomplete = %v, want [reads-unrecorded]", started.ManifestIncomplete)
	}

	completed := completeReviewGroupWith(t, repo, priorRunID, "G-ship", priorDispatch,
		&[]string{shippingReadFile, shippingNotesFile}, proofShipRows...)
	if !completed.ManifestComplete || len(completed.ManifestIncomplete) != 0 {
		t.Fatalf("completed manifest = %v/%v, want it complete with no reasons",
			completed.ManifestComplete, completed.ManifestIncomplete)
	}
	proofAssertShipManifest(t, repo, completed.EvidencePath, policy)

	completeGroupWith(t, repo, priorRunID, "G-orders", priorDispatch, policy,
		&[]string{"/tmp/outside.txt"}, proofOrdersRows...)
	completeGroupWith(t, repo, priorRunID, "G-pay", priorDispatch, policy, nil, proofPayRows...)
	proofCaughtRow(t, repo, priorRunID, "SC-12", "site-a", func(t *testing.T, worktreePath string) {
		proofMutateShippingRate(t, worktreePath, shippingGuardAccepts)
	})

	base := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))

	t.Run("unchanged", func(t *testing.T) {
		const runID = "reuse-proof-inputs-unchanged"
		t.Cleanup(func() { proofResetTo(t, repo, base) })

		out := proofTraceAndReuse(t, repo, runID, priorRunID, policy)

		if len(out.ChangedPaths) != 0 {
			t.Errorf("changedPaths = %+v, want none at the commit the prior run was made on", out.ChangedPaths)
		}
		group := reuseDecisionFor(t, out.Review, "G-ship")
		proofReused(t, "G-ship", group, "holds")
		basis := group.Reference.Basis
		if basis.PriorCommit != base || len(basis.ChangedPaths) != 0 {
			t.Errorf("G-ship basis = %+v, want the prior commit and no changed path", basis)
		}
		if !slices.Equal(basis.ClaimRows, proofShipRows) || basis.ContextFiles != 3 {
			t.Errorf("G-ship basis = %+v, want its two rows and its three context files", basis)
		}
		if basis.Policy != basisMatched || basis.Toolchain != basisNotApplicable {
			t.Errorf("G-ship basis = %+v, want policy matched and toolchain not-applicable", basis)
		}

		row := reuseDecisionFor(t, out.Mutation, "SC-12")
		proofReused(t, "SC-12", row, "caught")
		if got := row.Reference.Basis; got.Policy != basisNotApplicable || got.Toolchain != basisMatched {
			t.Errorf("SC-12 basis = %+v, want policy not-applicable and toolchain matched", got)
		}

		proofRefused(t, "G-orders", reuseDecisionFor(t, out.Review, "G-orders"),
			"manifest-incomplete:unidentifiable-read:/tmp/outside.txt")
		proofRefused(t, "G-pay", reuseDecisionFor(t, out.Review, "G-pay"),
			"manifest-incomplete:reads-unrecorded")
		if want := (reuseCounts{Reused: 2, ReusedFailed: 0, Refused: 2}); out.Counts != want {
			t.Errorf("counts = %+v, want %+v", out.Counts, want)
		}

		resumed := runResume(t, repo, runID)
		if got := resumeGroupFor(t, resumed, "G-ship"); got.Status != string(runstate.StatusCompleted) || got.Reason != "" {
			t.Errorf("resumed G-ship = %+v, want it completed and valid", got)
		}
		if got := resumeRow(t, resumed, "SC-12"); got.Status != string(runstate.StatusCompleted) || got.Reason != "" {
			t.Errorf("resumed SC-12 = %+v, want it completed and valid", got)
		}
		want := proofDispatchedRows(proofTracedDispatch(t, resumed), proofShipRows...)
		if !slices.Equal(resumed.Review.Uncovered, want) {
			t.Errorf("uncovered = %v, want the rows of the two refused groups %v", resumed.Review.Uncovered, want)
		}
		// The adopted group and the adopted row are never offered again, and
		// what is offered is exactly the two refused groups' rows and the two
		// high-risk rows this run has no mutation record for.
		proofPendingIs(t, "unchanged", resumed.Pending, []resumePending{
			{Kind: runstate.KindReviewGroup, Reason: "uncovered: " + strings.Join(want, ", ")},
			{Kind: runstate.KindMutationRow, ID: "SC-10", Reason: reasonNeverStarted},
			{Kind: runstate.KindMutationRow, ID: "SC-07", Reason: reasonNeverStarted},
		})
	})

	otherPolicy := &reviewPolicy{
		Critic:       "coverage-critic",
		Instructions: []string{writePolicyFile(t, "critic.md", "judge the name, not the assertion\n")},
	}

	cases := []struct {
		name       string
		prepare    func(t *testing.T, repo string)
		policy     *reviewPolicy
		noPolicy   bool
		shipReason string // empty means the group is still adopted
		rowReason  string // empty means the mutation row is still adopted
		extra      func(t *testing.T, out reuseOutput)
	}{
		{
			name: "claim",
			prepare: func(t *testing.T, repo string) {
				rewriteFixtureDoc(t, repo, func(doc string) string {
					return strings.ReplaceAll(doc, `| Rate("eu") returns 7 |`, `| Rate("eu") returns the published eu rate |`)
				})
				commitAll(t, repo, "reword what SC-11 expects")
			},
			shipReason: "claim-changed:SC-11",
			extra: func(t *testing.T, out reuseOutput) {
				if len(out.ChangedPaths) != 1 || out.ChangedPaths[0].Path != fixtureCoverageSource ||
					out.ChangedPaths[0].Class != changeExempt {
					t.Errorf("changedPaths = %+v, want only %s, classified exempt", out.ChangedPaths, fixtureCoverageSource)
				}
			},
		},
		{
			name: "read",
			prepare: func(t *testing.T, repo string) {
				appendToRepoFile(t, repo, shippingReadFile, "# a note appended after the review\n")
				commitAll(t, repo, "edit the fixture file the critic read")
			},
			shipReason: "module-changed:" + shippingReadFile,
			rowReason:  "module-changed:" + shippingReadFile,
		},
		{
			// A read inside the exempt coverage directory is never widened, so
			// the refusal here comes from the recorded digest and nothing else.
			name: "exempt-read",
			prepare: func(t *testing.T, repo string) {
				appendToRepoFile(t, repo, shippingNotesFile, "\nA note appended after the review.\n")
				commitAll(t, repo, "edit the note the critic read inside the coverage directory")
			},
			shipReason: "context-changed:" + shippingNotesFile,
			rowReason:  "context-changed:" + shippingNotesFile,
			extra: func(t *testing.T, out reuseOutput) {
				if len(out.ChangedPaths) != 1 || out.ChangedPaths[0].Path != shippingNotesFile ||
					out.ChangedPaths[0].Class != changeExempt {
					t.Errorf("changedPaths = %+v, want only %s, classified exempt", out.ChangedPaths, shippingNotesFile)
				}
			},
		},
		{
			name: "production",
			prepare: func(t *testing.T, repo string) {
				appendToRepoFile(t, repo, shippingSrcFile, "\n// a production comment nobody reviewed\n")
				commitAll(t, repo, "edit the production file the group reviewed")
			},
			shipReason: "module-changed:" + shippingSrcFile,
			rowReason:  "module-changed:" + shippingSrcFile,
		},
		{
			name: "build",
			prepare: func(t *testing.T, repo string) {
				appendToRepoFile(t, repo, "go.mod", "\n// a build comment nothing imports\n")
				commitAll(t, repo, "edit the module's build file")
			},
			shipReason: "module-changed:go.mod",
			rowReason:  "module-changed:go.mod",
		},
		{
			name: "runner",
			prepare: func(t *testing.T, repo string) {
				rewriteFixtureDoc(t, repo, func(doc string) string {
					return strings.ReplaceAll(doc,
						`["go", "test", "-count=1", "-json", "./..."]`,
						`["go", "test", "-count=1", "-json", "-v", "./..."]`)
				})
				commitAll(t, repo, "run the suite verbosely")
			},
			shipReason: "runner-changed",
			rowReason:  "runner-changed",
		},
		{
			name: "toolchain",
			prepare: func(t *testing.T, repo string) {
				pinToolchain(t, "go version go0.0 test/arch")
			},
			rowReason: "toolchain-changed",
		},
		{
			name:       "policy",
			policy:     otherPolicy,
			shipReason: "policy-changed",
		},
		{
			name:       "no-policy",
			noPolicy:   true,
			shipReason: "policy-unspecified",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() { proofResetTo(t, repo, base) })
			if tc.prepare != nil {
				tc.prepare(t, repo)
			}
			asked := policy
			switch {
			case tc.noPolicy:
				asked = nil
			case tc.policy != nil:
				asked = tc.policy
			}

			out := proofTraceAndReuse(t, repo, "reuse-proof-inputs-"+tc.name, priorRunID, asked)

			group := reuseDecisionFor(t, out.Review, "G-ship")
			if tc.shipReason == "" {
				proofReused(t, "G-ship", group, "holds")
			} else {
				proofRefused(t, "G-ship", group, tc.shipReason)
			}
			row := reuseDecisionFor(t, out.Mutation, "SC-12")
			if tc.rowReason == "" {
				proofReused(t, "SC-12", row, "caught")
			} else {
				proofRefused(t, "SC-12", row, tc.rowReason)
			}
			if tc.extra != nil {
				tc.extra(t, out)
			}
		})
	}
}

// proofAssertShipManifest holds the shipping group's retained manifest to the
// inputs its verdict rested on, each compared against what the repository and
// the coverage document say now rather than against what the op reported.
func proofAssertShipManifest(t *testing.T, repo, evidencePath string, policy *reviewPolicy) {
	t.Helper()
	manifest := readReviewEvidence(t, evidencePath).Inputs.Manifest
	if manifest == nil {
		t.Fatal("retained evidence carries no manifest")
	}
	if manifest.Version != evidence.ManifestVersion || !manifest.Complete {
		t.Errorf("manifest version/complete = %q/%v, want %q/true",
			manifest.Version, manifest.Complete, evidence.ManifestVersion)
	}

	src, cliErr := loadVerifiedSource("test", repo, fixtureCoverageSource)
	if cliErr != nil {
		t.Fatalf("loadVerifiedSource: %v", cliErr.Message)
	}
	for _, row := range src.Rows {
		if !slices.Contains(proofShipRows, row.ID) {
			continue
		}
		want := evidence.ClaimDigest(evidence.Claim{
			ID: row.ID, Behavior: row.Behavior, Expected: row.Expected,
			Level: row.Level, Mutation: row.Mutation, Risk: row.Risk,
		})
		if manifest.Claims[row.ID] != want {
			t.Errorf("claim %s = %q, want the document row's digest %q", row.ID, manifest.Claims[row.ID], want)
		}
	}
	if len(manifest.Claims) != len(proofShipRows) {
		t.Errorf("claims = %v, want exactly the group's rows %v", manifest.Claims, proofShipRows)
	}

	proofDigestsMatch(t, repo, "bundle", manifest.Bundle, shippingTestFile)
	proofDigestsMatch(t, repo, "reads", manifest.Reads, shippingReadFile, shippingNotesFile)

	runner := manifest.Runner
	if runner.Kind != src.Runner.Kind || runner.Root != src.Runner.Root ||
		!slices.Equal(runner.Command, src.Runner.Command) || !slices.Equal(runner.ReportGlobs, src.Runner.ReportGlobs) {
		t.Errorf("runner = %+v, want the document's verification block %+v", runner, src.Runner)
	}
	if got := manifest.ToolVersions["go"]; !strings.HasPrefix(got, "go version") {
		t.Errorf("toolVersions[go] = %q, want the go toolchain's own version line", got)
	}

	if manifest.Policy == nil || manifest.Policy.Critic != policy.Critic {
		t.Fatalf("policy = %+v, want the critic the parent named", manifest.Policy)
	}
	for _, path := range policy.Instructions {
		if got, want := manifest.Policy.Instructions[path], proofDigest(t, path); got != want {
			t.Errorf("policy instructions[%s] = %q, want the file's digest %q", path, got, want)
		}
	}
	if len(manifest.Policy.Instructions) != len(policy.Instructions) {
		t.Errorf("policy instructions = %v, want exactly %v", manifest.Policy.Instructions, policy.Instructions)
	}
}

// proofDigestsMatch holds one half of a manifest's context to exactly paths,
// each digested from the file as the repository holds it.
func proofDigestsMatch(t *testing.T, repo, label string, digests evidence.FileDigest, paths ...string) {
	t.Helper()
	if len(digests) != len(paths) {
		t.Errorf("%s = %v, want exactly %v", label, digests, paths)
	}
	for _, path := range paths {
		want := proofDigest(t, filepath.Join(repo, filepath.FromSlash(path)))
		if digests[path] != want {
			t.Errorf("%s[%s] = %q, want the file's digest %q", label, path, digests[path], want)
		}
	}
}

func proofDigest(t *testing.T, path string) string {
	t.Helper()
	sum, err := evidence.SHA256File(path)
	if err != nil {
		t.Fatalf("digest %s: %v", path, err)
	}
	return sum
}

// proofTracedDispatch reads back the dispatch document of a resumed run's
// trace, which is the order its rows were dispatched in.
func proofTracedDispatch(t *testing.T, out resumeOutput) dispatchInputs {
	t.Helper()
	return readDispatchInputs(t, out.Trace.DispatchPath)
}

// TestReuse_IsolatedRepairInvalidatesOnlyItsGroup proves the invalidation
// boundary is read from the recorded manifests and never guessed. A repair
// confined to the one test file a single group was handed refuses that group
// and the mutation evidence of its rows, and nothing else; resume then offers
// exactly that group's rows and its high-risk row. A change to shared
// production, or a test file no manifest names, is uncertainty about what it
// invalidates, so it widens to every record at once and resume offers every
// obligation again. Only the coverage source's own directory, where finalize
// publishes, is exempt from the question.
// covers: SC-26
func TestReuse_IsolatedRepairInvalidatesOnlyItsGroup(t *testing.T) {
	repo := reuseFixture(t)
	policy := reusePolicy(t)
	const priorRunID = "reuse-proof-boundary-prior"

	priorDispatch := tracedDispatch(t, repo, priorRunID)
	completeGroupWith(t, repo, priorRunID, "G-orders", priorDispatch, policy, &[]string{}, proofOrdersRows...)
	completeGroupWith(t, repo, priorRunID, "G-pay", priorDispatch, policy, &[]string{}, proofPayRows...)
	completeGroupWith(t, repo, priorRunID, "G-ship", priorDispatch, policy, &[]string{}, proofShipRows...)
	proofCaughtRow(t, repo, priorRunID, "SC-10", "reserve", func(t *testing.T, worktreePath string) {
		writeOrdersGoReserveGuards(t, worktreePath, reserveGuardMutated, reserveBatchGuardDefault)
	})
	proofCaughtRow(t, repo, priorRunID, "SC-12", "site-a", func(t *testing.T, worktreePath string) {
		proofMutateShippingRate(t, worktreePath, shippingGuardAccepts)
	})
	unappliableRow(t, repo, priorRunID, "SC-07")

	base := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))

	t.Run("isolated", func(t *testing.T) {
		const runID = "reuse-proof-boundary-isolated"
		t.Cleanup(func() { proofResetTo(t, repo, base) })

		appendToRepoFile(t, repo, shippingTestFile, "\n// a repair to the file, not to any declaration it holds\n")
		commitAll(t, repo, "repair the one test file a single group was handed")

		out := proofTraceAndReuse(t, repo, runID, priorRunID, policy)

		if len(out.ChangedPaths) != 1 {
			t.Fatalf("changedPaths = %+v, want only the repaired test file", out.ChangedPaths)
		}
		changed := out.ChangedPaths[0]
		if changed.Path != shippingTestFile || changed.Class != changeIsolated || !slices.Equal(changed.Groups, []string{"G-ship"}) {
			t.Errorf("changedPaths[0] = %+v, want %s isolated to G-ship", changed, shippingTestFile)
		}

		want := "isolated-change:" + shippingTestFile
		proofReused(t, "G-orders", reuseDecisionFor(t, out.Review, "G-orders"), "blocked")
		proofReused(t, "G-pay", reuseDecisionFor(t, out.Review, "G-pay"), "blocked")
		proofRefused(t, "G-ship", reuseDecisionFor(t, out.Review, "G-ship"), want)
		proofReused(t, "SC-10", reuseDecisionFor(t, out.Mutation, "SC-10"), "caught")
		proofReused(t, "SC-07", reuseDecisionFor(t, out.Mutation, "SC-07"), "unappliable")
		proofRefused(t, "SC-12", reuseDecisionFor(t, out.Mutation, "SC-12"), want)
		// Three of the four adopted records carry a failed outcome: the two
		// groups are blocked, because each holds a row the trace found no test
		// for and the critic answered MISSING, and SC-07 is unappliable. A
		// failed outcome is adopted like any other and counted, because the run
		// owes the same answer either way.
		if want := (reuseCounts{Reused: 4, ReusedFailed: 3, Refused: 2}); out.Counts != want {
			t.Errorf("counts = %+v, want %+v", out.Counts, want)
		}

		resumed := runResume(t, repo, runID)
		if !slices.Equal(resumed.Review.Uncovered, proofShipRows) {
			t.Errorf("uncovered = %v, want exactly the refused group's rows %v", resumed.Review.Uncovered, proofShipRows)
		}
		proofPendingIs(t, "isolated", resumed.Pending, []resumePending{
			{Kind: runstate.KindReviewGroup, Reason: "uncovered: " + strings.Join(proofShipRows, ", ")},
			{Kind: runstate.KindMutationRow, ID: "SC-12", Reason: reasonNeverStarted},
		})

		regroup := refuseRecordReview(t, repo, runID, map[string]any{
			"groupId": "R", "status": "started", "rows": []string{"SC-01"},
		})
		if regroup.Code != "row-already-grouped" {
			t.Errorf("record-review error code = %q, want row-already-grouped over an adopted row", regroup.Code)
		}
	})

	t.Run("production", func(t *testing.T) {
		const runID = "reuse-proof-boundary-production"
		t.Cleanup(func() { proofResetTo(t, repo, base) })

		const production = "orders/orders.go"
		appendToRepoFile(t, repo, production, "\n// a production comment nobody reviewed\n")
		commitAll(t, repo, "change production every group's rows run against")

		out := proofTraceAndReuse(t, repo, runID, priorRunID, policy)

		if len(out.ChangedPaths) != 1 || out.ChangedPaths[0].Path != production ||
			out.ChangedPaths[0].Class != changeShared {
			t.Fatalf("changedPaths = %+v, want only %s, classified shared", out.ChangedPaths, production)
		}
		proofEveryRecordRefused(t, out, "module-changed:"+production)

		resumed := runResume(t, repo, runID)
		every := proofDispatchedRows(proofTracedDispatch(t, resumed))
		proofPendingIs(t, "production", resumed.Pending, []resumePending{
			{Kind: runstate.KindReviewGroup, Reason: "uncovered: " + strings.Join(every, ", ")},
			{Kind: runstate.KindMutationRow, ID: "SC-07", Reason: reasonNeverStarted},
			{Kind: runstate.KindMutationRow, ID: "SC-10", Reason: reasonNeverStarted},
			{Kind: runstate.KindMutationRow, ID: "SC-12", Reason: reasonNeverStarted},
		})
	})

	t.Run("unknown", func(t *testing.T) {
		const runID = "reuse-proof-boundary-unknown"
		t.Cleanup(func() { proofResetTo(t, repo, base) })

		const helper = "shipping/helpers_test.go"
		writeRepoFile(t, repo, helper, "package shipping\n\n// zoneUnderTest is a helper no coverage row cites.\nfunc zoneUnderTest() string { return \"eu\" }\n")
		commitAll(t, repo, "add a test file no prior manifest names")

		out := proofTraceAndReuse(t, repo, runID, priorRunID, policy)

		if len(out.ChangedPaths) != 1 || out.ChangedPaths[0].Path != helper ||
			out.ChangedPaths[0].Class != changeUnknown {
			t.Fatalf("changedPaths = %+v, want only %s, classified unknown", out.ChangedPaths, helper)
		}
		proofEveryRecordRefused(t, out, "unknown-ownership:"+helper)
	})

	t.Run("exempt", func(t *testing.T) {
		const runID = "reuse-proof-boundary-exempt"
		t.Cleanup(func() { proofResetTo(t, repo, base) })

		const published = "docs/superpowers/coverage/2026-08-30-fixture-verification.md"
		writeRepoFile(t, repo, published, "# verification report\n")
		commitAll(t, repo, "publish a verification report beside the coverage document")

		out := proofTraceAndReuse(t, repo, runID, priorRunID, policy)

		if len(out.ChangedPaths) != 1 || out.ChangedPaths[0].Path != published ||
			out.ChangedPaths[0].Class != changeExempt {
			t.Fatalf("changedPaths = %+v, want only %s, classified exempt", out.ChangedPaths, published)
		}
		proofReused(t, "G-orders", reuseDecisionFor(t, out.Review, "G-orders"), "blocked")
		proofReused(t, "G-pay", reuseDecisionFor(t, out.Review, "G-pay"), "blocked")
		proofReused(t, "G-ship", reuseDecisionFor(t, out.Review, "G-ship"), "holds")
		proofReused(t, "SC-10", reuseDecisionFor(t, out.Mutation, "SC-10"), "caught")
		proofReused(t, "SC-12", reuseDecisionFor(t, out.Mutation, "SC-12"), "caught")
		proofReused(t, "SC-07", reuseDecisionFor(t, out.Mutation, "SC-07"), "unappliable")
		if out.Counts.Refused != 0 || out.Counts.Reused != 6 {
			t.Errorf("counts = %+v, want every prior record adopted", out.Counts)
		}
	})
}

// TestReuse_LegacyRecordsAreHistoricalNotReusable holds the line at every
// ledger written before manifests existed. Such a record is reported by name,
// never repaired, upgraded or reconstructed: the run asking is told why it must
// do the work itself, its own resume offers those rows, and the prior run's
// ledger and artifact come out of the attempt byte for byte as they went in,
// still readable by a resume of the run that wrote them.
// covers: SC-33
func TestReuse_LegacyRecordsAreHistoricalNotReusable(t *testing.T) {
	repo := reuseFixture(t)
	policy := reusePolicy(t)

	cases := []struct {
		name     string
		manifest *evidence.Manifest
		reason   string
	}{
		{name: "no-manifest", manifest: nil, reason: "legacy-no-manifest"},
		{
			name:     "unsupported-version",
			manifest: &evidence.Manifest{Version: "0", Complete: true, Incomplete: []string{}},
			reason:   "manifest-version-unsupported",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			priorRunID := "reuse-proof-legacy-" + tc.name
			runID := "reuse-proof-legacy-" + tc.name + "-new"

			ledgerPath, artifactPath := proofLegacyReviewRun(t, repo, priorRunID, tc.manifest)
			ledgerBefore := proofDigest(t, ledgerPath)
			artifactBefore := proofDigest(t, artifactPath)

			out := proofTraceAndReuse(t, repo, runID, priorRunID, policy)

			proofRefused(t, "G-ship", reuseDecisionFor(t, out.Review, "G-ship"), tc.reason)
			if out.Counts.Refused != 1 || out.Counts.Reused != 0 {
				t.Errorf("counts = %+v, want the one legacy record refused and nothing adopted", out.Counts)
			}

			resumed := runResume(t, repo, runID)
			for _, row := range proofShipRows {
				if !slices.Contains(resumed.Review.Uncovered, row) {
					t.Errorf("uncovered = %v, want %s among the rows this run must review itself",
						resumed.Review.Uncovered, row)
				}
			}

			if got := proofDigest(t, ledgerPath); got != ledgerBefore {
				t.Errorf("prior ledger digest = %q, want it unchanged at %q", got, ledgerBefore)
			}
			if got := proofDigest(t, artifactPath); got != artifactBefore {
				t.Errorf("prior artifact digest = %q, want it unchanged at %q", got, artifactBefore)
			}

			legacy := runResume(t, repo, priorRunID)
			if got := resumeGroupFor(t, legacy, "G-ship"); got.Status != string(runstate.StatusCompleted) || got.Outcome != "holds" {
				t.Errorf("legacy resume G-ship = %+v, want it still completed holds", got)
			}
		})
	}
}

// proofLegacyReviewRun leaves a run whose review record is the shape ledgers
// carried before this wave: written straight through the store, with the
// group's rows, per-row sources and dispatch digest and no manifest of its
// own, and an artifact in the same pre-wave shape. It returns the two files the
// proof holds byte-identical. A non-nil manifest stands in for a ledger written
// against a manifest contract this executable does not implement.
func proofLegacyReviewRun(t *testing.T, repo, runID string, manifest *evidence.Manifest) (ledgerPath, artifactPath string) {
	t.Helper()
	return proofHandWrittenReviewRun(t, repo, runID, manifest, func(t *testing.T, path string, doc *reviewEvidence) {
		t.Helper()
		if err := evidence.WriteJSON(path, doc); err != nil {
			t.Fatalf("write review evidence: %v", err)
		}
	})
}

// proofHandWrittenReviewRun writes one completed review record of G-ship
// straight through the store, with manifest as the record's own, and lets
// write put whatever bytes it likes at the artifact path before the record is
// completed over them: the ledger digests the bytes as they are.
func proofHandWrittenReviewRun(
	t *testing.T, repo, runID string, manifest *evidence.Manifest,
	write func(t *testing.T, path string, doc *reviewEvidence),
) (ledgerPath, artifactPath string) {
	t.Helper()
	const groupID = "G-ship"

	dispatch := tracedDispatch(t, repo, runID)
	traced := runResume(t, repo, runID)
	digest := ledgerRecord(t, traced, runstate.KindPhase, tracePhaseID).Artifacts[artifactDispatchInputs].SHA256

	inputs, ok := reviewGroupInputsFor(&dispatch, proofShipRows, digest)
	if !ok {
		t.Fatalf("the trace of %s dispatched none of %v", runID, proofShipRows)
	}
	inputs.Manifest = manifest

	req := fixtureVerifyRequest(repo, runID)
	src, cliErr := loadVerifiedSource("test", repo, fixtureCoverageSource)
	if cliErr != nil {
		t.Fatalf("loadVerifiedSource: %v", cliErr.Message)
	}
	commit, cliErr := headCommit("test", repo)
	if cliErr != nil {
		t.Fatalf("headCommit: %v", cliErr.Message)
	}
	store, cliErr := openLedger("test", req)
	if cliErr != nil {
		t.Fatalf("openLedger: %v", cliErr.Message)
	}

	verdicts := make([]reviewVerdict, 0, len(proofShipRows))
	for _, row := range proofShipRows {
		verdicts = append(verdicts, reviewVerdict{RowID: row, Verdict: "HOLDS"})
	}

	err := store.Update(ledgerIdentity(req, repo, commit, src), func(l *runstate.Ledger) error {
		rec, err := l.Start(runstate.KindReviewGroup, groupID, inputs, "", "")
		if err != nil {
			return err
		}
		artifactPath = reviewEvidencePath(store.Dir(), groupID, rec.Seq)
		doc := reviewEvidence{
			Envelope:     evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
			RunID:        runID,
			RepoCommit:   commit,
			Source:       src.Path,
			SourceSHA256: src.SHA256,
			GroupID:      groupID,
			Rows:         proofShipRows,
			Inputs:       inputs,
			Verdicts:     verdicts,
		}
		write(t, artifactPath, &doc)
		_, err = l.Complete(runstate.KindReviewGroup, groupID, "holds", map[string]string{artifactEvidence: artifactPath}, "")
		return err
	})
	if err != nil {
		t.Fatalf("write the hand-written review record: %v", err)
	}
	return filepath.Join(store.Dir(), runstate.LedgerFile), artifactPath
}

// TestReuse_RefusesAnArtifactThatVerifiesButWillNotDecode reaches the one
// reason left between a digest that holds and a manifest that can be read: the
// ledger recorded the digest of bytes that were never a document. Only a
// record written by hand can carry one, and it is refused by name.
func TestReuse_RefusesAnArtifactThatVerifiesButWillNotDecode(t *testing.T) {
	repo := reuseFixture(t)
	policy := reusePolicy(t)
	const priorRunID = "reuse-proof-undecodable-prior"
	const runID = "reuse-proof-undecodable-new"

	manifest := &evidence.Manifest{Version: evidence.ManifestVersion, Complete: true, Incomplete: []string{}}
	_, artifactPath := proofHandWrittenReviewRun(t, repo, priorRunID, manifest, func(t *testing.T, path string, _ *reviewEvidence) {
		t.Helper()
		if err := os.WriteFile(path, []byte("{not a document"), 0o644); err != nil {
			t.Fatalf("write undecodable artifact: %v", err)
		}
	})
	before := proofDigest(t, artifactPath)

	out := proofTraceAndReuse(t, repo, runID, priorRunID, policy)

	proofRefused(t, "G-ship", reuseDecisionFor(t, out.Review, "G-ship"), "artifact-unreadable:"+artifactEvidence)
	if got := proofDigest(t, artifactPath); got != before {
		t.Errorf("prior artifact digest = %q, want it unchanged at %q", got, before)
	}
}

// TestReuse_AReadoptedRowIsJudgedOnItsOwnRecordedContext follows one measured
// row through three runs. The first reviewed its group with a critic that read
// the shipping test; the second adopted the row but reviewed the group afresh
// with a critic that did not; the third edits the shipping test. That file is
// isolated to another group as the second run recorded it, but the row's own
// manifest still names it, and the row is judged on its own manifest.
func TestReuse_AReadoptedRowIsJudgedOnItsOwnRecordedContext(t *testing.T) {
	repo := reuseFixture(t)
	first := reusePolicy(t)
	second := &reviewPolicy{
		Critic:       "coverage-critic",
		Instructions: []string{writePolicyFile(t, "critic.md", "judge the name, not the assertion\n")},
	}
	const runA = "reuse-proof-own-context-a"
	const runB = "reuse-proof-own-context-b"
	const runC = "reuse-proof-own-context-c"

	dispatchA := tracedDispatch(t, repo, runA)
	completeGroupWith(t, repo, runA, "G-orders", dispatchA, first, &[]string{shippingTestFile}, proofOrdersRows...)
	completeGroupWith(t, repo, runA, "G-pay", dispatchA, first, &[]string{}, proofPayRows...)
	completeGroupWith(t, repo, runA, "G-ship", dispatchA, first, &[]string{}, proofShipRows...)
	unappliableRow(t, repo, runA, "SC-10")

	dispatchB := tracedDispatch(t, repo, runB)
	adopted := runReuse(t, repo, runB, runA, second)
	proofRefused(t, "G-orders", reuseDecisionFor(t, adopted.Review, "G-orders"), "policy-changed")
	proofReused(t, "SC-10", reuseDecisionFor(t, adopted.Mutation, "SC-10"), "unappliable")
	completeGroupWith(t, repo, runB, "G-orders", dispatchB, second, &[]string{}, proofOrdersRows...)
	completeGroupWith(t, repo, runB, "G-pay", dispatchB, second, &[]string{}, proofPayRows...)
	completeGroupWith(t, repo, runB, "G-ship", dispatchB, second, &[]string{}, proofShipRows...)

	appendToRepoFile(t, repo, shippingTestFile, "\n// a repair to the file, not to any declaration it holds\n")
	commitAll(t, repo, "repair the test file the first run's critic read")

	out := proofTraceAndReuse(t, repo, runC, runB, second)

	changed := reuseChangedPathFor(t, out, shippingTestFile)
	if changed.Class != changeIsolated || !slices.Equal(changed.Groups, []string{"G-ship"}) {
		t.Errorf("%s = %+v, want it isolated to G-ship as the second run recorded it", shippingTestFile, changed)
	}
	proofReused(t, "G-orders", reuseDecisionFor(t, out.Review, "G-orders"), "blocked")
	proofRefused(t, "SC-10", reuseDecisionFor(t, out.Mutation, "SC-10"), "context-changed:"+shippingTestFile)
}
