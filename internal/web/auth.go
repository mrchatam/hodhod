package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/mrchatam/hodhod/internal/db"
)

type ctxKey string

const (
	ctxAdmin ctxKey = "admin"
	ctxCSRF  ctxKey = "csrf"
)

// SessionCookieName is the admin session cookie.
const SessionCookieName = "hodhod_session"

func newSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Server) createSession(ctx context.Context, admin *db.Admin) (string, error) {
	id, err := newSessionID()
	if err != nil {
		return "", err
	}
	csrf, _ := newSessionID()
	sess := &db.Session{
		ID:        id,
		AdminID:   admin.ID,
		CSRFToken: csrf,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := s.Store.CreateSession(ctx, sess); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Server) sessionAdmin(r *http.Request) (*db.Admin, string, bool) {
	c, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, "", false
	}
	sess, err := s.Store.GetSession(r.Context(), c.Value)
	if err != nil {
		return nil, "", false
	}
	admin, err := s.Store.GetAdmin(r.Context(), sess.AdminID)
	if err != nil {
		return nil, "", false
	}
	return admin, sess.CSRFToken, true
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		admin, csrf, ok := s.sessionAdmin(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if isMutatingMethod(r.Method) {
			_ = r.ParseForm()
			token := r.FormValue("_csrf")
			if token == "" {
				token = r.Header.Get("X-CSRF-Token")
			}
			if token == "" || token != csrf {
				http.Error(w, "bad csrf token", http.StatusForbidden)
				return
			}
		}
		ctx := context.WithValue(r.Context(), ctxAdmin, admin)
		ctx = context.WithValue(ctx, ctxCSRF, csrf)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func isMutatingMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func (s *Server) requireMaster(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		admin := r.Context().Value(ctxAdmin).(*db.Admin)
		if admin.Role != db.RoleMaster {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
