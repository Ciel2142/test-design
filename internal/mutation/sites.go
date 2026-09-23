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
func ResolveEnumerated(existing, requestSites []string, site string) ([]string, error) {
	if len(requestSites) == 0 {
		if len(existing) == 0 {
			return nil, ErrMissingSites
		}
		return existing, nil
	}
	if err := validateSiteLabels(requestSites, site); err != nil {
		return nil, err
	}
	if len(existing) > 0 && !sitesEqual(existing, requestSites) {
		return nil, fmt.Errorf("%w: got %v, want %v", ErrEnumerationChanged, requestSites, existing)
	}
	if len(existing) > 0 {
		return existing, nil
	}
	return requestSites, nil
}

func validateSiteLabels(sites []string, site string) error {
	seen := make(map[string]bool, len(sites))
	member := false
	for _, s := range sites {
		if s == "" || strings.ContainsAny(s, `/\`) {
			return fmt.Errorf("%w: site label %q must be non-empty and free of path separators", ErrInvalidSites, s)
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
	done := make(map[string]bool, len(evaluated)+1)
	for _, s := range evaluated {
		done[s] = true
	}
	done[site] = true
	var remaining []string
	for _, s := range registered {
		if !done[s] {
			remaining = append(remaining, s)
		}
	}
	if len(remaining) > 0 {
		return fmt.Errorf("%w: %s", ErrSitesRemaining, strings.Join(remaining, ", "))
	}
	return nil
}

// RowComplete reports whether every registered site has an evaluated
// outcome, independent of the order either list carries.
func RowComplete(registered, evaluated []string) bool {
	if len(evaluated) < len(registered) {
		return false
	}
	done := make(map[string]bool, len(evaluated))
	for _, s := range evaluated {
		done[s] = true
	}
	for _, s := range registered {
		if !done[s] {
			return false
		}
	}
	return true
}

// sitesEqual compares a and b as sets: same labels, order and repetition
// irrelevant.
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
