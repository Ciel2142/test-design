package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
	"test-design/internal/live"
	"test-design/internal/mutation"
	"test-design/internal/runstate"
)

// The status an obligation carries when the run has no record of it at all,
// and the reasons resume names for work it is still owed.
const (
	statusNone = "none"

	reasonTraceIncomplete    = "trace-incomplete"
	reasonNeverStarted       = "never-started"
	reasonNotCompleted       = "started-not-completed"
	reasonStale              = "stale"
	reasonInputsChanged      = "inputs-changed"
	reasonStateUnreadable    = "state-unreadable"
	reasonInputsUnreadable   = "inputs-unreadable"
	reasonDispatchUnreadable = "dispatch-inputs-unreadable"
)

type resumeOutput struct {
	evidence.Envelope
	RunID       string             `json:"runId"`
	LedgerPath  string             `json:"ledgerPath"`
	Initialized bool               `json:"initialized"`
	Stale       bool               `json:"stale"`
	StaleReason string             `json:"staleReason,omitempty"`
	Identity    *runstate.Identity `json:"identity,omitempty"`
	Trace       resumePhase        `json:"trace"`
	Review      resumeReview       `json:"review"`
	Live        resumeLive         `json:"live"`
	Mutation    resumeMutation     `json:"mutation"`
	Pending     []resumePending    `json:"pending"`
	Counts      resumeCounts       `json:"counts"`
	Records     []runstate.Record  `json:"records"`
}

type resumePhase struct {
	Status       string `json:"status"`
	Outcome      string `json:"outcome"`
	EvidencePath string `json:"evidencePath"`
	DispatchPath string `json:"dispatchPath"`
	RunDir       string `json:"runDir"`
	Reason       string `json:"reason"`
}

type resumeReview struct {
	Complete  bool          `json:"complete"`
	Groups    []resumeGroup `json:"groups"`
	Uncovered []string      `json:"uncovered"`
	Reason    string        `json:"reason,omitempty"`
}

type resumeGroup struct {
	ID           string   `json:"id"`
	Rows         []string `json:"rows"`
	Status       string   `json:"status"`
	Outcome      string   `json:"outcome"`
	EvidencePath string   `json:"evidencePath"`
	Reason       string   `json:"reason"`
}

type resumeLive struct {
	Required         bool   `json:"required"`
	Status           string `json:"status"`
	Outcome          string `json:"outcome"`
	EvidencePath     string `json:"evidencePath"`
	RecoveryRequired bool   `json:"recoveryRequired"`
	Reason           string `json:"reason"`
}

type resumeMutation struct {
	Complete bool                `json:"complete"`
	Rows     []resumeMutationRow `json:"rows"`
	Reason   string              `json:"reason"`
}

type resumeMutationRow struct {
	RowID           string   `json:"rowId"`
	Status          string   `json:"status"`
	Outcome         string   `json:"outcome"`
	EvidencePath    string   `json:"evidencePath"`
	RunDir          string   `json:"runDir"`
	WorktreePath    string   `json:"worktreePath"`
	CompletedSites  []string `json:"completedSites"`
	InterruptedSite string   `json:"interruptedSite"`
	Reason          string   `json:"reason"`
}

type resumePending struct {
	Kind   runstate.Kind `json:"kind"`
	ID     string        `json:"id"`
	Reason string        `json:"reason"`
}

type resumeCounts struct {
	Started    int `json:"started"`
	Completed  int `json:"completed"`
	Failed     int `json:"failed"`
	Superseded int `json:"superseded"`
	Invalid    int `json:"invalid"`
	Pending    int `json:"pending"`
}

func newResumeFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	addRequestFlag(fs)
	return fs
}

