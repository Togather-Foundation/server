// Package scraper defines the domain types and interfaces for scraper source management.
package scraper

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrNotFound is returned when a scraper source is not found.
var ErrNotFound = errors.New("scraper source not found")

// Source is the domain type for a scraper source configuration stored in the DB.
// It mirrors the scraper_sources table and maps to/from SourceConfig YAML files.
type Source struct {
	ID                            int64
	Name                          string
	URL                           string
	URLs                          []string
	Tier                          int
	Schedule                      string
	TrustLevel                    int
	License                       string
	Enabled                       bool
	MaxPages                      int
	Selectors                     []byte // JSONB: encoded SelectorConfig; nil for Tier 0
	Notes                         string
	EventURLPattern               string
	SkipMultiSessionCheck         bool
	MultiSessionDurationThreshold string
	FollowEventURLs               bool
	Timezone                      string
	LastScrapedAt                 *time.Time
	CreatedAt                     time.Time
	UpdatedAt                     time.Time
	// Headless fields (Tier 2)
	HeadlessWaitSelector    string
	HeadlessWaitTimeoutMs   int
	HeadlessPaginationBtn   string
	HeadlessHeaders         []byte // JSONB; nil when empty
	HeadlessRateLimitMs     int
	HeadlessWaitNetworkIdle bool
	HeadlessUndetected      bool
	HeadlessIframe          json.RawMessage
	HeadlessIntercept       json.RawMessage
	// GraphQL fields (Tier 3)
	GraphQLConfig []byte // JSONB-encoded GraphQLConfig; nil for non-Tier-3 sources
	// REST fields (Tier 3)
	RestConfig []byte // JSONB-encoded RestConfig; nil for non-Tier-3 REST sources
	// Sitemap fields (URL discovery)
	SitemapConfig []byte // JSONB-encoded SitemapConfig; nil for non-sitemap sources
}

// UpsertParams contains the fields used to create or update a scraper source.
type UpsertParams struct {
	Name                          string
	URL                           string
	URLs                          []string
	Tier                          int
	Schedule                      string
	TrustLevel                    int
	License                       string
	Enabled                       bool
	MaxPages                      int
	Selectors                     []byte
	Notes                         string
	EventURLPattern               string
	SkipMultiSessionCheck         bool
	MultiSessionDurationThreshold string
	FollowEventURLs               bool
	Timezone                      string
	LastScrapedAt                 *time.Time
	// Headless fields (Tier 2)
	HeadlessWaitSelector    string
	HeadlessWaitTimeoutMs   int
	HeadlessPaginationBtn   string
	HeadlessHeaders         []byte // JSONB; nil when empty
	HeadlessRateLimitMs     int
	HeadlessWaitNetworkIdle bool
	HeadlessUndetected      bool
	HeadlessIframe          json.RawMessage
	HeadlessIntercept       json.RawMessage
	// GraphQL fields (Tier 3)
	GraphQLConfig []byte // JSONB-encoded GraphQLConfig; nil for non-Tier-3 sources
	// REST fields (Tier 3)
	RestConfig []byte // JSONB-encoded RestConfig; nil for non-Tier-3 REST sources
	// Sitemap fields (URL discovery)
	SitemapConfig []byte // JSONB-encoded SitemapConfig; nil for non-sitemap sources
}

// Repository defines the persistence interface for scraper sources.
type Repository interface {
	// Upsert inserts or updates a scraper source by name (YAML sync).
	Upsert(ctx context.Context, params UpsertParams) (*Source, error)

	// GetByName returns a source by its unique name.
	GetByName(ctx context.Context, name string) (*Source, error)

	// List returns all sources, optionally filtered by enabled state.
	// Pass nil to return all sources regardless of enabled status.
	List(ctx context.Context, enabled *bool) ([]Source, error)

	// UpdateLastScraped sets last_scraped_at = NOW() for the named source.
	UpdateLastScraped(ctx context.Context, name string) error

	// Delete removes a source by name.
	Delete(ctx context.Context, name string) error

	// LinkToOrg associates a source with an organization.
	LinkToOrg(ctx context.Context, orgID string, sourceID int64) error

	// UnlinkFromOrg removes a source↔org association.
	UnlinkFromOrg(ctx context.Context, orgID string, sourceID int64) error

	// ListByOrg returns all sources linked to the given organization UUID.
	ListByOrg(ctx context.Context, orgID string) ([]Source, error)

	// LinkToPlace associates a source with a place.
	LinkToPlace(ctx context.Context, placeID string, sourceID int64) error

	// UnlinkFromPlace removes a source↔place association.
	UnlinkFromPlace(ctx context.Context, placeID string, sourceID int64) error

	// ListByPlace returns all sources linked to the given place UUID.
	ListByPlace(ctx context.Context, placeID string) ([]Source, error)
}
