package main

import (
	"errors"
	"fmt"

	"test-design/internal/mutation"
)

// siteRegistryRefusal names one of the mutation package's site-registry
// sentinels in op's own vocabulary.
func siteRegistryRefusal(op string, err error) *cliError {
	switch {
	case errors.Is(err, mutation.ErrMissingSites):
		return &cliError{Code: "missing-sites", Message: fmt.Sprintf("%s: %v", op, err)}
	case errors.Is(err, mutation.ErrInvalidSites):
		return &cliError{Code: "invalid-sites", Message: fmt.Sprintf("%s: %v", op, err)}
	case errors.Is(err, mutation.ErrEnumerationChanged):
		return &cliError{Code: "enumeration-changed", Message: fmt.Sprintf("%s: %v", op, err)}
	case errors.Is(err, mutation.ErrUnknownSite):
		return &cliError{Code: "unknown-site", Message: fmt.Sprintf("%s: %v", op, err)}
	case errors.Is(err, mutation.ErrSitesRemaining):
		return &cliError{Code: "sites-remaining", Message: fmt.Sprintf("%s: %v", op, err)}
	default:
		return &cliError{Code: "site-registry-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
}

// resolveRowSites establishes or reasserts st's registered site list from
// requestSites and site, refusing missing-sites, invalid-sites,
// enumeration-changed or unknown-site before st is touched. On success it
// sets st.Enumerated, which only reaches disk the next time the caller
// writes st.
func resolveRowSites(op string, st *mutation.State, requestSites []string, site string) *cliError {
	registered, err := mutation.ResolveEnumerated(st.Enumerated, evaluatedSiteLabels(st.Sites), requestSites, site)
	if err != nil {
		return siteRegistryRefusal(op, err)
	}
	if err := mutation.CheckSiteKnown(registered, site); err != nil {
		return siteRegistryRefusal(op, err)
	}
	st.Enumerated = registered
	return nil
}

// checkSitesRemaining refuses a lastSite call that arrives while a
// registered site other than site itself still has no evaluated outcome.
func checkSitesRemaining(op string, st *mutation.State, site string, lastSite bool) *cliError {
	if err := mutation.CheckSitesRemaining(st.Enumerated, evaluatedSiteLabels(st.Sites), site, lastSite); err != nil {
		return siteRegistryRefusal(op, err)
	}
	return nil
}

func evaluatedSiteLabels(sites []mutation.SiteOutcome) []string {
	labels := make([]string, len(sites))
	for i, s := range sites {
		labels[i] = s.Site
	}
	return labels
}
