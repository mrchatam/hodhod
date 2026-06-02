package billing

import (
	"testing"

	"github.com/mrchatam/hodhod/internal/db"
)

func TestValidatePlanForAgent(t *testing.T) {
	agent := &db.Agent{PriceFloorToman: 1000, PriceCeilingToman: 50000}
	plan := &db.Plan{PriceToman: 10000, DurationDays: 30, VolumeGB: 10}
	if err := ValidatePlanForAgent(plan, agent); err != nil {
		t.Fatal(err)
	}
	plan.PriceToman = 500
	if err := ValidatePlanForAgent(plan, agent); err == nil {
		t.Fatal("expected price floor error")
	}
}
