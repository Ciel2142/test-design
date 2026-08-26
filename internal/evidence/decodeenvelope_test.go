package evidence

import (
	"strings"
	"testing"
)

func TestDecodeEnvelopeSupportedVersion(t *testing.T) {
	data := []byte(`{"schemaVersion":"1","verifierVersion":"0.6.0","result":"ok"}`)

	env, err := DecodeEnvelope(data)
	if err != nil {
		t.Fatalf("DecodeEnvelope: %v", err)
	}
	if env.SchemaVersion != "1" {
		t.Errorf("env.SchemaVersion = %q, want %q", env.SchemaVersion, "1")
	}
	if env.VerifierVersion != "0.6.0" {
		t.Errorf("env.VerifierVersion = %q, want %q", env.VerifierVersion, "0.6.0")
	}
}

func TestDecodeEnvelopeUnsupportedVersionNamesBoth(t *testing.T) {
	data := []byte(`{"schemaVersion":"99","verifierVersion":"0.6.0"}`)

	env, err := DecodeEnvelope(data)
	if err == nil {
		t.Fatalf("DecodeEnvelope returned nil error with %+v, want error", env)
	}
	if !strings.Contains(err.Error(), "99") {
		t.Errorf("DecodeEnvelope error %q does not name the version found (%q)", err.Error(), "99")
	}
	if !strings.Contains(err.Error(), SchemaVersion) {
		t.Errorf("DecodeEnvelope error %q does not name the version supported (%q)", err.Error(), SchemaVersion)
	}
	if env != (Envelope{}) {
		t.Errorf("DecodeEnvelope on an unsupported version returned %+v, want zero Envelope", env)
	}
}

func TestDecodeEnvelopeMissingSchemaVersionErrors(t *testing.T) {
	data := []byte(`{"verifierVersion":"0.6.0"}`)

	env, err := DecodeEnvelope(data)
	if err == nil {
		t.Fatalf("DecodeEnvelope returned nil error with %+v, want error", env)
	}
	if env != (Envelope{}) {
		t.Errorf("DecodeEnvelope on missing schemaVersion returned %+v, want zero Envelope", env)
	}
}

func TestDecodeEnvelopeMalformedJSONErrors(t *testing.T) {
	data := []byte(`{"schemaVersion": `)

	env, err := DecodeEnvelope(data)
	if err == nil {
		t.Fatalf("DecodeEnvelope returned nil error with %+v, want error", env)
	}
	if !strings.Contains(err.Error(), "decode envelope") {
		t.Errorf("DecodeEnvelope error %q does not name the decode failure", err.Error())
	}
}
