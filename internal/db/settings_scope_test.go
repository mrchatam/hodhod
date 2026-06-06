package db

import (
	"context"
	"os"
	"testing"

	"github.com/mrchatam/hodhod/internal/db/migrate"
)

// Integration: verifies settings CHECK constraint for admin/panel scopes.
func TestSetSettingScopes(t *testing.T) {
	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		t.Skip("DATABASE_DSN not set")
	}
	if err := migrate.Up(dsn); err != nil {
		t.Fatal(err)
	}
	gdb, err := Connect(dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(gdb)
	for _, scope := range []string{"admin", "panel", "bot"} {
		err := store.SetSetting(context.Background(), scope, 1, "test_key", "test_val")
		if err != nil {
			t.Errorf("SetSetting scope=%q: %v", scope, err)
		}
	}
}
