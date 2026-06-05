package panels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type marzbanClient struct {
	cfg    Config
	http   *http.Client
	mu     sync.Mutex
	token  string
	expiry time.Time
}

func newMarzban(cfg Config, httpClient *http.Client) *marzbanClient {
	return &marzbanClient{cfg: cfg, http: httpClient}
}

func (c *marzbanClient) Kind() PanelKind { return KindMarzban }

func (c *marzbanClient) base() string {
	return strings.TrimRight(c.cfg.BaseURL, "/")
}

func (c *marzbanClient) TestConnection(ctx context.Context) error {
	return c.auth(ctx)
}

func (c *marzbanClient) auth(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.expiry) {
		return nil
	}
	form := url.Values{}
	form.Set("username", c.cfg.Username)
	form.Set("password", c.cfg.Password)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+"/api/admin/token", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &PanelAuthError{Msg: fmt.Sprintf("status %d", resp.StatusCode)}
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	c.token = out.AccessToken
	c.expiry = time.Now().Add(50 * time.Minute)
	return nil
}

func (c *marzbanClient) do(ctx context.Context, method, path string, body io.Reader, out any) error {
	if err := c.auth(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base()+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		c.mu.Lock()
		c.token = ""
		c.mu.Unlock()
		if err := c.auth(ctx); err != nil {
			return err
		}
		return c.do(ctx, method, path, body, out)
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrUserNotFound
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("marzban: %s: %s", resp.Status, string(b))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *marzbanClient) CreateUser(ctx context.Context, req CreateUserRequest) (*UserInfo, error) {
	inbounds := req.Scope.Inbounds
	if len(inbounds) == 0 {
		inbounds = []string{}
	}
	payload := map[string]any{
		"username": req.Username,
		"status":   "active",
		"proxies": map[string]any{
			"vless": map[string]any{},
			"vmess": map[string]any{},
		},
		"inbounds": map[string]any{
			"vless": inbounds,
			"vmess": inbounds,
		},
	}
	if req.DataLimitBytes > 0 {
		payload["data_limit"] = req.DataLimitBytes
	}
	if !req.ExpireAt.IsZero() {
		payload["expire"] = req.ExpireAt.Unix()
	}
	if req.Note != "" {
		payload["note"] = req.Note
	}
	b, _ := json.Marshal(payload)
	var raw map[string]any
	if err := c.do(ctx, http.MethodPost, "/api/user", strings.NewReader(string(b)), &raw); err != nil {
		return nil, err
	}
	return c.mapUser(raw), nil
}

func (c *marzbanClient) GetUser(ctx context.Context, username string) (*UserInfo, error) {
	var raw map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/user/"+url.PathEscape(username), nil, &raw); err != nil {
		return nil, err
	}
	return c.mapUser(raw), nil
}

func (c *marzbanClient) mapUser(raw map[string]any) *UserInfo {
	u := &UserInfo{Raw: raw, Enabled: true}
	if v, ok := raw["username"].(string); ok {
		u.Username = v
	}
	if v, ok := raw["used_traffic"].(float64); ok {
		u.UsedBytes = int64(v)
	}
	if v, ok := raw["data_limit"].(float64); ok {
		u.DataLimitBytes = int64(v)
	}
	if v, ok := raw["expire"].(float64); ok && v > 0 {
		u.ExpireAt = time.Unix(int64(v), 0)
	}
	if v, ok := raw["status"].(string); ok {
		u.Enabled = v == "active"
	}
	if v, ok := raw["subscription_url"].(string); ok {
		u.SubscriptionURL = v
		if !strings.HasPrefix(v, "http") {
			u.SubscriptionURL = c.base() + v
		}
	}
	return u
}

func (c *marzbanClient) ResetUsage(ctx context.Context, username string) error {
	return c.do(ctx, http.MethodPost, "/api/user/"+url.PathEscape(username)+"/reset", nil, nil)
}

func (c *marzbanClient) Disable(ctx context.Context, username string) error {
	return c.updateStatus(ctx, username, "disabled")
}

func (c *marzbanClient) Enable(ctx context.Context, username string) error {
	return c.updateStatus(ctx, username, "active")
}

func (c *marzbanClient) updateStatus(ctx context.Context, username, status string) error {
	b, _ := json.Marshal(map[string]string{"status": status})
	return c.do(ctx, http.MethodPut, "/api/user/"+url.PathEscape(username), strings.NewReader(string(b)), nil)
}

func (c *marzbanClient) DeleteUser(ctx context.Context, username string) error {
	return c.do(ctx, http.MethodDelete, "/api/user/"+url.PathEscape(username), nil, nil)
}

func (c *marzbanClient) SubscriptionURL(ctx context.Context, username string) (string, error) {
	u, err := c.GetUser(ctx, username)
	if err != nil {
		return "", err
	}
	return u.SubscriptionURL, nil
}

func (c *marzbanClient) UpdateUser(ctx context.Context, username string, req UpdateUserRequest) (*UserInfo, error) {
	cur, err := c.GetUser(ctx, username)
	if err != nil {
		return nil, err
	}
	limit := cur.DataLimitBytes
	expire := cur.ExpireAt
	if req.DataLimitBytes != nil {
		limit = *req.DataLimitBytes
	}
	if req.AddBytes > 0 {
		limit += req.AddBytes
	}
	if req.ExpireAt != nil {
		expire = *req.ExpireAt
	} else if req.AddDays > 0 {
		base := time.Now()
		if !cur.ExpireAt.IsZero() && cur.ExpireAt.After(base) {
			base = cur.ExpireAt
		}
		expire = base.Add(time.Duration(req.AddDays) * 24 * time.Hour)
	}
	payload := map[string]any{}
	if limit > 0 {
		payload["data_limit"] = limit
	}
	if !expire.IsZero() {
		payload["expire"] = expire.Unix()
	}
	if req.Enabled != nil {
		if *req.Enabled {
			payload["status"] = "active"
		} else {
			payload["status"] = "disabled"
		}
	}
	b, _ := json.Marshal(payload)
	if err := c.do(ctx, http.MethodPut, "/api/user/"+url.PathEscape(username), strings.NewReader(string(b)), nil); err != nil {
		return nil, err
	}
	return c.GetUser(ctx, username)
}

func (c *marzbanClient) ListInbounds(ctx context.Context) ([]InboundInfo, error) {
	_ = ctx
	return nil, ErrUnsupported
}

func (c *marzbanClient) ListUsers(ctx context.Context) ([]UserInfo, error) {
	var raw struct {
		Users []map[string]any `json:"users"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/users?offset=0&limit=1000", nil, &raw); err != nil {
		return nil, err
	}
	out := make([]UserInfo, 0, len(raw.Users))
	for _, u := range raw.Users {
		out = append(out, *c.mapUser(u))
	}
	return out, nil
}
