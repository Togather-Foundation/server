# Multi-stage Dockerfile for Togather Server
# Builds a minimal, secure container with embedded version metadata

# ============================================================================
# Stage 1: Build Stage
# ============================================================================
FROM golang:1.25-alpine AS builder

# Install build dependencies (git for version detection, ca-certificates for HTTPS)
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory for build
WORKDIR /build

# Copy dependency manifests first for better layer caching
COPY go.mod go.sum ./

# Download dependencies (cached if go.mod/go.sum unchanged)
RUN go mod download

# Copy source code
COPY . .

# Build arguments for version metadata
ARG VERSION=dev
ARG GIT_COMMIT=unknown
# Accept BUILD_TIMESTAMP from deploy script, use it as BUILD_DATE for ldflags
ARG BUILD_TIMESTAMP=unknown
ARG BUILD_DATE=${BUILD_TIMESTAMP}
# Domain for generating environment-specific web files (robots.txt, sitemap.xml)
ARG DOMAIN=togather.foundation

# Create placeholder web files for initial build (required for go:embed directives)
# These will be replaced with environment-specific versions after the first build
RUN mkdir -p /build/web && \
    echo "# robots.txt placeholder" > /build/web/robots.txt && \
    echo "<?xml version=\"1.0\" encoding=\"UTF-8\"?><urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\"></urlset>" > /build/web/sitemap.xml

# Build the server binary first (needed for webfiles generation)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags "-s -w \
      -X 'github.com/Togather-Foundation/server/cmd/server/cmd.Version=${VERSION}' \
      -X 'github.com/Togather-Foundation/server/cmd/server/cmd.GitCommit=${GIT_COMMIT}' \
      -X 'github.com/Togather-Foundation/server/cmd/server/cmd.BuildDate=${BUILD_DATE}'" \
    -o /build/server \
    ./cmd/server

# Generate web files with environment-specific domain
# This embeds the correct domain into robots.txt and sitemap.xml before they're embedded via go:embed
RUN /build/server webfiles --domain="${DOMAIN}" --output=/build/web/ || \
    echo "Warning: webfiles generation failed, continuing with defaults"

# Rebuild the server binary to embed the generated web files via go:embed
# CGO_ENABLED=0: Build fully static binary (no C dependencies)
# -trimpath: Remove file system paths from binary for reproducibility
# -ldflags "-s -w": Strip debug info and symbol table for smaller binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags "-s -w \
      -X 'github.com/Togather-Foundation/server/cmd/server/cmd.Version=${VERSION}' \
      -X 'github.com/Togather-Foundation/server/cmd/server/cmd.GitCommit=${GIT_COMMIT}' \
      -X 'github.com/Togather-Foundation/server/cmd/server/cmd.BuildDate=${BUILD_DATE}'" \
    -o /build/server \
    ./cmd/server

# ============================================================================
# Stage 2: Runtime Stage
# ============================================================================
FROM alpine:latest

# Build arguments for runtime
ARG GIT_COMMIT=unknown

# Set as environment variable for template cache-busting
ENV BUILD_COMMIT=${GIT_COMMIT}

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    && update-ca-certificates

# Create non-root user and group
RUN addgroup -g 1000 togather && \
    adduser -D -u 1000 -G togather togather

# Create app directory
WORKDIR /app

# Copy binary from build stage
COPY --from=builder /build/server /app/server

# Copy timezone data for time zone support
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy admin templates from build stage (for admin UI)
COPY --from=builder /build/web/admin /app/web/admin

# Copy JSON-LD contexts from build stage (for JSON-LD export)
COPY --from=builder /build/contexts /app/contexts

# Copy OpenAPI specification from build stage (for /api/v1/openapi.json endpoint)
COPY --from=builder /build/docs/api /app/docs/api

# Set ownership to non-root user
RUN chown -R togather:togather /app

# Switch to non-root user
USER togather

# Expose application port
EXPOSE 8080

# Health check endpoint (adjust path based on your API)
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/app/server", "healthcheck"] || exit 1

# Set entrypoint
ENTRYPOINT ["/app/server"]

# Default command (can be overridden)
CMD ["serve"]
