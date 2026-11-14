package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Togather-Foundation/server/tests/testhelpers/clock"
	"github.com/stretchr/testify/require"
)

func TestSourcePreflightPoliciesReadOnly(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	configs := []SourceRepairConfig{}
	ids := map[string]string{}
	for i := 0; i < 7; i++ {
		name := fmt.Sprintf("trust-%d", i)
		feed := "https://preflight.example.org/" + name
		var id string
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,feed_url,license_url,license_type,trust_level) VALUES ($1,'scraper',$2,$2,'CC0-1.0','CC0',5) RETURNING id`, name, feed).Scan(&id))
		ids[name] = id
		configs = append(configs, SourceRepairConfig{Name: name, URLs: []string{feed + "?view=all"}, TrustLevel: 6 + i%3, License: "https://creativecommons.org/publicdomain/zero/1.0/"})
	}
	// A stale persisted config does not conflict with its replacement YAML.
	_, err := pool.Exec(ctx, `INSERT INTO scraper_sources(name,url,trust_level,license) VALUES ('trust-0','https://preflight.example.org/trust-0',5,'CC0')`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO sources(name,source_type,base_url,feed_url,license_url,license_type,trust_level) VALUES ('version','scraper','https://preflight.example.org/version','https://preflight.example.org/version','https://creativecommons.org/licenses/by/3.0/','CC-BY',5)`)
	require.NoError(t, err)
	configs = append(configs,
		SourceRepairConfig{Name: "version", URLs: []string{"https://preflight.example.org/version"}, TrustLevel: 5, License: "CC-BY-4.0"},
		SourceRepairConfig{Name: "view-a", URLs: []string{"https://preflight.example.org/views?one"}, TrustLevel: 5, License: "CC0"},
		SourceRepairConfig{Name: "view-b", URLs: []string{"https://preflight.example.org/views?two"}, TrustLevel: 8, License: "CC0-1.0"},
		SourceRepairConfig{Name: "suppression-a", URLs: []string{"https://preflight.example.org/new?one"}, TrustLevel: 5, License: "CC0", SuppressReviewWarnings: []string{"missing_image"}},
		SourceRepairConfig{Name: "suppression-b", URLs: []string{"https://preflight.example.org/new?two"}, TrustLevel: 5, License: "CC0-1.0", SuppressReviewWarnings: []string{"missing_description"}},
		SourceRepairConfig{Name: "ownership", URLs: []string{"https://preflight.example.org/ownership"}, TrustLevel: 7, License: "CC0"},
		SourceRepairConfig{Name: "private", URLs: []string{"https://user:supersecret@preflight.example.org/private?token=secretquery"}, TrustLevel: 5, License: "CC0"},
		SourceRepairConfig{Name: "label-collision", URLs: []string{"https://preflight.example.org/label"}, TrustLevel: 5, License: "CC0"},
	)
	_, err = pool.Exec(ctx, `INSERT INTO sources(name,source_type,base_url,feed_url,license_url,license_type,trust_level) VALUES ('canonical-label','scraper','https://preflight.example.org/label','https://preflight.example.org/label','CC0','CC0',5),('label-collision','scraper','https://preflight.example.org/label',NULL,'CC0','CC0',5)`)
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		_, err := pool.Exec(ctx, `INSERT INTO sources(name,source_type,base_url,license_url,license_type,trust_level) VALUES ($1,'scraper','https://preflight.example.org/ownership','CC0','CC0',$2)`, fmt.Sprintf("legacy-owner-%d", i), 5+i)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO scraper_sources(name,url,trust_level,license) VALUES ($1,'https://preflight.example.org/ownership',$2,'CC0')`, fmt.Sprintf("legacy-owner-%d", i), 5+i)
		require.NoError(t, err)
	}
	snapshot := func() string {
		var value string
		require.NoError(t, pool.QueryRow(ctx, `SELECT jsonb_build_object('sources',(SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM sources s),'configs',(SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM scraper_sources s),'keys',(SELECT jsonb_agg(to_jsonb(k) ORDER BY id) FROM api_keys k),'attrs',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM event_sources e))::text`).Scan(&value))
		return value
	}
	before := snapshot()
	report, err := PreflightSourcePolicies(ctx, pool, configs)
	require.NoError(t, err)
	require.Equal(t, before, snapshot(), "inventory must not change any persisted data")
	require.Equal(t, 12, report.Conflicts)
	for _, entry := range report.Entries {
		switch {
		case strings.HasPrefix(entry.ConfigName, "trust-"):
			require.Equal(t, []string{"trust_mismatch"}, entry.Conflicts)
			require.Equal(t, ids[entry.ConfigName], entry.SourceID)
			require.Equal(t, 5, entry.Stored.TrustLevel)
			require.Contains(t, []int{6, 7, 8}, entry.Configured.TrustLevel)
			require.NotContains(t, entry.Feed, "?")
		case entry.ConfigName == "version":
			require.Equal(t, []string{"license_mismatch"}, entry.Conflicts)
		case strings.HasPrefix(entry.ConfigName, "view-"):
			require.Equal(t, []string{"configured_policy_conflict"}, entry.Conflicts)
		case entry.ConfigName == "ownership":
			require.Equal(t, []string{"canonical_ownership_conflict"}, entry.Conflicts)
		case entry.ConfigName == "label-collision":
			require.Equal(t, []string{"canonical_ownership_conflict"}, entry.Conflicts)
		default:
			require.Empty(t, entry.Conflicts)
		}
	}
	encoded, err := json.Marshal(report)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "supersecret")
	require.NotContains(t, string(encoded), "secretquery")
}

func TestSourceReconcileDryRunAndIdempotency(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	var catchAll, configured string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,license_url,license_type,trust_level) VALUES ('Calendar','api','https://calendar.example.org','https://creativecommons.org/publicdomain/zero/1.0/','CC0',5) RETURNING id`).Scan(&catchAll))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,license_url,license_type,trust_level) VALUES ('configured-feed','scraper','https://calendar.example.org/one.ics','https://creativecommons.org/licenses/by/4.0/','CC-BY',9) RETURNING id`).Scan(&configured))
	org := insertOrganization(t, ctx, pool, "Repair Org")
	place := insertPlace(t, ctx, pool, "Repair Hall", "Toronto", "ON")
	ulid := insertEvent(t, ctx, pool, "Repair Event", "A feed event", org, place, "community", "published", nil, clock.FutureTime(3))
	insertFeedAttribution(t, ctx, pool, ulid, catchAll, "https://calendar.example.org/one.ics/", "event-1")
	options := SourceRepairOptions{Configs: []SourceRepairConfig{{Name: "configured-feed", URLs: []string{"https://calendar.example.org/one.ics"}, TrustLevel: 9, License: "https://creativecommons.org/licenses/by/4.0/"}}}
	preview, err := ReconcileSources(ctx, pool, options)
	require.NoError(t, err)
	require.EqualValues(t, 1, preview.AttributionsMoved)
	var owner string
	require.NoError(t, pool.QueryRow(ctx, `SELECT source_id FROM event_sources`).Scan(&owner))
	require.Equal(t, catchAll, owner, "dry run rolls back every change")
	options.Apply = true
	applied, err := ReconcileSources(ctx, pool, options)
	require.NoError(t, err)
	require.EqualValues(t, 1, applied.AttributionsMoved)
	require.NoError(t, pool.QueryRow(ctx, `SELECT source_id FROM event_sources`).Scan(&owner))
	require.Equal(t, configured, owner)
	var trust int
	var license string
	require.NoError(t, pool.QueryRow(ctx, `SELECT trust_level,license_type FROM sources WHERE id=$1`, configured).Scan(&trust, &license))
	require.Equal(t, 9, trust)
	require.Equal(t, "CC-BY", license)
	again, err := ReconcileSources(ctx, pool, options)
	require.NoError(t, err)
	require.Zero(t, again.AttributionsMoved)
}

func TestSourcePruneProtectsConfiguredAndMetadata(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	for _, name := range []string{"configured-idle", "trusted", "orphan"} {
		_, err := pool.Exec(ctx, `INSERT INTO sources(name,source_type,base_url,license_url,license_type,trust_level) VALUES ($1,'api','https://example.org/' || $1,'https://creativecommons.org/publicdomain/zero/1.0/','CC0',CASE WHEN $1='trusted' THEN 9 ELSE 5 END)`, name)
		require.NoError(t, err)
	}
	options := SourceRepairOptions{Apply: true, Configs: []SourceRepairConfig{{Name: "configured-idle", URLs: []string{"https://example.org/configured-idle"}, TrustLevel: 5, License: "https://creativecommons.org/publicdomain/zero/1.0/"}}}
	report, err := PruneEmptySources(ctx, pool, options)
	require.NoError(t, err)
	require.EqualValues(t, 1, report.ConfiguredIdle)
	require.EqualValues(t, 2, report.OrphanedIdle)
	require.EqualValues(t, 1, report.MetadataProtected)
	require.EqualValues(t, 1, report.Deleted)
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM sources WHERE name IN ('configured-idle','trusted')`).Scan(&n))
	require.Equal(t, 2, n)
}

