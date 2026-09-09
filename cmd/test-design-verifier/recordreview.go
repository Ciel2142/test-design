package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
	"test-design/internal/runstate"
)

const (
	reviewStatusStarted   = "started"
	reviewStatusCompleted = "completed"
)

// reviewGroupIDPattern keeps a group id a plain name: it becomes a file name
// inside the ledger directory, so a separator or a traversal in it would place
// the group's evidence somewhere the run does not own.
var reviewGroupIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// reviewVerdict is one critic's judgement of one row.
type reviewVerdict struct {
	RowID   string `json:"rowId"`
	Verdict string `json:"verdict"`
	Reason  string `json:"reason"`
}

type recordReviewRequest struct {
	verifyRequest
	GroupID  string          `json:"groupId"`
	Status   string          `json:"status"`             // started | completed
	Rows     []string        `json:"rows,omitempty"`     // started
	Verdicts []reviewVerdict `json:"verdicts,omitempty"` // completed
}

// reviewGroupInputs is what a group was judged against: its rows, the source
// hashes each row was handed — the primary first, then every contributor — and
// the digest of the dispatch inputs document those came from.
type reviewGroupInputs struct {
	Rows                 []string            `json:"rows"`
	Sources              map[string][]string `json:"sources"`
	DispatchInputsSHA256 string              `json:"dispatchInputsSha256"`
}

type reviewEvidence struct {
	evidence.Envelope
	RunID        string            `json:"runId"`
	RepoCommit   string            `json:"repoCommit"`
	Source       string            `json:"source"`
	SourceSHA256 string            `json:"sourceSha256"`
	GroupID      string            `json:"groupId"`
	Rows         []string          `json:"rows"`
	Inputs       reviewGroupInputs `json:"inputs"`
	Verdicts     []reviewVerdict   `json:"verdicts"`
}

type recordReviewOutput struct {
	evidence.Envelope
	RunID        string `json:"runId"`
	GroupID      string `json:"groupId"`
	Status       string `json:"status"`
	Outcome      string `json:"outcome"`
	EvidencePath string `json:"evidencePath"`
}

func newRecordReviewFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("record-review", flag.ContinueOnError)
	addRequestFlag(fs)
	return fs
}

func decodeRecordReviewRequest(op string, requestBody []byte) (*recordReviewRequest, *cliError) {
	base, cliErr := decodeVerifyRequest(op, requestBody)
	if cliErr != nil {
		return nil, cliErr
	}
	var extra struct {
		GroupID  string          `json:"groupId"`
		Status   string          `json:"status"`
		Rows     []string        `json:"rows"`
		Verdicts []reviewVerdict `json:"verdicts"`
	}
	if err := json.Unmarshal(requestBody, &extra); err != nil {
		return nil, &cliError{Code: "malformed-request", Message: fmt.Sprintf("%s: decode request: %v", op, err)}
	}
	if base.RunID == "" {
		return nil, &cliError{Code: "missing-run-id", Message: op + ": runID is required"}
	}
	if extra.GroupID == "" {
		return nil, &cliError{Code: "missing-group-id", Message: op + ": groupId is required"}
	}
	if !reviewGroupIDPattern.MatchString(extra.GroupID) {
		return nil, &cliError{
			Code:    "invalid-group-id",
			Message: fmt.Sprintf("%s: group id %q is not a plain name of letters, digits, dot, dash or underscore", op, extra.GroupID),
		}
	}
	if extra.Status != reviewStatusStarted && extra.Status != reviewStatusCompleted {
		return nil, &cliError{
			Code:    "invalid-status",
			Message: fmt.Sprintf("%s: status %q is neither %s nor %s", op, extra.Status, reviewStatusStarted, reviewStatusCompleted),
		}
	}
	return &recordReviewRequest{
		verifyRequest: *base, GroupID: extra.GroupID, Status: extra.Status,
		Rows: extra.Rows, Verdicts: extra.Verdicts,
	}, nil
}

