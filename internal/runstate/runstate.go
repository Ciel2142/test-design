// Package runstate holds the executable-owned ledger of one verification run:
// the record of every obligation the run took on — a phase, a review group, a
// mutation row, a live attempt — as it moved from started to completed or was
// superseded by a later attempt. The ledger lives in its own directory outside
// the repository under test; every write transition takes an exclusive lock,
// re-reads the checkpoint, applies one transition and replaces the checkpoint
// atomically, so two concurrent callers never lose each other's record.
package runstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"test-design/internal/evidence"
)

// LedgerFile is the checkpoint's name inside the run's ledger directory.
const LedgerFile = "ledger.json"

// EnvStateDir overrides the verifier-owned state root the run directory is
// derived under.
const EnvStateDir = "TEST_DESIGN_STATE_DIR"

// stateDirName is the state root's directory name under the user cache dir.
const stateDirName = "test-design-verifier"

// Kind is the class of obligation a record tracks; with ID it is the record's key.
type Kind string

const (
	KindPhase       Kind = "phase"        // id "trace"
	KindReviewGroup Kind = "review-group" // id = the parent's group id
	KindMutationRow Kind = "mutation-row" // id = row id
	KindLiveAttempt Kind = "live-attempt" // id "live"
)

// Status is where a record stands: open, finished, or replaced by a later attempt.
type Status string

const (
	StatusStarted    Status = "started"
	StatusCompleted  Status = "completed"
	StatusSuperseded Status = "superseded"
)

// Identity is the run the ledger belongs to. It is written once, when the
// ledger is created, and every later transition must present the same one.
type Identity struct {
	RunID        string `json:"runID"`
	RepoRoot     string `json:"repoRoot"`
	RepoCommit   string `json:"repoCommit"`
	Source       string `json:"source"`
	SourceSHA256 string `json:"sourceSha256"`
}

// Artifact is one retained file a completed record points at, digested as it
// was at completion.
type Artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Record is one obligation's state. Seq is its 1-based position; the current
// record for a (Kind, ID) is the one not superseded. Inputs is the kind-specific
// immutable input identity captured at Start. Artifacts is filled at Complete,
// keyed by role.
type Record struct {
	Seq          int                 `json:"seq"`
	Kind         Kind                `json:"kind"`
	ID           string              `json:"id"`
	Status       Status              `json:"status"`
	Outcome      string              `json:"outcome,omitempty"`
	Inputs       json.RawMessage     `json:"inputs"`
	Artifacts    map[string]Artifact `json:"artifacts,omitempty"`
	Detail       string              `json:"detail,omitempty"`
	StartedAt    string              `json:"startedAt"`
	CompletedAt  string              `json:"completedAt,omitempty"`
	SupersededAt string              `json:"supersededAt,omitempty"`
	SupersededBy int                 `json:"supersededBy,omitempty"`
}

// Ledger is one run's whole checkpoint: its identity and every record it has
// opened, in the order they were opened.
type Ledger struct {
	SchemaVersion string   `json:"schemaVersion"`
	Identity      Identity `json:"identity"`
	Records       []Record `json:"records"`
}

var (
	ErrNoLedger            = errors.New("no ledger")
	ErrIdentityMismatch    = errors.New("ledger identity mismatch")
	ErrCompleted           = errors.New("obligation already completed")
	ErrNotStarted          = errors.New("obligation not started")
	ErrDuplicateCompletion = errors.New("duplicate completion")
)

// DefaultStateDir returns the verifier-owned state root: EnvStateDir when it
// holds a non-empty path, else <userCacheDir>/test-design-verifier.
func DefaultStateDir() (string, error) {
	if dir := os.Getenv(EnvStateDir); dir != "" {
		return dir, nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("default state dir: %w", err)
	}
	return filepath.Join(cache, stateDirName), nil
}

// Locate derives the run's ledger directory, <stateDir>/runs/<repoKey>/<runID>,
// where repoKey is the first 16 hex digits of SHA-256 over the resolved
// repository root. It validates runID as one path component, since joining it
// unchecked would let it place the ledger outside the state root.
func Locate(stateDir, repoRoot, runID string) (string, error) {
	if stateDir == "" {
		return "", errors.New("locate run state: state dir is empty")
	}
	if repoRoot == "" {
		return "", errors.New("locate run state: repo root is empty")
	}
	if err := validateRunID(runID); err != nil {
		return "", fmt.Errorf("locate run state: %w", err)
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("locate run state: resolve repo root %s: %w", repoRoot, err)
	}
	sum := sha256.Sum256([]byte(root))
	return filepath.Join(stateDir, "runs", hex.EncodeToString(sum[:])[:16], runID), nil
}