func TestSourceReconcileRetainsConflictingPoliciesAndPayloads(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	var legacy, target string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,license_url,license_type,trust_level) VALUES ('legacy','api','https://calendar.example.org','https://creativecommons.org/publicdomain/zero/1.0/','CC0',5) RETURNING id`).Scan(&legacy))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,feed_url,license_url,license_type,trust_level) VALUES ('target','scraper','https://calendar.example.org/one.ics','https://calendar.example.org/one.ics','https://creativecommons.org/publicdomain/zero/1.0/','CC0',5) RETURNING id`).Scan(&target))
	org := insertOrganization(t, ctx, pool, "Repair Conflict Org")
	place := insertPlace(t, ctx, pool, "Repair Conflict Hall", "Toronto", "ON")
	ulid := insertEvent(t, ctx, pool, "Repair Conflict Event", "Payload preservation", org, place, "community", "published", nil, clock.FutureTime(3))
	feed := "https://calendar.example.org/one.ics"
	insertFeedAttribution(t, ctx, pool, ulid, legacy, feed, "event-1")
	insertFeedAttribution(t, ctx, pool, ulid, target, feed, "event-1")
	_, err := pool.Exec(ctx, `UPDATE event_sources SET payload=jsonb_build_object('originalSource',source_id::text)`)
	require.NoError(t, err)
	options := SourceRepairOptions{Apply: true, Configs: []SourceRepairConfig{
		{Name: "view-one", URLs: []string{feed}, TrustLevel: 5, License: "https://creativecommons.org/publicdomain/zero/1.0/", SuppressReviewWarnings: []string{"missing_description"}},
		{Name: "view-two", URLs: []string{feed}, TrustLevel: 9, License: "https://creativecommons.org/publicdomain/zero/1.0/"},
	}}
	conflict, err := ReconcileSources(ctx, pool, options)
	require.NoError(t, err)
	require.Zero(t, conflict.AttributionsMoved)
	require.NotEmpty(t, conflict.Issues, "different configured policies must not be silently merged")
	options.Configs = options.Configs[:1]
	_, err = pool.Exec(ctx, `UPDATE sources SET trust_level=8,license_type='CC-BY',license_url='https://creativecommons.org/licenses/by/4.0/',config='{"policy":"retain"}',contact_email='curator@example.org',api_key_encrypted=decode('deadbeef','hex') WHERE id=$1`, legacy)
	require.NoError(t, err)
	var originalID, canonicalID string
	var originalSnapshot, sourceSnapshot, canonicalPayload []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT es.id,to_jsonb(es),to_jsonb(s) FROM event_sources es JOIN sources s ON s.id=es.source_id WHERE source_id=$1`, legacy).Scan(&originalID, &originalSnapshot, &sourceSnapshot))
	require.NoError(t, pool.QueryRow(ctx, `SELECT id,payload FROM event_sources WHERE source_id=$1`, target).Scan(&canonicalID, &canonicalPayload))
	options.Apply = false
	preview, err := ReconcileSources(ctx, pool, options)
	require.NoError(t, err)
	require.EqualValues(t, 1, preview.AttributionsArchived)
	require.EqualValues(t, 0, preview.Before.ArchivedAttributions)
	require.EqualValues(t, 1, preview.After.ArchivedAttributions)
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM source_repair_attribution_archive`).Scan(&n))
	require.Zero(t, n, "preview archive and retirement roll back together")
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM event_sources`).Scan(&n))
	require.Equal(t, 2, n)
	options.Apply = true
	duplicate, err := ReconcileSources(ctx, pool, options)
	require.NoError(t, err)
	require.Zero(t, duplicate.AttributionsMoved)
	require.EqualValues(t, 1, duplicate.AttributionsArchived)
	require.Equal(t, duplicate.Before.AttributionRows+int(duplicate.Before.ArchivedAttributions), duplicate.After.AttributionRows+int(duplicate.After.ArchivedAttributions), "every observation remains live or archived")
	var archivedOriginalID, archivedCanonicalID string
	var archivedOriginal, archivedSource, remainingPayload []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT original_attribution_id,canonical_attribution_id,original_attribution,original_source FROM source_repair_attribution_archive`).Scan(&archivedOriginalID, &archivedCanonicalID, &archivedOriginal, &archivedSource))
	require.Equal(t, originalID, archivedOriginalID)
	require.Equal(t, canonicalID, archivedCanonicalID)
	require.JSONEq(t, string(originalSnapshot), string(archivedOriginal), "complete attribution row survives byte-for-byte as JSONB")
	require.JSONEq(t, string(sourceSnapshot), string(archivedSource), "complete original source policy survives")
	require.NoError(t, pool.QueryRow(ctx, `SELECT payload FROM event_sources`).Scan(&remainingPayload))
	require.JSONEq(t, string(canonicalPayload), string(remainingPayload), "canonical live payload remains untouched")
	again, err := ReconcileSources(ctx, pool, options)
	require.NoError(t, err)
	require.Zero(t, again.AttributionsArchived)
	require.Zero(t, again.AttributionsMoved)
	require.EqualValues(t, 1, again.After.ArchivedAttributions)
	_, err = pool.Exec(ctx, `UPDATE source_repair_attribution_archive SET original_source='{}'`)
	require.ErrorContains(t, err, "immutable")
	_, err = pool.Exec(ctx, `DELETE FROM source_repair_attribution_archive`)
	require.ErrorContains(t, err, "immutable")

}

