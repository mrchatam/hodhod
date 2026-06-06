package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/mrchatam/hodhod/internal/backup"
	"github.com/mrchatam/hodhod/internal/billing"
	"github.com/mrchatam/hodhod/internal/botconfig"
	"github.com/mrchatam/hodhod/internal/config"
	"github.com/mrchatam/hodhod/internal/crypto"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/db/migrate"
	"github.com/mrchatam/hodhod/internal/httpx"
	"github.com/mrchatam/hodhod/internal/logging"
	"github.com/mrchatam/hodhod/internal/miniapp"
	"github.com/mrchatam/hodhod/internal/panels"
	"github.com/mrchatam/hodhod/internal/provisioning"
	"github.com/mrchatam/hodhod/internal/sales"
	"github.com/mrchatam/hodhod/internal/scheduler"
	"github.com/mrchatam/hodhod/internal/telegram"
	webpkg "github.com/mrchatam/hodhod/internal/web"
)

// Run starts the Hodhod application.
func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logging.Setup(cfg.LogLevel, cfg.Env)

	box, err := crypto.NewBox(cfg.AppEncryptionKey)
	if err != nil {
		return err
	}

	if cfg.RunMigrations {
		if err := migrate.Up(cfg.DatabaseDSN); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	gdb, err := db.Connect(cfg.DatabaseDSN, cfg.IsDev())
	if err != nil {
		return err
	}
	store := db.NewStore(gdb)

	ctx := context.Background()
	if cfg.MasterPassword != "" {
		if err := BootstrapAdmin(ctx, store, cfg.MasterUsername, cfg.MasterPassword); err != nil {
			slog.Warn("MASTER_PASSWORD bootstrap", "err", err)
		} else {
			slog.Warn("MASTER_PASSWORD is deprecated — use hodhod bootstrap-admin instead")
		}
	}
	if err := EnsureMasterExists(ctx, store); err != nil {
		return err
	}

	httpClient, err := httpx.New(httpx.Config{ProxyURL: cfg.OutboundSocksProxy})
	if err != nil {
		return err
	}
	if cfg.OutboundSocksProxy != "" {
		slog.Info("outbound proxy enabled", "proxy", cfg.OutboundSocksProxy)
	} else {
		slog.Info("outbound proxy disabled", "note", "containers use direct egress")
	}

	panelReg := panels.NewRegistry(box, httpClient, store)
	salesSvc := &sales.Service{Store: store, Panels: panelReg}
	backupSvc := &backup.Service{Store: store, Panels: panelReg, Box: box, Dir: cfg.BackupDir}
	wallet := &billing.WalletService{Store: store}
	orders := &billing.OrderService{Store: store, Wallet: wallet}
	prov := &provisioning.Service{Store: store, Sales: salesSvc}
	review := &billing.PaymentReviewService{Store: store, Wallet: wallet, Orders: orders, Prov: prov}
	botReader := &botconfig.Reader{Store: store}
	cardPick := &botconfig.CardPicker{Store: store}
	tgMgr := telegram.NewManager(cfg, box, store, httpClient, orders, wallet, prov, review, botReader, cardPick)

	if err := tgMgr.LoadActive(ctx); err != nil {
		slog.Warn("load bots", "err", err)
	}

	webSrv, err := webpkg.NewServer(cfg, store, box, panelReg, tgMgr, salesSvc, backupSvc, wallet, orders, review)
	if err != nil {
		return err
	}

	mini := &miniapp.API{Store: store, Box: box, Orders: orders, Wallet: wallet, Prov: prov, Reader: botReader}

	sched := scheduler.New(store, panelReg, tgMgr, backupSvc, cfg.PanelPollWorkers)
	sched.Start(cfg.CronUsagePoll, cfg.CronExpiryCheck, cfg.CronBackup)

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, webpkg.RateLimitWebhook)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(r.Context(), gdb); err != nil {
			http.Error(w, "db down", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mini.Routes(r)

	r.Group(func(r chi.Router) {
		r.Use(webSrv.HostMiddleware)
		r.Post("/wh/tg/{publicID}", func(w http.ResponseWriter, r *http.Request) {
			publicID := chi.URLParam(r, "publicID")
			secret := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
			expected, ok := tgMgr.SecretFor(r.Context(), publicID)
			if !ok || secret != expected {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			body, _ := io.ReadAll(r.Body)
			if err := tgMgr.Dispatch(r.Context(), publicID, body); err != nil {
				slog.Error("webhook", "err", err)
			}
			w.WriteHeader(http.StatusOK)
		})
		r.Handle("/miniapp/*", http.StripPrefix("/miniapp", http.FileServer(http.Dir("web/miniapp"))))
		r.Mount("/", webSrv.Handler())
	})

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: r}
	go func() {
		slog.Info("listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	sched.Stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
