package panels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

type xuiClient struct {
	cfg  Config
	http *http.Client
	mu   sync.Mutex
}

func newXUI(cfg Config, httpClient *http.Client) *xuiClient {
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{
		Timeout:   httpClient.Timeout,
		Transport: httpClient.Transport,
		Jar:       jar,
	}
	return &xuiClient{cfg: cfg, http: hc}
}

func (c *xuiClient) Kind() PanelKind { return KindXUI }

func (c *xuiClient) prefix() string {
	return strings.TrimRight(c.cfg.BaseURL, "/") + c.cfg.BasePath
}

func (c *xuiClient) TestConnection(ctx context.Context) error {
	_, err := c.GetUser(ctx, "__hodhod_healthcheck__")
	if err == nil || err == ErrUserNotFound {
		return nil
	}
	return err
}

func (c *xuiClient) login(ctx context.Context) error {
	payload := map[string]string{
		"username":      c.cfg.Username,
		"password":      c.cfg.Password,
		"twoFactorCode": "",
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.prefix()+"/login", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &PanelAuthError{Msg: fmt.Sprintf("login status %d", resp.StatusCode)}
	}
	var wrap xuiResp
	if err := json.NewDecoder(resp.Body).Decode(&wrap); err != nil {
		return err
	}
	if !wrap.Success {
		return &PanelAuthError{Msg: wrap.Msg}
	}
	return nil
}

type xuiResp struct {
	Success bool            `json:"success"`
	Msg     string          `json:"msg"`
	Obj     json.RawMessage `json:"obj"`
}

func (c *xuiClient) do(ctx context.Context, method, path string, body io.Reader, out any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.doLocked(ctx, method, path, body, out)
}

func (c *xuiClient) doLocked(ctx context.Context, method, path string, body io.Reader, out any) error {
	var lastErr error
	attempts := 1
	if method == http.MethodGet {
		attempts = 3
	}
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(time.Duration(i*200) * time.Millisecond)
		}
		if err := c.doOnce(ctx, method, path, body, out); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

func (c *xuiClient) doOnce(ctx context.Context, method, path string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.prefix()+path, body)
	if err != nil {
		return err
	}
	if c.cfg.APIToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIToken)
	} else if err := c.login(ctx); err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized && c.cfg.APIToken == "" {
		if err := c.login(ctx); err != nil {
			return err
		}
		req2, _ := http.NewRequestWithContext(ctx, method, c.prefix()+path, body)
		if body != nil {
			req2.Header.Set("Content-Type", "application/json")
		}
		resp2, err := c.http.Do(req2)
		if err != nil {
			return err
		}
		defer resp2.Body.Close()
		resp = resp2
	}
	var wrap xuiResp
	if err := json.NewDecoder(resp.Body).Decode(&wrap); err != nil {
		return err
	}
	if !wrap.Success {
		if strings.Contains(strings.ToLower(wrap.Msg), "not found") {
			return ErrUserNotFound
		}
		return fmt.Errorf("xui: %s", wrap.Msg)
	}
	if out != nil && len(wrap.Obj) > 0 {
		return json.Unmarshal(wrap.Obj, out)
	}
	return nil
}

func (c *xuiClient) CreateUser(ctx context.Context, req CreateUserRequest) (*UserInfo, error) {
	inboundIDs := req.Scope.InboundIDs
	if len(inboundIDs) == 0 && req.Scope.InboundID > 0 {
		inboundIDs = []int{req.Scope.InboundID}
	}
	if len(inboundIDs) == 0 {
		inboundIDs = []int{1}
	}
	expiry := int64(0)
	if !req.ExpireAt.IsZero() {
		expiry = req.ExpireAt.UnixMilli()
	}
	clientObj := map[string]any{
		"email":      req.Username,
		"enable":     true,
		"expiryTime": expiry,
		"totalGB":    req.DataLimitBytes,
		"limitIp":    0,
		"tgId":       0,
	}
	payload := map[string]any{
		"client":     clientObj,
		"inboundIds": inboundIDs,
	}
	b, _ := json.Marshal(payload)
	if err := c.do(ctx, http.MethodPost, "/panel/api/clients/add", strings.NewReader(string(b)), nil); err != nil {
		return nil, err
	}
	return c.GetUser(ctx, req.Username)
}

func (c *xuiClient) GetUser(ctx context.Context, username string) (*UserInfo, error) {
	var obj map[string]any
	path := "/panel/api/clients/traffic/" + url.PathEscape(username)
	if err := c.do(ctx, http.MethodGet, path, nil, &obj); err != nil {
		return nil, err
	}
	u := &UserInfo{Username: username, Enabled: true, Raw: obj}
	if v, ok := obj["up"].(float64); ok {
		u.UsedBytes += int64(v)
	}
	if v, ok := obj["down"].(float64); ok {
		u.UsedBytes += int64(v)
	}
	if v, ok := obj["total"].(float64); ok {
		u.DataLimitBytes = int64(v)
	}
	if v, ok := obj["expiryTime"].(float64); ok && v > 0 {
		u.ExpireAt = time.UnixMilli(int64(v))
	}
	u.SubscriptionURL, _ = c.fetchLinks(ctx, username)
	return u, nil
}

func (c *xuiClient) fetchLinks(ctx context.Context, username string) (string, error) {
	var links []string
	if err := c.do(ctx, http.MethodGet, "/panel/api/clients/links/"+url.PathEscape(username), nil, &links); err != nil {
		return "", err
	}
	if len(links) == 0 {
		return "", fmt.Errorf("xui: no subscription links")
	}
	return links[0], nil
}

func (c *xuiClient) ResetUsage(ctx context.Context, username string) error {
	path := "/panel/api/clients/resetTraffic/" + url.PathEscape(username)
	return c.do(ctx, http.MethodPost, path, nil, nil)
}

func (c *xuiClient) Disable(ctx context.Context, username string) error {
	return c.updateEnable(ctx, username, false)
}

func (c *xuiClient) Enable(ctx context.Context, username string) error {
	return c.updateEnable(ctx, username, true)
}

func (c *xuiClient) DeleteUser(ctx context.Context, username string) error {
	path := "/panel/api/clients/del/" + url.PathEscape(username) + "?keepTraffic=0"
	return c.do(ctx, http.MethodPost, path, nil, nil)
}

func (c *xuiClient) SubscriptionURL(ctx context.Context, username string) (string, error) {
	return c.fetchLinks(ctx, username)
}

func (c *xuiClient) updateEnable(ctx context.Context, username string, enable bool) error {
	u, err := c.GetUser(ctx, username)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"email":      username,
		"totalGB":    u.DataLimitBytes,
		"expiryTime": u.ExpireAt.UnixMilli(),
		"enable":     enable,
	}
	b, _ := json.Marshal(payload)
	return c.do(ctx, http.MethodPost, "/panel/api/clients/update/"+url.PathEscape(username), strings.NewReader(string(b)), nil)
}
