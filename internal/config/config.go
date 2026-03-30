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
	Server          ServerConfig
	Database        DatabaseConfig
	Auth            AuthConfig
	RateLimit       RateLimitConfig
	CORS            CORSConfig
	AdminBootstrap  AdminBootstrapConfig
	Jobs            JobsConfig
	Logging         LoggingConfig
	Email           EmailConfig
	Validation      ValidationConfig
	Tracing         TracingConfig
	Dedup           DedupConfig
	Geocoding       GeocodingConfig
	Artsdata        ArtsdataConfig
	Scraper         ScraperConfig
	Developer       DeveloperConfig
	Users           UsersConfig
	DefaultTimezone string
	Environment     string
}

// ScraperConfig holds configuration for the event scraper, including optional
// Tier 2 headless browser settings.
type ScraperConfig struct {
	// HeadlessEnabled controls whether Tier 2 headless browser scraping is active.
	// Environment variable: SCRAPER_HEADLESS_ENABLED (default: false)
	HeadlessEnabled bool

	// ChromePath overrides the Chromium binary path used by go-rod.
	// When empty, Rod uses its download-on-demand launcher.
	// Environment variable: SCRAPER_CHROME_PATH (default: "")
	ChromePath string

	// HeadlessMaxConc is the maximum number of concurrent browser sessions.
	// Environment variable: SCRAPER_HEADLESS_MAX_CONC (default: 2)
	HeadlessMaxConc int

	// PollBackoffStart is the initial delay before the first batch-status poll.
	// Environment variable: SCRAPER_POLL_BACKOFF_START_MS (default: 200)
	PollBackoffStart int

	// PollBackoffMax is the maximum delay between batch-status polls.
	// Environment variable: SCRAPER_POLL_BACKOFF_MAX_MS (default: 2000)
	PollBackoffMax int

	// PollTimeout is the maximum total time spent polling for a batch result.
	// Environment variable: SCRAPER_POLL_TIMEOUT_MS (default: 30000)
	PollTimeout int

	// HTTPClientTimeout is the HTTP client timeout for scraper requests.
	// Environment variable: SCRAPER_HTTP_CLIENT_TIMEOUT_MS (default: 30000)
	HTTPClientTimeout int
}

