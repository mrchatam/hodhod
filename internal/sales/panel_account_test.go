package sales

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
)

func TestBuildModifyUpdateRequest_expireAtPreferred(t *testing.T) {
	exp := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	days := 30
	req := buildModifyUpdateRequest(ModifyInput{
		ExpireAt:     &exp,
		DurationDays: &days,
		VolumeGB:     ptrInt(10),
	})
	if req.ExpireAt == nil || !req.ExpireAt.Equal(exp) {
		t.Fatalf("expire_at: got %v want %v", req.ExpireAt, exp)
	}
	if req.DataLimitBytes == nil || *req.DataLimitBytes != 10*1024*1024*1024 {
		t.Fatalf("volume: got %v", req.DataLimitBytes)
	}
}

func TestBuildModifyUpdateRequest_durationDays(t *testing.T) {
	days := 7
	req := buildModifyUpdateRequest(ModifyInput{DurationDays: &days})
	if req.ExpireAt == nil {
		t.Fatal("expected expire from duration")
	}
	if time.Until(*req.ExpireAt) < 6*24*time.Hour {
		t.Fatalf("expire too soon: %v", req.ExpireAt)
	}
}

func TestCreatePanelAccount_manualUsername(t *testing.T) {
	mock := &mockPanel{users: map[string]*panels.UserInfo{}}
	reg := panels.NewRegistry(nil, nil, nil)
	reg.InjectClient(42, mock)
	s := &Service{Panels: reg}
	_, err := s.CreatePanelAccount(context.Background(), CreatePanelAccountInput{
		CreateManualInput: CreateManualInput{
			PanelID: 42, InboundIDs: []int{1}, IsMaster: true,
		},
		ManualUsername: "custom-user",
		SkipPermCheck:  true,
		SkipAgentQuota: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := mock.users["custom-user"]; !ok {
		t.Fatal("panel user not created")
	}
}

func ptrInt(n int) *int { return &n }

func TestModifyPanelAccount_syncsService(t *testing.T) {
	mock := &mockPanel{users: map[string]*panels.UserInfo{
		"u1": {Username: "u1", DataLimitBytes: 1e9, Enabled: true},
	}}
	reg := panels.NewRegistry(nil, nil, nil)
	reg.InjectClient(1, mock)
	store := &panelTestStore{
		services: map[string]*db.Service{
			"1:u1": {ID: 10, AgentID: 1, PanelID: 1, PanelUsername: "u1", DataLimitBytes: 1e9, Status: "active"},
		},
	}
	s := &Service{Store: store, Panels: reg}
	gb := 20
	out, err := s.ModifyPanelAccount(context.Background(), ModifyPanelAccountInput{
		ModifyInput: ModifyInput{VolumeGB: &gb},
		PanelID:     1,
		Username:    "u1",
		IsMaster:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !store.updated {
		t.Fatal("expected service update")
	}
	if out.DataLimitBytes != 20*1024*1024*1024 {
		t.Fatalf("limit: got %d", out.DataLimitBytes)
	}
}

func TestModifyPanelAccount_noServiceRow(t *testing.T) {
	mock := &mockPanel{users: map[string]*panels.UserInfo{
		"orphan": {Username: "orphan", DataLimitBytes: 1e9, Enabled: true},
	}}
	reg := panels.NewRegistry(nil, nil, nil)
	reg.InjectClient(1, mock)
	store := &panelTestStore{services: map[string]*db.Service{}}
	s := &Service{Store: store, Panels: reg}
	gb := 5
	out, err := s.ModifyPanelAccount(context.Background(), ModifyPanelAccountInput{
		ModifyInput: ModifyInput{VolumeGB: &gb},
		PanelID:     1,
		Username:    "orphan",
		IsMaster:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Fatalf("expected nil service, got %+v", out)
	}
	if store.updated {
		t.Fatal("should not update db without service row")
	}
}

func TestCreatePanelAccount_agentQuotaDenied(t *testing.T) {
	mock := &mockPanel{users: map[string]*panels.UserInfo{}}
	reg := panels.NewRegistry(nil, nil, nil)
	reg.InjectClient(1, mock)
	store := &panelTestStore{
		agentPanel: &db.AgentPanel{QuotaBytes: 5 * 1024 * 1024 * 1024},
	}
	s := &Service{Store: store, Panels: reg}
	_, err := s.CreatePanelAccount(context.Background(), CreatePanelAccountInput{
		CreateManualInput: CreateManualInput{
			AgentID: 1, PanelID: 1, VolumeGB: 10, DurationDays: 30,
			InboundIDs: []int{1}, AdminID: 1, IsMaster: false,
		},
	})
	if err != ErrQuotaExceeded {
		t.Fatalf("got %v want ErrQuotaExceeded", err)
	}
}

func TestCreatePanelAccount_masterBypassQuota(t *testing.T) {
	mock := &mockPanel{users: map[string]*panels.UserInfo{}}
	reg := panels.NewRegistry(nil, nil, nil)
	reg.InjectClient(1, mock)
	store := &panelTestStore{
		agentPanel: &db.AgentPanel{QuotaBytes: 1 * 1024 * 1024 * 1024},
	}
	s := &Service{Store: store, Panels: reg}
	_, err := s.CreatePanelAccount(context.Background(), CreatePanelAccountInput{
		CreateManualInput: CreateManualInput{
			AgentID: 1, PanelID: 1, VolumeGB: 30, DurationDays: 30,
			InboundIDs: []int{1}, AdminID: 1, IsMaster: true,
		},
		ManualUsername: "big-user",
		SkipAgentQuota: true,
		SkipPermCheck:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.services) != 1 {
		t.Fatalf("expected service row, got %d", len(store.services))
	}
}

func TestProvisionFromOrder_unchanged(t *testing.T) {
	panelID := int64(7)
	mock := &mockPanel{users: map[string]*panels.UserInfo{}}
	reg := panels.NewRegistry(nil, nil, nil)
	reg.InjectClient(panelID, mock)
	store := &panelTestStore{
		plan: &db.Plan{ID: 1, VolumeGB: 10, DurationDays: 30, PanelID: &panelID},
		bot:  &db.Bot{ID: 2, AgentID: 3, PublicID: "mybot"},
		botPanels: []db.BotPanel{
			{BotID: 2, PanelID: panelID, ScopeJSON: scopeJSON([]int{1})},
		},
	}
	s := &Service{Store: store, Panels: reg}
	order := &db.Order{ID: 99, PlanID: 1, EndUserID: 5}
	svc, err := s.ProvisionFromOrder(context.Background(), 2, order)
	if err != nil {
		t.Fatal(err)
	}
	if svc.Source != db.ServiceSourceBot {
		t.Fatalf("source: %s", svc.Source)
	}
	if !strings.HasPrefix(svc.PanelUsername, "mybot-") {
		t.Fatalf("username pattern: %s", svc.PanelUsername)
	}
	if svc.BotID == nil || *svc.BotID != 2 {
		t.Fatalf("bot id: %v", svc.BotID)
	}
	if svc.OrderID == nil || *svc.OrderID != 99 {
		t.Fatalf("order id: %v", svc.OrderID)
	}
}

func TestSetPanelAccountEnabled_syncsService(t *testing.T) {
	mock := &mockPanel{users: map[string]*panels.UserInfo{
		"u1": {Username: "u1", DataLimitBytes: 1e9, Enabled: true},
	}}
	reg := panels.NewRegistry(nil, nil, nil)
	reg.InjectClient(1, mock)
	store := &panelTestStore{
		services: map[string]*db.Service{
			"1:u1": {ID: 1, PanelID: 1, PanelUsername: "u1", Status: "active"},
		},
	}
	s := &Service{Store: store, Panels: reg}
	if err := s.SetPanelAccountEnabled(context.Background(), 1, "u1", false); err != nil {
		t.Fatal(err)
	}
	if !store.updated {
		t.Fatal("expected db sync")
	}
	svc, _ := store.GetServiceByPanelUsername(context.Background(), 1, "u1")
	if svc.Status != "disabled" {
		t.Fatalf("status: %s", svc.Status)
	}
}
