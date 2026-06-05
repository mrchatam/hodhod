package provisioning

import (
	"context"

	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/sales"
)

// Service provisions VPN accounts via the shared sales layer.
type Service struct {
	Store *db.Store
	Sales *sales.Service
}

// ProvisionOrder creates a panel user for an approved order.
func (s *Service) ProvisionOrder(ctx context.Context, botID int64, order *db.Order) (*db.Service, error) {
	return s.Sales.ProvisionFromOrder(ctx, botID, order)
}
