package mutation

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func modifyDiff(path string, hunks int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "diff --git a/%s b/%s\n", path, path)
	sb.WriteString("index 1111111..2222222 100644\n")
	fmt.Fprintf(&sb, "--- a/%s\n", path)
	fmt.Fprintf(&sb, "+++ b/%s\n", path)
	for i := 0; i < hunks; i++ {
		fmt.Fprintf(&sb, "@@ -%d,1 +%d,1 @@\n", i+1, i+1)
		sb.WriteString("-old\n")
		sb.WriteString("+new\n")
	}
	return sb.String()
}

func newFileDiff(path string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "diff --git a/%s b/%s\n", path, path)
	sb.WriteString("new file mode 100644\n")
	sb.WriteString("index 0000000..2222222\n")
	sb.WriteString("--- /dev/null\n")
	fmt.Fprintf(&sb, "+++ b/%s\n", path)
	sb.WriteString("@@ -0,0 +1,1 @@\n")
	sb.WriteString("+added\n")
	return sb.String()
}

func deletedFileDiff(path string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "diff --git a/%s b/%s\n", path, path)
	sb.WriteString("deleted file mode 100644\n")
	sb.WriteString("index 1111111..0000000\n")
	fmt.Fprintf(&sb, "--- a/%s\n", path)
	sb.WriteString("+++ /dev/null\n")
	sb.WriteString("@@ -1,1 +0,0 @@\n")
	sb.WriteString("-removed\n")
	return sb.String()
}

func renameDiff(from, to string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "diff --git a/%s b/%s\n", from, to)
	sb.WriteString("similarity index 90%\n")
	fmt.Fprintf(&sb, "rename from %s\n", from)
	fmt.Fprintf(&sb, "rename to %s\n", to)
	sb.WriteString("index 1111111..2222222 100644\n")
	fmt.Fprintf(&sb, "--- a/%s\n", from)
	fmt.Fprintf(&sb, "+++ b/%s\n", to)
	sb.WriteString("@@ -1,1 +1,1 @@\n")
	sb.WriteString("-old\n")
	sb.WriteString("+new\n")
	return sb.String()
}

func modeOnlyDiff(path string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "diff --git a/%s b/%s\n", path, path)
	sb.WriteString("old mode 100644\n")
	sb.WriteString("new mode 100755\n")
	return sb.String()
}

const goSitePath = "internal/widget/widget.go"
const jvmSitePath = "src/main/java/com/example/Foo.java"

func TestPoliceDiffAcceptsSingleFileSingleHunkGo(t *testing.T) {
	summary, err := PoliceDiff(modifyDiff(goSitePath, 1), "go")
	if err != nil {
		t.Fatalf("PoliceDiff: %v", err)
	}
	if summary.File != goSitePath || summary.Hunks != 1 {
		t.Errorf("summary = %+v, want File=%q Hunks=1", summary, goSitePath)
	}
}

func TestPoliceDiffAcceptsSingleFileSingleHunkMaven(t *testing.T) {
	summary, err := PoliceDiff(modifyDiff(jvmSitePath, 1), "maven")
	if err != nil {
		t.Fatalf("PoliceDiff: %v", err)
	}
	if summary.File != jvmSitePath || summary.Hunks != 1 {
		t.Errorf("summary = %+v, want File=%q Hunks=1", summary, jvmSitePath)
	}
}

func TestPoliceDiffAcceptsSingleFileSingleHunkGradle(t *testing.T) {
	summary, err := PoliceDiff(modifyDiff("src/main/kotlin/com/example/Foo.kt", 1), "gradle")
	if err != nil {
		t.Fatalf("PoliceDiff: %v", err)
	}
	if summary.File != "src/main/kotlin/com/example/Foo.kt" || summary.Hunks != 1 {
		t.Errorf("summary = %+v", summary)
	}
}

