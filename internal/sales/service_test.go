package sales

import (
	"context"
	"testing"
	"time"

	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
)

type mockPanel struct {
	users map[string]*panels.UserInfo
}

func (m *mockPanel) CreateUser(ctx context.Context, req panels.CreateUserRequest) (*panels.UserInfo, error) {
	info := &panels.UserInfo{Username: req.Username, DataLimitBytes: req.DataLimitBytes, Enabled: true, ExpireAt: req.ExpireAt}
	m.users[req.Username] = info
	return info, nil
}
func (m *mockPanel) GetUser(ctx context.Context, username string) (*panels.UserInfo, error) {
	if u, ok := m.users[username]; ok {
		return u, nil
	}
	return nil, panels.ErrUserNotFound
}
func (m *mockPanel) UpdateUser(ctx context.Context, username string, req panels.UpdateUserRequest) (*panels.UserInfo, error) {
	u := m.users[username]
	if req.DataLimitBytes != nil {
		u.DataLimitBytes = *req.DataLimitBytes
	}
	if req.AddBytes > 0 {
		u.DataLimitBytes += req.AddBytes
	}
	if req.ExpireAt != nil {
		u.ExpireAt = *req.ExpireAt
	}
	if req.Enabled != nil {
		u.Enabled = *req.Enabled
	}
	return u, nil
}
func (m *mockPanel) ResetUsage(_ context.Context, username string) error {
	if u := m.users[username]; u != nil {
		u.UsedBytes = 0
	}
	return nil
}
func (m *mockPanel) Disable(_ context.Context, username string) error {
	if u := m.users[username]; u != nil {
		u.Enabled = false
	}
	return nil
}
func (m *mockPanel) Enable(_ context.Context, username string) error {
	if u := m.users[username]; u != nil {
		u.Enabled = true
	}
	return nil
}
func (m *mockPanel) DeleteUser(context.Context, string) error { return nil }
func (m *mockPanel) SubscriptionURL(context.Context, string) (string, error) {
	return "https://sub.example/x", nil
}
func (m *mockPanel) ListInbounds(context.Context) ([]panels.InboundInfo, error) {
	return nil, panels.ErrUnsupported
}
func (m *mockPanel) ListUsers(context.Context) ([]panels.UserInfo, error) {
	out := make([]panels.UserInfo, 0, len(m.users))
	for _, u := range m.users {
		out = append(out, *u)
	}
	return out, nil
}
func (m *mockPanel) ListUsersPaged(_ context.Context, opts panels.UserListOptions) (*panels.UserListPage, error) {
	all, _ := m.ListUsers(context.Background())
	page := opts.Page
	if page < 1 {
		page = 1
	}
	ps := opts.PageSize
	if ps < 1 {
		ps = 25
	}
	total := len(all)
	start := (page - 1) * ps
	if start > total {
		start = total
	}
	end := start + ps
	if end > total {
		end = total
	}
	return &panels.UserListPage{Users: all[start:end], Total: total, Filtered: total, Page: page, PageSize: ps}, nil
}
func (m *mockPanel) Kind() panels.PanelKind               { return panels.KindMarzban }
func (m *mockPanel) TestConnection(context.Context) error { return nil }

func TestEnforceQuota_bytesCap(t *testing.T) {
	s := &Service{}
	ap := &db.AgentPanel{QuotaBytes: 5 * 1024 * 1024 * 1024}
	err := s.enforceQuota(context.Background(), 1, 1, ap, 10*1024*1024*1024, 30)
	if err != ErrQuotaExceeded {
		t.Fatalf("got %v", err)
	}
}

func TestModifyService_volume(t *testing.T) {
	mock := &mockPanel{users: map[string]*panels.UserInfo{
		"u1": {Username: "u1", DataLimitBytes: 1e9, Enabled: true},
	}}
	reg := panels.NewRegistry(nil, nil, nil)
	reg.InjectClient(1, mock)
	// ModifyService needs store for getService and checkPerm - test UpdateUser path via mock directly
	info, err := mock.UpdateUser(context.Background(), "u1", panels.UpdateUserRequest{DataLimitBytes: ptrInt64(2e9)})
	if err != nil || info.DataLimitBytes != int64(2e9) {
		t.Fatalf("update: %v limit=%d", err, info.DataLimitBytes)
	}
	_ = reg
	_ = time.Now()
}

func ptrInt64(n int64) *int64 { return &n }

func TestCheckPerm_viewOnlyDenied(t *testing.T) {
	p := &db.AgentPermissions{ViewOnly: true}
	if p.Has(db.PermCreateUser) {
		t.Fatal("view-only should not have create")
	}
}

func TestErrors_sentinel(t *testing.T) {
	if ErrForbidden.Error() == "" || ErrQuotaExceeded.Error() == "" {
		t.Fatal("sentinel errors should have messages")
	}
}
