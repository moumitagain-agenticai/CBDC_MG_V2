package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the fully-resolved application configuration.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	CBDC     CBDCConfig     `yaml:"cbdc"`
	Database DatabaseConfig `yaml:"database"`
	Log      LogConfig      `yaml:"log"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port            int           `yaml:"port"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
	RateLimitPerMin int           `yaml:"rate_limit_per_min"`
}

// CBDCConfig holds the sponsor-bank e₹ API connection and resilience settings.
type CBDCConfig struct {
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

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	DSN             string        `yaml:"dsn"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
	Enabled         bool          `yaml:"enabled"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level string `yaml:"level"` // debug | info | warn | error
	JSON  bool   `yaml:"json"`
}

// Default returns a Config populated with production-sane defaults.
func Default() Config {
	return Config{
		Server: ServerConfig{
			Port:            8080,
			ReadTimeout:     10 * time.Second,
			WriteTimeout:    15 * time.Second,
			ShutdownTimeout: 20 * time.Second,
			RateLimitPerMin: 120,
		},
		CBDC: CBDCConfig{
			AuthMode:      "apikey",
			Timeout:       8 * time.Second,
			MaxRetries:    3,
			BreakerMaxReq: 5,
			BreakerName:   "cbdc-sponsor-bank",
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

// Load reads YAML config from path (if it exists), then applies environment
// variable overrides, then validates. A missing file is not an error: env vars
// alone can fully configure the service.
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

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// applyEnvOverrides maps environment variables onto the config. Secrets should
// always be supplied via env in production rather than committed to YAML.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("SERVER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
	if v := os.Getenv("CBDC_BASE_URL"); v != "" {
		cfg.CBDC.BaseURL = v
	}
	if v := os.Getenv("CBDC_AUTH_MODE"); v != "" {
		cfg.CBDC.AuthMode = v
	}
	if v := os.Getenv("CBDC_API_KEY"); v != "" {
		cfg.CBDC.APIKey = v
	}
	if v := os.Getenv("CBDC_OAUTH_TOKEN_URL"); v != "" {
		cfg.CBDC.OAuthTokenURL = v
	}
	if v := os.Getenv("CBDC_CLIENT_ID"); v != "" {
		cfg.CBDC.ClientID = v
	}
	if v := os.Getenv("CBDC_CLIENT_SECRET"); v != "" {
		cfg.CBDC.ClientSecret = v
	}
	if v := os.Getenv("DATABASE_DSN"); v != "" {
		cfg.Database.DSN = v
		cfg.Database.Enabled = true
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
}

// Validate fails fast on missing or inconsistent required configuration.
func (c Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	if c.CBDC.BaseURL == "" {
		return fmt.Errorf("cbdc.base_url is required (set CBDC_BASE_URL)")
	}
	switch c.CBDC.AuthMode {
	case "apikey":
		if c.CBDC.APIKey == "" {
			return fmt.Errorf("cbdc.api_key is required when auth_mode=apikey")
		}
	case "oauth2":
		if c.CBDC.OAuthTokenURL == "" || c.CBDC.ClientID == "" || c.CBDC.ClientSecret == "" {
			return fmt.Errorf("oauth2 auth requires oauth_token_url, client_id and client_secret")
		}
	case "mtls":
		// Certificate wiring handled at deployment (mounted key/cert); no inline secret.
	default:
		return fmt.Errorf("unsupported cbdc.auth_mode %q (want apikey|oauth2|mtls)", c.CBDC.AuthMode)
	}
	if c.Database.Enabled && c.Database.DSN == "" {
		return fmt.Errorf("database.dsn is required when database is enabled")
	}
	return nil
}
