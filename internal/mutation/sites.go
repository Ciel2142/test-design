package mutation

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrMissingSites       = errors.New("mutation sites: no sites are registered for this row and none were sent")
	ErrInvalidSites       = errors.New("mutation sites: sites is invalid")
	ErrEnumerationChanged = errors.New("mutation sites: sites does not match the row's registered list")
	ErrUnknownSite        = errors.New("mutation sites: site is not in the row's registered list")
	ErrSitesRemaining     = errors.New("mutation sites: registered sites remain unevaluated")
)

// ResolveEnumerated returns the row's registered site list: existing when the
// row already has one and requestSites is empty, requestSites once validated
// when the row has none yet, or existing again when requestSites repeats it
// as a set. A requestSites that disagrees with an existing list, in either
// content or membership of site, is refused before existing is ever touched.
// First registration is also refused if it omits a site alreadyEvaluated
// carries: a row a pre-1.3.0 verifier left mid-way already has evaluated
// sites with no registered list of their own.
func ResolveEnumerated(existing, alreadyEvaluated, requestSites []string, site string) ([]string, error) {
	if len(requestSites) == 0 {
		if len(existing) == 0 {
			return nil, ErrMissingSites
		}
		return existing, nil
	}
	if err := validateSiteLabels(requestSites, site); err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		if !sitesEqual(existing, requestSites) {
			return nil, fmt.Errorf("%w: got %v, want %v", ErrEnumerationChanged, requestSites, existing)
		}
		return existing, nil
	}
	if missing := remaining(alreadyEvaluated, requestSites); len(missing) > 0 {
		return nil, fmt.Errorf("%w: sites %v omits already-evaluated site(s) %v", ErrEnumerationChanged, requestSites, missing)
	}
	return requestSites, nil
}

// ValidateSiteLabel checks a single site label's shape: non-empty and free of
// path separators. It is the check a caller with no registered list to check
// membership against still owes a label — discard-mutation-edit's first call
// for a row, before any evaluate-mutation has registered one.
func ValidateSiteLabel(site string) error {
	if site == "" || strings.ContainsAny(site, `/\`) {
		return fmt.Errorf("%w: site label %q must be non-empty and free of path separators", ErrInvalidSites, site)
	}
	return nil
}

func validateSiteLabels(sites []string, site string) error {
	seen := make(map[string]bool, len(sites))
	member := false
	for _, s := range sites {
		if err := ValidateSiteLabel(s); err != nil {
			return err
		}
		if seen[s] {
			return fmt.Errorf("%w: site label %q is duplicated", ErrInvalidSites, s)
		}
		seen[s] = true
		if s == site {
			member = true
		}
	}
	if !member {
		return fmt.Errorf("%w: site %q is not among the sites it names", ErrInvalidSites, site)
	}
	return nil
}

// CheckSiteKnown refuses a site this call names that the row's registered
// list does not include.
func CheckSiteKnown(registered []string, site string) error {
	for _, s := range registered {
		if s == site {
			return nil
		}
	}
	return fmt.Errorf("%w: %q", ErrUnknownSite, site)
}

// CheckSitesRemaining refuses lastSite when registered sites other than
// site itself still have no evaluated outcome. It is silent whenever
// lastSite is false: that call asserts nothing about completion.
func CheckSitesRemaining(registered, evaluated []string, site string, lastSite bool) error {
	if !lastSite {
		return nil
	}
	left := remaining(registered, append(append([]string{}, evaluated...), site))
	if len(left) > 0 {
		return fmt.Errorf("%w: %s", ErrSitesRemaining, strings.Join(left, ", "))
	}
	return nil
}

// RowComplete reports whether evaluated is exactly the registered set, not
// merely a superset of it.
func RowComplete(registered, evaluated []string) bool {
	return sitesEqual(registered, evaluated)
}

// remaining returns the elements of want that have does not contain,
// preserving want's order.
func remaining(want, have []string) []string {
	present := make(map[string]bool, len(have))
	for _, s := range have {
		present[s] = true
	}
	var left []string
	for _, s := range want {
		if !present[s] {
			left = append(left, s)
		}
	}
	return left
}

// sitesEqual compares a and b as multisets, order irrelevant; a duplicate
// label never reaches it in practice, so this behaves as a plain set compare.
func sitesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, s := range a {
		counts[s]++
	}
	for _, s := range b {
		counts[s]--
	}
	for _, c := range counts {
		if c != 0 {
			return false
		}
	}
	return true
}
