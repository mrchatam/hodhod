package web

import (
	"fmt"
	"strings"
	"time"

	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
)

// PanelUserRow is a unified view of a panel client and/or Hodhod service.
type PanelUserRow struct {
	Username       string
	Label          string
	AgentName      string
	ServiceID      int64
	UsedBytes      int64
	DataLimitBytes int64
	ExpireAt       time.Time
	Enabled        bool
	Status         string
	Source         string // panel, hodhod, both
	InboundIDs     []int
	InboundTags    []string
	HodhodOnly     bool
	CanModify      bool
	SubLink        string
	PanelID        int64
}

type panelUserFilters struct {
	Query      string
	Inbound    string
	Status     string
	Source     string
	AgentID    int64
}

type panelUserStats struct {
	PanelCount  int
	HodhodCount int
	Shown       int
}

func mergePanelUsers(panelUsers []panels.UserInfo, services []db.Service, agents map[int64]string, inbounds []panels.InboundInfo, f panelUserFilters) ([]PanelUserRow, panelUserStats) {
	inboundTags := map[int]string{}
	for _, inb := range inbounds {
		inboundTags[inb.ID] = inb.Tag
	}

	panelByUser := map[string]panels.UserInfo{}
	for _, u := range panelUsers {
		if existing, ok := panelByUser[u.Username]; ok {
			u.InboundIDs = mergeIntSlices(existing.InboundIDs, u.InboundIDs)
			u.InboundTags = mergeStrSlices(existing.InboundTags, u.InboundTags)
		}
		panelByUser[u.Username] = u
	}

	svcByUser := map[string]db.Service{}
	for _, svc := range services {
		if f.AgentID > 0 && svc.AgentID != f.AgentID {
			continue
		}
		svcByUser[svc.PanelUsername] = svc
	}

	seen := map[string]bool{}
	var rows []PanelUserRow
	stats := panelUserStats{PanelCount: len(panelByUser), HodhodCount: len(svcByUser)}

	addRow := func(row PanelUserRow) {
		if !matchPanelUserFilters(row, f) {
			return
		}
		rows = append(rows, row)
		stats.Shown++
	}

	for username, u := range panelByUser {
		seen[username] = true
		row := panelUserRowFromPanel(u, inboundTags)
		if svc, ok := svcByUser[username]; ok {
			row = mergePanelUserWithService(row, svc, agents)
			row.Source = "both"
		} else {
			row.Source = "panel"
		}
		addRow(row)
	}

	for username, svc := range svcByUser {
		if seen[username] {
			continue
		}
		row := panelUserRowFromService(svc, agents)
		row.Source = "hodhod"
		row.HodhodOnly = true
		addRow(row)
	}

	return rows, stats
}

// mergePanelUsersPage merges one page of panel users with Hodhod services.
func mergePanelUsersPage(panelUsers []panels.UserInfo, services []db.Service, agents map[int64]string, inbounds []panels.InboundInfo, f panelUserFilters) ([]PanelUserRow, panelUserStats) {
	inboundTags := map[int]string{}
	for _, inb := range inbounds {
		inboundTags[inb.ID] = inb.Tag
	}
	svcByUser := map[string]db.Service{}
	for _, svc := range services {
		if f.AgentID > 0 && svc.AgentID != f.AgentID {
			continue
		}
		svcByUser[svc.PanelUsername] = svc
	}
	var rows []PanelUserRow
	stats := panelUserStats{PanelCount: len(panelUsers), HodhodCount: len(svcByUser)}
	for _, u := range panelUsers {
		row := panelUserRowFromPanel(u, inboundTags)
		if svc, ok := svcByUser[u.Username]; ok {
			row = mergePanelUserWithService(row, svc, agents)
			row.Source = "both"
		} else {
			row.Source = "panel"
		}
		if matchPanelUserFilters(row, f) {
			rows = append(rows, row)
			stats.Shown++
		}
	}
	return rows, stats
}

func panelUserRowFromPanel(u panels.UserInfo, inboundTags map[int]string) PanelUserRow {
	tags := u.InboundTags
	if len(tags) == 0 && len(u.InboundIDs) > 0 {
		for _, id := range u.InboundIDs {
			if t := inboundTags[id]; t != "" {
				tags = appendUniqueStr(tags, t)
			}
		}
	}
	if u.InboundTag != "" && len(tags) == 0 {
		tags = []string{u.InboundTag}
	}
	status := "active"
	if !u.Enabled {
		status = "disabled"
	} else if !u.ExpireAt.IsZero() && u.ExpireAt.Before(time.Now()) {
		status = "expired"
	}
	return PanelUserRow{
		Username: u.Username, UsedBytes: u.UsedBytes, DataLimitBytes: u.DataLimitBytes,
		ExpireAt: u.ExpireAt, Enabled: u.Enabled, Status: status,
		InboundIDs: u.InboundIDs, InboundTags: tags,
	}
}