func TestSourceReconcileMovesOnlyUnambiguousFieldAndChangeProvenance(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	var legacy string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,license_url,license_type) VALUES ('legacy','api','https://calendar.example.org','https://creativecommons.org/publicdomain/zero/1.0/','CC0') RETURNING id`).Scan(&legacy))
	org := insertOrganization(t, ctx, pool, "Repair Provenance Org")
	place := insertPlace(t, ctx, pool, "Repair Provenance Hall", "Toronto", "ON")
	ulid := insertEvent(t, ctx, pool, "Repair Provenance Event", "Multiple feeds", org, place, "community", "published", nil, clock.FutureTime(3))
	for _, feed := range []string{"https://calendar.example.org/one.ics", "https://calendar.example.org/two.ics"} {
		insertFeedAttribution(t, ctx, pool, ulid, legacy, feed, "event-1")
	}
	_, err := pool.Exec(ctx, `INSERT INTO field_provenance(event_id,field_path,value_hash,source_id,confidence) VALUES ((SELECT id FROM events WHERE ulid=$1),'name','hash',$2,1)`, ulid, legacy)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO event_changes(event_id,action,source_id) VALUES ((SELECT id FROM events WHERE ulid=$1),'update',$2)`, ulid, legacy)
	require.NoError(t, err)
	report, err := ReconcileSources(ctx, pool, SourceRepairOptions{Apply: true, Configs: []SourceRepairConfig{
		{Name: "one", URLs: []string{"https://calendar.example.org/one.ics"}, TrustLevel: 5, License: "https://creativecommons.org/publicdomain/zero/1.0/"},
		{Name: "two", URLs: []string{"https://calendar.example.org/two.ics"}, TrustLevel: 5, License: "https://creativecommons.org/publicdomain/zero/1.0/"},
	}})
	require.NoError(t, err)
	require.EqualValues(t, 2, report.AttributionsMoved)
	require.Zero(t, report.FieldsMoved)
	require.Zero(t, report.ChangesMoved)
	require.NotEmpty(t, report.Issues)
	var owner string
	require.NoError(t, pool.QueryRow(ctx, `SELECT source_id FROM field_provenance`).Scan(&owner))
	require.Equal(t, legacy, owner)
	pruned, err := PruneEmptySources(ctx, pool, SourceRepairOptions{Apply: true})
	require.NoError(t, err)
	require.Zero(t, pruned.Deleted, "historical source referenced by ambiguous audit records must survive")
}

