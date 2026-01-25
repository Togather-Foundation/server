package provenance

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("source not found")

type Source struct {
	ID          string
	Name        string
	SourceType  string
	BaseURL     string
	LicenseURL  string
	LicenseType string
	TrustLevel  int
	IsActive    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CreateSourceParams struct {
	Name        string
	SourceType  string
	BaseURL     string
	LicenseURL  string
	LicenseType string
	TrustLevel  int
}

type Repository interface {
	GetByBaseURL(ctx context.Context, baseURL string) (*Source, error)
	Create(ctx context.Context, params CreateSourceParams) (*Source, error)
}
