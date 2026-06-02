package config

import (
	"fmt"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config holds application configuration from environment variables.
type Config struct {
	Env                string `env:"ENV" envDefault:"development"`
	HTTPAddr           string `env:"HTTP_ADDR" envDefault:":8080"`
	PublicBaseURL      string `env:"PUBLIC_BASE_URL,required"`
	DatabaseDSN        string `env:"DATABASE_DSN,required"`
	AppEncryptionKey   string `env:"APP_ENCRYPTION_KEY,required"`
	OutboundSocksProxy string `env:"OUTBOUND_SOCKS_PROXY"`
	RunMigrations      bool   `env:"RUN_MIGRATIONS" envDefault:"true"`
	PanelPollWorkers   int    `env:"PANEL_POLL_WORKERS" envDefault:"4"`
	SessionSecret      string `env:"SESSION_SECRET,required"`
	LogLevel           string `env:"LOG_LEVEL" envDefault:"info"`
	MasterUsername     string `env:"MASTER_USERNAME" envDefault:"admin"`
	MasterPassword     string `env:"MASTER_PASSWORD,required"`
	CronUsagePoll      string `env:"CRON_USAGE_POLL" envDefault:"@every 5m"`
	CronExpiryCheck    string `env:"CRON_EXPIRY_CHECK" envDefault:"@every 1h"`
}

// Load reads .env (if present) and parses environment into Config.
func Load() (*Config, error) {
	_ = godotenv.Load()
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	cfg.PublicBaseURL = strings.TrimRight(cfg.PublicBaseURL, "/")
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	var missing []string
	if c.PublicBaseURL == "" {
		missing = append(missing, "PUBLIC_BASE_URL")
	}
	if c.DatabaseDSN == "" {
		missing = append(missing, "DATABASE_DSN")
	}
	if c.AppEncryptionKey == "" {
		missing = append(missing, "APP_ENCRYPTION_KEY")
	}
	if c.SessionSecret == "" {
		missing = append(missing, "SESSION_SECRET")
	}
	if c.MasterPassword == "" {
		missing = append(missing, "MASTER_PASSWORD")
	}
	if len(missing) > 0 {
		return fmt.Errorf("config: missing required: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (c *Config) IsDev() bool {
	return c.Env == "development" || c.Env == "dev"
}
