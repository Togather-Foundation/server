package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Togather-Foundation/server/internal/domain/events"
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
	Apply   bool
	Configs []SourceRepairConfig
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
	AttributionsArchived int64              `json:"attributionsArchived"`
	Applied              bool               `json:"applied"`
	Before               SourceRepairCounts `json:"before"`
	After                SourceRepairCounts `json:"after"`
	AttributionsMoved    int64              `json:"attributionsMoved"`
	FieldsMoved          int64              `json:"fieldsMoved"`
	ChangesMoved         int64              `json:"changesMoved"`
	ConfiguredIdle       int                `json:"configuredIdle"`
	OrphanedIdle         int                `json:"orphanedIdle"`
	MetadataProtected    int                `json:"metadataProtected"`
	Deleted              int64              `json:"deleted"`
	Issues               []string           `json:"issues"`
	Query                string             `json:"query"`
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
	warnings := append([]string{}, cfg.SuppressReviewWarnings...)
	sort.Strings(warnings)
	data, _ := json.Marshal([]any{cfg.TrustLevel, sourceLicenseKey(cfg.License), warnings})
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
	report := &SourceRepairReport{Applied: options.Apply, Issues: []string{}, Query: moveSourceRepairAttribution + "\n" + archiveSourceRepairAttribution + "\n" + deleteArchivedSourceRepairAttribution + "\n" + moveSourceRepairFields + "\n" + countSourceRepairRetainedFields + "\n" + moveSourceRepairChanges}
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
				if !sourcePolicyMatches(row.Source.TrustLevel, row.Source.LicenseUrl, row.Source.LicenseType, params) {
					report.Issues = append(report.Issues, "canonical policy differs from configured feed; retained unchanged: "+row.Source.FeedUrl.String)
					continue
				}
			}
			targets[row.Source.FeedUrl.String] = row.Source.ID
		}
	}
	attrs, err := q.ListSourceRepairAttributions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list repair attributions: %w", err)
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
