package live

import (
	"encoding/json"
	"strings"
	"testing"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
)

func samplePlanDoc(overrides map[string]any) map[string]any {
	doc := map[string]any{
		"schemaVersion": evidence.SchemaVersion,
		"steps": []map[string]any{
			{
				"expectation": "walkthrough completes",
				"command":     []string{"./scripts/walk.sh"},
				"predicates": []map[string]string{
					{"form": "exit-code", "value": "0"},
				},
				"scBindings": []string{"SC-1"},
			},
		},
		"cleanup": []map[string]any{},
	}
	for k, v := range overrides {
		doc[k] = v
	}
	return doc
}

func marshalPlanDoc(t *testing.T, doc map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal plan fixture: %v", err)
	}
	return data
}

func TestDecodePlanMalformedJSONFailsClosed(t *testing.T) {
	pl, err := DecodePlan([]byte("{not json"))
	if err == nil {
		t.Fatal("DecodePlan returned nil error, want a fail-closed error")
	}
	if pl != nil {
		t.Errorf("Plan = %+v, want nil on error", pl)
	}
}

func TestDecodePlanWrongSchemaVersionFailsClosed(t *testing.T) {
	data := marshalPlanDoc(t, samplePlanDoc(map[string]any{"schemaVersion": "999"}))

	pl, err := DecodePlan(data)
	if err == nil {
		t.Fatal("DecodePlan returned nil error, want a fail-closed error")
	}
	if pl != nil {
		t.Errorf("Plan = %+v, want nil on error", pl)
	}
	if !strings.Contains(err.Error(), "999") {
		t.Errorf("error = %q, want it to name the got schemaVersion", err.Error())
	}
	if !strings.Contains(err.Error(), evidence.SchemaVersion) {
		t.Errorf("error = %q, want it to name the want schemaVersion", err.Error())
	}
}

func TestDecodePlanEmptyStepsFailsClosed(t *testing.T) {
	data := marshalPlanDoc(t, samplePlanDoc(map[string]any{"steps": []map[string]any{}}))

	pl, err := DecodePlan(data)
	if err == nil {
		t.Fatal("DecodePlan returned nil error, want a fail-closed error")
	}
	if pl != nil {
		t.Errorf("Plan = %+v, want nil on error", pl)
	}
	if !strings.Contains(err.Error(), "empty steps") {
		t.Errorf("error = %q, want it to mention empty steps", err.Error())
	}
}

func TestDecodePlanStepMissingExpectationFailsClosed(t *testing.T) {
	data := marshalPlanDoc(t, samplePlanDoc(map[string]any{
		"steps": []map[string]any{
			{"command": []string{"./scripts/walk.sh"}},
		},
	}))

	pl, err := DecodePlan(data)
	if err == nil {
		t.Fatal("DecodePlan returned nil error, want a fail-closed error")
	}
	if pl != nil {
		t.Errorf("Plan = %+v, want nil on error", pl)
	}
	if !strings.Contains(err.Error(), "missing expectation") {
		t.Errorf("error = %q, want it to mention missing expectation", err.Error())
	}
}

func TestDecodePlanStepMissingCommandFailsClosed(t *testing.T) {
	data := marshalPlanDoc(t, samplePlanDoc(map[string]any{
		"steps": []map[string]any{
			{"expectation": "walkthrough completes"},
		},
	}))

	pl, err := DecodePlan(data)
	if err == nil {
		t.Fatal("DecodePlan returned nil error, want a fail-closed error")
	}
	if pl != nil {
		t.Errorf("Plan = %+v, want nil on error", pl)
	}
	if !strings.Contains(err.Error(), "missing command") {
		t.Errorf("error = %q, want it to mention missing command", err.Error())
	}
}

func TestDecodePlanCleanupStepMissingCommandFailsClosed(t *testing.T) {
	data := marshalPlanDoc(t, samplePlanDoc(map[string]any{
		"cleanup": []map[string]any{
			{"expectation": "cleanup runs"},
		},
	}))

	pl, err := DecodePlan(data)
	if err == nil {
		t.Fatal("DecodePlan returned nil error, want a fail-closed error")
	}
	if pl != nil {
		t.Errorf("Plan = %+v, want nil on error", pl)
	}
	if !strings.Contains(err.Error(), "missing command") {
		t.Errorf("error = %q, want it to mention missing command", err.Error())
	}
}

