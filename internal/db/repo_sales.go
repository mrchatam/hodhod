package db

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// --- Agent permissions ---

func (s *Store) GetAgentPermissions(ctx context.Context, agentID int64) (*AgentPermissions, error) {
	var p AgentPermissions
	err := s.DB.WithContext(ctx).First(&p, "agent_id = ?", agentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &AgentPermissions{AgentID: agentID, ViewOnly: true}, nil
	}
	return &p, err
}

func (s *Store) UpsertAgentPermissions(ctx context.Context, p *AgentPermissions) error {
	return s.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "agent_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"create_user", "modify_user", "add_time", "add_volume", "reset_usage",
			"disable_enable", "delete_user", "manage_bot", "manage_plans", "view_only",
		}),
	}).Create(p).Error
}

// --- Agent panels ---

func (s *Store) ListAgentPanels(ctx context.Context, agentID int64) ([]AgentPanel, error) {
	var out []AgentPanel
	return out, s.DB.WithContext(ctx).Where("agent_id = ?", agentID).Find(&out).Error
}

func (s *Store) ListPanelsForAgent(ctx context.Context, agentID int64) ([]Panel, error) {
	var out []Panel
	err := s.DB.WithContext(ctx).
		Model(&Panel{}).
		Joins("JOIN agent_panels ON agent_panels.panel_id = panels.id").
		Where("agent_panels.agent_id = ?", agentID).
		Order("panels.id").
		Find(&out).Error
	return out, err
}

func (s *Store) GetAgentPanel(ctx context.Context, agentID, panelID int64) (*AgentPanel, error) {
	var ap AgentPanel
	err := s.DB.WithContext(ctx).Where("agent_id = ? AND panel_id = ?", agentID, panelID).First(&ap).Error
	return &ap, err
}

func (s *Store) AgentHasPanel(ctx context.Context, agentID, panelID int64) (bool, error) {
	var n int64
	err := s.DB.WithContext(ctx).Model(&AgentPanel{}).Where("agent_id = ? AND panel_id = ?", agentID, panelID).Count(&n).Error
	return n > 0, err
}

func (s *Store) UpsertAgentPanel(ctx context.Context, ap *AgentPanel) error {
	return s.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "agent_id"}, {Name: "panel_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"scope_json", "quota_bytes", "max_users", "expiry_cap_days"}),
	}).Create(ap).Error
}

func (s *Store) DeleteAgentPanel(ctx context.Context, agentID, panelID int64) error {
	return s.DB.WithContext(ctx).Where("agent_id = ? AND panel_id = ?", agentID, panelID).Delete(&AgentPanel{}).Error
}

func (s *Store) CountServicesByAgentPanel(ctx context.Context, agentID, panelID int64) (int64, error) {
	var n int64
	err := s.DB.WithContext(ctx).Model(&Service{}).
		Where("agent_id = ? AND panel_id = ? AND status = ?", agentID, panelID, "active").
		Count(&n).Error
	return n, err
}

// --- Customers ---

func (s *Store) CreateCustomer(ctx context.Context, c *Customer) error {
	return s.DB.WithContext(ctx).Create(c).Error
}

func (s *Store) ListCustomersByAgent(ctx context.Context, agentID int64) ([]Customer, error) {
	var out []Customer
	return out, s.DB.WithContext(ctx).Where("agent_id = ?", agentID).Order("id DESC").Find(&out).Error
}

func (s *Store) GetCustomerForAgent(ctx context.Context, agentID, id int64) (*Customer, error) {
	var c Customer
	err := s.DB.WithContext(ctx).Where("id = ? AND agent_id = ?", id, agentID).First(&c).Error
	return &c, err
}

// --- Services (agent-scoped) ---

func (s *Store) GetService(ctx context.Context, id int64) (*Service, error) {
	var svc Service
	err := s.DB.WithContext(ctx).First(&svc, id).Error
	return &svc, err
}