func TestSourceReconcileReportsFieldProvenanceCollision(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	var legacy, target string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,license_url,license_type) VALUES ('legacy','api','https://calendar.example.org','https://creativecommons.org/publicdomain/zero/1.0/','CC0') RETURNING id`).Scan(&legacy))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,feed_url,license_url,license_type) VALUES ('target','scraper','https://calendar.example.org/one.ics','https://calendar.example.org/one.ics','https://creativecommons.org/publicdomain/zero/1.0/','CC0') RETURNING id`).Scan(&target))
	org := insertOrganization(t, ctx, pool, "Repair Collision Org")
	place := insertPlace(t, ctx, pool, "Repair Collision Hall", "Toronto", "ON")
	ulid := insertEvent(t, ctx, pool, "Repair Collision Event", "Same field timestamp", org, place, "community", "published", nil, clock.FutureTime(3))
	insertFeedAttribution(t, ctx, pool, ulid, legacy, "https://calendar.example.org/one.ics", "event-1")
	_, err := pool.Exec(ctx, `INSERT INTO field_provenance(event_id,field_path,value_hash,source_id,confidence,observed_at) VALUES
((SELECT id FROM events WHERE ulid=$1),'name','original',$2,1,now()),
((SELECT id FROM events WHERE ulid=$1),'name','target',$3,1,now())`, ulid, legacy, target)
	require.NoError(t, err)
	report, err := ReconcileSources(ctx, pool, SourceRepairOptions{Apply: true})
	require.NoError(t, err)
	require.EqualValues(t, 1, report.AttributionsMoved)
	require.Zero(t, report.FieldsMoved)
	require.Contains(t, report.Issues, "1 field provenance collision(s) retained: "+func() string {
		var id string
		require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM events WHERE ulid=$1`, ulid).Scan(&id))
		return id
	}()+"/"+legacy)
	var fields int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM field_provenance`).Scan(&fields))
	require.Equal(t, 2, fields, "both values and original source labels survive")
}

