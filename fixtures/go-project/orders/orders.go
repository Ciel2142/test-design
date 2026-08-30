package orders

import "fmt"

func ValidateQuantity(qty int) error {
	if qty < 0 {
		return fmt.Errorf("quantity must be greater than zero")
	}
	return nil
}
