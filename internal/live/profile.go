package live

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

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
}

type Predicate struct {
	Form   string `json:"form"`
	Stream string `json:"stream"`
	Value  string `json:"value"`
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
