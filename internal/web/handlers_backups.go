package web

import (
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
)

func (s *Server) pagePanelBackups(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	target := panelTabURL(panelID, "backups")
	if q := r.URL.RawQuery; q != "" {
		target += "&" + q
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

func (s *Server) loadPanelBackupsViewData(r *http.Request, panel *db.Panel) map[string]any {
	panelID := panel.ID
	total, _ := s.Store.CountPanelBackups(r.Context(), panelID)
	pag := paginationFromRequest(r, int(total), "tab")
	if pag.Query["tab"] == "" {
		pag.Query["tab"] = "backups"
	}
	backups, _ := s.Store.ListPanelBackupsPaginated(r.Context(), panelID, pag.PerPage, pag.Offset())
	settings := s.panelBackupSettings(r, panelID)
	client, _ := s.Panels.Get(r.Context(), panelID)
	_, supportsBackup := client.(panels.Backuper)
	return map[string]any{
		"Backups": backups, "Settings": settings, "SupportsBackup": supportsBackup,
		"Pagination": pag,
	}
}

func (s *Server) panelBackupSettings(r *http.Request, panelID int64) map[string]string {
	keys := []string{"backup_enabled", "backup_cron", "backup_retention", "backup_tg_bot_token", "backup_tg_chat_id"}
	out := make(map[string]string)
	for _, k := range keys {
		v, _ := s.Store.GetSetting(r.Context(), "panel", panelID, k)
		out[k] = v
	}
	if out["backup_retention"] == "" {
		out["backup_retention"] = "7"
	}
	return out
}

func (s *Server) postPanelBackupRun(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if s.Backup == nil {
		s.setFlash(w, "err", "Backup service not configured")
		http.Redirect(w, r, panelTabURL(panelID, "backups"), http.StatusSeeOther)
		return
	}
	_, err := s.Backup.RunForPanel(r.Context(), panelID)
	s.saveFlash(w, err, "Backup completed")
	http.Redirect(w, r, panelTabURL(panelID, "backups"), http.StatusSeeOther)
}

func (s *Server) getPanelBackupDownload(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	backupID, _ := strconv.ParseInt(chi.URLParam(r, "backupID"), 10, 64)
	rec, err := s.Store.GetPanelBackup(r.Context(), panelID, backupID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	path := s.Backup.FilePath(rec.Filename)
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename="+rec.Filename)
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(data)
}

func (s *Server) postPanelBackupSettings(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	enabled := "0"
	if r.FormValue("backup_enabled") == "on" {
		enabled = "1"
	}
	_ = s.Store.SetSetting(r.Context(), "panel", panelID, "backup_enabled", enabled)
	if v := strings.TrimSpace(r.FormValue("backup_cron")); v != "" {
		_ = s.Store.SetSetting(r.Context(), "panel", panelID, "backup_cron", v)
	}
	if v := strings.TrimSpace(r.FormValue("backup_retention")); v != "" {
		_ = s.Store.SetSetting(r.Context(), "panel", panelID, "backup_retention", v)
	}
	if v := strings.TrimSpace(r.FormValue("backup_tg_chat_id")); v != "" {
		_ = s.Store.SetSetting(r.Context(), "panel", panelID, "backup_tg_chat_id", v)
	}
	if token := strings.TrimSpace(r.FormValue("backup_tg_bot_token")); token != "" {
		enc, err := s.Box.Encrypt(token)
		if err != nil {
			s.setFlash(w, "err", "Could not encrypt bot token")
			http.Redirect(w, r, panelTabURL(panelID, "backups"), http.StatusSeeOther)
			return
		}
		_ = s.Store.SetSetting(r.Context(), "panel", panelID, "backup_tg_bot_token", enc)
	}
	s.setFlash(w, "ok", "Backup settings saved")
	http.Redirect(w, r, panelTabURL(panelID, "backups"), http.StatusSeeOther)
}
