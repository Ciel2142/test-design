package main

import "testing"

// Declared under coverage amendment A-2: publication naming accepts any
// non-empty Markdown basename, preferring the -coverage.md stem when present.
func TestArtifactStem_AcceptsAnyMarkdownBasename(t *testing.T) {
	cases := []struct {
		source   string
		wantDir  string
		wantStem string
		wantCode string
	}{
		{"docs/superpowers/coverage/2026-01-01-widgets-coverage.md", "docs/superpowers/coverage", "2026-01-01-widgets", ""},
		{"docs/notes.md", "docs", "notes", ""},
		{"plain.md", "", "plain", ""},
		{"docs/.md", "", "", "source-name-unconventional"},
		{"docs/coverage.txt", "", "", "source-name-unconventional"},
		{"docs/-coverage.md", "", "", "source-name-unconventional"},
	}
	for _, tc := range cases {
		dir, stem, cliErr := artifactStem(tc.source)
		if tc.wantCode != "" {
			if cliErr == nil || cliErr.Code != tc.wantCode {
				t.Errorf("artifactStem(%q) error = %v, want code %q", tc.source, cliErr, tc.wantCode)
			}
			continue
		}
		if cliErr != nil {
			t.Errorf("artifactStem(%q) refused: %v", tc.source, cliErr)
			continue
		}
		if dir != tc.wantDir || stem != tc.wantStem {
			t.Errorf("artifactStem(%q) = (%q, %q), want (%q, %q)", tc.source, dir, stem, tc.wantDir, tc.wantStem)
		}
	}
}
