package orders

import "fmt"

func ValidateQuantity(qty int) error {
	if qty < 0 {
		return fmt.Errorf("quantity must be greater than zero")
	}
	return nil
}

func Reserve(qty int) error {
	if qty <= 0 {
		return fmt.Errorf("reservation quantity must be positive")
	}
	return nil
}

func ReserveBatch(qtys []int) error {
	for _, qty := range qtys {
		if qty <= 0 {
			return fmt.Errorf("reservation quantity must be positive")
		}
	}
	return nil
}
