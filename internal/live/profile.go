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
	SchemaVersion    string      `json:"schemaVersion"`
	Classification   string      `json:"classification"`
	Start            [][]string  `json:"start"`
	Health           [][]string  `json:"health"`
	Walkthrough      [][]string  `json:"walkthrough"`
	Cleanup          [][]string  `json:"cleanup"`
	CleanupEvidence  []Predicate `json:"cleanupEvidence"`
	EnvAllowlist     []string    `json:"envAllowlist"`
	ResourcePrefixes []string    `json:"resourcePrefixes"`
	Endpoints        []string    `json:"endpoints"`
	TimeoutSeconds        int64 `json:"timeoutSeconds,omitempty"`
	CleanupTimeoutSeconds int64 `json:"cleanupTimeoutSeconds,omitempty"`
	MaxOutputBytes        int64 `json:"maxOutputBytes,omitempty"`
}

type Predicate struct {
	Form   string `json:"form"`
	Stream string `json:"stream"`
	Value  string `json:"value"`
}

const (
	defaultTimeout = 120 * time.Second
	defaultCleanupTimeout = 30 * time.Second
	defaultMaxOutputBytes int64 = 16777216
)

type Limits struct {
	Timeout time.Duration
	OutputBytes int64
}

// Limits returns independent deadlines for ordinary and cleanup commands.
// Validate must succeed before a profile is used for execution.
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

// Validate checks the fail-closed invariants required before any live
// process starts. runID is accepted for future {run-id} substitution
// callers; this task performs no substitution.
func (p *Profile) Validate(runID string) error {
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

	for _, entry := range p.ResourcePrefixes {
		if !strings.Contains(entry, "{run-id}") {
			return fmt.Errorf("live profile: run-id-missing-from-resources %s", entry)
		}
	}

	if len(p.Cleanup) == 0 || len(p.CleanupEvidence) == 0 {
		return fmt.Errorf("live profile: missing-cleanup")
	}

	return nil
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
