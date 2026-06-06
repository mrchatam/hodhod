package config

import (
	"fmt"
	"net"
	"net/url"
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
	MasterPassword     string `env:"MASTER_PASSWORD"`
	CronUsagePoll      string `env:"CRON_USAGE_POLL" envDefault:"@every 5m"`
	CronExpiryCheck    string `env:"CRON_EXPIRY_CHECK" envDefault:"@every 1h"`
	CronBackup         string `env:"CRON_BACKUP" envDefault:"0 3 * * *"`
	BackupDir          string `env:"BACKUP_DIR" envDefault:"./data/backups"`
	AllowCustomDomains bool   `env:"ALLOW_CUSTOM_DOMAINS" envDefault:"true"`
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
	if len(missing) > 0 {
		return fmt.Errorf("config: missing required: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (c *Config) IsDev() bool {
	return c.Env == "development" || c.Env == "dev"
}

// MainHost returns the hostname from PublicBaseURL for host routing.
func (c *Config) MainHost() string {
	u, err := url.Parse(c.PublicBaseURL)
	if err != nil || u.Host == "" {
		return strings.TrimPrefix(strings.TrimPrefix(c.PublicBaseURL, "https://"), "http://")
	}
	host := u.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(host)
}