func TestPoliceDiffRejects(t *testing.T) {
	tests := []struct {
		name      string
		diff      string
		kind      string
		wantErr   error // sentinel to check with errors.Is; nil means plain error (no sentinel)
		wantNames []string
	}{
		{
			name:    "empty diff",
			diff:    "",
			kind:    "go",
			wantErr: ErrEmptyDiff,
		},
		{
			name:    "whitespace only diff",
			diff:    "   \n\t\n",
			kind:    "go",
			wantErr: ErrEmptyDiff,
		},
		{
			name:      "two files",
			diff:      modifyDiff(goSitePath, 1) + modifyDiff("internal/widget/other.go", 1),
			kind:      "go",
			wantErr:   ErrMultiSite,
			wantNames: []string{goSitePath, "internal/widget/other.go"},
		},
		{
			name:      "one file two hunks",
			diff:      modifyDiff(goSitePath, 2),
			kind:      "go",
			wantErr:   ErrMultiSite,
			wantNames: []string{goSitePath, "2"},
		},
		{
			name:      "new file",
			diff:      newFileDiff(goSitePath),
			kind:      "go",
			wantErr:   ErrForeignDiff,
			wantNames: []string{goSitePath},
		},
		{
			name:      "deleted file",
			diff:      deletedFileDiff(goSitePath),
			kind:      "go",
			wantErr:   ErrForeignDiff,
			wantNames: []string{goSitePath},
		},
		{
			name:      "rename",
			diff:      renameDiff(goSitePath, "internal/widget/renamed.go"),
			kind:      "go",
			wantErr:   ErrForeignDiff,
			wantNames: []string{"internal/widget/renamed.go"},
		},
		{
			name:      "mode-only change",
			diff:      modeOnlyDiff(goSitePath),
			kind:      "go",
			wantErr:   ErrMultiSite,
			wantNames: []string{goSitePath},
		},
		{
			name:      "go test file",
			diff:      modifyDiff("internal/widget/widget_test.go", 1),
			kind:      "go",
			wantErr:   ErrForeignDiff,
			wantNames: []string{"internal/widget/widget_test.go"},
		},
		{
			name:      "go.mod",
			diff:      modifyDiff("go.mod", 1),
			kind:      "go",
			wantErr:   ErrForeignDiff,
			wantNames: []string{"go.mod"},
		},
		{
			name:      "go.sum",
			diff:      modifyDiff("go.sum", 1),
			kind:      "go",
			wantErr:   ErrForeignDiff,
			wantNames: []string{"go.sum"},
		},
		{
			name:      "go testdata path",
			diff:      modifyDiff("internal/widget/testdata/fixture.go", 1),
			kind:      "go",
			wantErr:   ErrForeignDiff,
			wantNames: []string{"testdata/fixture.go"},
		},
		{
			name:      "jvm src/test file",
			diff:      modifyDiff("src/test/java/com/example/FooTest.java", 1),
			kind:      "maven",
			wantErr:   ErrForeignDiff,
			wantNames: []string{"src/test/java/com/example/FooTest.java"},
		},
		{
			name:      "pom.xml",
			diff:      modifyDiff("pom.xml", 1),
			kind:      "maven",
			wantErr:   ErrForeignDiff,
			wantNames: []string{"pom.xml"},
		},
		{
			name:      "gradle.kts build file",
			diff:      modifyDiff("build.gradle.kts", 1),
			kind:      "gradle",
			wantErr:   ErrForeignDiff,
			wantNames: []string{"build.gradle.kts"},
		},
		{
			name:      "settings.gradle",
			diff:      modifyDiff("settings.gradle", 1),
			kind:      "gradle",
			wantErr:   ErrForeignDiff,
			wantNames: []string{"settings.gradle"},
		},
		{
			name:      "gradle wrapper",
			diff:      modifyDiff("gradlew", 1),
			kind:      "gradle",
			wantErr:   ErrForeignDiff,
			wantNames: []string{"gradlew"},
		},
		{
			name:      "mvnw",
			diff:      modifyDiff("mvnw", 1),
			kind:      "maven",
			wantErr:   ErrForeignDiff,
			wantNames: []string{"mvnw"},
		},
		{
			name:      "file outside src/main for JVM",
			diff:      modifyDiff("src/scripts/java/com/example/Foo.java", 1),
			kind:      "maven",
			wantErr:   ErrForeignDiff,
			wantNames: []string{"src/scripts/java/com/example/Foo.java"},
		},
		{
			name:      "non-source extension under src/main",
			diff:      modifyDiff("src/main/resources/config.xml", 1),
			kind:      "maven",
			wantErr:   ErrForeignDiff,
			wantNames: []string{"src/main/resources/config.xml"},
		},
		{
			name:      "dot test-design path",
			diff:      modifyDiff(".test-design/coverage.yaml", 1),
			kind:      "go",
			wantErr:   ErrForeignDiff,
			wantNames: []string{".test-design/coverage.yaml"},
		},
		{
			name:      "docs path",
			diff:      modifyDiff("docs/README.md", 1),
			kind:      "go",
			wantErr:   ErrForeignDiff,
			wantNames: []string{"docs/README.md"},
		},
		{
			name:      "dotfile at repo root",
			diff:      modifyDiff(".gitignore", 1),
			kind:      "go",
			wantErr:   ErrForeignDiff,
			wantNames: []string{".gitignore"},
		},
		{
			name:      "unknown kind",
			diff:      modifyDiff(goSitePath, 1),
			kind:      "python",
			wantErr:   nil,
			wantNames: []string{"python"},
		},
		{
			name:      "malformed diff text",
			diff:      "this is not a unified diff at all\n",
			kind:      "go",
			wantErr:   nil,
			wantNames: []string{"this is not a unified diff at all"},
		},
		{
			name:      "malformed hunk header",
			diff:      "diff --git a/foo.go b/foo.go\nindex 1..2 100644\n--- a/foo.go\n+++ b/foo.go\n@@ garbage @@\n-old\n+new\n",
			kind:      "go",
			wantErr:   nil,
			wantNames: []string{"@@ garbage @@"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			summary, err := PoliceDiff(tc.diff, tc.kind)
			if err == nil {
				t.Fatalf("PoliceDiff() = %+v, nil, want error", summary)
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("PoliceDiff() error = %v, want errors.Is(_, %v)", err, tc.wantErr)
				}
			} else {
				if errors.Is(err, ErrEmptyDiff) || errors.Is(err, ErrMultiSite) || errors.Is(err, ErrForeignDiff) {
					t.Errorf("PoliceDiff() error = %v, want a plain error (no sentinel)", err)
				}
			}
			for _, name := range tc.wantNames {
				if !strings.Contains(err.Error(), name) {
					t.Errorf("PoliceDiff() error = %q, want it to contain %q", err.Error(), name)
				}
			}
		})
	}
}

