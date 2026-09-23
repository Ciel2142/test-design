package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"test-design/internal/evidence"
	"test-design/internal/report"
	"test-design/internal/runstate"
)

// The proofs in this file drive the real ops over the ready subject and read
// only what those ops left in the repository and the ledger. What they hold
// between them is the line a run draws around its own publication: a report
// that could not be written is published again from the evidence the run
// already holds, recovery takes back nothing it cannot prove this run wrote,
// and committing the report does not move the revision the report measures.

const (
	pubProofDir  = "docs/superpowers/coverage"
	pubProofStem = "2026-09-07-ready-subject"

	// The two files one run of the ready subject publishes beside its coverage
	// document, stated rather than derived: a change to how finalize names them
	// is a change to what an operator commits, and it should fail here by path.
	pubProofJSON = pubProofDir + "/" + pubProofStem + "-verification.json"
	pubProofMD   = pubProofDir + "/" + pubProofStem + "-verification.md"
)

// pubProofRecord is what one ledger record looks like to a proof that has to
// show a publication moved none of them: its place, its key and where it
// stands. Inputs, artifacts and timestamps are the record's own business.
type pubProofRecord struct {
	Seq     int
	Kind    runstate.Kind
	ID      string
	Status  runstate.Status
	Outcome string
}

// pubProofVerificationRecords is every record of the run except its
// publication — the trace, review, live and mutation work a retry must not
// repeat.
func pubProofVerificationRecords(out resumeOutput) []pubProofRecord {
	records := []pubProofRecord{}
	for _, rec := range out.Records {
		if rec.Kind == runstate.KindPhase && rec.ID == publishPhaseID {
			continue
		}
		records = append(records, pubProofRecord{
			Seq: rec.Seq, Kind: rec.Kind, ID: rec.ID, Status: rec.Status, Outcome: rec.Outcome,
		})
	}
	return records
}

// pubProofPublicationStatuses is the other half: where each of the run's
// publication records stands, in ledger order.
func pubProofPublicationStatuses(out resumeOutput) []runstate.Status {
	statuses := []runstate.Status{}
	for _, rec := range out.Records {
		if rec.Kind == runstate.KindPhase && rec.ID == publishPhaseID {
			statuses = append(statuses, rec.Status)
		}
	}
	return statuses
}

// pubProofFinalize drives the real finalize op expecting a published report.
func pubProofFinalize(t *testing.T, s *liveSubject, runID string, body finalizeRequestBody) finalizeOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "finalize", readyFinalizeRequestJSON(t, s, runID, body))
	if code != exitOK {
		t.Fatalf("finalize exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	return decodeFinalizeOutput(t, stdout)
}

// pubProofRefusal drives finalize expecting the named refusal and returns it
// with the raw stderr, which is the whole of what a refusal says: a run that
// published nothing reports no output document at all.
func pubProofRefusal(
	t *testing.T, s *liveSubject, runID string, body finalizeRequestBody, code string,
) (cliError, string) {
	t.Helper()
	exit, stdout, stderr := runVerifyOp(t, "finalize", readyFinalizeRequestJSON(t, s, runID, body))
	if exit != exitError || stdout != "" {
		t.Fatalf("finalize was accepted: exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != code {
		t.Errorf("refusal code = %q, want %q; message=%s", got.Code, code, got.Message)
	}
	return got, stderr
}

// pubProofPlantDir puts a directory where a file of the publication belongs:
// the destination fault Publish cannot write through, and the one the harness
// repairs afterwards to prove the retry needs nothing else repaired. The
// cleanup takes back only a directory — by then the path is usually the
// published file itself.
func pubProofPlantDir(t *testing.T, s *liveSubject, relPath string) string {
	t.Helper()
	full := filepath.Join(s.repo, filepath.FromSlash(relPath))
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatalf("plant a directory at %s: %v", relPath, err)
	}
	t.Cleanup(func() {
		if info, err := os.Stat(full); err == nil && info.IsDir() {
			_ = os.RemoveAll(full)
		}
	})
	return full
}

// pubProofWrite plants a file the run never published, through the same writer
// every other subject file goes through, and returns its absolute path so a
// proof can digest the file it left behind.
func pubProofWrite(t *testing.T, s *liveSubject, relPath, content string) string {
	t.Helper()
	full := filepath.Join(s.repo, filepath.FromSlash(relPath))
	writeSubjectFile(t, s.repo, relPath, content)
	t.Cleanup(func() { _ = os.Remove(full) })
	return full
}

// pubProofExclude adds one ignore pattern to the repository's own exclude file,
// which is how a file lives inside the run's transcripts directory without
// being anything git — or the snapshot gate — ever sees.
func pubProofExclude(t *testing.T, s *liveSubject, pattern string) {
	t.Helper()
	path := filepath.Join(s.repo, ".git", "info", "exclude")
	original, err := os.ReadFile(path)
	existed := err == nil
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, append(slices.Clone(original), []byte(pattern+"\n")...), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.WriteFile(path, original, 0o644)
			return
		}
		_ = os.Remove(path)
	})
}

