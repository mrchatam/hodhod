package provisioning

import (
	"testing"

	"github.com/mrchatam/hodhod/internal/sales"
)

func TestService_requiresSalesLayer(t *testing.T) {
	s := &Service{Sales: &sales.Service{}}
	if s.Sales == nil {
		t.Fatal("provisioning must delegate to sales")
	}
}
