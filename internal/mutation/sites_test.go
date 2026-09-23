package mutation

import (
	"errors"
	"testing"
)

func TestResolveEnumeratedFirstCallMissingSites(t *testing.T) {
	_, err := ResolveEnumerated(nil, nil, nil, "site-a")
	if !errors.Is(err, ErrMissingSites) {
		t.Errorf("err = %v, want ErrMissingSites", err)
	}
}

func TestResolveEnumeratedFirstCallEstablishesTheList(t *testing.T) {
	got, err := ResolveEnumerated(nil, nil, []string{"site-a", "site-b"}, "site-a")
	if err != nil {
		t.Fatalf("ResolveEnumerated: %v", err)
	}
	if len(got) != 2 || got[0] != "site-a" || got[1] != "site-b" {
		t.Errorf("got = %v, want [site-a site-b]", got)
	}
}

// TestResolveEnumeratedFirstRegistrationMissingEvaluatedSiteRefused covers a
// row a pre-1.3.0 verifier left mid-way: it already has evaluated sites with
// no registered list, and the first registration under 1.3.0 must not omit
// one.
func TestResolveEnumeratedFirstRegistrationMissingEvaluatedSiteRefused(t *testing.T) {
	_, err := ResolveEnumerated(nil, []string{"site-a"}, []string{"site-b"}, "site-b")
	if !errors.Is(err, ErrEnumerationChanged) {
		t.Errorf("err = %v, want ErrEnumerationChanged", err)
	}
}

func TestResolveEnumeratedFirstRegistrationCoveringEvaluatedSiteAccepted(t *testing.T) {
	got, err := ResolveEnumerated(nil, []string{"site-a"}, []string{"site-a", "site-b"}, "site-b")
	if err != nil {
		t.Fatalf("ResolveEnumerated: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got = %v, want both sites registered", got)
	}
}

func TestResolveEnumeratedInvalidSites(t *testing.T) {
	cases := []struct {
		name  string
		sites []string
		site  string
	}{
		{"empty label", []string{"", "site-b"}, "site-b"},
		{"forward slash", []string{"a/b", "site-b"}, "site-b"},
		{"backslash", []string{`a\b`, "site-b"}, "site-b"},
		{"duplicate label", []string{"site-a", "site-a"}, "site-a"},
		{"site not among sites", []string{"site-a", "site-b"}, "site-c"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ResolveEnumerated(nil, nil, c.sites, c.site)
			if !errors.Is(err, ErrInvalidSites) {
				t.Errorf("err = %v, want ErrInvalidSites", err)
			}
		})
	}
}

func TestResolveEnumeratedLaterCallOmittingSitesKeepsExisting(t *testing.T) {
	existing := []string{"site-a", "site-b"}
	got, err := ResolveEnumerated(existing, nil, nil, "site-b")
	if err != nil {
		t.Fatalf("ResolveEnumerated: %v", err)
	}
	if len(got) != 2 || got[0] != "site-a" || got[1] != "site-b" {
		t.Errorf("got = %v, want the existing list unchanged", got)
	}
}

func TestResolveEnumeratedLaterCallSameSetDifferentOrderAccepted(t *testing.T) {
	existing := []string{"site-a", "site-b"}
	got, err := ResolveEnumerated(existing, nil, []string{"site-b", "site-a"}, "site-a")
	if err != nil {
		t.Fatalf("ResolveEnumerated: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got = %v, want a two-element set", got)
	}
}

func TestResolveEnumeratedLaterCallDifferentSetRefused(t *testing.T) {
	existing := []string{"site-a", "site-b"}
	_, err := ResolveEnumerated(existing, nil, []string{"site-b", "site-c"}, "site-b")
	if !errors.Is(err, ErrEnumerationChanged) {
		t.Errorf("err = %v, want ErrEnumerationChanged", err)
	}
}

func TestValidateSiteLabel(t *testing.T) {
	cases := []struct {
		name    string
		site    string
		wantErr bool
	}{
		{"valid", "site-a", false},
		{"empty", "", true},
		{"forward slash", "a/b", true},
		{"backslash", `a\b`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateSiteLabel(c.site)
			if c.wantErr && !errors.Is(err, ErrInvalidSites) {
				t.Errorf("ValidateSiteLabel(%q) = %v, want ErrInvalidSites", c.site, err)
			}
			if !c.wantErr && err != nil {
				t.Errorf("ValidateSiteLabel(%q) = %v, want nil", c.site, err)
			}
		})
	}
}

func TestCheckSiteKnown(t *testing.T) {
	registered := []string{"site-a", "site-b"}
	if err := CheckSiteKnown(registered, "site-a"); err != nil {
		t.Errorf("CheckSiteKnown(registered site) = %v, want nil", err)
	}
	if err := CheckSiteKnown(registered, "site-c"); !errors.Is(err, ErrUnknownSite) {
		t.Errorf("CheckSiteKnown(unregistered site) = %v, want ErrUnknownSite", err)
	}
}

func TestCheckSitesRemaining(t *testing.T) {
	registered := []string{"site-a", "site-b"}
	if err := CheckSitesRemaining(registered, nil, "site-a", false); err != nil {
		t.Errorf("lastSite=false = %v, want nil regardless of what remains", err)
	}
	if err := CheckSitesRemaining(registered, nil, "site-a", true); !errors.Is(err, ErrSitesRemaining) {
		t.Errorf("lastSite=true with site-b unevaluated = %v, want ErrSitesRemaining", err)
	}
	if err := CheckSitesRemaining(registered, []string{"site-a"}, "site-b", true); err != nil {
		t.Errorf("lastSite=true on the final registered site = %v, want nil", err)
	}
}

func TestRowComplete(t *testing.T) {
	registered := []string{"site-a", "site-b"}
	if RowComplete(registered, []string{"site-a"}) {
		t.Error("RowComplete = true, want false while site-b is unevaluated")
	}
	if !RowComplete(registered, []string{"site-a", "site-b"}) {
		t.Error("RowComplete = false, want true once every registered site is evaluated")
	}
	if !RowComplete(registered, []string{"site-b", "site-a"}) {
		t.Error("RowComplete = false, want the evaluated order irrelevant")
	}
	if RowComplete(registered, []string{"site-a", "site-b", "site-c"}) {
		t.Error("RowComplete = true, want false when evaluated carries a site outside the registered set")
	}
}
