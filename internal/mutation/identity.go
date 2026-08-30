package mutation

import (
	"errors"
	"fmt"
	"strings"

	"test-design/internal/jvmsource"
	"test-design/internal/trace"
)

// Identity is the single test a row resolves to: the target of the
// mutation's targeted run and selection verification.
type Identity struct {
	Scope string // Go: package import path; JVM: fully-qualified class
	Name  string // Go: parent test function name; JVM: declared method name
}

var (
	ErrRowNotBound  = errors.New("row not bound")
	ErrRowAmbiguous = errors.New("row bound ambiguously")
)

// citation is a source language's TagSite reduced to what identity
// resolution needs, so ResolveGoIdentity and ResolveJVMIdentity share one
// resolution algorithm.
type citation struct {
	Scope     string
	Name      string
	Ambiguous bool
	File      string
	Line      int
	Detail    string
}

// ResolveGoIdentity resolves rowID against Go TagSites to the one test
// function that unambiguously owns it.
func ResolveGoIdentity(sites []trace.TagSite, rowID string) (Identity, error) {
	var cites []citation
	for _, s := range sites {
		if !citesRow(s.IDs, rowID) {
			continue
		}
		// scan.go usually marks empty TestName Ambiguous already; don't rely on it.
		ambiguous := s.Ambiguous || s.TestName == ""
		detail := s.Detail
		if s.TestName == "" && detail == "" {
			detail = "test name is empty (unbound shape)"
		}
		cites = append(cites, citation{
			Scope:     s.ImportPath,
			Name:      s.TestName,
			Ambiguous: ambiguous,
			File:      s.File,
			Line:      s.Line,
			Detail:    detail,
		})
	}
	return resolveIdentity(rowID, cites)
}

// ResolveJVMIdentity resolves rowID against JVM TagSites to the one
// declared method that unambiguously owns it.
func ResolveJVMIdentity(sites []jvmsource.TagSite, rowID string) (Identity, error) {
	var cites []citation
	for _, s := range sites {
		if !citesRow(s.IDs, rowID) {
			continue
		}
		cites = append(cites, citation{
			Scope:     s.ClassName,
			Name:      s.Method,
			Ambiguous: s.Ambiguous,
			File:      s.File,
			Line:      s.Line,
			Detail:    s.Detail,
		})
	}
	return resolveIdentity(rowID, cites)
}

func citesRow(ids []string, rowID string) bool {
	for _, id := range ids {
		if id == rowID {
			return true
		}
	}
	return false
}

func resolveIdentity(rowID string, cites []citation) (Identity, error) {
	if len(cites) == 0 {
		return Identity{}, fmt.Errorf("%w: row %s has no citing site", ErrRowNotBound, rowID)
	}
	for _, c := range cites {
		if c.Ambiguous {
			return Identity{}, fmt.Errorf("%w: row %s cites ambiguous site %s:%d: %s", ErrRowAmbiguous, rowID, c.File, c.Line, c.Detail)
		}
	}

	seen := make(map[Identity]citation, len(cites))
	var order []Identity
	for _, c := range cites {
		id := Identity{Scope: c.Scope, Name: c.Name}
		if _, ok := seen[id]; !ok {
			seen[id] = c
			order = append(order, id)
		}
	}
	if len(order) > 1 {
		var b strings.Builder
		fmt.Fprintf(&b, "row %s cites %d distinct identities:", rowID, len(order))
		for _, id := range order {
			c := seen[id]
			fmt.Fprintf(&b, " %s.%s (%s:%d)", id.Scope, id.Name, c.File, c.Line)
		}
		return Identity{}, fmt.Errorf("%w: %s", ErrRowAmbiguous, b.String())
	}
	return order[0], nil
}
