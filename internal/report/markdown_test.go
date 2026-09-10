package report

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
)

func mustRender(t *testing.T, r *Report) string {
	t.Helper()
	out, err := RenderMarkdown(r)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	return string(out)
}

func TestRenderMarkdownDeterministic(t *testing.T) {
	r := mustCompute(t, greenInputs())
	first := mustRender(t, r)
	second := mustRender(t, r)
	if first != second {
		t.Fatalf("RenderMarkdown is not deterministic:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestRenderMarkdownSectionOrder(t *testing.T) {
	r := mustCompute(t, greenInputs())
	md := mustRender(t, r)

	headings := []string{
		"# Verification: " + r.Source,
		"## Layer 1 — trace",
		"## Layer 2 — adversarial read",
		"## Layer 3 — live",
		"## Layer 4 — mutation",
		"## Work",
		"## Transcripts",
		"## What must happen before this branch can finish",
	}
	last := -1
	for _, h := range headings {
		idx := strings.Index(md, h)
		if idx == -1 {
			t.Fatalf("markdown missing heading %q; got:\n%s", h, md)
		}
		if idx <= last {
			t.Fatalf("heading %q out of order (idx %d after %d)", h, idx, last)
		}
		last = idx
	}
}

func TestRenderMarkdownHeaderFields(t *testing.T) {
	r := mustCompute(t, greenInputs())
	r.Approval = `alex — "go ahead" (2026-08-31T00:00Z)`
	md := mustRender(t, r)

	for _, want := range []string{
		r.Source,
		r.SourceSHA256,
		r.HeadCommit,
		r.BaselineCommit,
		r.BaselineStatus,
		r.RunID,
		r.SchemaVersion,
		r.VerifierVersion,
		r.Approval,
		"**Verdict: PASS**",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing header field %q; got:\n%s", want, md)
		}
	}
}

func TestRenderMarkdownAcknowledgementPending(t *testing.T) {
	in := greenInputs()
	in.Trace.BaselineStatus = "missing"
	r := mustCompute(t, in)
	wantVerdict(t, r, "PARTIAL")

	md := mustRender(t, r)
	if !strings.Contains(md, "PARTIAL") {
		t.Fatalf("markdown missing PARTIAL verdict")
	}
	if !strings.Contains(strings.ToLower(md), "acknowledg") {
		t.Errorf("markdown for an unacknowledged PARTIAL should say acknowledgement is pending; got:\n%s", md)
	}
	if strings.Contains(md, r.Acknowledgement) && r.Acknowledgement != "" {
		t.Errorf("markdown should not carry an acknowledgement line when unacknowledged")
	}
}

func TestRenderMarkdownAcknowledgementPresent(t *testing.T) {
	const line = "PARTIAL accepted by igi — no baseline on this branch"
	in := greenInputs()
	in.Trace.BaselineStatus = "missing"
	in.Acknowledgement = line
	r := mustCompute(t, in)
	if !r.Acknowledged {
		t.Fatalf("scenario setup: want Acknowledged true")
	}

	md := mustRender(t, r)
	if !strings.Contains(md, line) {
		t.Errorf("markdown missing verbatim acknowledgement line; got:\n%s", md)
	}
}

func TestRenderMarkdownPassHasNoAcknowledgementLine(t *testing.T) {
	r := mustCompute(t, greenInputs())
	md := mustRender(t, r)
	if strings.Contains(strings.ToLower(md), "acknowledg") {
		t.Errorf("PASS markdown should not mention acknowledgement; got:\n%s", md)
	}
}

func TestRenderMarkdownLayer1Table(t *testing.T) {
	r := mustCompute(t, gapInputs())
	md := mustRender(t, r)

	if !strings.Contains(md, "| SC-01 | unit | low | bound | pkg/a TestA |") {
		t.Errorf("markdown missing bound row with scope/name; got:\n%s", md)
	}
	if !strings.Contains(md, "| SC-02 | smoke | high | gap |") {
		t.Errorf("markdown missing gap row; got:\n%s", md)
	}
	if strings.Contains(md, "gap | pkg/b") {
		t.Errorf("gap row should not carry a scope/name; got:\n%s", md)
	}
}

func TestRenderMarkdownLayer2BatchTable(t *testing.T) {
	r := mustCompute(t, greenInputs())
	md := mustRender(t, r)

	if !strings.Contains(md, "| batch-1 | run-1 | SC-01 | - |") {
		t.Errorf("markdown missing batch-1 row; got:\n%s", md)
	}
	if !strings.Contains(md, "| batch-2 | run-1 | SC-02, SC-03 | - |") {
		t.Errorf("markdown missing batch-2 row; got:\n%s", md)
	}
	if !strings.Contains(md, "HOLDS") {
		t.Errorf("markdown missing HOLDS verdict; got:\n%s", md)
	}
	if strings.Contains(md, "Canary") {
		t.Errorf("markdown should carry no Canary token; got:\n%s", md)
	}
}

func TestRenderMarkdownLayer3NAIsOneLine(t *testing.T) {
	in := greenInputs()
	in.Source.Rows = []coverage.Row{{ID: "SC-01", Level: "unit", Risk: "low"}}
	in.Source.Retired = nil
	in.Trace.RowResults = []TraceRowResult{{RowID: "SC-01", Scope: "pkg/a", Name: "TestA", Status: "passed"}}
	in.Trace.Counts = SuiteCounts{Passed: 1}
	in.Trace.RowAccounting = RowAccounting{ActiveRows: 1, WithResult: 1}
	in.Batches = []Batch{{
		BatchID:  "batch-1",
		RunID:    "run-1",
		Rows:     []string{"SC-01"},
		Verdicts: []CriticVerdict{{RowID: "SC-01", Verdict: "HOLDS"}},
	}}
	in.Live = nil
	in.Mutations = nil

	r := mustCompute(t, in)
	wantLayerStatus(t, r, "pass", "pass", "n/a", "n/a")

	md := mustRender(t, r)
	l3 := md[strings.Index(md, "## Layer 3"):strings.Index(md, "## Layer 4")]
	if strings.Count(l3, "\n") > 4 {
		t.Errorf("Layer 3 n/a section should be a single summary line, got:\n%s", l3)
	}
	const want = "Status: n/a — no applicable live rows (live decision not-applicable)."
	if !strings.Contains(l3, want) {
		t.Errorf("Layer 3 n/a section should state the exact n/a sentence %q; got:\n%s", want, l3)
	}
}

// n/a is also reachable with Applicable > 0: all applicable rows gap.
func TestRenderMarkdownLayer3NAAllGapsIsOneLine(t *testing.T) {
	in := &Inputs{
		Source: &coverage.Source{
			Path:           greenSourcePath,
			SHA256:         "sha-source-1",
			BaselineCommit: "base-1",
			Rows: []coverage.Row{
				{ID: "SC-00", Level: "unit", Risk: "low"},
				{ID: "SC-01", Level: "smoke", Risk: "low"},
				{ID: "SC-02", Level: "e2e", Risk: "low"},
			},
		},
		RunID:      "run-1",
		HeadCommit: "head-1",
		Trace: &TraceEvidence{
			RunID:          "run-1",
			Source:         greenSourcePath,
			SourceSHA256:   "sha-source-1",
			RepoCommit:     "head-1",
			BaselineCommit: "base-1",
			BaselineStatus: "ok",
			Counts:         SuiteCounts{Passed: 1},
			RowAccounting:  RowAccounting{ActiveRows: 3, WithResult: 1, Unaccounted: []string{"SC-01", "SC-02"}},
			FindingCounts:  map[string]int{},
			RowResults:     []TraceRowResult{{RowID: "SC-00", Scope: "pkg/a", Name: "TestA", Status: "passed"}},
		},
		Freshness: Freshness{Result: "fresh"},
		Batches: []Batch{
			{
				BatchID:  "batch-1",
				RunID:    "run-1",
				Rows:     []string{"SC-00"},
				Verdicts: []CriticVerdict{{RowID: "SC-00", Verdict: "HOLDS"}},
			},
			{
				BatchID: "batch-2",
				RunID:   "run-1",
				Rows:    []string{"SC-01", "SC-02"},
				Verdicts: []CriticVerdict{
					{RowID: "SC-01", Verdict: "MISSING"},
					{RowID: "SC-02", Verdict: "MISSING"},
				},
			},
		},
		Live: &LiveEvidence{
			RunID:        "run-1",
			Source:       greenSourcePath,
			SourceSHA256: "sha-source-1",
			RepoCommit:   "head-1",
			RowVerdicts: []LiveRowVerdict{
				{RowID: "SC-01", Verdict: "LIVE-N/A (layer 1 gap)"},
				{RowID: "SC-02", Verdict: "LIVE-N/A (layer 1 gap)"},
			},
			CleanupOK:     true,
			SnapshotEqual: true,
		},
	}

	r := mustCompute(t, in)
	if r.Layers.Layer3.Status != "n/a" || r.Layers.Layer3.Applicable != 2 {
		t.Fatalf("scenario setup: Layer3 = %+v, want status n/a with 2 applicable rows", r.Layers.Layer3)
	}

	md := mustRender(t, r)
	l3 := md[strings.Index(md, "## Layer 3"):strings.Index(md, "## Layer 4")]
	if strings.Count(l3, "\n") > 4 {
		t.Errorf("Layer 3 n/a-with-applicable-rows section should be a single summary line, got:\n%s", l3)
	}
	if !strings.Contains(l3, "n/a") || !strings.Contains(l3, "2") {
		t.Errorf("Layer 3 section should state n/a status and the applicable count; got:\n%s", l3)
	}
}

func TestRenderMarkdownLayer4SurvivingSites(t *testing.T) {
	in := greenInputs()
	in.Mutations[0].Evidence.Sites[1].Outcome = "survived"
	r := mustCompute(t, in)
	wantVerdict(t, r, "BLOCKED")

	md := mustRender(t, r)
	if !strings.Contains(md, "MUTATION-SURVIVED") || !strings.Contains(md, "pkg/b.go:20") {
		t.Errorf("markdown missing surviving site detail; got:\n%s", md)
	}
}

func TestWorkCountsOf(t *testing.T) {
	items := []WorkItem{
		{Source: WorkFresh, Standing: StandingOK},
		{Source: WorkFresh, Standing: StandingFailed},
		{Source: WorkReused, Standing: StandingOK},
		{Source: WorkReused, Standing: StandingIncomplete},
		{Source: WorkFresh, Standing: StandingIncomplete},
		{Source: WorkReused, Standing: StandingFailed},
	}
	got := WorkCountsOf(items)
	want := WorkCounts{Fresh: 3, Reused: 3, Failed: 2, Incomplete: 2}
	if got != want {
		t.Errorf("WorkCountsOf = %+v, want %+v", got, want)
	}
}

// workFixtureItems mixes every Kind, Source and Standing this wave defines so
// a single fixture exercises the full new vocabulary: a fresh ok phase, a
// reused ok review-group (carrying a full Reuse reference and basis), a fresh
// failed live-attempt, and a reused incomplete mutation-row.
func workFixtureItems() []WorkItem {
	return []WorkItem{
		{
			Kind: "phase", ID: "trace", Rows: []string{},
			Source: WorkFresh, Outcome: "ok", Standing: StandingOK,
			Artifact: TranscriptRef{Path: "transcripts/run-1/trace-evidence.json", SHA256: "trace-sha"},
		},
		{
			Kind: "review-group", ID: "g1", Rows: []string{"SC-01", "SC-02"},
			Source: WorkReused, Outcome: "holds", Standing: StandingOK,
			Artifact: TranscriptRef{Path: "transcripts/run-1/reused-g1.json", SHA256: "g1-sha"},
			Reuse: &evidence.ReuseReference{
				PriorRunID:      "run-0",
				PriorRepoCommit: "head-0",
				PriorSeq:        3,
				Outcome:         "holds",
				Artifact:        evidence.FileRef{Path: "transcripts/run-0/review-g1.json", SHA256: "g1-sha"},
				Basis: evidence.ReuseBasis{
					PriorCommit:  "head-0",
					ChangedPaths: []string{"pkg/a.go"},
					ClaimRows:    []string{"SC-01", "SC-02"},
					ContextFiles: 2,
					Policy:       "matched",
					Toolchain:    "matched",
				},
			},
		},
		{
			Kind: "live-attempt", ID: "live", Rows: []string{},
			Source: WorkFresh, Outcome: "fail", Standing: StandingFailed,
			Artifact: TranscriptRef{Path: "transcripts/run-1/live-evidence.json", SHA256: "live-sha"},
		},
		{
			Kind: "mutation-row", ID: "SC-03", Rows: []string{"SC-03"},
			Source: WorkReused, Outcome: "environmental", Standing: StandingIncomplete,
			Artifact: TranscriptRef{Path: "transcripts/run-1/reused-mutation-attempt-SC-03.json", SHA256: "sc03-sha"},
			Reuse: &evidence.ReuseReference{
				PriorRunID:      "run-0",
				PriorRepoCommit: "head-0",
				PriorSeq:        5,
				Outcome:         "environmental",
				Artifact:        evidence.FileRef{Path: "transcripts/run-0/mutation-attempt-SC-03.json", SHA256: "sc03-sha"},
				Basis: evidence.ReuseBasis{
					PriorCommit:  "head-0",
					ChangedPaths: []string{},
					ClaimRows:    []string{"SC-03"},
					ContextFiles: 0,
					Policy:       "not-applicable",
					Toolchain:    "matched",
				},
			},
		},
	}
}

func TestRenderMarkdownWorkCountsAndTable(t *testing.T) {
	r := mustCompute(t, greenInputs())
	items := workFixtureItems()
	r.Work = WorkSummary{Items: items, Counts: WorkCountsOf(items)}

	md := mustRender(t, r)
	if !strings.Contains(md, "Fresh 2 · Reused 2 · Failed 1 · Incomplete 1") {
		t.Errorf("markdown missing Work counts line; got:\n%s", md)
	}
	if !strings.Contains(md, "| phase | trace | - | fresh | ok | ok | `transcripts/run-1/trace-evidence.json` |") {
		t.Errorf("markdown missing fresh phase Work row; got:\n%s", md)
	}
	if !strings.Contains(md, "| review-group | g1 | SC-01, SC-02 | reused | holds | ok | `transcripts/run-1/reused-g1.json` |") {
		t.Errorf("markdown missing reused review-group Work row; got:\n%s", md)
	}
	if !strings.Contains(md, "| live-attempt | live | - | fresh | fail | failed | `transcripts/run-1/live-evidence.json` |") {
		t.Errorf("markdown missing fresh live-attempt Work row; got:\n%s", md)
	}
	if !strings.Contains(md, "| mutation-row | SC-03 | SC-03 | reused | environmental | incomplete | `transcripts/run-1/reused-mutation-attempt-SC-03.json` |") {
		t.Errorf("markdown missing reused mutation-row Work row; got:\n%s", md)
	}
}

func TestRenderMarkdownWorkReusedBullet(t *testing.T) {
	r := mustCompute(t, greenInputs())
	items := []WorkItem{workFixtureItems()[1]} // the reused review-group g1
	r.Work = WorkSummary{Items: items, Counts: WorkCountsOf(items)}

	md := mustRender(t, r)
	want := "- review-group g1: reused from run run-0 seq 3 at head-0; " +
		"basis: prior commit head-0, 1 changed paths, claims SC-01,SC-02, 2 context files, policy matched, toolchain matched"
	if !strings.Contains(md, want) {
		t.Errorf("markdown missing reused-item bullet %q; got:\n%s", want, md)
	}
}

func TestRenderMarkdownWorkEmpty(t *testing.T) {
	r := mustCompute(t, greenInputs())
	r.Work = WorkSummary{Items: []WorkItem{}}

	md := mustRender(t, r)
	if !strings.Contains(md, "## Work") {
		t.Fatalf("markdown missing ## Work heading; got:\n%s", md)
	}
	if !strings.Contains(md, "Fresh 0 · Reused 0 · Failed 0 · Incomplete 0") {
		t.Errorf("markdown missing empty Work counts line; got:\n%s", md)
	}
	work := md[strings.Index(md, "## Work"):strings.Index(md, "## Transcripts")]
	if got := strings.Count(work, "\n| "); got != 2 {
		t.Errorf("empty Work section should render only the header and separator rows (2 lines starting \"| \"), got %d; section:\n%s", got, work)
	}
}

func TestWorkSummaryItemsAndWorkItemRowsSerializeAsEmptyArrayNeverNull(t *testing.T) {
	r := mustCompute(t, greenInputs())
	r.Work = WorkSummary{Items: []WorkItem{{
		Kind: "phase", ID: "trace", // Rows left nil
		Source: WorkFresh, Outcome: "ok", Standing: StandingOK,
		Artifact: TranscriptRef{Path: "p", SHA256: "s"},
	}}}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	raw := string(data)
	if strings.Contains(raw, `"rows":null`) {
		t.Errorf("WorkItem.Rows serialized as null:\n%s", raw)
	}
	if !strings.Contains(raw, `"rows":[]`) {
		t.Errorf("WorkItem.Rows did not serialize as []:\n%s", raw)
	}

	empty := WorkSummary{} // Items left nil
	data2, err := json.Marshal(empty)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	raw2 := string(data2)
	if strings.Contains(raw2, `"items":null`) {
		t.Errorf("WorkSummary.Items serialized as null:\n%s", raw2)
	}
	if !strings.Contains(raw2, `"items":[]`) {
		t.Errorf("WorkSummary.Items did not serialize as []:\n%s", raw2)
	}
}

func TestRenderMarkdownTranscripts(t *testing.T) {
	r := mustCompute(t, greenInputs())
	r.Transcripts = []TranscriptRef{{Path: "transcripts/run-1/trace-evidence.json", SHA256: "abc123"}}

	md := mustRender(t, r)
	if !strings.Contains(md, "transcripts/run-1/trace-evidence.json") || !strings.Contains(md, "abc123") {
		t.Errorf("markdown missing transcript entry; got:\n%s", md)
	}
}

func TestRenderMarkdownTranscriptsEmpty(t *testing.T) {
	r := mustCompute(t, greenInputs())
	md := mustRender(t, r)
	section := md[strings.Index(md, "## Transcripts"):strings.Index(md, "## What must happen")]
	if strings.TrimSpace(section) == "## Transcripts" {
		t.Errorf("expected an explicit statement for no transcripts; got:\n%s", section)
	}
}

func TestRenderMarkdownReasonsPass(t *testing.T) {
	r := mustCompute(t, greenInputs())
	md := mustRender(t, r)
	if !strings.Contains(md, "Nothing — verdict PASS.") {
		t.Errorf("markdown missing the PASS no-reasons line; got:\n%s", md)
	}
}

func TestRenderMarkdownReasonsBlocked(t *testing.T) {
	r := mustCompute(t, gapInputs())
	wantVerdict(t, r, "BLOCKED")
	md := mustRender(t, r)

	for _, rn := range r.Reasons {
		want := "- [" + rn.Layer + "] [" + rn.RowID + "] " + rn.Code + " — " + rn.Detail
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing reason line %q; got:\n%s", want, md)
		}
	}
}

