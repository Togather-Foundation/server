package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// SourceRepairConfig is a config's identity and policy, supplied from YAML or DB.
// Reconciliation never changes the config or its warning suppressions.
type SourceRepairConfig struct {
	Name                   string
	URLs                   []string
	TrustLevel             int
	License                string
	SuppressReviewWarnings []string
}

type SourceRepairOptions struct {
	Apply          bool
	Configs        []SourceRepairConfig
	UseConfigTrust bool
}

type SourcePreflightPolicy struct {
	TrustLevel  int    `json:"trustLevel"`
	LicenseURL  string `json:"licenseUrl"`
	LicenseType string `json:"licenseType"`
}

type SourcePreflightEntry struct {
	ConfigName string                 `json:"configName"`
	Feed       string                 `json:"feed"`
	SourceID   string                 `json:"sourceId,omitempty"`
	Stored     *SourcePreflightPolicy `json:"stored,omitempty"`
	Configured SourcePreflightPolicy  `json:"configured"`
	Conflicts  []string               `json:"conflicts"`
}

type SourcePreflightReport struct {
	Entries   []SourcePreflightEntry `json:"entries"`
	Conflicts int                    `json:"conflicts"`
}

// PreflightSourcePolicies compares incoming YAML against the registry, without
// registering feeds or changing labels. Persisted scraper configs are ownership
// evidence for legacy selection, not desired policy: YAML may intentionally change.
func PreflightSourcePolicies(ctx context.Context, pool *pgxpool.Pool, configs []SourceRepairConfig) (*SourcePreflightReport, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin source preflight: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := New(tx)
	registry, err := q.ListSourcePreflightRegistry(ctx)
	if err != nil {
		return nil, fmt.Errorf("list source preflight registry: %w", err)
	}
	byFeed := map[string][]SourceRepairConfig{}
	for _, cfg := range configs {
		for _, raw := range cfg.URLs {
			feed := events.NormalizeFeedURL(raw)
			byFeed[feed] = append(byFeed[feed], cfg)
		}
	}
	feeds := make([]string, 0, len(byFeed))
	for feed := range byFeed {
		feeds = append(feeds, feed)
	}
	sort.Strings(feeds)
	report := &SourcePreflightReport{Entries: []SourcePreflightEntry{}}
	for _, feed := range feeds {
		group := byFeed[feed]
		sort.SliceStable(group, func(i, j int) bool { return group[i].Name < group[j].Name })
		for _, cfg := range group {
			params := events.SourceParamsForFeed(cfg.Name, feed, cfg.License, cfg.TrustLevel)
			entry := SourcePreflightEntry{ConfigName: cfg.Name, Feed: feed, Configured: SourcePreflightPolicy{params.TrustLevel, params.LicenseURL, params.LicenseType}, Conflicts: []string{}}
			parsed, parseErr := url.Parse(feed)
			if parseErr != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
				entry.Conflicts = append(entry.Conflicts, "invalid_feed")
			} else {
				for _, other := range group {
					if repairPolicy(other) != repairPolicy(cfg) {
						entry.Conflicts = append(entry.Conflicts, "configured_policy_conflict")
						break
					}
				}
				var source *ListSourcePreflightRegistryRow
				for _, row := range registry {
					if row.FeedUrl.Valid && row.FeedUrl.String == feed {
						copy := row
						source = &copy
						break
					}
				}
				if source == nil {
					candidate, selectErr := selectLegacyFeedSource(ctx, q, feed, params)
					if selectErr != nil {
						// Distinguish policy ambiguity from infrastructure/query failure.
						if !strings.HasPrefix(selectErr.Error(), "source policy conflict") {
							return nil, selectErr
						}
						entry.Conflicts = append(entry.Conflicts, "canonical_ownership_conflict")
					} else if candidate != nil {
						entry.SourceID = candidate.SID
						entry.Stored = &SourcePreflightPolicy{int(candidate.TrustLevel), candidate.LicenseUrl, candidate.LicenseType}
					}
				} else {
					entry.SourceID = source.ID.String()
					entry.Stored = &SourcePreflightPolicy{int(source.TrustLevel), source.LicenseUrl, source.LicenseType}
				}
				if entry.Stored != nil {
					if entry.Stored.TrustLevel != params.TrustLevel {
						entry.Conflicts = append(entry.Conflicts, "trust_mismatch")
					}
					if !sourcePolicyMatches(int32(params.TrustLevel), entry.Stored.LicenseURL, entry.Stored.LicenseType, params) {
						entry.Conflicts = append(entry.Conflicts, "license_mismatch")
					}
				}
				// A label shared by another legacy row with the same exact base URL
				// would reject the relabel/insert even when policy otherwise agrees.
				for _, row := range registry {
					if row.Name == cfg.Name && row.BaseUrl.Valid && row.BaseUrl.String == feed && row.ID.String() != entry.SourceID {
						entry.Conflicts = append(entry.Conflicts, "canonical_ownership_conflict")
						break
					}
				}
			}
			if len(entry.Conflicts) > 0 {
				report.Conflicts++
			}
			entry.Feed = redactSourcePreflightURL(entry.Feed)
			entry.Configured.LicenseURL = redactSourcePreflightURL(entry.Configured.LicenseURL)
			if entry.Stored != nil {
				entry.Stored.LicenseURL = redactSourcePreflightURL(entry.Stored.LicenseURL)
			}
			report.Entries = append(report.Entries, entry)
		}
	}
	return report, nil
}

func redactSourcePreflightURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return "[invalid URL]"
	}
	if parsed.Scheme == "" {
		return value
	} // SPDX license aliases.
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

type SourceTrustChange struct {
	SourceID string `json:"sourceId"`
	Name     string `json:"name"`
	Feed     string `json:"feed"`
	OldTrust int32  `json:"oldTrust"`
	NewTrust int32  `json:"newTrust"`
}

type SourceRepairCounts struct {
	ArchivedAttributions     int64 `json:"archivedAttributions"`
	NoncanonicalAttributions int   `json:"noncanonicalAttributions"`
	UnregisteredAttributions int   `json:"unregisteredAttributions"`
	FragmentedFeeds          int   `json:"fragmentedFeeds"`
	SourceRows               int   `json:"sourceRows"`
	AttributionRows          int   `json:"attributionRows"`
	ZeroEventRows            int   `json:"zeroEventRows"`
	ConfiguredIdle           int   `json:"configuredIdle"`
	OrphanedIdle             int   `json:"orphanedIdle"`
	MetadataProtected        int   `json:"metadataProtected"`
}

type SourceRepairReport struct {
	TrustChanges         []SourceTrustChange `json:"trustChanges"`
	TrustChangesCount    int                 `json:"trustChangesCount"`
	AttributionsArchived int64               `json:"attributionsArchived"`
	Applied              bool                `json:"applied"`
	Before               SourceRepairCounts  `json:"before"`
	After                SourceRepairCounts  `json:"after"`
	AttributionsMoved    int64               `json:"attributionsMoved"`
	FieldsMoved          int64               `json:"fieldsMoved"`
	ChangesMoved         int64               `json:"changesMoved"`
	ConfiguredIdle       int                 `json:"configuredIdle"`
	OrphanedIdle         int                 `json:"orphanedIdle"`
	MetadataProtected    int                 `json:"metadataProtected"`
	Deleted              int64               `json:"deleted"`
	Issues               []string            `json:"issues"`
	Query                string              `json:"query"`
}

func sourceRepairConfigs(ctx context.Context, q *Queries, supplied []SourceRepairConfig) ([]SourceRepairConfig, error) {
	rows, err := q.ListSourceRepairConfigs(ctx)
	if err != nil {
		return nil, fmt.Errorf("load source repair configs: %w", err)
	}
	configs := append([]SourceRepairConfig(nil), supplied...)
	for _, row := range rows {
		urls := row.Urls
		if len(urls) == 0 {
			urls = []string{row.Url}
		}
		configs = append(configs, SourceRepairConfig{Name: row.Name, URLs: urls, TrustLevel: int(row.TrustLevel), License: row.License, SuppressReviewWarnings: row.SuppressReviewWarnings})
	}
	sort.SliceStable(configs, func(i, j int) bool { return configs[i].Name < configs[j].Name })
	return configs, nil
}

