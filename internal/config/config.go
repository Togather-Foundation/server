package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server         ServerConfig
	Database       DatabaseConfig
	Auth           AuthConfig
	RateLimit      RateLimitConfig
	AdminBootstrap AdminBootstrapConfig
	Jobs           JobsConfig
	Environment    string
}

type ServerConfig struct {
	Host    string
	Port    int
	BaseURL string
}

type DatabaseConfig struct {
	URL            string
	MaxConnections int
	MaxIdle        int
}

type AuthConfig struct {
	JWTSecret string
	JWTExpiry time.Duration
}

type RateLimitConfig struct {
	PublicPerMinute int
	AgentPerMinute  int
	AdminPerMinute  int
}

type AdminBootstrapConfig struct {
	Username string
	Password string
	Email    string
}

type JobsConfig struct {
	RetryDeduplication  int
	RetryReconciliation int
	RetryEnrichment     int
}

func Load() (Config, error) {
	cfg := Config{
		Server: ServerConfig{
			Host:    getEnv("SERVER_HOST", "0.0.0.0"),
			Port:    getEnvInt("SERVER_PORT", 8080),
			BaseURL: getEnv("SERVER_BASE_URL", "http://localhost:8080"),
		},
		Database: DatabaseConfig{
			URL:            getEnv("DATABASE_URL", ""),
			MaxConnections: getEnvInt("DATABASE_MAX_CONNECTIONS", 25),
			MaxIdle:        getEnvInt("DATABASE_MAX_IDLE_CONNECTIONS", 5),
		},
		Auth: AuthConfig{
			JWTSecret: getEnv("JWT_SECRET", ""),
			JWTExpiry: time.Duration(getEnvInt("JWT_EXPIRY_HOURS", 24)) * time.Hour,
		},
		RateLimit: RateLimitConfig{
			PublicPerMinute: getEnvInt("RATE_LIMIT_PUBLIC", 60),
			AgentPerMinute:  getEnvInt("RATE_LIMIT_AGENT", 300),
			AdminPerMinute:  getEnvInt("RATE_LIMIT_ADMIN", 0),
		},
		AdminBootstrap: AdminBootstrapConfig{
			Username: getEnv("ADMIN_USERNAME", ""),
			Password: getEnv("ADMIN_PASSWORD", ""),
			Email:    getEnv("ADMIN_EMAIL", ""),
		},
		Jobs: JobsConfig{
			RetryDeduplication:  getEnvInt("JOB_RETRY_DEDUPLICATION", 1),
			RetryReconciliation: getEnvInt("JOB_RETRY_RECONCILIATION", 5),
			RetryEnrichment:     getEnvInt("JOB_RETRY_ENRICHMENT", 10),
		},
		Environment: getEnv("ENVIRONMENT", "development"),
	}

	if cfg.Database.URL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.Auth.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
