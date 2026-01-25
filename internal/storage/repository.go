package storage

import (
	"context"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/domain/places"
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

type ProvenanceRepository interface{}

type FederationRepository interface{}

type AuthRepository interface{}
