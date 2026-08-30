package junitxml

import (
	"fmt"
	"sort"

	"test-design/internal/runrecord"
)

// Family groups every Case whose derived Identity resolves to the same
// class and declared method — or, for an Unsupported Identity, the same
// raw name — across every merged Document. Status is the worst-first
// collapse of every member Case's Status.
type Family struct {
	Identity Identity
	Status   runrecord.Status
	Cases    []Case
}

// Counts is a raw per-case tally across every merged Document, taken
// before family collapse.
type Counts struct {
	Passed  int
	Failed  int
	Skipped int
}

// Report is the result of merging one or more normalized Documents.
type Report struct {
	Families []Family
	Counts   Counts
}

type familyKey struct {
	className   string
	position    string
	unsupported bool
}

// Merge concatenates docs in input order and groups their Cases into
// Families keyed by (ClassName, derived Identity). A derivable Identity
// groups by (ClassName, Method); an Unsupported Identity has no derived
// method, so it groups by (ClassName, RawName) instead — this keeps
// distinctly phrased unsupported names in separate families rather than
// collapsing them together, at the cost of merging distinct raw names that
// happen to render identically (e.g. Gradle's parameterized "[N] args"
// decoration reused across methods in one class).
//
// Each Family collapses worst-first via runrecord.Collapse and carries the
// Unsupported marking of its member Identity, if any; its Status still
// collapses normally. Families are ordered by ClassName, then by Method
// for a derivable family or by RawName for an Unsupported family (the
// "method position"), so the Report is byte-stable across runs. Counts is
// a raw per-case tally taken before collapse. Merge does not deduplicate
// across Documents: every invocation in every Document is a distinct Case.
func Merge(docs []*Document) (*Report, error) {
	var counts Counts
	families := map[familyKey]*Family{}
	var order []familyKey

	for _, doc := range docs {
		if doc == nil {
			continue
		}
		for _, c := range doc.Cases {
			switch c.Status {
			case runrecord.Passed:
				counts.Passed++
			case runrecord.Failed:
				counts.Failed++
			case runrecord.Skipped:
				counts.Skipped++
			}

			id := DeriveIdentity(c)
			key := familyKey{className: id.ClassName, unsupported: id.Unsupported != ""}
			if key.unsupported {
				key.position = c.RawName
			} else {
				key.position = id.Method
			}

			f, ok := families[key]
			if !ok {
				f = &Family{Identity: id}
				families[key] = f
				order = append(order, key)
			}
			f.Cases = append(f.Cases, c)
		}
	}

	sort.Slice(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if a.className != b.className {
			return a.className < b.className
		}
		return a.position < b.position
	})

	report := &Report{Counts: counts}
	for _, key := range order {
		f := families[key]
		statuses := make([]runrecord.Status, len(f.Cases))
		for i, c := range f.Cases {
			statuses[i] = c.Status
		}
		status, err := runrecord.Collapse(statuses)
		if err != nil {
			return nil, fmt.Errorf("junitxml: merge: family %s %q: %w", f.Identity.ClassName, key.position, err)
		}
		f.Status = status
		report.Families = append(report.Families, *f)
	}

	return report, nil
}
