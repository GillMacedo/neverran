// Package cart adds up an order.
package cart

// Item is one line of an order, with its price in cents.
type Item struct {
	SKU      string
	Cents    int
	Quantity int
}

// Total returns the order total in cents, after a percentage discount.
func Total(items []Item, discountPercent int) int {
	sum := 0
	for _, it := range items {
		sum += it.Cents * it.Quantity
	}
	return sum - sum*discountPercent/100
}
