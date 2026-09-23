package report

import (
	"fmt"
	"sort"
	"strings"
)

func RenderMarkdown(r *Report) ([]byte, error) {
	var b strings.Builder

	writeMarkdownHeader(&b, r)
	writeMarkdownFreshness(&b, r)
	writeMarkdownLayer1(&b, r)
	writeMarkdownLayer2(&b, r)
	writeMarkdownLayer3(&b, r)
	writeMarkdownLayer4(&b, r)
	writeMarkdownWork(&b, r)
	writeMarkdownTranscripts(&b, r)
	writeMarkdownReasons(&b, r)

	return []byte(b.String()), nil
}

func writeMarkdownHeader(b *strings.Builder, r *Report) {
	fmt.Fprintf(b, "# Verification: %s\n\n", r.Source)
	fmt.Fprintf(b, "- Source: `%s` (sha256 `%s`)\n", r.Source, r.SourceSHA256)
	fmt.Fprintf(b, "- Head commit: `%s`\n", r.HeadCommit)
	fmt.Fprintf(b, "- Baseline: `%s` (%s)\n", r.BaselineCommit, r.BaselineStatus)
	fmt.Fprintf(b, "- Run: `%s`\n", r.RunID)
	fmt.Fprintf(b, "- Verifier: `%s` · Schema: `%s`\n", r.VerifierVersion, r.SchemaVersion)
	fmt.Fprintf(b, "- Approved by: %s\n", r.Approval)
	fmt.Fprintf(b, "- **Verdict: %s**\n", r.Verdict)
	switch {
	case r.Acknowledged:
		fmt.Fprintf(b, "- Acknowledgement: %s\n", r.Acknowledgement)
	case r.Verdict == VerdictPartial:
		b.WriteString("- Acknowledgement pending — this PARTIAL verdict has not been acknowledged.\n")
	}
	b.WriteString("\n")
}

// writeMarkdownFreshness renders the freshness record: the two commits it was
// measured over, every changed production path with its basis, and any stale
// hunk. r.Freshness is nil for a report published before 1.3.0, which the
// section says plainly rather than rendering an empty table.
func writeMarkdownFreshness(b *strings.Builder, r *Report) {
	b.WriteString("## Freshness\n\n")
	f := r.Freshness
	if f == nil {
		b.WriteString("Not recorded — this report predates freshness paths.\n\n")
		return
	}
	fmt.Fprintf(b, "Result: %s · Baseline: `%s` → HEAD: `%s`\n\n", f.Result, f.Base, f.Measured)

	if len(f.Paths) == 0 {
		b.WriteString("No changed production paths.\n\n")
	} else {
		b.WriteString("| Path | Basis |\n")
		b.WriteString("| --- | --- |\n")
		for _, p := range f.Paths {
			fmt.Fprintf(b, "| %s | %s |\n", escapeCell(p.Path), escapeCell(p.Basis))
		}
		b.WriteString("\n")
	}

	if len(f.Stale) > 0 {
		b.WriteString("| Hunk | Behavior |\n")
		b.WriteString("| --- | --- |\n")
		for _, s := range f.Stale {
			fmt.Fprintf(b, "| %s | %s |\n", escapeCell(s.Hunk), escapeCell(s.Behavior))
		}
		b.WriteString("\n")
	}
}

func writeMarkdownLayer1(b *strings.Builder, r *Report) {
	b.WriteString("## Layer 1 — trace\n\n")
	s := r.Layers.Layer1
	fmt.Fprintf(b, "Status: %s · Suite passed %d · failed %d · skipped %d · new failures %d\n\n",
		s.Status, s.Suite.Passed, s.Suite.Failed, s.Suite.Skipped, s.NewFailures)
	if len(s.FindingCounts) > 0 {
		kinds := make([]string, 0, len(s.FindingCounts))
		for kind := range s.FindingCounts {
			kinds = append(kinds, kind)
		}
		sort.Strings(kinds)
		parts := make([]string, 0, len(kinds))
		for _, kind := range kinds {
			parts = append(parts, fmt.Sprintf("%s %d", kind, s.FindingCounts[kind]))
		}
		fmt.Fprintf(b, "Findings: %s\n\n", strings.Join(parts, " · "))
	}

	b.WriteString("| Row | Level | Risk | Layer 1 | Scope/Name |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, row := range r.Rows {
		scopeName := ""
		if row.Layer1.Status == "bound" {
			scopeName = strings.TrimSpace(row.Layer1.Scope + " " + row.Layer1.Name)
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s |\n",
			escapeCell(row.RowID), escapeCell(row.Level), escapeCell(row.Risk),
			escapeCell(row.Layer1.Status), escapeCell(scopeName))
	}
	b.WriteString("\n")
}

