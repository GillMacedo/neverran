package payments

import (
	"os"
	"testing"
)

func TestRefund(t *testing.T) {
	if _, err := Refund(1000, 1500); err == nil {
		t.Fatal("expected an error for a refund larger than the payment")
	}
}

func TestRefundAgainstSandbox(t *testing.T) {
	if os.Getenv("SHOP_PAYMENTS_SANDBOX") == "" {
		t.Skip("SHOP_PAYMENTS_SANDBOX not set")
	}
	got, err := Refund(1000, 400)
	if err != nil || got != 400 {
		t.Fatalf("Refund = %d, %v", got, err)
	}
}
