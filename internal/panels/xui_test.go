package panels

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestXUI_bearerTrafficAndLinks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case strings.Contains(r.URL.Path, "/panel/api/clients/traffic/"):
			_ = json.NewEncoder(w).Encode(xuiResp{
				Success: true,
				Obj:     mustJSON(map[string]any{"up": float64(10), "down": float64(20), "total": float64(1e9)}),
			})
		case strings.Contains(r.URL.Path, "/panel/api/clients/links/"):
			_ = json.NewEncoder(w).Encode(xuiResp{
				Success: true,
				Obj:     mustJSON([]string{"vless://example"}),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newXUI(Config{BaseURL: srv.URL, APIToken: "test-token"}, srv.Client())
	info, err := c.GetUser(context.Background(), "user@test")
	if err != nil {
		t.Fatal(err)
	}
	if info.UsedBytes != 30 {
		t.Fatalf("used=%d", info.UsedBytes)
	}
	if info.SubscriptionURL != "vless://example" {
		t.Fatalf("sub=%q", info.SubscriptionURL)
	}
}

func TestXUI_jsonLoginAndCreate(t *testing.T) {
	var sawLogin bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/login":
			sawLogin = true
			_ = json.NewEncoder(w).Encode(xuiResp{Success: true, Msg: "ok"})
		case r.URL.Path == "/panel/api/clients/add":
			_ = json.NewEncoder(w).Encode(xuiResp{Success: true, Msg: "added"})
		case strings.Contains(r.URL.Path, "/panel/api/clients/traffic/"):
			_ = json.NewEncoder(w).Encode(xuiResp{
				Success: true,
				Obj:     mustJSON(map[string]any{"up": float64(0), "down": float64(0), "total": float64(1e9)}),
			})
		case strings.Contains(r.URL.Path, "/panel/api/clients/links/"):
			_ = json.NewEncoder(w).Encode(xuiResp{
				Success: true,
				Obj:     mustJSON([]string{"vmess://example"}),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newXUI(Config{BaseURL: srv.URL, Username: "admin", Password: "pass"}, srv.Client())
	_, err := c.CreateUser(context.Background(), CreateUserRequest{
		Username:       "alice@test",
		DataLimitBytes: 1e9,
		Scope:          Scope{InboundIDs: []int{3}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawLogin {
		t.Fatal("expected json login")
	}
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