func sourceRepairConfigured(row Source, configs []SourceRepairConfig) bool {
	for _, cfg := range configs {
		if cfg.Name == row.Name {
			return true
		}
		for _, raw := range cfg.URLs {
			feed := events.NormalizeFeedURL(raw)
			if feed != "" && (feed == events.NormalizeFeedURL(row.BaseUrl.String) || feed == row.FeedUrl.String) {
				return true
			}
		}
	}
	return false
}

func sourceRepairCounts(rows []ListSourceRepairRegistryRow, attrs []ListSourceRepairAttributionsRow, configs []SourceRepairConfig) SourceRepairCounts {
	counts := SourceRepairCounts{SourceRows: len(rows), AttributionRows: len(attrs)}
	canonical := map[string]pgtype.UUID{}
	for _, row := range rows {
		if row.Source.FeedUrl.Valid {
			canonical[row.Source.FeedUrl.String] = row.Source.ID
		}
	}
	feeds := map[string]map[pgtype.UUID]bool{}
	for _, item := range attrs {
		a := item.EventSource
		feed := events.NormalizeFeedURL(a.SourceUrl)
		if feeds[feed] == nil {
			feeds[feed] = map[pgtype.UUID]bool{}
		}
		feeds[feed][a.SourceID] = true
		if target, ok := canonical[feed]; !ok {
			counts.UnregisteredAttributions++
		} else if target != a.SourceID {
			counts.NoncanonicalAttributions++
		}
	}
	for _, sources := range feeds {
		if len(sources) > 1 {
			counts.FragmentedFeeds++
		}
	}

	for _, row := range rows {
		if row.EventReferences != 0 {
			continue
		}
		counts.ZeroEventRows++
		if sourceRepairConfigured(row.Source, configs) {
			counts.ConfiguredIdle++
		} else {
			counts.OrphanedIdle++
			if row.MetadataProtected || row.OtherReferences {
				counts.MetadataProtected++
			}
		}
	}
	return counts
}

func sourceRepairSnapshot(ctx context.Context, q *Queries, configs []SourceRepairConfig) (SourceRepairCounts, error) {
	rows, err := q.ListSourceRepairRegistry(ctx)
	if err != nil {
		return SourceRepairCounts{}, fmt.Errorf("list source registry: %w", err)
	}
	attrs, err := q.ListSourceRepairAttributions(ctx)
	if err != nil {
		return SourceRepairCounts{}, fmt.Errorf("list attributions: %w", err)
	}
	counts := sourceRepairCounts(rows, attrs, configs)
	archived, err := q.CountSourceRepairArchivedAttributions(ctx)
	if err != nil {
		return SourceRepairCounts{}, fmt.Errorf("count archived attributions: %w", err)
	}
	counts.ArchivedAttributions = archived
	return counts, nil
}

// InspectSourceRegistry classifies every zero-event row without changing data.
func InspectSourceRegistry(ctx context.Context, pool *pgxpool.Pool, options SourceRepairOptions) (SourceRepairCounts, error) {
	q := New(pool)
	configs, err := sourceRepairConfigs(ctx, q, options.Configs)
	if err != nil {
		return SourceRepairCounts{}, err
	}
	return sourceRepairSnapshot(ctx, q, configs)
}

func repairPolicy(cfg SourceRepairConfig) string {
	data, _ := json.Marshal([]any{cfg.TrustLevel, sourceLicenseKey(cfg.License)})
	return string(data)
}

