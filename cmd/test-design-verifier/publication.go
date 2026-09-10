package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"test-design/internal/evidence"
	"test-design/internal/runstate"
)

// This file holds what finalize knows about its own publication: which paths of
// the repository one run may write, which revision a report still measures once
// those paths have been committed, and how a publication that was interrupted
// is taken back before it is written again.
//
// Ownership is proved by digest against the run's own phase/publish record,
// path by path. A directory name, a file-name pattern or a timestamp proves
// nothing: a file this run did not write is not this run's to remove, and a
// file it did write whose bytes have changed is somebody's edit, not a
// half-finished publication.

// publishInputs is the phase/publish record's input identity: the revision the
// report measures, the run dir the report was written into, the report's
// digest, whether it carries an acknowledgement, and every file the publication
// writes with the digest of the bytes it writes — repo-relative slash paths,
// sorted. The record carries no artifacts: these files are the contract.
type publishInputs struct {
	MeasuredCommit string             `json:"measuredCommit"`
	RunDir         string             `json:"runDir"`
	ReportSHA256   string             `json:"reportSha256"`
	Acknowledged   bool               `json:"acknowledged"`
	Files          []evidence.FileRef `json:"files"`
}

// recoveredFile names one own artifact finalize put back before republishing.
type recoveredFile struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

// The two things recovery may do to a file it has proved this run's own.
const (
	recoveryRestored = "restored"
	recoveryRemoved  = "removed"
)

// republicationReason supersedes a completed publication when finalize is asked
// for the report again. Publishing twice is not repeating verification work, so
// it never asks the caller for a repeat reason.
const republicationReason = "republication"

// dirtyEntry is one porcelain v1 status line: the two-character code and the
// unquoted path.
type dirtyEntry struct{ Code, Path string }

// isPublicationPath reports whether path is one this run may publish:
// <dir>/<stem>-verification.json, <dir>/<stem>-verification.md, or a .json
// document directly under <dir>/transcripts/<runID>/ (dir "" = repository
// root).
//
// The exemption is the shape of a publication, not the directory it lands in.
// Every transcript this run writes is a JSON document it puts in that directory
// itself; a file of any other kind there — an executable above all — and
// anything in a subdirectory below it are changes to the subject the report
// describes, whoever committed them, and finalize never published them.
func isPublicationPath(artifactDir, stem, runID, path string) bool {
	if path == "" {
		return false
	}
	if path == repoPath(artifactDir, stem+"-verification.json") ||
		path == repoPath(artifactDir, stem+"-verification.md") {
		return true
	}
	// The trailing separator is what keeps another run whose id this one is a
	// prefix of — and the transcripts directory itself — outside the set.
	name, under := strings.CutPrefix(path, repoPath(artifactDir, "transcripts", runID)+"/")
	return under && name != "" && !strings.Contains(name, "/") && strings.HasSuffix(name, ".json")
}

// measuredCommit returns ledgerCommit when head == ledgerCommit, or when every
// path of changedPaths(ledgerCommit, head) is a publication path of this run
// (publication drift); otherwise "" and the first offending path in sorted
// order. err carries a git failure (unknown commit).
//
// Publishing a report is not a change to the thing the report describes, so a
// commit of nothing but this run's own artifacts leaves the measured revision
// where it was. Anything else — a production file, the coverage document,
// another run's transcripts — is a different subject.
func measuredCommit(repoRoot, artifactDir, stem, runID, ledgerCommit, head string) (commit, offending string, err error) {
	if head == ledgerCommit {
		return ledgerCommit, "", nil
	}
	changed, err := changedPaths(repoRoot, ledgerCommit, head)
	if err != nil {
		return "", "", err
	}
	for _, path := range changed {
		if !isPublicationPath(artifactDir, stem, runID, path) {
			return "", path, nil
		}
	}
	return ledgerCommit, "", nil
}