// pubProofAppend adds a line to a file already in the repository and returns
// the digest of what it becomes.
func pubProofAppend(t *testing.T, s *liveSubject, relPath, extra string) string {
	t.Helper()
	full := filepath.Join(s.repo, filepath.FromSlash(relPath))
	data, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	if err := os.WriteFile(full, append(slices.Clone(data), []byte(extra)...), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
	return readyDigest(t, full)
}

// pubProofRestoreFromRunDir puts a published file back from the copy the run
// dir still holds, which is the same bytes the publication wrote.
func pubProofRestoreFromRunDir(t *testing.T, s *liveSubject, runDir, relPath string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runDir, filepath.Base(relPath)))
	if err != nil {
		t.Fatalf("read the run dir's copy of %s: %v", relPath, err)
	}
	if err := os.WriteFile(filepath.Join(s.repo, filepath.FromSlash(relPath)), data, 0o644); err != nil {
		t.Fatalf("restore %s: %v", relPath, err)
	}
}

// pubProofDigests digests every repo-relative path, so a proof can say a whole
// set of files is byte-identical before and after a refusal.
func pubProofDigests(t *testing.T, s *liveSubject, paths []string) map[string]string {
	t.Helper()
	digests := make(map[string]string, len(paths))
	for _, path := range paths {
		digests[path] = readyDigest(t, filepath.Join(s.repo, filepath.FromSlash(path)))
	}
	return digests
}

// pubProofPaths is the path list of a recorded publication, in the order the
// record names them.
func pubProofPaths(files []evidence.FileRef) []string {
	paths := make([]string, 0, len(files))
	for _, ref := range files {
		paths = append(paths, ref.Path)
	}
	return paths
}

// pubProofRecovered is the recovery a whole publication earns when every file
// of it is still exactly where the run left it. Both lists are sorted by path.
func pubProofRecovered(files []evidence.FileRef, action string) []recoveredFile {
	recovered := make([]recoveredFile, 0, len(files))
	for _, ref := range files {
		recovered = append(recovered, recoveredFile{Path: ref.Path, Action: action})
	}
	return recovered
}

// pubProofPublishedFiles asserts every file the publication record names is in
// the repository with the digest that record recorded for it.
func pubProofPublishedFiles(t *testing.T, s *liveSubject, files []evidence.FileRef) {
	t.Helper()
	if len(files) == 0 {
		t.Fatalf("the publication record names no files at all")
	}
	for _, ref := range files {
		got, err := evidence.SHA256File(filepath.Join(s.repo, filepath.FromSlash(ref.Path)))
		if err != nil {
			t.Errorf("published %s: %v", ref.Path, err)
			continue
		}
		if got != ref.SHA256 {
			t.Errorf("%s: published sha256 = %s, want the recorded %s", ref.Path, got, ref.SHA256)
		}
	}
}

// pubProofPublication is what resume says about the run's publication.
func pubProofPublication(t *testing.T, s *liveSubject, runID string) resumePublication {
	t.Helper()
	return readyResume(t, s, runID).Publication
}

