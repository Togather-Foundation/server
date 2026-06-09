package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var (
	reviewQueueCmd = &cobra.Command{
		Use:   "queue",
		Short: "List review queue items, optionally grouped",
		RunE:  runReviewQueue,
	}

	queueStatus  string
	queueLimit   int
	queueWarning string
	queueName    string
	queueSource  string
	queueGroupBy string
)

func init() {
	reviewQueueCmd.Flags().StringVar(&queueStatus, "status", "pending", "Filter by status (pending, approved, rejected, merged)")
	reviewQueueCmd.Flags().IntVar(&queueLimit, "limit", 50, "Maximum items to return (1-200)")
	reviewQueueCmd.Flags().StringVar(&queueWarning, "warning", "", "Filter by warning code (client-side)")
	reviewQueueCmd.Flags().StringVar(&queueName, "name", "", "Filter by event name substring (client-side)")
	reviewQueueCmd.Flags().StringVar(&queueSource, "source", "", "Filter by source_id (client-side)")
	reviewQueueCmd.Flags().StringVar(&queueGroupBy, "group-by", "", "Group results by: name, source, warning")
}

func runReviewQueue(cmd *cobra.Command, args []string) error {
	jwt, err := getReviewJWT()
	if err != nil {
		return err
	}

	serverURL := resolveReviewServerURL()
	client := &http.Client{Timeout: 30 * time.Second}
	out := cmd.OutOrStdout()

	fetchLimit := queueLimit
	if queueName != "" || queueSource != "" || queueWarning != "" {
		if !cmd.Flags().Changed("limit") {
			fetchLimit = getEnvInt("REVIEW_QUEUE_MAX_ITEMS", 1000)
		}
	}

	allItems, err := fetchAllReviewQueue(client, serverURL, jwt, queueStatus, fetchLimit)
	if err != nil {
		return err
	}

	filtered := filterItemsClientSide(allItems)

	if reviewJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(filtered)
	}

	if queueGroupBy != "" {
		err = printGrouped(out, filtered, queueGroupBy)
	} else {
		err = printFlatTable(out, filtered)
	}
	if err != nil {
		return err
	}
	if len(allItems) >= fetchLimit && fetchLimit > 0 {
		_, _ = fmt.Fprintf(out, "\n(Showing first %d of %d+ items — use --limit to see more)\n", fetchLimit, len(allItems))
	}
	return nil
}

func filterItemsClientSide(items []ReviewQueueItem) []ReviewQueueItem {
	var result []ReviewQueueItem
	for _, item := range items {
		if queueName != "" && !strings.Contains(strings.ToLower(item.EventName), strings.ToLower(queueName)) {
			continue
		}
		if queueWarning != "" && !containsWarning(item.Warnings, queueWarning) {
			continue
		}
		if queueSource != "" {
			if item.SourceID == nil || *item.SourceID != queueSource {
				continue
			}
		}
		result = append(result, item)
	}
	return result
}

func printFlatTable(out io.Writer, items []ReviewQueueItem) error {
	if len(items) == 0 {
		_, _ = fmt.Fprintln(out, "No review queue items found.")
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tULID\tNAME\tSTART DATE\tWARNINGS\tOCC\tAGE")

	for _, item := range items {
		startDate := "-"
		if item.EventStartTime != nil {
			startDate = item.EventStartTime.Format("2006-01-02")
		}
		occ := fmt.Sprintf("%d", item.OccurrenceCount)
		warnings := headerWarningCodes(item.Warnings)
		age := formatAge(item.CreatedAt)

		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
			item.ID, item.EventID, item.EventName, startDate, warnings, occ, age)
	}

	return w.Flush()
}

type itemGroup struct {
	Name     string
	Items    []ReviewQueueItem
	Warnings []string
}

func printGrouped(out io.Writer, items []ReviewQueueItem, groupBy string) error {
	if len(items) == 0 {
		_, _ = fmt.Fprintln(out, "No review queue items found.")
		return nil
	}

	groups := groupItems(items, groupBy)

	sort.Slice(groups, func(i, j int) bool {
		return len(groups[i].Items) > len(groups[j].Items)
	})

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	switch groupBy {
	case "name":
		_, _ = fmt.Fprintln(w, "NAME\tCOUNT\tWARNINGS\tAGE RANGE")
		for _, g := range groups {
			oldest, newest := findAgeRange(g.Items)
			_, _ = fmt.Fprintf(w, "%s\t%d\t%s\t%s\n",
				g.Name, len(g.Items), strings.Join(g.Warnings, ", "), formatTimeRange(newest, oldest))
		}
	case "source":
		_, _ = fmt.Fprintln(w, "SOURCE\tCOUNT\tWARNINGS\tAGE RANGE")
		for _, g := range groups {
			oldest, newest := findAgeRange(g.Items)
			_, _ = fmt.Fprintf(w, "%s\t%d\t%s\t%s\n",
				g.Name, len(g.Items), strings.Join(g.Warnings, ", "), formatTimeRange(newest, oldest))
		}
	case "warning":
		_, _ = fmt.Fprintln(w, "WARNING\tCOUNT\tAGE RANGE")
		for _, g := range groups {
			oldest, newest := findAgeRange(g.Items)
			_, _ = fmt.Fprintf(w, "%s\t%d\t%s\n",
				g.Name, len(g.Items), formatTimeRange(newest, oldest))
		}
	}

	return w.Flush()
}

func groupItems(items []ReviewQueueItem, by string) []itemGroup {
	type groupKey struct{ key, display string }
	var groupsMap map[groupKey][]ReviewQueueItem

	switch by {
	case "name":
		groupsMap = map[groupKey][]ReviewQueueItem{}
		for _, item := range items {
			k := groupKey{key: item.EventName, display: item.EventName}
			groupsMap[k] = append(groupsMap[k], item)
		}
	case "source":
		groupsMap = map[groupKey][]ReviewQueueItem{}
		for _, item := range items {
			src := "-"
			if item.SourceID != nil {
				src = *item.SourceID
			}
			k := groupKey{key: src, display: src}
			groupsMap[k] = append(groupsMap[k], item)
		}
	case "warning":
		groupsMap = map[groupKey][]ReviewQueueItem{}
		for _, item := range items {
			if len(item.Warnings) == 0 {
				k := groupKey{key: "none", display: "none"}
				groupsMap[k] = append(groupsMap[k], item)
				continue
			}
			for _, w := range item.Warnings {
				k := groupKey{key: w.Code, display: w.Code}
				groupsMap[k] = append(groupsMap[k], item)
			}
		}
	default:
		return nil
	}

	var result []itemGroup
	for k, items := range groupsMap {
		warningsSet := map[string]bool{}
		for _, item := range items {
			for _, w := range item.Warnings {
				warningsSet[w.Code] = true
			}
		}
		var wList []string
		for w := range warningsSet {
			wList = append(wList, w)
		}
		sort.Strings(wList)
		result = append(result, itemGroup{
			Name:     k.display,
			Items:    items,
			Warnings: wList,
		})
	}

	return result
}

func findAgeRange(items []ReviewQueueItem) (oldest, newest *time.Time) {
	var earliest, latest time.Time
	first := true
	for _, item := range items {
		if first {
			earliest = item.CreatedAt
			latest = item.CreatedAt
			first = false
		} else {
			if item.CreatedAt.Before(earliest) {
				earliest = item.CreatedAt
			}
			if item.CreatedAt.After(latest) {
				latest = item.CreatedAt
			}
		}
	}
	if first {
		return nil, nil
	}
	return &earliest, &latest
}
