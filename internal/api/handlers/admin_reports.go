package handlers

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
)

type AdminReportHandler struct {
	queries postgres.Querier
	cfg     config.ReportingConfig
	loc     *time.Location
	env     string
	logger  zerolog.Logger
}

func NewAdminReportHandler(queries postgres.Querier, cfg config.ReportingConfig, loc *time.Location, env string, logger zerolog.Logger) *AdminReportHandler {
	return &AdminReportHandler{
		queries: queries,
		cfg:     cfg.WithDefaults(),
		loc:     loc,
		env:     env,
		logger:  logger.With().Str("component", "admin_reports").Logger(),
	}
}

type dailyUsageResponse struct {
	From         string                 `json:"from"`
	To           string                 `json:"to"`
	Totals       dailyUsageTotals       `json:"totals"`
	Distribution dailyUsageDistribution `json:"distribution"`
	TopKeys      []topKeyEntry          `json:"top_keys"`
	TopIPs       []topIPEntry           `json:"top_ips"`
	Daily        []dailyRollup          `json:"daily"`
	Trouble      troubleSection         `json:"trouble"`
}

type dailyUsageTotals struct {
	Requests         int64   `json:"requests"`
	Errors           int64   `json:"errors"`
	ErrorRatePct     float64 `json:"error_rate_pct"`
	ActiveAPIKeys    int     `json:"active_api_keys"`
	ActiveDevelopers int     `json:"active_developers"`
}

type dailyUsageDistribution struct {
	RequestsPerKeyAvg    float64 `json:"requests_per_key_avg"`
	RequestsPerKeyMedian int64   `json:"requests_per_key_median"`
}

type topKeyEntry struct {
	KeyName        string `json:"key_name"`
	KeyPrefix      string `json:"key_prefix"`
	DeveloperName  string `json:"developer_name"`
	DeveloperEmail string `json:"developer_email"`
	Requests       int64  `json:"requests"`
	Errors         int64  `json:"errors"`
}

type topIPEntry struct {
	IP       string `json:"ip"`
	Requests int64  `json:"requests"`
}

type dailyRollup struct {
	Date     string `json:"date"`
	Requests int64  `json:"requests"`
	Errors   int64  `json:"errors"`
}

type troubleSection struct {
	TopErrorKeys []topKeyEntry `json:"top_error_keys"`
	Flag         string        `json:"flag"`
}

func (h *AdminReportHandler) DailyUsage(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.queries == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.env)
		return
	}

	q := r.URL.Query()

	fromDate, toDate, err := parseDateRange(q, h.loc)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid date range", err, h.env)
		return
	}

	topN := parseTopParam(q.Get("top"))

	excludeIPs := parseExcludeIPs(q.Get("exclude_ips"), h.cfg.DailyReportExcludeIPs)
	excludeSet := make(map[string]bool, len(excludeIPs))
	for _, ip := range excludeIPs {
		excludeSet[ip] = true
	}

	rows, err := h.queries.GetDailyUsageReportData(r.Context(), postgres.GetDailyUsageReportDataParams{
		Date:   pgtype.Date{Time: fromDate, Valid: true},
		Date_2: pgtype.Date{Time: toDate, Valid: true},
	})
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to retrieve usage report", err, h.env)
		return
	}

	filtered := filterRows(rows, excludeSet)

	resp := buildReport(filtered, fromDate, toDate, topN)

	writeJSON(w, http.StatusOK, resp, contentTypeFromRequest(r))
}

func parseDateRange(q map[string][]string, loc *time.Location) (time.Time, time.Time, error) {
	todayStr := time.Now().In(loc).Format("2006-01-02")

	fromStr := getQueryParam(q, "from", todayStr)
	toStr := getQueryParam(q, "to", fromStr)

	fromDate, err := time.ParseInLocation("2006-01-02", fromStr, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	toDate, err := time.ParseInLocation("2006-01-02", toStr, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	if toDate.Before(fromDate) {
		toDate = fromDate
	}

	return fromDate, toDate, nil
}

func getQueryParam(q map[string][]string, key, fallback string) string {
	vals, ok := q[key]
	if !ok || len(vals) == 0 || vals[0] == "" {
		return fallback
	}
	return vals[0]
}

func parseTopParam(raw string) int {
	n := 3
	if raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			n = parsed
		}
	}
	if n < 1 {
		n = 1
	}
	if n > 20 {
		n = 20
	}
	return n
}

