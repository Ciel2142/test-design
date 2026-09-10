package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"test-design/internal/evidence"
	"test-design/internal/runstate"
)

// These are the unit proofs of what finalize knows about its own publication:
// which repository paths one run may write, which revision a report still
// measures once those paths have been committed, and what recovery may take
// back. Each drives the real function over a real temporary repository — the
// questions are all about git and the filesystem, so a fake would prove
// nothing.

const (
	pubDir    = "docs/superpowers/coverage"
	pubStem   = "2026-08-30-subject"
	pubRunID  = "pub-run-7"
	pubJSON   = pubDir + "/" + pubStem + "-verification.json"
	pubMD     = pubDir + "/" + pubStem + "-verification.md"
	pubTrace  = pubDir + "/transcripts/" + pubRunID + "/trace-evidence.json"
	pubSource = pubDir + "/" + pubStem + "-coverage.md"
)

// pubRepo is a small committed repository: a coverage document, a production
// file and nothing else, so every dirty path a proof creates is one it made.
func pubRepo(t *testing.T) string {
	t.Helper()
	isolateEnv(t)
	repo := t.TempDir()
	pubWrite(t, repo, "go.mod", "module subject\n\ngo 1.26\n")
	pubWrite(t, repo, "orders/orders.go", "package orders\n\nfunc Total() int { return 1 }\n")
	pubWrite(t, repo, pubSource, "# Coverage\n")
	mustGit(t, repo, "init", "-q")
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "subject")
	return repo
}

func pubWrite(t *testing.T, repo, relPath, content string) string {
	t.Helper()
	full := filepath.Join(repo, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", full, err)
	}
	return full
}

func pubHead(t *testing.T, repo string) string {
	t.Helper()
	return strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))
}