// covers: SC-30
func TestPublication_RetriesFromRetainedEvidence(t *testing.T) {
	s := readySubject(t, "not-applicable")
	runA := readyAdoptedRun(t, s, "pub-retry", readyCaughtVersionRow)
	beforeA := pubProofVerificationRecords(readyResume(t, s, runA))

	// A destination that cannot take the first file leaves the repository as
	// it was: the run owes a publication and nothing else.
	t.Run("zero-written", func(t *testing.T) {
		planted := pubProofPlantDir(t, s, pubProofJSON)

		got, _ := pubProofRefusal(t, s, runA, freshFinalizeBody(), "publish-failed")
		for _, want := range []string{"did not write", "rerun finalize"} {
			if !strings.Contains(got.Message, want) {
				t.Errorf("message = %q, want it to say %q", got.Message, want)
			}
		}
		if lines := gitStatusLines(t, s.repo); len(lines) != 0 {
			t.Errorf("git status = %v, want a publication that wrote nothing to leave the tree clean", lines)
		}

		state := readyResume(t, s, runA)
		if state.Publication.Status != string(runstate.StatusStarted) {
			t.Errorf("publication = %+v, want a record left started", state.Publication)
		}
		owed := resumePending{Kind: runstate.KindPhase, ID: publishPhaseID, Reason: "started-not-completed"}
		if !slices.Contains(state.Pending, owed) {
			t.Errorf("pending = %+v, want it to carry %+v", state.Pending, owed)
		}
		if records := pubProofVerificationRecords(state); !slices.Equal(records, beforeA) {
			t.Errorf("records = %+v, want the failed publication to have moved none of them: %+v", records, beforeA)
		}

		// The destination alone is repaired. One rerun publishes the whole set
		// from the evidence the run already held.
		if err := os.RemoveAll(planted); err != nil {
			t.Fatalf("remove the planted directory: %v", err)
		}
		out := pubProofFinalize(t, s, runA, freshFinalizeBody())
		if out.Verdict != report.VerdictPass {
			t.Errorf("verdict = %q, want %s; reasons=%+v", out.Verdict, report.VerdictPass, out.Reasons)
		}
		if len(out.Recovered) != 0 {
			t.Errorf("recovered = %+v, want none: the failed publication wrote no file to take back", out.Recovered)
		}

		after := readyResume(t, s, runA)
		pubProofPublishedFiles(t, s, after.Publication.Files)
		if after.Publication.Status != string(runstate.StatusCompleted) || after.Publication.Outcome != "ok" {
			t.Errorf("publication = %+v, want a completed record", after.Publication)
		}
		if records := pubProofVerificationRecords(after); !slices.Equal(records, beforeA) {
			t.Errorf("records = %+v, want the retry to have redone no verification work: %+v", records, beforeA)
		}
	})

	// A destination that takes the report but not the transcript beside it
	// leaves a prefix of the set behind, and the rerun takes that prefix back
	// before it writes the whole set again.
	t.Run("prefix-written", func(t *testing.T) {
		mustGit(t, s.repo, "add", "-A")
		mustGit(t, s.repo, "commit", "-q", "-m", "publish the retried run's verification")
		published := subjectHead(t, s.repo)

		runB := readyAdoptedRun(t, s, "pub-prefix", readyCaughtVersionRow)
		beforeB := pubProofVerificationRecords(readyResume(t, s, runB))
		transcript := readyTranscriptPath(s, runB, traceEvidenceFile)
		planted := pubProofPlantDir(t, s, transcript)

		got, _ := pubProofRefusal(t, s, runB, freshFinalizeBody(), "publish-failed")
		if want := fmt.Sprintf("wrote [%s %s]", pubProofJSON, pubProofMD); !strings.Contains(got.Message, want) {
			t.Errorf("message = %q, want it to name %q as written", got.Message, want)
		}
		if want := "did not write [" + transcript; !strings.Contains(got.Message, want) {
			t.Errorf("message = %q, want it to name %q as unwritten", got.Message, want)
		}
		wantDirty := []string{" M " + pubProofJSON, " M " + pubProofMD}
		if lines := gitStatusLines(t, s.repo); !slices.Equal(lines, wantDirty) {
			t.Errorf("git status = %v, want exactly the report and its Markdown modified: %v", lines, wantDirty)
		}
		if publication := pubProofPublication(t, s, runB); publication.Status != string(runstate.StatusStarted) {
			t.Errorf("publication = %+v, want a record left started", publication)
		}

		if err := os.RemoveAll(planted); err != nil {
			t.Fatalf("remove the planted directory: %v", err)
		}
		out := pubProofFinalize(t, s, runB, freshFinalizeBody())
		wantRecovered := []recoveredFile{
			{Path: pubProofJSON, Action: recoveryRestored},
			{Path: pubProofMD, Action: recoveryRestored},
		}
		if !slices.Equal(out.Recovered, wantRecovered) {
			t.Errorf("recovered = %+v, want %+v", out.Recovered, wantRecovered)
		}
		if rep := readPublishedReport(t, s.repo, out.JSONPath); rep.HeadCommit != published {
			t.Errorf("report headCommit = %q, want the commit the run traced %q", rep.HeadCommit, published)
		}

		after := readyResume(t, s, runB)
		pubProofPublishedFiles(t, s, after.Publication.Files)
		if records := pubProofVerificationRecords(after); !slices.Equal(records, beforeB) {
			t.Errorf("records = %+v, want the retry to have redone no verification work: %+v", records, beforeB)
		}
	})

	// An open publication owes the publication and nothing else. A run whose
	// live decision is required and whose journey already passed is not asked
	// for that layer again because its report could not be written: the retry
	// stands on the completed attempt, and a layer re-offered here would be a
	// live environment driven twice for one verdict.
	t.Run("live-answered", func(t *testing.T) {
		live := readySubject(t, "required")
		runID := liveRunID("pub-live")

		tr, _, exec := runLivePipeline(t, live, runID, revisionLivePlan(runID), nil)
		if got := rowVerdict(t, exec.RowVerdicts, readyMarkerRow).Verdict; got != "LIVE-PASS" {
			t.Fatalf("live verdict for %s = %q, want LIVE-PASS", readyMarkerRow, got)
		}
		readyGroups(t, live, runID, readDispatchInputs(t, tr.DispatchPath), reusePolicy(t))
		readyCaughtVersionRow(t, live, runID)
		before := pubProofVerificationRecords(readyResume(t, live, runID))

		planted := pubProofPlantDir(t, live, pubProofJSON)
		pubProofRefusal(t, live, runID, freshFinalizeBody(), "publish-failed")

		state := readyResume(t, live, runID)
		if state.Publication.Status != string(runstate.StatusStarted) {
			t.Fatalf("publication = %+v, want a record left started", state.Publication)
		}
		if !state.Live.Required || state.Live.Status != string(runstate.StatusCompleted) || state.Live.Reason != "" {
			t.Fatalf("live = %+v, want the required layer completed with nothing against it", state.Live)
		}
		wantOwed := []resumePending{{Kind: runstate.KindPhase, ID: publishPhaseID, Reason: reasonNotCompleted}}
		if !slices.Equal(state.Pending, wantOwed) {
			t.Errorf("pending = %+v, want only the publication owed %+v: an open publication does not re-owe an answered layer",
				state.Pending, wantOwed)
		}

		if err := os.RemoveAll(planted); err != nil {
			t.Fatalf("remove the planted directory: %v", err)
		}
		out := pubProofFinalize(t, live, runID, freshFinalizeBody())
		if out.Verdict != report.VerdictPass {
			t.Errorf("verdict = %q, want %s; reasons=%+v", out.Verdict, report.VerdictPass, out.Reasons)
		}
		after := readyResume(t, live, runID)
		if after.Publication.Status != string(runstate.StatusCompleted) || after.Publication.Outcome != "ok" {
			t.Errorf("publication = %+v, want a completed record", after.Publication)
		}
		if records := pubProofVerificationRecords(after); !slices.Equal(records, before) {
			t.Errorf("records = %+v, want the retry to have redone no verification work: %+v", records, before)
		}
	})
}