// resumeHandler answers what a run still owes. It is the one op that only
// reads: it opens no worktree, runs no suite and writes no record, so a parent
// may ask it at any point, including while another op holds the run's work.
func resumeHandler(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError {
	const op = "resume"
	req, cliErr := decodeVerifyRequest(op, requestBody)
	if cliErr != nil {
		return cliErr
	}
	if req.RunID == "" {
		return &cliError{Code: "missing-run-id", Message: op + ": runID is required"}
	}
	src, cliErr := loadVerifiedSource(op, req.RepoRoot, req.CoverageSource)
	if cliErr != nil {
		return cliErr
	}
	repoCommit, cliErr := headCommit(op, req.RepoRoot)
	if cliErr != nil {
		return cliErr
	}
	store, cliErr := openLedger(op, req)
	if cliErr != nil {
		return cliErr
	}
	ledger, cliErr := loadLedgerFor(op, runstate.KindPhase, store)
	if cliErr != nil {
		return cliErr
	}
	return writeStdout(stdout, buildResume(req, src, repoCommit, store.Dir(), ledger))
}

// buildResume derives the whole report from the checkpoint, the coverage
// document and the current commit. Nothing here consults an op's output: what
// a run owes is a function of what it recorded, which is why a resumed run
// agrees with an interrupted one.
func buildResume(req *verifyRequest, src *coverage.Source, repoCommit, ledgerDir string, ledger *runstate.Ledger) resumeOutput {
	out := resumeOutput{
		Envelope:    evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:       req.RunID,
		LedgerPath:  filepath.Join(ledgerDir, runstate.LedgerFile),
		Initialized: ledger != nil,
		Trace:       resumePhase{Status: statusNone},
		Review:      resumeReview{Groups: []resumeGroup{}, Uncovered: []string{}},
		Live:        resumeLive{Required: len(live.ApplicableRows(src)) > 0, Status: statusNone},
		Mutation:    resumeMutation{Rows: []resumeMutationRow{}},
		Pending:     []resumePending{},
		Records:     []runstate.Record{},
	}

	if ledger == nil {
		out.Review.Reason = reasonTraceIncomplete
		out.Mutation.Reason = reasonTraceIncomplete
		out.Pending = append(out.Pending, resumePending{Kind: runstate.KindPhase, ID: tracePhaseID, Reason: reasonNeverStarted})
		out.Counts.Pending = len(out.Pending)
		return out
	}

	out.Identity = &ledger.Identity
	out.Records = ledger.Records
	out.Stale, out.StaleReason = staleAgainst(ledger.Identity, repoCommit, src)

	// invalidReason names why a completed record is not reusable work: the run
	// moved out from under it, or an artifact it points at is no longer what it
	// was. It is empty for a record that still stands.
	invalidReason := func(rec *runstate.Record) string {
		if rec == nil || rec.Status != runstate.StatusCompleted {
			return ""
		}
		if out.Stale {
			return reasonStale
		}
		return strings.Join(runstate.Verify(rec), ", ")
	}

	traceRec := runstate.Current(ledger, runstate.KindPhase, tracePhaseID)
	if traceRec != nil {
		var inputs phaseInputs
		inputsErr := recordInputs(traceRec, &inputs)
		out.Trace = resumePhase{
			Status:       string(traceRec.Status),
			Outcome:      traceRec.Outcome,
			EvidencePath: traceRec.Artifacts[artifactEvidence].Path,
			DispatchPath: traceRec.Artifacts[artifactDispatchInputs].Path,
			RunDir:       inputs.RunDir,
			Reason:       invalidReason(traceRec),
		}
		// A record whose own inputs cannot be read is a damaged checkpoint, not
		// work to resume from; it says so rather than reporting a phase that
		// ran nowhere.
		if inputsErr != nil && out.Trace.Reason == "" {
			out.Trace.Reason = reasonInputsUnreadable
		}
	}

	// The dispatch inputs of the current trace record are what the later two
	// layers are measured against: which rows are active, which of them are
	// grouped, and which owe a mutation. Without them there is nothing to
	// resume but the trace itself.
	var dispatch *dispatchInputs
	traceComplete := traceRec != nil && traceRec.Status == runstate.StatusCompleted && out.Trace.Reason == ""
	if traceComplete {
		payload, err := loadDispatchInputs(out.Trace.DispatchPath)
		if err != nil {
			traceComplete = false
			out.Trace.Reason = reasonDispatchUnreadable
		} else {
			dispatch = payload
		}
	}

	liveRec := runstate.Current(ledger, runstate.KindLiveAttempt, liveAttemptID)
	if liveRec != nil {
		out.Live.Status = string(liveRec.Status)
		out.Live.Outcome = liveRec.Outcome
		out.Live.EvidencePath = recordEvidencePath(liveRec)
		out.Live.RecoveryRequired = liveRec.Status == runstate.StatusStarted
		out.Live.Reason = invalidReason(liveRec)
	}

	if !traceComplete {
		out.Review.Reason = reasonTraceIncomplete
		out.Mutation.Reason = reasonTraceIncomplete
		out.Pending = append(out.Pending, resumePending{
			Kind: runstate.KindPhase, ID: tracePhaseID, Reason: phasePendingReason(traceRec, out.Trace.Reason),
		})
		out.Counts = countRecords(ledger, invalidReason, nil, len(out.Pending))
		return out
	}

	currentDigest := traceRec.Artifacts[artifactDispatchInputs].SHA256
	grouped, groupInvalid := resumeGroups(ledger, dispatch, currentDigest, invalidReason, &out)

	out.Review.Uncovered = uncoveredRows(dispatch, grouped)
	out.Review.Complete = len(out.Review.Uncovered) == 0 && everyGroupSettled(out.Review.Groups)

	required, rowRecords := resumeMutationRows(ledger, dispatch, invalidReason, &out)
	out.Mutation.Complete = mutationComplete(required, rowRecords, invalidReason)

	out.Pending = pendingObligations(ledger, out, required, rowRecords, groupInvalid, invalidReason)
	out.Counts = countRecords(ledger, invalidReason, groupInvalid, len(out.Pending))
	return out
}

// staleAgainst reports whether the run's inputs moved since the ledger was
// opened. A stale ledger invalidates every completed record at once: work done
// against a different commit or a different coverage document is not this run's.
func staleAgainst(id runstate.Identity, repoCommit string, src *coverage.Source) (bool, string) {
	if id.RepoCommit != repoCommit {
		return true, fmt.Sprintf("HEAD is %s; the ledger was opened at %s", repoCommit, id.RepoCommit)
	}
	if id.SourceSHA256 != src.SHA256 {
		return true, fmt.Sprintf("%s digest is %s; the ledger was opened at %s", src.Path, src.SHA256, id.SourceSHA256)
	}
	return false, ""
}

// phasePendingReason names why the trace phase is still owed.
func phasePendingReason(rec *runstate.Record, invalid string) string {
	switch {
	case rec == nil:
		return reasonNeverStarted
	case rec.Status == runstate.StatusStarted:
		return reasonNotCompleted
	default:
		// A completed phase is owed again only when it stopped standing, and
		// invalid carries why.
		return invalid
	}
}

// resumeGroups reports every current review group, in ledger order, and the row
// each one claims. A group whose recorded inputs no longer match the current
// trace record's is invalid: it judged sources the run no longer hands out.
func resumeGroups(
	ledger *runstate.Ledger,
	dispatch *dispatchInputs,
	currentDigest string,
	invalidReason func(*runstate.Record) string,
	out *resumeOutput,
) (grouped map[string]string, groupInvalid map[string]string) {
	grouped = map[string]string{}
	groupInvalid = map[string]string{}
	for i := range ledger.Records {
		rec := &ledger.Records[i]
		if rec.Kind != runstate.KindReviewGroup || rec.Status == runstate.StatusSuperseded {
			continue
		}
		var recorded reviewGroupInputs
		inputsErr := recordInputs(rec, &recorded)
		group := resumeGroup{
			ID:           rec.ID,
			Rows:         recorded.Rows,
			Status:       string(rec.Status),
			Outcome:      rec.Outcome,
			EvidencePath: rec.Artifacts[artifactEvidence].Path,
			Reason:       invalidReason(rec),
		}
		if group.Rows == nil {
			group.Rows = []string{}
		}
		if inputsErr != nil && group.Reason == "" {
			group.Reason = reasonInputsUnreadable
		}
		if current, ok := reviewGroupInputsFor(dispatch, recorded.Rows, currentDigest); !ok || !groupInputsMatch(recorded, current) {
			groupInvalid[rec.ID] = reasonInputsChanged
			if group.Reason == "" {
				group.Reason = reasonInputsChanged
			}
		}
		out.Review.Groups = append(out.Review.Groups, group)
		for _, row := range recorded.Rows {
			grouped[row] = rec.ID
		}
	}
	return grouped, groupInvalid
}

// uncoveredRows names the active rows of the current dispatch inputs that no
// current group claims, in dispatch order.
func uncoveredRows(dispatch *dispatchInputs, grouped map[string]string) []string {
	uncovered := []string{}
	for _, row := range dispatch.Rows {
		if _, ok := grouped[row.RowID]; !ok {
			uncovered = append(uncovered, row.RowID)
		}
	}
	return uncovered
}

func everyGroupSettled(groups []resumeGroup) bool {
	for _, group := range groups {
		if group.Status != string(runstate.StatusCompleted) || group.Reason != "" {
			return false
		}
	}
	return true
}

// resumeMutationRows reports the rows the mutation layer owes plus every row the
// run already has a record for, in dispatch order. A row the dispatch inputs no
// longer name still appears if the run recorded one, so nothing the ledger holds
// disappears from the report.
func resumeMutationRows(
	ledger *runstate.Ledger,
	dispatch *dispatchInputs,
	invalidReason func(*runstate.Record) string,
	out *resumeOutput,
) (required []string, rowRecords map[string]*runstate.Record) {
	required = []string{}
	rowRecords = map[string]*runstate.Record{}
	for i := range ledger.Records {
		rec := &ledger.Records[i]
		if rec.Kind == runstate.KindMutationRow && rec.Status != runstate.StatusSuperseded {
			rowRecords[rec.ID] = rec
		}
	}

	reported := map[string]bool{}
	for _, row := range dispatch.Rows {
		// A row owes a mutation when it is high risk and the trace bound it to
		// something to mutate. A row that owes nothing is still reported when
		// the run recorded one for it anyway.
		owed := row.Risk == "high" && row.Status != "gap" && row.Status != "ambiguity"
		if owed {
			required = append(required, row.RowID)
		}
		if !owed && rowRecords[row.RowID] == nil {
			continue
		}
		out.Mutation.Rows = append(out.Mutation.Rows, mutationRowReport(row.RowID, rowRecords[row.RowID], invalidReason))
		reported[row.RowID] = true
	}
	for i := range ledger.Records {
		rec := &ledger.Records[i]
		if rec.Kind != runstate.KindMutationRow || rec.Status == runstate.StatusSuperseded || reported[rec.ID] {
			continue
		}
		out.Mutation.Rows = append(out.Mutation.Rows, mutationRowReport(rec.ID, rec, invalidReason))
		reported[rec.ID] = true
	}
	return required, rowRecords
}

// mutationRowReport describes one row's standing. A started row is read back
// from the workspace prepare-mutation left, so an interrupted evaluation says
// which sites it finished and which one it was inside.
func mutationRowReport(rowID string, rec *runstate.Record, invalidReason func(*runstate.Record) string) resumeMutationRow {
	row := resumeMutationRow{RowID: rowID, Status: statusNone, CompletedSites: []string{}}
	if rec == nil {
		return row
	}
	var inputs mutationRowInputs
	inputsErr := recordInputs(rec, &inputs)
	row.Status = string(rec.Status)
	row.Outcome = rec.Outcome
	row.EvidencePath = recordEvidencePath(rec)
	row.RunDir = inputs.RunDir
	row.WorktreePath = inputs.WorktreePath
	row.Reason = invalidReason(rec)
	if inputsErr != nil && row.Reason == "" {
		row.Reason = reasonInputsUnreadable
	}

	if rec.Status == runstate.StatusStarted && inputs.RunDir != "" {
		sites, interrupted, err := mutationProgress(inputs.RunDir)
		if err != nil {
			row.Reason = reasonStateUnreadable
		} else {
			row.CompletedSites, row.InterruptedSite = sites, interrupted
		}
	}
	return row
}

// startedSiteProbe reads the site an interrupted evaluation was inside. It is a
// probe over the state document rather than a field of the loaded shape because
// evaluate-mutation owns that field: resume must report what the document says
// whether or not this build's state package declares it.
type startedSiteProbe struct {
	Started *struct {
		Site string `json:"site"`
	} `json:"started"`
}

// mutationProgress reads a prepared row's state document: the sites it already
// evaluated, and the site it was inside when it stopped.
func mutationProgress(runDir string) ([]string, string, error) {
	st, err := mutation.LoadState(runDir)
	if err != nil {
		return nil, "", err
	}
	completed := make([]string, 0, len(st.Sites))
	for _, site := range st.Sites {
		completed = append(completed, site.Site)
	}

	data, err := os.ReadFile(filepath.Join(runDir, "mutation-state.json"))
	if err != nil {
		return nil, "", err
	}
	var probe startedSiteProbe
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, "", err
	}
	if probe.Started == nil {
		return completed, "", nil
	}
	return completed, probe.Started.Site, nil
}

