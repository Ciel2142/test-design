// Package gotestjson normalizes a captured `go test -json` (test2json) event
// stream into the verifier's runrecord shape.
package gotestjson

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"test-design/internal/runrecord"
)

type Counts struct {
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

// RunnerFailure is a package-level failure with no associated test identity:
// a build error, or a package that failed without attributing the failure to
// any single test.
type RunnerFailure struct {
	Package string
	Detail  string
}

type Report struct {
	Records  []runrecord.Record
	Counts   Counts
	Failures []RunnerFailure
}

type event struct {
	Action     string
	Package    string
	ImportPath string
	Test       string
	Output     string
}

var testActionStatus = map[string]string{
	"pass": "passed",
	"fail": "failed",
	"skip": "skipped",
}

func Normalize(r io.Reader) (*Report, error) {
	report := &Report{}
	packageOutput := make(map[string]string)

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var ev event
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, fmt.Errorf("gotestjson: line %d: %w", lineNum, err)
		}

		key := ev.Package
		if key == "" {
			key = ev.ImportPath
		}

		switch ev.Action {
		case "pass", "fail", "skip":
			if ev.Test != "" {
				status, err := runrecord.ParseStatus(testActionStatus[ev.Action])
				if err != nil {
					return nil, fmt.Errorf("gotestjson: line %d: %w", lineNum, err)
				}
				report.Records = append(report.Records, runrecord.Record{
					Status: status,
					Scope:  ev.Package,
					Name:   ev.Test,
				})
				tally(&report.Counts, status)
				break
			}
			if ev.Action == "fail" {
				report.Failures = append(report.Failures, RunnerFailure{
					Package: key,
					Detail:  failureDetail(packageOutput, key),
				})
			}
		case "build-fail":
			report.Failures = append(report.Failures, RunnerFailure{
				Package: key,
				Detail:  failureDetail(packageOutput, key),
			})
		case "output", "build-output":
			if ev.Test == "" {
				packageOutput[key] += ev.Output
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("gotestjson: %w", err)
	}

	return report, nil
}

func failureDetail(packageOutput map[string]string, key string) string {
	if detail, ok := packageOutput[key]; ok && detail != "" {
		return detail
	}
	return fmt.Sprintf("gotestjson: package %s failed with no captured output", key)
}

func tally(c *Counts, status runrecord.Status) {
	switch status {
	case runrecord.Passed:
		c.Passed++
	case runrecord.Failed:
		c.Failed++
	case runrecord.Skipped:
		c.Skipped++
	}
}