func TestSourceArchiveSurvivesOrphanPruning(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	var legacy, target string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,license_url,license_type) VALUES ('legacy','api','https://calendar.example.org','https://creativecommons.org/publicdomain/zero/1.0/','CC0') RETURNING id`).Scan(&legacy))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,feed_url,license_url,license_type) VALUES ('target','scraper','https://calendar.example.org/one.ics','https://calendar.example.org/one.ics','https://creativecommons.org/publicdomain/zero/1.0/','CC0') RETURNING id`).Scan(&target))
	org := insertOrganization(t, ctx, pool, "Archive Prune Org")
	place := insertPlace(t, ctx, pool, "Archive Prune Hall", "Toronto", "ON")
	ulid := insertEvent(t, ctx, pool, "Archive Prune Event", "Historical source cleanup", org, place, "community", "published", nil, clock.FutureTime(3))
	insertFeedAttribution(t, ctx, pool, ulid, legacy, "https://calendar.example.org/one.ics", "event-1")
	insertFeedAttribution(t, ctx, pool, ulid, target, "https://calendar.example.org/one.ics", "event-1")
	reconciled, err := ReconcileSources(ctx, pool, SourceRepairOptions{Apply: true, Configs: []SourceRepairConfig{
		{Name: "target", URLs: []string{"https://calendar.example.org/one.ics"}, TrustLevel: 5, License: "CC0"},
		{Name: "same-policy-view", URLs: []string{"https://calendar.example.org/one.ics"}, TrustLevel: 5, License: "https://creativecommons.org/publicdomain/zero/1.0/"},
	}})
	require.NoError(t, err)
	require.EqualValues(t, 1, reconciled.AttributionsArchived)
	require.Empty(t, reconciled.Issues, "CC0 alias and canonical URL describe the same configured policy")
	pruned, err := PruneEmptySources(ctx, pool, SourceRepairOptions{Apply: true})
	require.NoError(t, err)
	require.EqualValues(t, 1, pruned.Deleted)
	var archivedSource string
	require.NoError(t, pool.QueryRow(ctx, `SELECT original_source->>'id' FROM source_repair_attribution_archive`).Scan(&archivedSource))
	require.Equal(t, legacy, archivedSource, "source snapshot survives retirement of its unreferenced registry row")
	require.EqualValues(t, 1, pruned.After.ArchivedAttributions)
}

func TestSourceReconcileRegistersAttributedDetailURLs(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	var configured, catchAll string
	for _, item := range []struct {
		name string
		id   *string
	}{{"configured-sitemap", &configured}, {"Catch All", &catchAll}} {
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,license_url,license_type,trust_level) VALUES ($1,'scraper','https://calendar.example.org','https://creativecommons.org/publicdomain/zero/1.0/','CC0',7) RETURNING id`, item.name).Scan(item.id))
	}
	org := insertOrganization(t, ctx, pool, "Detail Repair Org")
	place := insertPlace(t, ctx, pool, "Detail Repair Hall", "Toronto", "ON")
	details := []string{"https://calendar.example.org/event/one", "https://calendar.example.org/event/two"}
	for i, feed := range details {
		ulid := insertEvent(t, ctx, pool, feed, "Detail identity", org, place, "community", "published", nil, clock.FutureTime(3+i))
		insertFeedAttribution(t, ctx, pool, ulid, configured, feed, "event-1")
		if i == 0 {
			insertFeedAttribution(t, ctx, pool, ulid, catchAll, feed, "event-1")
		}
	}
	options := SourceRepairOptions{Configs: []SourceRepairConfig{{Name: "configured-sitemap", URLs: []string{"https://calendar.example.org/events"}, TrustLevel: 7, License: "CC0-1.0"}}}
	preview, err := ReconcileSources(ctx, pool, options)
	require.NoError(t, err)
	require.Zero(t, preview.After.UnregisteredAttributions)
	require.EqualValues(t, 1, preview.AttributionsArchived)
	var canonical int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM sources WHERE feed_url IS NOT NULL`).Scan(&canonical))
	require.Zero(t, canonical, "preview rolls back detail and listing registration")
	options.Apply = true
	report, err := ReconcileSources(ctx, pool, options)
	require.NoError(t, err)
	require.Zero(t, report.After.UnregisteredAttributions)
	require.Zero(t, report.After.NoncanonicalAttributions)
	require.EqualValues(t, 1, report.AttributionsArchived)
	for _, feed := range details {
		var base string
		require.NoError(t, pool.QueryRow(ctx, `SELECT s.base_url FROM event_sources es JOIN sources s ON s.id=es.source_id WHERE es.source_url=$1`, feed).Scan(&base))
		require.Equal(t, feed, base, "detail URLs remain distinct feed identities")
	}
	again, err := ReconcileSources(ctx, pool, options)
	require.NoError(t, err)
	require.Zero(t, again.AttributionsMoved)
	require.Zero(t, again.AttributionsArchived)
	require.EqualValues(t, 1, again.After.ArchivedAttributions)
}