func writeMarkdownLayer2(b *strings.Builder, r *Report) {
	b.WriteString("## Layer 2 — adversarial read\n\n")
	s := r.Layers.Layer2
	fmt.Fprintf(b, "Status: %s · Applicable %d · Holds %d · Weak %d · Vacuous %d · Missing %d\n\n",
		s.Status, s.Applicable, s.Holds, s.Weak, s.Vacuous, s.Missing)

	b.WriteString("| Row | Layer 2 | Batch |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, row := range r.Rows {
		fmt.Fprintf(b, "| %s | %s | %s |\n",
			escapeCell(row.RowID), escapeCell(row.Layer2.Verdict), escapeCell(row.Layer2.BatchID))
	}
	b.WriteString("\n")

	b.WriteString("| Batch | Run | Rows | Reused from |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, batch := range r.Batches {
		reusedFrom := "-"
		if batch.Reuse != nil {
			reusedFrom = fmt.Sprintf("%s seq %d", batch.Reuse.PriorRunID, batch.Reuse.PriorSeq)
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n",
			escapeCell(batch.BatchID), escapeCell(batch.RunID), escapeCell(strings.Join(batch.Rows, ", ")),
			escapeCell(reusedFrom))
	}
	b.WriteString("\n")
}

func writeMarkdownLayer3(b *strings.Builder, r *Report) {
	b.WriteString("## Layer 3 — live\n\n")
	s := r.Layers.Layer3
	if s.Status == statusNA {
		if s.Applicable == 0 {
			fmt.Fprintf(b, "Status: %s — no applicable live rows (live decision not-applicable).\n\n", s.Status)
		} else {
			fmt.Fprintf(b, "Status: %s — %d applicable rows, all layer-1 gaps.\n\n", s.Status, s.Applicable)
		}
		return
	}
	fmt.Fprintf(b, "Status: %s · Applicable %d · Pass %d · Fail %d · Inconclusive %d · N/A %d\n\n",
		s.Status, s.Applicable, s.Pass, s.Fail, s.Inconclusive, s.NA)
	b.WriteString("| Row | Layer 3 |\n")
	b.WriteString("| --- | --- |\n")
	for _, row := range r.Rows {
		fmt.Fprintf(b, "| %s | %s |\n", escapeCell(row.RowID), escapeCell(row.Layer3))
	}
	b.WriteString("\n")
}

func writeMarkdownLayer4(b *strings.Builder, r *Report) {
	b.WriteString("## Layer 4 — mutation\n\n")
	s := r.Layers.Layer4
	fmt.Fprintf(b, "Status: %s · Applicable %d · Caught %d · Survived %d · Unappliable %d · N/A %d\n\n",
		s.Status, s.Applicable, s.Caught, s.Survived, s.Unappliable, s.NA)
	b.WriteString("| Row | Layer 4 | Surviving Sites |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, row := range r.Rows {
		sites := "-"
		if len(row.Layer4.Sites) > 0 {
			sites = strings.Join(row.Layer4.Sites, ", ")
		}
		fmt.Fprintf(b, "| %s | %s | %s |\n", escapeCell(row.RowID), escapeCell(row.Layer4.Status), escapeCell(sites))
	}
	b.WriteString("\n")
}

func writeMarkdownWork(b *strings.Builder, r *Report) {
	b.WriteString("## Work\n\n")
	c := r.Work.Counts
	fmt.Fprintf(b, "Fresh %d · Reused %d · Failed %d · Incomplete %d\n\n", c.Fresh, c.Reused, c.Failed, c.Incomplete)

	b.WriteString("| Kind | ID | Rows | Source | Outcome | Standing | Artifact |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, item := range r.Work.Items {
		rows := "-"
		if len(item.Rows) > 0 {
			rows = strings.Join(item.Rows, ", ")
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s | `%s` |\n",
			escapeCell(item.Kind), escapeCell(item.ID), escapeCell(rows),
			escapeCell(item.Source), escapeCell(item.Outcome), escapeCell(item.Standing),
			escapeCell(item.Artifact.Path))
	}
	b.WriteString("\n")

	wrote := false
	for _, item := range r.Work.Items {
		if item.Reuse == nil {
			continue
		}
		ref := item.Reuse
		fmt.Fprintf(b, "- %s %s: reused from run %s seq %d at %s; basis: prior commit %s, %d changed paths, claims %s, %d context files, policy %s, toolchain %s\n",
			item.Kind, item.ID, ref.PriorRunID, ref.PriorSeq, ref.PriorRepoCommit,
			ref.Basis.PriorCommit, len(ref.Basis.ChangedPaths), strings.Join(ref.Basis.ClaimRows, ","),
			ref.Basis.ContextFiles, ref.Basis.Policy, ref.Basis.Toolchain)
		wrote = true
	}
	if wrote {
		b.WriteString("\n")
	}
}

func writeMarkdownTranscripts(b *strings.Builder, r *Report) {
	b.WriteString("## Transcripts\n\n")
	if len(r.Transcripts) == 0 {
		b.WriteString("None.\n\n")
		return
	}
	for _, t := range r.Transcripts {
		fmt.Fprintf(b, "- `%s` (sha256 `%s`)\n", t.Path, t.SHA256)
	}
	b.WriteString("\n")
}

func writeMarkdownReasons(b *strings.Builder, r *Report) {
	b.WriteString("## What must happen before this branch can finish\n\n")
	if len(r.Reasons) == 0 {
		b.WriteString("Nothing — verdict PASS.\n")
		return
	}
	for _, rn := range r.Reasons {
		fmt.Fprintf(b, "- [%s] [%s] %s — %s\n", rn.Layer, rn.RowID, rn.Code, rn.Detail)
	}
}

func escapeCell(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
