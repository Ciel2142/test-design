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
	Kind   string // one of: gap, red, drift, ambiguity
	RowID  string // empty when not row-bound (e.g. red(runner), drift on retired tag)
	Detail string
}

// Contributor is one bound tag site crediting a row: the test that cites
// the row, and how that test's family fared in the run.
type Contributor struct {
	Scope    string           // package import path
	Name     string           // parent test name
	Status   runrecord.Status // collapsed over the contributor's family; "" when nothing ran
	Executed bool
	File     string // repo-relative path of the tag site
	Line     int    // line of the tag comment
	EndLine  int    // last line of the test declaration; 0 when unknown
}

type RowResult struct {
	RowID        string
	Scope        string // package import path
	Name         string // parent test name
	Status       runrecord.Status
	File         string        // repo-relative path of the bound tag site; "" when unbound
	Line         int           // line of the bound tag comment; 0 when unbound
	EndLine      int           // last line of the bound test declaration; 0 when unknown
	Contributors []Contributor // every bound site, sorted by File then Line
}

// TagClass: evidence for foreign/unknown IDs; never binds a row.
type TagClass struct {
	ID, Class, File string
	Line            int
}

// Join binds coverage rows in src to the test sites that cite them and
// reports every discrepancy found along the way. Every distinct active ID a
// site cites binds, and every site bound to a row credits it as a
// Contributor: the row's status collapses over its executed contributors and
// each contributor earns its own finding. It fails closed: a corrupt source
// contributes no bindings rather than a best guess, and a row no contributor
// executed is not credited at all.
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
		if len(boundSites) == 0 {
			findings = append(findings, Finding{Kind: "gap", RowID: row.ID, Detail: fmt.Sprintf("no test cites %s", row.ID)})
			continue
		}

		contributors, rowFindings, err := rowContributors(row.ID, boundSites, rep.Records)
		if err != nil {
			return nil, nil, nil, err
		}
		findings = append(findings, rowFindings...)

		var executed []runrecord.Status
		for _, c := range contributors {
			if c.Executed {
				executed = append(executed, c.Status)
			}
		}
		if len(executed) == 0 {
			continue
		}
		status, err := runrecord.Collapse(executed)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("trace: join: row %s: %w", row.ID, err)
		}

		primary := contributors[0]
		results = append(results, RowResult{
			RowID:        row.ID,
			Scope:        primary.Scope,
			Name:         primary.Name,
			Status:       status,
			File:         primary.File,
			Line:         primary.Line,
			EndLine:      primary.EndLine,
			Contributors: contributors,
		})
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

// rowContributors turns every site bound to rowID into a Contributor and
// reports what each contributor's execution earns the row: a gap when its
// family never ran or collapsed to skipped, a red when it collapsed to
// failed. Contributors come back sorted by File then Line.
func rowContributors(rowID string, sites []TagSite, records []runrecord.Record) ([]Contributor, []Finding, error) {
	contributors := make([]Contributor, 0, len(sites))
	var findings []Finding

	for _, site := range sites {
		c := Contributor{
			Scope:   site.ImportPath,
			Name:    site.TestName,
			File:    site.File,
			Line:    site.Line,
			EndLine: site.EndLine,
		}

		family := familyRecords(records, site.ImportPath, site.TestName)
		if len(family) == 0 {
			findings = append(findings, Finding{
				Kind:   "gap",
				RowID:  rowID,
				Detail: fmt.Sprintf("no test executed for %s cited by %s:%d", rowID, site.File, site.Line),
			})
			contributors = append(contributors, c)
			continue
		}

		statuses := make([]runrecord.Status, len(family))
		for i, rec := range family {
			statuses[i] = rec.Status
		}
		status, err := runrecord.Collapse(statuses)
		if err != nil {
			return nil, nil, fmt.Errorf("trace: join: row %s: %w", rowID, err)
		}
		c.Status = status
		c.Executed = true

		switch status {
		case runrecord.Failed:
			findings = append(findings, Finding{
				Kind:   "red",
				RowID:  rowID,
				Detail: fmt.Sprintf("%s: %s at %s:%d failed", rowID, site.TestName, site.File, site.Line),
			})
		case runrecord.Skipped:
			findings = append(findings, Finding{
				Kind:   "gap",
				RowID:  rowID,
				Detail: fmt.Sprintf("%s: %s at %s:%d skipped", rowID, site.TestName, site.File, site.Line),
			})
		}
		contributors = append(contributors, c)
	}

	sort.SliceStable(contributors, func(i, j int) bool {
		if contributors[i].File != contributors[j].File {
			return contributors[i].File < contributors[j].File
		}
		return contributors[i].Line < contributors[j].Line
	})

	return contributors, findings, nil
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