// pubDigest is the digest of one repo-relative path as it stands on disk.
func pubDigest(t *testing.T, repo, relPath string) string {
	t.Helper()
	sum, err := evidence.SHA256File(filepath.Join(repo, filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatalf("SHA256File %s: %v", relPath, err)
	}
	return sum
}

// pubRecord is a phase/publish record naming files, as startPublication writes
// one: the proofs of recovery all turn on what this record claims.
func pubRecord(t *testing.T, files ...evidence.FileRef) *runstate.Record {
	t.Helper()
	raw, err := json.Marshal(publishInputs{
		MeasuredCommit: strings.Repeat("a", 40),
		RunDir:         t.TempDir(),
		ReportSHA256:   "report-digest",
		Files:          files,
	})
	if err != nil {
		t.Fatalf("marshal publishInputs: %v", err)
	}
	return &runstate.Record{
		Seq: 1, Kind: runstate.KindPhase, ID: publishPhaseID,
		Status: runstate.StatusStarted, Inputs: raw,
	}
}

// pubTreeDigests digests every file the repository holds outside .git, which
// is what a refusal that touches nothing leaves unchanged.
func pubTreeDigests(t *testing.T, repo string) map[string]string {
	t.Helper()
	digests := map[string]string{}
	err := filepath.WalkDir(repo, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(repo, path)
		if err != nil {
			return err
		}
		sum, err := evidence.SHA256File(path)
		if err != nil {
			return err
		}
		digests[filepath.ToSlash(rel)] = sum
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", repo, err)
	}
	return digests
}

func TestIsPublicationPath(t *testing.T) {
	for _, tc := range []struct {
		name, dir, path string
		want            bool
	}{
		{"the report JSON", pubDir, pubJSON, true},
		{"the report Markdown", pubDir, pubMD, true},
		{"this run's transcript", pubDir, pubTrace, true},
		{"another run's transcript", pubDir, pubDir + "/transcripts/other-run/trace-evidence.json", false},
		{"a run whose id this one prefixes", pubDir, pubDir + "/transcripts/" + pubRunID + "0/trace-evidence.json", false},
		{"the transcripts directory itself", pubDir, pubDir + "/transcripts/" + pubRunID, false},
		{"an executable committed among this run's transcripts", pubDir, pubDir + "/transcripts/" + pubRunID + "/x_test.go", false},
		{"a transcript-shaped file below this run's transcripts", pubDir, pubDir + "/transcripts/" + pubRunID + "/sub/a.json", false},
		// A row ID may name an area ("orders/SC-05"), and a mutation
		// transcript carries the row ID in its file name, so this run's own
		// document lands one directory down.
		{"an area-prefixed mutation transcript", pubDir, pubDir + "/transcripts/" + pubRunID + "/mutation-orders/SC-05.json", true},
		{"an area-prefixed mutation attempt transcript", pubDir, pubDir + "/transcripts/" + pubRunID + "/mutation-attempt-orders/SC-05.json", true},
		{"an area-prefixed adopted mutation transcript", pubDir, pubDir + "/transcripts/" + pubRunID + "/reused-mutation-unappliable-orders/SC-05.json", true},
		{"a row-shaped path under no transcript name", pubDir, pubDir + "/transcripts/" + pubRunID + "/orders/SC-05.json", false},
		{"an executable inside a mutation transcript's area", pubDir, pubDir + "/transcripts/" + pubRunID + "/mutation-orders/x_test.go", false},
		{"a mutation transcript nested deeper than a row area", pubDir, pubDir + "/transcripts/" + pubRunID + "/mutation-orders/deeper/SC-05.json", false},
		{"the coverage source", pubDir, pubSource, false},
		{"a sibling report", pubDir, pubDir + "/2026-08-30-other-verification.json", false},
		{"a production file", pubDir, "orders/orders.go", false},
		{"the report JSON at the repository root", "", pubStem + "-verification.json", true},
		{"the report Markdown at the repository root", "", pubStem + "-verification.md", true},
		{"a transcript at the repository root", "", "transcripts/" + pubRunID + "/live-evidence.json", true},
		{"a production file with an empty artifact dir", "", "orders/orders.go", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPublicationPath(tc.dir, pubStem, pubRunID, tc.path); got != tc.want {
				t.Errorf("isPublicationPath(%q, %q, %q, %q) = %v, want %v", tc.dir, pubStem, pubRunID, tc.path, got, tc.want)
			}
		})
	}
}