func TestPoliceDiffAcceptsRealGitDiffOutput(t *testing.T) {
	diff := "diff --git a/tracked.go b/tracked.go\n" +
		"index 2d71bad..2856e2d 100644\n" +
		"--- a/tracked.go\n" +
		"+++ b/tracked.go\n" +
		"@@ -1,2 +1,2 @@\n" +
		" package main\n" +
		"-func add(a, b int) int { return a + b }\n" +
		"+func add(a, b int) int { return a - b }\n"

	summary, err := PoliceDiff(diff, "go")
	if err != nil {
		t.Fatalf("PoliceDiff: %v", err)
	}
	if summary.File != "tracked.go" || summary.Hunks != 1 {
		t.Errorf("summary = %+v, want File=%q Hunks=1", summary, "tracked.go")
	}
}

func TestPoliceDiffRejectsRealGitNoIndexNewFileOutput(t *testing.T) {
	diff := "diff --git a/new_file.go b/new_file.go\n" +
		"new file mode 100644\n" +
		"index 0000000..b794a18\n" +
		"--- /dev/null\n" +
		"+++ b/new_file.go\n" +
		"@@ -0,0 +1,2 @@\n" +
		"+package main\n" +
		"+func sub(a,b int) int { return a-b }\n"

	_, err := PoliceDiff(diff, "go")
	if !errors.Is(err, ErrForeignDiff) {
		t.Fatalf("PoliceDiff() error = %v, want ErrForeignDiff", err)
	}
	if !strings.Contains(err.Error(), "new_file.go") {
		t.Errorf("PoliceDiff() error = %q, want it to name new_file.go", err.Error())
	}
}

func TestPoliceDiffBothConditionsHoldNamesPath(t *testing.T) {
	path := "src/test/java/com/example/FooTest.java"
	_, err := PoliceDiff(newFileDiff(path), "maven")
	if err == nil {
		t.Fatal("PoliceDiff() = nil, want error")
	}
	if !errors.Is(err, ErrForeignDiff) && !errors.Is(err, ErrMultiSite) {
		t.Errorf("PoliceDiff() error = %v, want ErrForeignDiff or ErrMultiSite", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("PoliceDiff() error = %q, want it to name %q", err.Error(), path)
	}
}