func validateRunID(runID string) error {
	if runID == "" {
		return errors.New("run ID is empty")
	}
	if filepath.IsAbs(runID) {
		return fmt.Errorf("run ID %q is not a single path component: absolute path", runID)
	}
	if strings.ContainsRune(runID, filepath.Separator) || strings.ContainsRune(runID, '/') {
		return fmt.Errorf("run ID %q is not a single path component: contains a path separator", runID)
	}
	if strings.Contains(runID, "..") {
		return fmt.Errorf("run ID %q is not a single path component: contains \"..\"", runID)
	}
	return nil
}

// Store is a handle on one ledger directory. Every method that writes takes the
// exclusive lock, re-reads the checkpoint, applies one transition, saves
// atomically, then unlocks.
type Store struct {
	dir string
}

// Open returns a Store on dir, creating the directory when it does not exist.
// Opening does not create the ledger; the first Start does.
func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("open run state: directory is empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("open run state %s: %w", dir, err)
	}
	return &Store{dir: dir}, nil
}

// Dir returns the ledger directory the Store is a handle on.
func (s *Store) Dir() string { return s.dir }

// Load reads the checkpoint under the shared lock. It reports ErrNoLedger when
// there is no ledger yet, and fails closed — returning no ledger at all — on
// malformed JSON, a foreign schemaVersion, or two current records for one
// (Kind, ID).
func (s *Store) Load() (*Ledger, error) {
	unlock, err := lockDir(s.dir, false)
	if err != nil {
		return nil, err
	}
	defer unlock()

	return loadLedger(s.dir)
}

// Start opens a started record for (kind, key) and returns it. With no current
// record it is simply new. Over a current started record the prior one is
// superseded with detail "restarted"; over a current completed record Start
// reports ErrCompleted unless repeat carries a reason, in which case the prior
// record is superseded with that reason as its detail. The first Start creates
// the ledger with id; a later Start presenting a different identity reports
// ErrIdentityMismatch.
func (s *Store) Start(id Identity, kind Kind, key string, inputs any, detail, repeat string) (*Record, error) {
	unlock, err := lockDir(s.dir, true)
	if err != nil {
		return nil, err
	}
	defer unlock()

	l, err := loadLedger(s.dir)
	switch {
	case errors.Is(err, ErrNoLedger):
		l = &Ledger{SchemaVersion: evidence.SchemaVersion, Identity: id, Records: []Record{}}
	case err != nil:
		return nil, err
	case l.Identity != id:
		return nil, fmt.Errorf("start %s/%s: %w", kind, key, ErrIdentityMismatch)
	}

	raw, err := json.Marshal(inputs)
	if err != nil {
		return nil, fmt.Errorf("start %s/%s: marshal inputs: %w", kind, key, err)
	}

	now := nowStamp()
	seq := len(l.Records) + 1

	if cur := Current(l, kind, key); cur != nil {
		switch cur.Status {
		case StatusStarted:
			supersede(cur, seq, now, "restarted")
		case StatusCompleted:
			if repeat == "" {
				return nil, fmt.Errorf("start %s/%s: %w", kind, key, ErrCompleted)
			}
			supersede(cur, seq, now, repeat)
		default:
			return nil, fmt.Errorf("start %s/%s: current record %d has unknown status %q", kind, key, cur.Seq, cur.Status)
		}
	}

	l.Records = append(l.Records, Record{
		Seq:       seq,
		Kind:      kind,
		ID:        key,
		Status:    StatusStarted,
		Inputs:    raw,
		Detail:    detail,
		StartedAt: now,
	})
	if err := saveLedger(s.dir, l); err != nil {
		return nil, err
	}

	started := l.Records[len(l.Records)-1]
	return &started, nil
}

