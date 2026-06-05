package db

import (
	"context"
)

func (s *Store) CreatePanelBackup(ctx context.Context, b *PanelBackup) error {
	return s.DB.WithContext(ctx).Create(b).Error
}

func (s *Store) ListPanelBackups(ctx context.Context, panelID int64, limit int) ([]PanelBackup, error) {
	var out []PanelBackup
	q := s.DB.WithContext(ctx).Where("panel_id = ?", panelID).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	return out, q.Find(&out).Error
}

func (s *Store) GetPanelBackup(ctx context.Context, panelID, backupID int64) (*PanelBackup, error) {
	var b PanelBackup
	err := s.DB.WithContext(ctx).Where("id = ? AND panel_id = ?", backupID, panelID).First(&b).Error
	return &b, err
}

func (s *Store) DeletePanelBackup(ctx context.Context, panelID, backupID int64) error {
	return s.DB.WithContext(ctx).Where("id = ? AND panel_id = ?", backupID, panelID).Delete(&PanelBackup{}).Error
}

// PrunePanelBackups keeps the newest retainCount rows and deletes older ones.
func (s *Store) PrunePanelBackups(ctx context.Context, panelID int64, retainCount int) error {
	if retainCount <= 0 {
		return nil
	}
	var keep []int64
	err := s.DB.WithContext(ctx).Model(&PanelBackup{}).
		Where("panel_id = ?", panelID).
		Order("created_at DESC").
		Limit(retainCount).
		Pluck("id", &keep).Error
	if err != nil || len(keep) == 0 {
		return err
	}
	return s.DB.WithContext(ctx).
		Where("panel_id = ? AND id NOT IN ?", panelID, keep).
		Delete(&PanelBackup{}).Error
}

func (s *Store) ListPanelsWithBackupEnabled(ctx context.Context) ([]Panel, error) {
	var out []Panel
	err := s.DB.WithContext(ctx).
		Table("panels").
		Joins(`JOIN settings ON settings.scope = 'panel' AND settings.scope_id = panels.id AND settings.key = 'backup_enabled' AND settings.value = '1'`).
		Where("panels.status = ?", "active").
		Find(&out).Error
	return out, err
}