func mutationComplete(required []string, rowRecords map[string]*runstate.Record, invalidReason func(*runstate.Record) string) bool {
	for _, id := range required {
		rec := rowRecords[id]
		if rec == nil || rec.Status != runstate.StatusCompleted || invalidReason(rec) != "" {
			return false
		}
	}
	return true
}

// recordEvidencePath is the file a completed record's outcome rests on: its
// evidence, or the attempt document for an outcome that has no evidence to
// point at because the attempt never produced one.
func recordEvidencePath(rec *runstate.Record) string {
	if path := rec.Artifacts[artifactEvidence].Path; path != "" {
		return path
	}
	return rec.Artifacts[artifactAttempt].Path
}

// pendingObligations lists what the run still owes: first the obligations it
// already opened, in ledger order, then the ones it never opened, in dispatch
// row order. A completed failure is never pending — it is an answer, not a gap.
func pendingObligations(
	ledger *runstate.Ledger,
	out resumeOutput,
	required []string,
	rowRecords map[string]*runstate.Record,
	groupInvalid map[string]string,
	invalidReason func(*runstate.Record) string,
) []resumePending {
	requiredRow := make(map[string]bool, len(required))
	for _, id := range required {
		requiredRow[id] = true
	}

	pending := []resumePending{}
	for i := range ledger.Records {
		rec := &ledger.Records[i]
		if rec.Status == runstate.StatusSuperseded || rec.Kind == runstate.KindPhase {
			continue
		}
		switch rec.Kind {
		case runstate.KindMutationRow:
			if !requiredRow[rec.ID] {
				continue
			}
		case runstate.KindLiveAttempt:
			if !out.Live.Required {
				continue
			}
		}
		if rec.Status == runstate.StatusStarted {
			pending = append(pending, resumePending{Kind: rec.Kind, ID: rec.ID, Reason: reasonNotCompleted})
			continue
		}
		if runstate.IsFailedOutcome(rec.Kind, rec.Outcome) {
			continue
		}
		reason := invalidReason(rec)
		if reason == "" && rec.Kind == runstate.KindReviewGroup {
			reason = groupInvalid[rec.ID]
		}
		if reason != "" {
			pending = append(pending, resumePending{Kind: rec.Kind, ID: rec.ID, Reason: reason})
		}
	}

	if len(out.Review.Uncovered) > 0 {
		pending = append(pending, resumePending{
			Kind:   runstate.KindReviewGroup,
			Reason: "uncovered: " + strings.Join(out.Review.Uncovered, ", "),
		})
	}
	if out.Live.Required && runstate.Current(ledger, runstate.KindLiveAttempt, liveAttemptID) == nil {
		pending = append(pending, resumePending{Kind: runstate.KindLiveAttempt, ID: liveAttemptID, Reason: reasonNeverStarted})
	}
	for _, id := range required {
		if rowRecords[id] == nil {
			pending = append(pending, resumePending{Kind: runstate.KindMutationRow, ID: id, Reason: reasonNeverStarted})
		}
	}
	return pending
}

