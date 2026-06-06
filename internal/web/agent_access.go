package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

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
		if !g.AllowView && !g.AllowModify {
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
		return false
	}
	return false
}

func agentCanViewUser(v agentVisibleUser) bool {
	if v.OwnCreate {
		return true
	}
	if v.UserGrant != nil && (v.UserGrant.AllowView || v.UserGrant.AllowModify) {
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

func parseInboundGrantsFromForm(form url.Values) []db.AgentInboundGrant {
	var inboundGrants []db.AgentInboundGrant
	for key, val := range form {
		if !strings.HasPrefix(key, "inbound_") || len(val) == 0 || val[0] != "on" {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(key, "inbound_"), "_")
		if len(parts) != 2 {
			continue
		}
		inboundID, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		g := db.AgentInboundGrant{InboundID: inboundID}
		switch parts[1] {
		case "create":
			g.AllowCreate = true
		case "view":
			g.AllowViewUsers = true
		default:
			continue
		}
		found := false
		for i := range inboundGrants {
			if inboundGrants[i].InboundID == inboundID {
				if parts[1] == "create" {
					inboundGrants[i].AllowCreate = true
				} else {
					inboundGrants[i].AllowViewUsers = true
				}
				found = true
				break
			}
		}
		if !found {
			inboundGrants = append(inboundGrants, g)
		}
	}
	return inboundGrants
}

func parseUserGrantsFromForm(form url.Values) []db.AgentUserGrant {
	var userGrants []db.AgentUserGrant
	for key, val := range form {
		if !strings.HasPrefix(key, "user_") || len(val) == 0 || val[0] != "on" {
			continue
		}
		parts := strings.SplitN(strings.TrimPrefix(key, "user_"), "_", 2)
		if len(parts) != 2 {
			continue
		}
		username, err := url.PathUnescape(parts[0])
		if err != nil {
			username = parts[0]
		}
		g := db.AgentUserGrant{PanelUsername: username}
		switch parts[1] {
		case "view":
			g.AllowView = true
		case "modify":
			g.AllowModify = true
		default:
			continue
		}
		found := false
		for i := range userGrants {
			if userGrants[i].PanelUsername == username {
				if parts[1] == "view" {
					userGrants[i].AllowView = true
				} else {
					userGrants[i].AllowModify = true
				}
				found = true
				break
			}
		}
		if !found {
			userGrants = append(userGrants, g)
		}
	}
	return userGrants
}

func parseAccessTableUsernames(form url.Values) []string {
	raw := form.Get("access_user_names")
	if raw == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		u, err := url.PathUnescape(strings.TrimSpace(part))
		if err != nil {
			u = strings.TrimSpace(part)
		}
		if u != "" {
			out = append(out, u)
		}
	}
	return out
}

func (s *Server) attachPanelUsersToAgent(ctx context.Context, agentID, panelID int64, usernames []string) (int, error) {
	client, err := s.Panels.Get(ctx, panelID)
	if err != nil {
		return 0, err
	}
	attached := 0
	for _, username := range usernames {
		if username == "" {
			continue
		}
		if err := s.Store.UpsertAgentUserGrant(ctx, &db.AgentUserGrant{
			AgentID: agentID, PanelID: panelID, PanelUsername: username,
			AllowView: true, AllowModify: true,
		}); err != nil {
			return attached, err
		}
		exists, err := s.Store.ServiceExistsByPanelUsername(ctx, panelID, username)
		if err != nil {
			return attached, err
		}
		if !exists {
			sub, _ := client.SubscriptionURL(ctx, username)
			u, uerr := client.GetUser(ctx, username)
			svc := &db.Service{
				AgentID: agentID, Source: db.ServiceSourcePanel, PanelID: panelID,
				PanelUsername: username, SubLink: sub, Status: "active",
			}
			if uerr == nil {
				svc.DataLimitBytes = u.DataLimitBytes
				svc.UsedBytes = u.UsedBytes
				if !u.ExpireAt.IsZero() {
					t := u.ExpireAt
					svc.ExpireAt = &t
				}
			}
			_ = s.Store.CreateService(ctx, svc)
		}
		attached++
	}
	return attached, nil
}