func parseExcludeIPs(paramValue, configDefault string) []string {
	raw := paramValue
	if raw == "" {
		raw = configDefault
	}
	if raw == "" {
		return nil
	}
	var result []string
	for _, ip := range strings.Split(raw, ",") {
		trimmed := strings.TrimSpace(ip)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

type rowAgg struct {
	ipStr          string
	apiKeyID       uuid.UUID
	keyName        string
	keyPrefix      string
	developerID    uuid.UUID
	developerName  string
	developerEmail string
	date           string
	requests       int64
	errors         int64
}

func filterRows(rows []postgres.GetDailyUsageReportDataRow, excludeSet map[string]bool) []rowAgg {
	out := make([]rowAgg, 0, len(rows))
	for _, row := range rows {
		ipStr := row.Ip.String()
		if !row.Ip.IsValid() || excludeSet[ipStr] {
			continue
		}
		dateStr := row.Date.Time.Format("2006-01-02")
		var keyID uuid.UUID
		var devID uuid.UUID
		if row.ApiKeyID.Valid {
			keyID = row.ApiKeyID.Bytes
		}
		if row.DeveloperID.Valid {
			devID = row.DeveloperID.Bytes
		}
		out = append(out, rowAgg{
			ipStr:          ipStr,
			apiKeyID:       keyID,
			keyName:        row.KeyName,
			keyPrefix:      row.KeyPrefix,
			developerID:    devID,
			developerName:  row.DeveloperName,
			developerEmail: row.DeveloperEmail,
			date:           dateStr,
			requests:       row.RequestCount,
			errors:         row.ErrorCount,
		})
	}
	return out
}

func buildReport(rows []rowAgg, from, to time.Time, topN int) dailyUsageResponse {
	resp := dailyUsageResponse{
		From:    from.Format("2006-01-02"),
		To:      to.Format("2006-01-02"),
		TopKeys: []topKeyEntry{},
		TopIPs:  []topIPEntry{},
		Daily:   []dailyRollup{},
		Trouble: troubleSection{
			TopErrorKeys: []topKeyEntry{},
		},
	}

	if len(rows) == 0 {
		resp.Distribution.RequestsPerKeyAvg = 0
		resp.Distribution.RequestsPerKeyMedian = 0
		return resp
	}

	var totalRequests, totalErrors int64
	keySet := make(map[uuid.UUID]bool)
	devSet := make(map[uuid.UUID]bool)
	keyRequests := make(map[uuid.UUID]int64)
	keyErrors := make(map[uuid.UUID]int64)
	keyMeta := make(map[uuid.UUID]rowAgg)
	ipRequests := make(map[string]int64)
	dateAgg := make(map[string][2]int64)

	for _, row := range rows {
		totalRequests += row.requests
		totalErrors += row.errors

		if row.apiKeyID != uuid.Nil {
			keySet[row.apiKeyID] = true
		}
		if row.developerID != uuid.Nil {
			devSet[row.developerID] = true
		}

		if row.apiKeyID != uuid.Nil {
			keyRequests[row.apiKeyID] += row.requests
			keyErrors[row.apiKeyID] += row.errors
			if _, ok := keyMeta[row.apiKeyID]; !ok {
				keyMeta[row.apiKeyID] = row
			}
		}

		ipRequests[row.ipStr] += row.requests

		d := dateAgg[row.date]
		d[0] += row.requests
		d[1] += row.errors
		dateAgg[row.date] = d
	}

	activeKeys := len(keySet)
	activeDevs := len(devSet)

	errorRate := 0.0
	if totalRequests > 0 {
		errorRate = math.Round(float64(totalErrors)/float64(totalRequests)*1000) / 10
	}

	resp.Totals = dailyUsageTotals{
		Requests:         totalRequests,
		Errors:           totalErrors,
		ErrorRatePct:     errorRate,
		ActiveAPIKeys:    activeKeys,
		ActiveDevelopers: activeDevs,
	}

	if activeKeys > 0 {
		resp.Distribution.RequestsPerKeyAvg = math.Round(float64(totalRequests)/float64(activeKeys)*10) / 10
	} else {
		resp.Distribution.RequestsPerKeyAvg = 0
	}
	resp.Distribution.RequestsPerKeyMedian = calcMedian(keyRequests)

	resp.TopKeys = buildTopKeys(keyRequests, keyErrors, keyMeta, topN)
	resp.TopIPs = buildTopIPs(ipRequests, topN)
	resp.Daily = buildDaily(dateAgg)

	resp.Trouble.TopErrorKeys = buildTopErrorKeys(keyErrors, keyRequests, keyMeta, 3)
	if errorRate > 5.0 {
		resp.Trouble.Flag = "error_rate_above_5pct"
	}

	return resp
}

func calcMedian(perKey map[uuid.UUID]int64) int64 {
	if len(perKey) == 0 {
		return 0
	}
	counts := make([]int64, 0, len(perKey))
	for _, v := range perKey {
		counts = append(counts, v)
	}
	sort.Slice(counts, func(i, j int) bool { return counts[i] < counts[j] })
	n := len(counts)
	if n%2 == 1 {
		return counts[n/2]
	}
	return int64(math.Round(float64(counts[n/2-1]+counts[n/2]) / 2.0))
}

func buildTopKeys(keyRequests, keyErrors map[uuid.UUID]int64, keyMeta map[uuid.UUID]rowAgg, topN int) []topKeyEntry {
	type kv struct {
		id uuid.UUID
		r  int64
	}
	entries := make([]kv, 0, len(keyRequests))
	for id, r := range keyRequests {
		entries = append(entries, kv{id, r})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].r > entries[j].r })

	result := make([]topKeyEntry, 0, topN)
	for i := 0; i < topN && i < len(entries); i++ {
		id := entries[i].id
		meta := keyMeta[id]
		result = append(result, topKeyEntry{
			KeyName:        meta.keyName,
			KeyPrefix:      meta.keyPrefix,
			DeveloperName:  meta.developerName,
			DeveloperEmail: meta.developerEmail,
			Requests:       entries[i].r,
			Errors:         keyErrors[id],
		})
	}
	return result
}

