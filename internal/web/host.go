package web

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/mrchatam/hodhod/internal/db"
)

type hostKind string

const (
	hostMain  hostKind = "main"
	hostAgent hostKind = "agent"
)

const (
	ctxHostKind    ctxKey = "hostKind"
	ctxHostAgentID ctxKey = "hostAgentID"
)

// HostMiddleware resolves the request host to main or agent-branded context.
func (s *Server) HostMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		reqHost := normalizeHost(r.Host)
		mainHost := s.Cfg.MainHost()
		ctx := r.Context()
		if hostsMatch(reqHost, mainHost) {
			ctx = context.WithValue(ctx, ctxHostKind, hostMain)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if !s.Cfg.AllowCustomDomains {
			http.NotFound(w, r)
			return
		}
		agent, err := s.Store.GetAgentByDomain(ctx, reqHost)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		ctx = context.WithValue(ctx, ctxHostKind, hostAgent)
		ctx = context.WithValue(ctx, ctxHostAgentID, agent.ID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func hostsMatch(reqHost, mainHost string) bool {
	if reqHost == "" || mainHost == "" {
		return reqHost == mainHost
	}
	if isLoopbackHost(reqHost) {
		return true
	}
	return stripWWW(reqHost) == stripWWW(mainHost)
}

func stripWWW(host string) string {
	return strings.TrimPrefix(host, "www.")
}

func isLoopbackHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.TrimSuffix(host, ".")
}

func (s *Server) requireMainHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hk, _ := r.Context().Value(ctxHostKind).(hostKind); hk == hostAgent {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// AgentPublicURL returns HTTPS base URL for an agent's branded panel when enabled.
func (s *Server) AgentPublicURL(ctx context.Context, agent *db.Agent) string {
	if agent != nil && agent.DomainEnabled && agent.DomainVerifiedAt != nil && db.AgentDomain(agent) != "" {
		return "https://" + db.AgentDomain(agent)
	}
	return s.Cfg.PublicBaseURL
}