func (s *Store) GetServiceForAgent(ctx context.Context, agentID, id int64) (*Service, error) {
	var svc Service
	err := s.DB.WithContext(ctx).Where("id = ? AND agent_id = ?", id, agentID).First(&svc).Error
	return &svc, err
}

func (s *Store) ListServicesByAgent(ctx context.Context, agentID int64) ([]Service, error) {
	return s.ListServicesByAgentFiltered(ctx, agentID, "", "")
}

func (s *Store) ListServicesByAgentFiltered(ctx context.Context, agentID int64, q, status string) ([]Service, error) {
	return s.ListServicesByAgentFilteredPaginated(ctx, agentID, q, status, 0, 0)
}

func (s *Store) ListAllServices(ctx context.Context) ([]Service, error) {
	return s.ListAllServicesFiltered(ctx, "", "")
}

func (s *Store) ListAllServicesFiltered(ctx context.Context, q, status string) ([]Service, error) {
	return s.ListAllServicesFilteredPaginated(ctx, q, status, 0, 0)
}

func (s *Store) CountAllServicesFiltered(ctx context.Context, q, status string, panelID int64) (int64, error) {
	var n int64
	query := s.DB.WithContext(ctx).Model(&Service{})
	query = applyServiceFilters(query, q, status)
	if panelID > 0 {
		query = query.Where("panel_id = ?", panelID)
	}
	return n, query.Count(&n).Error
}

func (s *Store) CountServicesByAgentFiltered(ctx context.Context, agentID int64, q, status string, panelID int64) (int64, error) {
	var n int64
	query := s.DB.WithContext(ctx).Model(&Service{}).Where("agent_id = ?", agentID)
	query = applyServiceFilters(query, q, status)
	if panelID > 0 {
		query = query.Where("panel_id = ?", panelID)
	}
	return n, query.Count(&n).Error
}

func (s *Store) ListAllServicesFilteredPaginated(ctx context.Context, q, status string, limit, offset int) ([]Service, error) {
	var out []Service
	query := s.DB.WithContext(ctx).Model(&Service{})
	query = applyServiceFilters(query, q, status)
	query = query.Order("id DESC")
	if limit > 0 {
		query = query.Limit(limit).Offset(offset)
	}
	return out, query.Find(&out).Error
}

func (s *Store) ListServicesByAgentFilteredPaginated(ctx context.Context, agentID int64, q, status string, limit, offset int) ([]Service, error) {
	var out []Service
	query := s.DB.WithContext(ctx).Where("agent_id = ?", agentID)
	query = applyServiceFilters(query, q, status)
	query = query.Order("id DESC")
	if limit > 0 {
		query = query.Limit(limit).Offset(offset)
	}
	return out, query.Find(&out).Error
}

func (s *Store) ListAllServicesFilteredForPanel(ctx context.Context, q, status string, panelID int64, limit, offset int) ([]Service, error) {
	var out []Service
	query := s.DB.WithContext(ctx).Model(&Service{})
	query = applyServiceFilters(query, q, status)
	if panelID > 0 {
		query = query.Where("panel_id = ?", panelID)
	}
	query = query.Order("id DESC")
	if limit > 0 {
		query = query.Limit(limit).Offset(offset)
	}
	return out, query.Find(&out).Error
}

func (s *Store) ListServicesByAgentFilteredForPanel(ctx context.Context, agentID int64, q, status string, panelID int64, limit, offset int) ([]Service, error) {
	var out []Service
	query := s.DB.WithContext(ctx).Where("agent_id = ?", agentID)
	query = applyServiceFilters(query, q, status)
	if panelID > 0 {
		query = query.Where("panel_id = ?", panelID)
	}
	query = query.Order("id DESC")
	if limit > 0 {
		query = query.Limit(limit).Offset(offset)
	}
	return out, query.Find(&out).Error
}

func applyServiceFilters(q *gorm.DB, search, status string) *gorm.DB {
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if search != "" {
		like := "%" + search + "%"
		q = q.Where("label ILIKE ? OR panel_username ILIKE ?", like, like)
	}
	return q
}