// covers: SC-31
func TestPublication_RecoversOnlyProvenOwnArtifacts(t *testing.T) {
	s := readySubject(t, "not-applicable")
	runC := readyAdoptedRun(t, s, "pub-recover", readyCaughtVersionRow)

	current := pubProofFinalize(t, s, runC, freshFinalizeBody())
	if current.Verdict != report.VerdictPass {
		t.Fatalf("verdict = %q, want %s; reasons=%+v", current.Verdict, report.VerdictPass, current.Reasons)
	}
	if current.JSONPath != pubProofJSON || current.MarkdownPath != pubProofMD {
		t.Fatalf("published %s and %s, want %s and %s",
			current.JSONPath, current.MarkdownPath, pubProofJSON, pubProofMD)
	}

	// A file the run never wrote is not the run's to remove, whatever directory
	// it sits in — and one nothing tracks is invisible to recovery entirely.
	t.Run("ignored-sentinel-survives", func(t *testing.T) {
		before := readyResume(t, s, runC).Publication.Files
		pubProofExclude(t, s, "scratch.log")
		sentinel := readyTranscriptPath(s, runC, "scratch.log")
		full := pubProofWrite(t, s, sentinel, "a scratch file no run published\n")
		want := readyDigest(t, full)

		out := pubProofFinalize(t, s, runC, freshFinalizeBody())
		current = out

		wantRecovered := pubProofRecovered(before, recoveryRemoved)
		if !slices.Equal(out.Recovered, wantRecovered) {
			t.Errorf("recovered = %+v, want every file of the first publication removed: %+v",
				out.Recovered, wantRecovered)
		}
		if got := readyDigest(t, full); got != want {
			t.Errorf("%s digests to %s, want the ignored file untouched at %s", sentinel, got, want)
		}
	})

	// One dirty path this run cannot prove its own stops the whole recovery,
	// inside the transcripts directory or anywhere else in the repository.
	t.Run("unrelated-dirty-refused", func(t *testing.T) {
		published := pubProofPaths(readyResume(t, s, runC).Publication.Files)
		before := pubProofDigests(t, s, published)
		notes := readyTranscriptPath(s, runC, "notes.txt")
		full := pubProofWrite(t, s, notes, "a note nobody published\n")
		noteDigest := readyDigest(t, full)

		got, stderr := pubProofRefusal(t, s, runC, freshFinalizeBody(), "source-not-reproducible")
		if !strings.Contains(got.Message, notes) {
			t.Errorf("message = %q, want it to name %s", got.Message, notes)
		}
		if strings.Contains(stderr, "recovered") {
			t.Errorf("refusal = %s, want a refusal that took nothing back to report no recovery", stderr)
		}
		if after := pubProofDigests(t, s, published); !maps.Equal(after, before) {
			t.Errorf("published digests = %v, want every file preserved: %v", after, before)
		}
		if got := readyDigest(t, full); got != noteDigest {
			t.Errorf("%s digests to %s, want it preserved at %s", notes, got, noteDigest)
		}

		if err := os.Remove(full); err != nil {
			t.Fatalf("remove %s: %v", notes, err)
		}

		// A tracked file this run never published answers the same way. The
		// restore is registered before the refusal is asked for: the subtests
		// share one repository, and a failure here must not hand the next one a
		// modified go.mod.
		pubProofAppend(t, s, "go.mod", "\n// a later thought\n")
		t.Cleanup(func() { mustGit(t, s.repo, "checkout", "--", "go.mod") })
		got, _ = pubProofRefusal(t, s, runC, freshFinalizeBody(), "source-not-reproducible")
		if !strings.Contains(got.Message, "go.mod") {
			t.Errorf("message = %q, want it to name go.mod", got.Message)
		}
		if after := pubProofDigests(t, s, published); !maps.Equal(after, before) {
			t.Errorf("published digests = %v, want every file preserved: %v", after, before)
		}
	})

	// An own path whose bytes are somebody's edit is a distinct refusal: this
	// run wrote the file, so it says so, and still discards nothing.
	t.Run("edited-output-refused", func(t *testing.T) {
		published := pubProofPaths(readyResume(t, s, runC).Publication.Files)
		others := slices.DeleteFunc(slices.Clone(published), func(path string) bool { return path == pubProofMD })
		before := pubProofDigests(t, s, others)
		edited := pubProofAppend(t, s, pubProofMD, "\na later thought\n")
		// The edit is taken back from the run dir the last publication wrote,
		// which still holds the bytes it published.
		t.Cleanup(func() { pubProofRestoreFromRunDir(t, s, current.RunDir, pubProofMD) })

		got, _ := pubProofRefusal(t, s, runC, freshFinalizeBody(), codePublicationUnverifiable)
		if !strings.Contains(got.Message, pubProofMD) {
			t.Errorf("message = %q, want it to name %s", got.Message, pubProofMD)
		}
		if now := readyDigest(t, filepath.Join(s.repo, filepath.FromSlash(pubProofMD))); now != edited {
			t.Errorf("%s digests to %s, want the edit kept at %s", pubProofMD, now, edited)
		}
		if after := pubProofDigests(t, s, others); !maps.Equal(after, before) {
			t.Errorf("digests = %v, want the report and its transcripts preserved: %v", after, before)
		}
	})

	// Once the publication is committed, its own files are tracked: recovery
	// puts those back rather than removing them, and touches nothing that did
	// not move.
	t.Run("tracked-restored-untracked-removed", func(t *testing.T) {
		published := pubProofPaths(readyResume(t, s, runC).Publication.Files)
		// The committed copies are made to differ from what a republication
		// writes, and to differ whatever the clock does: the report varies only
		// in finishedAt, which RFC3339 resolves to one second, and its Markdown
		// carries no timestamp at all, so two publications of an unchanged run
		// are otherwise byte-identical. A trailing newline leaves the JSON
		// valid, a trailing comment line leaves the Markdown valid, and both
		// make the file finalize writes next a modification of a tracked one.
		pubProofAppend(t, s, pubProofJSON, "\n")
		pubProofAppend(t, s, pubProofMD, "\n<!-- the committed copy of this report -->\n")
		mustGit(t, s.repo, append([]string{"add"}, published...)...)
		mustGit(t, s.repo, "commit", "-q", "-m", "publish the recovered run's verification")
		if lines := gitStatusLines(t, s.repo); len(lines) != 0 {
			t.Fatalf("git status = %v, want the subject clean once the publication is committed", lines)
		}

		pubProofFinalize(t, s, runC, freshFinalizeBody())
		wantDirty := []string{" M " + pubProofJSON, " M " + pubProofMD}
		if lines := gitStatusLines(t, s.repo); !slices.Equal(lines, wantDirty) {
			t.Errorf("git status = %v, want only the report rewritten over its committed self: %v", lines, wantDirty)
		}

		again := pubProofFinalize(t, s, runC, freshFinalizeBody())
		wantRecovered := []recoveredFile{
			{Path: pubProofJSON, Action: recoveryRestored},
			{Path: pubProofMD, Action: recoveryRestored},
		}
		if !slices.Equal(again.Recovered, wantRecovered) {
			t.Errorf("recovered = %+v, want %+v", again.Recovered, wantRecovered)
		}
		for _, file := range again.Recovered {
			if file.Action == recoveryRemoved {
				t.Errorf("recovered = %+v, want nothing removed: every own file is tracked at HEAD", again.Recovered)
			}
		}
		if lines := gitStatusLines(t, s.repo); !slices.Equal(lines, wantDirty) {
			t.Errorf("git status = %v, want only the report rewritten over its committed self: %v", lines, wantDirty)
		}
	})

	// A restore is one path at a time, and the proof is an edit the gate never
	// sees. A tracked file marked assume-unchanged is the operator's promise
	// that git may ignore its local modification: status lists nothing, so
	// recovery has nothing to refuse and the run's own restore goes ahead. A
	// checkout of the whole tree — what a per-path restore must not become —
	// would take that edit back along with the run's own files, silently.
	t.Run("assumed-unchanged-edit-survives-restore", func(t *testing.T) {
		const local = "cmd/ready/main.go"
		edited := pubProofAppend(t, s, local, "\n// a local edit git was told to assume unchanged\n")
		mustGit(t, s.repo, "update-index", "--assume-unchanged", local)
		t.Cleanup(func() {
			mustGit(t, s.repo, "update-index", "--no-assume-unchanged", local)
			mustGit(t, s.repo, "checkout", "--", local)
		})
		wantDirty := []string{" M " + pubProofJSON, " M " + pubProofMD}
		if lines := gitStatusLines(t, s.repo); !slices.Equal(lines, wantDirty) {
			t.Fatalf("git status = %v, want the run's own report dirty and the assumed-unchanged edit invisible: %v", lines, wantDirty)
		}

		out := pubProofFinalize(t, s, runC, freshFinalizeBody())
		wantRecovered := []recoveredFile{
			{Path: pubProofJSON, Action: recoveryRestored},
			{Path: pubProofMD, Action: recoveryRestored},
		}
		if !slices.Equal(out.Recovered, wantRecovered) {
			t.Errorf("recovered = %+v, want %+v", out.Recovered, wantRecovered)
		}
		if got := readyDigest(t, filepath.Join(s.repo, filepath.FromSlash(local))); got != edited {
			t.Errorf("%s digests to %s, want the local edit kept at %s: restoring the run's own files took back a file the run never wrote",
				local, got, edited)
		}
		if lines := gitStatusLines(t, s.repo); !slices.Equal(lines, wantDirty) {
			t.Errorf("git status = %v, want only the report rewritten over its committed self: %v", lines, wantDirty)
		}
	})

	// Recovery removes files, never trees. The artifact directory is where an
	// operator keeps the coverage document, other runs' transcripts and their
	// own notes, and finalize is run from inside the repository as an operator
	// runs it: from there the directory's relative name is a real path, and a
	// recovery that cleared it would clear all of that. Neither neighbour is
	// anything git sees — recovery refuses what it can see and cannot prove —
	// so nothing but the recovery itself stands between them and deletion.
	t.Run("artifact-directory-neighbours-survive", func(t *testing.T) {
		t.Chdir(s.repo)
		pubProofExclude(t, s, "notes.local.md")
		pubProofExclude(t, s, "run-2020-01-01-0001/")
		notes := repoPath(pubProofDir, "notes.local.md")
		foreign := repoPath(pubProofDir, "transcripts", "run-2020-01-01-0001", "trace-evidence.json")
		neighbours := map[string]string{
			notes:   readyDigest(t, pubProofWrite(t, s, notes, "an operator's notes beside the coverage document\n")),
			foreign: readyDigest(t, pubProofWrite(t, s, foreign, "{\"runId\":\"run-2020-01-01-0001\"}\n")),
		}
		wantDirty := []string{" M " + pubProofJSON, " M " + pubProofMD}
		if lines := gitStatusLines(t, s.repo); !slices.Equal(lines, wantDirty) {
			t.Fatalf("git status = %v, want the run's own report dirty and both neighbours ignored: %v", lines, wantDirty)
		}

		out := pubProofFinalize(t, s, runC, freshFinalizeBody())
		wantRecovered := []recoveredFile{
			{Path: pubProofJSON, Action: recoveryRestored},
			{Path: pubProofMD, Action: recoveryRestored},
		}
		if !slices.Equal(out.Recovered, wantRecovered) {
			t.Errorf("recovered = %+v, want %+v", out.Recovered, wantRecovered)
		}
		for path, want := range neighbours {
			got, err := evidence.SHA256File(filepath.Join(s.repo, filepath.FromSlash(path)))
			if err != nil {
				t.Errorf("%s: %v; want a file the run never wrote left where it was", path, err)
				continue
			}
			if got != want {
				t.Errorf("%s digests to %s, want it untouched at %s", path, got, want)
			}
		}
		if lines := gitStatusLines(t, s.repo); !slices.Equal(lines, wantDirty) {
			t.Errorf("git status = %v, want only the report rewritten over its committed self: %v", lines, wantDirty)
		}
	})
}

