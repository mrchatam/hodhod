package web

import (
	"testing"
	"time"

	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
)

func TestMergePanelUsers(t *testing.T) {
	panelUsers := []panels.UserInfo{
		{Username: "a@test", Enabled: true, DataLimitBytes: 1e9, InboundIDs: []int{1}},
		{Username: "b@test", Enabled: true, UsedBytes: 100},
	}
	services := []db.Service{
		{ID: 10, AgentID: 2, PanelUsername: "a@test", Label: "Customer A", Status: "active", DataLimitBytes: 1e9},
		{ID: 11, AgentID: 2, PanelUsername: "c@test", Label: "Hodhod only", Status: "active"},
	}
	agents := map[int64]string{2: "Agent Two"}
	inbounds := []panels.InboundInfo{{ID: 1, Tag: "vless-443"}}
	rows, stats := mergePanelUsers(panelUsers, services, agents, inbounds, panelUserFilters{Status: "all"})
	if stats.PanelCount != 2 || stats.HodhodCount != 2 {
		t.Fatalf("stats=%+v", stats)
	}
	if len(rows) != 3 {
		t.Fatalf("rows=%d", len(rows))
	}
	byUser := map[string]PanelUserRow{}
	for _, r := range rows {
		byUser[r.Username] = r
	}
	if byUser["a@test"].Source != "both" || byUser["a@test"].Label != "Customer A" {
		t.Fatalf("a@test=%+v", byUser["a@test"])
	}
	if !byUser["c@test"].HodhodOnly {
		t.Fatal("expected hodhod-only row")
	}
}

func TestMatchPanelUserFilters_source(t *testing.T) {
	row := PanelUserRow{Username: "x", Source: "panel", Status: "active"}
	if !matchPanelUserFilters(row, panelUserFilters{Source: "panel", Status: "all"}) {
		t.Fatal("expected panel match")
	}
	if matchPanelUserFilters(row, panelUserFilters{Source: "hodhod", Status: "all"}) {
		t.Fatal("expected no hodhod match")
	}
}

func TestMatchPanelUserFilters_inbound(t *testing.T) {
	row := PanelUserRow{Username: "x", InboundIDs: []int{1, 3}, Status: "active"}
	if !matchPanelUserFilters(row, panelUserFilters{Inbound: "3", Status: "all"}) {
		t.Fatal("expected inbound match")
	}
	if matchPanelUserFilters(row, panelUserFilters{Inbound: "9", Status: "all"}) {
		t.Fatal("expected no inbound match")
	}
}

func TestMatchPanelUserFilters_expired(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour)
	row := PanelUserRow{Username: "x", Status: "expired", ExpireAt: past}
	if !matchPanelUserFilters(row, panelUserFilters{Status: "expired"}) {
		t.Fatal("expected expired match")
	}
}

func TestUserCreateTemplateRoundTrip(t *testing.T) {
	// unit test validation only (no DB)
	tpl := UserCreateTemplate{Name: "Trial", VolumeGB: 5, DurationDays: 7, InboundIDs: []int{1, 2}}
	if tpl.Name == "" || len(tpl.InboundIDs) != 2 {
		t.Fatal("template struct mismatch")
	}
}

func TestFormatInboundLabels(t *testing.T) {
	got := formatInboundLabels([]int{1, 3}, nil, map[int]string{1: "a", 3: "b"})
	if got != "1:a, 3:b" {
		t.Fatalf("got %q", got)
	}
}
