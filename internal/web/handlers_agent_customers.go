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

	panelIDs := []int64{}
	if panelID > 0 {
		panelIDs = []int64{panelID}
	} else {
		panels, _ := s.Store.ListPanelsForAgent(r.Context(), agentID)
		for _, p := range panels {
			panelIDs = append(panelIDs, p.ID)
		}
	}
	enrichByPanel, listErrs := s.loadAgentCustomerEnrich(r.Context(), agentID, panelIDs)

	for _, v := range visible {
		if !agentCanViewUser(v) {
			continue
		}
		enrich := enrichByPanel[v.PanelID]
		row := s.agentCustomerRow(v, agents, perms, enrich)
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

	var panelListErr error
	for _, err := range listErrs {
		if err != nil {
			panelListErr = err
			break
		}
	}

	s.renderPage(w, "agent_customers", r, map[string]any{
		"Rows": pageRows, "Query": q, "StatusFilter": status, "PanelID": panelID,
		"Pagination": pag, "CanCreate": canCreate && s.canPerm(r, admin, db.PermCreateUser),
		"Panels": assignedPanels, "PanelListErr": panelListErr,
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
	enrichByPanel, listErrs := s.loadAgentCustomerEnrich(r.Context(), agentID, []int64{panelID})
	var panelListErr error
	if err := listErrs[panelID]; err != nil {
		panelListErr = err
	}
	var rows []PanelUserRow
	for _, v := range visible {
		if !agentCanViewUser(v) {
			continue
		}
		row := s.agentCustomerRow(v, agents, perms, enrichByPanel[panelID])
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
	templates, _ := loadUserCreateTemplates(r.Context(), s.Store, panelID)

	s.renderPage(w, "agent_panel_customers", r, map[string]any{
		"Panel": panel, "Rows": rows[start:end], "Filters": f, "Pagination": pag,
		"CanCreate": canCreate && s.canPerm(r, admin, db.PermCreateUser),
		"Perms": perms, "ShowCreateModal": r.URL.Query().Get("create") == "1",
		"Templates": templates, "PanelListErr": panelListErr,
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
	svc, err := s.Sales.CreateManualService(r.Context(), sales.CreateManualInput{
		AgentID: agentID, PanelID: panelID, Label: label, Contact: r.FormValue("contact"),
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
