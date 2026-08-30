package jvmrun

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"test-design/internal/coverage"
	"test-design/internal/junitxml"
	"test-design/internal/runrecord"
)

// Result is one JVM suite run normalized to the shared record contract.
// NormalizationError non-empty means the run produced no trustworthy report
// — zero fresh XML files, or an unreadable one — even when ExitStatus is 0.
type Result struct {
	Records            []runrecord.Record
	Counts             junitxml.Counts
	Unsupported        []junitxml.Family
	ExitStatus         int
	StartedAt          string
	FinishedAt         string
	NormalizationError string
}

// ReportDirs resolves each glob's static directory prefix under root — the
// directories cleared before a run. A glob whose first segment already
// contains a meta character has no static prefix and contributes no
// directory. Any glob escaping root (absolute or traversing above) errors.
func ReportDirs(root string, globs []string) ([]string, error) {
	seen := map[string]bool{}
	var dirs []string
	for _, glob := range globs {
		if err := confineGlob(glob); err != nil {
			return nil, err
		}
		prefix := staticPrefix(glob)
		if prefix == "" {
			continue
		}
		abs := filepath.Join(root, filepath.FromSlash(prefix))
		if !seen[abs] {
			seen[abs] = true
			dirs = append(dirs, abs)
		}
	}
	sort.Strings(dirs)
	return dirs, nil
}

func confineGlob(glob string) error {
	if glob == "" {
		return errors.New("report-glob-escapes-root: empty glob")
	}
	if filepath.IsAbs(glob) {
		return fmt.Errorf("report-glob-escapes-root: %q is absolute", glob)
	}
	cleaned := path.Clean(filepath.ToSlash(glob))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("report-glob-escapes-root: %q traverses above the runner root", glob)
	}
	return nil
}

func staticPrefix(glob string) string {
	var static []string
	for _, seg := range strings.Split(path.Clean(filepath.ToSlash(glob)), "/") {
		if strings.ContainsAny(seg, "*?[") {
			break
		}
		static = append(static, seg)
	}
	if len(static) == 0 {
		return ""
	}
	return path.Join(static...)
}

// Run clears the report directories, executes cfg.Command in the suite's
// runner root, collects XML files matching cfg.ReportGlobs that are fresh
// (mtime at-or-after command start), and normalizes them via junitxml.
// Report-problem outcomes land in Result.NormalizationError; the error
// return is reserved for infrastructure failures (escaping glob,
// unstartable command, unwritable runDir).
func Run(suiteDir, runDir string, cfg coverage.RunnerConfig) (*Result, error) {
	root := filepath.Join(suiteDir, filepath.FromSlash(cfg.Root))
	clearDirs, err := ReportDirs(root, cfg.ReportGlobs)
	if err != nil {
		return nil, err
	}
	for _, dir := range clearDirs {
		if err := os.RemoveAll(dir); err != nil {
			return nil, fmt.Errorf("clear report dir %s: %w", dir, err)
		}
	}

	stdout, err := os.Create(filepath.Join(runDir, "runner-stdout.log"))
	if err != nil {
		return nil, fmt.Errorf("create runner stdout log: %w", err)
	}
	defer stdout.Close()
	stderr, err := os.Create(filepath.Join(runDir, "runner-stderr.log"))
	if err != nil {
		return nil, fmt.Errorf("create runner stderr log: %w", err)
	}
	defer stderr.Close()

	start := time.Now()
	res := &Result{StartedAt: start.UTC().Format(time.RFC3339)}

	cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
	cmd.Dir = root
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	runErr := cmd.Run()
	res.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			return nil, fmt.Errorf("run suite command: %w", runErr)
		}
		res.ExitStatus = exitErr.ExitCode()
	}

	// Truncate to whole seconds: a coarse-mtime filesystem rounds a fresh
	// file's mtime down, which must not exclude it.
	cutoff := start.Truncate(time.Second)
	files, err := freshReports(root, cfg.ReportGlobs, cutoff)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		res.NormalizationError = "no fresh report file matched report-globs after the run"
		return res, nil
	}

	var docs []*junitxml.Document
	for _, file := range files {
		doc, err := normalizeOne(file)
		if err != nil {
			rel, relErr := filepath.Rel(root, file)
			if relErr != nil {
				rel = file
			}
			res.NormalizationError = fmt.Sprintf("%s: %v", filepath.ToSlash(rel), err)
			return res, nil
		}
		docs = append(docs, doc)
	}

	report, err := junitxml.Merge(docs)
	if err != nil {
		res.NormalizationError = err.Error()
		return res, nil
	}

	res.Counts = report.Counts
	for _, f := range report.Families {
		name := f.Identity.Method
		if f.Identity.Unsupported != "" {
			name = f.Cases[0].RawName
			res.Unsupported = append(res.Unsupported, f)
		}
		res.Records = append(res.Records, runrecord.Record{
			Status: f.Status,
			Scope:  f.Identity.ClassName,
			Name:   name,
		})
	}
	return res, nil
}

func freshReports(root string, globs []string, cutoff time.Time) ([]string, error) {
	seen := map[string]bool{}
	var files []string
	for _, glob := range globs {
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(glob)))
		if err != nil {
			return nil, fmt.Errorf("report glob %q: %w", glob, err)
		}
		for _, m := range matches {
			if seen[m] {
				continue
			}
			seen[m] = true
			info, err := os.Stat(m)
			if err != nil || info.IsDir() || info.ModTime().Before(cutoff) {
				continue
			}
			files = append(files, m)
		}
	}
	sort.Strings(files)
	return files, nil
}

func normalizeOne(file string) (*junitxml.Document, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return junitxml.NormalizeFile(f)
}