// Complete finishes the current started record for (kind, key) with outcome and
// the digested artifacts (role → path; the SHA-256 is computed from the file as
// retained). A non-empty detail replaces the record's detail. It reports
// ErrNotStarted when there is no current record and ErrDuplicateCompletion when
// the current record is already completed — a completed record is never
// rewritten, only superseded.
func (s *Store) Complete(id Identity, kind Kind, key, outcome string, artifacts map[string]string, detail string) (*Record, error) {
	unlock, err := lockDir(s.dir, true)
	if err != nil {
		return nil, err
	}
	defer unlock()

	l, err := loadLedger(s.dir)
	if errors.Is(err, ErrNoLedger) {
		return nil, fmt.Errorf("complete %s/%s: %w", kind, key, ErrNotStarted)
	}
	if err != nil {
		return nil, err
	}
	if l.Identity != id {
		return nil, fmt.Errorf("complete %s/%s: %w", kind, key, ErrIdentityMismatch)
	}

	cur := Current(l, kind, key)
	switch {
	case cur == nil:
		return nil, fmt.Errorf("complete %s/%s: %w", kind, key, ErrNotStarted)
	case cur.Status == StatusCompleted:
		return nil, fmt.Errorf("complete %s/%s: %w", kind, key, ErrDuplicateCompletion)
	case cur.Status != StatusStarted:
		return nil, fmt.Errorf("complete %s/%s: current record %d has unknown status %q", kind, key, cur.Seq, cur.Status)
	}

	var digested map[string]Artifact
	if len(artifacts) > 0 {
		digested = make(map[string]Artifact, len(artifacts))
		for role, path := range artifacts {
			sum, err := evidence.SHA256File(path)
			if err != nil {
				return nil, fmt.Errorf("complete %s/%s: artifact %s: %w", kind, key, role, err)
			}
			digested[role] = Artifact{Path: path, SHA256: sum}
		}
	}

	cur.Status = StatusCompleted
	cur.Outcome = outcome
	cur.Artifacts = digested
	cur.CompletedAt = nowStamp()
	if detail != "" {
		cur.Detail = detail
	}

	if err := saveLedger(s.dir, l); err != nil {
		return nil, err
	}

	completed := *cur
	return &completed, nil
}

// Current returns the non-superseded record for (kind, key), nil when there is
// none. A valid ledger has at most one; Load refuses to return one that has more.
func Current(l *Ledger, kind Kind, key string) *Record {
	if l == nil {
		return nil
	}
	for i := range l.Records {
		if rec := &l.Records[i]; rec.Kind == kind && rec.ID == key && rec.Status != StatusSuperseded {
			return rec
		}
	}
	return nil
}

// Verify re-checks a completed record's artifacts: every path still exists and
// its SHA-256 still matches what was digested at completion. It returns the
// reasons the record is not valid, ordered by role; empty means valid. A file
// that cannot be read at all is artifact-missing:<role>, one whose bytes changed
// is artifact-digest-mismatch:<role>.
func Verify(rec *Record) []string {
	if rec == nil || len(rec.Artifacts) == 0 {
		return nil
	}

	roles := make([]string, 0, len(rec.Artifacts))
	for role := range rec.Artifacts {
		roles = append(roles, role)
	}
	sort.Strings(roles)

	var reasons []string
	for _, role := range roles {
		art := rec.Artifacts[role]
		sum, err := evidence.SHA256File(art.Path)
		if err != nil {
			reasons = append(reasons, "artifact-missing:"+role)
			continue
		}
		if sum != art.SHA256 {
			reasons = append(reasons, "artifact-digest-mismatch:"+role)
		}
	}
	return reasons
}

// failedOutcomes is the flat set of outcomes that count as a failure, across
// every kind. The four kinds' vocabularies are disjoint, so no outcome here is
// a success for some other kind; phase contributes none, since its only outcome
// is "ok".
var failedOutcomes = map[string]bool{
	"blocked":       true,
	"survived":      true,
	"unappliable":   true,
	"environmental": true,
	"fail":          true,
	"inconclusive":  true,
	"unavailable":   true,
}

// IsFailedOutcome reports whether outcome is a failing one. The set is flat: an
// outcome that fails for one kind is not silently forgiven under another, or
// under a kind string a future writer adds without updating this file. kind is
// part of the cross-wave signature and is reserved for a kind-specific
// vocabulary; today no kind narrows the answer.
func IsFailedOutcome(kind Kind, outcome string) bool {
	return failedOutcomes[outcome]
}

// supersede replaces rec's status with superseded, pointing at the record that
// takes over and recording why. Its outcome, artifacts and earlier timestamps
// stay as they were.
func supersede(rec *Record, by int, at, detail string) {
	rec.Status = StatusSuperseded
	rec.SupersededAt = at
	rec.SupersededBy = by
	rec.Detail = detail
}

func nowStamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}
