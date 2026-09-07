package coverage

import (
	"fmt"
	"regexp"
	"strings"
)

// Decision values of a coverage source's ## Live decision section.
const (
	LiveRequired      = "required"
	LiveNotApplicable = "not-applicable"
)

const liveDecisionHeading = "## Live decision"

var journeyIDPattern = regexp.MustCompile(`^J-[0-9]+$`)

// journeyHeaderSignature is inert to the registry and to extractRows: both
// harvest only the scenario header signatures in table.go, so a journeys
// table contributes no row and no owned id.
var journeyHeaderSignature = headerSignature{"Journey", "Scenarios", "Expected"}

// Journey is one live walkthrough of the deployed system: the scenario rows
// it observes and the observable result that settles it.
type Journey struct {
	ID        string
	Scenarios []string
	Expected  string
}

// LiveDecision is the machine-readable ## Live decision section. Journeys is
// empty for a not-applicable decision and is the single live selection source
// for a required one.
type LiveDecision struct {
	Decision string
	Reason   string
	Journeys []Journey
}

// LiveDecisionError names one incomplete live decision. Code is exactly one
// of missing, reason-missing, contradiction, journeys-missing,
// journey-unknown-row, live-row-unjourneyed and target-missing.
type LiveDecisionError struct {
	Path   string
	Code   string
	Detail string
}

func (e *LiveDecisionError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("live decision %s: %s", e.Path, e.Code)
	}
	return fmt.Sprintf("live decision %s: %s: %s", e.Path, e.Code, e.Detail)
}

// sectionBounds returns the 0-based index of the first line after a heading
// line equal to heading and the 0-based index one past the section's last
// line, which is the next line starting with "#" or the end of the document.
func sectionBounds(lines []string, heading string) (start, end int, found bool) {
	for i, line := range lines {
		if strings.TrimSpace(line) != heading {
			continue
		}
		start = i + 1
		for j := start; j < len(lines); j++ {
			if strings.HasPrefix(strings.TrimSpace(lines[j]), "#") {
				return start, j, true
			}
		}
		return start, len(lines), true
	}
	return 0, 0, false
}

// blockInSection reports whether b was scanned from inside the line range
// [start, end). A table with no data row carries no line and declares no
// journey, so it is treated as absent.
func blockInSection(b tableBlock, start, end int) bool {
	if len(b.rows) == 0 {
		return false
	}
	idx := b.rows[0].line - 1
	return idx >= start && idx < end
}

// parseLiveDecisionSection parses the optional ## Live decision section from
// lines, taking its journeys table from the document-wide blocks. It returns
// (nil, nil) when the section is absent — legacy documents keep parsing — and
// fails closed naming path and the offending line otherwise. Non-bullet,
// non-table prose inside the section is ignored.
func parseLiveDecisionSection(path string, lines []string, blocks []tableBlock) (*LiveDecision, error) {
	start, end, found := sectionBounds(lines, liveDecisionHeading)
	if !found {
		return nil, nil
	}
	headingLine := start // the heading's own 1-based line number

	var decision, reason string
	decisionLine := 0
	seen := make(map[string]bool)
	for i := start; i < end; i++ {
		key, value, ok := parseBulletKV(lines[i])
		if !ok {
			continue
		}
		if seen[key] {
			return nil, fmt.Errorf("parse source %s: live decision line %d: key %q: duplicate", path, i+1, key)
		}
		seen[key] = true
		switch key {
		case "decision":
			decision, decisionLine = strings.TrimSpace(value), i+1
		case "reason":
			reason = strings.TrimSpace(value)
		}
	}

	if !seen["decision"] {
		return nil, fmt.Errorf("parse source %s: live decision line %d: key %q: missing", path, headingLine, "decision")
	}
	if decision == "" {
		return nil, fmt.Errorf("parse source %s: live decision line %d: key %q: empty", path, decisionLine, "decision")
	}
	if decision != LiveRequired && decision != LiveNotApplicable {
		return nil, fmt.Errorf("parse source %s: live decision line %d: key %q: unknown decision %q, want %q or %q", path, decisionLine, "decision", decision, LiveRequired, LiveNotApplicable)
	}

	var journeyBlock *tableBlock
	for i := range blocks {
		b := &blocks[i]
		if !journeyHeaderSignature.matches(b.header) || !blockInSection(*b, start, end) {
			continue
		}
		if journeyBlock != nil {
			return nil, fmt.Errorf("parse source %s: live decision line %d: a second journeys table", path, b.rows[0].line)
		}
		journeyBlock = b
	}

	var journeys []Journey
	if journeyBlock != nil {
		for _, r := range journeyBlock.rows {
			journey, err := parseJourneyRow(path, r)
			if err != nil {
				return nil, err
			}
			journeys = append(journeys, journey)
		}
	}

	return &LiveDecision{Decision: decision, Reason: reason, Journeys: journeys}, nil
}