func TestDecodePlanSuccess(t *testing.T) {
	data := marshalPlanDoc(t, samplePlanDoc(nil))

	pl, err := DecodePlan(data)
	if err != nil {
		t.Fatalf("DecodePlan: %v", err)
	}
	if pl.SchemaVersion != evidence.SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", pl.SchemaVersion, evidence.SchemaVersion)
	}
	if len(pl.Steps) != 1 {
		t.Fatalf("Steps = %+v, want one step", pl.Steps)
	}
	step := pl.Steps[0]
	if step.Expectation != "walkthrough completes" {
		t.Errorf("Expectation = %q, want %q", step.Expectation, "walkthrough completes")
	}
	if len(step.Command) != 1 || step.Command[0] != "./scripts/walk.sh" {
		t.Errorf("Command = %+v, want [./scripts/walk.sh]", step.Command)
	}
	if len(step.Predicates) != 1 || step.Predicates[0] != (Predicate{Form: "exit-code", Value: "0"}) {
		t.Errorf("Predicates = %+v, want one exit-code predicate", step.Predicates)
	}
	if len(step.SCBindings) != 1 || step.SCBindings[0] != "SC-1" {
		t.Errorf("SCBindings = %+v, want [SC-1]", step.SCBindings)
	}
	if len(pl.Cleanup) != 0 {
		t.Errorf("Cleanup = %+v, want empty", pl.Cleanup)
	}
}

func planProfile() *Profile {
	return &Profile{
		SchemaVersion:  evidence.SchemaVersion,
		Classification: "local",
		Start:          [][]string{{"./scripts/start.sh"}},
		Health:         [][]string{{"./scripts/health.sh"}},
		Walkthrough:    [][]string{{"./scripts/walk.sh"}},
		Cleanup:        [][]string{{"./scripts/cleanup.sh"}},
		Endpoints:      []string{"http://127.0.0.1:8099"},
	}
}

func rowStep(command []string, scBindings []string) Step {
	return Step{
		Expectation: "step runs",
		Command:     command,
		Predicates: []Predicate{
			{Form: "exit-code", Value: "0"},
		},
		SCBindings: scBindings,
	}
}

func applicableRows() []coverage.Row {
	return []coverage.Row{
		{ID: "SC-1"},
		{ID: "SC-2"},
		{ID: "SC-3"},
	}
}

func TestValidateAcceptsWellFormedPlan(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			rowStep([]string{"./scripts/walk.sh"}, []string{"SC-1"}),
			rowStep([]string{"./scripts/start.sh"}, []string{"SC-2"}),
		},
	}

	if err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true}); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestValidateAcceptsBindingViaCleanupStep(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			rowStep([]string{"./scripts/walk.sh"}, []string{"SC-1"}),
		},
		Cleanup: []Step{
			rowStep([]string{"./scripts/cleanup.sh"}, []string{"SC-2"}),
		},
	}

	if err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true}); err != nil {
		t.Errorf("Validate() = %v, want nil (cleanup step binds SC-2)", err)
	}
}

func TestValidateAcceptsCommandDeclaredViaAnyProfileArray(t *testing.T) {
	for _, cmd := range [][]string{
		{"./scripts/start.sh"},
		{"./scripts/health.sh"},
		{"./scripts/walk.sh"},
		{"./scripts/cleanup.sh"},
	} {
		t.Run(cmd[0], func(t *testing.T) {
			pl := &Plan{
				SchemaVersion: evidence.SchemaVersion,
				Steps: []Step{
					rowStep(cmd, []string{"SC-1", "SC-2"}),
				},
			}
			if err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true}); err != nil {
				t.Errorf("Validate() = %v, want nil for declared command %v", err, cmd)
			}
		})
	}
}

func TestValidateInexpressiblePredicateBadForm(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			{
				Expectation: "step runs",
				Command:     []string{"./scripts/walk.sh"},
				Predicates: []Predicate{
					{Form: "regex-match", Value: "x"},
				},
				SCBindings: []string{"SC-1", "SC-2"},
			},
		},
	}

	err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true})
	if err == nil {
		t.Fatal("Validate() returned nil error, want inexpressible-predicate")
	}
	if !strings.Contains(err.Error(), "inexpressible-predicate") {
		t.Errorf("error = %q, want it to contain inexpressible-predicate", err.Error())
	}
	if !strings.Contains(err.Error(), "regex-match") {
		t.Errorf("error = %q, want it to name the offending form", err.Error())
	}
}

func TestValidateInexpressiblePredicateBadStream(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			{
				Expectation: "step runs",
				Command:     []string{"./scripts/walk.sh"},
				Predicates: []Predicate{
					{Form: "matches", Stream: "stdin", Value: "ok"},
				},
				SCBindings: []string{"SC-1", "SC-2"},
			},
		},
	}

	err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true})
	if err == nil {
		t.Fatal("Validate() returned nil error, want inexpressible-predicate")
	}
	if !strings.Contains(err.Error(), "inexpressible-predicate") {
		t.Errorf("error = %q, want it to contain inexpressible-predicate", err.Error())
	}
	if !strings.Contains(err.Error(), "stdin") {
		t.Errorf("error = %q, want it to name the offending stream", err.Error())
	}
}

