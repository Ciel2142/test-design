// Package evidence defines the JSON envelope every verifier operation
// embeds in its output, plus the version constants that populate it.
package evidence

const (
	SchemaVersion   = "1"
	VerifierVersion = "0.6.0"
)

type Envelope struct {
	SchemaVersion   string `json:"schemaVersion"`
	VerifierVersion string `json:"verifierVersion"`
}
