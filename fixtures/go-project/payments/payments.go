package payments

import "fmt"

func ValidateAmount(cents int) error {
	if cents < 0 {
		return fmt.Errorf("amount must be greater than zero")
	}
	return nil
}

func ValidateCurrency(code string) error {
	switch code {
	case "USD", "EUR", "GBP":
		return nil
	default:
		return fmt.Errorf("unsupported currency %q", code)
	}
}
