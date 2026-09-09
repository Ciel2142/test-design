package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"test-design/internal/evidence"
	"test-design/internal/runstate"
)

// outcomeUnappliable is the one outcome this op may record. A caught or
// survived row is decided by running the mutant, which evaluate-mutation owns;
// letting a caller assert either here would let the ledger carry a verdict no
// run produced.
const outcomeUnappliable = "unappliable"

type recordMutationRequest struct {
	verifyRequest
	RowID   string `json:"rowId"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`
}

type mutationUnappliableEvidence struct {
	evidence.Envelope
	RunID        string `json:"runId"`
	RowID        string `json:"rowId"`
	RepoCommit   string `json:"repoCommit"`
	Source       string `json:"source"`
	SourceSHA256 string `json:"sourceSha256"`
	Reason       string `json:"reason"`
	RunDir       string `json:"runDir"`
	WorktreePath string `json:"worktreePath"`
}

type recordMutationOutput struct {
	evidence.Envelope
	RunID        string `json:"runId"`
	RowID        string `json:"rowId"`
	Outcome      string `json:"outcome"`
	EvidencePath string `json:"evidencePath"`
}

func newRecordMutationFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("record-mutation", flag.ContinueOnError)
	addRequestFlag(fs)
	return fs
}

func decodeRecordMutationRequest(op string, requestBody []byte) (*recordMutationRequest, *cliError) {
	base, cliErr := decodeVerifyRequest(op, requestBody)
	if cliErr != nil {
		return nil, cliErr
	}
	var extra struct {
		RowID   string `json:"rowId"`
		Outcome string `json:"outcome"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal(requestBody, &extra); err != nil {
		return nil, &cliError{Code: "malformed-request", Message: fmt.Sprintf("%s: decode request: %v", op, err)}
	}
	if base.RunID == "" {
		return nil, &cliError{Code: "missing-run-id", Message: op + ": runID is required"}
	}
	if extra.RowID == "" {
		return nil, &cliError{Code: "missing-row-id", Message: op + ": rowId is required"}
	}
	return &recordMutationRequest{
		verifyRequest: *base, RowID: extra.RowID, Outcome: extra.Outcome, Reason: extra.Reason,
	}, nil
}

// recordMutationHandler closes a mutation row the mutator found no site for.
// It records the caller's account of why — never a verdict about the test — and
// completes the row against a document that names the prepared workspace the
// judgement was made in.
func recordMutationHandler(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError {
	const op = "record-mutation"
	const kind = runstate.KindMutationRow

	req, cliErr := decodeRecordMutationRequest(op, requestBody)
	if cliErr != nil {
		return cliErr
	}
	src, cliErr := loadVerifiedSource(op, req.RepoRoot, req.CoverageSource)
	if cliErr != nil {
		return cliErr
	}
	if !slices.Contains(activeRowIDs(src), req.RowID) {
		return &cliError{
			Code:    "unknown-row",
			Message: fmt.Sprintf("%s: %s is not an active row of %s", op, req.RowID, src.Path),
		}
	}
	if req.Outcome != outcomeUnappliable {
		return &cliError{
			Code: "invalid-outcome",
			Message: fmt.Sprintf("%s: outcome %q is not %s; a caught or survived row is decided by evaluate-mutation",
				op, req.Outcome, outcomeUnappliable),
		}
	}
	if strings.TrimSpace(req.Reason) == "" {
		return &cliError{
			Code:    "missing-reason",
			Message: op + ": name why no site applies; an unappliable row without a reason is not evidence",
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

	// The row must already be prepared: this op finishes an obligation, it never
	// opens one, so a row nothing started has nothing here to close. The check,
	// the evidence write and the completion run under one exclusive lock, so a
	// second verdict arriving meanwhile finds the row completed and writes
	// nothing.
	var out recordMutationOutput
	err := store.Update(identity, func(l *runstate.Ledger) error {
		current := runstate.Current(l, kind, req.RowID)
		switch {
		case current == nil:
			return &cliError{
				Code:    "mutation-row-not-started",
				Message: fmt.Sprintf("%s: %s was never prepared for this run", op, req.RowID),
			}
		case current.Status == runstate.StatusCompleted:
			return completedRecordRefusal(op, kind, req.RowID)
		}

		var prepared mutationRowInputs
		if err := recordInputs(current, &prepared); err != nil {
			return &cliError{Code: "ledger-invalid", Message: fmt.Sprintf("%s: %v", op, err)}
		}

		doc := mutationUnappliableEvidence{
			Envelope:     evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
			RunID:        req.RunID,
			RowID:        req.RowID,
			RepoCommit:   repoCommit,
			Source:       src.Path,
			SourceSHA256: src.SHA256,
			Reason:       req.Reason,
			RunDir:       prepared.RunDir,
			WorktreePath: prepared.WorktreePath,
		}
		path := filepath.Join(store.Dir(), "mutation-unappliable-"+req.RowID+".json")
		if err := evidence.WriteJSON(path, &doc); err != nil {
			return &cliError{Code: "evidence-write-failed", Message: fmt.Sprintf("%s: %v", op, err)}
		}

		if _, err := l.Complete(kind, req.RowID, outcomeUnappliable, map[string]string{artifactEvidence: path}, ""); err != nil {
			return err
		}
		out = recordMutationOutput{
			Envelope: doc.Envelope, RunID: req.RunID, RowID: req.RowID,
			Outcome: outcomeUnappliable, EvidencePath: path,
		}
		return nil
	})
	if cliErr := updateError(op, kind, err); cliErr != nil {
		return cliErr
	}
	return writeStdout(stdout, out)
}
