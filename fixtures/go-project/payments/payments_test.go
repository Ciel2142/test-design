package payments

import "testing"

// covers: SC-05
func TestAmountValidation(t *testing.T) {
	t.Run("positive_amount_accepted", func(t *testing.T) {
		if err := ValidateAmount(100); err != nil {
			t.Fatalf("ValidateAmount(100) = %v, want nil", err)
		}
	})
	t.Run("zero_amount_rejected", func(t *testing.T) {
		if err := ValidateAmount(0); err == nil {
			t.Fatalf("ValidateAmount(0) = nil, want error")
		}
	})
}

// covers: SC-06
func TestCurrencyValidation(t *testing.T) {
	t.Run("usd_accepted", func(t *testing.T) {
		if err := ValidateCurrency("USD"); err != nil {
			t.Fatalf("ValidateCurrency(USD) = %v, want nil", err)
		}
	})
	t.Run("jpy_not_yet_supported", func(t *testing.T) {
		t.Skip("JPY support not yet implemented")
	})
}

// covers: SC-07, SC-01, SC-05
func TestOverclaimedCoverage(t *testing.T) {
	if err := ValidateCurrency("USD"); err != nil {
		t.Fatalf("ValidateCurrency(USD) = %v, want nil", err)
	}
}

// covers: SC-08
func TestAmbiguousBindingTwo(t *testing.T) {
	if err := ValidateAmount(300); err != nil {
		t.Fatalf("ValidateAmount(300) = %v, want nil", err)
	}
}

// covers: SC-77
func TestUnknownScenarioCited(t *testing.T) {
	if err := ValidateCurrency("GBP"); err != nil {
		t.Fatalf("ValidateCurrency(GBP) = %v, want nil", err)
	}
}

func TestSameName(t *testing.T) {
	if err := ValidateAmount(50); err != nil {
		t.Fatalf("ValidateAmount(50) = %v, want nil", err)
	}
}
