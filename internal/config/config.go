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
	Validation     ValidationConfig
	Tracing        TracingConfig
	Dedup          DedupConfig
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
	GitHub    GitHubOAuthConfig
}

// GitHubOAuthConfig holds GitHub OAuth 2.0 configuration.
// These settings are optional - if not configured, GitHub OAuth login is disabled.
type GitHubOAuthConfig struct {
	ClientID     string   // GitHub OAuth App Client ID
	ClientSecret string   // GitHub OAuth App Client Secret
	CallbackURL  string   // OAuth callback URL (e.g., "http://localhost:8080/auth/github/callback")
	AllowedOrgs  []string // Optional: restrict login to members of these GitHub organizations
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

// TracingConfig holds OpenTelemetry tracing configuration.
// Tracing is opt-in and disabled by default to avoid breaking existing deployments.
type TracingConfig struct {
	// Enabled controls whether OpenTelemetry tracing is active.
	// When false (default): no traces are generated, zero performance overhead.
	// When true: spans are created for HTTP requests and key operations.
	Enabled bool

	// ServiceName identifies this service in traces (default: "togather-sel-server")
	ServiceName string

	// Exporter determines where traces are sent: "stdout", "otlp", "none" (default: "stdout")
	// - stdout: human-readable traces to console (good for development/debugging)
	// - otlp: send to OpenTelemetry Collector via OTLP (production setup)
	// - none: traces generated but not exported (useful for testing instrumentation)
	Exporter string

	// OTLPEndpoint is the OTLP gRPC endpoint URL (e.g., "localhost:4317")
	// Only used when Exporter is "otlp".
	OTLPEndpoint string

	// SampleRate controls what percentage of requests are traced (0.0 to 1.0).
	// - 1.0 (default): trace all requests
	// - 0.1: trace 10% of requests
	// - 0.0: trace nothing (effectively disables tracing)
	SampleRate float64
}

// DedupConfig holds configuration for the unified duplicate detection system.
// These thresholds control pg_trgm similarity scoring for flagging and auto-merging
// duplicate events, places, and organizations.
type DedupConfig struct {
	// NearDuplicateThreshold is the pg_trgm similarity threshold for
	// flagging potential duplicate events (same venue + date + similar name).
	// Range: 0.0-1.0. Lower = more flags, higher = fewer flags.
	// Default: 0.4
	NearDuplicateThreshold float64

	// PlaceReviewThreshold is the similarity threshold for flagging
	// a potential place duplicate for review. Default: 0.6
	PlaceReviewThreshold float64

	// PlaceAutoMergeThreshold is the similarity above which places
	// are auto-merged without review. Default: 0.95
	PlaceAutoMergeThreshold float64

	// OrgReviewThreshold is the similarity threshold for flagging
	// a potential organization duplicate for review. Default: 0.6
	OrgReviewThreshold float64

	// OrgAutoMergeThreshold is the similarity above which orgs
	// are auto-merged without review. Default: 0.95
	OrgAutoMergeThreshold float64
}

// ValidationConfig holds validation behavior configuration for event ingestion.
// These settings control quality checks and review queue routing.
type ValidationConfig struct {
	// RequireImage controls whether events must have an image to be published automatically.
	//
	// When false (default):
	//   - Events without images are published immediately
	//   - No quality warnings are generated for missing images
	//   - Confidence score is not reduced for missing images
	//   - Use this for data sources where images are optional or unavailable
	//
	// When true:
	//   - Events without images are sent to the review queue (lifecycle_state='pending_review')
	//   - A "missing_image" quality warning is added to the review queue entry
	//   - Confidence score is reduced by 0.2 (from baseline 1.0)
	//   - Admin must manually approve or reject the event via /admin/review-queue
	//   - Use this for high-quality feeds where images are expected
	//
	// Environment variable: VALIDATION_REQUIRE_IMAGE (default: false)
	//
	// Related code:
	//   - internal/domain/events/ingest.go:needsReview() - Review queue routing logic
	//   - internal/domain/events/ingest.go:reviewConfidence() - Confidence scoring
	//   - internal/domain/events/ingest.go:appendQualityWarnings() - Warning generation
	RequireImage bool
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
			GitHub: GitHubOAuthConfig{
				ClientID:     getEnv("GITHUB_CLIENT_ID", ""),
				ClientSecret: getEnv("GITHUB_CLIENT_SECRET", ""),
				CallbackURL:  getEnv("GITHUB_CALLBACK_URL", ""),
				AllowedOrgs:  parseCommaSeparated(getEnv("GITHUB_ALLOWED_ORGS", "")),
			},
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
		Validation: ValidationConfig{
			RequireImage: getEnvBool("VALIDATION_REQUIRE_IMAGE", false),
		},
		Tracing: TracingConfig{
			Enabled:      getEnvBool("TRACING_ENABLED", false),
			ServiceName:  getEnv("TRACING_SERVICE_NAME", "togather-sel-server"),
			Exporter:     getEnv("TRACING_EXPORTER", "stdout"),
			OTLPEndpoint: getEnv("TRACING_OTLP_ENDPOINT", "localhost:4317"),
			SampleRate:   getEnvFloat("TRACING_SAMPLE_RATE", 1.0),
		},
		Dedup: DedupConfig{
			NearDuplicateThreshold:  getEnvFloat("DEDUP_NEAR_DUPLICATE_THRESHOLD", 0.4),
			PlaceReviewThreshold:    getEnvFloat("DEDUP_PLACE_REVIEW_THRESHOLD", 0.6),
			PlaceAutoMergeThreshold: getEnvFloat("DEDUP_PLACE_AUTO_MERGE_THRESHOLD", 0.95),
			OrgReviewThreshold:      getEnvFloat("DEDUP_ORG_REVIEW_THRESHOLD", 0.6),
			OrgAutoMergeThreshold:   getEnvFloat("DEDUP_ORG_AUTO_MERGE_THRESHOLD", 0.95),
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

func getEnvFloat(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

// parseTrustedProxies parses a comma-separated list of CIDR ranges
func parseTrustedProxies(value string) []string {
	return parseCommaSeparated(value)
}

// parseCommaSeparated parses a comma-separated list of strings
func parseCommaSeparated(value string) []string {
	if value == "" {
		return nil
	}

	var items []string
	for _, item := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
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
