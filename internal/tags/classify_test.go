package tags

import (
	"path/filepath"
	"testing"

	"test-design/internal/coverage"
)

func buildClassifyTestRegistry(t *testing.T) (*coverage.Registry, string) {
	t.Helper()
	root := filepath.Join("testdata", "classify-basic")
	reg, err := coverage.BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	docPath := filepath.Join("docs", "superpowers", "coverage", "2026-01-01-basic-coverage.md")
	return reg, docPath
}

func TestClassifyActiveOwnedByThisDocAndActive(t *testing.T) {
	reg, thisDoc := buildClassifyTestRegistry(t)

	got := Classify("SC-01", thisDoc, reg)

	want := Classification{ID: "SC-01", Class: ClassActive, OwnerDoc: thisDoc}
	if got != want {
		t.Errorf("Classify = %+v, want %+v", got, want)
	}
}

func TestClassifyRetiredOwnedByThisDocButNotActive(t *testing.T) {
	reg, thisDoc := buildClassifyTestRegistry(t)

	got := Classify("SC-04", thisDoc, reg)

	want := Classification{ID: "SC-04", Class: ClassRetired, OwnerDoc: thisDoc}
	if got != want {
		t.Errorf("Classify = %+v, want %+v", got, want)
	}
}

func TestClassifyForeignOwnedByAnotherDoc(t *testing.T) {
	reg, ownerDoc := buildClassifyTestRegistry(t)
	thisDoc := filepath.Join("docs", "superpowers", "coverage", "2026-01-01-other-coverage.md")

	got := Classify("SC-01", thisDoc, reg)

	want := Classification{ID: "SC-01", Class: ClassForeign, OwnerDoc: ownerDoc}
	if got != want {
		t.Errorf("Classify = %+v, want %+v", got, want)
	}
}

func TestClassifyForeignRetiredIDStaysForeignNotRetired(t *testing.T) {
	// R10: ownership decides before retirement. SC-04 is retired in its
	// owning document, but a different thisDoc must see it as foreign, not
	// retired — the owning document's retirement is not this run's finding.
	reg, ownerDoc := buildClassifyTestRegistry(t)
	thisDoc := filepath.Join("docs", "superpowers", "coverage", "2026-01-01-other-coverage.md")

	got := Classify("SC-04", thisDoc, reg)

	want := Classification{ID: "SC-04", Class: ClassForeign, OwnerDoc: ownerDoc}
	if got != want {
		t.Errorf("Classify = %+v, want %+v", got, want)
	}
}

func TestClassifyUnknownIDOwnedByNoDocument(t *testing.T) {
	reg, thisDoc := buildClassifyTestRegistry(t)

	got := Classify("SC-99", thisDoc, reg)

	want := Classification{ID: "SC-99", Class: ClassUnknown}
	if got != want {
		t.Errorf("Classify = %+v, want %+v", got, want)
	}
}

func TestClassifyMalformedTokenIsUnknown(t *testing.T) {
	reg, thisDoc := buildClassifyTestRegistry(t)

	got := Classify("not-an-id", thisDoc, reg)

	want := Classification{ID: "not-an-id", Class: ClassUnknown}
	if got != want {
		t.Errorf("Classify = %+v, want %+v", got, want)
	}
}
