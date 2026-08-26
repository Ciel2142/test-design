package coverage

import "testing"

func TestIDPatternAcceptsDocumentedForms(t *testing.T) {
	valid := []string{"SC-01", "SC-1", "SC-123", "orders/SC-07", "billing.v2/SC-3", "a_b-c/SC-9"}
	for _, id := range valid {
		if !idPattern.MatchString(id) {
			t.Errorf("idPattern rejects %q, want accept", id)
		}
	}
}

func TestIDPatternRejectsMalformedForms(t *testing.T) {
	invalid := []string{"SC-", "SC-abc", "sc-01", "orders/SC-", "orders//SC-01", "SC01", ""}
	for _, id := range invalid {
		if idPattern.MatchString(id) {
			t.Errorf("idPattern accepts %q, want reject", id)
		}
	}
}