// ReconcileSources executes the same transaction in preview and apply mode.
// Preview rolls it all back, including canonical source registration.
func ReconcileSources(ctx context.Context, pool *pgxpool.Pool, options SourceRepairOptions) (*SourceRepairReport, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin source reconciliation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := New(tx)
	if err = q.LockSourceRegistryExclusive(ctx); err != nil {
		return nil, fmt.Errorf("lock source registry: %w", err)
	}
	if err = q.LockSourceRepairTables(ctx); err != nil {
		return nil, fmt.Errorf("lock source reconciliation: %w", err)
	}
	configs, err := sourceRepairConfigs(ctx, q, options.Configs)
	if err != nil {
		return nil, err
	}
	report := &SourceRepairReport{Applied: options.Apply, Issues: []string{}, TrustChanges: []SourceTrustChange{}, Query: useConfiguredSourceTrust + "\n" + moveSourceRepairAttribution + "\n" + archiveSourceRepairAttribution + "\n" + deleteArchivedSourceRepairAttribution + "\n" + moveSourceRepairFields + "\n" + countSourceRepairRetainedFields + "\n" + moveSourceRepairChanges}
	report.Before, err = sourceRepairSnapshot(ctx, q, configs)
	if err != nil {
		return nil, err
	}
	byFeed := map[string][]SourceRepairConfig{}
	for _, cfg := range configs {
		for _, raw := range cfg.URLs {
			feed := events.NormalizeFeedURL(raw)
			if feed != "" {
				byFeed[feed] = append(byFeed[feed], cfg)
			}
		}
	}
	feeds := make([]string, 0, len(byFeed))
	for feed := range byFeed {
		feeds = append(feeds, feed)
	}
	sort.Strings(feeds)
	blockedFeeds := map[string]bool{}
	repo := &EventRepository{pool: pool, tx: tx, logger: zerolog.Nop()}
	for _, feed := range feeds {
		group := byFeed[feed]
		cfg := group[0]
		conflict := false
		for _, other := range group[1:] {
			if repairPolicy(other) != repairPolicy(cfg) {
				conflict = true
			}
		}
		if conflict {
			blockedFeeds[feed] = true
			report.Issues = append(report.Issues, "conflicting configured policies; retained unchanged: "+feed)
			continue
		}
		params := events.SourceParamsForFeed(cfg.Name, feed, cfg.License, cfg.TrustLevel)
		params.SourceType = "scraper"
		_, err = repo.GetOrCreateSource(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("register repair feed %q: %w", feed, err)
		}
	}
	registry, err := q.ListSourceRepairRegistry(ctx)
	if err != nil {
		return nil, fmt.Errorf("list repair registry: %w", err)
	}
	targets := map[string]pgtype.UUID{}
	for _, row := range registry {
		if row.Source.FeedUrl.Valid && !blockedFeeds[row.Source.FeedUrl.String] {
			if group := byFeed[row.Source.FeedUrl.String]; len(group) > 0 {
				params := events.SourceParamsForFeed(group[0].Name, row.Source.FeedUrl.String, group[0].License, group[0].TrustLevel)
				matches, change, trustErr := useConfiguredRepairTrust(ctx, q, row.Source, params, options.UseConfigTrust)
				if trustErr != nil {
					return nil, trustErr
				}
				if !matches {
					report.Issues = append(report.Issues, "canonical policy differs from configured feed; retained unchanged: "+row.Source.FeedUrl.String)
					continue
				}
				if change != nil {
					report.TrustChanges = append(report.TrustChanges, *change)
					report.TrustChangesCount++
				}
			}
			targets[row.Source.FeedUrl.String] = row.Source.ID
		}
	}
	attrs, err := q.ListSourceRepairAttributions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list repair attributions: %w", err)
	}
	if err = registerAttributedRepairFeeds(ctx, repo, configs, registry, attrs, targets, blockedFeeds, report, options.UseConfigTrust); err != nil {
		return nil, err
	}
	// Compute event+old-source decisions before moving anything. Audit records
	// have no feed URL: only an unambiguous single target can safely replace it.
	type pair struct{ event, source pgtype.UUID }
	decisions := map[pair]map[pgtype.UUID]bool{}
	occupied := map[string]pgtype.UUID{}
	key := func(event, source pgtype.UUID, url string) string {
		return event.String() + "|" + source.String() + "|" + url
	}
	for _, item := range attrs {
		a := item.EventSource
		occupied[key(a.EventID, a.SourceID, a.SourceUrl)] = a.ID
		target := a.SourceID
		if candidate, ok := targets[events.NormalizeFeedURL(a.SourceUrl)]; ok {
			target = candidate
		}
		p := pair{a.EventID, a.SourceID}
		if decisions[p] == nil {
			decisions[p] = map[pgtype.UUID]bool{}
		}
		decisions[p][target] = true
	}
	for _, item := range attrs {
		a := item.EventSource
		target, ok := targets[events.NormalizeFeedURL(a.SourceUrl)]
		if !ok || target == a.SourceID {
			continue
		}
		if canonicalID, collision := occupied[key(a.EventID, target, a.SourceUrl)]; collision {
			archived, archiveErr := q.ArchiveSourceRepairAttribution(ctx, ArchiveSourceRepairAttributionParams{OriginalAttributionID: a.ID, CanonicalAttributionID: canonicalID})
			if archiveErr != nil {
				return nil, fmt.Errorf("archive colliding source attribution: %w", archiveErr)
			}
			if archived != 1 {
				return nil, fmt.Errorf("archive colliding source attribution %s: original snapshot already exists or row disappeared", a.ID.String())
			}
			deleted, deleteErr := q.DeleteArchivedSourceRepairAttribution(ctx, a.ID)
			if deleteErr != nil {
				return nil, fmt.Errorf("retire archived source attribution: %w", deleteErr)
			}
			if deleted != 1 {
				return nil, fmt.Errorf("retire archived source attribution %s: complete snapshot does not match", a.ID.String())
			}
			report.AttributionsArchived += archived
			continue
		}
		n, moveErr := q.MoveSourceRepairAttribution(ctx, MoveSourceRepairAttributionParams{TargetSource: target, AttributionID: a.ID})
		if moveErr != nil {
			return nil, fmt.Errorf("move source attribution: %w", moveErr)
		}
		report.AttributionsMoved += n
		occupied[key(a.EventID, target, a.SourceUrl)] = a.ID
	}
	for p, choices := range decisions {
		if len(choices) != 1 {
			report.Issues = append(report.Issues, "ambiguous field/change provenance retained: "+p.event.String()+"/"+p.source.String())
			continue
		}
		var target pgtype.UUID
		for choice := range choices {
			target = choice
		}
		if target == p.source {
			continue
		}
		n, moveErr := q.MoveSourceRepairFields(ctx, MoveSourceRepairFieldsParams{TargetSource: target, EventID: p.event, OldSource: p.source})
		if moveErr != nil {
			return nil, fmt.Errorf("move field provenance: %w", moveErr)
		}
		report.FieldsMoved += n
		retained, countErr := q.CountSourceRepairRetainedFields(ctx, CountSourceRepairRetainedFieldsParams{TargetSource: target, EventID: p.event, OldSource: p.source})
		if countErr != nil {
			return nil, fmt.Errorf("count retained field provenance: %w", countErr)
		}
		if retained > 0 {
			report.Issues = append(report.Issues, fmt.Sprintf("%d field provenance collision(s) retained: %s/%s", retained, p.event.String(), p.source.String()))
		}

		n, moveErr = q.MoveSourceRepairChanges(ctx, MoveSourceRepairChangesParams{TargetSource: target, EventID: p.event, OldSource: p.source})
		if moveErr != nil {
			return nil, fmt.Errorf("move change provenance: %w", moveErr)
		}
		report.ChangesMoved += n
	}
	report.After, err = sourceRepairSnapshot(ctx, q, configs)
	if err != nil {
		return nil, err
	}
	if options.Apply {
		if err = tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit source reconciliation: %w", err)
		}
	}
	return report, nil
}

