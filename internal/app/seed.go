package app

import (
	"context"
	"log/slog"

	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/web"
)

func seedMaster(ctx context.Context, store *db.Store, username, password string) error {
	n, err := store.CountAdminsByRole(ctx, db.RoleMaster)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	hash, err := web.HashPassword(password)
	if err != nil {
		return err
	}
	admin := &db.Admin{
		Username:     username,
		PasswordHash: hash,
		Role:         db.RoleMaster,
	}
	if err := store.CreateAdmin(ctx, admin); err != nil {
		return err
	}
	slog.Info("master admin created", "username", username)
	return nil
}
