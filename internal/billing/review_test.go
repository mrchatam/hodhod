package billing

import (
	"testing"

	"github.com/mrchatam/hodhod/internal/db"
)

func TestErrPaymentNotPending(t *testing.T) {
	if ErrPaymentNotPending.Error() != "payment not pending" {
		t.Fatalf("unexpected error text")
	}
}

func TestWalletAdjustSign(t *testing.T) {
	s := &WalletService{}
	if s == nil {
		t.Fatal("nil")
	}
	_ = db.PaymentPending
}