// covers: SC-32
func TestPublication_CommitKeepsMeasuredRevision(t *testing.T) {
	s := readySubject(t, "not-applicable")
	// One policy identity for both runs: reuse compares the critic's
	// instructions by path and digest, so a second file would read as a changed
	// policy rather than as the same one.
	policy := reusePolicy(t)

	runD := liveRunID("pub-rev-was")
	traced := readyTrace(t, s, runD)
	readyGroups(t, s, runD, readDispatchInputs(t, traced.DispatchPath), policy)
	readyEnvironmentalVersionRow(t, s, runD)
	measured := subjectHead(t, s.repo)
	beforeD := pubProofVerificationRecords(readyResume(t, s, runD))

	partial := pubProofFinalize(t, s, runD, freshFinalizeBody())
	if partial.Verdict != report.VerdictPartial {
		t.Fatalf("verdict = %q, want %s; reasons=%+v", partial.Verdict, report.VerdictPartial, partial.Reasons)
	}
	if partial.Acknowledged {
		t.Errorf("acknowledged = true, want a PARTIAL nobody has accepted yet")
	}
	if partial.MeasuredCommit != measured {
		t.Errorf("measuredCommit = %q, want the commit the run traced %q", partial.MeasuredCommit, measured)
	}

	// Exactly the run's published paths are committed: the report, its Markdown
	// and every transcript the report names, and nothing else.
	rep := readPublishedReport(t, s.repo, partial.JSONPath)
	paths := []string{partial.JSONPath, partial.MarkdownPath}
	for _, ref := range rep.Transcripts {
		paths = append(paths, ref.Path)
	}
	mustGit(t, s.repo, append([]string{"add"}, paths...)...)
	mustGit(t, s.repo, "commit", "-q", "-m", "publish the run's verification")
	afterPublish := subjectHead(t, s.repo)
	if lines := gitStatusLines(t, s.repo); len(lines) != 0 {
		t.Fatalf("git status = %v, want the subject clean once the publication is committed", lines)
	}

	body := freshFinalizeBody()
	body.Acknowledgement = "PARTIAL accepted: mutation environment unavailable"
	out := pubProofFinalize(t, s, runD, body)
	if !out.Acknowledged {
		t.Errorf("acknowledged = false, want the acknowledgement the parent handed in to stand")
	}
	if out.MeasuredCommit != measured {
		t.Errorf("measuredCommit = %q, want the commit the run traced %q", out.MeasuredCommit, measured)
	}
	if acknowledged := readPublishedReport(t, s.repo, out.JSONPath); acknowledged.HeadCommit != measured {
		t.Errorf("report headCommit = %q, want the commit the run traced %q", acknowledged.HeadCommit, measured)
	}
	wantWork := report.WorkCounts{Fresh: 4, Reused: 0, Failed: 0, Incomplete: 1}
	if out.Work != wantWork {
		t.Errorf("work = %+v, want %+v", out.Work, wantWork)
	}

	state := readyResume(t, s, runD)
	if state.Stale {
		t.Errorf("stale = true (%s), want a publication-only commit to leave the run standing", state.StaleReason)
	}
	if state.HeadCommit != afterPublish {
		t.Errorf("headCommit = %q, want %q", state.HeadCommit, afterPublish)
	}
	if state.Publication.MeasuredCommit != measured {
		t.Errorf("publication.measuredCommit = %q, want %q", state.Publication.MeasuredCommit, measured)
	}
	if records := pubProofVerificationRecords(state); !slices.Equal(records, beforeD) {
		t.Errorf("records = %+v, want the two publications to have moved none of them: %+v", records, beforeD)
	}
	wantPublications := []runstate.Status{runstate.StatusSuperseded, runstate.StatusCompleted}
	if statuses := pubProofPublicationStatuses(state); !slices.Equal(statuses, wantPublications) {
		t.Errorf("publication records = %v, want %v", statuses, wantPublications)
	}

	// Anything the run's own publication does not account for is a different
	// subject, and the run that measured the old one says so.
	pubProofAppend(t, s, "ready/version.go", "\n// a later thought\n")
	mustGit(t, s.repo, "add", "-A")
	mustGit(t, s.repo, "commit", "-q", "-m", "a production change over the acknowledged report")
	moved := subjectHead(t, s.repo)

	refusal, _ := pubProofRefusal(t, s, runD, freshFinalizeBody(), "head-moved")
	for _, want := range []string{"ready/version.go", measured, moved} {
		if !strings.Contains(refusal.Message, want) {
			t.Errorf("message = %q, want it to name %s", refusal.Message, want)
		}
	}
	if moved := readyResume(t, s, runD); !moved.Stale {
		t.Errorf("stale = false after a production commit, want the run stale")
	}

	// The new target is a new run: it traces the new commit, adopts nothing the
	// change touched, and publishes a report that measures the commit it read.
	runE := liveRunID("pub-rev-now")
	tracedE := readyTrace(t, s, runE)
	evidenceE, err := report.LoadTrace(tracedE.EvidencePath)
	if err != nil {
		t.Fatalf("load the new run's trace evidence: %v", err)
	}
	if evidenceE.RepoCommit != moved {
		t.Errorf("trace evidence repoCommit = %q, want the commit it traced %q", evidenceE.RepoCommit, moved)
	}

	decisions := readyReuse(t, s, runE, runD, policy)
	offered := slices.Concat(decisions.Review, decisions.Mutation)
	if len(offered) == 0 {
		t.Fatalf("reuse offered no decision at all about %s", runD)
	}
	const wantReason = "module-changed:ready/version.go"
	for _, decision := range offered {
		if decision.Reused || decision.Reason != wantReason {
			t.Errorf("%s %s = %+v, want it refused %q", decision.Kind, decision.ID, decision, wantReason)
		}
	}

	readyGroups(t, s, runE, readDispatchInputs(t, tracedE.DispatchPath), policy)
	readyCaughtVersionRow(t, s, runE)

	fresh := pubProofFinalize(t, s, runE, freshFinalizeBodyWithPaths("ready/version.go"))
	if fresh.Verdict != report.VerdictPass {
		t.Errorf("verdict = %q, want %s; reasons=%+v", fresh.Verdict, report.VerdictPass, fresh.Reasons)
	}
	freshReport := readPublishedReport(t, s.repo, fresh.JSONPath)
	if freshReport.HeadCommit != moved {
		t.Errorf("report headCommit = %q, want the new commit %q", freshReport.HeadCommit, moved)
	}
	if len(freshReport.Work.Items) == 0 {
		t.Fatalf("the new run's report carries no work items at all")
	}
	for _, item := range freshReport.Work.Items {
		if item.Source != report.WorkFresh {
			t.Errorf("work item %s %s source = %q, want %q", item.Kind, item.ID, item.Source, report.WorkFresh)
		}
	}
}
