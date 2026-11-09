package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
)

// batchStatusDBTX is a minimal DBTX that only answers the single-row batch
// results query. Everything else returns an empty result, which is enough for
// GetBatchStatus.
type batchStatusDBTX struct {
	results []byte
}

func (d batchStatusDBTX) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (d batchStatusDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (d batchStatusDBTX) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	return batchStatusRow(d)
}

// batchStatusRow scans into the four columns of GetBatchIngestionResult.
type batchStatusRow struct {
	results []byte
}

func (r batchStatusRow) Scan(dest ...any) error {
	*dest[0].(*string) = "batch-1"
	*dest[1].(*[]byte) = r.results
	completed := pgtype.Timestamptz{Time: time.Unix(0, 0).UTC(), Valid: true}
	*dest[2].(*pgtype.Timestamptz) = completed
	*dest[3].(*pgtype.Timestamptz) = completed
	return nil
}

func TestGetBatchStatus_LocationResolutionCounts(t *testing.T) {
	tests := []struct {
		name       string
		results    []map[string]any
		wantCounts bool
		want       map[string]int
	}{
		{
			name: "reports counts when at least one result carries the field",
			results: []map[string]any{
				{"index": 0, "status": "created", "location_resolution": "resolved"},
				{"index": 1, "status": "failed", "location_resolution": "unresolved"},
				{"index": 2, "status": "failed", "location_resolution": "skipped"},
				{"index": 3, "status": "created"},
			},
			wantCounts: true,
			want: map[string]int{
				"location_resolved":   1,
				"location_unresolved": 1,
				"location_skipped":    1,
			},
		},
		{
			name: "omits counts when no result carries the field",
			results: []map[string]any{
				{"index": 0, "status": "created"},
				{"index": 1, "status": "duplicate"},
			},
			wantCounts: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.results)
			if err != nil {
				t.Fatalf("marshal results: %v", err)
			}

			h := &EventsHandler{
				Queries:     postgres.New(batchStatusDBTX{results: body}),
				RiverClient: &river.Client[pgx.Tx]{},
				Env:         "test",
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/batch-status/batch-1", nil)
			req.SetPathValue("id", "batch-1")
			rec := httptest.NewRecorder()

			h.GetBatchStatus(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
			}
			var resp map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}

			if !tc.wantCounts {
				for _, key := range []string{"location_resolved", "location_unresolved", "location_skipped"} {
					if _, ok := resp[key]; ok {
						t.Errorf("response contains %q, want it omitted", key)
					}
				}
				return
			}

			for key, want := range tc.want {
				raw, ok := resp[key]
				if !ok {
					t.Errorf("response missing %q", key)
					continue
				}
				got, ok := raw.(float64)
				if !ok || int(got) != want {
					t.Errorf("%s = %v, want %d", key, raw, want)
				}
			}
		})
	}
}
