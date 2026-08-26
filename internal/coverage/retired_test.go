package coverage

import "testing"

func TestParseRetiredSectionExtractsLeadingIDToken(t *testing.T) {
	lines := []string{
		"## Retired",
		"",
		"- SC-04 merged into SC-03 (same mutation: uniqueness check outside the transaction)",
		"- orders/SC-02 removed, feature deleted",
		"",
		"## Scenarios",
	}

	ids, err := parseRetiredSection("path.md", lines)
	if err != nil {
		t.Fatalf("parseRetiredSection: %v", err)
	}
	want := []string{"SC-04", "orders/SC-02"}
	if len(ids) != len(want) {
		t.Fatalf("parseRetiredSection = %v, want %v", ids, want)
	}
	for i, id := range want {
		if ids[i] != id {
			t.Errorf("ids[%d] = %q, want %q", i, ids[i], id)
		}
	}
}

func TestParseRetiredSectionAbsentSectionReturnsNoIDs(t *testing.T) {
	lines := []string{"# Title", "", "## Verification"}
	ids, err := parseRetiredSection("path.md", lines)
	if err != nil {
		t.Fatalf("parseRetiredSection: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("parseRetiredSection = %v, want empty", ids)
	}
}

func TestParseRetiredSectionMalformedLeadingTokenErrorsNamingBullet(t *testing.T) {
	lines := []string{
		"## Retired",
		"- not-an-id merged into SC-03",
	}

	_, err := parseRetiredSection("path.md", lines)
	wantErrContains(t, err, "not-an-id merged into SC-03")
	wantErrContains(t, err, "path.md")
}