// dirtyEntries reads the working tree's status, sorted by path. The codes come
// with it: recovery answers for a modified file differently than for a deleted
// one, and dirtyPaths (which finalize's own gate uses for its message) drops
// them.
func dirtyEntries(repoRoot string) ([]dirtyEntry, error) {
	// core.quotePath=false keeps a non-ASCII path as the bytes it is rather
	// than an escaped rendering no digest or comparison would match.
	out, err := gitOutput(repoRoot, "--no-optional-locks", "-c", "core.quotePath=false",
		"status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	entries := []dirtyEntry{}
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		entries = append(entries, dirtyEntry{Code: line[:2], Path: line[3:]})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// recoverableCode reports whether a working-tree state is one recovery can
// undo: a file this run wrote over a tracked one, or one nothing tracks. A
// deletion, a rename, a staged addition and every conflict state are somebody
// else's decision about the repository, whoever wrote the file first.
func recoverableCode(code string) bool {
	switch code {
	case " M", "MM", "??":
		return true
	}
	return false
}

// recoverOwnPublication verifies first and acts second. Every dirty entry must
// be a path of rec's publishInputs.Files with a recoverable code whose bytes
// digest to the recorded value; any other entry refuses source-not-reproducible
// naming it, a recorded path whose bytes differ or cannot be read refuses
// publication-unverifiable naming it, and a nil rec with a dirty tree refuses
// source-not-reproducible. Every one of those leaves the tree exactly as it
// stands. Only when every entry verifies: a path tracked at HEAD is restored,
// an untracked one removed, and a directory this run's own files were the whole
// content of goes with them; a filesystem that refuses one of those returns
// recovery-failed with what had already been put back.
func recoverOwnPublication(op, repoRoot string, rec *runstate.Record) ([]recoveredFile, *cliError) {
	entries, err := dirtyEntries(repoRoot)
	if err != nil {
		// A status that cannot be read is not a tree anything can be proved
		// about, and the gate this recovery precedes would refuse it anyway.
		return nil, &cliError{
			Code:    "source-not-reproducible",
			Message: fmt.Sprintf("%s: the working tree's status could not be read, so nothing in it can be proved this run's own: %v", op, err),
		}
	}
	if len(entries) == 0 {
		return []recoveredFile{}, nil
	}

	recorded := map[string]string{}
	if rec != nil {
		var inputs publishInputs
		if err := recordInputs(rec, &inputs); err != nil {
			return nil, unreadableRecordError(op, rec, err)
		}
		for _, ref := range inputs.Files {
			recorded[ref.Path] = ref.SHA256
		}
	}

	// Verification runs to the end before anything is touched: one entry this
	// run cannot prove it wrote — or cannot decide what to do with — leaves the
	// whole tree as it stands. What comes out of this loop is the complete
	// plan, each entry already carrying the action it will be given.
	planned := make([]recoveredFile, 0, len(entries))
	for _, entry := range entries {
		want, ours := recorded[entry.Path]
		if !ours || !recoverableCode(entry.Code) {
			return nil, notOwnPathRefusal(op, entry, ours)
		}
		got, err := evidence.SHA256File(filepath.Join(repoRoot, filepath.FromSlash(entry.Path)))
		if err != nil {
			return nil, &cliError{
				Code:    codePublicationUnverifiable,
				Message: fmt.Sprintf("%s: %s is a file this run published but its bytes cannot be read, so nothing can be proved about them: %v. Nothing was touched", op, entry.Path, err),
			}
		}
		if got != want {
			return nil, &cliError{
				Code: codePublicationUnverifiable,
				Message: fmt.Sprintf("%s: %s digests to %s but this run published %s: the file is not what the run wrote, so recovery will not discard it. Restore or commit it, then rerun finalize. Nothing was touched",
					op, entry.Path, got, want),
			}
		}
		tracked, err := headTracked(repoRoot, entry.Path)
		if err != nil {
			return nil, &cliError{
				Code: "recovery-failed",
				Message: fmt.Sprintf("%s: whether HEAD carries %s could not be answered, and recovery will not remove a file on an unanswered question: %v. Nothing was touched; repair the repository and rerun finalize",
					op, entry.Path, err),
			}
		}
		action := recoveryRemoved
		if tracked {
			action = recoveryRestored
		}
		planned = append(planned, recoveredFile{Path: entry.Path, Action: action})
	}

	recovered := make([]recoveredFile, 0, len(planned))
	emptied := map[string]struct{}{}
	for _, file := range planned {
		full := filepath.Join(repoRoot, filepath.FromSlash(file.Path))
		if file.Action == recoveryRestored {
			// One path at a time: a checkout of the directory would take back
			// files this run never wrote.
			if _, err := gitOutput(repoRoot, "checkout", "--", file.Path); err != nil {
				return recovered, &cliError{
					Code:    "recovery-failed",
					Message: fmt.Sprintf("%s: restore %s: %v", op, file.Path, err),
				}
			}
			recovered = append(recovered, file)
			continue
		}
		if err := os.Remove(full); err != nil {
			return recovered, &cliError{
				Code:    "recovery-failed",
				Message: fmt.Sprintf("%s: remove %s: %v", op, file.Path, err),
			}
		}
		recovered = append(recovered, file)
		emptied[filepath.Dir(full)] = struct{}{}
	}

	// A directory whose whole content was this run's own goes with it —
	// <dir>/transcripts/<runID> in practice. os.Remove refuses a directory
	// anything still lives in, an ignored sentinel included, which is the whole
	// of the rule: recovery removes files, never trees.
	dirs := make([]string, 0, len(emptied))
	for dir := range emptied {
		dirs = append(dirs, dir)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, dir := range dirs {
		_ = os.Remove(dir)
	}
	return recovered, nil
}

// headTracked reports whether HEAD carries path. It is asked with ls-tree
// rather than with the package's plain trackedness query because recovery is
// the one caller whose answer decides between putting a file back and deleting
// it, and there the two failures must not look alike: `git cat-file -e` exits
// 128 both for a path HEAD does not carry and for a repository that could not
// be read at all, while ls-tree answers the first with an empty listing and the
// second by failing. A question nobody answered is not an absence.
func headTracked(repoRoot, path string) (bool, error) {
	// core.quotePath=false keeps a non-ASCII path as the bytes it is, so the
	// listing can be compared against the path that was asked for, and
	// --literal-pathspecs keeps a run ID or a document stem carrying a glob
	// character from asking about some other file: nothing here is a pattern.
	out, err := gitOutput(repoRoot, "--literal-pathspecs", "-c", "core.quotePath=false",
		"ls-tree", "--name-only", "HEAD", "--", path)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// notOwnPathRefusal names a dirty path recovery will not undo: one this run
// never published, or one it did whose working-tree state is not a publication
// left behind.
func notOwnPathRefusal(op string, entry dirtyEntry, ours bool) *cliError {
	what := "was not published by this run"
	if ours {
		what = fmt.Sprintf("is a file this run published but its working-tree state is %q, which recovery does not undo", entry.Code)
	}
	return &cliError{
		Code: "source-not-reproducible",
		Message: fmt.Sprintf("%s: source not reproducible: %s %s; commit or discard it, then rerun finalize. Nothing was touched",
			op, entry.Path, what),
	}
}

// recordFingerprint is one record reduced to what a report rests on: which
// obligation it answers, where it stands and what it answered. Nothing here is
// a timestamp or a path, so two fingerprints differ exactly when the ledger's
// account of the run's work differs.
type recordFingerprint struct {
	Seq     int
	Kind    runstate.Kind
	ID      string
	Status  runstate.Status
	Outcome string
}

func (f recordFingerprint) String() string {
	return fmt.Sprintf("seq %d %s %s %s %q", f.Seq, f.Kind, f.ID, f.Status, f.Outcome)
}

// ledgerFingerprint is a whole ledger's records in order, each fingerprinted.
type ledgerFingerprint []recordFingerprint

// fingerprintLedger takes the fingerprint of the ledger finalize assembled the
// report from, so the transaction that opens the publication can hold the
// ledger it re-reads under the lock to that same account.
func fingerprintLedger(l *runstate.Ledger) ledgerFingerprint {
	if l == nil {
		return ledgerFingerprint{}
	}
	f := make(ledgerFingerprint, 0, len(l.Records))
	for _, rec := range l.Records {
		f = append(f, recordFingerprint{
			Seq: rec.Seq, Kind: rec.Kind, ID: rec.ID, Status: rec.Status, Outcome: rec.Outcome,
		})
	}
	return f
}

// differsFrom names the first way now is not the ledger before was — the record
// count, the last record's seq, then each record in turn — and returns "" when
// the two accounts agree.
func (before ledgerFingerprint) differsFrom(now ledgerFingerprint) string {
	switch {
	case len(before) != len(now):
		return fmt.Sprintf("finalize read %d records ending at %s, and the ledger now holds %d ending at %s",
			len(before), before.last(), len(now), now.last())
	case before.lastSeq() != now.lastSeq():
		return fmt.Sprintf("the last record was %s and is now %s", before.last(), now.last())
	}
	for i, was := range before {
		if is := now[i]; is != was {
			return fmt.Sprintf("%s is now %s", was, is)
		}
	}
	return ""
}

func (f ledgerFingerprint) last() string {
	if len(f) == 0 {
		return "no record"
	}
	return f[len(f)-1].String()
}

func (f ledgerFingerprint) lastSeq() int {
	if len(f) == 0 {
		return 0
	}
	return f[len(f)-1].Seq
}

// startPublication opens the run's phase/publish record. A publication that was
// interrupted is restarted where it stands; one that completed is superseded
// with the republication reason, because publishing the report again repeats no
// verification work and so needs no repeat reason from the caller.
//
// read is the fingerprint of the ledger the report was assembled from. The
// ledger lives outside the repository, so the Git snapshot gate cannot see a
// concurrent caller repeating an obligation and answering it differently; this
// transaction holds the store lock, and is where the ledger that was read and
// the ledger that stands are held to each other. A ledger that moved refuses
// ledger-changed and writes nothing.
func startPublication(
	op string, store *runstate.Store, identity runstate.Identity, read ledgerFingerprint, inputs publishInputs,
) *cliError {
	detail := ""
	if inputs.Acknowledged {
		detail = "acknowledged"
	}
	err := store.Update(identity, func(l *runstate.Ledger) error {
		if differs := read.differsFrom(fingerprintLedger(l)); differs != "" {
			return &cliError{
				Code: codeLedgerChanged,
				Message: fmt.Sprintf("%s: the run's ledger changed since finalize read it (%s); rerun finalize",
					op, differs),
			}
		}
		repeat := ""
		if cur := runstate.Current(l, runstate.KindPhase, publishPhaseID); cur != nil && cur.Status == runstate.StatusCompleted {
			repeat = republicationReason
		}
		_, err := l.Start(runstate.KindPhase, publishPhaseID, inputs, detail, repeat)
		return err
	})
	return updateError(op, runstate.KindPhase, err)
}

// completePublication closes the record once every file landed. The record
// names no artifacts: the files it published are its inputs, digested as the
// bytes that were written, and they live in the repository rather than beside
// the ledger.
func completePublication(op string, store *runstate.Store, identity runstate.Identity) *cliError {
	if _, err := store.Complete(identity, runstate.KindPhase, publishPhaseID, "ok", nil, ""); err != nil {
		return ledgerError(op, runstate.KindPhase, err)
	}
	return nil
}
