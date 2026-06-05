package panels

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestMarzban_createAndGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/admin/token":
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/user":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"username": "u1", "used_traffic": float64(0), "data_limit": float64(1e9),
				"status": "active", "subscription_url": "https://sub.example/u1",
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/user/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"username": "u1", "used_traffic": float64(100), "data_limit": float64(1e9),
				"status": "active", "subscription_url": "https://sub.example/u1",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newMarzban(Config{BaseURL: srv.URL, Username: "a", Password: "b"}, srv.Client())
	info, err := c.CreateUser(context.Background(), CreateUserRequest{
		Username: "u1", DataLimitBytes: 1e9, ExpireAt: time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if info.SubscriptionURL == "" {
		t.Fatal("expected sub url")
	}
	got, err := c.GetUser(context.Background(), "u1")
	if err != nil || got.UsedBytes != 100 {
		t.Fatalf("get user: %v used=%d", err, got.UsedBytes)
	}
}

func TestMarzban_authRefresh(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/admin/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
			return
		}
		if r.URL.Path == "/api/user/x" {
			calls++
			if calls == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"username": "x"})
			return
		}
	}))
	defer srv.Close()
	c := newMarzban(Config{BaseURL: srv.URL, Username: "a", Password: "b"}, srv.Client())
	_, err := c.GetUser(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if calls < 2 {
		t.Fatalf("expected retry after 401, calls=%d", calls)
	}
}

func TestNew_unsupportedType(t *testing.T) {
	_, err := New(Config{Type: "unknown"}, http.DefaultClient)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMarzban_notFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/admin/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := newMarzban(Config{BaseURL: srv.URL, Username: "a", Password: "b"}, srv.Client())
	_, err := c.GetUser(context.Background(), "missing")
	if err != ErrUserNotFound {
		t.Fatalf("got %v", err)
	}
}

var _ = url.URL{}

func TestMarzban_updateUser(t *testing.T) {
	updated := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/admin/token":
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/user/"):
			limit := float64(1e9)
			if updated {
				limit = 2e9
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"username": "u1", "used_traffic": float64(0), "data_limit": limit,
				"status": "active", "subscription_url": "https://sub.example/u1",
			})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/user/"):
			updated = true
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newMarzban(Config{BaseURL: srv.URL, Username: "a", Password: "b"}, srv.Client())
	add := int64(1024 * 1024 * 1024)
	info, err := c.UpdateUser(context.Background(), "u1", UpdateUserRequest{AddBytes: add})
	if err != nil {
		t.Fatal(err)
	}
	if info.DataLimitBytes != int64(2e9) {
		t.Fatalf("limit=%d want %d", info.DataLimitBytes, int64(2e9))
	}
}