func TestMeasuredCommit_PublicationDriftKeepsTheRunsCommit(t *testing.T) {
	t.Run("HEAD is the run's own commit", func(t *testing.T) {
		repo := pubRepo(t)
		head := pubHead(t, repo)

		commit, offending, err := measuredCommit(repo, pubDir, pubStem, pubRunID, head, head)

		if err != nil || offending != "" || commit != head {
			t.Fatalf("measuredCommit = (%q, %q, %v), want (%q, \"\", nil)", commit, offending, err, head)
		}
	})

	t.Run("a commit of this run's publication alone", func(t *testing.T) {
		repo := pubRepo(t)
		opened := pubHead(t, repo)
		pubWrite(t, repo, pubJSON, "{}\n")
		pubWrite(t, repo, pubMD, "# report\n")
		pubWrite(t, repo, pubTrace, "{}\n")
		mustGit(t, repo, "add", pubJSON, pubMD, pubTrace)
		mustGit(t, repo, "commit", "-q", "-m", "publish")

		commit, offending, err := measuredCommit(repo, pubDir, pubStem, pubRunID, opened, pubHead(t, repo))

		if err != nil || offending != "" || commit != opened {
			t.Fatalf("measuredCommit = (%q, %q, %v), want the run's own commit %s", commit, offending, err, opened)
		}
	})

	t.Run("a production commit names the path", func(t *testing.T) {
		repo := pubRepo(t)
		opened := pubHead(t, repo)
		pubWrite(t, repo, "go.mod", "module subject\n\ngo 1.26\n// changed\n")
		mustGit(t, repo, "add", "go.mod")
		mustGit(t, repo, "commit", "-q", "-m", "production change")

		commit, offending, err := measuredCommit(repo, pubDir, pubStem, pubRunID, opened, pubHead(t, repo))

		if err != nil {
			t.Fatalf("measuredCommit err = %v", err)
		}
		if commit != "" || offending != "go.mod" {
			t.Errorf("measuredCommit = (%q, %q), want (\"\", \"go.mod\")", commit, offending)
		}
	})

	t.Run("another run's transcript names the path", func(t *testing.T) {
		repo := pubRepo(t)
		opened := pubHead(t, repo)
		other := pubDir + "/transcripts/other-run/trace-evidence.json"
		pubWrite(t, repo, pubJSON, "{}\n")
		pubWrite(t, repo, other, "{}\n")
		mustGit(t, repo, "add", pubJSON, other)
		mustGit(t, repo, "commit", "-q", "-m", "another run's publication")

		commit, offending, err := measuredCommit(repo, pubDir, pubStem, pubRunID, opened, pubHead(t, repo))

		if err != nil {
			t.Fatalf("measuredCommit err = %v", err)
		}
		if commit != "" || offending != other {
			t.Errorf("measuredCommit = (%q, %q), want (\"\", %q)", commit, offending, other)
		}
	})

	// A run's transcripts directory is a place this run wrote JSON documents
	// into, not a place anything may be committed to: an executable landing
	// there is a change to the subject, whoever put it there.
	t.Run("an executable committed under this run's transcripts names the path", func(t *testing.T) {
		repo := pubRepo(t)
		opened := pubHead(t, repo)
		planted := pubDir + "/transcripts/" + pubRunID + "/x_test.go"
		pubWrite(t, repo, pubTrace, "{}\n")
		pubWrite(t, repo, planted, "package x\n")
		mustGit(t, repo, "add", pubTrace, planted)
		mustGit(t, repo, "commit", "-q", "-m", "a test file among the transcripts")

		commit, offending, err := measuredCommit(repo, pubDir, pubStem, pubRunID, opened, pubHead(t, repo))

		if err != nil {
			t.Fatalf("measuredCommit err = %v", err)
		}
		if commit != "" || offending != planted {
			t.Errorf("measuredCommit = (%q, %q), want (\"\", %q)", commit, offending, planted)
		}
	})

	// The other side of the same rule: a publication whose mutation row carried
	// an area prefix put one of its own transcripts a directory down, and
	// committing it is still this run publishing itself.
	t.Run("a commit of an area-prefixed mutation transcript keeps the run's commit", func(t *testing.T) {
		repo := pubRepo(t)
		opened := pubHead(t, repo)
		areaRow := pubDir + "/transcripts/" + pubRunID + "/mutation-orders/SC-05.json"
		pubWrite(t, repo, pubJSON, "{}\n")
		pubWrite(t, repo, pubMD, "# report\n")
		pubWrite(t, repo, pubTrace, "{}\n")
		pubWrite(t, repo, areaRow, "{}\n")
		mustGit(t, repo, "add", pubJSON, pubMD, pubTrace, areaRow)
		mustGit(t, repo, "commit", "-q", "-m", "publish an area-prefixed row")

		commit, offending, err := measuredCommit(repo, pubDir, pubStem, pubRunID, opened, pubHead(t, repo))

		if err != nil || offending != "" || commit != opened {
			t.Fatalf("measuredCommit = (%q, %q, %v), want the run's own commit %s", commit, offending, err, opened)
		}
	})

	t.Run("a commit this repository never had", func(t *testing.T) {
		repo := pubRepo(t)

		commit, offending, err := measuredCommit(repo, pubDir, pubStem, pubRunID, strings.Repeat("b", 40), pubHead(t, repo))

		if err == nil {
			t.Fatalf("measuredCommit = (%q, %q, nil), want an error for an unknown commit", commit, offending)
		}
		if commit != "" || offending != "" {
			t.Errorf("measuredCommit = (%q, %q), want both empty beside the error", commit, offending)
		}
	})
}

