package sales

import (
	"context"

	"github.com/mrchatam/hodhod/internal/db"
)

// salesStore is the DB surface used by Service (satisfied by *db.Store).
type salesStore interface {
	GetAgentPermissions(ctx context.Context, agentID int64) (*db.AgentPermissions, error)
	GetAgentPanel(ctx context.Context, agentID, panelID int64) (*db.AgentPanel, error)
	ListAgentInboundCreateIDs(ctx context.Context, agentID, panelID int64) ([]int, error)
	CountServicesByAgentPanel(ctx context.Context, agentID, panelID int64) (int64, error)
	AgentHasPanel(ctx context.Context, agentID, panelID int64) (bool, error)
	CreateService(ctx context.Context, svc *db.Service) error
	GetPlan(ctx context.Context, botID, planID int64) (*db.Plan, error)
	GetBot(ctx context.Context, botID int64) (*db.Bot, error)
	BotHasPanel(ctx context.Context, botID, panelID int64) (bool, error)
	ListBotPanels(ctx context.Context, botID int64) ([]db.BotPanel, error)
	GetService(ctx context.Context, serviceID int64) (*db.Service, error)
	GetServiceForAgent(ctx context.Context, agentID, serviceID int64) (*db.Service, error)
	UpdateServiceByID(ctx context.Context, svc *db.Service) error
	GetServiceByPanelUsername(ctx context.Context, panelID int64, username string) (*db.Service, error)
	DeleteService(ctx context.Context, serviceID int64) error
}
