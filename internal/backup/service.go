package backup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mrchatam/hodhod/internal/crypto"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
	"github.com/mrchatam/hodhod/internal/telegram"
)

// Service runs panel backups, stores files, prunes retention, and optionally pushes to Telegram.
type Service struct {
	Store  *db.Store
	Panels *panels.Registry
	Box    *crypto.Box
	Dir    string
}

const defaultRetention = 7

// RunForPanel downloads, stores, prunes, and optionally pushes a backup for one panel.
func (s *Service) RunForPanel(ctx context.Context, panelID int64) (*db.PanelBackup, error) {
	panel, err := s.Store.GetPanel(ctx, panelID)
	if err != nil {
		return nil, err
	}
	filename, data, err := s.Panels.Backup(ctx, panelID)
	rec := &db.PanelBackup{PanelID: panelID, Filename: filename, Status: "ok"}
	if err != nil {
		rec.Status = "failed"
		rec.Error = err.Error()
		_ = s.Store.CreatePanelBackup(ctx, rec)
		return rec, err
	}
	rec.SizeBytes = int64(len(data))
	if err := os.MkdirAll(s.Dir, 0o750); err != nil {
		rec.Status = "failed"
		rec.Error = err.Error()
		_ = s.Store.CreatePanelBackup(ctx, rec)
		return rec, err
	}
	safeName := filepath.Base(filename)
	path := filepath.Join(s.Dir, fmt.Sprintf("%d-%s", panelID, safeName))
	if err := os.WriteFile(path, data, 0o640); err != nil {
		rec.Status = "failed"
		rec.Error = err.Error()
		_ = s.Store.CreatePanelBackup(ctx, rec)
		return rec, err
	}
	rec.Filename = filepath.Base(path)
	if err := s.Store.CreatePanelBackup(ctx, rec); err != nil {
		return rec, err
	}
	if err := s.prune(ctx, panelID); err != nil {
		slog.Warn("backup prune", "panel", panelID, "err", err)
	}
	if err := s.pushTelegram(ctx, panelID, panel.Name, path, rec.Filename); err != nil {
		slog.Warn("backup telegram push", "panel", panelID, "err", err)
	}
	return rec, nil
}

func (s *Service) prune(ctx context.Context, panelID int64) error {
	retain := s.retention(ctx, panelID)
	backups, err := s.Store.ListPanelBackups(ctx, panelID, 0)
	if err != nil {
		return err
	}
	if len(backups) <= retain {
		return nil
	}
	for _, b := range backups[retain:] {
		_ = os.Remove(filepath.Join(s.Dir, b.Filename))
		_ = s.Store.DeletePanelBackup(ctx, panelID, b.ID)
	}
	return nil
}

func (s *Service) retention(ctx context.Context, panelID int64) int {
	v, _ := s.Store.GetSetting(ctx, "panel", panelID, "backup_retention")
	if v == "" {
		return defaultRetention
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultRetention
	}
	return n
}

func (s *Service) pushTelegram(ctx context.Context, panelID int64, panelName, filePath, displayName string) error {
	tokenEnc, _ := s.Store.GetSetting(ctx, "panel", panelID, "backup_tg_bot_token")
	chatID, _ := s.Store.GetSetting(ctx, "panel", panelID, "backup_tg_chat_id")
	tokenEnc = strings.TrimSpace(tokenEnc)
	chatID = strings.TrimSpace(chatID)
	if tokenEnc == "" || chatID == "" {
		return nil
	}
	token, err := s.Box.Decrypt(tokenEnc)
	if err != nil {
		return err
	}
	var chat int64
	if _, err := fmt.Sscanf(chatID, "%d", &chat); err != nil {
		return fmt.Errorf("invalid backup chat id")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	caption := fmt.Sprintf("Hodhod backup · %s · %s", panelName, time.Now().Format("2006-01-02 15:04"))
	return telegram.SendDocument(ctx, nil, token, chat, displayName, data, caption)
}

// FilePath returns the on-disk path for a stored backup filename.
func (s *Service) FilePath(filename string) string {
	return filepath.Join(s.Dir, filepath.Base(filename))
}
