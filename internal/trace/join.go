package trace

import (
	"fmt"
	"sort"
	"strings"

	"test-design/internal/coverage"
	"test-design/internal/gotestjson"
	"test-design/internal/runrecord"
	"test-design/internal/tags"
)

type Finding struct {
	Kind   string // one of: gap, red, drift, overclaim, ambiguity
	RowID  string // empty when not row-bound (e.g. red(runner), drift on retired tag)
	Detail string
}

type RowResult struct {
	RowID   string
	Scope   string // package import path
	Name    string // parent test name
	Status  runrecord.Status
	File    string // repo-relative path of the bound tag site; "" when unbound
	Line    int    // declaration line of the bound test; 0 when unbound
	EndLine int    // last line of the bound test declaration; 0 when unknown
}

// TagClass: evidence for foreign/unknown IDs; never binds a row.
type TagClass struct {
	ID, Class, File string
	Line            int
}

const maxActiveIDsPerSite = 2

// Join binds coverage rows in src to the test sites that cite them and
// reports every discrepancy found along the way. It fails closed: a
// corrupt source, or a site that overclaims, contributes no bindings
// rather than a best guess.
func Join(src *coverage.Source, reg *coverage.Registry, sites []TagSite, rep *gotestjson.Report) ([]RowResult, []Finding, []TagClass, error) {
	if src == nil {
		return nil, nil, nil, fmt.Errorf("trace: join: nil source")
	}
	if reg == nil {
		return nil, nil, nil, fmt.Errorf("trace: join: nil registry")
	}
	if rep == nil {
		return nil, nil, nil, fmt.Errorf("trace: join: nil report")
	}
	for i, row := range src.Rows {
		if row.ID == "" {
			return nil, nil, nil, fmt.Errorf("trace: join: row at index %d has empty id", i)
		}
	}

	var findings []Finding
	var classes []TagClass
	bindings := make(map[string][]TagSite)

	for _, site := range sites {
		if site.Ambiguous {
			findings = append(findings, Finding{
				Kind:   "ambiguity",
				Detail: fmt.Sprintf("%s:%d: %s", site.File, site.Line, site.Detail),
			})
			continue
		}

		var activeIDs []string
		seen := make(map[string]bool)
		for _, id := range site.IDs {
			if seen[id] {
				continue
			}
			seen[id] = true

			classification := tags.Classify(id, src.Path, reg)
			switch classification.Class {
			case tags.ClassActive:
				activeIDs = append(activeIDs, id)
			case tags.ClassRetired:
				findings = append(findings, Finding{
					Kind:   "drift",
					Detail: fmt.Sprintf("retired ID %s at %s:%d", id, site.File, site.Line),
				})
			case tags.ClassForeign:
				classes = append(classes, TagClass{ID: id, Class: classification.Class.String(), File: site.File, Line: site.Line})
			default:
				findings = append(findings, Finding{
					Kind:   "drift",
					Detail: fmt.Sprintf("unknown ID %s at %s:%d", id, site.File, site.Line),
				})
				classes = append(classes, TagClass{ID: id, Class: classification.Class.String(), File: site.File, Line: site.Line})
			}
		}

		if len(activeIDs) > maxActiveIDsPerSite {
			findings = append(findings, Finding{
				Kind:   "overclaim",
				Detail: fmt.Sprintf("test %s at %s:%d cites too many active ids: %s", site.TestName, site.File, site.Line, strings.Join(activeIDs, ", ")),
			})
			continue
		}

		for _, id := range activeIDs {
			bindings[id] = append(bindings[id], site)
		}
	}

	retiredSet := make(map[string]bool, len(src.Retired))
	for _, id := range src.Retired {
		retiredSet[id] = true
	}

	var results []RowResult
	for _, row := range src.Rows {
		if retiredSet[row.ID] {
			continue
		}

		boundSites := bindings[row.ID]
		switch len(boundSites) {
		case 0:
			findings = append(findings, Finding{Kind: "gap", RowID: row.ID, Detail: fmt.Sprintf("no test cites %s", row.ID)})
		case 1:
			site := boundSites[0]
			family := familyRecords(rep.Records, site.ImportPath, site.TestName)
			if len(family) == 0 {
				findings = append(findings, Finding{
					Kind:   "gap",
					RowID:  row.ID,
					Detail: fmt.Sprintf("no test executed for %s cited by %s:%d", row.ID, site.File, site.Line),
				})
				continue
			}

			statuses := make([]runrecord.Status, len(family))
			for i, rec := range family {
				statuses[i] = rec.Status
			}
			status, err := runrecord.Collapse(statuses)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("trace: join: row %s: %w", row.ID, err)
			}

			results = append(results, RowResult{
				RowID:   row.ID,
				Scope:   site.ImportPath,
				Name:    site.TestName,
				Status:  status,
				File:    site.File,
				Line:    site.Line,
				EndLine: site.EndLine,
			})
			switch status {
			case runrecord.Failed:
				findings = append(findings, Finding{
					Kind:   "red",
					RowID:  row.ID,
					Detail: fmt.Sprintf("%s: %s at %s:%d failed", row.ID, site.TestName, site.File, site.Line),
				})
			case runrecord.Skipped:
				findings = append(findings, Finding{
					Kind:   "gap",
					RowID:  row.ID,
					Detail: fmt.Sprintf("%s: %s at %s:%d skipped", row.ID, site.TestName, site.File, site.Line),
				})
			}
		default:
			descs := make([]string, len(boundSites))
			for i, site := range boundSites {
				descs[i] = fmt.Sprintf("%s:%d", site.File, site.Line)
			}
			findings = append(findings, Finding{
				Kind:   "ambiguity",
				RowID:  row.ID,
				Detail: fmt.Sprintf("%s bound by multiple sites: %s", row.ID, strings.Join(descs, "; ")),
			})
		}
	}

	for _, f := range rep.Failures {
		findings = append(findings, Finding{
			Kind:   "red",
			Detail: fmt.Sprintf("runner: %s: %s", f.Package, f.Detail),
		})
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Kind != findings[j].Kind {
			return findings[i].Kind < findings[j].Kind
		}
		if findings[i].RowID != findings[j].RowID {
			return findings[i].RowID < findings[j].RowID
		}
		return findings[i].Detail < findings[j].Detail
	})
	sort.Slice(results, func(i, j int) bool { return results[i].RowID < results[j].RowID })
	sort.Slice(classes, func(i, j int) bool {
		if classes[i].File != classes[j].File {
			return classes[i].File < classes[j].File
		}
		if classes[i].Line != classes[j].Line {
			return classes[i].Line < classes[j].Line
		}
		return classes[i].ID < classes[j].ID
	})

	return results, findings, classes, nil
}

func familyRecords(records []runrecord.Record, importPath, testName string) []runrecord.Record {
	prefix := testName + "/"
	var family []runrecord.Record
	for _, r := range records {
		if r.Scope != importPath {
			continue
		}
		if r.Name == testName || strings.HasPrefix(r.Name, prefix) {
			family = append(family, r)
		}
	}
	return family
}
