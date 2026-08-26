package coverage

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildRegistryHarvestsFromCoverageAndSpecDocs(t *testing.T) {
	root := filepath.Join("testdata", "registry-basic")

	reg, err := BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}

	owner, ok := reg.Owner("SC-01")
	if !ok {
		t.Fatalf("Owner(SC-01) not found")
	}
	wantDoc := filepath.Join("docs", "superpowers", "coverage", "2026-01-01-basic-coverage.md")
	if owner.DocPath != wantDoc {
		t.Errorf("Owner(SC-01).DocPath = %q, want %q", owner.DocPath, wantDoc)
	}
	if !owner.Active {
		t.Errorf("Owner(SC-01).Active = false, want true")
	}

	owner, ok = reg.Owner("orders/SC-02")
	if !ok {
		t.Fatalf("Owner(orders/SC-02) not found")
	}
	wantSpecDoc := filepath.Join("docs", "superpowers", "specs", "2026-01-01-orders-design.md")
	if owner.DocPath != wantSpecDoc {
		t.Errorf("Owner(orders/SC-02).DocPath = %q, want %q", owner.DocPath, wantSpecDoc)
	}
	if !owner.Active {
		t.Errorf("Owner(orders/SC-02).Active = false, want true")
	}

	if _, ok := reg.Owner("SC-99"); ok {
		t.Errorf("Owner(SC-99) found, want not found")
	}
}

func TestBuildRegistryRetiredIDIsOwnedButInactive(t *testing.T) {
	root := filepath.Join("testdata", "registry-basic")

	reg, err := BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}

	owner, ok := reg.Owner("SC-04")
	if !ok {
		t.Fatalf("Owner(SC-04) not found, want owned by the retiring document")
	}
	wantDoc := filepath.Join("docs", "superpowers", "coverage", "2026-01-01-basic-coverage.md")
	if owner.DocPath != wantDoc {
		t.Errorf("Owner(SC-04).DocPath = %q, want %q", owner.DocPath, wantDoc)
	}
	if owner.Active {
		t.Errorf("Owner(SC-04).Active = true, want false (retired)")
	}
}

func TestNextIDSkipsRetiredIDInsteadOfReissuing(t *testing.T) {
	root := filepath.Join("testdata", "registry-basic")

	reg, err := BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}

	// Active SC-01, SC-02 plus retired SC-04: the max active number alone is
	// 2, but the retired SC-04 must still occupy 4, so the next free number
	// is 5, not 3.
	got, err := reg.NextID("")
	if err != nil {
		t.Fatalf("NextID(\"\"): %v", err)
	}
	if got != 5 {
		t.Errorf("NextID(\"\") = %d, want 5 (SC-04 retired must not be reissued)", got)
	}
}

func TestNextIDNumbersAreasIndependently(t *testing.T) {
	root := filepath.Join("testdata", "registry-basic")

	reg, err := BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}

	gotUnprefixed, err := reg.NextID("")
	if err != nil {
		t.Fatalf("NextID(\"\"): %v", err)
	}
	gotOrders, err := reg.NextID("orders")
	if err != nil {
		t.Fatalf("NextID(\"orders\"): %v", err)
	}

	if gotUnprefixed != 5 {
		t.Errorf("NextID(\"\") = %d, want 5", gotUnprefixed)
	}
	if gotOrders != 3 {
		t.Errorf("NextID(\"orders\") = %d, want 3 (orders/SC-01, orders/SC-02 present)", gotOrders)
	}
	if gotUnprefixed == gotOrders {
		t.Errorf("NextID(\"\") and NextID(\"orders\") both = %d, want independent numbering", gotUnprefixed)
	}
}

func TestNextIDUnknownAreaYieldsOne(t *testing.T) {
	root := filepath.Join("testdata", "registry-basic")

	reg, err := BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}

	got, err := reg.NextID("billing")
	if err != nil {
		t.Fatalf("NextID(\"billing\"): %v", err)
	}
	if got != 1 {
		t.Errorf("NextID(\"billing\") = %d, want 1", got)
	}
}

func TestNextIDRejectsInvalidArea(t *testing.T) {
	root := filepath.Join("testdata", "registry-basic")

	reg, err := BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}

	if _, err := reg.NextID("orders/"); err == nil {
		t.Errorf("NextID(\"orders/\") returned nil error, want rejection of malformed area")
	}
}

func TestAreasSortedWithUnprefixedFirst(t *testing.T) {
	root := filepath.Join("testdata", "registry-basic")

	reg, err := BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}

	got := reg.Areas()
	want := []string{"", "orders"}
	if len(got) != len(want) {
		t.Fatalf("Areas() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Areas()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAreasIncludesAreaOwnedOnlyByARetiredID(t *testing.T) {
	root := filepath.Join("testdata", "registry-basic")

	reg, err := BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}

	// SC-04 retired, no active row in "" area — area must still show.
	found := false
	for _, a := range reg.Areas() {
		if a == "" {
			found = true
		}
	}
	if !found {
		t.Errorf("Areas() = %v, want unprefixed area present via retired SC-04", reg.Areas())
	}
}

func TestAreasEmptyRegistryYieldsEmptySlice(t *testing.T) {
	root := filepath.Join("testdata", "registry-empty")

	reg, err := BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}

	if got := reg.Areas(); len(got) != 0 {
		t.Errorf("Areas() = %v, want empty", got)
	}
}

func TestBuildRegistryCrossDocumentCollisionNamesBothPaths(t *testing.T) {
	root := filepath.Join("testdata", "registry-cross-collision")

	reg, err := BuildRegistry(root)
	if err == nil {
		t.Fatalf("BuildRegistry returned nil error with %+v, want collision error", reg)
	}
	if reg != nil {
		t.Errorf("BuildRegistry returned %+v alongside error, want nil *Registry", reg)
	}
	wantErrContains(t, err, "SC-01")
	wantErrContains(t, err, "2026-01-01-a-coverage.md")
	wantErrContains(t, err, "2026-01-01-b-coverage.md")
}

