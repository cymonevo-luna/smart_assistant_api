// Package config loads and validates application configuration.
//
// Configuration is sourced from environment variables (optionally seeded from a
// .env file). Centralising it here keeps the rest of the codebase free of
// os.Getenv calls and makes the full configuration surface easy to discover.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Driver enumerates the supported persistence backends. Switching this value is
// the only change required to move the whole application between databases.
type Driver string

const (
	DriverPostgres Driver = "postgres"
	DriverMongo    Driver = "mongo"
)

// CacheDriver enumerates the supported cache backends.
type CacheDriver string

const (
	CacheRedis  CacheDriver = "redis"
	CacheMemory CacheDriver = "memory"
)

// Config is the root configuration aggregate.
type Config struct {
	App       AppConfig
	HTTP      HTTPConfig
	Database  DatabaseConfig
	Cache     CacheConfig
	Queue     QueueConfig
	RateLimit RateLimitConfig
	Auth      AuthConfig
}

type AppConfig struct {
	Name        string `env:"APP_NAME" envDefault:"go_template"`
	Environment string `env:"APP_ENV" envDefault:"development"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"debug"`
}

func (a AppConfig) IsProduction() bool { return a.Environment == "production" }

type HTTPConfig struct {
	Host            string        `env:"HTTP_HOST" envDefault:"0.0.0.0"`
	Port            int           `env:"HTTP_PORT" envDefault:"8080"`
	ReadTimeout     time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"10s"`
	WriteTimeout    time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"15s"`
	IdleTimeout     time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"60s"`
	ShutdownTimeout time.Duration `env:"HTTP_SHUTDOWN_TIMEOUT" envDefault:"15s"`
	RequestTimeout  time.Duration `env:"HTTP_REQUEST_TIMEOUT" envDefault:"30s"`
}

func (h HTTPConfig) Addr() string { return fmt.Sprintf("%s:%d", h.Host, h.Port) }

type DatabaseConfig struct {
	Driver Driver `env:"DB_DRIVER" envDefault:"postgres"`

	// Shared connection settings.
	URI             string        `env:"DB_URI"`
	MaxOpenConns    int32         `env:"DB_MAX_OPEN_CONNS" envDefault:"25"`
	MinConns        int32         `env:"DB_MIN_CONNS" envDefault:"5"`
	MaxConnLifetime time.Duration `env:"DB_MAX_CONN_LIFETIME" envDefault:"1h"`
	ConnTimeout     time.Duration `env:"DB_CONN_TIMEOUT" envDefault:"5s"`

	// Mongo specific.
	MongoDatabase string `env:"MONGO_DATABASE" envDefault:"go_template"`
}

type CacheConfig struct {
	Driver   CacheDriver   `env:"CACHE_DRIVER" envDefault:"memory"`
	Addr     string        `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	Password string        `env:"REDIS_PASSWORD"`
	DB       int           `env:"REDIS_DB" envDefault:"0"`
	TTL      time.Duration `env:"CACHE_TTL" envDefault:"5m"`
}

// QueueDriver enumerates the supported message queue backends.
type QueueDriver string

const (
	QueueRedis  QueueDriver = "redis"
	QueueMemory QueueDriver = "memory"
)

type QueueConfig struct {
	Driver      QueueDriver   `env:"QUEUE_DRIVER" envDefault:"memory"`
	MaxAttempts int           `env:"QUEUE_MAX_ATTEMPTS" envDefault:"3"`
	Backoff     time.Duration `env:"QUEUE_BACKOFF" envDefault:"1s"`
}

type RateLimitConfig struct {
	Enabled  bool          `env:"RATE_LIMIT_ENABLED" envDefault:"true"`
	Requests int           `env:"RATE_LIMIT_REQUESTS" envDefault:"100"`
	Window   time.Duration `env:"RATE_LIMIT_WINDOW" envDefault:"1m"`
}

type AuthConfig struct {
	JWTSecret  string        `env:"JWT_SECRET" envDefault:"change-me-in-production"`
	JWTTTL     time.Duration `env:"JWT_TTL" envDefault:"15m"`
	RefreshTTL time.Duration `env:"JWT_REFRESH_TTL" envDefault:"168h"`
	Issuer     string        `env:"JWT_ISSUER" envDefault:"go_template"`
}

// Load reads configuration from the environment. The .env file, if present, is
// loaded first but never overrides variables already set in the environment.
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	switch c.Database.Driver {
	case DriverPostgres, DriverMongo:
	default:
		return fmt.Errorf("unsupported DB_DRIVER %q", c.Database.Driver)
	}

	if c.Database.URI == "" {
		// Provide sensible local defaults so the template runs out of the box.
		switch c.Database.Driver {
		case DriverPostgres:
			c.Database.URI = "postgres://postgres:postgres@localhost:5432/go_template?sslmode=disable"
		case DriverMongo:
			c.Database.URI = "mongodb://localhost:27017"
		}
	}

	switch c.Cache.Driver {
	case CacheRedis, CacheMemory:
	default:
		return fmt.Errorf("unsupported CACHE_DRIVER %q", c.Cache.Driver)
	}

	switch c.Queue.Driver {
	case QueueRedis, QueueMemory:
	default:
		return fmt.Errorf("unsupported QUEUE_DRIVER %q", c.Queue.Driver)
	}

	return nil
}

// UsesRedis reports whether any subsystem is configured to use Redis, so a
// single shared client can be created.
func (c *Config) UsesRedis() bool {
	return c.Cache.Driver == CacheRedis || c.Queue.Driver == QueueRedis
}
