package events

import (
	"strings"
	"unicode"

	"github.com/Togather-Foundation/server/internal/config"
)

func normalizeLocationName(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func CheckGeographicBoundary(input EventInput, cfg config.GeographicBoundaryConfig) error {
	if len(cfg.Regions) == 0 && len(cfg.Localities) == 0 {
		return nil
	}
	if input.Location == nil {
		return nil
	}

	normalizedRegions := make(map[string]struct{}, len(cfg.Regions))
	for _, r := range cfg.Regions {
		normalizedRegions[normalizeLocationName(r)] = struct{}{}
	}
	normalizedLocalities := make(map[string]struct{}, len(cfg.Localities))
	for _, l := range cfg.Localities {
		normalizedLocalities[normalizeLocationName(l)] = struct{}{}
	}

	if len(cfg.Regions) > 0 && input.Location.AddressRegion != "" {
		normRegion := normalizeLocationName(input.Location.AddressRegion)
		if _, ok := normalizedRegions[normRegion]; !ok {
			return &ErrOutsideGeographicBoundary{
				Field:       "region",
				Value:       input.Location.AddressRegion,
				AllowedList: cfg.Regions,
			}
		}
	}

	if len(cfg.Localities) > 0 && input.Location.AddressLocality != "" {
		normLocality := normalizeLocationName(input.Location.AddressLocality)
		if _, ok := normalizedLocalities[normLocality]; !ok {
			return &ErrOutsideGeographicBoundary{
				Field:       "locality",
				Value:       input.Location.AddressLocality,
				AllowedList: cfg.Localities,
			}
		}
	}

	return nil
}
