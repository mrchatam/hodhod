package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mrchatam/hodhod/internal/db"
)

type pruneStore struct {
	backups []db.PanelBackup
}

func (m *pruneStore) ListPanelBackups(_ context.Context, panelID int64, _ int) ([]db.PanelBackup, error) {
	var out []db.PanelBackup
	for _, b := range m.backups {
		if b.PanelID == panelID {
			out = append(out, b)
		}
	}
	return out, nil
}

func (m *pruneStore) DeletePanelBackup(_ context.Context, panelID, backupID int64) error {
	for i, b := range m.backups {
		if b.PanelID == panelID && b.ID == backupID {
			m.backups = append(m.backups[:i], m.backups[i+1:]...)
			return nil
		}
	}
	return nil
}

func TestPruneRetention(t *testing.T) {
	dir := t.TempDir()
	mem := &pruneStore{}
	for i := 1; i <= 4; i++ {
		fn := filepath.Join(dir, "1-backup-"+string(rune('0'+i))+".db")
		_ = os.WriteFile(fn, []byte("x"), 0o640)
		mem.backups = append(mem.backups, db.PanelBackup{
			ID: int64(i), PanelID: 1, Filename: filepath.Base(fn), Status: "ok",
		})
	}
	svc := &Service{Dir: dir}
	backups, _ := mem.ListPanelBackups(context.Background(), 1, 0)
	retain := 2
	if len(backups) <= retain {
		t.Fatal("need more backups for test")
	}
	for _, b := range backups[retain:] {
		_ = os.Remove(filepath.Join(svc.Dir, b.Filename))
		_ = mem.DeletePanelBackup(context.Background(), 1, b.ID)
	}
	if len(mem.backups) != 2 {
		t.Fatalf("expected 2 backups retained, got %d", len(mem.backups))
	}
}