func collectJSONStrings(t *testing.T, r *Report) map[string]bool {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	set := map[string]bool{}
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case string:
			set[t] = true
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(v)
	return set
}

func vocabularyTokens(r *Report) []string {
	tokens := []string{
		"pass", "blocked", "partial", "n/a",
		"PASS", "BLOCKED", "PARTIAL",
		"bound", "gap", "red", "drift", "overclaim", "ambiguity",
		"HOLDS", "WEAK", "VACUOUS", "MISSING",
		"LIVE-PASS", "LIVE-FAIL", "LIVE-INCONCLUSIVE", "LIVE-N/A (layer 1 gap)", "N/A (profile unavailable)", "N/A",
		"MUTATION-CAUGHT", "MUTATION-SURVIVED", "MUTATION-UNAPPLIABLE", "MUTATION-N/A (layer 1 gap)", "MUTATION-N/A (environmental)",
		"freshness",
		// Work vocabulary. "ok" and "failed" are deliberately excluded: "ok" is
		// a substring of the unrelated row level "smoke", and "failed" already
		// appears as a hardcoded label in the Layer 1 suite-counts line, so
		// either would false-fail this check against fixtures that carry no
		// Work data at all.
		"phase", "review-group", "live-attempt", "mutation-row",
		"fresh", "reused", "incomplete",
	}
	for _, rn := range r.Reasons {
		tokens = append(tokens, rn.Code)
	}
	return tokens
}