// countRecords tallies the ledger: what is open, what is answered, what failed,
// what was replaced, and what no longer stands.
func countRecords(
	ledger *runstate.Ledger,
	invalidReason func(*runstate.Record) string,
	groupInvalid map[string]string,
	pending int,
) resumeCounts {
	counts := resumeCounts{Pending: pending}
	for i := range ledger.Records {
		rec := &ledger.Records[i]
		switch rec.Status {
		case runstate.StatusStarted:
			counts.Started++
		case runstate.StatusSuperseded:
			counts.Superseded++
			continue
		case runstate.StatusCompleted:
			counts.Completed++
			if runstate.IsFailedOutcome(rec.Kind, rec.Outcome) {
				counts.Failed++
			}
			if invalidReason(rec) != "" || (rec.Kind == runstate.KindReviewGroup && groupInvalid[rec.ID] != "") {
				counts.Invalid++
			}
		}
	}
	return counts
}

// groupInputsMatch reports whether a group's recorded input identity still
// describes what the run hands a critic today.
func groupInputsMatch(recorded, current reviewGroupInputs) bool {
	if recorded.DispatchInputsSHA256 != current.DispatchInputsSHA256 {
		return false
	}
	if len(recorded.Sources) != len(current.Sources) {
		return false
	}
	for row, want := range current.Sources {
		got, ok := recorded.Sources[row]
		if !ok || !slices.Equal(got, want) {
			return false
		}
	}
	return true
}
