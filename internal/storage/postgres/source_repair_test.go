package postgres

import (
	"context"
	"testing"

	"github.com/Togather-Foundation/server/tests/testhelpers/clock"
	"github.com/stretchr/testify/require"
)

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