// Longest-first with masking: LIVE-PASS must not register as a bare PASS.
func assertMarkdownVocabularyIsInJSON(t *testing.T, r *Report) {
	t.Helper()
	md := mustRender(t, r)
	jsonStrings := collectJSONStrings(t, r)

	tokens := vocabularyTokens(r)
	sort.Slice(tokens, func(i, j int) bool { return len(tokens[i]) > len(tokens[j]) })

	masked := md
	seen := map[string]bool{}
	for _, tok := range tokens {
		if tok == "" || seen[tok] {
			continue
		}
		seen[tok] = true
		if !strings.Contains(masked, tok) {
			continue
		}
		if !jsonStrings[tok] {
			t.Errorf("markdown token %q found but not present as a JSON string value (verdict %s)", tok, r.Verdict)
		}
		masked = strings.ReplaceAll(masked, tok, strings.Repeat("\x00", len(tok)))
	}
}

func TestRenderMarkdownVocabularyIsJSONSourcedOnly(t *testing.T) {
	t.Run("all-green PASS", func(t *testing.T) {
		assertMarkdownVocabularyIsInJSON(t, mustCompute(t, greenInputs()))
	})
	t.Run("BLOCKED multi-cause", func(t *testing.T) {
		assertMarkdownVocabularyIsInJSON(t, mustCompute(t, gapInputs()))
	})
	t.Run("PARTIAL baseline missing", func(t *testing.T) {
		in := greenInputs()
		in.Trace.BaselineStatus = "missing"
		assertMarkdownVocabularyIsInJSON(t, mustCompute(t, in))
	})
	t.Run("PARTIAL acknowledged", func(t *testing.T) {
		in := greenInputs()
		in.Trace.BaselineStatus = "missing"
		in.Acknowledgement = "PARTIAL accepted by igi — no baseline on this branch"
		assertMarkdownVocabularyIsInJSON(t, mustCompute(t, in))
	})
	t.Run("reused work items", func(t *testing.T) {
		r := mustCompute(t, greenInputs())
		items := workFixtureItems()
		r.Work = WorkSummary{Items: items, Counts: WorkCountsOf(items)}
		assertMarkdownVocabularyIsInJSON(t, r)
	})
}
