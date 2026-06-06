package botconfig

import "github.com/mrchatam/hodhod/internal/db"

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