// recordReviewHandler records what the parent's critics were asked and what
// they answered. Starting a group fixes the sources it will be judged against;
// completing it writes the group's evidence and closes the obligation. The op
// never judges a row itself — it refuses anything it cannot attribute to the
// group's own rows and the trace those rows were dispatched from. Every check
// that reads the ledger and the transition it guards run inside one exclusive
// ledger transaction, so two parents working the same run at once cannot each
// pass a check the other's write has already falsified.
func recordReviewHandler(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError {
	const op = "record-review"

	req, cliErr := decodeRecordReviewRequest(op, requestBody)
	if cliErr != nil {
		return cliErr
	}
	src, cliErr := loadVerifiedSource(op, req.RepoRoot, req.CoverageSource)
	if cliErr != nil {
		return cliErr
	}
	if req.Status == reviewStatusStarted {
		if cliErr := checkGroupRows(op, req.Rows, src); cliErr != nil {
			return cliErr
		}
	}

	repoCommit, cliErr := headCommit(op, req.RepoRoot)
	if cliErr != nil {
		return cliErr
	}
	store, cliErr := openLedger(op, &req.verifyRequest)
	if cliErr != nil {
		return cliErr
	}
	identity := ledgerIdentity(&req.verifyRequest, req.RepoRoot, repoCommit, src)

	if req.Status == reviewStatusStarted {
		return recordReviewStarted(op, req, store, identity, stdout)
	}
	return recordReviewCompleted(op, req, src, repoCommit, store, identity, stdout)
}

// recordReviewStarted opens the group. The check that no current group already
// claims one of its rows, the dispatch inputs it is judged against and the
// Start that claims the rows are decided over one ledger, under the exclusive
// lock: ten parents opening ten groups over one row at once leave the row in
// exactly one of them, the other nine refused row-already-grouped.
func recordReviewStarted(op string, req *recordReviewRequest, store *runstate.Store, identity runstate.Identity, stdout io.Writer) *cliError {
	const kind = runstate.KindReviewGroup
	err := store.Update(identity, func(l *runstate.Ledger) error {
		if cliErr := checkRowsUngrouped(op, req, l); cliErr != nil {
			return cliErr
		}
		dispatch, digest, cliErr := currentDispatch(op, l)
		if cliErr != nil {
			return cliErr
		}
		inputs, ok := reviewGroupInputsFor(dispatch, req.Rows, digest)
		if !ok {
			return &cliError{
				Code:    "unknown-row",
				Message: fmt.Sprintf("%s: the trace's dispatch inputs name no such row among %v", op, req.Rows),
			}
		}
		_, err := l.Start(kind, req.GroupID, inputs, "", repeatReason(&req.verifyRequest))
		return err
	})
	if cliErr := updateError(op, kind, err); cliErr != nil {
		return cliErr
	}
	return writeStdout(stdout, recordReviewOutput{
		Envelope: evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:    req.RunID, GroupID: req.GroupID, Status: reviewStatusStarted,
	})
}

// recordReviewCompleted closes the group. The evidence is written after the
// group is known to be started and before its record is completed, all under
// the one exclusive lock, so the digest the record carries is of the bytes the
// outcome was computed from; a second completer arriving meanwhile sees a
// completed group and writes nothing.
func recordReviewCompleted(
	op string, req *recordReviewRequest, src *coverage.Source, repoCommit string,
	store *runstate.Store, identity runstate.Identity, stdout io.Writer,
) *cliError {
	const kind = runstate.KindReviewGroup
	var out recordReviewOutput
	err := store.Update(identity, func(l *runstate.Ledger) error {
		dispatch, digest, cliErr := currentDispatch(op, l)
		if cliErr != nil {
			return cliErr
		}
		current := runstate.Current(l, kind, req.GroupID)
		switch {
		case current == nil:
			return &cliError{
				Code:    "review-group-not-started",
				Message: fmt.Sprintf("%s: group %s was never started for this run", op, req.GroupID),
			}
		case current.Status == runstate.StatusCompleted:
			return &cliError{
				Code:    "duplicate-completion",
				Message: fmt.Sprintf("%s: group %s is already completed; the first result stands", op, req.GroupID),
			}
		}

		var recorded reviewGroupInputs
		if err := recordInputs(current, &recorded); err != nil {
			return &cliError{Code: "ledger-invalid", Message: fmt.Sprintf("%s: %v", op, err)}
		}
		if cliErr := checkVerdicts(op, req.GroupID, req.Verdicts, recorded.Rows); cliErr != nil {
			return cliErr
		}
		inputs, ok := reviewGroupInputsFor(dispatch, recorded.Rows, digest)
		if !ok || !groupInputsMatch(recorded, inputs) {
			return &cliError{
				Code: "dispatch-inputs-changed",
				Message: fmt.Sprintf("%s: group %s was judged against dispatch inputs the run no longer hands out; regroup against the current trace",
					op, req.GroupID),
			}
		}

		verdicts := req.Verdicts
		if verdicts == nil {
			verdicts = []reviewVerdict{}
		}
		doc := reviewEvidence{
			Envelope:     evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
			RunID:        req.RunID,
			RepoCommit:   repoCommit,
			Source:       src.Path,
			SourceSHA256: src.SHA256,
			GroupID:      req.GroupID,
			Rows:         recorded.Rows,
			Inputs:       recorded,
			Verdicts:     verdicts,
		}
		path := reviewEvidencePath(store.Dir(), req.GroupID, current.Seq)
		if err := evidence.WriteJSON(path, &doc); err != nil {
			return &cliError{Code: "evidence-write-failed", Message: fmt.Sprintf("%s: %v", op, err)}
		}

		outcome := reviewOutcome(verdicts)
		if _, err := l.Complete(kind, req.GroupID, outcome, map[string]string{artifactEvidence: path}, ""); err != nil {
			return err
		}
		out = recordReviewOutput{
			Envelope: doc.Envelope, RunID: req.RunID, GroupID: req.GroupID,
			Status: reviewStatusCompleted, Outcome: outcome, EvidencePath: path,
		}
		return nil
	})
	if cliErr := updateError(op, kind, err); cliErr != nil {
		return cliErr
	}
	return writeStdout(stdout, out)
}

// reviewEvidencePath names the evidence of the group's completion after the
// started record it completes, review-<groupId>-<seq>.json: a repeated group
// gets a file of its own, so the superseded record's artifact stays the bytes
// it digested rather than a path whose content has moved on.
func reviewEvidencePath(ledgerDir, groupID string, seq int) string {
	return filepath.Join(ledgerDir, fmt.Sprintf("review-%s-%d.json", groupID, seq))
}

// checkGroupRows answers from the coverage document alone what a group may
// claim: a non-empty set of distinct active rows.
func checkGroupRows(op string, rows []string, src *coverage.Source) *cliError {
	if len(rows) == 0 {
		return &cliError{Code: "missing-rows", Message: op + ": a started group needs the rows it covers"}
	}
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		if seen[row] {
			return &cliError{Code: "duplicate-row", Message: fmt.Sprintf("%s: %s appears twice in the group", op, row)}
		}
		seen[row] = true
	}
	active := activeRowIDs(src)
	for _, row := range rows {
		if !slices.Contains(active, row) {
			return &cliError{Code: "unknown-row", Message: fmt.Sprintf("%s: %s is not an active row of %s", op, row, src.Path)}
		}
	}
	return nil
}

