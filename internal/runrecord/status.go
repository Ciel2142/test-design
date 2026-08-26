// Package runrecord defines the normalized record shape shared by every
// result adapter — go test -json, JUnit XML — that reports test outcomes to
// the verifier.
package runrecord

import "fmt"

// Status is a test outcome. The only valid values are Passed, Failed, and
// Skipped; construct one from untrusted input via ParseStatus.
type Status string

const (
	Passed  Status = "passed"
	Failed  Status = "failed"
	Skipped Status = "skipped"
)

func validStatus(s Status) bool {
	switch s {
	case Passed, Failed, Skipped:
		return true
	default:
		return false
	}
}

// ParseStatus normalizes an untrusted string into a Status. It rejects
// anything other than "passed", "failed", or "skipped" with an error naming
// the rejected value.
func ParseStatus(s string) (Status, error) {
	status := Status(s)
	if !validStatus(status) {
		return "", fmt.Errorf("runrecord: unknown status %q", s)
	}
	return status, nil
}
