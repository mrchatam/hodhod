package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
	"github.com/mrchatam/hodhod/internal/sales"
)

func (s *Server) redirectAgentServices(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if admin.Role != db.RoleAgent {
		http.NotFound(w, r)
		return
	}
	target := strings.TrimPrefix(r.URL.Path, "/services")
	if target == "" || target == "/" {
		http.Redirect(w, r, "/customers", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/customers"+target, http.StatusSeeOther)
}

func (s *Server) pageCustomers(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if admin.Role == db.RoleMaster {
		s.pageServices(w, r)
		return
	}
	if admin.AgentID == nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	agentID := *admin.AgentID
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	panelID, _ := strconv.ParseInt(r.URL.Query().Get("panel_id"), 10, 64)
	pag := paginationFromRequest(r, 0, "q", "status", "panel_id")

	visible, err := s.buildAgentVisibleUsers(r.Context(), agentID, panelID)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}

	perms, _ := s.permsFor(r, admin)
	var rows []PanelUserRow
	agents := s.agentNameMap(r.Context())
	for _, v := range visible {
		if !agentCanViewUser(v) {
			continue
		}
		row := s.agentCustomerRow(r.Context(), v, agents, perms)
		if q != "" && !strings.Contains(strings.ToLower(row.Username), strings.ToLower(q)) &&
			!strings.Contains(strings.ToLower(row.Label), strings.ToLower(q)) {
			continue
		}
		if status != "" && row.Status != status {
			continue
		}
		rows = append(rows, row)
	}
	pag.Total = len(rows)
	start := pag.Offset()
	end := start + pag.PerPage
	if start > len(rows) {
		start = len(rows)
	}
	if end > len(rows) {
		end = len(rows)
	}
	pageRows := rows[start:end]

	canCreate, _ := s.Store.AgentHasAnyCreateInbound(r.Context(), agentID)
	if !canCreate {
		panels, _ := s.Store.ListPanelsForAgent(r.Context(), agentID)
		for _, p := range panels {
			if ok, _ := s.agentPanelCanCreate(r.Context(), agentID, p.ID); ok {
				canCreate = true
				break
			}
		}
	}
	assignedPanels, _ := s.Store.ListPanelsForAgent(r.Context(), agentID)

	s.renderPage(w, "agent_customers", r, map[string]any{
		"Rows": pageRows, "Query": q, "StatusFilter": status, "PanelID": panelID,
		"Pagination": pag, "CanCreate": canCreate && s.canPerm(r, admin, db.PermCreateUser),
		"Panels": assignedPanels,
	})
}

func (s *Server) agentCustomerRow(ctx context.Context, v agentVisibleUser, agents map[int64]string, perms *db.AgentPermissions) PanelUserRow {
	row := PanelUserRow{Username: v.Username, ServiceID: v.ServiceID, PanelID: v.PanelID}
	if v.ServiceID > 0 {
		if svc, err := s.Store.GetService(ctx, v.ServiceID); err == nil {
			row = panelUserRowFromService(*svc, agents)
			row.PanelID = v.PanelID
		}
	}
	client, err := s.Panels.Get(ctx, v.PanelID)
	if err == nil {
		if u, err := client.GetUser(ctx, v.Username); err == nil {
			inbounds, _ := client.ListInbounds(ctx)
			tagMap := map[int]string{}
			for _, inb := range inbounds {
				tagMap[inb.ID] = inb.Tag
			}
			panelRow := panelUserRowFromPanel(*u, tagMap)
			row.SubLink = u.SubscriptionURL
			if v.ServiceID > 0 {
				if svc, err := s.Store.GetService(ctx, v.ServiceID); err == nil {
					row = mergePanelUserWithService(panelRow, *svc, agents)
				}
			} else {
				row = panelRow
				row.Source = "panel"
			}
			row.PanelID = v.PanelID
		}
	}
	row.CanModify = agentCanModifyUser(v, perms.Has(db.PermModifyUser))
	return row
}

func (s *Server) pageCustomerCreate(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if admin.Role == db.RoleMaster {
		s.pageServiceCreate(w, r)
		return
	}
	if admin.AgentID == nil || !s.canPerm(r, admin, db.PermCreateUser) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	agentID := *admin.AgentID
	var creatable []db.Panel
	panels, _ := s.Store.ListPanelsForAgent(r.Context(), agentID)
	for _, p := range panels {
		if ok, _ := s.agentPanelCanCreate(r.Context(), agentID, p.ID); ok {
			creatable = append(creatable, p)
		}
	}
	s.renderPage(w, "customer_create", r, map[string]any{"Panels": creatable})
}

func (s *Server) pageAgentPanelCustomers(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if admin.AgentID == nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	agentID := *admin.AgentID
	ok, err := s.Store.AgentHasPanel(r.Context(), agentID, panelID)
	if err != nil || !ok {
		http.NotFound(w, r)
		return
	}
	panel, err := s.Store.GetPanel(r.Context(), panelID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f := s.panelUserFiltersFromRequest(r)
	pag := paginationFromRequest(r, 0, "q", "status")
	perms, _ := s.permsFor(r, admin)
	agents := s.agentNameMap(r.Context())

	visible, err := s.buildAgentVisibleUsers(r.Context(), agentID, panelID)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	var rows []PanelUserRow
	for _, v := range visible {
		if !agentCanViewUser(v) {
			continue
		}
		row := s.agentCustomerRow(r.Context(), v, agents, perms)
		if matchPanelUserFilters(row, panelUserFilters{Query: f.Query, Status: f.Status}) {
			rows = append(rows, row)
		}
	}
	pag.Total = len(rows)
	start := pag.Offset()
	end := start + pag.PerPage
	if start > len(rows) {
		start = len(rows)
	}
	if end > len(rows) {
		end = len(rows)
	}
	canCreate, _ := s.agentPanelCanCreate(r.Context(), agentID, panelID)
	createInbounds, _ := s.Store.ListAgentInboundCreateIDs(r.Context(), agentID, panelID)
	templates, _ := loadUserCreateTemplates(r.Context(), s.Store, panelID)
	var inboundRows []panels.InboundInfo
	if client, err := s.Panels.Get(r.Context(), panelID); err == nil {
		allInb, _ := client.ListInbounds(r.Context())
		grantSet := map[int]bool{}
		for _, id := range createInbounds {
			grantSet[id] = true
		}
		for _, inb := range allInb {
			if grantSet[inb.ID] {
				inboundRows = append(inboundRows, inb)
			}
		}
	}

	s.renderPage(w, "agent_panel_customers", r, map[string]any{
		"Panel": panel, "Rows": rows[start:end], "Filters": f, "Pagination": pag,
		"CanCreate": canCreate && s.canPerm(r, admin, db.PermCreateUser),
		"Perms": perms, "ShowCreateModal": r.URL.Query().Get("create") == "1",
		"CreateInbounds": createInbounds, "InboundRows": inboundRows, "Templates": templates,
	})
}

func (s *Server) postAgentPanelUser(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	lang, _ := s.Store.GetSetting(r.Context(), "admin", admin.ID, "lang")
	if lang == "" {
		lang = "fa"
	}
	if admin.AgentID == nil || !s.canPerm(r, admin, db.PermCreateUser) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	agentID := *admin.AgentID
	_ = r.ParseForm()
	vol, _ := strconv.Atoi(r.FormValue("volume_gb"))
	dur, _ := strconv.Atoi(r.FormValue("duration_days"))
	if vol <= 0 {
		vol = 30
	}
	if dur <= 0 {
		dur = 30
	}
	inboundIDs := inboundIDsFromForm(r.Form["inbound_ids"], r.FormValue("inbound_ids"), 0)
	svc, err := s.Sales.CreateManualService(r.Context(), sales.CreateManualInput{
		AgentID: agentID, PanelID: panelID, Label: r.FormValue("label"), Contact: r.FormValue("contact"),
		VolumeGB: vol, DurationDays: dur, AdminID: admin.ID, IsMaster: false, InboundIDs: inboundIDs,
	})
	if err != nil {
		s.setFlash(w, "err", friendlySalesErr(lang, err))
		http.Redirect(w, r, fmt.Sprintf("/agent/panels/%d/customers", panelID), http.StatusSeeOther)
		return
	}
	s.audit(r, &admin.ID, "create_customer", "service", svc.ID, map[string]any{"panel_id": panelID})
	s.setFlash(w, "ok", "Customer created")
	http.Redirect(w, r, fmt.Sprintf("/agent/panels/%d/customers", panelID), http.StatusSeeOther)
}

func (s *Server) postAgentPanelUserModify(w http.ResponseWriter, r *http.Request) {
	s.postAgentPanelUserAction(w, r, db.PermModifyUser, func(client panels.Client, email string) error {
		_ = r.ParseForm()
		req := panels.UpdateUserRequest{}
		if v := r.FormValue("volume_gb"); v != "" {
			gb, _ := strconv.Atoi(v)
			bytes := int64(gb) * 1024 * 1024 * 1024
			req.DataLimitBytes = &bytes
		}
		if v := r.FormValue("duration_days"); v != "" {
			d, _ := strconv.Atoi(v)
			t := time.Now().Add(time.Duration(d) * 24 * time.Hour)
			req.ExpireAt = &t
		}
		_, err := client.UpdateUser(r.Context(), email, req)
		return err
	})
}

func (s *Server) postAgentPanelUserReset(w http.ResponseWriter, r *http.Request) {
	s.postAgentPanelUserAction(w, r, db.PermResetUsage, func(client panels.Client, email string) error {
		return client.ResetUsage(r.Context(), email)
	})
}

func (s *Server) postAgentPanelUserDisable(w http.ResponseWriter, r *http.Request) {
	s.postAgentPanelUserAction(w, r, db.PermDisableEnable, func(client panels.Client, email string) error {
		return client.Disable(r.Context(), email)
	})
}

func (s *Server) postAgentPanelUserEnable(w http.ResponseWriter, r *http.Request) {
	s.postAgentPanelUserAction(w, r, db.PermDisableEnable, func(client panels.Client, email string) error {
		return client.Enable(r.Context(), email)
	})
}

func (s *Server) postAgentPanelUserAction(w http.ResponseWriter, r *http.Request, perm db.Perm, fn func(panels.Client, string) error) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	email := s.panelUserEmail(r)
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if admin.AgentID == nil || !s.canPerm(r, admin, perm) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	agentID := *admin.AgentID
	visible, _ := s.buildAgentVisibleUsers(r.Context(), agentID, panelID)
	v, ok := visible[agentUserKeyStr(panelID, email)]
	if !ok || !agentCanViewUser(v) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	perms, _ := s.permsFor(r, admin)
	if !agentCanModifyUser(v, perms.Has(perm)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	client, err := s.Panels.Get(r.Context(), panelID)
	if err != nil {
		s.panelUserHTMLError(w, err.Error())
		return
	}
	if err = fn(client, email); err != nil {
		s.panelUserHTMLError(w, err.Error())
		return
	}
	if r.Header.Get("HX-Request") != "" {
		s.renderPanelUserRow(w, r, panelID, email)
		return
	}
	s.setFlash(w, "ok", "Done")
	http.Redirect(w, r, fmt.Sprintf("/agent/panels/%d/customers", panelID), http.StatusSeeOther)
}
