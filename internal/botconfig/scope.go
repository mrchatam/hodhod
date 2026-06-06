package botconfig

import (
	"context"
	"errors"

	"github.com/mrchatam/hodhod/internal/db"
)

var ErrInboundNotGranted = errors.New("botconfig: inbound not granted to agent")

// ValidatePanelScope ensures requested inbound IDs are within agent create grants.
func ValidatePanelScope(ctx context.Context, store *db.Store, agentID, panelID int64, inboundIDs []int) error {
	if len(inboundIDs) == 0 {
		return nil
	}
	granted, err := store.ListAgentInboundCreateIDs(ctx, agentID, panelID)
	if err != nil {
		return err
	}
	if len(granted) == 0 {
		return nil
	}
	grantSet := map[int]bool{}
	for _, id := range granted {
		grantSet[id] = true
	}
	for _, id := range inboundIDs {
		if !grantSet[id] {
			return ErrInboundNotGranted
		}
	}
	return nil
}

// Scope identifies who is accessing bot management UI/handlers.
type Scope struct {
	Base     string // "/master" or "/agent"
	IsMaster bool
	AgentID  int64
}

// Capabilities drives role-gated UI tabs and actions.
type Capabilities struct {
	ManageBot     bool
	ShowPanelsTab bool
	ShowDelete    bool
	ShowAgentCol  bool
}

func CapabilitiesFor(adminRole db.AdminRole, perms *db.AgentPermissions) Capabilities {
	if adminRole == db.RoleMaster {
		return Capabilities{ManageBot: true, ShowPanelsTab: true, ShowDelete: true, ShowAgentCol: true}
	}
	return Capabilities{
		ManageBot:     perms != nil && perms.ManageBot,
		ShowPanelsTab: false,
		ShowDelete:    false,
		ShowAgentCol:  false,
	}
}
