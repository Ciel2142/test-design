package jvmrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"test-design/internal/coverage"
	"test-design/internal/runrecord"
)

const passFailXML = `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.example.ATest" tests="2">
  <testcase classname="com.example.ATest" name="adds"/>
  <testcase classname="com.example.ATest" name="failsHard"><failure message="boom">trace</failure></testcase>
</testsuite>`

func jvmConfig(command ...string) coverage.RunnerConfig {
	return coverage.RunnerConfig{
		Kind:        "maven",
		Root:        ".",
		Command:     command,
		ReportGlobs: []string{"reports/*.xml"},
	}
}

func shWriting(script string) []string {
	return []string{"sh", "-c", script}
}

func mustRun(t *testing.T, suiteDir string, cfg coverage.RunnerConfig) *Result {
	t.Helper()
	res, err := Run(suiteDir, t.TempDir(), cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}

func writeFixture(t *testing.T, root, name, content string) string {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestRun_WildcardCleanupPreservesModuleSource(t *testing.T) {
	root := t.TempDir()
	source := writeFixture(t, root, "modules/a/src/Example.java", "class Example {}")
	stale := writeFixture(t, root, "modules/a/target/reports/old.xml", passFailXML)
	other := writeFixture(t, root, "modules/a/target/reports/notes.txt", "keep")
	cfg := jvmConfig("sh", "-c", "true")
	cfg.ReportGlobs = []string{"modules/*/target/reports/*.xml"}

	res := mustRun(t, root, cfg)

	for file, want := range map[string]string{source: "class Example {}", other: "keep"} {
		if got, err := os.ReadFile(file); err != nil || string(got) != want {
			t.Errorf("unmatched file %s changed: %q, %v", file, got, err)
		}
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("preexisting matching report survived: %v", err)
	}
	if res.NormalizationError == "" || len(res.Records) != 0 {
		t.Fatalf("no-output command attested records: %+v", res)
	}
}

func TestRun_LeadingWildcardCannotReusePreexistingXML(t *testing.T) {
	for _, tc := range []struct{ glob, report string }{
		{"*/target/reports/*.xml", "module/target/reports/old.xml"},
		{"*.xml", "old.xml"},
	} {
		t.Run(tc.glob, func(t *testing.T) {
			root := t.TempDir()
			old := writeFixture(t, root, tc.report, passFailXML)
			// An unchanged report must be refused regardless of mtime precision
			// or clock skew; the former rounded cutoff accepts this report.
			stamp := time.Now().Add(time.Hour)
			if err := os.Chtimes(old, stamp, stamp); err != nil {
				t.Fatal(err)
			}
			cfg := jvmConfig("sh", "-c", "true")
			cfg.ReportGlobs = []string{tc.glob}
			res := mustRun(t, root, cfg)
			if res.NormalizationError == "" || len(res.Records) != 0 {
				t.Fatalf("unchanged XML attested execution: %+v", res)
			}
			if _, err := os.Stat(old); !os.IsNotExist(err) {
				t.Errorf("preexisting report not removed: %v", err)
			}
		})
	}
}

func TestRun_ValidatesAllPatternsBeforeCleanup(t *testing.T) {
	for _, glob := range []string{"[", "missing/[", "../outside/*.xml", "reports/../*.xml", "/abs/*.xml", ""} {
		t.Run(glob, func(t *testing.T) {
			root := t.TempDir()
			old := writeFixture(t, root, "reports/old.xml", passFailXML)
			cfg := jvmConfig("sh", "-c", "touch executed")
			cfg.ReportGlobs = []string{"reports/*.xml", glob}
			if _, err := Run(root, t.TempDir(), cfg); err == nil {
				t.Fatal("invalid report glob accepted")
			}
			if got, err := os.ReadFile(old); err != nil || string(got) != passFailXML {
				t.Errorf("report removed before all patterns validated: %q, %v", got, err)
			}
			if _, err := os.Stat(filepath.Join(root, "executed")); !os.IsNotExist(err) {
				t.Errorf("command executed with invalid glob: %v", err)
			}
		})
	}
}

func TestRun_SymlinkEscapeRefusedBeforeCleanup(t *testing.T) {
	for _, tc := range []struct{ link, target, glob string }{
		{"linked", ".", "linked/*.xml"},
		{"modules/linked", ".", "modules/*/*.xml"},
		{"reports/linked.xml", "outside.xml", "reports/*.xml"},
	} {
		t.Run(tc.link, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			old := writeFixture(t, root, "reports/old.xml", passFailXML)
			foreign := writeFixture(t, outside, "outside.xml", passFailXML)
			link := filepath.Join(root, filepath.FromSlash(tc.link))
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(outside, tc.target), link); err != nil {
				t.Fatal(err)
			}
			cfg := jvmConfig("sh", "-c", "touch executed")
			cfg.ReportGlobs = []string{"reports/old.xml", tc.glob}
			if _, err := Run(root, t.TempDir(), cfg); err == nil {
				t.Fatal("symlink escape accepted")
			}
			for _, file := range []string{old, foreign} {
				if got, err := os.ReadFile(file); err != nil || string(got) != passFailXML {
					t.Errorf("file %s changed before confinement completed: %q, %v", file, got, err)
				}
			}
			if _, err := os.Stat(filepath.Join(root, "executed")); !os.IsNotExist(err) {
				t.Errorf("command executed with escaping match: %v", err)
			}
		})
	}
}

