package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server         ServerConfig
	Database       DatabaseConfig
	Auth           AuthConfig
	RateLimit      RateLimitConfig
	CORS           CORSConfig
	AdminBootstrap AdminBootstrapConfig
	Jobs           JobsConfig
	Logging        LoggingConfig
	Email          EmailConfig
	Environment    string
}

type ServerConfig struct {
	Host      string
	Port      int
	BaseURL   string
	PublicURL string // Public-facing domain for invitation links (e.g., "toronto.togather.foundation")
}

type DatabaseConfig struct {
	URL            string
	MaxConnections int
	MaxIdle        int
}

type AuthConfig struct {
	JWTSecret string
	JWTExpiry time.Duration
	CSRFKey   string // 32-byte key for CSRF token encryption (required for admin HTML forms)
}

type RateLimitConfig struct {
	PublicPerMinute     int
	AgentPerMinute      int
	AdminPerMinute      int
	LoginPer15Minutes   int      // Login attempts allowed per 15-minute window per IP
	FederationPerMinute int      // Federation sync rate limit
	TrustedProxyCIDRs   []string // CIDRs of trusted proxies (e.g., load balancers) for X-Forwarded-For validation
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

type LoggingConfig struct {
	Level  string
	Format string
}

type CORSConfig struct {
	AllowAllOrigins bool     // Development mode: allow all origins
	AllowedOrigins  []string // Production mode: whitelist of allowed origins
}

// EmailConfig holds email service configuration for Gmail SMTP
type EmailConfig struct {
	Enabled      bool   // Enable/disable email sending (useful for dev/test)
	From         string // From email address (e.g., "noreply@togather.foundation")
	SMTPHost     string // SMTP server hostname (default: "smtp.gmail.com")
	SMTPPort     int    // SMTP server port (default: 587 for TLS)
	SMTPUser     string // SMTP username (Gmail address)
	SMTPPassword string // Gmail App Password (NOT regular Gmail password - see https://support.google.com/accounts/answer/185833)
	TemplatesDir string // Path to email templates directory (default: "web/email/templates")
}

func Load() (Config, error) {
	// Try to load .env files if DATABASE_URL not already set
	if os.Getenv("DATABASE_URL") == "" {
		env := strings.TrimSpace(strings.ToLower(os.Getenv("ENVIRONMENT")))
		switch env {
		case "", "development", "dev", "test":
			LoadEnvFile(".env")
			LoadEnvFile("deploy/docker/.env")
		default:
			if path := strings.TrimSpace(os.Getenv("ENV_FILE")); path != "" {
				LoadEnvFile(path)
			}
			if os.Getenv("DATABASE_URL") == "" {
				return Config{}, fmt.Errorf("DATABASE_URL is required; set DATABASE_URL or ENV_FILE in %s", env)
			}
		}
	}

	cfg := Config{
		Server: ServerConfig{
			Host:      getEnv("SERVER_HOST", "0.0.0.0"),
			Port:      getEnvInt("SERVER_PORT", 8080),
			BaseURL:   getEnv("SERVER_BASE_URL", "http://localhost:8080"),
			PublicURL: getEnv("PUBLIC_URL", "localhost:8080"),
		},
		Database: DatabaseConfig{
			URL:            getEnv("DATABASE_URL", ""),
			MaxConnections: getEnvInt("DATABASE_MAX_CONNECTIONS", 25),
			MaxIdle:        getEnvInt("DATABASE_MAX_IDLE_CONNECTIONS", 5),
		},
		Auth: AuthConfig{
			JWTSecret: getEnv("JWT_SECRET", ""),
			JWTExpiry: time.Duration(getEnvInt("JWT_EXPIRY_HOURS", 24)) * time.Hour,
			CSRFKey:   getEnv("CSRF_KEY", ""),
		},
		RateLimit: RateLimitConfig{
			PublicPerMinute:     getEnvInt("RATE_LIMIT_PUBLIC", 60),
			AgentPerMinute:      getEnvInt("RATE_LIMIT_AGENT", 300),
			AdminPerMinute:      getEnvInt("RATE_LIMIT_ADMIN", 0),
			LoginPer15Minutes:   getEnvInt("RATE_LIMIT_LOGIN", 5),
			FederationPerMinute: getEnvInt("RATE_LIMIT_FEDERATION", 500),
			TrustedProxyCIDRs:   parseTrustedProxies(getEnv("TRUSTED_PROXY_CIDRS", "")),
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
		Logging: LoggingConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
		Email: EmailConfig{
			Enabled:      getEnvBool("EMAIL_ENABLED", false),
			From:         getEnv("EMAIL_FROM", "noreply@togather.foundation"),
			SMTPHost:     getEnv("SMTP_HOST", "smtp.gmail.com"),
			SMTPPort:     getEnvInt("SMTP_PORT", 587),
			SMTPUser:     getEnv("SMTP_USER", ""),
			SMTPPassword: getEnv("SMTP_PASSWORD", ""),
			TemplatesDir: getEnv("EMAIL_TEMPLATES_DIR", "web/email/templates"),
		},
		Environment: getEnv("ENVIRONMENT", "development"),
	}

	// CORS configuration
	env := cfg.Environment
	if env == "development" || env == "test" {
		// Development/test: allow all origins
		cfg.CORS = CORSConfig{
			AllowAllOrigins: true,
			AllowedOrigins:  nil,
		}
	} else {
		// Production: require explicit whitelist or wildcard
		allowedOrigins := getEnv("CORS_ALLOWED_ORIGINS", "")
		if allowedOrigins == "" {
			return Config{}, fmt.Errorf("CORS_ALLOWED_ORIGINS is required in production environment (use '*' for all origins or comma-separated list)")
		}

		// Handle wildcard '*' as special case to allow all origins
		if strings.TrimSpace(allowedOrigins) == "*" {
			cfg.CORS = CORSConfig{
				AllowAllOrigins: true,
				AllowedOrigins:  nil,
			}
		} else {
			// Parse comma-separated list of allowed origins
			origins := []string{}
			for _, origin := range strings.Split(allowedOrigins, ",") {
				trimmed := strings.TrimSpace(origin)
				if trimmed != "" {
					origins = append(origins, trimmed)
				}
			}
			if len(origins) == 0 {
				return Config{}, fmt.Errorf("CORS_ALLOWED_ORIGINS is required in production environment (use '*' for all origins or comma-separated list)")
			}
			cfg.CORS = CORSConfig{
				AllowAllOrigins: false,
				AllowedOrigins:  origins,
			}
		}
	}

	if cfg.Database.URL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.Auth.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}
	if len(cfg.Auth.JWTSecret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must be at least 32 characters for security (currently %d characters)", len(cfg.Auth.JWTSecret))
	}
	// CSRF key is optional but recommended if admin HTML UI is enabled
	if cfg.Auth.CSRFKey != "" && len(cfg.Auth.CSRFKey) < 32 {
		return Config{}, fmt.Errorf("CSRF_KEY must be at least 32 characters when provided (currently %d characters)", len(cfg.Auth.CSRFKey))
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

func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

// parseTrustedProxies parses a comma-separated list of CIDR ranges
func parseTrustedProxies(value string) []string {
	if value == "" {
		return nil
	}

	var cidrs []string
	for _, cidr := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(cidr)
		if trimmed != "" {
			cidrs = append(cidrs, trimmed)
		}
	}
	return cidrs
}

// LoadEnvFile loads environment variables from a .env file
// Silently ignores if file doesn't exist (not all setups use .env)
func LoadEnvFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return // File doesn't exist, that's ok
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Only set if not already in environment
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}