func TestDirtyEntries_NamesEveryStatusCodeInPathOrder(t *testing.T) {
	repo := pubRepo(t)
	pubWrite(t, repo, "alpha.txt", "alpha\n")
	pubWrite(t, repo, "delta.txt", "delta\n")
	mustGit(t, repo, "add", "alpha.txt", "delta.txt")
	mustGit(t, repo, "commit", "-q", "-m", "two more files")

	// One of each shape the working tree can present: a staged change, a
	// worktree change, a deletion and a file git has never seen.
	pubWrite(t, repo, "alpha.txt", "alpha staged\n")
	mustGit(t, repo, "add", "alpha.txt")
	pubWrite(t, repo, "orders/orders.go", "package orders\n\nfunc Total() int { return 2 }\n")
	if err := os.Remove(filepath.Join(repo, "delta.txt")); err != nil {
		t.Fatalf("Remove delta.txt: %v", err)
	}
	pubWrite(t, repo, "beta.txt", "beta\n")

	entries, err := dirtyEntries(repo)

	if err != nil {
		t.Fatalf("dirtyEntries: %v", err)
	}
	want := []dirtyEntry{
		{Code: "M ", Path: "alpha.txt"},
		{Code: "??", Path: "beta.txt"},
		{Code: " D", Path: "delta.txt"},
		{Code: " M", Path: "orders/orders.go"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("dirtyEntries = %+v, want %+v", entries, want)
	}
}

func TestDirtyEntries_CleanTreeHasNone(t *testing.T) {
	repo := pubRepo(t)

	entries, err := dirtyEntries(repo)

	if err != nil {
		t.Fatalf("dirtyEntries: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("dirtyEntries = %+v, want none for a clean tree", entries)
	}
}

// TestRecoverOwnPublication_RestoresTrackedAndRemovesUntracked is the whole of
// recovery's happy path: a published file the repository already carries is put
// back to what HEAD holds, one it does not is deleted, and the run's own
// transcripts directory goes with the last file in it.
func TestRecoverOwnPublication_RestoresTrackedAndRemovesUntracked(t *testing.T) {
	repo := pubRepo(t)
	// The first publication is committed, so its two report files are tracked.
	pubWrite(t, repo, pubJSON, "{\"finishedAt\":\"first\"}\n")
	pubWrite(t, repo, pubMD, "# first\n")
	mustGit(t, repo, "add", pubJSON, pubMD)
	mustGit(t, repo, "commit", "-q", "-m", "publish")
	committedJSON := pubDigest(t, repo, pubJSON)
	committedMD := pubDigest(t, repo, pubMD)

	// The second publication rewrote both and added a transcript nothing
	// tracks; that is what the record names.
	pubWrite(t, repo, pubJSON, "{\"finishedAt\":\"second\"}\n")
	pubWrite(t, repo, pubMD, "# second\n")
	pubWrite(t, repo, pubTrace, "{\"trace\":true}\n")
	rec := pubRecord(t,
		evidence.FileRef{Path: pubJSON, SHA256: pubDigest(t, repo, pubJSON)},
		evidence.FileRef{Path: pubMD, SHA256: pubDigest(t, repo, pubMD)},
		evidence.FileRef{Path: pubTrace, SHA256: pubDigest(t, repo, pubTrace)},
	)

	recovered, cliErr := recoverOwnPublication("finalize", repo, rec)

	if cliErr != nil {
		t.Fatalf("recoverOwnPublication: %s: %s", cliErr.Code, cliErr.Message)
	}
	want := []recoveredFile{
		{Path: pubJSON, Action: "restored"},
		{Path: pubMD, Action: "restored"},
		{Path: pubTrace, Action: "removed"},
	}
	if !reflect.DeepEqual(recovered, want) {
		t.Errorf("recovered = %+v, want %+v", recovered, want)
	}
	if got := pubDigest(t, repo, pubJSON); got != committedJSON {
		t.Errorf("%s was not restored to the committed bytes", pubJSON)
	}
	if got := pubDigest(t, repo, pubMD); got != committedMD {
		t.Errorf("%s was not restored to the committed bytes", pubMD)
	}
	if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(pubTrace))); !os.IsNotExist(err) {
		t.Errorf("stat %s = %v, want the untracked transcript removed", pubTrace, err)
	}
	transcripts := filepath.Join(repo, filepath.FromSlash(pubDir+"/transcripts/"+pubRunID))
	if _, err := os.Stat(transcripts); !os.IsNotExist(err) {
		t.Errorf("stat %s = %v, want the emptied transcripts directory removed", transcripts, err)
	}
	if lines := gitStatusLines(t, repo); len(lines) != 0 {
		t.Errorf("git status = %v, want recovery to have left a clean tree", lines)
	}
}

