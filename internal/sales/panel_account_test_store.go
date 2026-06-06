package sales

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mrchatam/hodhod/internal/db"
)

type panelTestStore struct {
	services       map[string]*db.Service
	nextServiceID  int64
	agentPanel     *db.AgentPanel
	agentPerms     *db.AgentPermissions
	serviceCount   int64
	quotaFailBytes int64
	plan           *db.Plan
	bot            *db.Bot
	botPanels      []db.BotPanel
	updated        bool
}

func (m *panelTestStore) svcKey(panelID int64, username string) string {
	return fmt.Sprintf("%d:%s", panelID, username)
}

func (m *panelTestStore) GetAgentPermissions(context.Context, int64) (*db.AgentPermissions, error) {
	if m.agentPerms == nil {
		return &db.AgentPermissions{CreateUser: true, ModifyUser: true}, nil
	}
	return m.agentPerms, nil
}

func (m *panelTestStore) GetAgentPanel(context.Context, int64, int64) (*db.AgentPanel, error) {
	if m.agentPanel == nil {
		return &db.AgentPanel{}, nil
	}
	return m.agentPanel, nil
}

func (m *panelTestStore) ListAgentInboundCreateIDs(context.Context, int64, int64) ([]int, error) {
	return []int{1}, nil
}

func (m *panelTestStore) CountServicesByAgentPanel(context.Context, int64, int64) (int64, error) {
	return m.serviceCount, nil
}

func (m *panelTestStore) AgentHasPanel(context.Context, int64, int64) (bool, error) {
	return true, nil
}

func (m *panelTestStore) CreateService(_ context.Context, svc *db.Service) error {
	if m.services == nil {
		m.services = map[string]*db.Service{}
	}
	m.nextServiceID++
	svc.ID = m.nextServiceID
	cp := *svc
	m.services[m.svcKey(svc.PanelID, svc.PanelUsername)] = &cp
	return nil
}

func (m *panelTestStore) GetPlan(context.Context, int64, int64) (*db.Plan, error) {
	if m.plan == nil {
		return nil, errors.New("no plan")
	}
	return m.plan, nil
}

func (m *panelTestStore) GetBot(context.Context, int64) (*db.Bot, error) {
	if m.bot == nil {
		return nil, errors.New("no bot")
	}
	return m.bot, nil
}

func (m *panelTestStore) BotHasPanel(context.Context, int64, int64) (bool, error) {
	return true, nil
}

func (m *panelTestStore) ListBotPanels(context.Context, int64) ([]db.BotPanel, error) {
	return m.botPanels, nil
}

func (m *panelTestStore) GetService(_ context.Context, serviceID int64) (*db.Service, error) {
	for _, svc := range m.services {
		if svc.ID == serviceID {
			cp := *svc
			return &cp, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *panelTestStore) GetServiceForAgent(ctx context.Context, _ int64, serviceID int64) (*db.Service, error) {
	return m.GetService(ctx, serviceID)
}

func (m *panelTestStore) UpdateServiceByID(_ context.Context, svc *db.Service) error {
	m.updated = true
	if m.services == nil {
		return errors.New("no services")
	}
	key := m.svcKey(svc.PanelID, svc.PanelUsername)
	if _, ok := m.services[key]; !ok {
		return errors.New("not found")
	}
	cp := *svc
	m.services[key] = &cp
	return nil
}

func (m *panelTestStore) GetServiceByPanelUsername(_ context.Context, panelID int64, username string) (*db.Service, error) {
	if m.services == nil {
		return nil, errors.New("not found")
	}
	svc, ok := m.services[m.svcKey(panelID, username)]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *svc
	return &cp, nil
}

func (m *panelTestStore) DeleteService(context.Context, int64) error { return nil }

func scopeJSON(ids []int) []byte {
	b, _ := json.Marshal(map[string]any{"inbound_ids": ids})
	return b
}
