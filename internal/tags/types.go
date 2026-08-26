// Package tags extracts coverage-ID tags from comment text and classifies
// each ID relative to a document being verified. Binding a tag to a source
// declaration is language-specific and belongs to the AST/token-scanner
// binders in later waves; this package owns only the language-neutral half.
package tags

// Class is the relationship between an extracted ID and the document being
// verified.
type Class int

const (
	// ClassActive: owned by thisDoc, owner active.
	ClassActive Class = iota
	// ClassRetired: owned by thisDoc, owner not active.
	ClassRetired
	// ClassForeign: owned by a document other than thisDoc — that
	// document's coverage, not a finding in this run.
	ClassForeign
	// ClassUnknown: owned by no document, including a malformed token.
	ClassUnknown
)

// Classification is the result of classifying one extracted ID against a
// registry and the document being verified.
type Classification struct {
	ID       string
	Class    Class
	OwnerDoc string
}
