package main

import (
	"flag"
	"fmt"
	"io"
	"sort"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
)

type nextIDEntry struct {
	Kind string `json:"kind"`
	Area string `json:"area"`
	Next int    `json:"next"`
}

type nextIDOutput struct {
	evidence.Envelope
	NextIDs []nextIDEntry `json:"nextIds"`
}

func nextIDHandler(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError {
	repoRoot := flagString(fs, "repo")

	reg, err := coverage.BuildRegistry(repoRoot)
	if err != nil {
		return &cliError{Code: "registry-error", Message: fmt.Sprintf("next-id: %v", err)}
	}

	var areas []string
	if flagWasSet(fs, "area") {
		areas = []string{flagString(fs, "area")}
	} else {
		areas = reg.Areas()
	}

	entries := make([]nextIDEntry, 0, len(areas))
	for _, area := range areas {
		next, err := reg.NextID(area)
		if err != nil {
			return &cliError{Code: "invalid-area", Message: fmt.Sprintf("next-id: %v", err)}
		}
		entries = append(entries, nextIDEntry{Kind: "sc", Area: area, Next: next})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}
		return entries[i].Area < entries[j].Area
	})

	out := nextIDOutput{
		Envelope: evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		NextIDs:  entries,
	}
	return writeStdout(stdout, out)
}
