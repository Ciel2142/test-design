package coverage

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Registry is the repo-global set of scenario IDs harvested from every
// coverage document and floor-rule spec under a repo root. It is derived
// fresh by BuildRegistry on each call and never stored to disk.
type Registry struct {
	owners map[string]Owner
}

// Owner identifies the document that defines a scenario ID. DocPath is
// repo-root-relative and slash-separated.
type Owner struct {
	ID      string
	DocPath string
	Active  bool
}

var areaPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// BuildRegistry scans docs/superpowers/coverage/*-coverage.md and
// docs/superpowers/specs/*.md under repoRoot and harvests every scenario
// ID they define, plus each document's retired IDs. A directory that does
// not exist contributes nothing, and so does a document with no scenario
// table in either header form — including its ## Retired section, which in
// such a document is prose rather than an ID list. An unreadable document
// that does exist is a structured error naming its path. An ID defined by
// two documents, or twice within one document, is a hard failure naming the
// ID and the document path(s) involved.
func BuildRegistry(repoRoot string) (*Registry, error) {
	info, err := os.Stat(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("build registry: repository root %s: %w", repoRoot, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("build registry: repository root %s is not a directory", repoRoot)
	}
	owners := make(map[string]Owner)

	if err := scanRegistryDir(repoRoot, filepath.Join("docs", "superpowers", "coverage"), "*-coverage.md", owners); err != nil {
		return nil, err
	}
	if err := scanRegistryDir(repoRoot, filepath.Join("docs", "superpowers", "specs"), "*.md", owners); err != nil {
		return nil, err
	}

	return &Registry{owners: owners}, nil
}

func scanRegistryDir(repoRoot, relDir, glob string, owners map[string]Owner) error {
	dir := filepath.Join(repoRoot, relDir)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("build registry: stat %s: %w", dir, err)
	}

	matches, err := filepath.Glob(filepath.Join(dir, glob))
	if err != nil {
		return fmt.Errorf("build registry: glob %s: %w", dir, err)
	}

	for _, path := range matches {
		if err := harvestDocument(repoRoot, path, owners); err != nil {
			return err
		}
	}
	return nil
}

func harvestDocument(repoRoot, path string, owners map[string]Owner) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("build registry: read %s: %w", path, err)
	}
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return fmt.Errorf("build registry: relativize %s: %w", path, err)
	}
	docPath := filepath.ToSlash(rel)
	lines := strings.Split(string(data), "\n")

	var scenarioBlocks []tableBlock
	for _, b := range scanTables(lines) {
		if scenarioHeaderSignature.matches(b.header) || oldScenarioHeaderSignature.matches(b.header) {
			scenarioBlocks = append(scenarioBlocks, b)
		}
	}
	// docs/superpowers/specs holds arbitrary design documents; only one that
	// carries a scenario table is a coverage source whose ## Retired section
	// is an id list rather than prose.
	if len(scenarioBlocks) == 0 {
		return nil
	}

	retired, err := parseRetiredSection(docPath, lines)
	if err != nil {
		return fmt.Errorf("build registry: %w", err)
	}
	retiredSet := make(map[string]bool, len(retired))
	for _, id := range retired {
		retiredSet[id] = true
	}

	seen := make(map[string]bool)
	for _, b := range scenarioBlocks {
		for _, r := range b.rows {
			if len(r.cells) == 0 || !idPattern.MatchString(r.cells[0]) {
				continue
			}
			id := r.cells[0]
			if err := claimOwner(owners, seen, id, docPath, !retiredSet[id]); err != nil {
				return err
			}
		}
	}

	for _, id := range retired {
		if seen[id] {
			continue
		}
		if err := claimOwner(owners, seen, id, docPath, false); err != nil {
			return err
		}
	}

	return nil
}

func claimOwner(owners map[string]Owner, seen map[string]bool, id, docPath string, active bool) error {
	if seen[id] {
		return fmt.Errorf("build registry: duplicate id %s in %s", id, docPath)
	}
	seen[id] = true

	if existing, ok := owners[id]; ok {
		return fmt.Errorf("build registry: id %s defined in both %s and %s", id, existing.DocPath, docPath)
	}
	owners[id] = Owner{ID: id, DocPath: docPath, Active: active}
	return nil
}

// Owner returns the document that owns id and whether id is registered at
// all.
func (r *Registry) Owner(id string) (Owner, bool) {
	o, ok := r.owners[id]
	return o, ok
}

// Areas returns every area the registry has seen at least one ID for
// (active or retired), sorted so the unprefixed area ("") sorts first.
func (r *Registry) Areas() []string {
	seen := make(map[string]bool)
	for id := range r.owners {
		area, _ := splitID(id)
		seen[area] = true
	}
	areas := make([]string, 0, len(seen))
	for a := range seen {
		areas = append(areas, a)
	}
	sort.Strings(areas)
	return areas
}

// NextID returns one greater than the highest scenario number seen for
// area across every scanned document, counting retired IDs so a retired
// number is never reissued. The empty string means unprefixed SC-nn IDs.
func (r *Registry) NextID(area string) (int, error) {
	if area != "" && !areaPattern.MatchString(area) {
		return 0, fmt.Errorf("next id: invalid area %q", area)
	}

	highest := 0
	for id := range r.owners {
		idArea, num := splitID(id)
		if idArea != area {
			continue
		}
		if num > highest {
			highest = num
		}
	}
	return highest + 1, nil
}

func splitID(id string) (area string, num int) {
	idx := strings.LastIndex(id, "SC-")
	if idx < 0 {
		return "", 0
	}
	area = strings.TrimSuffix(id[:idx], "/")
	num, _ = strconv.Atoi(id[idx+len("SC-"):])
	return area, num
}
