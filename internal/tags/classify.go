package tags

import "test-design/internal/coverage"

// Classify returns the Classification for one extracted ID. Ownership
// decides before retirement (R10): an ID owned by a document other than
// thisDoc is ClassForeign regardless of whether that other document
// retired it — ClassRetired describes only thisDoc's own IDs.
//
// thisDoc must be repo-root-relative and slash-separated, the same form
// coverage.Owner.DocPath carries. Any other form silently classifies every
// one of thisDoc's own IDs as ClassForeign, which by spec never binds — the
// document's entire coverage disappears instead of erroring.
func Classify(id, thisDoc string, reg *coverage.Registry) Classification {
	owner, ok := reg.Owner(id)
	if !ok {
		return Classification{ID: id, Class: ClassUnknown}
	}
	if owner.DocPath != thisDoc {
		return Classification{ID: id, Class: ClassForeign, OwnerDoc: owner.DocPath}
	}
	if owner.Active {
		return Classification{ID: id, Class: ClassActive, OwnerDoc: owner.DocPath}
	}
	return Classification{ID: id, Class: ClassRetired, OwnerDoc: owner.DocPath}
}