func TestBuildRegistryDuplicateIDWithinOneDocumentNamesDocAndID(t *testing.T) {
	root := filepath.Join("testdata", "registry-dup-in-doc")

	reg, err := BuildRegistry(root)
	if err == nil {
		t.Fatalf("BuildRegistry returned nil error with %+v, want duplicate error", reg)
	}
	if reg != nil {
		t.Errorf("BuildRegistry returned %+v alongside error, want nil *Registry", reg)
	}
	wantErrContains(t, err, "SC-01")
	wantErrContains(t, err, "2026-01-01-dup-coverage.md")
}

func TestBuildRegistryUnreadableDocumentErrorsNamingPath(t *testing.T) {
	root := filepath.Join("testdata", "registry-unreadable")

	reg, err := BuildRegistry(root)
	if err == nil {
		t.Fatalf("BuildRegistry returned nil error with %+v, want error", reg)
	}
	if reg != nil {
		t.Errorf("BuildRegistry returned %+v alongside error, want nil *Registry", reg)
	}
	wantErrContains(t, err, "broken-coverage.md")
}

func TestBuildRegistryOldHeaderDocumentHarvestsIDs(t *testing.T) {
	root := filepath.Join("testdata", "registry-old-header")

	reg, err := BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}

	owner, ok := reg.Owner("SC-01")
	if !ok {
		t.Fatalf("Owner(SC-01) not found, want harvested from the old-header table")
	}
	if !owner.Active {
		t.Errorf("Owner(SC-01).Active = false, want true")
	}
}

func TestBuildRegistryMissingCoverageDirContributesNothing(t *testing.T) {
	root := filepath.Join("testdata", "registry-no-coverage-dir")

	reg, err := BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}

	owner, ok := reg.Owner("SC-01")
	if !ok {
		t.Fatalf("Owner(SC-01) not found, want harvested from the spec despite no coverage dir")
	}
	if owner.DocPath != filepath.Join("docs", "superpowers", "specs", "2026-01-01-solo-design.md") {
		t.Errorf("Owner(SC-01).DocPath = %q", owner.DocPath)
	}
}

func TestBuildRegistryMissingSpecsDirContributesNothing(t *testing.T) {
	root := filepath.Join("testdata", "registry-no-specs-dir")

	reg, err := BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}

	owner, ok := reg.Owner("SC-01")
	if !ok {
		t.Fatalf("Owner(SC-01) not found, want harvested from coverage despite no specs dir")
	}
	if owner.DocPath != filepath.Join("docs", "superpowers", "coverage", "2026-01-01-solo-coverage.md") {
		t.Errorf("Owner(SC-01).DocPath = %q", owner.DocPath)
	}
}

func TestBuildRegistrySpecWithNoScenarioTableContributesNothing(t *testing.T) {
	root := filepath.Join("testdata", "registry-spec-no-table")

	reg, err := BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}

	got, err := reg.NextID("")
	if err != nil {
		t.Fatalf("NextID(\"\"): %v", err)
	}
	if got != 1 {
		t.Errorf("NextID(\"\") = %d, want 1 (no scenario table anywhere in this fixture)", got)
	}
}

func TestBuildRegistrySpecWithProseRetiredSectionAndNoTableContributesNothing(t *testing.T) {
	root := filepath.Join("testdata", "registry-prose-retired")

	reg, err := BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v (a prose ## Retired bullet in a document with no scenario table must not be parsed as an id list)", err)
	}

	got, err := reg.NextID("")
	if err != nil {
		t.Fatalf("NextID(\"\"): %v", err)
	}
	if got != 1 {
		t.Errorf("NextID(\"\") = %d, want 1", got)
	}
}

func TestBuildRegistryMalformedRetiredBulletInScenarioDocumentIsAnError(t *testing.T) {
	root := filepath.Join("testdata", "registry-bad-retired")

	_, err := BuildRegistry(root)
	if err == nil {
		t.Fatalf("BuildRegistry returned nil error, want a hard failure: the document carries a scenario table, so its ## Retired section is an id list")
	}
	docPath := filepath.Join("docs", "superpowers", "coverage", "2026-01-01-bad-retired-coverage.md")
	if !strings.Contains(err.Error(), docPath) {
		t.Errorf("BuildRegistry error %q does not name the repo-relative document path %q", err.Error(), docPath)
	}
	if strings.Contains(err.Error(), root+string(filepath.Separator)+docPath) {
		t.Errorf("BuildRegistry error %q names the scan-side path, want the repo-relative form only", err.Error())
	}
}

func TestBuildRegistryDocumentWithNoMatchingTableHarvestsNothing(t *testing.T) {
	root := filepath.Join("testdata", "registry-spec-no-table")

	reg, err := BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}

	if _, ok := reg.Owner("not-an-id"); ok {
		t.Errorf("Owner(not-an-id) found, want the non-matching table's row ignored")
	}
}

func TestBuildRegistryEmptyRepoYieldsEmptyRegistry(t *testing.T) {
	root := filepath.Join("testdata", "registry-empty")

	reg, err := BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}

	if _, ok := reg.Owner("SC-01"); ok {
		t.Errorf("Owner(SC-01) found in empty registry")
	}
	got, err := reg.NextID("")
	if err != nil {
		t.Fatalf("NextID(\"\"): %v", err)
	}
	if got != 1 {
		t.Errorf("NextID(\"\") = %d, want 1", got)
	}
}
