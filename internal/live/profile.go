package live

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"test-design/internal/evidence"
)

type Profile struct {
	SchemaVersion         string      `json:"schemaVersion"`
	Classification        string      `json:"classification"`
	Start                 [][]string  `json:"start"`
	Health                [][]string  `json:"health"`
	Walkthrough           [][]string  `json:"walkthrough"`
	Cleanup               [][]string  `json:"cleanup"`
	CleanupEvidence       []Predicate `json:"cleanupEvidence"`
	EnvAllowlist          []string    `json:"envAllowlist"`
	ResourcePrefixes      []string    `json:"resourcePrefixes"`
	Endpoints             []string    `json:"endpoints"`
	TimeoutSeconds        int64       `json:"timeoutSeconds,omitempty"`
	CleanupTimeoutSeconds int64       `json:"cleanupTimeoutSeconds,omitempty"`
	MaxOutputBytes        int64       `json:"maxOutputBytes,omitempty"`
}

type Predicate struct {
	Form   string `json:"form"`
	Stream string `json:"stream"`
	Value  string `json:"value"`
}

const (
	defaultTimeout              = 120 * time.Second
	defaultCleanupTimeout       = 30 * time.Second
	defaultMaxOutputBytes int64 = 16777216
)

type Limits struct {
	Timeout     time.Duration
	OutputBytes int64
}

// Limits returns independent deadlines for ordinary and cleanup commands.
// ValidateTemplate or ValidateResolved must succeed before a profile is used
// for execution.
func (p *Profile) Limits(cleanup bool) Limits {
	seconds, timeout := p.TimeoutSeconds, defaultTimeout
	if cleanup {
		seconds, timeout = p.CleanupTimeoutSeconds, defaultCleanupTimeout
	}
	if seconds > 0 {
		timeout = time.Duration(seconds) * time.Second
	}
	output := p.MaxOutputBytes
	if output == 0 {
		output = defaultMaxOutputBytes
	}
	return Limits{Timeout: timeout, OutputBytes: output}
}

// LoadProfile fails closed: a missing or unreadable file, malformed JSON, or
// an unsupported SchemaVersion each report an error naming the path rather
// than returning a Profile a caller might mistake for valid.
func LoadProfile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load live profile %s: %w", path, err)
	}

	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("load live profile %s: malformed JSON: %w", path, err)
	}
	if p.SchemaVersion != evidence.SchemaVersion {
		return nil, fmt.Errorf("load live profile %s: schemaVersion %q, want %q", path, p.SchemaVersion, evidence.SchemaVersion)
	}
	return &p, nil
}

// runIDToken is the placeholder a committed profile writes wherever the run
// identifier belongs; prepare-live replaces it with the run ID it minted.
const runIDToken = "{run-id}"

// ValidateTemplate checks a profile as committed: the fail-closed invariants
// every live run needs, plus the requirement that each resourcePrefixes entry
// still carries the {run-id} token, so no committed profile can name a
// resource shared between runs.
func (p *Profile) ValidateTemplate() error {
	if err := p.validateInvariants(); err != nil {
		return err
	}

	for _, entry := range p.ResourcePrefixes {
		if !strings.Contains(entry, runIDToken) {
			return fmt.Errorf("live profile: run-id-missing-from-resources %s", entry)
		}
	}

	return p.validateCleanup()
}

// ValidateResolved checks the descriptor prepare-live produced for runID: the
// same invariants as the template, that each resourcePrefixes entry now names
// runID itself, and that substitution left no {run-id} token behind anywhere a
// command, prefix or endpoint could still carry one. A leftover token would
// otherwise reach the running system verbatim.
//
// An empty runID is refused first: every string contains the empty string, so
// the resourcePrefixes check would pass vacuously and a descriptor whose
// resources belong to no run at all would validate.
func (p *Profile) ValidateResolved(runID string) error {
	if runID == "" {
		return fmt.Errorf("live profile: run-id-missing: a resolved descriptor is validated against the run id it was substituted with, and none was given")
	}

	if err := p.validateInvariants(); err != nil {
		return err
	}

	for _, entry := range p.ResourcePrefixes {
		if !strings.Contains(entry, runID) {
			return fmt.Errorf("live profile: run-id-missing-from-resources %s", entry)
		}
	}

	for _, field := range p.substitutedFields() {
		for _, value := range field.values {
			if strings.Contains(value, runIDToken) {
				return fmt.Errorf("live profile: unresolved-run-id %s", field.name)
			}
		}
	}

	return p.validateCleanup()
}

// validateInvariants holds the checks a template and a resolved descriptor
// share, in the refusal order both report: invalid-limits,
// unsupported-classification, undeclared-endpoint.
func (p *Profile) validateInvariants() error {
	if p.TimeoutSeconds < 0 || p.CleanupTimeoutSeconds < 0 || p.MaxOutputBytes < 0 {
		return fmt.Errorf("live profile: invalid-limits: limits must not be negative")
	}
	const maxSeconds = int64((1<<63 - 1) / time.Second)
	if p.TimeoutSeconds > maxSeconds || p.CleanupTimeoutSeconds > maxSeconds {
		return fmt.Errorf("live profile: invalid-limits: timeout exceeds supported duration")
	}
	if p.Classification != "local" && p.Classification != "ephemeral" {
		return fmt.Errorf("live profile: unsupported-classification %q", p.Classification)
	}

	if p.Classification == "local" {
		for _, endpoint := range p.Endpoints {
			if !isLoopbackEndpoint(endpoint) {
				return fmt.Errorf("live profile: undeclared-endpoint %s", endpoint)
			}
		}
	}

	return nil
}

func (p *Profile) validateCleanup() error {
	if len(p.Cleanup) == 0 || len(p.CleanupEvidence) == 0 {
		return fmt.Errorf("live profile: missing-cleanup")
	}
	return nil
}

// substitutedField is one profile field prepare-live substitutes, carried
// under the JSON name a refusal reports.
type substitutedField struct {
	name   string
	values []string
}

// substitutedFields lists every field prepare-live substitutes, so the
// unresolved-run-id sweep and the substitution stay in step.
func (p *Profile) substitutedFields() []substitutedField {
	return []substitutedField{
		{"start", flattenArgv(p.Start)},
		{"health", flattenArgv(p.Health)},
		{"walkthrough", flattenArgv(p.Walkthrough)},
		{"cleanup", flattenArgv(p.Cleanup)},
		{"resourcePrefixes", p.ResourcePrefixes},
		{"endpoints", p.Endpoints},
	}
}

func flattenArgv(groups [][]string) []string {
	var out []string
	for _, argv := range groups {
		out = append(out, argv...)
	}
	return out
}

func isLoopbackEndpoint(endpoint string) bool {
	host := ""
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		host = u.Hostname()
	} else if h, _, err := net.SplitHostPort(endpoint); err == nil {
		host = h
	} else {
		host = endpoint
	}

	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
