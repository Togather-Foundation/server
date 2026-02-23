package handlers

import (
	"context"
	"errors"

	domainScraper "github.com/Togather-Foundation/server/internal/domain/scraper"
)

// errStubNotImplemented is returned by stubScraperSourceRepo methods that are
// not expected to be called in a given test. Using a named sentinel makes
// unexpected-call failures easier to identify in test output.
var errStubNotImplemented = errors.New("stubScraperSourceRepo: method not implemented")

// stubScraperSourceRepo is a test double for domainScraper.Repository.
// Fields are set to nil by default, meaning calls return empty results.
type stubScraperSourceRepo struct {
	listByOrgFn   func(ctx context.Context, orgID string) ([]domainScraper.Source, error)
	listByPlaceFn func(ctx context.Context, placeID string) ([]domainScraper.Source, error)
}

func (s stubScraperSourceRepo) Upsert(_ context.Context, _ domainScraper.UpsertParams) (*domainScraper.Source, error) {
	return nil, errStubNotImplemented
}

func (s stubScraperSourceRepo) GetByName(_ context.Context, _ string) (*domainScraper.Source, error) {
	return nil, errStubNotImplemented
}

func (s stubScraperSourceRepo) List(_ context.Context, _ *bool) ([]domainScraper.Source, error) {
	return nil, errStubNotImplemented
}

func (s stubScraperSourceRepo) UpdateLastScraped(_ context.Context, _ string) error {
	return errStubNotImplemented
}

func (s stubScraperSourceRepo) Delete(_ context.Context, _ string) error {
	return errStubNotImplemented
}

func (s stubScraperSourceRepo) LinkToOrg(_ context.Context, _ string, _ int64) error {
	return errStubNotImplemented
}

func (s stubScraperSourceRepo) UnlinkFromOrg(_ context.Context, _ string, _ int64) error {
	return errStubNotImplemented
}

func (s stubScraperSourceRepo) ListByOrg(ctx context.Context, orgID string) ([]domainScraper.Source, error) {
	if s.listByOrgFn != nil {
		return s.listByOrgFn(ctx, orgID)
	}
	return nil, nil
}

func (s stubScraperSourceRepo) LinkToPlace(_ context.Context, _ string, _ int64) error {
	return errStubNotImplemented
}

func (s stubScraperSourceRepo) UnlinkFromPlace(_ context.Context, _ string, _ int64) error {
	return errStubNotImplemented
}

func (s stubScraperSourceRepo) ListByPlace(ctx context.Context, placeID string) ([]domainScraper.Source, error) {
	if s.listByPlaceFn != nil {
		return s.listByPlaceFn(ctx, placeID)
	}
	return nil, nil
}
