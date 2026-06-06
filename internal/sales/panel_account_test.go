package sales

import (
	"context"
	"testing"
	"time"

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