func buildTopIPs(ipRequests map[string]int64, topN int) []topIPEntry {
	type kv struct {
		ip string
		r  int64
	}
	entries := make([]kv, 0, len(ipRequests))
	for ip, r := range ipRequests {
		entries = append(entries, kv{ip, r})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].r > entries[j].r })

	result := make([]topIPEntry, 0, topN)
	for i := 0; i < topN && i < len(entries); i++ {
		result = append(result, topIPEntry{
			IP:       entries[i].ip,
			Requests: entries[i].r,
		})
	}
	return result
}

func buildDaily(dateAgg map[string][2]int64) []dailyRollup {
	type kv struct {
		date string
		r    int64
		e    int64
	}
	entries := make([]kv, 0, len(dateAgg))
	for d, v := range dateAgg {
		entries = append(entries, kv{d, v[0], v[1]})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].date < entries[j].date })

	result := make([]dailyRollup, 0, len(entries))
	for _, e := range entries {
		result = append(result, dailyRollup{
			Date:     e.date,
			Requests: e.r,
			Errors:   e.e,
		})
	}
	return result
}

func buildTopErrorKeys(keyErrors, keyRequests map[uuid.UUID]int64, keyMeta map[uuid.UUID]rowAgg, topN int) []topKeyEntry {
	type kv struct {
		id uuid.UUID
		e  int64
	}
	entries := make([]kv, 0, len(keyErrors))
	for id, e := range keyErrors {
		if e > 0 {
			entries = append(entries, kv{id, e})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].e > entries[j].e })

	result := make([]topKeyEntry, 0, topN)
	for i := 0; i < topN && i < len(entries); i++ {
		id := entries[i].id
		meta := keyMeta[id]
		result = append(result, topKeyEntry{
			KeyName:        meta.keyName,
			KeyPrefix:      meta.keyPrefix,
			DeveloperName:  meta.developerName,
			DeveloperEmail: meta.developerEmail,
			Requests:       keyRequests[id],
			Errors:         entries[i].e,
		})
	}
	return result
}
