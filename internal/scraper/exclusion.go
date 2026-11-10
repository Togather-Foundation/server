package scraper

import (
	"fmt"
	"regexp"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/rs/zerolog"
)

type exclusionRule struct {
	field   string
	pattern string
	re      *regexp.Regexp
}

func compileExclusionRules(source SourceConfig) ([]exclusionRule, error) {
	var rules []exclusionRule
	for _, rule := range []exclusionRule{
		{field: "url_pattern", pattern: source.ExcludeURLPattern},
		{field: "title_pattern", pattern: source.ExcludeTitlePattern},
	} {
		if rule.pattern == "" {
			continue
		}
		re, err := regexp.Compile(rule.pattern)
		if err != nil {
			return nil, fmt.Errorf("compile exclusion %s: %w", rule.field, err)
		}
		rule.re = re
		rules = append(rules, rule)
	}
	return rules, nil
}

// excludeEvents counts each matching rule independently, dropping each event once.
// A single sample per pattern bounds log volume even for a broad exclusion.
func (s *Scraper) excludeEvents(input []events.EventInput, rules []exclusionRule, source string) []events.EventInput {
	return filterExcludedEvents(input, rules, source, s.logger)
}

func filterExcludedEvents(input []events.EventInput, rules []exclusionRule, source string, logger zerolog.Logger) []events.EventInput {
	if len(rules) == 0 {
		return input
	}
	counts := make([]int, len(rules))
	samples := make([]string, len(rules))
	sampleTitles := make([]string, len(rules))
	kept := make([]events.EventInput, 0, len(input))
	for _, event := range input {
		excluded := false
		for i, rule := range rules {
			value := event.URL
			if rule.field == "title_pattern" {
				value = event.Name
			}
			if rule.re.MatchString(value) {
				excluded = true
				counts[i]++
				if counts[i] == 1 {
					samples[i] = value
					sampleTitles[i] = event.Name
				}
			}
		}
		if !excluded {
			kept = append(kept, event)
		}
	}
	for i, rule := range rules {
		logger.Info().Str("source", source).Int("excluded", counts[i]).
			Str(rule.field, rule.pattern).Str("sample", samples[i]).Str("sample_title", sampleTitles[i]).
			Msgf("scraper: excluded: %d by %s=%s", counts[i], rule.field, rule.pattern)
	}
	return kept
}