// Sitemap/detail scrapers attribute the fetched detail URL, which can differ
// from the configured listing URL. Recover ownership from historical explicit
// config labels or an exact full-feed row, never from a shared hostname.
func registerAttributedRepairFeeds(ctx context.Context, repo *EventRepository, configs []SourceRepairConfig, registry []ListSourceRepairRegistryRow, attrs []ListSourceRepairAttributionsRow, targets map[string]pgtype.UUID, blocked map[string]bool, report *SourceRepairReport, useConfigTrust bool) error {
	byName := map[string][]SourceRepairConfig{}
	for _, cfg := range configs {
		byName[cfg.Name] = append(byName[cfg.Name], cfg)
	}
	sources := map[pgtype.UUID]Source{}
	for _, row := range registry {
		sources[row.Source.ID] = row.Source
	}
	byFeed := map[string][]EventSource{}
	feedsBySource := map[pgtype.UUID]map[string]bool{}
	for _, item := range attrs {
		a := item.EventSource
		feed := events.NormalizeFeedURL(a.SourceUrl)
		byFeed[feed] = append(byFeed[feed], a)
		if feedsBySource[a.SourceID] == nil {
			feedsBySource[a.SourceID] = map[string]bool{}
		}
		feedsBySource[a.SourceID][feed] = true
	}
	feeds := make([]string, 0, len(byFeed))
	for feed := range byFeed {
		feeds = append(feeds, feed)
	}
	sort.Strings(feeds)
	for _, feed := range feeds {
		parsed, parseErr := url.Parse(feed)
		valid := parseErr == nil && parsed.Host != "" && (parsed.Scheme == "https" || parsed.Scheme == "http")
		if _, ok := targets[feed]; ok && valid {
			continue
		}
		reason := ""
		var lookup *events.SourceLookupParams
		switch {
		case !valid:
			delete(targets, feed)
			reason = "invalid absolute HTTP(S) URL"
		case blocked[feed]:
			reason = "conflicting configured feed policies"
		default:
			var policy *SourceRepairConfig
			for _, attribution := range byFeed[feed] {
				owner := sources[attribution.SourceID]
				for _, cfg := range byName[owner.Name] {
					candidate := events.SourceParamsForFeed(cfg.Name, feed, cfg.License, cfg.TrustLevel)
					candidate.SourceType = owner.SourceType
					if policy != nil && repairPolicy(*policy) != repairPolicy(cfg) {
						reason = "conflicting named config owner policies"
					}
					if !sourcePolicyMatches(owner.TrustLevel, owner.LicenseUrl, owner.LicenseType, candidate) && (!useConfigTrust || !sourcePolicyMatches(int32(candidate.TrustLevel), owner.LicenseUrl, owner.LicenseType, candidate)) {
						reason = "named owner source policy differs from configured policy"
					}
					if policy == nil {
						copy := cfg
						policy = &copy
						lookup = &candidate
					}
				}
			}
			// An exact legacy feed URL is evidence only when it is a full path
			// and the row has never attributed a different normalized feed.
			if policy == nil && parsed.Path != "" && parsed.Path != "/" {
				for _, row := range registry {
					owner := row.Source
					ownedFeeds := feedsBySource[owner.ID]
					if owner.FeedUrl.Valid || events.NormalizeFeedURL(owner.BaseUrl.String) != feed || len(ownedFeeds) > 1 || (len(ownedFeeds) == 1 && !ownedFeeds[feed]) {
						continue
					}
					candidate := events.SourceLookupParams{Name: owner.Name, SourceType: owner.SourceType, BaseURL: feed, FeedURL: feed, TrustLevel: int(owner.TrustLevel), LicenseURL: owner.LicenseUrl, LicenseType: owner.LicenseType}
					if lookup != nil && !sourcePolicyMatches(owner.TrustLevel, owner.LicenseUrl, owner.LicenseType, *lookup) {
						reason = "conflicting exact-feed legacy source policies"
					}
					if lookup == nil {
						lookup = &candidate
					}
				}
			}
			if lookup == nil && reason == "" {
				// Without config ownership, the exact attribution URL still supplies
				// identity when every contributor agrees on policy. Shared operational
				// settings cannot safely be assigned to one feed and require a decision.
				contributors := map[pgtype.UUID]bool{}
				for _, attribution := range byFeed[feed] {
					contributors[attribution.SourceID] = true
				}
				for _, row := range registry {
					owner := row.Source
					if !contributors[owner.ID] {
						continue
					}
					operational := strings.TrimSpace(string(owner.Config))
					if (operational != "" && operational != "null" && operational != "{}") || owner.RequiresAuthentication.Bool || len(owner.ApiKeyEncrypted) > 0 || owner.RateLimitRequests.Valid || owner.RateLimitWindowSeconds.Valid || owner.ApiEndpoint.Valid {
						reason = "unconfigured contributor has operational config/auth/rate-limit settings with no safe per-feed owner"
					}
					candidate := events.SourceLookupParams{Name: owner.Name, SourceType: owner.SourceType, BaseURL: feed, FeedURL: feed, TrustLevel: int(owner.TrustLevel), LicenseURL: owner.LicenseUrl, LicenseType: owner.LicenseType}
					if lookup != nil && !sourcePolicyMatches(owner.TrustLevel, owner.LicenseUrl, owner.LicenseType, *lookup) {
						reason = "conflicting unconfigured attribution source policies"
					}
					if lookup == nil {
						lookup = &candidate
					}
					if row.MetadataProtected {
						report.Issues = append(report.Issues, "original source metadata retained on "+owner.ID.String()+" while resolving feed "+feed)
					}
				}
				if lookup == nil && reason == "" {
					reason = "no attributed source row supplies an ownership policy"
				}
			}
		}
		if reason == "" && lookup != nil {
			// A savepoint lets a conflicted legacy registry row remain explicitly
			// unresolved without poisoning the surrounding repair transaction.
			savepoint, err := repo.tx.Begin(ctx)
			if err != nil {
				return fmt.Errorf("begin detail feed registration: %w", err)
			}
			lookup.RequirePolicyMatch = !useConfigTrust
			resolver := &EventRepository{pool: repo.pool, tx: savepoint, logger: repo.logger}
			id, resolveErr := resolver.GetOrCreateSource(ctx, *lookup)
			if resolveErr != nil {
				if err = savepoint.Rollback(ctx); err != nil {
					return fmt.Errorf("rollback detail feed registration: %w", err)
				}
				reason = "registry rejected explicit ownership: " + resolveErr.Error()
			} else {
				var target pgtype.UUID
				if err = target.Scan(id); err != nil {
					_ = savepoint.Rollback(ctx)
					return fmt.Errorf("parse detail source ID: %w", err)
				}
				policy, policyErr := New(savepoint).GetCanonicalSourcePolicy(ctx, feed)
				if policyErr != nil {
					_ = savepoint.Rollback(ctx)
					return fmt.Errorf("read detail source policy: %w", policyErr)
				}
				matches, change, trustErr := useConfiguredRepairTrust(ctx, New(savepoint), Source{ID: target, Name: lookup.Name, FeedUrl: pgtype.Text{String: feed, Valid: true}, TrustLevel: policy.TrustLevel, LicenseUrl: policy.LicenseUrl, LicenseType: policy.LicenseType}, *lookup, useConfigTrust && len(byName[lookup.Name]) > 0)
				if trustErr != nil {
					_ = savepoint.Rollback(ctx)
					return trustErr
				}
				if !matches {
					if err = savepoint.Rollback(ctx); err != nil {
						return fmt.Errorf("rollback detail policy conflict: %w", err)
					}
					reason = "canonical source license or policy differs from the explicit owner"
					report.Issues = append(report.Issues, fmt.Sprintf("unmapped feed %q (%d attribution(s)): %s", feed, len(byFeed[feed]), reason))
					continue
				}
				if err = savepoint.Commit(ctx); err != nil {
					return fmt.Errorf("commit detail feed registration: %w", err)
				}
				if change != nil {
					report.TrustChanges = append(report.TrustChanges, *change)
					report.TrustChangesCount++
				}
				targets[feed] = target
			}
		}
		if reason != "" {
			report.Issues = append(report.Issues, fmt.Sprintf("unmapped feed %q (%d attribution(s)): %s", feed, len(byFeed[feed]), strings.TrimSpace(reason)))
		}
	}
	return nil
}

