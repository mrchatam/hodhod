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
		"limitIp":    req.LimitIP,
		"tgId":       0,
	}
	if req.Note != "" {
		clientObj["comment"] = req.Note
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

func (c *xuiClient) UpdateUser(ctx context.Context, username string, req UpdateUserRequest) (*UserInfo, error) {
	cur, err := c.GetUser(ctx, username)
	if err != nil {
		return nil, err
	}
	limit := cur.DataLimitBytes
	expire := cur.ExpireAt
	enable := cur.Enabled
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
	if req.Enabled != nil {
		enable = *req.Enabled
	}
	payload := map[string]any{
		"email":      username,
		"totalGB":    limit,
		"expiryTime": expire.UnixMilli(),
		"enable":     enable,
	}
	b, _ := json.Marshal(payload)
	if err := c.do(ctx, http.MethodPost, "/panel/api/clients/update/"+url.PathEscape(username), strings.NewReader(string(b)), nil); err != nil {
		return nil, err
	}
	return c.GetUser(ctx, username)
}

func (c *xuiClient) ListInbounds(ctx context.Context) ([]InboundInfo, error) {
	var raw []map[string]any
	if err := c.do(ctx, http.MethodGet, "/panel/api/inbounds/list", nil, &raw); err != nil {
		return nil, err
	}
	out := make([]InboundInfo, 0, len(raw))
	for _, item := range raw {
		info := InboundInfo{}
		if v, ok := item["id"].(float64); ok {
			info.ID = int(v)
		}
		if v, ok := item["tag"].(string); ok {
			info.Tag = v
		}
		if v, ok := item["port"].(float64); ok {
			info.Port = int(v)
		}
		out = append(out, info)
	}
	return out, nil
}

func (c *xuiClient) ListUsers(ctx context.Context) ([]UserInfo, error) {
	users, err := c.listUsersViaClientsAPI(ctx)
	if err == nil {
		return users, nil
	}
	return c.listUsersViaInbounds(ctx)
}

func (c *xuiClient) ListUsersPaged(ctx context.Context, opts UserListOptions) (*UserListPage, error) {
	page := opts.Page
	if page < 1 {
		page = 1
	}
	pageSize := opts.PageSize
	if pageSize < 1 {
		pageSize = 25
	}
	if pageSize > 200 {
		pageSize = 200
	}
	filter := ""
	switch opts.Status {
	case "disabled":
		filter = "deactive"
	case "expired":
		filter = "depleted"
	case "active":
		filter = "active"
	}
	q := url.Values{}
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("pageSize", fmt.Sprintf("%d", pageSize))
	q.Set("search", opts.Search)
	q.Set("filter", filter)
	q.Set("protocol", "")
	q.Set("sort", "expiryTime")
	q.Set("order", "descend")
	path := "/panel/api/clients/list/paged?" + q.Encode()
	var obj struct {
		Items    []map[string]any `json:"items"`
		Total    int              `json:"total"`
		Filtered int              `json:"filtered"`
		Page     int              `json:"page"`
		PageSize int              `json:"pageSize"`
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &obj); err != nil {
		// Fallback: load all and slice in memory.
		all, err2 := c.ListUsers(ctx)
		if err2 != nil {
			return nil, err
		}
		return sliceUserListPage(all, opts), nil
	}
	users := make([]UserInfo, 0, len(obj.Items))
	for _, cl := range obj.Items {
		u := xuiMapClientRow(cl)
		if u.Username != "" {
			users = append(users, u)
		}
	}
	return &UserListPage{
		Users: users, Total: obj.Total, Filtered: obj.Filtered,
		Page: obj.Page, PageSize: obj.PageSize,
	}, nil
}

func xuiMapClientRow(cl map[string]any) UserInfo {
	u := UserInfo{Enabled: true, Raw: cl}
	if v, ok := cl["email"].(string); ok {
		u.Username = v
	}
	if v, ok := cl["enable"].(bool); ok {
		u.Enabled = v
	}
	if v, ok := cl["totalGB"].(float64); ok {
		u.DataLimitBytes = int64(v)
	}
	if v, ok := cl["expiryTime"].(float64); ok && v > 0 {
		u.ExpireAt = time.UnixMilli(int64(v))
	}
	if v, ok := cl["limitIp"].(float64); ok {
		u.LimitIP = int(v)
	}
	if v, ok := cl["comment"].(string); ok {
		u.Note = v
	}
	u.InboundIDs = xuiParseIntSlice(cl["inboundIds"])
	if len(u.InboundIDs) > 0 {
		u.InboundID = u.InboundIDs[0]
	}
	if tr, ok := cl["traffic"].(map[string]any); ok {
		if v, ok := tr["up"].(float64); ok {
			u.UsedBytes += int64(v)
		}
		if v, ok := tr["down"].(float64); ok {
			u.UsedBytes += int64(v)
		}
	}
	return u
}

func sliceUserListPage(all []UserInfo, opts UserListOptions) *UserListPage {
	filtered := filterUsersInMemory(all, opts)
	page := opts.Page
	if page < 1 {
		page = 1
	}
	pageSize := opts.PageSize
	if pageSize < 1 {
		pageSize = 25
	}
	total := len(filtered)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return &UserListPage{
		Users: filtered[start:end], Total: total, Filtered: total,
		Page: page, PageSize: pageSize,
	}
}

func filterUsersInMemory(users []UserInfo, opts UserListOptions) []UserInfo {
	q := strings.ToLower(strings.TrimSpace(opts.Search))
	var out []UserInfo
	for _, u := range users {
		if q != "" && !strings.Contains(strings.ToLower(u.Username), q) && !strings.Contains(strings.ToLower(u.Note), q) {
			continue
		}
		switch opts.Status {
		case "disabled":
			if u.Enabled {
				continue
			}
		case "active":
			if !u.Enabled {
				continue
			}
			if !u.ExpireAt.IsZero() && u.ExpireAt.Before(time.Now()) {
				continue
			}
		case "expired":
			if u.ExpireAt.IsZero() || !u.ExpireAt.Before(time.Now()) {
				continue
			}
		}
		out = append(out, u)
	}
	return out
}

func (c *xuiClient) listUsersViaClientsAPI(ctx context.Context) ([]UserInfo, error) {
	var raw []map[string]any
	if err := c.do(ctx, http.MethodGet, "/panel/api/clients/list", nil, &raw); err != nil {
		return nil, err
	}
	out := make([]UserInfo, 0, len(raw))
	for _, cl := range raw {
		u := UserInfo{Enabled: true, Raw: cl}
		if v, ok := cl["email"].(string); ok {
			u.Username = v
		}
		if u.Username == "" {
			continue
		}
		if v, ok := cl["enable"].(bool); ok {
			u.Enabled = v
		}
		if v, ok := cl["totalGB"].(float64); ok {
			u.DataLimitBytes = int64(v)
		}
		if v, ok := cl["expiryTime"].(float64); ok && v > 0 {
			u.ExpireAt = time.UnixMilli(int64(v))
		}
		if v, ok := cl["limitIp"].(float64); ok {
			u.LimitIP = int(v)
		}
		if v, ok := cl["comment"].(string); ok {
			u.Note = v
		}
		u.InboundIDs = xuiParseIntSlice(cl["inboundIds"])
		if len(u.InboundIDs) > 0 {
			u.InboundID = u.InboundIDs[0]
		}
		if tr, ok := cl["traffic"].(map[string]any); ok {
			if v, ok := tr["up"].(float64); ok {
				u.UsedBytes += int64(v)
			}
			if v, ok := tr["down"].(float64); ok {
				u.UsedBytes += int64(v)
			}
		}
		out = append(out, u)
	}
	return out, nil
}

func (c *xuiClient) listUsersViaInbounds(ctx context.Context) ([]UserInfo, error) {
	var raw []map[string]any
	if err := c.do(ctx, http.MethodGet, "/panel/api/inbounds/list", nil, &raw); err != nil {
		return nil, err
	}
	byEmail := map[string]*UserInfo{}
	for _, inbound := range raw {
		inboundID := 0
		if v, ok := inbound["id"].(float64); ok {
			inboundID = int(v)
		}
		tag, _ := inbound["tag"].(string)
		statsByEmail := map[string]map[string]any{}
		if stats, ok := inbound["clientStats"].([]any); ok {
			for _, s := range stats {
				st, ok := s.(map[string]any)
				if !ok {
					continue
				}
				email, _ := st["email"].(string)
				if email != "" {
					statsByEmail[email] = st
				}
			}
		}
		for _, cl := range xuiParseSettingsClients(inbound["settings"]) {
			email, _ := cl["email"].(string)
			if email == "" {
				continue
			}
			u, ok := byEmail[email]
			if !ok {
				u = &UserInfo{Username: email, Enabled: true, Raw: cl}
				byEmail[email] = u
			}
			u.InboundIDs = appendUniqueInt(u.InboundIDs, inboundID)
			if u.InboundID == 0 {
				u.InboundID = inboundID
			}
			if tag != "" {
				u.InboundTags = appendUniqueStr(u.InboundTags, tag)
				if u.InboundTag == "" {
					u.InboundTag = tag
				}
			}
			if v, ok := cl["enable"].(bool); ok {
				u.Enabled = v
			}
			if v, ok := cl["totalGB"].(float64); ok && u.DataLimitBytes == 0 {
				u.DataLimitBytes = int64(v)
			}
			if v, ok := cl["expiryTime"].(float64); ok && v > 0 && u.ExpireAt.IsZero() {
				u.ExpireAt = time.UnixMilli(int64(v))
			}
			if v, ok := cl["limitIp"].(float64); ok {
				u.LimitIP = int(v)
			}
			if v, ok := cl["comment"].(string); ok {
				u.Note = v
			}
			if st, ok := statsByEmail[email]; ok {
				if v, ok := st["up"].(float64); ok {
					u.UsedBytes += int64(v)
				}
				if v, ok := st["down"].(float64); ok {
					u.UsedBytes += int64(v)
				}
			}
		}
	}
	out := make([]UserInfo, 0, len(byEmail))
	for _, u := range byEmail {
		out = append(out, *u)
	}
	return out, nil
}

func xuiParseSettingsClients(settingsRaw any) []map[string]any {
	switch v := settingsRaw.(type) {
	case string:
		if v == "" {
			return nil
		}
		var settings struct {
			Clients []map[string]any `json:"clients"`
		}
		if err := json.Unmarshal([]byte(v), &settings); err != nil {
			return nil
		}
		return settings.Clients
	case map[string]any:
		clientsRaw, ok := v["clients"]
		if !ok {
			return nil
		}
		switch cl := clientsRaw.(type) {
		case []any:
			out := make([]map[string]any, 0, len(cl))
			for _, item := range cl {
				if m, ok := item.(map[string]any); ok {
					out = append(out, m)
				}
			}
			return out
		case []map[string]any:
			return cl
		}
	}
	return nil
}

func xuiParseIntSlice(v any) []int {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]int, 0, len(arr))
	for _, item := range arr {
		if n, ok := item.(float64); ok {
			out = append(out, int(n))
		}
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

func (c *xuiClient) Backup(ctx context.Context) (string, []byte, error) {
	data, err := c.doRaw(ctx, http.MethodGet, "/panel/api/server/getDb", nil)
	if err != nil {
		return "", nil, err
	}
	name := fmt.Sprintf("xui-backup-%s.db", time.Now().Format("20060102-150405"))
	return name, data, nil
}

func (c *xuiClient) doRaw(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, err := c.doRawLocked(ctx, method, path, body)
	if err == nil {
		return data, nil
	}
	if c.cfg.APIToken != "" {
		return nil, err
	}
	if loginErr := c.login(ctx); loginErr != nil {
		return nil, loginErr
	}
	return c.doRawLocked(ctx, method, path, body)
}

func (c *xuiClient) doRawLocked(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.prefix()+path, body)
	if err != nil {
		return nil, err
	}
	if c.cfg.APIToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIToken)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, &PanelAuthError{Msg: "unauthorized"}
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("xui: status %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		var wrap xuiResp
		if err := json.Unmarshal(data, &wrap); err == nil && !wrap.Success {
			return nil, fmt.Errorf("xui: %s", wrap.Msg)
		}
	}
	if len(data) < 16 || string(data[:16]) != "SQLite format 3\x00" {
		if len(data) > 0 && data[0] == '{' {
			var wrap xuiResp
			if err := json.Unmarshal(data, &wrap); err == nil && wrap.Success && len(wrap.Obj) > 0 {
				return wrap.Obj, nil
			}
		}
	}
	return data, nil
}