func (s *Store) UpdateServiceByID(ctx context.Context, svc *Service) error {
	return s.DB.WithContext(ctx).Save(svc).Error
}

func (s *Store) DeleteService(ctx context.Context, id int64) error {
	return s.DB.WithContext(ctx).Delete(&Service{}, id).Error
}

func (s *Store) DeletePanel(ctx context.Context, id int64) error {
	return s.DB.WithContext(ctx).Delete(&Panel{}, id).Error
}

func (s *Store) ListAdminsByAgent(ctx context.Context, agentID int64) ([]Admin, error) {
	var out []Admin
	return out, s.DB.WithContext(ctx).Where("agent_id = ?", agentID).Find(&out).Error
}

func (s *Store) UpdateAdmin(ctx context.Context, a *Admin) error {
	return s.DB.WithContext(ctx).Save(a).Error
}

func (s *Store) CountServicesByAgent(ctx context.Context, agentID int64) (int64, error) {
	var n int64
	err := s.DB.WithContext(ctx).Model(&Service{}).Where("agent_id = ? AND status = ?", agentID, "active").Count(&n).Error
	return n, err
}

func (s *Store) SumApprovedPaymentsByAgent(ctx context.Context, agentID int64) (int64, error) {
	var total int64
	err := s.DB.WithContext(ctx).Model(&Payment{}).
		Joins("JOIN bots ON bots.id = payments.bot_id").
		Where("bots.agent_id = ? AND payments.status = ?", agentID, PaymentApproved).
		Select("COALESCE(SUM(payments.amount_toman), 0)").
		Scan(&total).Error
	return total, err
}

func (s *Store) ListPaymentsByAgent(ctx context.Context, agentID int64, status PaymentStatus) ([]Payment, error) {
	return s.ListPaymentsByAgentPaginated(ctx, agentID, status, 0, 0)
}

func (s *Store) ListAllPayments(ctx context.Context, status PaymentStatus) ([]Payment, error) {
	return s.ListAllPaymentsPaginated(ctx, status, 0, 0)
}

func (s *Store) CountPaymentsByAgent(ctx context.Context, agentID int64, status PaymentStatus) (int64, error) {
	var n int64
	q := s.DB.WithContext(ctx).Model(&Payment{}).
		Joins("JOIN bots ON bots.id = payments.bot_id").
		Where("bots.agent_id = ?", agentID)
	if status != "" {
		q = q.Where("payments.status = ?", status)
	}
	return n, q.Count(&n).Error
}

func (s *Store) CountAllPayments(ctx context.Context, status PaymentStatus) (int64, error) {
	var n int64
	q := s.DB.WithContext(ctx).Model(&Payment{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	return n, q.Count(&n).Error
}

func (s *Store) ListPaymentsByAgentPaginated(ctx context.Context, agentID int64, status PaymentStatus, limit, offset int) ([]Payment, error) {
	var out []Payment
	q := s.DB.WithContext(ctx).
		Joins("JOIN bots ON bots.id = payments.bot_id").
		Where("bots.agent_id = ?", agentID)
	if status != "" {
		q = q.Where("payments.status = ?", status)
	}
	q = q.Order("payments.id DESC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	return out, q.Find(&out).Error
}

func (s *Store) ListAllPaymentsPaginated(ctx context.Context, status PaymentStatus, limit, offset int) ([]Payment, error) {
	var out []Payment
	q := s.DB.WithContext(ctx).Model(&Payment{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	q = q.Order("id DESC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	return out, q.Find(&out).Error
}

func (s *Store) CountBots(ctx context.Context) (int64, error) {
	var n int64
	err := s.DB.WithContext(ctx).Model(&Bot{}).Count(&n).Error
	return n, err
}

func (s *Store) CountAgents(ctx context.Context) (int64, error) {
	var n int64
	err := s.DB.WithContext(ctx).Model(&Agent{}).Where("status = ?", AgentActive).Count(&n).Error
	return n, err
}
