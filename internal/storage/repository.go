package storage

import "context"

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

type EventRepository interface{}

type PlaceRepository interface{}

type OrganizationRepository interface{}

type SourceRepository interface{}

type ProvenanceRepository interface{}

type FederationRepository interface{}

type AuthRepository interface{}
