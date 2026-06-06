package web

import (
	"net/http"

	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
)

// ListPanelAccountsQuery configures unified panel account listing.
type ListPanelAccountsQuery struct {
	PanelID      int64
	AgentID      int64 // >0 for agent-scoped visibility
	Filters      panelUserFilters
	Pagination   Pagination
	EnrichOnline bool
}

// ListPanelAccountsResult is the unified list output.
type ListPanelAccountsResult struct {
	Rows       []PanelUserRow
	Stats      panelUserStats
	Pagination Pagination
	PanelErr   error
}

// ListPanelAccounts builds a role-scoped panel account list (master panel tab or agent customers).
func (s *Server) ListPanelAccounts(r *http.Request, panel *db.Panel, q ListPanelAccountsQuery) ListPanelAccountsResult {
	if q.AgentID > 0 {
		return s.listAgentPanelAccounts(r, panel, q)
	}
	return s.listMasterPanelAccounts(r, panel, q)
}

func (s *Server) listMasterPanelAccounts(r *http.Request, panel *db.Panel, q ListPanelAccountsQuery) ListPanelAccountsResult {
	ctx := r.Context()
	panelID := panel.ID
	pag := q.Pagination
	f := q.Filters
	agents := s.agentNameMap(ctx)

	client, err := s.Panels.Get(ctx, panelID)
	if err != nil {
		return ListPanelAccountsResult{PanelErr: err, Pagination: pag}
	}
	inbounds, _ := client.ListInbounds(ctx)
	allServices, _ := s.Store.ListServicesForPanel(ctx, panelID)

	if needsLocalPanelUserMerge(f) {
		allUsers, err := client.ListUsers(ctx)
		var panelErr error
		var rows []PanelUserRow
		stats := panelUserStats{}
		if err != nil {
			panelErr = err
		} else {
			rows, stats = mergePanelUsers(allUsers, allServices, agents, inbounds, f)
			rows, pag = paginatePanelUserRows(rows, pag)
			stats.Shown = len(rows)
			stats.PanelCount = len(dedupePanelUsersByUsername(allUsers))
		}
		return ListPanelAccountsResult{Rows: rows, Stats: stats, Pagination: pag, PanelErr: panelErr}
	}

	statusFilter := f.Status
	if statusFilter == "all" {
		statusFilter = ""
	}
	page, err := client.ListUsersPaged(ctx, panels.UserListOptions{
		Page: pag.Page, PageSize: pag.PerPage, Search: f.Query, Status: statusFilter,
	})
	var panelErr error
	var rows []PanelUserRow
	stats := panelUserStats{}
	if err != nil {
		panelErr = err
	} else {
		rows, stats = mergePanelUsersPage(page.Users, allServices, agents, inbounds, f, pag.Page)
		if q.EnrichOnline {
			rows = enrichPanelUserOnline(ctx, client, rows)
		}
		if f.Source == "both" {
			filtered := rows[:0]
			for _, row := range rows {
				if row.Source == "both" {
					filtered = append(filtered, row)
				}
			}
			rows = filtered
		}
		if f.Source == "panel" {
			for i := range rows {
				if rows[i].Source == "both" {
					rows[i].Source = "panel"
				}
			}
		}
		stats.Shown = len(rows)
		pag.Total = page.Filtered
		if pag.Total == 0 {
			pag.Total = page.Total
		}
		stats.PanelCount = page.Total
	}
	return ListPanelAccountsResult{Rows: rows, Stats: stats, Pagination: pag, PanelErr: panelErr}
}

func (s *Server) listAgentPanelAccounts(r *http.Request, panel *db.Panel, q ListPanelAccountsQuery) ListPanelAccountsResult {
	ctx := r.Context()
	agentID := q.AgentID
	panelID := q.PanelID
	if panel != nil {
		panelID = panel.ID
	}
	pag := q.Pagination
	f := q.Filters
	perms, _ := s.permsFor(r, r.Context().Value(ctxAdmin).(*db.Admin))
	agents := s.agentNameMap(ctx)

	visible, err := s.buildAgentVisibleUsers(ctx, agentID, panelID)
	if err != nil {
		return ListPanelAccountsResult{PanelErr: err, Pagination: pag}
	}

	panelIDs := []int64{}
	if panelID > 0 {
		panelIDs = []int64{panelID}
	} else {
		panels, _ := s.Store.ListPanelsForAgent(ctx, agentID)
		for _, p := range panels {
			panelIDs = append(panelIDs, p.ID)
		}
	}
	enrichByPanel, listErrs := s.loadAgentCustomerEnrich(ctx, agentID, panelIDs)
	var panelListErr error
	for _, e := range listErrs {
		if e != nil {
			panelListErr = e
			break
		}
	}

	var rows []PanelUserRow
	for _, v := range visible {
		if !agentCanViewUser(v) {
			continue
		}
		enrich := enrichByPanel[v.PanelID]
		row := s.agentCustomerRow(v, agents, perms, enrich)
		if panelID > 0 && v.PanelID != panelID {
			continue
		}
		if !matchPanelUserFilters(row, f) {
			continue
		}
		rows = append(rows, row)
	}
	pag.Total = len(rows)
	rows, pag = paginatePanelUserRows(rows, pag)
	return ListPanelAccountsResult{
		Rows: rows, Pagination: pag, PanelErr: panelListErr,
		Stats: panelUserStats{Shown: len(rows)},
	}
}
