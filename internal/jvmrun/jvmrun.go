package jvmrun

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"test-design/internal/coverage"
	"test-design/internal/junitxml"
	"test-design/internal/runrecord"
	"test-design/internal/subprocess"
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

func confineGlob(glob string) error {
	if glob == "" {
		return errors.New("report-glob-escapes-root: empty glob")
	}
	if filepath.IsAbs(glob) {
		return fmt.Errorf("report-glob-escapes-root: %q is absolute", glob)
	}
	for _, segment := range strings.Split(filepath.ToSlash(glob), "/") {
		if segment == ".." {
			return fmt.Errorf("report-glob-escapes-root: %q contains traversal", glob)
		}
		if _, err := filepath.Match(segment, ""); err != nil {
			return fmt.Errorf("report glob %q: %w", glob, err)
		}
	}
	return nil
}

// Run removes only concrete preexisting report files, executes cfg.Command in
// the suite's runner root, and normalizes newly produced reports via junitxml.
// Report-problem outcomes land in Result.NormalizationError; the error return
// is reserved for infrastructure failures, including unsafe report paths and
// command timeout or cancellation.
func Run(suiteDir, runDir string, cfg coverage.RunnerConfig) (*Result, error) {
	root := filepath.Join(suiteDir, filepath.FromSlash(cfg.Root))
	reportsRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer reportsRoot.Close()
	clearFiles, err := reportFiles(reportsRoot, cfg.ReportGlobs)
	if err != nil {
		return nil, err
	}
	for _, file := range clearFiles {
		if err := reportsRoot.Remove(file); err != nil {
			return nil, fmt.Errorf("clear report file %s: %w", file, err)
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
	runErr := subprocess.Run(cmd, subprocess.SuiteTimeout)
	res.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			return nil, fmt.Errorf("run suite command: %w", runErr)
		}
		res.ExitStatus = exitErr.ExitCode()
	}

	// Every matching preexisting file was removed before execution. Collection
	// therefore does not rely on rounded mtimes or filesystem clock precision.
	files, err := reportFiles(reportsRoot, cfg.ReportGlobs)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		res.NormalizationError = "no fresh report file matched report-globs after the run"
		return res, nil
	}

	var docs []*junitxml.Document
	for _, file := range files {
		doc, err := normalizeOne(reportsRoot, file)
		if err != nil {
			res.NormalizationError = fmt.Sprintf("%s: %v", filepath.ToSlash(file), err)
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

// reportFiles validates every pattern and match before callers remove anything.
// Expansion uses rooted operations rather than filepath.Glob, which follows
// directory symlinks and silently ignores filesystem errors.
func reportFiles(root *os.Root, globs []string) ([]string, error) {
	for _, glob := range globs {
		if err := confineGlob(glob); err != nil {
			return nil, err
		}
	}
	seen := map[string]bool{}
	var files []string
	for _, glob := range globs {
		segments := strings.Split(filepath.Clean(filepath.FromSlash(glob)), string(filepath.Separator))
		dirs := []string{"."}
		for i, segment := range segments {
			var next []string
			for _, dir := range dirs {
				f, err := root.Open(dir)
				if err != nil {
					return nil, fmt.Errorf("open report directory %s: %w", dir, err)
				}
				entries, readErr := f.ReadDir(-1)
				closeErr := f.Close()
				if err := errors.Join(readErr, closeErr); err != nil {
					return nil, fmt.Errorf("read report directory %s: %w", dir, err)
				}
				for _, entry := range entries {
					matched, err := filepath.Match(segment, entry.Name())
					if err != nil {
						return nil, fmt.Errorf("report glob %q: %w", glob, err)
					}
					if !matched {
						continue
					}
					name := filepath.Join(dir, entry.Name())
					info, err := root.Lstat(name)
					if err != nil {
						return nil, fmt.Errorf("stat report path %s: %w", name, err)
					}
					if info.Mode()&os.ModeSymlink != 0 {
						return nil, fmt.Errorf("report-glob-escapes-root: symlink report path %s is not allowed", name)
					}
					if i < len(segments)-1 {
						if info.IsDir() {
							next = append(next, name)
						}
						continue
					}
					if info.IsDir() {
						continue
					}
					if !info.Mode().IsRegular() {
						return nil, fmt.Errorf("report path %s is not a regular file", name)
					}
					if !seen[name] {
						seen[name] = true
						files = append(files, name)
					}
				}
			}
			dirs = next
		}
	}
	sort.Strings(files)
	return files, nil
}

func normalizeOne(root *os.Root, file string) (*junitxml.Document, error) {
	f, err := root.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return junitxml.NormalizeFile(f)
}
