package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
	"github.com/mrchatam/hodhod/internal/sales"
)

func (s *Server) pageServices(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	var svcs []db.Service
	var err error
	if admin.Role == db.RoleMaster {
		svcs, err = s.Store.ListAllServicesFiltered(r.Context(), q, status)
	} else if admin.AgentID != nil {
		svcs, err = s.Store.ListServicesByAgentFiltered(r.Context(), *admin.AgentID, q, status)
	}
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, "services", r, map[string]any{
		"Services": svcs, "Query": q, "StatusFilter": status,
	})
}

func (s *Server) pageServiceCreate(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if !s.canPerm(r, admin, db.PermCreateUser) && admin.Role != db.RoleMaster {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var panelList []db.Panel
	var agents []db.Agent
	if admin.Role == db.RoleMaster {
		panelList, _ = s.Store.ListPanels(r.Context())
		agents, _ = s.Store.ListAgents(r.Context())
	} else if admin.AgentID != nil {
		panelList, _ = s.Store.ListPanelsForAgent(r.Context(), *admin.AgentID)
	}
	s.renderPage(w, "service_create", r, map[string]any{"Panels": panelList, "Agents": agents})
}

func (s *Server) postService(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	_ = r.ParseForm()
	panelID, _ := strconv.ParseInt(r.FormValue("panel_id"), 10, 64)
	vol, _ := strconv.Atoi(r.FormValue("volume_gb"))
	dur, _ := strconv.Atoi(r.FormValue("duration_days"))
	isMaster := admin.Role == db.RoleMaster
	agentID := int64(0)
	if admin.AgentID != nil {
		agentID = *admin.AgentID
	}
	if isMaster {
		if v := r.FormValue("agent_id"); v != "" {
			agentID, _ = strconv.ParseInt(v, 10, 64)
		}
		if agentID == 0 {
			http.Error(w, "agent required", http.StatusBadRequest)
			return
		}
	}
	svc, err := s.Sales.CreateManualService(r.Context(), sales.CreateManualInput{
		AgentID: agentID, PanelID: panelID, Label: r.FormValue("label"), Contact: r.FormValue("contact"),
		VolumeGB: vol, DurationDays: dur, AdminID: admin.ID, IsMaster: isMaster,
	})
	if err != nil {
		s.setFlash(w, "err", err.Error())
		http.Redirect(w, r, "/services/new", http.StatusSeeOther)
		return
	}
	s.audit(r, &admin.ID, "create_service", "service", svc.ID, map[string]any{"agent_id": agentID})
	s.setFlash(w, "ok", "Service created")
	http.Redirect(w, r, "/services", http.StatusSeeOther)
}

func (s *Server) salesAgentID(admin *db.Admin) (int64, bool) {
	if admin.Role == db.RoleMaster {
		return 0, true
	}
	if admin.AgentID == nil {
		return 0, false
	}
	return *admin.AgentID, true
}

func (s *Server) serviceAction(w http.ResponseWriter, r *http.Request, auditAction string, fn func(agentID, sid int64, isMaster bool) error) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	isMaster := admin.Role == db.RoleMaster
	agentID, ok := s.salesAgentID(admin)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := fn(agentID, id, isMaster); err != nil {
		if r.Header.Get("HX-Request") == "true" {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.setFlash(w, "err", err.Error())
	} else {
		if auditAction != "" {
			s.audit(r, &admin.ID, auditAction, "service", id, nil)
		}
		if r.Header.Get("HX-Request") != "true" {
			s.setFlash(w, "ok", "Done")
		}
	}
	if r.Header.Get("HX-Request") == "true" {
		s.renderServiceRow(w, r, id)
		return
	}
	http.Redirect(w, r, "/services", http.StatusSeeOther)
}

func (s *Server) renderServiceRow(w http.ResponseWriter, r *http.Request, serviceID int64) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	isMaster := admin.Role == db.RoleMaster
	agentID := int64(0)
	if admin.AgentID != nil {
		agentID = *admin.AgentID
	}
	var svc *db.Service
	var err error
	if isMaster {
		svc, err = s.Store.GetService(r.Context(), serviceID)
	} else {
		svc, err = s.Store.GetServiceForAgent(r.Context(), agentID, serviceID)
	}
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	perms, _ := s.permsFor(r, admin)
	data := map[string]any{
		"Admin": admin, "CSRF": r.Context().Value(ctxCSRF), "Perms": perms,
		"IsMaster": isMaster, "Svc": svc,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t, ok := s.pages["services"]
	if !ok {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, "service_row", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (s *Server) postServiceModify(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	in := sales.ModifyInput{}
	if v := r.FormValue("volume_gb"); v != "" {
		gb, _ := strconv.Atoi(v)
		in.VolumeGB = &gb
	}
	if v := r.FormValue("duration_days"); v != "" {
		d, _ := strconv.Atoi(v)
		in.DurationDays = &d
	}
	if v := r.FormValue("expire_at"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			in.ExpireAt = &t
		}
	}
	s.serviceAction(w, r, "modify_service", func(agentID, sid int64, isMaster bool) error {
		_, err := s.Sales.ModifyService(r.Context(), agentID, sid, in, isMaster)
		return err
	})
}

func (s *Server) postServiceAddTime(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	days, _ := strconv.Atoi(r.FormValue("days"))
	if days <= 0 {
		days = 30
	}
	s.serviceAction(w, r, "add_time", func(agentID, sid int64, isMaster bool) error {
		_, err := s.Sales.AddTime(r.Context(), agentID, sid, days, isMaster)
		return err
	})
}

func (s *Server) postServiceAddVolume(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	gb, _ := strconv.Atoi(r.FormValue("gb"))
	if gb <= 0 {
		gb = 10
	}
	s.serviceAction(w, r, "add_volume", func(agentID, sid int64, isMaster bool) error {
		_, err := s.Sales.AddVolume(r.Context(), agentID, sid, gb, isMaster)
		return err
	})
}

func (s *Server) postServiceDisable(w http.ResponseWriter, r *http.Request) {
	s.serviceAction(w, r, "disable_service", func(agentID, sid int64, isMaster bool) error {
		_, err := s.Sales.SetEnabled(r.Context(), agentID, sid, false, isMaster)
		return err
	})
}

func (s *Server) postServiceEnable(w http.ResponseWriter, r *http.Request) {
	s.serviceAction(w, r, "enable_service", func(agentID, sid int64, isMaster bool) error {
		_, err := s.Sales.SetEnabled(r.Context(), agentID, sid, true, isMaster)
		return err
	})
}

func (s *Server) postServiceReset(w http.ResponseWriter, r *http.Request) {
	s.serviceAction(w, r, "reset_usage", func(agentID, sid int64, isMaster bool) error {
		_, err := s.Sales.ResetUsage(r.Context(), agentID, sid, isMaster)
		return err
	})
}

func (s *Server) postServiceDelete(w http.ResponseWriter, r *http.Request) {
	s.serviceAction(w, r, "delete_service", func(agentID, sid int64, isMaster bool) error {
		return s.Sales.DeleteService(r.Context(), agentID, sid, isMaster)
	})
}

func parseInboundIDs(raw string) []int {
	var ids []int
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if n, err := strconv.Atoi(p); err == nil {
			ids = append(ids, n)
		}
	}
	return ids
}

func scopeJSONFromInboundIDs(ids []int) []byte {
	sc := panels.Scope{InboundIDs: ids}
	b, _ := json.Marshal(sc)
	return b
}
