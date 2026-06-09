package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var reviewStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show aggregate review queue statistics",
	RunE:  runReviewStats,
}

func runReviewStats(cmd *cobra.Command, args []string) error {
	jwt, err := getReviewJWT()
	if err != nil {
		return err
	}

	serverURL := resolveReviewServerURL()
	client := &http.Client{Timeout: 30 * time.Second}
	out := cmd.OutOrStdout()

	allItems, err := fetchAllReviewQueue(client, serverURL, jwt, "pending", 0)
	if err != nil {
		return err
	}

	if reviewJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(allItems)
	}

	if len(allItems) == 0 {
		_, _ = fmt.Fprintln(out, "Queue: 0 pending")
		return nil
	}

	printStats(out, allItems)
	return nil
}

func printStats(out interface{ Write([]byte) (int, error) }, items []ReviewQueueItem) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})

	oldest := items[0].CreatedAt
	newest := items[len(items)-1].CreatedAt
	median := items[len(items)/2].CreatedAt

	_, _ = fmt.Fprintf(out, "Queue: %d pending (oldest: %s, newest: %s, median: %s)\n",
		len(items), formatAge(oldest), formatAge(newest), formatAge(median))

	printWarningBreakdown(out, items)
	printNameGroups(out, items)
	printAgeBuckets(out, items)
}

func printWarningBreakdown(out interface{ Write([]byte) (int, error) }, items []ReviewQueueItem) {
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "WARNING TYPE      COUNT  %       AGE RANGE")

	type warningStats struct {
		count  int
		oldest time.Time
		newest time.Time
	}
	statsMap := map[string]*warningStats{}

	for _, item := range items {
		for _, w := range item.Warnings {
			ws, ok := statsMap[w.Code]
			if !ok {
				ws = &warningStats{oldest: item.CreatedAt, newest: item.CreatedAt}
				statsMap[w.Code] = ws
			}
			ws.count++
			if item.CreatedAt.Before(ws.oldest) {
				ws.oldest = item.CreatedAt
			}
			if item.CreatedAt.After(ws.newest) {
				ws.newest = item.CreatedAt
			}
		}
	}

	type warningEntry struct {
		code string
		ws   *warningStats
	}
	var entries []warningEntry
	for code, ws := range statsMap {
		entries = append(entries, warningEntry{code, ws})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ws.count > entries[j].ws.count
	})

	total := len(items)
	for _, e := range entries {
		pct := float64(e.ws.count) / float64(total) * 100
		oldest := e.ws.oldest
		newest := e.ws.newest
		_, _ = fmt.Fprintf(out, "%-18s %-6d %.1f%%   %s-%s\n",
			e.code, e.ws.count, pct, formatAge(oldest), formatAge(newest))
	}
}

func printNameGroups(out interface{ Write([]byte) (int, error) }, items []ReviewQueueItem) {
	nameMap := map[string][]ReviewQueueItem{}
	for _, item := range items {
		key := item.EventName
		if key == "" {
			key = "(unnamed)"
		}
		nameMap[key] = append(nameMap[key], item)
	}

	type nameGroup struct {
		name  string
		items []ReviewQueueItem
	}
	var groups []nameGroup
	totalGrouped := 0
	for name, grp := range nameMap {
		if len(grp) >= 2 {
			groups = append(groups, nameGroup{name, grp})
			totalGrouped += len(grp)
		}
	}

	sort.Slice(groups, func(i, j int) bool {
		return len(groups[i].items) > len(groups[j].items)
	})

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintf(out, "NAME GROUPS (≥2 instances)   %d groups / %d items\n", len(groups), totalGrouped)
	for _, g := range groups {
		oldest, newest := findAgeRange(g.items)
		ageRange := formatTimeRange(oldest, newest)
		_, _ = fmt.Fprintf(out, "  %-40s %d  (%s)\n", truncate(g.name, 40), len(g.items), ageRange)
	}
}

func printAgeBuckets(out interface{ Write([]byte) (int, error) }, items []ReviewQueueItem) {
	buckets := make([]int, 8)
	bucketLabels := []string{
		"0-9 days", "10-19 days", "20-29 days", "30-39 days",
		"40-49 days", "50-59 days", "60-69 days", "70+ days",
	}

	for _, item := range items {
		days := int(time.Since(item.CreatedAt).Hours() / 24)
		idx := days / 10
		if idx > 7 {
			idx = 7
		}
		buckets[idx]++
	}

	maxCount := 0
	for _, c := range buckets {
		if c > maxCount {
			maxCount = c
		}
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "AGE BUCKETS")

	maxBarLen := 40
	totalItems := len(items)
	annotations := detectSpikes(buckets, totalItems)

	for i, label := range bucketLabels {
		count := buckets[i]
		barLen := 0
		if maxCount > 0 {
			barLen = int(math.Round(float64(count) / float64(maxCount) * float64(maxBarLen)))
		}
		bar := strings.Repeat("\u2588", barLen)

		var annot string
		for _, a := range annotations {
			if a.bucket == i {
				annot = "  ← " + a.label
			}
		}

		_, _ = fmt.Fprintf(out, "  %s: %d  %s%s\n", label, count, bar, annot)
	}
}

type spikeAnnotation struct {
	bucket int
	label  string
}

func detectSpikes(buckets []int, total int) []spikeAnnotation {
	if total == 0 {
		return nil
	}
	var annotations []spikeAnnotation

	for i, count := range buckets {
		pct := float64(count) / float64(total) * 100
		if pct > 40 {
			label := fmt.Sprintf("spike at %d-%d: %.0f%% in one bucket", i*10, (i+1)*10-1, pct)
			annotations = append(annotations, spikeAnnotation{i, label})
		}
	}

	for i := 0; i < len(buckets)-1; i++ {
		pct := float64(buckets[i]+buckets[i+1]) / float64(total) * 100
		if pct > 40 && len(annotations) == 0 {
			label := fmt.Sprintf("adjacent spike: %d-%d + %d-%d = %.0f%%",
				i*10, (i+1)*10-1, (i+1)*10, (i+2)*10-1, pct)
			annotations = append(annotations, spikeAnnotation{i, label})
		}
	}

	return annotations
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