// Opt-in trust reconciliation touches a canonical row only, with an unchanged
// license. Shared legacy catchalls never have their trust changed in place.
func useConfiguredRepairTrust(ctx context.Context, q *Queries, source Source, desired events.SourceLookupParams, enabled bool) (bool, *SourceTrustChange, error) {
	if sourcePolicyMatches(source.TrustLevel, source.LicenseUrl, source.LicenseType, desired) {
		return true, nil, nil
	}
	if !enabled || !source.FeedUrl.Valid || source.FeedUrl.String != events.NormalizeFeedURL(desired.FeedURL) || !sourcePolicyMatches(int32(desired.TrustLevel), source.LicenseUrl, source.LicenseType, desired) {
		return false, nil, nil
	}
	n, err := q.UseConfiguredSourceTrust(ctx, UseConfiguredSourceTrustParams{NewTrust: int32(desired.TrustLevel), SourceID: source.ID, FeedUrl: source.FeedUrl.String, OldTrust: source.TrustLevel, LicenseUrl: source.LicenseUrl, LicenseType: source.LicenseType})
	if err != nil {
		return false, nil, fmt.Errorf("use configured source trust: %w", err)
	}
	if n != 1 {
		return false, nil, fmt.Errorf("use configured source trust for %s: canonical source policy changed", source.ID.String())
	}
	return true, &SourceTrustChange{SourceID: source.ID.String(), Name: source.Name, Feed: source.FeedUrl.String, OldTrust: source.TrustLevel, NewTrust: int32(desired.TrustLevel)}, nil
}