func (c *xuiClient) ListOnlineUsernames(ctx context.Context) (map[string]bool, error) {
	var obj []string
	if err := c.do(ctx, http.MethodPost, "/panel/api/clients/onlines", nil, &obj); err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(obj))
	for _, email := range obj {
		out[email] = true
	}
	return out, nil
}

func (c *xuiClient) ListLastOnline(ctx context.Context) (map[string]time.Time, error) {
	var obj map[string]float64
	if err := c.do(ctx, http.MethodPost, "/panel/api/clients/lastOnline", nil, &obj); err != nil {
		return nil, err
	}
	out := make(map[string]time.Time, len(obj))
	for email, ts := range obj {
		if ts > 0 {
			out[email] = time.Unix(int64(ts), 0)
		}
	}
	return out, nil
}

func (c *xuiClient) ServerStatus(ctx context.Context) (*ServerStatusInfo, error) {
	var obj map[string]any
	if err := c.do(ctx, http.MethodGet, "/panel/api/server/status", nil, &obj); err != nil {
		return &ServerStatusInfo{Reachable: false, Err: err.Error()}, nil
	}
	info := &ServerStatusInfo{Reachable: true}
	if v, ok := obj["cpu"].(float64); ok {
		info.CPUPct = v
	}
	if mem, ok := obj["mem"].(map[string]any); ok {
		if v, ok := mem["current"].(float64); ok {
			info.MemUsed = int64(v)
		}
		if v, ok := mem["total"].(float64); ok {
			info.MemTotal = int64(v)
		}
	}
	if disk, ok := obj["disk"].(map[string]any); ok {
		if v, ok := disk["current"].(float64); ok {
			info.DiskUsed = int64(v)
		}
		if v, ok := disk["total"].(float64); ok {
			info.DiskTotal = int64(v)
		}
	}
	if net, ok := obj["netIO"].(map[string]any); ok {
		if v, ok := net["up"].(float64); ok {
			info.NetUp = int64(v)
		}
		if v, ok := net["down"].(float64); ok {
			info.NetDown = int64(v)
		}
	}
	if xray, ok := obj["xray"].(map[string]any); ok {
		if v, ok := xray["state"].(string); ok {
			info.XrayState = v
		}
		if v, ok := xray["version"].(string); ok {
			info.XrayVer = v
		}
	}
	if v, ok := obj["tcpCount"].(float64); ok {
		info.TCPCount = int(v)
	}
	if load, ok := obj["load"].(map[string]any); ok {
		if v, ok := load["load1"].(float64); ok {
			info.Load1 = v
		}
	}
	if v, ok := obj["online"].(float64); ok {
		info.Online = int(v)
	}
	return info, nil
}

func (c *xuiClient) DeleteDepletedClients(ctx context.Context) (int, error) {
	var obj struct {
		Deleted int `json:"deleted"`
	}
	if err := c.do(ctx, http.MethodPost, "/panel/api/clients/delDepleted", nil, &obj); err != nil {
		return 0, err
	}
	return obj.Deleted, nil
}
