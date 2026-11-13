// Package payments computes refunds.
package payments

import "errors"

// Refund returns how many cents to refund, or an error when the request is
// more than what was paid.
func Refund(paidCents, requestedCents int) (int, error) {
	if requestedCents > paidCents {
		return 0, errors.New("refund is more than the payment")
	}
	return requestedCents, nil
}