func TestSourceReconcileUnconfiguredAgreementAndUnmappedReasons(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	org := insertOrganization(t, ctx, pool, "Fallback Repair Org")
	place := insertPlace(t, ctx, pool, "Fallback Repair Hall", "Toronto", "ON")
	for _, scenario := range []struct {
		path        string
		trusts      []int
		operational bool
	}{
		{"agreement", []int{8, 8}, false}, {"conflict", []int{5, 9}, false}, {"operational", []int{5}, true}, {"invalid", []int{5}, false},
	} {
		feed := "https://calendar.example.org/" + scenario.path
		if scenario.path == "invalid" {
			feed = "invalid-url"
		}
		ulid := insertEvent(t, ctx, pool, scenario.path, "Explicit policy fallback", org, place, "community", "published", nil, clock.FutureTime(3))
		for i, trust := range scenario.trusts {
			var id string
			require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,license_url,license_type,trust_level) VALUES ($1,'api','https://calendar.example.org','https://creativecommons.org/licenses/by/4.0/','CC-BY',$2) RETURNING id`, scenario.path+fmt.Sprint(i), trust).Scan(&id))
			if scenario.operational {
				_, err := pool.Exec(ctx, `UPDATE sources SET config='{"requiresOperator":true}' WHERE id=$1`, id)
				require.NoError(t, err)
			}
			insertFeedAttribution(t, ctx, pool, ulid, id, feed, "event-1")
		}
	}
	report, err := ReconcileSources(ctx, pool, SourceRepairOptions{Apply: true})
	require.NoError(t, err)
	require.EqualValues(t, 1, report.AttributionsArchived, "agreed nondefault trust/license sources consolidate without guessing")
	var trust int
	var license string
	require.NoError(t, pool.QueryRow(ctx, `SELECT trust_level,license_type FROM sources WHERE feed_url='https://calendar.example.org/agreement'`).Scan(&trust, &license))
	require.Equal(t, 8, trust)
	require.Equal(t, "CC-BY", license)
	issues := strings.Join(report.Issues, "\n")
	require.Contains(t, issues, `unmapped feed "https://calendar.example.org/conflict" (2 attribution(s)): conflicting unconfigured attribution source policies`)
	require.Contains(t, issues, `unmapped feed "https://calendar.example.org/operational" (1 attribution(s)): unconfigured contributor has operational config/auth/rate-limit settings`)
	require.Contains(t, issues, `unmapped feed "invalid-url" (1 attribution(s)): invalid absolute HTTP(S) URL`)
}

func TestSourceReconcileKeepsPerConfigWarningSuppressions(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	for _, item := range []struct{ name, warning string }{{"view-one", "missing_description"}, {"view-two", "missing_image"}} {
		_, err := pool.Exec(ctx, `INSERT INTO scraper_sources(name,url,trust_level,license,suppress_review_warnings) VALUES ($1,'https://calendar.example.org/events',5,'CC0',ARRAY[$2]::text[])`, item.name, item.warning)
		require.NoError(t, err)
	}
	report, err := ReconcileSources(ctx, pool, SourceRepairOptions{Apply: true})
	require.NoError(t, err)
	require.Empty(t, report.Issues, "warning suppression is a per-config input policy, not canonical source identity")
	var canonical int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM sources WHERE feed_url='https://calendar.example.org/events'`).Scan(&canonical))
	require.Equal(t, 1, canonical)
	for _, name := range []string{"view-one", "view-two"} {
		var warnings []string
		require.NoError(t, pool.QueryRow(ctx, `SELECT suppress_review_warnings FROM scraper_sources WHERE name=$1`, name).Scan(&warnings))
		expected := "missing_description"
		if name == "view-two" {
			expected = "missing_image"
		}
		require.Equal(t, []string{expected}, warnings)
	}
}

