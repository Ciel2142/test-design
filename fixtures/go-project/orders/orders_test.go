package orders

import "testing"

// covers: SC-01
func TestValidateQuantityAccepts(t *testing.T) {
	if err := ValidateQuantity(5); err != nil {
		t.Fatalf("ValidateQuantity(5) = %v, want nil", err)
	}
}

// covers: SC-02
func TestValidateQuantityRejectsZero(t *testing.T) {
	if err := ValidateQuantity(0); err == nil {
		t.Fatalf("ValidateQuantity(0) = nil, want error")
	}
}

// covers: SC-03
func TestValidateQuantityFutureRule(t *testing.T) {
	t.Skip("backorder quantity policy not yet decided")
}

// covers: SC-09
func TestSameName(t *testing.T) {
	if err := ValidateQuantity(1); err != nil {
		t.Fatalf("ValidateQuantity(1) = %v, want nil", err)
	}
}

// covers: SC-08
func TestAmbiguousBindingOne(t *testing.T) {
	if err := ValidateQuantity(3); err != nil {
		t.Fatalf("ValidateQuantity(3) = %v, want nil", err)
	}
}

// covers: SC-90
func TestRetiredScenarioStillCited(t *testing.T) {
	if err := ValidateQuantity(2); err != nil {
		t.Fatalf("ValidateQuantity(2) = %v, want nil", err)
	}
}

// covers: SC-04

func TestOrdersGapPlaceholder(t *testing.T) {
	if err := ValidateQuantity(7); err != nil {
		t.Fatalf("ValidateQuantity(7) = %v, want nil", err)
	}
}

// covers: SC-10
func TestReserveGuard(t *testing.T) {
	if err := Reserve(5); err != nil {
		t.Fatalf("Reserve(5) = %v, want nil", err)
	}
	if err := Reserve(0); err == nil {
		t.Fatal("Reserve(0) = nil, want error")
	}
}