func panelUserRowFromService(svc db.Service, agents map[int64]string) PanelUserRow {
	expire := time.Time{}
	if svc.ExpireAt != nil {
		expire = *svc.ExpireAt
	}
	return PanelUserRow{
		Username: svc.PanelUsername, Label: svc.Label, ServiceID: svc.ID,
		AgentName: agents[svc.AgentID], UsedBytes: svc.UsedBytes,
		DataLimitBytes: svc.DataLimitBytes, ExpireAt: expire,
		Enabled: svc.Status == "active", Status: svc.Status,
	}
}

func mergePanelUserWithService(row PanelUserRow, svc db.Service, agents map[int64]string) PanelUserRow {
	row.Label = svc.Label
	row.ServiceID = svc.ID
	row.AgentName = agents[svc.AgentID]
	if row.UsedBytes == 0 {
		row.UsedBytes = svc.UsedBytes
	}
	if row.DataLimitBytes == 0 {
		row.DataLimitBytes = svc.DataLimitBytes
	}
	if row.ExpireAt.IsZero() && svc.ExpireAt != nil {
		row.ExpireAt = *svc.ExpireAt
	}
	if svc.Status == "expired" {
		row.Status = "expired"
	} else if svc.Status == "disabled" {
		row.Status = "disabled"
	}
	return row
}

func matchPanelUserFilters(row PanelUserRow, f panelUserFilters) bool {
	if f.Query != "" {
		q := strings.ToLower(f.Query)
		if !strings.Contains(strings.ToLower(row.Username), q) &&
			!strings.Contains(strings.ToLower(row.Label), q) &&
			!strings.Contains(strings.ToLower(row.AgentName), q) {
			return false
		}
	}
	if f.Inbound != "" {
		found := false
		for _, id := range row.InboundIDs {
			if f.Inbound == fmt.Sprintf("%d", id) {
				found = true
				break
			}
		}
		if !found {
			for _, tag := range row.InboundTags {
				if strings.EqualFold(tag, f.Inbound) {
					found = true
					break
				}
			}
		}
		if !found && len(row.InboundIDs) == 0 && row.Source == "hodhod" {
			// Hodhod-only rows have no inbound metadata — still show unless filtering by inbound
			return false
		}
		if !found {
			return false
		}
	}
	if f.Status != "" && f.Status != "all" && row.Status != f.Status {
		return false
	}
	switch f.Source {
	case "panel":
		if row.Source != "panel" {
			return false
		}
	case "hodhod":
		if row.Source != "hodhod" && row.Source != "both" {
			return false
		}
	case "both":
		if row.Source != "both" {
			return false
		}
	}
	return true
}

func mergeIntSlices(a, b []int) []int {
	out := append([]int{}, a...)
	for _, v := range b {
		out = appendUniqueInt(out, v)
	}
	return out
}

func mergeStrSlices(a, b []string) []string {
	out := append([]string{}, a...)
	for _, v := range b {
		out = appendUniqueStr(out, v)
	}
	return out
}

func appendUniqueInt(s []int, v int) []int {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

func appendUniqueStr(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

func inboundIDsFromForm(values []string, comma string, single int) []int {
	if len(values) > 0 {
		var ids []int
		for _, v := range values {
			if n, err := parseIntTrim(v); err == nil && n > 0 {
				ids = append(ids, n)
			}
		}
		if len(ids) > 0 {
			return ids
		}
	}
	if ids := parseInboundIDs(comma); len(ids) > 0 {
		return ids
	}
	if single > 0 {
		return []int{single}
	}
	return nil
}

func parseIntTrim(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func formatInboundLabels(ids []int, tags []string, tagMap map[int]string) string {
	if len(tags) > 0 {
		return strings.Join(tags, ", ")
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if t := tagMap[id]; t != "" {
			parts = append(parts, fmt.Sprintf("%d:%s", id, t))
		} else {
			parts = append(parts, fmt.Sprintf("%d", id))
		}
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, ", ")
}