func TestSourceReconcileRejectsConflictingNamedDetailOwners(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	org := insertOrganization(t, ctx, pool, "Conflicting Detail Org")
	place := insertPlace(t, ctx, pool, "Conflicting Detail Hall", "Toronto", "ON")
	ulid := insertEvent(t, ctx, pool, "Conflicting Detail", "Ownership conflict", org, place, "community", "published", nil, clock.FutureTime(3))
	configs := []SourceRepairConfig{}
	for i, trust := range []int{5, 9} {
		name := "named-owner-" + fmt.Sprint(i)
		var source string
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,license_url,license_type,trust_level) VALUES ($1,'scraper','https://calendar.example.org','https://creativecommons.org/publicdomain/zero/1.0/','CC0',$2) RETURNING id`, name, trust).Scan(&source))
		insertFeedAttribution(t, ctx, pool, ulid, source, "https://calendar.example.org/detail/one", "event-1")
		configs = append(configs, SourceRepairConfig{Name: name, URLs: []string{"https://calendar.example.org/list/" + fmt.Sprint(i)}, TrustLevel: trust, License: "CC0"})
	}
	report, err := ReconcileSources(ctx, pool, SourceRepairOptions{Apply: true, Configs: configs})
	require.NoError(t, err)
	require.Zero(t, report.AttributionsMoved)
	require.Zero(t, report.AttributionsArchived)
	require.Contains(t, strings.Join(report.Issues, "\n"), `unmapped feed "https://calendar.example.org/detail/one" (2 attribution(s)): conflicting named config owner policies`)
	require.Equal(t, 2, report.After.UnregisteredAttributions)
}

func TestSourceReconcileAdoptsSafeExactLegacyFeed(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	var owner, catchAll string
	feed := "https://calendar.example.org/full/feed.ics"
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,license_url,license_type,trust_level,notes) VALUES ('Exact Feed','api',$1,'https://creativecommons.org/licenses/by/4.0/','CC-BY',9,'retain exact owner metadata') RETURNING id`, feed).Scan(&owner))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,license_url,license_type) VALUES ('Catchall','api','https://calendar.example.org','https://creativecommons.org/publicdomain/zero/1.0/','CC0') RETURNING id`).Scan(&catchAll))
	org := insertOrganization(t, ctx, pool, "Exact Feed Org")
	place := insertPlace(t, ctx, pool, "Exact Feed Hall", "Toronto", "ON")
	ulid := insertEvent(t, ctx, pool, "Exact Feed Event", "Exact ownership evidence", org, place, "community", "published", nil, clock.FutureTime(3))
	insertFeedAttribution(t, ctx, pool, ulid, owner, feed, "event-1")
	insertFeedAttribution(t, ctx, pool, ulid, catchAll, feed, "event-1")
	report, err := ReconcileSources(ctx, pool, SourceRepairOptions{Apply: true})
	require.NoError(t, err)
	require.EqualValues(t, 1, report.AttributionsArchived)
	var canonical, notes string
	require.NoError(t, pool.QueryRow(ctx, `SELECT id,notes FROM sources WHERE feed_url=$1`, feed).Scan(&canonical, &notes))
	require.Equal(t, owner, canonical)
	require.Equal(t, "retain exact owner metadata", notes)
	require.Zero(t, report.After.UnregisteredAttributions)
}

func TestSourceReconcileConfigTrustRequiresExplicitOptIn(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	feed := "https://calendar.example.org/configured.ics"
	var id string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,feed_url,license_url,license_type,trust_level,config,contact_email,api_key_encrypted,notes) VALUES ('configured','scraper',$1,$1,'https://creativecommons.org/publicdomain/zero/1.0/','CC0',5,'{"operator":"preserve"}','owner@example.org',decode('deadbeef','hex'),'retain all metadata') RETURNING id`, feed).Scan(&id))
	var before []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT to_jsonb(s)-'trust_level' FROM sources s WHERE id=$1`, id).Scan(&before))
	options := SourceRepairOptions{Apply: true, Configs: []SourceRepairConfig{{Name: "configured", URLs: []string{feed}, TrustLevel: 8, License: "CC0-1.0"}}}
	defaultReport, err := ReconcileSources(ctx, pool, options)
	require.NoError(t, err)
	require.Zero(t, defaultReport.TrustChangesCount)
	require.NotEmpty(t, defaultReport.Issues)
	var trust int
	require.NoError(t, pool.QueryRow(ctx, `SELECT trust_level FROM sources WHERE id=$1`, id).Scan(&trust))
	require.Equal(t, 5, trust, "ordinary repair preserves stored operator policy")
	options.UseConfigTrust = true
	options.Apply = false
	preview, err := ReconcileSources(ctx, pool, options)
	require.NoError(t, err)
	require.Equal(t, 1, preview.TrustChangesCount)
	require.Equal(t, []SourceTrustChange{{SourceID: id, Name: "configured", Feed: feed, OldTrust: 5, NewTrust: 8}}, preview.TrustChanges)
	require.Contains(t, preview.Query, "UPDATE sources SET trust_level")
	require.NoError(t, pool.QueryRow(ctx, `SELECT trust_level FROM sources WHERE id=$1`, id).Scan(&trust))
	require.Equal(t, 5, trust, "preview rolls trust changes back")
	options.Apply = true
	applied, err := ReconcileSources(ctx, pool, options)
	require.NoError(t, err)
	require.Equal(t, 1, applied.TrustChangesCount)
	require.Equal(t, preview.TrustChanges, applied.TrustChanges)
	var after []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT trust_level,to_jsonb(s)-'trust_level' FROM sources s WHERE id=$1`, id).Scan(&trust, &after))
	require.Equal(t, 8, trust)
	require.JSONEq(t, string(before), string(after), "trust is the only changed source metadata")
	again, err := ReconcileSources(ctx, pool, options)
	require.NoError(t, err)
	require.Zero(t, again.TrustChangesCount)
	require.Empty(t, again.TrustChanges)
}

