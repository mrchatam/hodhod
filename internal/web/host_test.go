package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mrchatam/hodhod/internal/config"
)

func TestHostMiddleware_mainHost(t *testing.T) {
	cfg := &config.Config{PublicBaseURL: "https://admin.example.com", AllowCustomDomains: true}
	s := &Server{Cfg: cfg}
	called := false
	h := s.HostMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if hk, _ := r.Context().Value(ctxHostKind).(hostKind); hk != hostMain {
			t.Fatalf("host kind %v", hk)
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "admin.example.com"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called {
		t.Fatal("handler not called")
	}
}

func TestRequireMainHost_blocksAgentHost(t *testing.T) {
	s := &Server{}
	h := s.requireMainHost(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/master/agents", nil)
	ctx := context.WithValue(req.Context(), ctxHostKind, hostAgent)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestConfigMainHost(t *testing.T) {
	cfg := &config.Config{PublicBaseURL: "https://admin.example.com:443"}
	if cfg.MainHost() != "admin.example.com" {
		t.Fatalf("got %q", cfg.MainHost())
	}
}
