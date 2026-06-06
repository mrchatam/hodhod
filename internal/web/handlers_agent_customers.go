package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
	"github.com/mrchatam/hodhod/internal/sales"
)

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

	result := s.ListPanelAccounts(r, nil, ListPanelAccountsQuery{
		AgentID: agentID, PanelID: panelID,
		Filters: panelUserFilters{Query: q, Status: status},
		Pagination: pag,
	})

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
		"Rows": result.Rows, "Query": q, "StatusFilter": status, "PanelID": panelID,
		"Pagination": result.Pagination, "CanCreate": canCreate && s.canPerm(r, admin, db.PermCreateUser),
		"Panels": assignedPanels, "PanelListErr": result.PanelErr,
	})
}

type agentCustomerEnrich struct {
	panelUsers  map[string]panels.UserInfo
	inboundTags map[int]string
	services    map[int64]db.Service
}

func (s *Server) loadAgentCustomerEnrich(ctx context.Context, agentID int64, panelIDs []int64) (map[int64]*agentCustomerEnrich, map[int64]error) {
	out := map[int64]*agentCustomerEnrich{}
	errs := map[int64]error{}
	svcs, _ := s.Store.ListServicesByAgent(ctx, agentID)
	for _, pid := range panelIDs {
		client, err := s.Panels.Get(ctx, pid)
		if err != nil {
			errs[pid] = err
			continue
		}
		inbounds, _ := client.ListInbounds(ctx)
		tagMap := map[int]string{}
		for _, inb := range inbounds {
			tagMap[inb.ID] = inb.Tag
		}
		users, err := client.ListUsers(ctx)
		if err != nil {
			errs[pid] = err
			continue
		}
		byUser := map[string]panels.UserInfo{}
		for _, u := range dedupePanelUsersByUsername(users) {
			byUser[u.Username] = u
		}
		panelSvcs := map[int64]db.Service{}
		for _, svc := range svcs {
			if svc.PanelID == pid {
				panelSvcs[svc.ID] = svc
			}
		}
		out[pid] = &agentCustomerEnrich{
			panelUsers: byUser, inboundTags: tagMap, services: panelSvcs,
		}
	}
	return out, errs
}

func (s *Server) agentCustomerRow(v agentVisibleUser, agents map[int64]string, perms *db.AgentPermissions, enrich *agentCustomerEnrich) PanelUserRow {
	row := PanelUserRow{Username: v.Username, ServiceID: v.ServiceID, PanelID: v.PanelID}
	if enrich != nil {
		if svc, ok := enrich.services[v.ServiceID]; ok && v.ServiceID > 0 {
			row = panelUserRowFromService(svc, agents)
			row.PanelID = v.PanelID
		}
		if u, ok := enrich.panelUsers[v.Username]; ok {
			panelRow := panelUserRowFromPanel(u, enrich.inboundTags)
			row.SubLink = u.SubscriptionURL
			if v.ServiceID > 0 {
				if svc, ok := enrich.services[v.ServiceID]; ok {
					row = mergePanelUserWithService(panelRow, svc, agents)
				}
			} else {
				row = panelRow
				row.Source = "panel"
			}
			row.PanelID = v.PanelID
		}
	}
	row.CanModify = agentCanModifyUser(v, s.agentHasModifyPerm(perms))
	return row
}

func (s *Server) agentHasModifyPerm(perms *db.AgentPermissions) bool {
	if perms == nil {
		return false
	}
	if perms.ViewOnly {
		return false
	}
	return perms.Has(db.PermModifyUser)
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

	result := s.ListPanelAccounts(r, panel, ListPanelAccountsQuery{
		AgentID: agentID, PanelID: panelID, Filters: f, Pagination: pag,
	})
	canCreate, _ := s.agentPanelCanCreate(r.Context(), agentID, panelID)
	templates, _ := loadUserCreateTemplates(r.Context(), s.Store, panelID)

	s.renderPage(w, "agent_panel_customers", r, map[string]any{
		"Panel": panel, "Rows": result.Rows, "Filters": f, "Pagination": result.Pagination,
		"CanCreate": canCreate && s.canPerm(r, admin, db.PermCreateUser),
		"Perms": perms, "ShowCreateModal": r.URL.Query().Get("create") == "1",
		"Templates": templates, "PanelListErr": result.PanelErr,
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
	label := r.FormValue("label")
	note := r.FormValue("note")
	var inboundIDs []int
	templates, _ := loadUserCreateTemplates(r.Context(), s.Store, panelID)
	if tpl, ok := findUserCreateTemplate(templates, r.FormValue("template_name")); ok {
		if tpl.VolumeGB > 0 {
			vol = tpl.VolumeGB
		}
		if tpl.DurationDays > 0 {
			dur = tpl.DurationDays
		}
		if tpl.Note != "" && note == "" {
			note = tpl.Note
		}
		if label == "" && tpl.Note != "" {
			label = tpl.Note
		}
		inboundIDs = tpl.InboundIDs
	}
	svc, err := s.Sales.CreatePanelAccount(r.Context(), sales.CreatePanelAccountInput{
		CreateManualInput: sales.CreateManualInput{
			AgentID: agentID, PanelID: panelID, Label: label, Contact: r.FormValue("contact"),
			VolumeGB: vol, DurationDays: dur, AdminID: admin.ID, IsMaster: false, InboundIDs: inboundIDs,
		},
		Note: note,
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
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	email := s.panelUserEmail(r)
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if admin.AgentID == nil || !s.canPerm(r, admin, db.PermModifyUser) {
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
	if !agentCanModifyUser(v, s.agentHasModifyPerm(perms)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = r.ParseForm()
	in := modifyInputFromForm(r)
	_, err := s.Sales.ModifyPanelAccount(r.Context(), sales.ModifyPanelAccountInput{
		ModifyInput: in,
		PanelID:     panelID,
		Username:    email,
		AgentID:     agentID,
	})
	if err != nil {
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

func (s *Server) postAgentPanelUserReset(w http.ResponseWriter, r *http.Request) {
	s.postAgentPanelUserAction(w, r, db.PermResetUsage, s.Sales.ResetPanelUsage)
}

func (s *Server) postAgentPanelUserDisable(w http.ResponseWriter, r *http.Request) {
	s.postAgentPanelUserAction(w, r, db.PermDisableEnable, func(ctx context.Context, panelID int64, email string) error {
		return s.Sales.SetPanelAccountEnabled(ctx, panelID, email, false)
	})
}

func (s *Server) postAgentPanelUserEnable(w http.ResponseWriter, r *http.Request) {
	s.postAgentPanelUserAction(w, r, db.PermDisableEnable, func(ctx context.Context, panelID int64, email string) error {
		return s.Sales.SetPanelAccountEnabled(ctx, panelID, email, true)
	})
}

func (s *Server) postAgentPanelUserAction(w http.ResponseWriter, r *http.Request, perm db.Perm, fn func(context.Context, int64, string) error) {
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
	if !agentCanModifyUser(v, s.agentHasModifyPerm(perms)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := fn(r.Context(), panelID, email); err != nil {
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
