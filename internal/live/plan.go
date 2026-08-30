package live

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
)

type Plan struct {
	SchemaVersion string `json:"schemaVersion"`
	Steps         []Step `json:"steps"`
	Cleanup       []Step `json:"cleanup"`
}

type Step struct {
	Expectation string      `json:"expectation"`
	Command     []string    `json:"command"`
	Predicates  []Predicate `json:"predicates"`
	SCBindings  []string    `json:"scBindings"`
}

// DecodePlan fails closed: malformed JSON, an unsupported schemaVersion, no
// steps, or a step missing its expectation or command each report an error
// rather than returning a Plan a caller might mistake for valid.
func DecodePlan(data []byte) (*Plan, error) {
	var pl Plan
	if err := json.Unmarshal(data, &pl); err != nil {
		return nil, fmt.Errorf("decode live plan: %w", err)
	}
	if pl.SchemaVersion != evidence.SchemaVersion {
		return nil, fmt.Errorf("decode live plan: unsupported schemaVersion %q, want %q", pl.SchemaVersion, evidence.SchemaVersion)
	}
	if len(pl.Steps) == 0 {
		return nil, fmt.Errorf("decode live plan: empty steps")
	}
	for i, step := range pl.Steps {
		if err := validateStepShape("steps", i, step); err != nil {
			return nil, err
		}
	}
	for i, step := range pl.Cleanup {
		if err := validateStepShape("cleanup", i, step); err != nil {
			return nil, err
		}
	}
	return &pl, nil
}

func validateStepShape(section string, index int, step Step) error {
	if step.Expectation == "" {
		return fmt.Errorf("decode live plan: %s[%d]: missing expectation", section, index)
	}
	if len(step.Command) == 0 {
		return fmt.Errorf("decode live plan: %s[%d]: missing command", section, index)
	}
	return nil
}

var (
	shellPrograms    = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true}
	endpointURL      = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://\S+`)
	endpointHostPort = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?:[0-9]+$`)
)

// Validate checks the fail-closed invariants a live plan must satisfy
// against the profile it will run under and the coverage rows it claims to
// exercise. applicable is already filtered by the caller to active
// smoke/e2e rows; layer1Gaps marks rows that must not be bound because they
// already report LIVE-N/A (layer 1 gap).
func (pl *Plan) Validate(p *Profile, applicable []coverage.Row, layer1Gaps map[string]bool) error {
	declared := declaredCommands(p)
	applicableIDs := make(map[string]bool, len(applicable))
	for _, row := range applicable {
		applicableIDs[row.ID] = true
	}

	bound := map[string]bool{}
	allSteps := make([]Step, 0, len(pl.Steps)+len(pl.Cleanup))
	allSteps = append(allSteps, pl.Steps...)
	allSteps = append(allSteps, pl.Cleanup...)

	for _, step := range allSteps {
		for _, pred := range step.Predicates {
			if err := validatePredicate(pred); err != nil {
				return err
			}
		}
		if isShellInvocation(step.Command) {
			return fmt.Errorf("live plan: shell-invocation-rejected %s", strings.Join(step.Command, " "))
		}
		if len(step.Command) == 0 || !declared[step.Command[0]] {
			program := ""
			if len(step.Command) > 0 {
				program = step.Command[0]
			}
			return fmt.Errorf("live plan: undeclared-command %s", program)
		}
		for _, token := range step.Command {
			if isEndpointShaped(token) && !endpointDeclared(token, p.Endpoints) {
				return fmt.Errorf("live plan: undeclared-endpoint %s", token)
			}
		}
		for _, id := range step.SCBindings {
			if !applicableIDs[id] {
				return fmt.Errorf("live plan: unknown-row %s", id)
			}
			if layer1Gaps[id] {
				return fmt.Errorf("live plan: gap-row-bound %s (LIVE-N/A layer 1 gap)", id)
			}
			bound[id] = true
		}
	}

	for _, row := range applicable {
		if layer1Gaps[row.ID] {
			continue
		}
		if !bound[row.ID] {
			return fmt.Errorf("live plan: unbound-row %s", row.ID)
		}
	}

	return nil
}

func declaredCommands(p *Profile) map[string]bool {
	declared := map[string]bool{}
	for _, group := range [][][]string{p.Start, p.Health, p.Walkthrough, p.Cleanup} {
		for _, cmd := range group {
			if len(cmd) > 0 {
				declared[cmd[0]] = true
			}
		}
	}
	return declared
}

func validatePredicate(pred Predicate) error {
	switch pred.Form {
	case "exit-code":
		if _, err := strconv.Atoi(pred.Value); err != nil {
			return fmt.Errorf("live plan: inexpressible-predicate exit-code value %q", pred.Value)
		}
	case "matches", "not-matches":
		if pred.Stream != "stdout" && pred.Stream != "stderr" && pred.Stream != "combined" {
			return fmt.Errorf("live plan: inexpressible-predicate %s stream %q", pred.Form, pred.Stream)
		}
		if _, err := regexp.Compile(pred.Value); err != nil {
			return fmt.Errorf("live plan: inexpressible-predicate %s regex %q: %v", pred.Form, pred.Value, err)
		}
	default:
		return fmt.Errorf("live plan: inexpressible-predicate form %q", pred.Form)
	}
	return nil
}

func isShellInvocation(cmd []string) bool {
	if len(cmd) == 0 {
		return false
	}
	if shellPrograms[filepath.Base(cmd[0])] {
		for _, arg := range cmd[1:] {
			if arg == "-c" {
				return true
			}
		}
	}
	return len(cmd) == 1 && strings.ContainsAny(cmd[0], " \t")
}

func isEndpointShaped(token string) bool {
	return endpointURL.MatchString(token) || endpointHostPort.MatchString(token)
}

func endpointDeclared(token string, declared []string) bool {
	for _, d := range declared {
		if token == d || strings.HasPrefix(token, d) {
			return true
		}
	}
	return false
}
