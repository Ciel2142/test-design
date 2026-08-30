package jvmrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestReportDirs_StaticPrefixUniqueSorted(t *testing.T) {
	root := t.TempDir()
	dirs, err := ReportDirs(root, []string{
		"target/surefire-reports/*.xml",
		"target/surefire-reports/*.xml",
		"build/test-results/test/TEST-*.xml",
	})
	if err != nil {
		t.Fatalf("ReportDirs: %v", err)
	}
	want := []string{
		filepath.Join(root, "build", "test-results", "test"),
		filepath.Join(root, "target", "surefire-reports"),
	}
	if len(dirs) != 2 || dirs[0] != want[0] || dirs[1] != want[1] {
		t.Errorf("ReportDirs = %v, want %v", dirs, want)
	}
}

func TestReportDirs_MetaFirstSegmentContributesNoDir(t *testing.T) {
	dirs, err := ReportDirs(t.TempDir(), []string{"*.xml"})
	if err != nil {
		t.Fatalf("ReportDirs: %v", err)
	}
	if len(dirs) != 0 {
		t.Errorf("ReportDirs = %v, want none for a rootless glob", dirs)
	}
}

func TestReportDirs_EscapeRefused(t *testing.T) {
	for _, glob := range []string{"../outside/*.xml", "/abs/*.xml"} {
		_, err := ReportDirs(t.TempDir(), []string{glob})
		if err == nil || !strings.Contains(err.Error(), "report-glob-escapes-root") {
			t.Errorf("glob %q: err = %v, want report-glob-escapes-root", glob, err)
		}
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

func TestRun_ClearsPreexistingReportDirAndFailsOnZeroFresh(t *testing.T) {
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

func TestRun_OldMtimeReportExcluded(t *testing.T) {
	suiteDir := t.TempDir()
	cfg := jvmConfig(shWriting(`mkdir -p reports && cat > reports/a.xml <<'EOF'
` + passFailXML + `
EOF
touch -t 202001010000 reports/a.xml`)...)

	res := mustRun(t, suiteDir, cfg)

	if res.NormalizationError == "" {
		t.Error("NormalizationError empty, want stale mtime excluded leaving zero fresh reports")
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