func TestRun_PostRunSymlinkEscapeRefused(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	foreign := writeFixture(t, outside, "outside.xml", passFailXML)
	cfg := jvmConfig("sh", "-c", `mkdir reports && ln -s "$1" reports/new.xml`, "sh", foreign)
	if res, err := Run(root, t.TempDir(), cfg); err == nil {
		t.Fatalf("post-run external XML accepted: %+v", res)
	}
	if got, err := os.ReadFile(foreign); err != nil || string(got) != passFailXML {
		t.Fatalf("external report changed: %q, %v", got, err)
	}
}

func TestRun_CommandWritesFreshXMLProducesRecords(t *testing.T) {
	suiteDir := t.TempDir()
	cfg := jvmConfig(shWriting(`mkdir -p reports && cat > reports/a.xml <<'EOF'
` + passFailXML + `
EOF`)...)

	res := mustRun(t, suiteDir, cfg)

	if res.ExitStatus != 0 || res.NormalizationError != "" {
		t.Fatalf("ExitStatus = %d, NormalizationError = %q", res.ExitStatus, res.NormalizationError)
	}
	if res.Counts.Passed != 1 || res.Counts.Failed != 1 || res.Counts.Skipped != 0 {
		t.Errorf("Counts = %+v, want 1/1/0", res.Counts)
	}
	want := runrecord.Record{Status: runrecord.Failed, Scope: "com.example.ATest", Name: "failsHard"}
	found := false
	for _, r := range res.Records {
		if r == want {
			found = true
		}
	}
	if !found {
		t.Errorf("Records = %+v, want to contain %+v", res.Records, want)
	}
	if res.StartedAt == "" || res.FinishedAt == "" {
		t.Errorf("StartedAt = %q, FinishedAt = %q, want both set", res.StartedAt, res.FinishedAt)
	}
}

func TestRun_RemovesPreexistingReportAndFailsOnZeroFresh(t *testing.T) {
	suiteDir := t.TempDir()
	stale := filepath.Join(suiteDir, "reports", "stale.xml")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte(passFailXML), 0o644); err != nil {
		t.Fatal(err)
	}

	res := mustRun(t, suiteDir, jvmConfig("sh", "-c", "true"))

	if res.NormalizationError == "" {
		t.Error("NormalizationError empty, want zero-fresh-reports failure on exit 0")
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale report still present: %v", err)
	}
}

func TestRun_NewBackdatedWildcardReportNormalizes(t *testing.T) {
	suiteDir := t.TempDir()
	writeFixture(t, suiteDir, "module/target/reports/a.xml", "<old/>")
	cfg := jvmConfig(shWriting(`cat > module/target/reports/a.xml <<'EOF'
` + passFailXML + `
EOF
touch -t 202001010000 module/target/reports/a.xml`)...)
	cfg.ReportGlobs = []string{"*/target/reports/*.xml", "module/target/reports/a.xml"}

	res := mustRun(t, suiteDir, cfg)

	if res.NormalizationError != "" || res.Counts.Passed != 1 || res.Counts.Failed != 1 {
		t.Fatalf("new XML did not normalize exactly once: %+v", res)
	}
}

