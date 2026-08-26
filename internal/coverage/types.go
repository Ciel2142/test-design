// Package coverage parses coverage-source Markdown documents: the header
// lines, the ## Verification and ## Retired sections, and scenario tables
// identified by header signature.
package coverage

type RunnerConfig struct {
	Kind        string
	Root        string
	Command     []string
	ReportGlobs []string
}

type Row struct {
	ID       string
	Behavior string
	Expected string
	Level    string
	Mutation string
	Risk     string
}

type Source struct {
	Path                 string
	SpecPath             string
	BaselineCommit       string
	BaselineEvidencePath string
	BaselineSHA256       string
	SHA256               string
	Runner               RunnerConfig
	LiveProfile          string
	Rows                 []Row
	Retired              []string
	FinalCommand         []string
}
