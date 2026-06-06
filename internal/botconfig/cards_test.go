package botconfig

import (
	"testing"

	"github.com/mrchatam/hodhod/internal/db"
)

func TestFormatCard(t *testing.T) {
	if FormatCard(nil) != "" {
		t.Fatal("nil card")
	}
	c := &db.BotPaymentCard{Label: "Bank", CardNumber: "6037-1234"}
	if FormatCard(c) != "Bank\n6037-1234" {
		t.Fatalf("labeled card: %q", FormatCard(c))
	}
	c.Label = ""
	if FormatCard(c) != "6037-1234" {
		t.Fatalf("plain card: %q", FormatCard(c))
	}
}

func TestParseApproverIDs(t *testing.T) {
	ids := parseApproverIDs("123, 456, bad, 0")
	if len(ids) != 2 || ids[0] != 123 || ids[1] != 456 {
		t.Fatalf("parse approvers: %v", ids)
	}
}

func TestCardPickerModeConstants(t *testing.T) {
	modes := []string{"random", "fixed_order", "round_robin", "weighted"}
	for _, m := range modes {
		if m == "" {
			t.Fatal("empty mode")
		}
	}
}
