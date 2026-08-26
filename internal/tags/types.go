// Package tags extracts coverage-ID tags from comment text and classifies
// each ID relative to a document being verified. Binding a tag to a source
// declaration is language-specific and belongs to the AST/token-scanner
// binders in later waves; this package owns only the language-neutral half.
package tags

import "encoding/json"

// Class is the relationship between an extracted ID and the document being
// verified. Its JSON form is the spec vocabulary string, never the integer.
type Class int

const (
	// ClassUnknown: owned by no document, including a malformed token. It
	// is iota-0 so an unassigned Class never binds a tag to this run.
	ClassUnknown Class = iota
	// ClassActive: owned by thisDoc, owner active.
	ClassActive
	// ClassRetired: owned by thisDoc, owner not active.
	ClassRetired
	// ClassForeign: owned by a document other than thisDoc — that
	// document's coverage, not a finding in this run.
	ClassForeign
)

// String returns the spec's own vocabulary for c. Any value outside the
// defined set reports "unknown".
func (c Class) String() string {
	switch c {
	case ClassActive:
		return "active"
	case ClassRetired:
		return "retired"
	case ClassForeign:
		return "foreign"
	default:
		return "unknown"
	}
}

// MarshalJSON emits c as its vocabulary string so evidence artifacts never
// carry a class as a bare integer.
func (c Class) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

// Classification is the result of classifying one extracted ID against a
// registry and the document being verified.
type Classification struct {
	ID       string
	Class    Class
	OwnerDoc string
}
