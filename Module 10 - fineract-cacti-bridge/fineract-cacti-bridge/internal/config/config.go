package config

import "github.com/fineract/cacti-bridge/pkg/flog"
import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the fully-resolved application configuration.
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Ledgers    LedgersConfig    `yaml:"ledgers"`
	Settlement SettlementConfig `yaml:"settlement"`
	Database   DatabaseConfig   `yaml:"database"`
	Log        LogConfig        `yaml:"log"`
}

type ServerConfig struct {
	Port            int           `yaml:"port"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
	RateLimitPerMin int           `yaml:"rate_limit_per_min"`
}

// LedgersConfig holds the two Cacti connector endpoints the bridge coordinates.
type LedgersConfig struct {
	Source LedgerConfig `yaml:"source"`
	Dest   LedgerConfig `yaml:"dest"`
}

// LedgerConfig is the connection + resilience config for one Cacti ledger
// connector (Hyperledger Cacti exposes each ledger behind a REST connector).
type LedgerConfig struct {
	Name          string        `yaml:"name"`
	BaseURL       string        `yaml:"base_url"`
	AuthMode      string        `yaml:"auth_mode"` // apikey | oauth2 | mtls
	APIKey        string        `yaml:"api_key"`
	OAuthTokenURL string        `yaml:"oauth_token_url"`
	ClientID      string        `yaml:"client_id"`
	ClientSecret  string        `yaml:"client_secret"`
	Timeout       time.Duration `yaml:"timeout"`
	MaxRetries    int           `yaml:"max_retries"`
	BreakerMaxReq uint32        `yaml:"breaker_max_requests"`
	BreakerName   string        `yaml:"breaker_name"`
}

// SettlementConfig tunes the lock-release-burn saga.
type SettlementConfig struct {
	BurnMaxAttempts  int           `yaml:"burn_max_attempts"`
	StepTimeout      time.Duration `yaml:"step_timeout"`
	RecoverOnStartup bool          `yaml:"recover_on_startup"`
}

type DatabaseConfig struct {
	DSN             string        `yaml:"dsn"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
	Enabled         bool          `yaml:"enabled"`
}

type LogConfig struct {
	Level string `yaml:"level"`
	JSON  bool   `yaml:"json"`
}

func ledgerDefaults(name string) LedgerConfig {
	return LedgerConfig{
		Name:          name,
		AuthMode:      "apikey",
		Timeout:       10 * time.Second,
		MaxRetries:    3,
		BreakerMaxReq: 5,
		BreakerName:   name + "-connector",
	}
}

// Default returns production-sane defaults.
func Default() Config {
	return Config{
		Server: ServerConfig{
			Port:            8080,
			ReadTimeout:     10 * time.Second,
			WriteTimeout:    15 * time.Second,
			ShutdownTimeout: 20 * time.Second,
			RateLimitPerMin: 120,
		},
		Ledgers: LedgersConfig{
			Source: ledgerDefaults("source"),
			Dest:   ledgerDefaults("dest"),
		},
		Settlement: SettlementConfig{
			BurnMaxAttempts:  4,
			StepTimeout:      15 * time.Second,
			RecoverOnStartup: true,
		},
		Database: DatabaseConfig{
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 30 * time.Minute,
			Enabled:         false,
		},
		Log: LogConfig{Level: "info", JSON: true},
	}
}

// Load reads YAML (if present), applies env overrides, then validates.
func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return Config{}, fmt.Errorf("parsing config %s: %w", path, err)
			}
		} else if !os.IsNotExist(err) {
			return Config{}, fmt.Errorf("reading config %s: %w", path, err)
		}
	}
	applyEnvOverrides(&cfg)
	if cfg.Settlement.BurnMaxAttempts <= 0 {
		cfg.Settlement.BurnMaxAttempts = 4
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("SERVER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
	applyLedgerEnv(&cfg.Ledgers.Source, "SOURCE")
	applyLedgerEnv(&cfg.Ledgers.Dest, "DEST")
	if v := os.Getenv("DATABASE_DSN"); v != "" {
		cfg.Database.DSN = v
		cfg.Database.Enabled = true
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
}

func applyLedgerEnv(l *LedgerConfig, prefix string) {
	if v := os.Getenv(prefix + "_LEDGER_NAME"); v != "" {
		l.Name = v
	}
	if v := os.Getenv(prefix + "_BASE_URL"); v != "" {
		l.BaseURL = v
	}
	if v := os.Getenv(prefix + "_AUTH_MODE"); v != "" {
		l.AuthMode = v
	}
	if v := os.Getenv(prefix + "_API_KEY"); v != "" {
		l.APIKey = v
	}
	if v := os.Getenv(prefix + "_OAUTH_TOKEN_URL"); v != "" {
		l.OAuthTokenURL = v
	}
	if v := os.Getenv(prefix + "_CLIENT_ID"); v != "" {
		l.ClientID = v
	}
	if v := os.Getenv(prefix + "_CLIENT_SECRET"); v != "" {
		l.ClientSecret = v
	}
}

// Validate fails fast on inconsistent configuration.
func (c Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	if err := c.Ledgers.Source.validate("source"); err != nil {
		return err
	}
	if err := c.Ledgers.Dest.validate("dest"); err != nil {
		return err
	}
	if c.Database.Enabled && c.Database.DSN == "" {
		return fmt.Errorf("database.dsn is required when database is enabled")
	}
	return nil
}

func (l LedgerConfig) validate(which string) error {
	if l.BaseURL == "" {
		return fmt.Errorf("ledgers.%s.base_url is required", which)
	}
	switch l.AuthMode {
	case "apikey":
		if l.APIKey == "" {
			return fmt.Errorf("ledgers.%s.api_key is required when auth_mode=apikey", which)
		}
	case "oauth2":
		if l.OAuthTokenURL == "" || l.ClientID == "" || l.ClientSecret == "" {
			return fmt.Errorf("ledgers.%s oauth2 auth requires oauth_token_url, client_id and client_secret", which)
		}
	case "mtls":
	default:
		return fmt.Errorf("ledgers.%s unsupported auth_mode %q (want apikey|oauth2|mtls)", which, l.AuthMode)
	}
	return nil
}

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_config.log at runtime.
var _ = func() bool { flog.For("10_config").Info("source file initialized"); return true }()
