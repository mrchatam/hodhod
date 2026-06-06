package web

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
)

type agentUserKey struct {
	PanelID  int64
	Username string
}

func agentUserKeyStr(panelID int64, username string) string {
	return fmt.Sprintf("%d|%s", panelID, username)
}

// agentVisibleUser tracks why a user is visible and whether modify is allowed.
type agentVisibleUser struct {
	PanelID        int64
	Username       string
	OwnCreate      bool
	ServiceID      int64
	InboundVisible bool
	UserGrant      *db.AgentUserGrant
}

func userOnInbound(u panels.UserInfo, inboundIDs map[int]bool) bool {
	if len(inboundIDs) == 0 {
		return false
	}
	if inboundIDs[u.InboundID] {
		return true
	}
	for _, id := range u.InboundIDs {
		if inboundIDs[id] {
			return true
		}
	}
	return false
}

func (s *Server) buildAgentVisibleUsers(ctx context.Context, agentID int64, panelID int64) (map[string]agentVisibleUser, error) {
	out := map[string]agentVisibleUser{}

	var panelIDs []int64
	if panelID > 0 {
		ok, err := s.Store.AgentHasPanel(ctx, agentID, panelID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return out, nil
		}
		panelIDs = []int64{panelID}
	} else {
		panels, err := s.Store.ListPanelsForAgent(ctx, agentID)
		if err != nil {
			return nil, err
		}
		for _, p := range panels {
			panelIDs = append(panelIDs, p.ID)
		}
	}

	svcs, err := s.Store.ListServicesByAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	for _, svc := range svcs {
		if panelID > 0 && svc.PanelID != panelID {
			continue
		}
		k := agentUserKeyStr(svc.PanelID, svc.PanelUsername)
		v := out[k]
		v.PanelID = svc.PanelID
		v.Username = svc.PanelUsername
		v.OwnCreate = true
		v.ServiceID = svc.ID
		out[k] = v
	}

	userGrants, err := s.Store.ListAgentUserGrants(ctx, agentID, panelID)
	if err != nil {
		return nil, err
	}
	for _, g := range userGrants {
		if !g.AllowView {
			continue
		}
		k := agentUserKeyStr(g.PanelID, g.PanelUsername)
		v := out[k]
		v.PanelID = g.PanelID
		v.Username = g.PanelUsername
		gcopy := g
		v.UserGrant = &gcopy
		out[k] = v
	}

	for _, pid := range panelIDs {
		viewIDs, err := s.Store.ListAgentInboundViewIDs(ctx, agentID, pid)
		if err != nil {
			return nil, err
		}
		if len(viewIDs) == 0 {
			continue
		}
		viewSet := map[int]bool{}
		for _, id := range viewIDs {
			viewSet[id] = true
		}
		client, err := s.Panels.Get(ctx, pid)
		if err != nil {
			continue
		}
		users, err := client.ListUsers(ctx)
		if err != nil {
			continue
		}
		for _, u := range users {
			if !userOnInbound(u, viewSet) {
				continue
			}
			k := agentUserKeyStr(pid, u.Username)
			v := out[k]
			v.PanelID = pid
			v.Username = u.Username
			v.InboundVisible = true
			out[k] = v
		}
	}
	return out, nil
}

func agentCanModifyUser(v agentVisibleUser, hasPerm bool) bool {
	if !hasPerm {
		return false
	}
	if v.OwnCreate {
		return true
	}
	if v.UserGrant != nil {
		if v.UserGrant.AllowModify {
			return true
		}
		if v.UserGrant.AllowView {
			return false
		}
	}
	if v.InboundVisible {
		return true
	}
	return false
}

func agentCanViewUser(v agentVisibleUser) bool {
	if v.OwnCreate {
		return true
	}
	if v.UserGrant != nil && v.UserGrant.AllowView {
		return true
	}
	if v.InboundVisible {
		return true
	}
	return false
}

func (s *Server) agentPanelCanCreate(ctx context.Context, agentID, panelID int64) (bool, error) {
	ids, err := s.Store.ListAgentInboundCreateIDs(ctx, agentID, panelID)
	if err != nil {
		return false, err
	}
	if len(ids) > 0 {
		return true, nil
	}
	ap, err := s.Store.GetAgentPanel(ctx, agentID, panelID)
	if err != nil {
		return false, err
	}
	var scope panels.Scope
	_ = json.Unmarshal(ap.ScopeJSON, &scope)
	if len(scope.InboundIDs) == 0 && scope.InboundID > 0 {
		scope.InboundIDs = []int{scope.InboundID}
	}
	return len(scope.InboundIDs) > 0, nil
}