func TestRun_DTDReportFailsNormalizationNamingFile(t *testing.T) {
	suiteDir := t.TempDir()
	cfg := jvmConfig(shWriting(`mkdir -p reports && cat > reports/evil.xml <<'EOF'
<?xml version="1.0"?>
<!DOCTYPE testsuite [<!ENTITY x "y">]>
<testsuite name="a"><testcase classname="a" name="b"/></testsuite>
EOF`)...)

	res := mustRun(t, suiteDir, cfg)

	if res.NormalizationError == "" || !strings.Contains(res.NormalizationError, "evil.xml") {
		t.Errorf("NormalizationError = %q, want DTD refusal naming evil.xml", res.NormalizationError)
	}
}

func TestRun_NonzeroExitRecordedWithRecords(t *testing.T) {
	suiteDir := t.TempDir()
	cfg := jvmConfig(shWriting(`mkdir -p reports && cat > reports/a.xml <<'EOF'
` + passFailXML + `
EOF
exit 3`)...)

	res := mustRun(t, suiteDir, cfg)

	if res.ExitStatus != 3 {
		t.Errorf("ExitStatus = %d, want 3", res.ExitStatus)
	}
	if res.NormalizationError != "" || len(res.Records) == 0 {
		t.Errorf("NormalizationError = %q, Records = %d, want normalized records despite exit 3", res.NormalizationError, len(res.Records))
	}
}

func TestRun_UnsupportedFamilySurfaced(t *testing.T) {
	suiteDir := t.TempDir()
	cfg := jvmConfig(shWriting(`mkdir -p reports && cat > reports/a.xml <<'EOF'
<?xml version="1.0"?>
<testsuite name="com.example.DisplayTest">
  <testcase classname="com.example.DisplayTest" name="a phrased display name"/>
</testsuite>
EOF`)...)

	res := mustRun(t, suiteDir, cfg)

	if len(res.Unsupported) != 1 || res.Unsupported[0].Identity.Unsupported == "" {
		t.Fatalf("Unsupported = %+v, want one family with a reason", res.Unsupported)
	}
	want := runrecord.Record{Status: runrecord.Passed, Scope: "com.example.DisplayTest", Name: "a phrased display name"}
	found := false
	for _, r := range res.Records {
		if r == want {
			found = true
		}
	}
	if !found {
		t.Errorf("Records = %+v, want unsupported family present by raw name %+v", res.Records, want)
	}
}

func TestRun_GlobEscapeRefused(t *testing.T) {
	cfg := jvmConfig("sh", "-c", "true")
	cfg.ReportGlobs = []string{"../outside/*.xml"}
	_, err := Run(t.TempDir(), t.TempDir(), cfg)
	if err == nil || !strings.Contains(err.Error(), "report-glob-escapes-root") {
		t.Errorf("err = %v, want report-glob-escapes-root", err)
	}
}

func TestRun_CapturesStdoutAndStderrInRunDir(t *testing.T) {
	suiteDir := t.TempDir()
	runDir := t.TempDir()
	cfg := jvmConfig(shWriting(`mkdir -p reports && cat > reports/a.xml <<'EOF'
` + passFailXML + `
EOF
echo out-line
echo err-line >&2`)...)

	if _, err := Run(suiteDir, runDir, cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}

	out, err := os.ReadFile(filepath.Join(runDir, "runner-stdout.log"))
	if err != nil || !strings.Contains(string(out), "out-line") {
		t.Errorf("runner-stdout.log = %q, %v", out, err)
	}
	errLog, err := os.ReadFile(filepath.Join(runDir, "runner-stderr.log"))
	if err != nil || !strings.Contains(string(errLog), "err-line") {
		t.Errorf("runner-stderr.log = %q, %v", errLog, err)
	}
}