func TestSourceReconcileConfigTrustNeverOverridesLicenseOrGroupConflict(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	feed := "https://calendar.example.org/licensed.ics"
	var id string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,feed_url,license_url,license_type,trust_level) VALUES ('licensed','scraper',$1,$1,'https://creativecommons.org/licenses/by/4.0/','CC-BY',5) RETURNING id`, feed).Scan(&id))
	options := SourceRepairOptions{Apply: true, UseConfigTrust: true, Configs: []SourceRepairConfig{{Name: "licensed", URLs: []string{feed}, TrustLevel: 8, License: "CC0"}}}
	report, err := ReconcileSources(ctx, pool, options)
	require.NoError(t, err)
	require.Zero(t, report.TrustChangesCount)
	require.NotEmpty(t, report.Issues)
	var trust int
	var license string
	require.NoError(t, pool.QueryRow(ctx, `SELECT trust_level,license_type FROM sources WHERE id=$1`, id).Scan(&trust, &license))
	require.Equal(t, 5, trust)
	require.Equal(t, "CC-BY", license)
	options.Configs = []SourceRepairConfig{
		{Name: "licensed", URLs: []string{feed}, TrustLevel: 8, License: "CC-BY-4.0"},
		{Name: "other-view", URLs: []string{feed}, TrustLevel: 6, License: "CC-BY-4.0"},
	}
	report, err = ReconcileSources(ctx, pool, options)
	require.NoError(t, err)
	require.Zero(t, report.TrustChangesCount)
	require.Contains(t, strings.Join(report.Issues, "\n"), "conflicting configured policies")
	require.NoError(t, pool.QueryRow(ctx, `SELECT trust_level FROM sources WHERE id=$1`, id).Scan(&trust))
	require.Equal(t, 5, trust)
}

func TestSourceReconcileDoesNotInferFromLegacyRowOwningDifferentFeed(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	feedA, feedB := "https://calendar.example.org/a.ics", "https://calendar.example.org/b.ics"
	var misleading, actual string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,license_url,license_type,trust_level) VALUES ('misleading','api',$1,'https://creativecommons.org/publicdomain/zero/1.0/','CC0',9) RETURNING id`, feedA).Scan(&misleading))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,license_url,license_type,trust_level) VALUES ('actual','api','https://calendar.example.org','https://creativecommons.org/publicdomain/zero/1.0/','CC0',5) RETURNING id`).Scan(&actual))
	org := insertOrganization(t, ctx, pool, "Wrong Feed Org")
	place := insertPlace(t, ctx, pool, "Wrong Feed Hall", "Toronto", "ON")
	ulidA := insertEvent(t, ctx, pool, "Feed A", "Correct contribution policy", org, place, "community", "published", nil, clock.FutureTime(3))
	ulidB := insertEvent(t, ctx, pool, "Feed B", "Misleading registry URL", org, place, "community", "published", nil, clock.FutureTime(4))
	insertFeedAttribution(t, ctx, pool, ulidA, actual, feedA, "event-a")
	insertFeedAttribution(t, ctx, pool, ulidB, misleading, feedB, "event-b")
	report, err := ReconcileSources(ctx, pool, SourceRepairOptions{Apply: true})
	require.NoError(t, err)
	require.Zero(t, report.After.UnregisteredAttributions)
	var trust int
	require.NoError(t, pool.QueryRow(ctx, `SELECT trust_level FROM sources WHERE feed_url=$1`, feedA).Scan(&trust))
	require.Equal(t, 5, trust, "a row whose only attribution belongs to feed B cannot supply feed A policy")
}

func TestSourceReconcileConfigTrustDoesNotRewriteSharedCatchall(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	var shared string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,license_url,license_type,trust_level) VALUES ('shared','scraper','https://calendar.example.org','https://creativecommons.org/publicdomain/zero/1.0/','CC0',5) RETURNING id`).Scan(&shared))
	org := insertOrganization(t, ctx, pool, "Shared Trust Org")
	place := insertPlace(t, ctx, pool, "Shared Trust Hall", "Toronto", "ON")
	configs := []SourceRepairConfig{}
	for i, trust := range []int{6, 8} {
		feed := "https://calendar.example.org/feed/" + fmt.Sprint(i)
		ulid := insertEvent(t, ctx, pool, feed, "Independent feed policy", org, place, "community", "published", nil, clock.FutureTime(3+i))
		insertFeedAttribution(t, ctx, pool, ulid, shared, feed, "event-1")
		configs = append(configs, SourceRepairConfig{Name: "shared", URLs: []string{feed}, TrustLevel: trust, License: "CC0"})
	}
	report, err := ReconcileSources(ctx, pool, SourceRepairOptions{Apply: true, UseConfigTrust: true, Configs: configs})
	require.NoError(t, err)
	require.Zero(t, report.TrustChangesCount, "new per-feed rows use config policy; shared host row is not overwritten")
	var oldTrust int
	require.NoError(t, pool.QueryRow(ctx, `SELECT trust_level FROM sources WHERE id=$1`, shared).Scan(&oldTrust))
	require.Equal(t, 5, oldTrust)
	for _, cfg := range configs {
		var actual int
		require.NoError(t, pool.QueryRow(ctx, `SELECT trust_level FROM sources WHERE feed_url=$1`, cfg.URLs[0]).Scan(&actual))
		require.Equal(t, cfg.TrustLevel, actual)
	}
}
