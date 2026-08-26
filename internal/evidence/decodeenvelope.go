package evidence

import (
	"encoding/json"
	"fmt"
)

// DecodeEnvelope extracts the envelope fields from a JSON document. It
// fails closed: malformed JSON, a missing schemaVersion, or a schemaVersion
// other than SchemaVersion all return an error and a zero Envelope.
func DecodeEnvelope(data []byte) (Envelope, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return Envelope{}, fmt.Errorf("decode envelope: %w", err)
	}

	versionRaw, ok := raw["schemaVersion"]
	if !ok {
		return Envelope{}, fmt.Errorf("decode envelope: missing schemaVersion")
	}

	var version string
	if err := json.Unmarshal(versionRaw, &version); err != nil {
		return Envelope{}, fmt.Errorf("decode envelope: schemaVersion: %w", err)
	}
	if version != SchemaVersion {
		return Envelope{}, fmt.Errorf("decode envelope: unsupported schemaVersion %q, want %q", version, SchemaVersion)
	}

	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return Envelope{}, fmt.Errorf("decode envelope: %w", err)
	}
	return env, nil
}
