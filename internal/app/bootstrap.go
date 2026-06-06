package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/web"
)

// BootstrapAdmin creates the first master admin if none exists.
func BootstrapAdmin(ctx context.Context, store *db.Store, username, password string) error {
	if username == "" {
		return fmt.Errorf("username required")
	}
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	n, err := store.CountAdminsByRole(ctx, db.RoleMaster)
	if err != nil {
		return err
	}
	if n > 0 {
		slog.Info("bootstrap-admin: master already exists, skipping")
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

// EnsureMasterExists returns an error if no master admin exists.
func EnsureMasterExists(ctx context.Context, store *db.Store) error {
	n, err := store.CountAdminsByRole(ctx, db.RoleMaster)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no master admin found — run: hodhod bootstrap-admin --username admin --password <password>")
	}
	return nil
}