// reviewOutcome is holds only when every verdict does; anything else — a weak
// assertion, a vacuous one, a row no test covers — blocks the group.
func reviewOutcome(verdicts []reviewVerdict) string {
	for _, verdict := range verdicts {
		if verdict.Verdict != "HOLDS" {
			return "blocked"
		}
	}
	return "holds"
}

// reviewGroupInputsFor builds a group's input identity from the dispatch inputs
// of the current trace record: for each row the primary source first, then every
// contributor's, and the digest of the document those came from. It reports
// false when the dispatch inputs name no such row, which is the only way a group
// can have no identity to build.
func reviewGroupInputsFor(dispatch *dispatchInputs, rows []string, digest string) (reviewGroupInputs, bool) {
	inputs := reviewGroupInputs{
		Rows:                 append([]string{}, rows...),
		Sources:              map[string][]string{},
		DispatchInputsSHA256: digest,
	}
	if dispatch == nil {
		return reviewGroupInputs{}, false
	}
	byID := make(map[string]dispatchRow, len(dispatch.Rows))
	for _, row := range dispatch.Rows {
		byID[row.RowID] = row
	}
	for _, id := range rows {
		row, ok := byID[id]
		if !ok {
			return reviewGroupInputs{}, false
		}
		// The primary source comes first, then every contributor's that is not
		// already listed: the dispatch document repeats the primary among the
		// contributors, and a list that carried it twice would describe a set of
		// sources no critic was handed.
		hashes := []string{}
		listed := map[string]bool{}
		for _, hash := range append([]string{row.TestSourceSHA256}, contributorHashes(row)...) {
			if hash == "" || listed[hash] {
				continue
			}
			listed[hash] = true
			hashes = append(hashes, hash)
		}
		inputs.Sources[id] = hashes
	}
	return inputs, true
}

