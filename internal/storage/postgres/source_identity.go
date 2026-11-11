package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/jackc/pgx/v5/pgconn"
)

func sourceIdentityError(feed string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "sources_base_url_name_unique" {
		return fmt.Errorf("source label collision for feed %q with a legacy row; run source reconcile and resolve the legacy label explicitly: %w", feed, err)
	}
	return fmt.Errorf("resolve canonical source for feed %q: %w", feed, err)
}

// Config-owned full-feed rows take precedence over historical generic rows.
// Conflicting configured policies require an operator decision, never an
// arbitrary oldest-row choice. Without configs, an exact requested policy may
// disambiguate legacy rows; otherwise differing policies remain unresolved.
func selectLegacyFeedSource(ctx context.Context, q *Queries, feed string, params events.SourceLookupParams) (*ListLegacyFeedSourcesRow, error) {
	candidates, err := q.ListLegacyFeedSources(ctx, feed)
	if err != nil {
		return nil, fmt.Errorf("list legacy feed sources: %w", err)
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	configured := candidates[0].Configured
	if configured {
		end := 0
		for end < len(candidates) && candidates[end].Configured {
			end++
		}
		candidates = candidates[:end]
	}
	first := candidates[0]
	policy := events.SourceLookupParams{TrustLevel: int(first.TrustLevel), LicenseURL: first.LicenseUrl, LicenseType: first.LicenseType}
	conflicting := false
	for _, candidate := range candidates[1:] {
		if !sourcePolicyMatches(candidate.TrustLevel, candidate.LicenseUrl, candidate.LicenseType, policy) {
			conflicting = true
			break
		}
	}
	if !conflicting {
		return &first, nil
	}
	if !configured {
		for _, candidate := range candidates {
			if sourcePolicyMatches(candidate.TrustLevel, candidate.LicenseUrl, candidate.LicenseType, params) {
				return &candidate, nil
			}
		}
	}
	return nil, fmt.Errorf("source policy conflict for feed %q: legacy rows have different trust/licenses; reconcile policy explicitly before adopting a source", feed)
}

func sourcePolicyMatches(trust int32, license, kind string, params events.SourceLookupParams) bool {
	return trust == int32(params.TrustLevel) && strings.EqualFold(kind, params.LicenseType) && sourceLicenseKey(license) == sourceLicenseKey(params.LicenseURL)
}

// Treat known SPDX-style aliases and their Creative Commons URLs equally;
// preserve variant/version distinctions such as BY 3.0 versus BY 4.0.
func sourceLicenseKey(license string) string {
	value := strings.TrimRight(strings.ToLower(strings.TrimSpace(license)), "/")
	if value == "cc0" || value == "cc0-1.0" || value == "https://creativecommons.org/publicdomain/zero/1.0" || value == "http://creativecommons.org/publicdomain/zero/1.0" {
		return "https://creativecommons.org/publicdomain/zero/1.0"
	}
	if strings.HasPrefix(value, "cc-by-") {
		last := strings.LastIndex(value, "-")
		version := value[last+1:]
		if len(version) > 0 && version[0] >= '0' && version[0] <= '9' {
			return "https://creativecommons.org/licenses/" + strings.TrimPrefix(value[:last], "cc-") + "/" + version
		}
	}
	if strings.HasPrefix(value, "http://creativecommons.org/") {
		return "https://" + strings.TrimPrefix(value, "http://")
	}
	return value
}