type ServerConfig struct {
	Host        string
	Port        int
	BaseURL     string
	PublicURL   string // Public-facing domain for invitation links (e.g., "toronto.togather.foundation")
	AdminLocale string // BCP 47 locale tag for admin UI date/time formatting (e.g., "en-CA", "en-US")
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
	PublicPerMinute        int
	AgentPerMinute         int
	AdminPerMinute         int
	LoginPer15Minutes      int      // Login attempts allowed per 15-minute window per IP
	FederationPerMinute    int      // Federation sync rate limit
	SubmissionsPerIPPer24h int      // Max URLs a single IP may submit via POST /scraper/submissions in 24 hours
	TrustedProxyCIDRs      []string // CIDRs of trusted proxies (e.g., load balancers) for X-Forwarded-For validation
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

// EmailConfig holds email service configuration
type EmailConfig struct {
	Enabled      bool   // Enable/disable email sending (useful for dev/test)
	Provider     string // Provider selects the email backend ("smtp" or "resend", default "resend")
	From         string // From email address (e.g., "noreply@togather.foundation")
	ResendAPIKey string // Resend API key (required when provider is "resend")
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

	// ReviewConfidenceThreshold is the minimum quality-confidence score [0.0–1.0] an
	// event must achieve to bypass the review queue.  Events that score below this
	// threshold are flagged for manual review and receive a "low_confidence" warning.
	// Environment variable: VALIDATION_REVIEW_CONFIDENCE_THRESHOLD (default: 0.6)
	ReviewConfidenceThreshold float64

	// MaxFutureDays is the maximum number of days in the future an event's start date
	// may be before it is sent to the review queue and receives a "too_far_future"
	// warning.  730 ≈ 2 years.
	// Environment variable: VALIDATION_MAX_FUTURE_DAYS (default: 730)
	MaxFutureDays int

	// MaxEventNameLength is the maximum byte length allowed for an event name in
	// admin update operations.
	// Environment variable: VALIDATION_MAX_EVENT_NAME_LENGTH (default: 500)
	MaxEventNameLength int

	// AllowTestDomains disables the example.com / images.example.com blocklist check.
	// Set to true only in test code. Never set via environment variable.
	// Zero value (false) activates the blocklist in production.
	AllowTestDomains bool
}

// WithDefaults returns a copy of ValidationConfig with zero-values replaced by
// their production defaults.  Call this in service constructors so that tests
// that only set RequireImage (the original field) continue to behave correctly
// even after new fields were added.
func (v ValidationConfig) WithDefaults() ValidationConfig {
	if v.ReviewConfidenceThreshold == 0 {
		v.ReviewConfidenceThreshold = 0.6
	}
	if v.MaxFutureDays == 0 {
		v.MaxFutureDays = 730
	}
	if v.MaxEventNameLength == 0 {
		v.MaxEventNameLength = 500
	}
	return v
}

// WithDefaults returns a copy of DeveloperConfig with zero-values replaced by
// their production defaults.  Call this in service constructors so that tests
// that use DeveloperConfig{} continue to behave correctly.
func (d DeveloperConfig) WithDefaults() DeveloperConfig {
	if d.PasswordMinLength == 0 {
		d.PasswordMinLength = 8
	}
	if d.PasswordMaxLength == 0 {
		d.PasswordMaxLength = 128
	}
	if d.UsageFlushTimeoutSeconds == 0 {
		d.UsageFlushTimeoutSeconds = 10
	}
	return d
}

// DeveloperConfig holds tunables for the developer account and API-key subsystem.
type DeveloperConfig struct {
	// PasswordMinLength is the minimum number of Unicode code points required for a
	// developer account password.  Developer passwords are intentionally less strict
	// than user passwords because developers typically use password managers.
	// Environment variable: DEVELOPER_PASSWORD_MIN_LENGTH (default: 8)
	PasswordMinLength int

	// PasswordMaxLength is the maximum number of Unicode code points allowed for a
	// developer account password.  Consistent with NIST SP 800-63B upper bound.
	// Environment variable: DEVELOPER_PASSWORD_MAX_LENGTH (default: 128)
	PasswordMaxLength int

	// UsageFlushTimeoutSeconds is the context timeout (in seconds) used when
	// flushing buffered API-key usage metrics to the database.
	// Environment variable: DEVELOPER_USAGE_FLUSH_TIMEOUT_SECONDS (default: 10)
	UsageFlushTimeoutSeconds int
}

// WithDefaults returns a copy of UsersConfig with zero-values replaced by
// their production defaults.  Call this in service constructors so that tests
// that use UsersConfig{} continue to behave correctly.
func (u UsersConfig) WithDefaults() UsersConfig {
	if u.PasswordMinLength == 0 {
		u.PasswordMinLength = 12
	}
	if u.PasswordMaxLength == 0 {
		u.PasswordMaxLength = 128
	}
	return u
}

// UsersConfig holds tunables for the end-user account subsystem.
type UsersConfig struct {
	// PasswordMinLength is the minimum byte length required for a user account
	// password.  Follows NIST SP 800-63B guidelines; higher than developer minimum
	// because regular users are less likely to use password managers.
	// Environment variable: USERS_PASSWORD_MIN_LENGTH (default: 12)
	PasswordMinLength int

	// PasswordMaxLength is the maximum byte length allowed for a user account
	// password.  Consistent with NIST SP 800-63B upper bound.
	// Environment variable: USERS_PASSWORD_MAX_LENGTH (default: 128)
	PasswordMaxLength int
}

// GeocodingConfig holds configuration for the Nominatim geocoding client and cache.
type GeocodingConfig struct {
	// NominatimAPIURL is the base URL for the Nominatim API
	NominatimAPIURL string
	// NominatimUserEmail is included in User-Agent header per OSM usage policy
	NominatimUserEmail string
	// NominatimRateLimitPerSec controls max requests per second to Nominatim (default: 1.0)
	NominatimRateLimitPerSec float64
	// NominatimTimeoutSeconds is the HTTP request timeout in seconds (default: 5)
	NominatimTimeoutSeconds int
	// CacheTTLDays is the TTL for successful geocoding results (default: 30)
	CacheTTLDays int
	// FailureTTLDays is the TTL for failed geocoding attempts (default: 7)
	FailureTTLDays int
	// PopularPreserveCount is the number of top queries to preserve past TTL (default: 10000)
	PopularPreserveCount int
	// DefaultCountry is the default country code for geocoding queries (default: "ca")
	DefaultCountry string
}

// ArtsdataConfig holds configuration for Artsdata knowledge graph reconciliation.
type ArtsdataConfig struct {
	// Endpoint is the W3C Reconciliation API endpoint (default: "https://api.artsdata.ca/recon")
	Endpoint string
	// Enabled controls whether Artsdata reconciliation is active (default: false)
	Enabled bool
	// RateLimitPerSec is the max requests per second (default: 1.0)
	RateLimitPerSec float64
	// TimeoutSeconds is the HTTP request timeout in seconds (default: 10)
	TimeoutSeconds int
	// CacheTTLDays is the TTL for successful reconciliation results (default: 30)
	CacheTTLDays int
	// FailureTTLDays is the TTL for negative/failed reconciliation attempts (default: 7)
	FailureTTLDays int
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
			Host:        getEnv("SERVER_HOST", "0.0.0.0"),
			Port:        getEnvInt("SERVER_PORT", 8080),
			BaseURL:     getEnv("SERVER_BASE_URL", "http://localhost:8080"),
			PublicURL:   getEnv("PUBLIC_URL", "localhost:8080"),
			AdminLocale: getEnv("ADMIN_LOCALE", "en-CA"),
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
			PublicPerMinute:        getEnvInt("RATE_LIMIT_PUBLIC", 60),
			AgentPerMinute:         getEnvInt("RATE_LIMIT_AGENT", 300),
			AdminPerMinute:         getEnvInt("RATE_LIMIT_ADMIN", 0),
			LoginPer15Minutes:      getEnvInt("RATE_LIMIT_LOGIN", 5),
			FederationPerMinute:    getEnvInt("RATE_LIMIT_FEDERATION", 500),
			SubmissionsPerIPPer24h: getEnvInt("RATE_LIMIT_SUBMISSIONS_PER_IP_PER_24H", 20),
			TrustedProxyCIDRs:      parseTrustedProxies(getEnv("TRUSTED_PROXY_CIDRS", "")),
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
			Provider:     getEnv("EMAIL_PROVIDER", "smtp"),
			From:         getEnv("EMAIL_FROM", "noreply@togather.foundation"),
			ResendAPIKey: getEnv("RESEND_API_KEY", ""),
			SMTPHost:     getEnv("SMTP_HOST", "smtp.gmail.com"),
			SMTPPort:     getEnvInt("SMTP_PORT", 587),
			SMTPUser:     getEnv("SMTP_USER", ""),
			SMTPPassword: getEnv("SMTP_PASSWORD", ""),
			TemplatesDir: getEnv("EMAIL_TEMPLATES_DIR", "web/email/templates"),
		},
		Validation: ValidationConfig{
			RequireImage:              getEnvBool("VALIDATION_REQUIRE_IMAGE", false),
			ReviewConfidenceThreshold: getEnvFloat("VALIDATION_REVIEW_CONFIDENCE_THRESHOLD", 0.6),
			MaxFutureDays:             getEnvInt("VALIDATION_MAX_FUTURE_DAYS", 730),
			MaxEventNameLength:        getEnvInt("VALIDATION_MAX_EVENT_NAME_LENGTH", 500),
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
		Geocoding: GeocodingConfig{
			NominatimAPIURL:          getEnv("NOMINATIM_API_URL", "https://nominatim.openstreetmap.org"),
			NominatimUserEmail:       getEnv("NOMINATIM_USER_EMAIL", "nominatim@togather.foundation"),
			NominatimRateLimitPerSec: getEnvFloat("NOMINATIM_RATE_LIMIT_PER_SEC", 1.0),
			NominatimTimeoutSeconds:  getEnvInt("NOMINATIM_TIMEOUT_SECONDS", 5),
			CacheTTLDays:             getEnvInt("GEOCODING_CACHE_TTL_DAYS", 30),
			FailureTTLDays:           getEnvInt("GEOCODING_FAILURE_TTL_DAYS", 7),
			PopularPreserveCount:     getEnvInt("GEOCODING_POPULAR_PRESERVE_COUNT", 10000),
			DefaultCountry:           getEnv("GEOCODING_DEFAULT_COUNTRY", "ca"),
		},
		Artsdata: ArtsdataConfig{
			Endpoint:        getEnv("ARTSDATA_ENDPOINT", "https://api.artsdata.ca/recon"),
			Enabled:         getEnvBool("ARTSDATA_ENABLED", false),
			RateLimitPerSec: getEnvFloat("ARTSDATA_RATE_LIMIT_PER_SEC", 1.0),
			TimeoutSeconds:  getEnvInt("ARTSDATA_TIMEOUT_SECONDS", 10),
			CacheTTLDays:    getEnvInt("ARTSDATA_CACHE_TTL_DAYS", 30),
			FailureTTLDays:  getEnvInt("ARTSDATA_FAILURE_TTL_DAYS", 7),
		},
		Scraper: ScraperConfig{
			HeadlessEnabled:   getEnvBool("SCRAPER_HEADLESS_ENABLED", false),
			ChromePath:        getEnv("SCRAPER_CHROME_PATH", ""),
			HeadlessMaxConc:   getEnvInt("SCRAPER_HEADLESS_MAX_CONC", 2),
			PollBackoffStart:  getEnvInt("SCRAPER_POLL_BACKOFF_START_MS", 200),
			PollBackoffMax:    getEnvInt("SCRAPER_POLL_BACKOFF_MAX_MS", 2000),
			PollTimeout:       getEnvInt("SCRAPER_POLL_TIMEOUT_MS", 30000),
			HTTPClientTimeout: getEnvInt("SCRAPER_HTTP_CLIENT_TIMEOUT_MS", 30000),
		},
		Developer: DeveloperConfig{
			PasswordMinLength:        getEnvInt("DEVELOPER_PASSWORD_MIN_LENGTH", 8),
			PasswordMaxLength:        getEnvInt("DEVELOPER_PASSWORD_MAX_LENGTH", 128),
			UsageFlushTimeoutSeconds: getEnvInt("DEVELOPER_USAGE_FLUSH_TIMEOUT_SECONDS", 10),
		},
		Users: UsersConfig{
			PasswordMinLength: getEnvInt("USERS_PASSWORD_MIN_LENGTH", 12),
			PasswordMaxLength: getEnvInt("USERS_PASSWORD_MAX_LENGTH", 128),
		},
		DefaultTimezone: getEnv("DEFAULT_TIMEZONE", "America/Toronto"),
		Environment:     getEnv("ENVIRONMENT", "development"),
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
