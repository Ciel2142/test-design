package runrecord

import "testing"

func TestRecordFields(t *testing.T) {
	r := Record{Status: Failed, Scope: "example.com/pkg", Name: "TestThing"}
	if r.Status != Failed {
		t.Errorf("r.Status = %q, want %q", r.Status, Failed)
	}
	if r.Scope != "example.com/pkg" {
		t.Errorf("r.Scope = %q, want %q", r.Scope, "example.com/pkg")
	}
	if r.Name != "TestThing" {
		t.Errorf("r.Name = %q, want %q", r.Name, "TestThing")
	}
}
