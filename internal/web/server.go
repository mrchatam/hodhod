package web

import (
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mrchatam/hodhod/internal/backup"
	"github.com/mrchatam/hodhod/internal/billing"
	"github.com/mrchatam/hodhod/internal/botconfig"
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
	Backup         *backup.Service
	DomainVerifier *domains.Verifier
	BotSvc         *botconfig.BotService
	BotReader      *botconfig.Reader
	CardPick       *botconfig.CardPicker
	Review         *billing.PaymentReviewService
	Orders         *billing.OrderService
	Wallet         *billing.WalletService
	pages          map[string]*template.Template
	loginT         *template.Template
}

// NewServer creates a web server.
func NewServer(cfg *config.Config, store *db.Store, box *crypto.Box, reg *panels.Registry, tg *telegram.Manager, salesSvc *sales.Service, backupSvc *backup.Service, wallet *billing.WalletService, orders *billing.OrderService, review *billing.PaymentReviewService) (*Server, error) {
	pages, loginT, err := parseTemplates()
	if err != nil {
		return nil, err
	}
	reader := &botconfig.Reader{Store: store}
	return &Server{
		Cfg: cfg, Store: store, Box: box, Panels: reg, Telegram: tg, Sales: salesSvc, Backup: backupSvc,
		DomainVerifier: &domains.Verifier{Resolver: domains.NetResolver{}},
		BotSvc:         &botconfig.BotService{Store: store},
		BotReader:      reader,
		CardPick:       &botconfig.CardPicker{Store: store},
		Review:         review,
		Orders:         orders,
		Wallet:         wallet,
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
	r.Handle("/static/*", s.staticHandler())

	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Post("/account/lang", s.postAccountLang)
		r.Post("/account/theme", s.postAccountTheme)
		r.Get("/account/password", s.pageAccountPassword)
		r.Post("/account/password", s.postAccountPassword)
		r.Get("/", s.pageDashboard)
		r.Get("/onboarding", s.pageOnboarding)

		r.Get("/services", s.pageServices)
		r.Get("/services/new", s.pageServiceCreate)
		r.Post("/services", s.postService)
		r.Get("/customers", s.pageCustomers)
		r.Get("/customers/new", s.pageCustomerCreate)
		r.Post("/customers", s.postService)
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
			r.Post("/agents/{id}/access", s.postAgentAccess)
			r.Post("/agents/{id}/attach-inbound-users", s.postAgentAttachInboundUsers)
			r.Post("/agents/{id}/reset-password", s.postAgentResetPassword)
			r.Post("/agents/{id}/domain", s.postAgentDomain)
			r.Post("/agents/{id}/verify-domain", s.postAgentVerifyDomain)
			r.Post("/agents/{id}/domain-toggle", s.postAgentDomainToggle)
			r.Get("/panels", s.pagePanels)
			r.Post("/panels", s.postPanel)
			r.Get("/panels/{id}", s.pagePanelEdit)
			r.Post("/panels/{id}", s.postPanelUpdate)
			r.Post("/panels/{id}/agents", s.postPanelAgent)
			r.Post("/panels/{id}/bots", s.postPanelBot)
			r.Post("/panels/{id}/bots/{bot_id}/delete", s.postPanelBotDelete)
			r.Post("/panels/{id}/disable", s.postPanelDisable)
			r.Post("/panels/{id}/test", s.testPanel)
			r.Get("/panels/{id}/users", s.pagePanelUsers)
			r.Post("/panels/{id}/users", s.postPanelUser)
			r.Post("/panels/{id}/users/{email}/modify", s.postPanelUserModify)
			r.Post("/panels/{id}/users/{email}/reset", s.postPanelUserReset)
			r.Post("/panels/{id}/users/{email}/disable", s.postPanelUserDisable)
			r.Post("/panels/{id}/users/{email}/enable", s.postPanelUserEnable)
			r.Post("/panels/{id}/users/{email}/attach-agent", s.postPanelUserAttachAgent)
			r.Post("/panels/{id}/users/del-depleted", s.postPanelUsersDelDepleted)
			r.Post("/panels/{id}/users/templates", s.postPanelUserTemplate)
			r.Post("/panels/{id}/users/templates/{name}/delete", s.postPanelUserTemplateDelete)
			r.Get("/panels/{id}/backups", s.pagePanelBackups)
			r.Post("/panels/{id}/backups/run", s.postPanelBackupRun)
			r.Get("/panels/{id}/backups/{backupID}/download", s.getPanelBackupDownload)
			r.Post("/panels/{id}/backups/settings", s.postPanelBackupSettings)
			r.Get("/bots", s.pageBots)
			r.Post("/bots", s.postBot)
			r.Get("/bots/{id}/settings", s.pageBotSettings)
			r.Post("/bots/{id}/settings", s.postBotSettings)
			r.Post("/bots/{id}/delete", s.postBotDelete)
			r.Post("/bots/{id}/bot-panels", s.postBotPanel)
			r.Post("/bots/{id}/bot-panels/{panel_id}/delete", s.postBotPanelDelete)
			r.Post("/bots/{id}/cards", s.postBotCard)
			r.Post("/bots/{id}/cards/{cid}/delete", s.postBotCardDelete)
			r.Post("/bots/{id}/cards/{cid}", s.postBotCardUpdate)
			r.Post("/bots/{id}/channels", s.postBotChannel)
			r.Post("/bots/{id}/channels/{cid}/delete", s.postBotChannelDelete)
			r.Get("/bots/{id}/users", s.pageBotUsers)
			r.Get("/bots/{id}/users/{uid}", s.pageBotUserDetail)
			r.Post("/bots/{id}/users/{uid}/block", s.postBotUserBlock)
			r.Post("/bots/{id}/users/{uid}/unblock", s.postBotUserUnblock)
			r.Post("/bots/{id}/users/{uid}/wallet/adjust", s.postBotUserWalletAdjust)
		})

		r.Route("/agent", func(r chi.Router) {
			r.Use(s.requireAgent)
			r.Get("/plans", s.pageAgentPlans)
			r.Post("/plans", s.postAgentPlan)
			r.Post("/plans/{id}", s.postAgentPlanUpdate)
			r.Post("/plans/{id}/disable", s.postAgentPlanDisable)
			r.Get("/bots", s.pageBots)
			r.Post("/bots", s.postBot)
			r.Get("/bots/{id}/settings", s.pageBotSettings)
			r.Post("/bots/{id}/settings", s.postBotSettings)
			r.Post("/bots/{id}/cards", s.postBotCard)
			r.Post("/bots/{id}/cards/{cid}/delete", s.postBotCardDelete)
			r.Post("/bots/{id}/cards/{cid}", s.postBotCardUpdate)
			r.Post("/bots/{id}/channels", s.postBotChannel)
			r.Post("/bots/{id}/channels/{cid}/delete", s.postBotChannelDelete)
			r.Get("/bots/{id}/users", s.pageBotUsers)
			r.Get("/bots/{id}/users/{uid}", s.pageBotUserDetail)
			r.Post("/bots/{id}/users/{uid}/block", s.postBotUserBlock)
			r.Post("/bots/{id}/users/{uid}/unblock", s.postBotUserUnblock)
			r.Post("/bots/{id}/users/{uid}/wallet/adjust", s.postBotUserWalletAdjust)
			r.Get("/panels", s.pageAgentPanels)
			r.Get("/panels/{id}/customers", s.pageAgentPanelCustomers)
			r.Post("/panels/{id}/users", s.postAgentPanelUser)
			r.Post("/panels/{id}/users/{email}/modify", s.postAgentPanelUserModify)
			r.Post("/panels/{id}/users/{email}/reset", s.postAgentPanelUserReset)
			r.Post("/panels/{id}/users/{email}/disable", s.postAgentPanelUserDisable)
			r.Post("/panels/{id}/users/{email}/enable", s.postAgentPanelUserEnable)
		})
	})
	return r
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' https://unpkg.com https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
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
