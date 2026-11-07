package domain

import "math"

// Money is an integer number of rupiah. Negative money is never a price/amount;
// ledger direction represents inflow/outflow. Arithmetic checks int64 overflow.
type Money int64

func (m Money) Validate(field string) error {
	if m < 0 {
		return invalid(field, "Nilai rupiah tidak boleh negatif.")
	}
	return nil
}

func moneyOverflow() error {
	return &Error{Kind: Validation, Code: "money_overflow", Field: "amount", Message: "Nilai rupiah melebihi batas perhitungan."}
}

func addMoney(a, b Money) (Money, error) {
	if err := a.Validate("amount"); err != nil {
		return 0, err
	}
	if err := b.Validate("amount"); err != nil {
		return 0, err
	}
	if a > Money(math.MaxInt64)-b {
		return 0, moneyOverflow()
	}
	return a + b, nil
}

func multiplyMoney(price Money, quantity int64) (Money, error) {
	if err := price.Validate("unit_price"); err != nil {
		return 0, err
	}
	if quantity < 0 {
		return 0, invalid("quantity", "Jumlah tidak boleh negatif.")
	}
	if quantity != 0 && price > Money(math.MaxInt64/quantity) {
		return 0, moneyOverflow()
	}
	return price * Money(quantity), nil
}

// weightTotal rounds each line to the nearest rupiah, half up. Decomposition
// avoids intermediate price*grams overflow when the final amount still fits.
func weightTotal(price Money, grams int64) (Money, error) {
	whole, err := multiplyMoney(price, grams/1000)
	if err != nil {
		return 0, err
	}
	rem := grams % 1000
	fraction, err := multiplyMoney(price/1000, rem)
	if err != nil {
		return 0, err
	}
	rounded := Money((int64(price%1000)*rem + 500) / 1000)
	result, err := addMoney(whole, fraction)
	if err != nil {
		return 0, err
	}
	return addMoney(result, rounded)
}
