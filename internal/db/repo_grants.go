package db

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Store) ListAgentInboundGrants(ctx context.Context, agentID, panelID int64) ([]AgentInboundGrant, error) {
	var out []AgentInboundGrant
	q := s.DB.WithContext(ctx).Where("agent_id = ?", agentID)
	if panelID > 0 {
		q = q.Where("panel_id = ?", panelID)
	}
	return out, q.Order("panel_id, inbound_id").Find(&out).Error
}

func (s *Store) ListAgentInboundCreateIDs(ctx context.Context, agentID, panelID int64) ([]int, error) {
	var grants []AgentInboundGrant
	err := s.DB.WithContext(ctx).
		Where("agent_id = ? AND panel_id = ? AND allow_create = ?", agentID, panelID, true).
		Order("inbound_id").
		Find(&grants).Error
	if err != nil {
		return nil, err
	}
	ids := make([]int, len(grants))
	for i, g := range grants {
		ids[i] = g.InboundID
	}
	return ids, nil
}

func (s *Store) ListAgentInboundViewIDs(ctx context.Context, agentID, panelID int64) ([]int, error) {
	var grants []AgentInboundGrant
	err := s.DB.WithContext(ctx).
		Where("agent_id = ? AND panel_id = ? AND allow_view_users = ?", agentID, panelID, true).
		Order("inbound_id").
		Find(&grants).Error
	if err != nil {
		return nil, err
	}
	ids := make([]int, len(grants))
	for i, g := range grants {
		ids[i] = g.InboundID
	}
	return ids, nil
}

func (s *Store) AgentHasAnyCreateInbound(ctx context.Context, agentID int64) (bool, error) {
	var n int64
	err := s.DB.WithContext(ctx).Model(&AgentInboundGrant{}).
		Where("agent_id = ? AND allow_create = ?", agentID, true).
		Count(&n).Error
	return n > 0, err
}

func (s *Store) ReplaceAgentInboundGrants(ctx context.Context, agentID, panelID int64, grants []AgentInboundGrant) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("agent_id = ? AND panel_id = ?", agentID, panelID).Delete(&AgentInboundGrant{}).Error; err != nil {
			return err
		}
		if len(grants) == 0 {
			return nil
		}
		for i := range grants {
			grants[i].AgentID = agentID
			grants[i].PanelID = panelID
		}
		return tx.Create(&grants).Error
	})
}

func (s *Store) ListAgentUserGrants(ctx context.Context, agentID, panelID int64) ([]AgentUserGrant, error) {
	var out []AgentUserGrant
	q := s.DB.WithContext(ctx).Where("agent_id = ?", agentID)
	if panelID > 0 {
		q = q.Where("panel_id = ?", panelID)
	}
	return out, q.Order("panel_id, panel_username").Find(&out).Error
}

func (s *Store) ListAgentUserGrantsByAgent(ctx context.Context, agentID int64) ([]AgentUserGrant, error) {
	return s.ListAgentUserGrants(ctx, agentID, 0)
}

func (s *Store) GetAgentUserGrant(ctx context.Context, agentID, panelID int64, username string) (*AgentUserGrant, error) {
	var g AgentUserGrant
	err := s.DB.WithContext(ctx).
		Where("agent_id = ? AND panel_id = ? AND panel_username = ?", agentID, panelID, username).
		First(&g).Error
	return &g, err
}

func (s *Store) UpsertAgentUserGrant(ctx context.Context, g *AgentUserGrant) error {
	return s.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "agent_id"}, {Name: "panel_id"}, {Name: "panel_username"}},
		DoUpdates: clause.AssignmentColumns([]string{"allow_view", "allow_modify"}),
	}).Create(g).Error
}

func (s *Store) ReplaceAgentUserGrants(ctx context.Context, agentID, panelID int64, grants []AgentUserGrant) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("agent_id = ? AND panel_id = ?", agentID, panelID).Delete(&AgentUserGrant{}).Error; err != nil {
			return err
		}
		if len(grants) == 0 {
			return nil
		}
		for i := range grants {
			grants[i].AgentID = agentID
			grants[i].PanelID = panelID
		}
		return tx.Create(&grants).Error
	})
}

// SaveAgentUserGrants merges access-form grants with existing rows.
// Users listed in tableUsernames get view/modify from formGrants (missing = revoked).
// Existing grants for usernames not in tableUsernames are preserved (bulk attach survives inbound-only saves).
func (s *Store) SaveAgentUserGrants(ctx context.Context, agentID, panelID int64, tableUsernames []string, formGrants []AgentUserGrant) error {
	existing, err := s.ListAgentUserGrants(ctx, agentID, panelID)
	if err != nil {
		return err
	}
	tableSet := map[string]bool{}
	for _, u := range tableUsernames {
		if u != "" {
			tableSet[u] = true
		}
	}
	formByUser := map[string]AgentUserGrant{}
	for _, g := range formGrants {
		formByUser[g.PanelUsername] = g
	}
	merged := map[string]AgentUserGrant{}
	for _, g := range existing {
		if !tableSet[g.PanelUsername] {
			merged[g.PanelUsername] = g
		}
	}
	for username := range tableSet {
		g, ok := formByUser[username]
		if !ok || (!g.AllowView && !g.AllowModify) {
			continue
		}
		g.AgentID = agentID
		g.PanelID = panelID
		g.PanelUsername = username
		merged[username] = g
	}
	var out []AgentUserGrant
	for _, g := range merged {
		out = append(out, g)
	}
	return s.ReplaceAgentUserGrants(ctx, agentID, panelID, out)
}