// contributorHashes names the source of every bound site crediting a row, in
// the order the dispatch document lists them.
func contributorHashes(row dispatchRow) []string {
	hashes := make([]string, 0, len(row.Contributors))
	for _, contributor := range row.Contributors {
		hashes = append(hashes, contributor.TestSourceSHA256)
	}
	return hashes
}

// currentDispatch reads the dispatch inputs of the run's current trace record.
// Every review obligation is defined against them, so a run whose trace has not
// completed — or whose evidence no longer verifies — has no group to record.
func currentDispatch(op string, ledger *runstate.Ledger) (*dispatchInputs, string, *cliError) {
	incomplete := func(detail string) *cliError {
		return &cliError{
			Code:    "trace-incomplete",
			Message: fmt.Sprintf("%s: %s; run trace for this run ID before recording a review", op, detail),
		}
	}
	rec := runstate.Current(ledger, runstate.KindPhase, tracePhaseID)
	switch {
	case rec == nil:
		return nil, "", incomplete("this run has no trace phase")
	case rec.Status != runstate.StatusCompleted:
		return nil, "", incomplete("the trace phase is " + string(rec.Status))
	}
	if reasons := runstate.Verify(rec); len(reasons) > 0 {
		return nil, "", incomplete("the trace evidence no longer verifies: " + strings.Join(reasons, ", "))
	}
	artifact := rec.Artifacts[artifactDispatchInputs]
	dispatch, err := loadDispatchInputs(artifact.Path)
	if err != nil {
		return nil, "", incomplete(err.Error())
	}
	return dispatch, artifact.SHA256, nil
}

// checkRowsUngrouped keeps a row in one current group: two groups claiming one
// row would let two critics answer for it, and the run could not say which
// answer stands.
func checkRowsUngrouped(op string, req *recordReviewRequest, ledger *runstate.Ledger) *cliError {
	if ledger == nil {
		return nil
	}
	for i := range ledger.Records {
		rec := &ledger.Records[i]
		if rec.Kind != runstate.KindReviewGroup || rec.Status == runstate.StatusSuperseded || rec.ID == req.GroupID {
			continue
		}
		var inputs reviewGroupInputs
		if err := recordInputs(rec, &inputs); err != nil {
			continue
		}
		for _, row := range req.Rows {
			if slices.Contains(inputs.Rows, row) {
				return &cliError{
					Code:    "row-already-grouped",
					Message: fmt.Sprintf("%s: %s already belongs to group %s; supersede that group or group different rows", op, row, rec.ID),
				}
			}
		}
	}
	return nil
}

// checkVerdicts holds a completion to exactly the group's rows. The supplied
// verdicts are judged first — a duplicate, a foreign row or an unknown verdict
// is a fault in what the caller sent — and only then is the group asked whether
// every row it owns was answered.
func checkVerdicts(op, groupID string, verdicts []reviewVerdict, rows []string) *cliError {
	seen := make(map[string]bool, len(verdicts))
	for _, verdict := range verdicts {
		if seen[verdict.RowID] {
			return &cliError{
				Code:    "duplicate-verdict",
				Message: fmt.Sprintf("%s: group %s carries two verdicts for %s", op, groupID, verdict.RowID),
			}
		}
		seen[verdict.RowID] = true
	}
	for _, verdict := range verdicts {
		if !slices.Contains(rows, verdict.RowID) {
			return &cliError{
				Code:    "verdict-outside-group",
				Message: fmt.Sprintf("%s: %s is not a row of group %s", op, verdict.RowID, groupID),
			}
		}
	}
	for _, verdict := range verdicts {
		if !validReviewVerdict(verdict.Verdict) {
			return &cliError{
				Code:    "invalid-verdict",
				Message: fmt.Sprintf("%s: verdict %q for %s is not one of HOLDS, WEAK, VACUOUS, MISSING", op, verdict.Verdict, verdict.RowID),
			}
		}
	}
	for _, row := range rows {
		if !seen[row] {
			return &cliError{
				Code:    "missing-verdict",
				Message: fmt.Sprintf("%s: group %s has no verdict for %s", op, groupID, row),
			}
		}
	}
	return nil
}

func validReviewVerdict(verdict string) bool {
	switch verdict {
	case "HOLDS", "WEAK", "VACUOUS", "MISSING":
		return true
	}
	return false
}