func parseJourneyRow(path string, r tableRow) (Journey, error) {
	if len(r.cells) != len(journeyHeaderSignature) {
		return Journey{}, fmt.Errorf("parse source %s: journey row at line %d: expected %d columns, got %d", path, r.line, len(journeyHeaderSignature), len(r.cells))
	}
	id, scenarios, expected := r.cells[0], r.cells[1], strings.TrimSpace(r.cells[2])

	if !journeyIDPattern.MatchString(id) {
		return Journey{}, fmt.Errorf("parse source %s: journey row at line %d: column Journey: malformed id %q", path, r.line, id)
	}
	if strings.TrimSpace(scenarios) == "" {
		return Journey{}, fmt.Errorf("parse source %s: journey row at line %d: column Scenarios: empty", path, r.line)
	}
	var ids []string
	for _, token := range strings.Split(scenarios, ",") {
		token = strings.TrimSpace(token)
		if !idPattern.MatchString(token) {
			return Journey{}, fmt.Errorf("parse source %s: journey row at line %d: column Scenarios: malformed id %q", path, r.line, token)
		}
		ids = append(ids, token)
	}
	if expected == "" {
		return Journey{}, fmt.Errorf("parse source %s: journey row at line %d: column Expected: empty", path, r.line)
	}

	return Journey{ID: id, Scenarios: ids, Expected: expected}, nil
}

// ValidateLiveDecision returns a *LiveDecisionError carrying exactly one code
// for the first fault found in this order — missing, reason-missing,
// contradiction, journeys-missing, journey-unknown-row, live-row-unjourneyed,
// target-missing — and nil for a justified not-applicable decision or a
// complete required one. An absent or incomplete decision is never defaulted
// to not-applicable.
func ValidateLiveDecision(src *Source) error {
	var path string
	if src != nil {
		path = src.Path
	}
	if src == nil || src.LiveDecision == nil {
		return &LiveDecisionError{Path: path, Code: "missing", Detail: "no " + liveDecisionHeading + " section"}
	}
	decision := src.LiveDecision

	retired := make(map[string]bool, len(src.Retired))
	for _, id := range src.Retired {
		retired[id] = true
	}
	active := make(map[string]bool, len(src.Rows))
	var activeLiveRows []string
	for _, row := range src.Rows {
		if retired[row.ID] {
			continue
		}
		active[row.ID] = true
		if row.Level == "smoke" || row.Level == "e2e" {
			activeLiveRows = append(activeLiveRows, row.ID)
		}
	}

	if decision.Decision == LiveNotApplicable {
		if strings.TrimSpace(decision.Reason) == "" {
			return &LiveDecisionError{Path: path, Code: "reason-missing", Detail: "decision not-applicable without a reason"}
		}
		if len(activeLiveRows) > 0 {
			return &LiveDecisionError{Path: path, Code: "contradiction", Detail: fmt.Sprintf("decision not-applicable while these active rows are smoke or e2e: %s", strings.Join(activeLiveRows, ", "))}
		}
		if len(decision.Journeys) > 0 {
			return &LiveDecisionError{Path: path, Code: "contradiction", Detail: fmt.Sprintf("decision not-applicable with journeys: %s", strings.Join(journeyIDs(decision.Journeys), ", "))}
		}
	}

	if decision.Decision == LiveRequired && len(decision.Journeys) == 0 {
		return &LiveDecisionError{Path: path, Code: "journeys-missing", Detail: "decision required with no journey"}
	}

	journeyed := make(map[string]bool)
	for _, journey := range decision.Journeys {
		for _, id := range journey.Scenarios {
			if !active[id] {
				return &LiveDecisionError{Path: path, Code: "journey-unknown-row", Detail: fmt.Sprintf("journey %s cites %s, which is not an active row of this document", journey.ID, id)}
			}
			journeyed[id] = true
		}
	}

	if decision.Decision == LiveRequired {
		var unjourneyed []string
		for _, id := range activeLiveRows {
			if !journeyed[id] {
				unjourneyed = append(unjourneyed, id)
			}
		}
		if len(unjourneyed) > 0 {
			return &LiveDecisionError{Path: path, Code: "live-row-unjourneyed", Detail: fmt.Sprintf("these active smoke or e2e rows appear in no journey: %s", strings.Join(unjourneyed, ", "))}
		}
		if strings.TrimSpace(src.LiveProfile) == "" {
			return &LiveDecisionError{Path: path, Code: "target-missing", Detail: "decision required while the ## Verification key \"live-profile\" is absent or empty"}
		}
	}

	return nil
}

func journeyIDs(journeys []Journey) []string {
	ids := make([]string, 0, len(journeys))
	for _, j := range journeys {
		ids = append(ids, j.ID)
	}
	return ids
}
