package web

import (
	"testing"
	"time"

	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
)

func TestMergePanelUsersPage(t *testing.T) {
	now := time.Now().Add(24 * time.Hour)
	panelUsers := []panels.UserInfo{
		{Username: "u1@x", Enabled: true, DataLimitBytes: 1e9, ExpireAt: now, InboundIDs: []int{1}},
		{Username: "u2@x", Enabled: true},
	}
	services := []db.Service{
		{PanelUsername: "u1@x", Label: "VIP", AgentID: 1, Status: "active"},
	}
	agents := map[int64]string{1: "Agent A"}
	inbounds := []panels.InboundInfo{{ID: 1, Tag: "vless-in"}}
	f := panelUserFilters{Status: "all"}

	rows, stats := mergePanelUsersPage(panelUsers, services, agents, inbounds, f, 1)
	if len(rows) != 2 {
		t.Fatalf("rows=%d", len(rows))
	}
	if stats.Shown != 2 {
		t.Fatalf("shown=%d", stats.Shown)
	}
	if rows[0].Source != "both" || rows[0].Label != "VIP" {
		t.Fatalf("row0: source=%s label=%s", rows[0].Source, rows[0].Label)
	}
	if rows[1].Source != "panel" {
		t.Fatalf("row1 source=%s", rows[1].Source)
	}
}

func TestMergePanelUsersPage_hodhodOnly(t *testing.T) {
	panelUsers := []panels.UserInfo{{Username: "on@panel", Enabled: true}}
	services := []db.Service{
		{PanelUsername: "on@panel", AgentID: 1},
		{PanelUsername: "hodhod@only", AgentID: 1, Status: "active"},
	}
	rows, stats := mergePanelUsersPage(panelUsers, services, map[int64]string{1: "A"}, nil, panelUserFilters{Status: "all"}, 1)
	if len(rows) != 2 {
		t.Fatalf("rows=%d want 2", len(rows))
	}
	if stats.HodhodCount != 2 {
		t.Fatalf("hodhodCount=%d", stats.HodhodCount)
	}
	found := false
	for _, row := range rows {
		if row.Username == "hodhod@only" && row.HodhodOnly {
			found = true
		}
	}
	if !found {
		t.Fatal("missing hodhod-only row")
	}
}

func TestMergePanelUsersPage_sourceBothFilter(t *testing.T) {
	panelUsers := []panels.UserInfo{
		{Username: "a@x", Enabled: true},
		{Username: "b@x", Enabled: true},
	}
	services := []db.Service{{PanelUsername: "a@x", AgentID: 1}}
	rows, _ := mergePanelUsersPage(panelUsers, services, map[int64]string{1: "A"}, nil, panelUserFilters{Status: "all"}, 1)
	for i := range rows {
		if rows[i].Username == "a@x" {
			rows[i].Source = "both"
		} else {
			rows[i].Source = "panel"
		}
	}
	var both []PanelUserRow
	for _, row := range rows {
		if row.Source == "both" {
			both = append(both, row)
		}
	}
	if len(both) != 1 {
		t.Fatalf("both=%d", len(both))
	}
}
