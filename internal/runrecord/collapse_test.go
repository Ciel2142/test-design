package runrecord

import (
	"strings"
	"testing"
)

func TestCollapseWorstFirst(t *testing.T) {
	tests := []struct {
		name    string
		members []Status
		want    Status
	}{
		{"single passed", []Status{Passed}, Passed},
		{"single failed", []Status{Failed}, Failed},
		{"single skipped", []Status{Skipped}, Skipped},
		{"all passed stays passed", []Status{Passed, Passed, Passed}, Passed},
		{"skipped beats passed", []Status{Passed, Skipped, Passed}, Skipped},
		{"failed beats skipped and passed", []Status{Passed, Skipped, Failed}, Failed},
		{"failed beats skipped regardless of order", []Status{Skipped, Failed}, Failed},
		{"failed beats passed regardless of order", []Status{Failed, Passed}, Failed},
		{"skipped first still beaten by later failed", []Status{Skipped, Passed, Failed}, Failed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Collapse(tt.members)
			if err != nil {
				t.Fatalf("Collapse(%v): %v", tt.members, err)
			}
			if got != tt.want {
				t.Errorf("Collapse(%v) = %q, want %q", tt.members, got, tt.want)
			}
		})
	}
}

func TestCollapseEmptyFamilyErrors(t *testing.T) {
	got, err := Collapse(nil)
	if err == nil {
		t.Fatalf("Collapse(nil) returned nil error with %q, want error", got)
	}
	if got != "" {
		t.Errorf("Collapse(nil) returned %q, want zero Status", got)
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("Collapse(nil) error %q does not name the empty-family condition", err.Error())
	}
}

func TestCollapseEmptySliceErrors(t *testing.T) {
	got, err := Collapse([]Status{})
	if err == nil {
		t.Fatalf("Collapse([]Status{}) returned nil error with %q, want error", got)
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("Collapse([]Status{}) error %q does not name the empty-family condition", err.Error())
	}
}

func TestCollapseInvalidMemberErrorsNamingValue(t *testing.T) {
	tests := []struct {
		name    string
		members []Status
	}{
		{"lone invalid member", []Status{Status("errored")}},
		{"invalid member alongside valid ones", []Status{Passed, Status("errored"), Failed}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Collapse(tt.members)
			if err == nil {
				t.Fatalf("Collapse(%v) returned nil error with %q, want error", tt.members, got)
			}
			if got != "" {
				t.Errorf("Collapse(%v) returned %q, want zero Status", tt.members, got)
			}
			if !strings.Contains(err.Error(), "errored") {
				t.Errorf("Collapse error %q does not name the offending value %q", err.Error(), "errored")
			}
		})
	}
}
