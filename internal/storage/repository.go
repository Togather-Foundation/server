package storage

import (
	"context"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/federation"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/Togather-Foundation/server/internal/domain/provenance"
)

// Repository groups data access by domain.
type Repository interface {
	Events() EventRepository
	Places() PlaceRepository
	Organizations() OrganizationRepository
	Sources() SourceRepository
	Provenance() ProvenanceRepository
	Federation() FederationRepository
	Auth() AuthRepository

	WithTx(ctx context.Context, fn func(context.Context, Repository) error) error
}

type EventRepository = events.Repository

type PlaceRepository = places.Repository

type OrganizationRepository = organizations.Repository

type SourceRepository interface{}

type ProvenanceRepository = provenance.Repository

type FederationRepository = federation.Repository

type AuthRepository interface {
	APIKeys() APIKeyRepository
}

type APIKeyRepository interface {
	LookupByPrefix(ctx context.Context, prefix string) (*auth.APIKey, error)
	UpdateLastUsed(ctx context.Context, id string) error
}
