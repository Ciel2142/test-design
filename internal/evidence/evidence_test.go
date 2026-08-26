package evidence

import "testing"

func TestVersionConstants(t *testing.T) {
	if VerifierVersion != "0.6.0" {
		t.Errorf("VerifierVersion = %q, want %q", VerifierVersion, "0.6.0")
	}
	if SchemaVersion != "1" {
		t.Errorf("SchemaVersion = %q, want %q", SchemaVersion, "1")
	}
}

func TestEnvelopeFields(t *testing.T) {
	env := Envelope{SchemaVersion: SchemaVersion, VerifierVersion: VerifierVersion}
	if env.SchemaVersion != "1" {
		t.Errorf("env.SchemaVersion = %q, want %q", env.SchemaVersion, "1")
	}
	if env.VerifierVersion != "0.6.0" {
		t.Errorf("env.VerifierVersion = %q, want %q", env.VerifierVersion, "0.6.0")
	}
}