// PruneEmptySources protects configured feeds and every row with nondefault
// policy or any remaining reference. Config files must be supplied by the CLI.
func PruneEmptySources(ctx context.Context, pool *pgxpool.Pool, options SourceRepairOptions) (*SourceRepairReport, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin source prune: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := New(tx)
	if err = q.LockSourceRegistryExclusive(ctx); err != nil {
		return nil, fmt.Errorf("lock source registry: %w", err)
	}
	if err = q.LockSourceRepairTables(ctx); err != nil {
		return nil, fmt.Errorf("lock source prune: %w", err)
	}
	configs, err := sourceRepairConfigs(ctx, q, options.Configs)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListSourceRepairRegistry(ctx)
	if err != nil {
		return nil, fmt.Errorf("list source prune registry: %w", err)
	}
	report := &SourceRepairReport{Applied: options.Apply, Issues: []string{}, Query: deleteSourceRepairOrphan}
	report.Before, err = sourceRepairSnapshot(ctx, q, configs)
	if err != nil {
		return nil, err
	}
	report.ConfiguredIdle = report.Before.ConfiguredIdle
	report.OrphanedIdle = report.Before.OrphanedIdle
	report.MetadataProtected = report.Before.MetadataProtected
	for _, row := range rows {
		if row.EventReferences != 0 || row.OtherReferences || row.MetadataProtected || sourceRepairConfigured(row.Source, configs) {
			continue
		}
		n, deleteErr := q.DeleteSourceRepairOrphan(ctx, row.Source.ID)
		if deleteErr != nil {
			return nil, fmt.Errorf("delete orphan source: %w", deleteErr)
		}
		report.Deleted += n
	}
	report.After, err = sourceRepairSnapshot(ctx, q, configs)
	if err != nil {
		return nil, err
	}
	if options.Apply {
		if err = tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit source prune: %w", err)
		}
	}
	return report, nil
}
