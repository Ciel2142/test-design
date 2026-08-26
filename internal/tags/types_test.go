package tags

import (
	"encoding/json"
	"testing"
)

func TestClassZeroValueIsUnknownSoAnUnassignedClassNeverBinds(t *testing.T) {
	var c Class

	if c != ClassUnknown {
		t.Errorf("zero Class = %v, want ClassUnknown: an unassigned class must not bind a tag to this run's rows", c)
	}
	if (Classification{}).Class != ClassUnknown {
		t.Errorf("Classification{}.Class = %v, want ClassUnknown", Classification{}.Class)
	}
}

func TestClassStringUsesTheSpecVocabulary(t *testing.T) {
	tests := []struct {
		class Class
		want  string
	}{
		{ClassActive, "active"},
		{ClassRetired, "retired"},
		{ClassForeign, "foreign"},
		{ClassUnknown, "unknown"},
	}
	for _, tt := range tests {
		if got := tt.class.String(); got != tt.want {
			t.Errorf("Class(%d).String() = %q, want %q", int(tt.class), got, tt.want)
		}
	}
}

func TestClassStringOutsideTheDefinedSetIsUnknown(t *testing.T) {
	if got := Class(99).String(); got != "unknown" {
		t.Errorf("Class(99).String() = %q, want %q", got, "unknown")
	}
}

func TestClassMarshalJSONEmitsTheVocabularyStringNotAnInteger(t *testing.T) {
	data, err := json.Marshal(Classification{ID: "SC-01", Class: ClassRetired, OwnerDoc: "docs/x.md"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"ID":"SC-01","Class":"retired","OwnerDoc":"docs/x.md"}`
	if string(data) != want {
		t.Errorf("Marshal = %s, want %s", data, want)
	}
}

func TestClassMarshalJSONOfAnUnassignedClassIsUnknown(t *testing.T) {
	data, err := json.Marshal(Class(0))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `"unknown"` {
		t.Errorf("Marshal(Class(0)) = %s, want %q", data, "unknown")
	}
}
