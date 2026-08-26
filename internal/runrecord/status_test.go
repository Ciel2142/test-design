package runrecord

import (
	"strings"
	"testing"
)

func TestParseStatusAcceptsValidValues(t *testing.T) {
	tests := []struct {
		input string
		want  Status
	}{
		{"passed", Passed},
		{"failed", Failed},
		{"skipped", Skipped},
	}
	for _, tt := range tests {
		got, err := ParseStatus(tt.input)
		if err != nil {
			t.Errorf("ParseStatus(%q): %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("ParseStatus(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseStatusRejectsUnknownValueNamingIt(t *testing.T) {
	got, err := ParseStatus("errored")
	if err == nil {
		t.Fatalf("ParseStatus(%q) returned nil error with %q, want error", "errored", got)
	}
	if got != "" {
		t.Errorf("ParseStatus(%q) returned %q, want zero Status", "errored", got)
	}
	if !strings.Contains(err.Error(), "errored") {
		t.Errorf("ParseStatus error %q does not name the rejected value %q", err.Error(), "errored")
	}
}

func TestParseStatusRejectsEmptyString(t *testing.T) {
	got, err := ParseStatus("")
	if err == nil {
		t.Fatalf("ParseStatus(\"\") returned nil error with %q, want error", got)
	}
}
