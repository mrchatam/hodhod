package web

import (
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mrchatam/hodhod/internal/config"
	"github.com/mrchatam/hodhod/internal/crypto"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/domains"
	"github.com/mrchatam/hodhod/internal/panels"
	"github.com/mrchatam/hodhod/internal/sales"
	"github.com/mrchatam/hodhod/internal/telegram"
)

// Server is the admin web GUI.
type Server struct {
	Cfg            *config.Config
	Store          *db.Store
	Box            *crypto.Box
	Panels         *panels.Registry
	Telegram       *telegram.Manager
	Sales          *sales.Service
	DomainVerifier *domains.Verifier
	pages          map[string]*template.Template
	loginT         *template.Template
}

// NewServer creates a web server.
func NewServer(cfg *config.Config, store *db.Store, box *crypto.Box, reg *panels.Registry, tg *telegram.Manager, salesSvc *sales.Service) (*Server, error) {
	pages, loginT, err := parseTemplates()
	if err != nil {
		return nil, err
	}
	return &Server{
		Cfg: cfg, Store: store, Box: box, Panels: reg, Telegram: tg, Sales: salesSvc,
		DomainVerifier: &domains.Verifier{Resolver: domains.NetResolver{}},
		pages:          pages, loginT: loginT,
	}, nil
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, s.securityHeaders, RateLimitLogin)

	r.Get("/login", s.pageLogin)
	r.Post("/login", s.postLogin)
	r.Post("/logout", s.postLogout)

	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/", s.pageDashboard)
		r.Get("/onboarding", s.pageOnboarding)

		r.Get("/services", s.pageServices)
		r.Get("/services/new", s.pageServiceCreate)
		r.Post("/services", s.postService)
		r.Post("/services/{id}/modify", s.postServiceModify)
		r.Post("/services/{id}/add-time", s.postServiceAddTime)
		r.Post("/services/{id}/add-volume", s.postServiceAddVolume)
		r.Post("/services/{id}/disable", s.postServiceDisable)
		r.Post("/services/{id}/enable", s.postServiceEnable)
		r.Post("/services/{id}/reset", s.postServiceReset)
		r.Post("/services/{id}/delete", s.postServiceDelete)

		r.Get("/payments/pending", s.pagePendingPayments)
		r.Get("/payments/{botID}/{id}/receipt", s.getPaymentReceipt)
		r.Post("/payments/{botID}/{id}/approve", s.approvePayment)
		r.Post("/payments/{botID}/{id}/reject", s.rejectPayment)

		r.Route("/master", func(r chi.Router) {
			r.Use(s.requireMaster, s.requireMainHost)
			r.Get("/agents", s.pageAgents)
			r.Post("/agents", s.postAgent)
			r.Get("/agents/{id}", s.pageAgentEdit)
			r.Post("/agents/{id}", s.postAgentUpdate)
			r.Post("/agents/{id}/permissions", s.postAgentPermissions)
			r.Post("/agents/{id}/panels", s.postAgentPanel)
			r.Post("/agents/{id}/reset-password", s.postAgentResetPassword)
			r.Post("/agents/{id}/domain", s.postAgentDomain)
			r.Post("/agents/{id}/verify-domain", s.postAgentVerifyDomain)
			r.Post("/agents/{id}/domain-toggle", s.postAgentDomainToggle)
			r.Get("/panels", s.pagePanels)
			r.Post("/panels", s.postPanel)
			r.Get("/panels/{id}", s.pagePanelEdit)
			r.Post("/panels/{id}", s.postPanelUpdate)
			r.Post("/panels/{id}/disable", s.postPanelDisable)
			r.Post("/panels/{id}/test", s.testPanel)
			r.Get("/bots", s.pageBots)
			r.Post("/bots", s.postBot)
			r.Get("/bots/{id}/settings", s.pageBotSettings)
			r.Post("/bots/{id}/settings", s.postBotSettings)
			r.Post("/bots/{id}/bot-panels", s.postBotPanel)
		})

		r.Route("/agent", func(r chi.Router) {
			r.Use(s.requireAgent)
			r.Get("/plans", s.pageAgentPlans)
			r.Post("/plans", s.postAgentPlan)
			r.Post("/plans/{id}", s.postAgentPlanUpdate)
			r.Post("/plans/{id}/disable", s.postAgentPlanDisable)
			r.Get("/bots", s.pageAgentBots)
			r.Post("/bots", s.postAgentBot)
			r.Get("/bots/{id}/settings", s.pageAgentBotSettings)
			r.Post("/bots/{id}/settings", s.postAgentBotSettings)
		})
	})
	return r
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self' https://cdn.tailwindcss.com https://unpkg.com https://cdn.jsdelivr.net; script-src 'self' https://cdn.tailwindcss.com https://unpkg.com https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) canAccessBot(r *http.Request, admin *db.Admin, botID int64) bool {
	if admin.Role == db.RoleMaster {
		return true
	}
	if admin.AgentID == nil {
		return false
	}
	_, err := s.Store.GetBotForAgent(r.Context(), *admin.AgentID, botID)
	return err == nil
}
