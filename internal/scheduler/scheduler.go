package scheduler

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/mrchatam/hodhod/internal/backup"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/i18n"
	"github.com/mrchatam/hodhod/internal/panels"
	"github.com/mrchatam/hodhod/internal/telegram"
)

// Runner runs background jobs.
type Runner struct {
	Cron     *cron.Cron
	Store    *db.Store
	Panels   *panels.Registry
	Telegram *telegram.Manager
	Backup   *backup.Service
	Workers  int
}

// New creates a scheduler runner.
func New(store *db.Store, reg *panels.Registry, tg *telegram.Manager, backupSvc *backup.Service, workers int) *Runner {
	return &Runner{
		Cron:     cron.New(),
		Store:    store,
		Panels:   reg,
		Telegram: tg,
		Backup:   backupSvc,
		Workers:  workers,
	}
}

// Start registers cron jobs.
func (r *Runner) Start(usageSpec, expirySpec, backupSpec string) {
	_, _ = r.Cron.AddFunc(usageSpec, func() { r.pollUsage(context.Background()) })
	_, _ = r.Cron.AddFunc(expirySpec, func() { r.checkExpiry(context.Background()) })
	if r.Backup != nil && backupSpec != "" {
		_, _ = r.Cron.AddFunc(backupSpec, func() { r.runBackups(context.Background()) })
	}
	r.Cron.Start()
}

// Stop stops the cron scheduler.
func (r *Runner) Stop() { r.Cron.Stop() }

func (r *Runner) pollUsage(ctx context.Context) {
	panelsList, err := r.Store.ListPanels(ctx)
	if err != nil {
		slog.Error("scheduler list panels", "err", err)
		return
	}
	for _, p := range panelsList {
		if p.Status != "active" {
			continue
		}
		svcs, err := r.Store.ListActiveServicesForPanel(ctx, p.ID)
		if err != nil {
			continue
		}
		client, err := r.Panels.Get(ctx, p.ID)
		if err != nil {
			continue
		}
		for _, svc := range svcs {
			info, err := client.GetUser(ctx, svc.PanelUsername)
			if err != nil {
				continue
			}
			svc.UsedBytes = info.UsedBytes
			_ = r.Store.UpdateServiceByID(ctx, &svc)
			r.maybeWarn(ctx, &svc)
		}
	}
}

func (r *Runner) maybeWarn(ctx context.Context, svc *db.Service) {
	if svc.DataLimitBytes <= 0 || svc.BotID == nil || svc.EndUserID == nil {
		return
	}
	user, err := r.Store.GetEndUser(ctx, *svc.BotID, *svc.EndUserID)
	if err != nil {
		return
	}
	threshold := user.WarnPercent
	if v, _ := r.Store.GetSetting(ctx, "bot", *svc.BotID, "warn_percent"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			threshold = n
		}
	}
	pct := int(svc.UsedBytes * 100 / svc.DataLimitBytes)
	if pct < threshold || pct <= svc.LastWarnedPercent {
		return
	}
	bot, err := r.Store.GetBot(ctx, *svc.BotID)
	if err != nil {
		return
	}
	text := i18n.T(user.Lang, "usage_warn", pct)
	_ = r.Telegram.SendMessage(ctx, bot.PublicID, user.TelegramID, text)
	svc.LastWarnedPercent = pct
	_ = r.Store.UpdateServiceByID(ctx, svc)
}

func (r *Runner) checkExpiry(ctx context.Context) {
	panelsList, err := r.Store.ListPanels(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	for _, p := range panelsList {
		svcs, _ := r.Store.ListActiveServicesForPanel(ctx, p.ID)
		for _, svc := range svcs {
			if svc.ExpireAt == nil {
				continue
			}
			if svc.BotID == nil || svc.EndUserID == nil {
				if svc.ExpireAt.Before(now) {
					if client, err := r.Panels.Get(ctx, p.ID); err == nil {
						_ = client.Disable(ctx, svc.PanelUsername)
					}
					svc.Status = "expired"
					_ = r.Store.UpdateServiceByID(ctx, &svc)
				}
				continue
			}
			user, err := r.Store.GetEndUser(ctx, *svc.BotID, *svc.EndUserID)
			if err != nil {
				continue
			}
			bot, _ := r.Store.GetBot(ctx, *svc.BotID)
			if svc.ExpireAt.Before(now) {
				if client, err := r.Panels.Get(ctx, p.ID); err == nil {
					_ = client.Disable(ctx, svc.PanelUsername)
				}
				text := i18n.T(user.Lang, "service_expired")
				_ = r.Telegram.SendMessage(ctx, bot.PublicID, user.TelegramID, text)
				svc.Status = "expired"
				_ = r.Store.UpdateServiceByID(ctx, &svc)
			} else if svc.ExpireAt.Before(now.Add(48 * time.Hour)) {
				text := i18n.T(user.Lang, "service_expiring")
				_ = r.Telegram.SendMessage(ctx, bot.PublicID, user.TelegramID, text)
			}
		}
	}
}

func (r *Runner) runBackups(ctx context.Context) {
	panelsList, err := r.Store.ListPanelsWithBackupEnabled(ctx)
	if err != nil {
		slog.Error("scheduler backup list panels", "err", err)
		return
	}
	for _, p := range panelsList {
		if _, err := r.Backup.RunForPanel(ctx, p.ID); err != nil {
			slog.Warn("scheduled backup failed", "panel", p.ID, "err", err)
		}
	}
}
