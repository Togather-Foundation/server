package scraper

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/rs/zerolog"
)

// ErrNoExtractorConfig is returned by NewExtractor when a source has neither
// a REST nor GraphQL configuration.
var ErrNoExtractorConfig = errors.New("no REST or GraphQL config for source")

// Extractor fetches events from an API endpoint. RestExtractor and
// GraphQLExtractor both satisfy this interface; scrapeTier3 dispatches
// through it so that adding a new API variant requires only a new
// implementation—no changes to the scraper core.
//
// Tier 3 extractors use their own endpoint config (RestConfig.Endpoint or
// GraphQLConfig.Endpoint), not SourceConfig.URL which is retained only as
// metadata in scraper_run records.
type Extractor interface {
	Extract(ctx context.Context, source SourceConfig, client *http.Client) ([]RawEvent, error)
}

// NewExtractor returns the appropriate Extractor for the given source
// configuration. REST takes precedence when both configs are present
// (matching the existing scrapeTier3 dispatch logic).
func NewExtractor(source SourceConfig, logger zerolog.Logger) (Extractor, error) {
	switch {
	case source.REST != nil:
		return NewRestExtractor(logger), nil
	case source.GraphQL != nil:
		return NewGraphQLExtractor(logger), nil
	default:
		return nil, fmt.Errorf("%w %q", ErrNoExtractorConfig, source.Name)
	}
}
