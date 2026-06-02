package config

import (
	"os"
	"testing"
)

func TestLoad_missingRequired(t *testing.T) {
	os.Clearenv()
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing required keys")
	}
}

func TestLoad_presentKeys(t *testing.T) {
	os.Clearenv()
	os.Setenv("PUBLIC_BASE_URL", "https://example.com")
	os.Setenv("DATABASE_DSN", "postgres://u:p@localhost/db?sslmode=disable")
	os.Setenv("APP_ENCRYPTION_KEY", "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGs=")
	os.Setenv("SESSION_SECRET", "secret")
	os.Setenv("MASTER_PASSWORD", "pass")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicBaseURL != "https://example.com" {
		t.Fatalf("got %q", cfg.PublicBaseURL)
	}
}
