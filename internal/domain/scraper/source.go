// Package scraper defines the domain types and interfaces for scraper source management.
package scraper

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a scraper source is not found.
var ErrNotFound = errors.New("scraper source not found")

// Source is the domain type for a scraper source configuration stored in the DB.
// It mirrors the scraper_sources table and maps to/from SourceConfig YAML files.
type Source struct {
	ID            int64
	Name          string
	URL           string
	Tier          int
	Schedule      string
	TrustLevel    int
	License       string
	Enabled       bool
	MaxPages      int
	Selectors     []byte // JSONB: encoded SelectorConfig; nil for Tier 0
	Notes         string
	LastScrapedAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// UpsertParams contains the fields used to create or update a scraper source.
type UpsertParams struct {
	Name          string
	URL           string
	Tier          int
	Schedule      string
	TrustLevel    int
	License       string
	Enabled       bool
	MaxPages      int
	Selectors     []byte
	Notes         string
	LastScrapedAt *time.Time
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

// Service provides business logic for scraper source management.
type Service struct {
	repo Repository
}

// NewService creates a new scraper source Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Upsert inserts or updates a scraper source.
func (s *Service) Upsert(ctx context.Context, params UpsertParams) (*Source, error) {
	return s.repo.Upsert(ctx, params)
}

// GetByName returns a source by name, or ErrNotFound.
func (s *Service) GetByName(ctx context.Context, name string) (*Source, error) {
	return s.repo.GetByName(ctx, name)
}

// List returns sources, filtered by enabled if non-nil.
func (s *Service) List(ctx context.Context, enabled *bool) ([]Source, error) {
	return s.repo.List(ctx, enabled)
}

// UpdateLastScraped marks a source as scraped now.
func (s *Service) UpdateLastScraped(ctx context.Context, name string) error {
	return s.repo.UpdateLastScraped(ctx, name)
}

// Delete removes a scraper source.
func (s *Service) Delete(ctx context.Context, name string) error {
	return s.repo.Delete(ctx, name)
}

// LinkToOrg links a source to an organization.
func (s *Service) LinkToOrg(ctx context.Context, orgID string, sourceID int64) error {
	return s.repo.LinkToOrg(ctx, orgID, sourceID)
}

// UnlinkFromOrg removes a source↔org link.
func (s *Service) UnlinkFromOrg(ctx context.Context, orgID string, sourceID int64) error {
	return s.repo.UnlinkFromOrg(ctx, orgID, sourceID)
}

// ListByOrg returns sources linked to an organization.
func (s *Service) ListByOrg(ctx context.Context, orgID string) ([]Source, error) {
	return s.repo.ListByOrg(ctx, orgID)
}

// LinkToPlace links a source to a place.
func (s *Service) LinkToPlace(ctx context.Context, placeID string, sourceID int64) error {
	return s.repo.LinkToPlace(ctx, placeID, sourceID)
}

// UnlinkFromPlace removes a source↔place link.
func (s *Service) UnlinkFromPlace(ctx context.Context, placeID string, sourceID int64) error {
	return s.repo.UnlinkFromPlace(ctx, placeID, sourceID)
}

// ListByPlace returns sources linked to a place.
func (s *Service) ListByPlace(ctx context.Context, placeID string) ([]Source, error) {
	return s.repo.ListByPlace(ctx, placeID)
}
