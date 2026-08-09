package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

type mockReportQuerier struct {
	postgres.Querier
	rows []postgres.GetDailyUsageReportDataRow
	err  error
}

func (m *mockReportQuerier) GetDailyUsageReportData(ctx context.Context, arg postgres.GetDailyUsageReportDataParams) ([]postgres.GetDailyUsageReportDataRow, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.rows, nil
}

func parseIP(s string) netip.Addr {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		panic(err)
	}
	return addr
}

func testDate(year, month, day int) pgtype.Date {
	return pgtype.Date{Time: time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), Valid: true}
}

func TestAdminReports_DailyUsage_NilHandler(t *testing.T) {
	handler := &AdminReportHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage", nil)
	w := httptest.NewRecorder()
	handler.DailyUsage(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAdminReports_DailyUsage_EmptyData(t *testing.T) {
	loc := time.UTC
	handler := NewAdminReportHandler(
		&mockReportQuerier{rows: []postgres.GetDailyUsageReportDataRow{}},
		config.ReportingConfig{},
		loc,
		"test",
		zerolog.Nop(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage", nil)
	w := httptest.NewRecorder()
	handler.DailyUsage(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dailyUsageResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	assert.NoError(t, err)

	assert.Equal(t, int64(0), resp.Totals.Requests)
	assert.Equal(t, int64(0), resp.Totals.Errors)
	assert.Equal(t, float64(0), resp.Totals.ErrorRatePct)
	assert.Equal(t, 0, resp.Totals.ActiveAPIKeys)
	assert.Empty(t, resp.TopKeys)
	assert.Empty(t, resp.TopIPs)
	assert.Empty(t, resp.Daily)
	assert.Empty(t, resp.Trouble.TopErrorKeys)
	assert.Equal(t, "", resp.Trouble.Flag)
}

func TestAdminReports_DailyUsage_BasicTotals(t *testing.T) {
	key1 := testUUID("00000000-0000-0000-0000-000000000001")
	dev1 := testUUID("00000000-0000-0000-0000-00000000000a")

	rows := []postgres.GetDailyUsageReportDataRow{
		{
			ApiKeyID:       key1,
			KeyName:        "Scraper Key",
			KeyPrefix:      "sk-abc",
			DeveloperID:    dev1,
			DeveloperName:  "Alice",
			DeveloperEmail: "alice@example.com",
			Date:           testDate(2026, 8, 7),
			Ip:             parseIP("203.0.113.7"),
			RequestCount:   300,
			ErrorCount:     10,
		},
		{
			ApiKeyID:       key1,
			KeyName:        "Scraper Key",
			KeyPrefix:      "sk-abc",
			DeveloperID:    dev1,
			DeveloperName:  "Alice",
			DeveloperEmail: "alice@example.com",
			Date:           testDate(2026, 8, 7),
			Ip:             parseIP("203.0.113.8"),
			RequestCount:   200,
			ErrorCount:     5,
		},
	}

	loc := time.UTC
	handler := NewAdminReportHandler(
		&mockReportQuerier{rows: rows},
		config.ReportingConfig{},
		loc,
		"test",
		zerolog.Nop(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage", nil)
	w := httptest.NewRecorder()
	handler.DailyUsage(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dailyUsageResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	assert.NoError(t, err)

	assert.Equal(t, int64(500), resp.Totals.Requests)
	assert.Equal(t, int64(15), resp.Totals.Errors)
	assert.Equal(t, float64(3), resp.Totals.ErrorRatePct)
	assert.Equal(t, 1, resp.Totals.ActiveAPIKeys)
	assert.Equal(t, 1, resp.Totals.ActiveDevelopers)

	assert.Equal(t, float64(500), resp.Distribution.RequestsPerKeyAvg)
	assert.Equal(t, int64(500), resp.Distribution.RequestsPerKeyMedian)

	assert.Len(t, resp.TopKeys, 1)
	assert.Equal(t, "Scraper Key", resp.TopKeys[0].KeyName)
	assert.Equal(t, int64(500), resp.TopKeys[0].Requests)

	assert.Len(t, resp.TopIPs, 2)

	assert.Len(t, resp.Daily, 1)
	assert.Equal(t, "2026-08-07", resp.Daily[0].Date)
	assert.Equal(t, int64(500), resp.Daily[0].Requests)
}

func TestAdminReports_DailyUsage_ExcludeIPs_Config(t *testing.T) {
	key1 := testUUID("00000000-0000-0000-0000-000000000001")
	dev1 := testUUID("00000000-0000-0000-0000-00000000000a")

	rows := []postgres.GetDailyUsageReportDataRow{
		{
			ApiKeyID:       key1,
			KeyName:        "Key",
			KeyPrefix:      "sk-xxx",
			DeveloperID:    dev1,
			DeveloperName:  "Bob",
			DeveloperEmail: "bob@example.com",
			Date:           testDate(2026, 8, 7),
			Ip:             parseIP("192.168.1.1"),
			RequestCount:   100,
			ErrorCount:     1,
		},
		{
			ApiKeyID:       key1,
			KeyName:        "Key",
			KeyPrefix:      "sk-xxx",
			DeveloperID:    dev1,
			DeveloperName:  "Bob",
			DeveloperEmail: "bob@example.com",
			Date:           testDate(2026, 8, 7),
			Ip:             parseIP("10.0.0.1"),
			RequestCount:   200,
			ErrorCount:     2,
		},
	}

	loc := time.UTC
	handler := NewAdminReportHandler(
		&mockReportQuerier{rows: rows},
		config.ReportingConfig{DailyReportExcludeIPs: "192.168.1.1"},
		loc,
		"test",
		zerolog.Nop(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage", nil)
	w := httptest.NewRecorder()
	handler.DailyUsage(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dailyUsageResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	assert.NoError(t, err)

	assert.Equal(t, int64(200), resp.Totals.Requests)
	assert.Len(t, resp.TopIPs, 1)
	assert.Equal(t, "10.0.0.1", resp.TopIPs[0].IP)
}

func TestAdminReports_DailyUsage_ExcludeIPs_QueryOverride(t *testing.T) {
	key1 := testUUID("00000000-0000-0000-0000-000000000001")
	dev1 := testUUID("00000000-0000-0000-0000-00000000000a")

	rows := []postgres.GetDailyUsageReportDataRow{
		{
			ApiKeyID:       key1,
			KeyName:        "Key",
			KeyPrefix:      "sk-xxx",
			DeveloperID:    dev1,
			DeveloperName:  "Bob",
			DeveloperEmail: "bob@example.com",
			Date:           testDate(2026, 8, 7),
			Ip:             parseIP("192.168.1.1"),
			RequestCount:   50,
			ErrorCount:     0,
		},
		{
			ApiKeyID:       key1,
			KeyName:        "Key",
			KeyPrefix:      "sk-xxx",
			DeveloperID:    dev1,
			DeveloperName:  "Bob",
			DeveloperEmail: "bob@example.com",
			Date:           testDate(2026, 8, 7),
			Ip:             parseIP("10.0.0.1"),
			RequestCount:   150,
			ErrorCount:     0,
		},
	}

	loc := time.UTC
	handler := NewAdminReportHandler(
		&mockReportQuerier{rows: rows},
		config.ReportingConfig{DailyReportExcludeIPs: "192.168.1.1"},
		loc,
		"test",
		zerolog.Nop(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage?exclude_ips=10.0.0.1", nil)
	w := httptest.NewRecorder()
	handler.DailyUsage(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dailyUsageResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	assert.NoError(t, err)

	assert.Equal(t, int64(50), resp.Totals.Requests)
	assert.Len(t, resp.TopIPs, 1)
	assert.Equal(t, "192.168.1.1", resp.TopIPs[0].IP)
}

func TestAdminReports_DailyUsage_DateDefaults(t *testing.T) {
	loc := time.UTC
	handler := NewAdminReportHandler(
		&mockReportQuerier{rows: []postgres.GetDailyUsageReportDataRow{}},
		config.ReportingConfig{},
		loc,
		"test",
		zerolog.Nop(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage", nil)
	w := httptest.NewRecorder()
	handler.DailyUsage(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dailyUsageResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	assert.NoError(t, err)

	today := time.Now().In(loc).Format("2006-01-02")
	assert.Equal(t, today, resp.From)
	assert.Equal(t, today, resp.To)
}

func TestAdminReports_DailyUsage_ExplicitDates(t *testing.T) {
	loc := time.UTC
	handler := NewAdminReportHandler(
		&mockReportQuerier{rows: []postgres.GetDailyUsageReportDataRow{}},
		config.ReportingConfig{},
		loc,
		"test",
		zerolog.Nop(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage?from=2026-08-01&to=2026-08-07", nil)
	w := httptest.NewRecorder()
	handler.DailyUsage(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dailyUsageResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	assert.NoError(t, err)

	assert.Equal(t, "2026-08-01", resp.From)
	assert.Equal(t, "2026-08-07", resp.To)
}

func TestAdminReports_DailyUsage_TopClamping(t *testing.T) {
	key1 := testUUID("00000000-0000-0000-0000-000000000001")
	key2 := testUUID("00000000-0000-0000-0000-000000000002")
	key3 := testUUID("00000000-0000-0000-0000-000000000003")
	key4 := testUUID("00000000-0000-0000-0000-000000000004")
	key5 := testUUID("00000000-0000-0000-0000-000000000005")
	dev1 := testUUID("00000000-0000-0000-0000-00000000000a")

	makeRow := func(keyID pgtype.UUID, name, ip string, req int64) postgres.GetDailyUsageReportDataRow {
		return postgres.GetDailyUsageReportDataRow{
			ApiKeyID:       keyID,
			KeyName:        name,
			KeyPrefix:      "sk-" + name,
			DeveloperID:    dev1,
			DeveloperName:  "Dev",
			DeveloperEmail: "dev@example.com",
			Date:           testDate(2026, 8, 7),
			Ip:             parseIP(ip),
			RequestCount:   req,
			ErrorCount:     0,
		}
	}

	rows := []postgres.GetDailyUsageReportDataRow{
		makeRow(key1, "Key1", "1.1.1.1", 100),
		makeRow(key2, "Key2", "2.2.2.2", 200),
		makeRow(key3, "Key3", "3.3.3.3", 300),
		makeRow(key4, "Key4", "4.4.4.4", 400),
		makeRow(key5, "Key5", "5.5.5.5", 500),
	}

	loc := time.UTC

	t.Run("default top 3", func(t *testing.T) {
		handler := NewAdminReportHandler(
			&mockReportQuerier{rows: rows},
			config.ReportingConfig{},
			loc,
			"test",
			zerolog.Nop(),
		)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage", nil)
		w := httptest.NewRecorder()
		handler.DailyUsage(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		var resp dailyUsageResponse
		_ = json.NewDecoder(w.Body).Decode(&resp)
		assert.Len(t, resp.TopKeys, 3)
		assert.Equal(t, int64(500), resp.TopKeys[0].Requests)
	})

	t.Run("explicit top 5", func(t *testing.T) {
		handler := NewAdminReportHandler(
			&mockReportQuerier{rows: rows},
			config.ReportingConfig{},
			loc,
			"test",
			zerolog.Nop(),
		)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage?top=5", nil)
		w := httptest.NewRecorder()
		handler.DailyUsage(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		var resp dailyUsageResponse
		_ = json.NewDecoder(w.Body).Decode(&resp)
		assert.Len(t, resp.TopKeys, 5)
	})

	t.Run("top clamped to 20", func(t *testing.T) {
		handler := NewAdminReportHandler(
			&mockReportQuerier{rows: rows},
			config.ReportingConfig{},
			loc,
			"test",
			zerolog.Nop(),
		)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage?top=100", nil)
		w := httptest.NewRecorder()
		handler.DailyUsage(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		var resp dailyUsageResponse
		_ = json.NewDecoder(w.Body).Decode(&resp)
		assert.Len(t, resp.TopKeys, 5)
	})

	t.Run("top clamped to 1", func(t *testing.T) {
		handler := NewAdminReportHandler(
			&mockReportQuerier{rows: rows},
			config.ReportingConfig{},
			loc,
			"test",
			zerolog.Nop(),
		)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage?top=0", nil)
		w := httptest.NewRecorder()
		handler.DailyUsage(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		var resp dailyUsageResponse
		_ = json.NewDecoder(w.Body).Decode(&resp)
		assert.Len(t, resp.TopKeys, 1)
	})
}

func TestAdminReports_DailyUsage_Median(t *testing.T) {
	key1 := testUUID("00000000-0000-0000-0000-000000000001")
	key2 := testUUID("00000000-0000-0000-0000-000000000002")
	key3 := testUUID("00000000-0000-0000-0000-000000000003")
	dev1 := testUUID("00000000-0000-0000-0000-00000000000a")

	loc := time.UTC

	t.Run("odd count median", func(t *testing.T) {
		rows := []postgres.GetDailyUsageReportDataRow{
			{
				ApiKeyID: key1, KeyName: "A", KeyPrefix: "sk-a",
				DeveloperID: dev1, DeveloperName: "D", DeveloperEmail: "d@x.com",
				Date: testDate(2026, 8, 7), Ip: parseIP("1.1.1.1"), RequestCount: 10, ErrorCount: 0,
			},
			{
				ApiKeyID: key2, KeyName: "B", KeyPrefix: "sk-b",
				DeveloperID: dev1, DeveloperName: "D", DeveloperEmail: "d@x.com",
				Date: testDate(2026, 8, 7), Ip: parseIP("2.2.2.2"), RequestCount: 30, ErrorCount: 0,
			},
			{
				ApiKeyID: key3, KeyName: "C", KeyPrefix: "sk-c",
				DeveloperID: dev1, DeveloperName: "D", DeveloperEmail: "d@x.com",
				Date: testDate(2026, 8, 7), Ip: parseIP("3.3.3.3"), RequestCount: 20, ErrorCount: 0,
			},
		}

		handler := NewAdminReportHandler(
			&mockReportQuerier{rows: rows},
			config.ReportingConfig{},
			loc,
			"test",
			zerolog.Nop(),
		)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage", nil)
		w := httptest.NewRecorder()
		handler.DailyUsage(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		var resp dailyUsageResponse
		_ = json.NewDecoder(w.Body).Decode(&resp)
		assert.Equal(t, int64(20), resp.Distribution.RequestsPerKeyMedian)
	})

	t.Run("even count median", func(t *testing.T) {
		rows := []postgres.GetDailyUsageReportDataRow{
			{
				ApiKeyID: key1, KeyName: "A", KeyPrefix: "sk-a",
				DeveloperID: dev1, DeveloperName: "D", DeveloperEmail: "d@x.com",
				Date: testDate(2026, 8, 7), Ip: parseIP("1.1.1.1"), RequestCount: 10, ErrorCount: 0,
			},
			{
				ApiKeyID: key2, KeyName: "B", KeyPrefix: "sk-b",
				DeveloperID: dev1, DeveloperName: "D", DeveloperEmail: "d@x.com",
				Date: testDate(2026, 8, 7), Ip: parseIP("2.2.2.2"), RequestCount: 40, ErrorCount: 0,
			},
			{
				ApiKeyID: key3, KeyName: "C", KeyPrefix: "sk-c",
				DeveloperID: dev1, DeveloperName: "D", DeveloperEmail: "d@x.com",
				Date: testDate(2026, 8, 7), Ip: parseIP("3.3.3.3"), RequestCount: 30, ErrorCount: 0,
			},
			{
				ApiKeyID: testUUID("00000000-0000-0000-0000-000000000004"), KeyName: "D", KeyPrefix: "sk-d",
				DeveloperID: dev1, DeveloperName: "D", DeveloperEmail: "d@x.com",
				Date: testDate(2026, 8, 7), Ip: parseIP("4.4.4.4"), RequestCount: 20, ErrorCount: 0,
			},
		}

		handler := NewAdminReportHandler(
			&mockReportQuerier{rows: rows},
			config.ReportingConfig{},
			loc,
			"test",
			zerolog.Nop(),
		)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage", nil)
		w := httptest.NewRecorder()
		handler.DailyUsage(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		var resp dailyUsageResponse
		_ = json.NewDecoder(w.Body).Decode(&resp)
		assert.Equal(t, int64(25), resp.Distribution.RequestsPerKeyMedian)
	})
}

func TestAdminReports_DailyUsage_TroubleFlag(t *testing.T) {
	key1 := testUUID("00000000-0000-0000-0000-000000000001")
	dev1 := testUUID("00000000-0000-0000-0000-00000000000a")

	loc := time.UTC

	t.Run("error rate above 5 percent", func(t *testing.T) {
		rows := []postgres.GetDailyUsageReportDataRow{
			{
				ApiKeyID: key1, KeyName: "Key", KeyPrefix: "sk-x",
				DeveloperID: dev1, DeveloperName: "Dev", DeveloperEmail: "d@x.com",
				Date: testDate(2026, 8, 7), Ip: parseIP("1.1.1.1"), RequestCount: 100, ErrorCount: 10,
			},
		}
		handler := NewAdminReportHandler(
			&mockReportQuerier{rows: rows},
			config.ReportingConfig{},
			loc,
			"test",
			zerolog.Nop(),
		)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage", nil)
		w := httptest.NewRecorder()
		handler.DailyUsage(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		var resp dailyUsageResponse
		_ = json.NewDecoder(w.Body).Decode(&resp)
		assert.Equal(t, 10.0, resp.Totals.ErrorRatePct)
		assert.Equal(t, "error_rate_above_5pct", resp.Trouble.Flag)
	})

	t.Run("error rate at 5 percent", func(t *testing.T) {
		rows := []postgres.GetDailyUsageReportDataRow{
			{
				ApiKeyID: key1, KeyName: "Key", KeyPrefix: "sk-x",
				DeveloperID: dev1, DeveloperName: "Dev", DeveloperEmail: "d@x.com",
				Date: testDate(2026, 8, 7), Ip: parseIP("1.1.1.1"), RequestCount: 100, ErrorCount: 5,
			},
		}
		handler := NewAdminReportHandler(
			&mockReportQuerier{rows: rows},
			config.ReportingConfig{},
			loc,
			"test",
			zerolog.Nop(),
		)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage", nil)
		w := httptest.NewRecorder()
		handler.DailyUsage(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		var resp dailyUsageResponse
		_ = json.NewDecoder(w.Body).Decode(&resp)
		assert.Equal(t, 5.0, resp.Totals.ErrorRatePct)
		assert.Equal(t, "", resp.Trouble.Flag)
	})
}

func TestAdminReports_DailyUsage_TopErrorKeys(t *testing.T) {
	key1 := testUUID("00000000-0000-0000-0000-000000000001")
	key2 := testUUID("00000000-0000-0000-0000-000000000002")
	dev1 := testUUID("00000000-0000-0000-0000-00000000000a")

	rows := []postgres.GetDailyUsageReportDataRow{
		{
			ApiKeyID: key1, KeyName: "Good", KeyPrefix: "sk-g",
			DeveloperID: dev1, DeveloperName: "D", DeveloperEmail: "d@x.com",
			Date: testDate(2026, 8, 7), Ip: parseIP("1.1.1.1"), RequestCount: 100, ErrorCount: 2,
		},
		{
			ApiKeyID: key2, KeyName: "Bad", KeyPrefix: "sk-b",
			DeveloperID: dev1, DeveloperName: "D", DeveloperEmail: "d@x.com",
			Date: testDate(2026, 8, 7), Ip: parseIP("2.2.2.2"), RequestCount: 200, ErrorCount: 15,
		},
		{
			ApiKeyID: key2, KeyName: "Bad", KeyPrefix: "sk-b",
			DeveloperID: dev1, DeveloperName: "D", DeveloperEmail: "d@x.com",
			Date: testDate(2026, 8, 7), Ip: parseIP("2.2.2.3"), RequestCount: 50, ErrorCount: 5,
		},
	}

	loc := time.UTC
	handler := NewAdminReportHandler(
		&mockReportQuerier{rows: rows},
		config.ReportingConfig{},
		loc,
		"test",
		zerolog.Nop(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage", nil)
	w := httptest.NewRecorder()
	handler.DailyUsage(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dailyUsageResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	assert.NoError(t, err)

	assert.Len(t, resp.Trouble.TopErrorKeys, 2)
	assert.Equal(t, "Bad", resp.Trouble.TopErrorKeys[0].KeyName)
	assert.Equal(t, int64(20), resp.Trouble.TopErrorKeys[0].Errors)
	assert.Equal(t, "Good", resp.Trouble.TopErrorKeys[1].KeyName)
	assert.Equal(t, int64(2), resp.Trouble.TopErrorKeys[1].Errors)
}

func TestAdminReports_DailyUsage_DBError(t *testing.T) {
	loc := time.UTC
	handler := NewAdminReportHandler(
		&mockReportQuerier{err: assert.AnError},
		config.ReportingConfig{},
		loc,
		"test",
		zerolog.Nop(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage", nil)
	w := httptest.NewRecorder()
	handler.DailyUsage(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAdminReports_DailyUsage_InvalidDate(t *testing.T) {
	loc := time.UTC
	handler := NewAdminReportHandler(
		&mockReportQuerier{rows: []postgres.GetDailyUsageReportDataRow{}},
		config.ReportingConfig{},
		loc,
		"test",
		zerolog.Nop(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage?from=not-a-date", nil)
	w := httptest.NewRecorder()
	handler.DailyUsage(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminReports_DailyUsage_ResponseContentType(t *testing.T) {
	loc := time.UTC
	handler := NewAdminReportHandler(
		&mockReportQuerier{rows: []postgres.GetDailyUsageReportDataRow{}},
		config.ReportingConfig{},
		loc,
		"test",
		zerolog.Nop(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage", nil)
	req.Header.Set("Accept", "application/ld+json")
	w := httptest.NewRecorder()
	handler.DailyUsage(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/ld+json", w.Header().Get("Content-Type"))
}

func TestAdminReports_NewAdminReportHandler_Defaults(t *testing.T) {
	loc := time.UTC
	handler := NewAdminReportHandler(
		&mockReportQuerier{},
		config.ReportingConfig{DailyReportExcludeIPs: "10.0.0.1, 10.0.0.2"},
		loc,
		"test",
		zerolog.Nop(),
	)

	assert.NotNil(t, handler)
	assert.Equal(t, zerolog.Disabled, handler.logger.GetLevel())
	assert.Equal(t, loc, handler.loc)
	assert.Equal(t, "test", handler.env)
	assert.Equal(t, "10.0.0.1, 10.0.0.2", handler.cfg.DailyReportExcludeIPs)
}

func TestAdminReports_DailyUsage_MultipleDates(t *testing.T) {
	key1 := testUUID("00000000-0000-0000-0000-000000000001")
	dev1 := testUUID("00000000-0000-0000-0000-00000000000a")

	rows := []postgres.GetDailyUsageReportDataRow{
		{
			ApiKeyID: key1, KeyName: "Key", KeyPrefix: "sk-x",
			DeveloperID: dev1, DeveloperName: "Dev", DeveloperEmail: "d@x.com",
			Date: testDate(2026, 8, 1), Ip: parseIP("1.1.1.1"), RequestCount: 10, ErrorCount: 1,
		},
		{
			ApiKeyID: key1, KeyName: "Key", KeyPrefix: "sk-x",
			DeveloperID: dev1, DeveloperName: "Dev", DeveloperEmail: "d@x.com",
			Date: testDate(2026, 8, 2), Ip: parseIP("1.1.1.1"), RequestCount: 20, ErrorCount: 2,
		},
		{
			ApiKeyID: key1, KeyName: "Key", KeyPrefix: "sk-x",
			DeveloperID: dev1, DeveloperName: "Dev", DeveloperEmail: "d@x.com",
			Date: testDate(2026, 8, 3), Ip: parseIP("1.1.1.1"), RequestCount: 30, ErrorCount: 3,
		},
	}

	loc := time.UTC
	handler := NewAdminReportHandler(
		&mockReportQuerier{rows: rows},
		config.ReportingConfig{},
		loc,
		"test",
		zerolog.Nop(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/daily-usage?from=2026-08-01&to=2026-08-03", nil)
	w := httptest.NewRecorder()
	handler.DailyUsage(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dailyUsageResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	assert.NoError(t, err)

	assert.Equal(t, int64(60), resp.Totals.Requests)
	assert.Equal(t, int64(6), resp.Totals.Errors)

	assert.Len(t, resp.Daily, 3)
	assert.Equal(t, "2026-08-01", resp.Daily[0].Date)
	assert.Equal(t, int64(10), resp.Daily[0].Requests)
	assert.Equal(t, "2026-08-02", resp.Daily[1].Date)
	assert.Equal(t, int64(20), resp.Daily[1].Requests)
	assert.Equal(t, "2026-08-03", resp.Daily[2].Date)
	assert.Equal(t, int64(30), resp.Daily[2].Requests)

	assert.Equal(t, "2026-08-01", resp.From)
	assert.Equal(t, "2026-08-03", resp.To)
}