func TestValidateInexpressiblePredicateNonIntegerExitCode(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			{
				Expectation: "step runs",
				Command:     []string{"./scripts/walk.sh"},
				Predicates: []Predicate{
					{Form: "exit-code", Value: "zero"},
				},
				SCBindings: []string{"SC-1", "SC-2"},
			},
		},
	}

	err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true})
	if err == nil {
		t.Fatal("Validate() returned nil error, want inexpressible-predicate")
	}
	if !strings.Contains(err.Error(), "inexpressible-predicate") {
		t.Errorf("error = %q, want it to contain inexpressible-predicate", err.Error())
	}
	if !strings.Contains(err.Error(), "zero") {
		t.Errorf("error = %q, want it to name the offending value", err.Error())
	}
}

func TestValidateInexpressiblePredicateExitCodeWithStream(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			{
				Expectation: "step runs",
				Command:     []string{"./scripts/walk.sh"},
				Predicates: []Predicate{
					{Form: "exit-code", Stream: "stdout", Value: "0"},
				},
				SCBindings: []string{"SC-1", "SC-2"},
			},
		},
	}

	err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true})
	if err == nil {
		t.Fatal("Validate() returned nil error, want inexpressible-predicate (exit-code takes no stream)")
	}
	if !strings.Contains(err.Error(), "inexpressible-predicate") {
		t.Errorf("error = %q, want it to contain inexpressible-predicate", err.Error())
	}
	if !strings.Contains(err.Error(), "stdout") {
		t.Errorf("error = %q, want it to name the offending stream", err.Error())
	}
}

func TestValidateInexpressiblePredicateBadRegex(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			{
				Expectation: "step runs",
				Command:     []string{"./scripts/walk.sh"},
				Predicates: []Predicate{
					{Form: "matches", Stream: "stdout", Value: "(unclosed"},
				},
				SCBindings: []string{"SC-1", "SC-2"},
			},
		},
	}

	err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true})
	if err == nil {
		t.Fatal("Validate() returned nil error, want inexpressible-predicate")
	}
	if !strings.Contains(err.Error(), "inexpressible-predicate") {
		t.Errorf("error = %q, want it to contain inexpressible-predicate", err.Error())
	}
	if !strings.Contains(err.Error(), "(unclosed") {
		t.Errorf("error = %q, want it to name the offending regex", err.Error())
	}
}

func TestValidateUndeclaredCommand(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			rowStep([]string{"./scripts/unknown.sh"}, []string{"SC-1", "SC-2"}),
		},
	}

	err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true})
	if err == nil {
		t.Fatal("Validate() returned nil error, want undeclared-command")
	}
	if !strings.Contains(err.Error(), "undeclared-command") {
		t.Errorf("error = %q, want it to contain undeclared-command", err.Error())
	}
	if !strings.Contains(err.Error(), "./scripts/unknown.sh") {
		t.Errorf("error = %q, want it to name the offending program", err.Error())
	}
}

func TestValidateUndeclaredEndpointURL(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			rowStep([]string{"./scripts/walk.sh", "http://198.51.100.7:9000/check"}, []string{"SC-1", "SC-2"}),
		},
	}

	err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true})
	if err == nil {
		t.Fatal("Validate() returned nil error, want undeclared-endpoint")
	}
	if !strings.Contains(err.Error(), "undeclared-endpoint") {
		t.Errorf("error = %q, want it to contain undeclared-endpoint", err.Error())
	}
	if !strings.Contains(err.Error(), "http://198.51.100.7:9000/check") {
		t.Errorf("error = %q, want it to name the offending token", err.Error())
	}
}

func TestValidateUndeclaredEndpointBareHostPort(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			rowStep([]string{"./scripts/walk.sh", "198.51.100.7:9000"}, []string{"SC-1", "SC-2"}),
		},
	}

	err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true})
	if err == nil {
		t.Fatal("Validate() returned nil error, want undeclared-endpoint")
	}
	if !strings.Contains(err.Error(), "undeclared-endpoint") {
		t.Errorf("error = %q, want it to contain undeclared-endpoint", err.Error())
	}
	if !strings.Contains(err.Error(), "198.51.100.7:9000") {
		t.Errorf("error = %q, want it to name the offending token", err.Error())
	}
}

func TestValidateLoopbackEndpointStillMustBeDeclared(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			rowStep([]string{"./scripts/walk.sh", "http://127.0.0.1:9999"}, []string{"SC-1", "SC-2"}),
		},
	}

	err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true})
	if err == nil {
		t.Fatal("Validate() returned nil error, want undeclared-endpoint even for loopback token")
	}
	if !strings.Contains(err.Error(), "undeclared-endpoint") {
		t.Errorf("error = %q, want it to contain undeclared-endpoint", err.Error())
	}
	if !strings.Contains(err.Error(), "http://127.0.0.1:9999") {
		t.Errorf("error = %q, want it to name the offending token", err.Error())
	}
}