// TestRecoverOwnPublication_RefusesWhatItCannotProveOwn is the other half:
// every path the record does not vouch for, by name or by bytes, stops
// recovery before it touches anything.
func TestRecoverOwnPublication_RefusesWhatItCannotProveOwn(t *testing.T) {
	for _, tc := range []struct {
		name string
		// plant leaves the working tree in the shape under test and returns
		// the record finalize would have written and the path the refusal must
		// name.
		plant func(t *testing.T, repo string) (*runstate.Record, string)
		code  string
	}{
		{
			name: "an untracked file this run never published",
			plant: func(t *testing.T, repo string) (*runstate.Record, string) {
				pubWrite(t, repo, pubTrace, "{\"trace\":true}\n")
				notes := pubDir + "/transcripts/" + pubRunID + "/notes.txt"
				pubWrite(t, repo, notes, "scratch\n")
				return pubRecord(t, evidence.FileRef{Path: pubTrace, SHA256: pubDigest(t, repo, pubTrace)}), notes
			},
			code: "source-not-reproducible",
		},
		{
			name: "a tracked file this run never published",
			plant: func(t *testing.T, repo string) (*runstate.Record, string) {
				pubWrite(t, repo, pubTrace, "{\"trace\":true}\n")
				pubWrite(t, repo, "go.mod", "module subject\n\ngo 1.26\n// edited\n")
				return pubRecord(t, evidence.FileRef{Path: pubTrace, SHA256: pubDigest(t, repo, pubTrace)}), "go.mod"
			},
			code: "source-not-reproducible",
		},
		{
			name: "a published file deleted from the working tree",
			plant: func(t *testing.T, repo string) (*runstate.Record, string) {
				pubWrite(t, repo, pubJSON, "{\"finishedAt\":\"first\"}\n")
				mustGit(t, repo, "add", pubJSON)
				mustGit(t, repo, "commit", "-q", "-m", "publish")
				digest := pubDigest(t, repo, pubJSON)
				if err := os.Remove(filepath.Join(repo, filepath.FromSlash(pubJSON))); err != nil {
					t.Fatalf("Remove %s: %v", pubJSON, err)
				}
				return pubRecord(t, evidence.FileRef{Path: pubJSON, SHA256: digest}), pubJSON
			},
			code: "source-not-reproducible",
		},
		{
			name: "a published file whose bytes were edited",
			plant: func(t *testing.T, repo string) (*runstate.Record, string) {
				pubWrite(t, repo, pubMD, "# published\n")
				recorded := pubDigest(t, repo, pubMD)
				pubWrite(t, repo, pubMD, "# published\nedited by hand\n")
				return pubRecord(t, evidence.FileRef{Path: pubMD, SHA256: recorded}), pubMD
			},
			code: codePublicationUnverifiable,
		},
		{
			name: "a dirty tree with no publication record at all",
			plant: func(t *testing.T, repo string) (*runstate.Record, string) {
				pubWrite(t, repo, pubJSON, "{}\n")
				return nil, pubJSON
			},
			code: "source-not-reproducible",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := pubRepo(t)
			rec, named := tc.plant(t, repo)
			before := pubTreeDigests(t, repo)
			status := gitStatusLines(t, repo)

			recovered, cliErr := recoverOwnPublication("finalize", repo, rec)

			if cliErr == nil {
				t.Fatalf("recoverOwnPublication = %+v, nil, want the %s refusal", recovered, tc.code)
			}
			if cliErr.Code != tc.code {
				t.Errorf("refusal code = %q, want %q; message=%s", cliErr.Code, tc.code, cliErr.Message)
			}
			if !strings.Contains(cliErr.Message, named) {
				t.Errorf("message = %q, want it to name %s", cliErr.Message, named)
			}
			if after := pubTreeDigests(t, repo); !reflect.DeepEqual(after, before) {
				t.Errorf("tree digests = %+v, want the refusal to have touched nothing: %+v", after, before)
			}
			if got := gitStatusLines(t, repo); !reflect.DeepEqual(got, status) {
				t.Errorf("git status = %v, want it unchanged: %v", got, status)
			}
		})
	}
}

