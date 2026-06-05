package panels

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/mrchatam/hodhod/internal/crypto"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/httpx"
)

// Registry caches panel clients by panel ID.
type Registry struct {
	mu    sync.RWMutex
	box   *crypto.Box
	http  *http.Client
	store *db.Store
	cache map[int64]Client
}

// NewRegistry creates a panel client registry.
func NewRegistry(box *crypto.Box, httpClient *http.Client, store *db.Store) *Registry {
	return &Registry{
		box: box, http: httpClient, store: store, cache: make(map[int64]Client),
	}
}

// Get returns a panel client, building it on first use.
func (r *Registry) Get(ctx context.Context, panelID int64) (Client, error) {
	r.mu.RLock()
	if c, ok := r.cache[panelID]; ok {
		r.mu.RUnlock()
		return c, nil
	}
	r.mu.RUnlock()

	p, err := r.store.GetPanel(ctx, panelID)
	if err != nil {
		return nil, err
	}
	pass, err := r.box.Decrypt(p.PasswordEnc)
	if err != nil {
		return nil, fmt.Errorf("decrypt panel password: %w", err)
	}
	apiToken := ""
	if p.APITokenEnc != "" {
		apiToken, err = r.box.Decrypt(p.APITokenEnc)
		if err != nil {
			return nil, fmt.Errorf("decrypt panel api token: %w", err)
		}
	}
	var extra map[string]any
	_ = json.Unmarshal(p.ExtraJSON, &extra)
	httpClient := r.http
	if extra != nil {
		if proxyURL, ok := extra["socks_proxy"].(string); ok && proxyURL != "" {
			if hc, err := httpx.New(httpx.Config{ProxyURL: proxyURL}); err == nil {
				httpClient = hc
			}
		}
	}
	cfg := Config{
		Type:     PanelKind(p.Type),
		BaseURL:  p.BaseURL,
		BasePath: p.BasePath,
		Username: p.Username,
		Password: pass,
		APIToken: apiToken,
		Extra:    extra,
	}
	client, err := New(cfg, httpClient)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.cache[panelID] = client
	r.mu.Unlock()
	return client, nil
}

// Invalidate removes a cached client (after panel update).
func (r *Registry) Invalidate(panelID int64) {
	r.mu.Lock()
	delete(r.cache, panelID)
	r.mu.Unlock()
}

// InjectClient installs a prebuilt client (tests only).
func (r *Registry) InjectClient(panelID int64, c Client) {
	r.mu.Lock()
	if r.cache == nil {
		r.cache = make(map[int64]Client)
	}
	r.cache[panelID] = c
	r.mu.Unlock()
}

// TestConnection verifies panel credentials.
func (r *Registry) TestConnection(ctx context.Context, panelID int64) error {
	c, err := r.Get(ctx, panelID)
	if err != nil {
		return err
	}
	return c.TestConnection(ctx)
}

// Backup downloads a native panel database when the client supports it.
func (r *Registry) Backup(ctx context.Context, panelID int64) (filename string, data []byte, err error) {
	c, err := r.Get(ctx, panelID)
	if err != nil {
		return "", nil, err
	}
	b, ok := c.(Backuper)
	if !ok {
		return "", nil, fmt.Errorf("panels: backup not supported for this panel type")
	}
	return b.Backup(ctx)
}