func TestValidateAcceptsDeclaredEndpointWithPathSuffix(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			rowStep([]string{"./scripts/walk.sh", "http://127.0.0.1:8099/health"}, []string{"SC-1", "SC-2"}),
		},
	}

	if err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true}); err != nil {
		t.Errorf("Validate() = %v, want nil for declared endpoint with path suffix", err)
	}
}

func TestValidateUndeclaredEndpointRejectsNumericallyAdjacentPort(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			rowStep([]string{"./scripts/walk.sh", "http://127.0.0.1:80990/foo"}, []string{"SC-1", "SC-2"}),
		},
	}

	err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true})
	if err == nil {
		t.Fatal("Validate() returned nil error, want undeclared-endpoint (8099 is a string-prefix of 80990, not a boundary match)")
	}
	if !strings.Contains(err.Error(), "undeclared-endpoint") {
		t.Errorf("error = %q, want it to contain undeclared-endpoint", err.Error())
	}
	if !strings.Contains(err.Error(), "http://127.0.0.1:80990/foo") {
		t.Errorf("error = %q, want it to name the offending token", err.Error())
	}
}

func TestValidateShellInvocationRejectedBareShellWithDashC(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			rowStep([]string{"bash", "-c", "echo hi"}, []string{"SC-1", "SC-2"}),
		},
	}

	err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true})
	if err == nil {
		t.Fatal("Validate() returned nil error, want shell rejection")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "shell") {
		t.Errorf("error = %q, want it to contain shell rejection wording", err.Error())
	}
}

func TestValidateShellInvocationRejectedShellByPathWithDashC(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			rowStep([]string{"/bin/sh", "-c", "echo hi"}, []string{"SC-1", "SC-2"}),
		},
	}

	err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true})
	if err == nil {
		t.Fatal("Validate() returned nil error, want shell rejection")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "shell") {
		t.Errorf("error = %q, want it to contain shell rejection wording", err.Error())
	}
}

func TestValidateShellInvocationRejectedSingleElementWithWhitespace(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			rowStep([]string{"./scripts/walk.sh arg1"}, []string{"SC-1", "SC-2"}),
		},
	}

	err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true})
	if err == nil {
		t.Fatal("Validate() returned nil error, want shell rejection")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "shell") {
		t.Errorf("error = %q, want it to contain shell rejection wording", err.Error())
	}
}

func TestValidateUnknownRowBinding(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			rowStep([]string{"./scripts/walk.sh"}, []string{"SC-999"}),
		},
	}

	err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true})
	if err == nil {
		t.Fatal("Validate() returned nil error, want unknown-row")
	}
	if !strings.Contains(err.Error(), "unknown-row") {
		t.Errorf("error = %q, want it to contain unknown-row", err.Error())
	}
	if !strings.Contains(err.Error(), "SC-999") {
		t.Errorf("error = %q, want it to name the offending row", err.Error())
	}
}

func TestValidateGapRowBindingRejected(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			rowStep([]string{"./scripts/walk.sh"}, []string{"SC-3"}),
		},
	}

	err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true})
	if err == nil {
		t.Fatal("Validate() returned nil error, want gap row binding rejected")
	}
	if !strings.Contains(err.Error(), "SC-3") {
		t.Errorf("error = %q, want it to name the offending row", err.Error())
	}
	if !strings.Contains(err.Error(), "layer 1 gap") {
		t.Errorf("error = %q, want it to mention the layer 1 gap", err.Error())
	}
}

func TestValidateUnboundApplicableRow(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			rowStep([]string{"./scripts/walk.sh"}, []string{"SC-1"}),
		},
	}

	err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true})
	if err == nil {
		t.Fatal("Validate() returned nil error, want unbound-row")
	}
	if !strings.Contains(err.Error(), "unbound-row") {
		t.Errorf("error = %q, want it to contain unbound-row", err.Error())
	}
	if !strings.Contains(err.Error(), "SC-2") {
		t.Errorf("error = %q, want it to name the offending row", err.Error())
	}
}

func TestValidateSkipsUnboundGapRow(t *testing.T) {
	pl := &Plan{
		SchemaVersion: evidence.SchemaVersion,
		Steps: []Step{
			rowStep([]string{"./scripts/walk.sh"}, []string{"SC-1"}),
			rowStep([]string{"./scripts/start.sh"}, []string{"SC-2"}),
		},
	}

	if err := pl.Validate(planProfile(), applicableRows(), map[string]bool{"SC-3": true}); err != nil {
		t.Errorf("Validate() = %v, want nil (SC-3 is a layer-1 gap, exempt from unbound-row)", err)
	}
}