// TestRecoverOwnPublication_CleanTreeRecoversNothing keeps the ordinary first
// publication cheap: with nothing dirty there is nothing to prove and nothing
// to undo.
func TestRecoverOwnPublication_CleanTreeRecoversNothing(t *testing.T) {
	repo := pubRepo(t)

	recovered, cliErr := recoverOwnPublication("finalize", repo, pubRecord(t))

	if cliErr != nil {
		t.Fatalf("recoverOwnPublication: %s: %s", cliErr.Code, cliErr.Message)
	}
	if len(recovered) != 0 {
		t.Errorf("recovered = %+v, want none", recovered)
	}
	if recovered == nil {
		t.Error("recovered = nil, want an empty list so the output never carries null")
	}
}

// TestRecoverOwnPublication_KeepsAnIgnoredFileAndTheDirectoryHoldingIt is the
// boundary the removal rule turns on: recovery deletes files, never
// directories with anything else in them.
func TestRecoverOwnPublication_KeepsAnIgnoredFileAndTheDirectoryHoldingIt(t *testing.T) {
	repo := pubRepo(t)
	exclude := filepath.Join(repo, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(exclude, []byte("scratch.log\n"), 0o644); err != nil {
		t.Fatalf("WriteFile exclude: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(exclude) })

	pubWrite(t, repo, pubTrace, "{\"trace\":true}\n")
	sentinel := pubDir + "/transcripts/" + pubRunID + "/scratch.log"
	pubWrite(t, repo, sentinel, "kept\n")
	sentinelDigest := pubDigest(t, repo, sentinel)
	rec := pubRecord(t, evidence.FileRef{Path: pubTrace, SHA256: pubDigest(t, repo, pubTrace)})

	recovered, cliErr := recoverOwnPublication("finalize", repo, rec)

	if cliErr != nil {
		t.Fatalf("recoverOwnPublication: %s: %s", cliErr.Code, cliErr.Message)
	}
	want := []recoveredFile{{Path: pubTrace, Action: "removed"}}
	if !reflect.DeepEqual(recovered, want) {
		t.Errorf("recovered = %+v, want %+v", recovered, want)
	}
	if got := pubDigest(t, repo, sentinel); got != sentinelDigest {
		t.Errorf("%s digest = %q, want the ignored file byte-identical (%q)", sentinel, got, sentinelDigest)
	}
}

// TestRecoverOwnPublication_TrackednessDecidesOnlyWhenItIsAnswered guards the
// one question in recovery whose answer decides between restoring a file and
// deleting it. A git query that fails is not the same as one that says "HEAD
// does not carry this", and reading the first as the second would delete a file
// on no evidence, leaving a deletion the next run refuses to recover from.
func TestRecoverOwnPublication_TrackednessDecidesOnlyWhenItIsAnswered(t *testing.T) {
	t.Run("unanswerable refuses before anything is touched", func(t *testing.T) {
		isolateEnv(t)
		repo := t.TempDir()
		// A repository with no commit at all: `git status` answers, so recovery
		// gets as far as the question, and `git ls-tree HEAD` cannot answer it.
		mustGit(t, repo, "init", "-q")
		pubWrite(t, repo, pubJSON, "{\"finishedAt\":\"first\"}\n")
		rec := pubRecord(t, evidence.FileRef{Path: pubJSON, SHA256: pubDigest(t, repo, pubJSON)})
		before := pubTreeDigests(t, repo)

		recovered, cliErr := recoverOwnPublication("finalize", repo, rec)

		if cliErr == nil {
			t.Fatalf("recoverOwnPublication = %+v, nil, want the recovery-failed refusal", recovered)
		}
		if cliErr.Code != "recovery-failed" {
			t.Errorf("refusal code = %q, want %q; message=%s", cliErr.Code, "recovery-failed", cliErr.Message)
		}
		if !strings.Contains(cliErr.Message, pubJSON) {
			t.Errorf("message = %q, want it to name %s", cliErr.Message, pubJSON)
		}
		if after := pubTreeDigests(t, repo); !reflect.DeepEqual(after, before) {
			t.Errorf("tree digests = %+v, want the refusal to have touched nothing: %+v", after, before)
		}
	})

	t.Run("answered tracked is restored, never removed", func(t *testing.T) {
		repo := pubRepo(t)
		pubWrite(t, repo, pubJSON, "{\"finishedAt\":\"first\"}\n")
		mustGit(t, repo, "add", pubJSON)
		mustGit(t, repo, "commit", "-q", "-m", "publish")
		committed := pubDigest(t, repo, pubJSON)
		pubWrite(t, repo, pubJSON, "{\"finishedAt\":\"second\"}\n")
		rec := pubRecord(t, evidence.FileRef{Path: pubJSON, SHA256: pubDigest(t, repo, pubJSON)})

		recovered, cliErr := recoverOwnPublication("finalize", repo, rec)

		if cliErr != nil {
			t.Fatalf("recoverOwnPublication: %s: %s", cliErr.Code, cliErr.Message)
		}
		want := []recoveredFile{{Path: pubJSON, Action: "restored"}}
		if !reflect.DeepEqual(recovered, want) {
			t.Fatalf("recovered = %+v, want %+v", recovered, want)
		}
		if got := pubDigest(t, repo, pubJSON); got != committed {
			t.Errorf("%s digest = %q, want the committed bytes %q", pubJSON, got, committed)
		}
	})
}

// pubLedger opens a ledger holding one completed review group — the shape
// finalize loads before it assembles a report — and returns the store and the
// identity every later transition must present.
func pubLedger(t *testing.T) (*runstate.Store, runstate.Identity) {
	t.Helper()
	store, err := runstate.Open(filepath.Join(t.TempDir(), "ledger"))
	if err != nil {
		t.Fatalf("runstate.Open: %v", err)
	}
	identity := runstate.Identity{
		RunID: pubRunID, RepoRoot: t.TempDir(), RepoCommit: strings.Repeat("a", 40),
		Source: pubSource, SourceSHA256: strings.Repeat("b", 64),
	}
	if err := store.Update(identity, func(l *runstate.Ledger) error {
		if _, err := l.Start(runstate.KindReviewGroup, "g1", reviewGroupInputs{Rows: []string{"SC-01"}}, "", ""); err != nil {
			return err
		}
		_, err := l.Complete(runstate.KindReviewGroup, "g1", "holds", nil, "")
		return err
	}); err != nil {
		t.Fatalf("seed ledger: %v", err)
	}
	return store, identity
}

// TestStartPublication_RefusesALedgerThatChangedSinceFinalizeReadIt is the
// publication's own read-your-writes rule. The report finalize is about to
// publish was assembled from the ledger it loaded; the ledger lives outside the
// repository, so another caller repeating a review group and closing it WEAK
// between that load and the publish record is invisible to the Git snapshot
// gate. The transaction that opens the publication holds the store lock, and is
// therefore the only place the change can be caught.
func TestStartPublication_RefusesALedgerThatChangedSinceFinalizeReadIt(t *testing.T) {
	t.Run("a ledger changed under finalize is refused", func(t *testing.T) {
		store, identity := pubLedger(t)
		loaded, err := store.Load()
		if err != nil {
			t.Fatalf("load ledger: %v", err)
		}
		read := fingerprintLedger(loaded)

		// Another caller repeats the group and closes it WEAK.
		if err := store.Update(identity, func(l *runstate.Ledger) error {
			if _, err := l.Start(runstate.KindReviewGroup, "g1",
				reviewGroupInputs{Rows: []string{"SC-01"}}, "", "second opinion"); err != nil {
				return err
			}
			_, err := l.Complete(runstate.KindReviewGroup, "g1", "weak", nil, "")
			return err
		}); err != nil {
			t.Fatalf("repeat the group: %v", err)
		}
		changed, err := store.Load()
		if err != nil {
			t.Fatalf("load the changed ledger: %v", err)
		}

		cliErr := startPublication("finalize", store, identity, read, publishInputs{MeasuredCommit: identity.RepoCommit})

		if cliErr == nil {
			t.Fatal("startPublication = nil, want the ledger-changed refusal")
		}
		if cliErr.Code != "ledger-changed" {
			t.Errorf("refusal code = %q, want %q; message=%s", cliErr.Code, "ledger-changed", cliErr.Message)
		}
		for _, want := range []string{"review-group", "g1", "weak", "rerun finalize"} {
			if !strings.Contains(cliErr.Message, want) {
				t.Errorf("message = %q, want it to name %s", cliErr.Message, want)
			}
		}
		after, err := store.Load()
		if err != nil {
			t.Fatalf("load ledger after the refusal: %v", err)
		}
		if rec := runstate.Current(after, runstate.KindPhase, publishPhaseID); rec != nil {
			t.Errorf("phase/%s record = %+v, want a refused publication to have written none", publishPhaseID, rec)
		}
		if !reflect.DeepEqual(after.Records, changed.Records) {
			t.Errorf("records = %+v, want the refusal to have left the ledger as it stood: %+v", after.Records, changed.Records)
		}
	})

	t.Run("an unchanged ledger starts the record", func(t *testing.T) {
		store, identity := pubLedger(t)
		loaded, err := store.Load()
		if err != nil {
			t.Fatalf("load ledger: %v", err)
		}
		inputs := publishInputs{
			MeasuredCommit: identity.RepoCommit,
			RunDir:         t.TempDir(),
			ReportSHA256:   strings.Repeat("c", 64),
			Acknowledged:   true,
			Files:          []evidence.FileRef{{Path: pubJSON, SHA256: strings.Repeat("d", 64)}},
		}

		if cliErr := startPublication("finalize", store, identity, fingerprintLedger(loaded), inputs); cliErr != nil {
			t.Fatalf("startPublication: %s: %s", cliErr.Code, cliErr.Message)
		}

		after, err := store.Load()
		if err != nil {
			t.Fatalf("load ledger: %v", err)
		}
		rec := runstate.Current(after, runstate.KindPhase, publishPhaseID)
		if rec == nil {
			t.Fatalf("records = %+v, want a started phase/%s record", after.Records, publishPhaseID)
		}
		if rec.Status != runstate.StatusStarted || rec.Detail != "acknowledged" {
			t.Errorf("record = %+v, want it started with detail %q", rec, "acknowledged")
		}
		var got publishInputs
		if err := recordInputs(rec, &got); err != nil {
			t.Fatalf("decode publish inputs: %v", err)
		}
		if !reflect.DeepEqual(got, inputs) {
			t.Errorf("publish inputs = %+v, want %+v", got, inputs)
		}
	})
}
