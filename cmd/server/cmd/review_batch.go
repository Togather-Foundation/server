package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	reviewBatchCmd = &cobra.Command{
		Use:   "batch",
		Short: "Batch approve, reject, fix, or merge multiple review items",
		Long: `Batch-process review queue items that match filter criteria.
At least one of --name, --source, or --warning is required as a safety guard.

Examples:
  server review batch --name "Astro" --action approve --dry-run
  server review batch --source "14a37f6d" --action reject --reason "bad scrape" --dry-run
  server review batch --warning missing_description --action approve
  server review batch --name "Weekly" --source "src" --action fix --start-date ...`,
		RunE: runReviewBatch,
	}

	batchName      string
	batchSource    string
	batchWarning   string
	batchAction    string
	batchReason    string
	batchPrimaryID string
	batchDryRun    bool
	batchLimit     int
	batchNotes     string
	batchStartDate string
	batchEndDate   string
)

func init() {
	reviewBatchCmd.Flags().StringVar(&batchName, "name", "", "Filter by event name substring")
	reviewBatchCmd.Flags().StringVar(&batchSource, "source", "", "Filter by source_id")
	reviewBatchCmd.Flags().StringVar(&batchWarning, "warning", "", "Filter by warning code")
	reviewBatchCmd.Flags().StringVar(&batchAction, "action", "", "Action: approve, reject, fix, merge-into-primary")
	reviewBatchCmd.Flags().StringVar(&batchReason, "reason", "", "Rejection reason (required for --action reject)")
	reviewBatchCmd.Flags().StringVar(&batchPrimaryID, "primary-id", "", "Primary event ULID (required for --action merge-into-primary)")
	reviewBatchCmd.Flags().BoolVar(&batchDryRun, "dry-run", false, "Preview what would be changed")
	reviewBatchCmd.Flags().IntVar(&batchLimit, "limit", 200, "Maximum items to process")
	reviewBatchCmd.Flags().StringVar(&batchNotes, "notes", "", "Optional review notes for approve/fix actions")
	reviewBatchCmd.Flags().StringVar(&batchStartDate, "start-date", "", "Corrected start date in RFC3339 format (required for --action fix)")
	reviewBatchCmd.Flags().StringVar(&batchEndDate, "end-date", "", "Corrected end date in RFC3339 format (optional for --action fix)")
}

func runReviewBatch(cmd *cobra.Command, args []string) error {
	if batchName == "" && batchSource == "" && batchWarning == "" {
		return fmt.Errorf("at least one filter is required: --name, --source, or --warning")
	}

	jwt, err := getReviewJWT()
	if err != nil {
		return err
	}

	serverURL := resolveReviewServerURL()
	client := &http.Client{Timeout: 30 * time.Second}
	out := cmd.OutOrStdout()

	fetchLimit := batchLimit
	if !cmd.Flags().Changed("limit") {
		fetchLimit = getEnvInt("REVIEW_BATCH_MAX_FETCH", 1000)
	}

	allItems, err := fetchAllReviewQueue(client, serverURL, jwt, "pending", fetchLimit)
	if err != nil {
		return err
	}

	var matching []ReviewQueueItem
	for _, item := range allItems {
		if !strings.Contains(strings.ToLower(item.EventName), strings.ToLower(batchName)) {
			continue
		}
		if batchSource != "" {
			if item.SourceID == nil || *item.SourceID != batchSource {
				continue
			}
		}
		if batchWarning != "" && !containsWarning(item.Warnings, batchWarning) {
			continue
		}
		matching = append(matching, item)
	}

	if len(matching) == 0 {
		_, _ = fmt.Fprintln(out, "No matching items found.")
		if len(allItems) >= fetchLimit && fetchLimit > 0 {
			_, _ = fmt.Fprintf(out, "(Fetched %d items — more may exist. Use --limit to fetch more.)\n", len(allItems))
		}
		return nil
	}

	if len(allItems) >= fetchLimit && fetchLimit > 0 {
		_, _ = fmt.Fprintf(out, "Fetched %d of %d+ pending items. ", len(allItems), fetchLimit)
		if len(matching) < len(allItems) {
			_, _ = fmt.Fprintln(out, "More matching items may exist beyond the fetch limit.")
		} else {
			_, _ = fmt.Fprintln(out)
		}
	}

	if reviewJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(matching)
	}

	if batchAction == "" || batchDryRun {
		_, _ = fmt.Fprintf(out, "Would %s %d items:\n", actionLabel(batchAction), len(matching))
		for _, item := range matching {
			_, _ = fmt.Fprintf(out, "  #%d — %s (%s)\n", item.ID, item.EventName, item.EventID)
		}
		return nil
	}

	if batchAction == "reject" && batchReason == "" {
		return fmt.Errorf("--reason is required for reject action")
	}
	if batchAction == "merge-into-primary" && batchPrimaryID == "" {
		return fmt.Errorf("--primary-id is required for merge-into-primary action")
	}
	if batchAction == "fix" && batchStartDate == "" && batchEndDate == "" {
		return fmt.Errorf("--start-date or --end-date is required for fix action")
	}

	batchMaxSize := getEnvInt("REVIEW_BATCH_MAX_SIZE", 100)
	batchDelayMs := getEnvInt("REVIEW_BATCH_DELAY_MS", 50)

	var succeeded, failed int
	var errors []string

	chunks := chunkItems(matching, batchMaxSize)

	for _, chunk := range chunks {
		for _, item := range chunk {
			err := processBatchItem(client, serverURL, jwt, item, batchAction, batchReason, batchPrimaryID, batchNotes, batchStartDate, batchEndDate)
			if err != nil {
				failed++
				errStr := err.Error()
				errors = append(errors, fmt.Sprintf("  %s", errStr))
				if strings.Contains(errStr, "401") || strings.Contains(errStr, "403") {
					_, _ = fmt.Fprintf(out, "✓ %d %s, ✗ %d failed (stopped: auth error)\n", succeeded, actionLabel(batchAction), failed)
					for _, e := range errors {
						_, _ = fmt.Fprintln(out, e)
					}
					return fmt.Errorf("authentication error: %s", errStr)
				}
				continue
			}
			succeeded++
			if batchDelayMs > 0 {
				time.Sleep(time.Duration(batchDelayMs) * time.Millisecond)
			}
		}
	}

	_, _ = fmt.Fprintf(out, "✓ %d %s, ✗ %d failed\n", succeeded, actionLabel(batchAction), failed)
	if len(errors) > 0 {
		_, _ = fmt.Fprintln(out, "Errors:")
		for _, e := range errors {
			_, _ = fmt.Fprintln(out, e)
		}
	}

	return nil
}

func chunkItems(items []ReviewQueueItem, size int) [][]ReviewQueueItem {
	var chunks [][]ReviewQueueItem
	for i := 0; i < len(items); i += size {
		end := i + size
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[i:end])
	}
	return chunks
}

func processBatchItem(client *http.Client, serverURL, jwt string, item ReviewQueueItem, action, reason, primaryID, notes, startDate, endDate string) error {
	wrapErr := func(err error) error {
		if err == nil {
			return nil
		}
		return fmt.Errorf("#%d %s [%s]: %s", item.ID, item.EventName, item.EventID, parseErrorDetail(err))
	}
	switch action {
	case "approve":
		body, err := json.Marshal(map[string]any{"notes": notes})
		if err != nil {
			return wrapErr(fmt.Errorf("marshal approve body: %w", err))
		}
		u := fmt.Sprintf("%s/api/v1/admin/review-queue/%d/approve", serverURL, item.ID)
		_, err = doPOST(client, u, bytes.NewReader(body), jwt)
		return wrapErr(err)
	case "reject":
		body, err := json.Marshal(map[string]any{"reason": reason, "notes": notes})
		if err != nil {
			return wrapErr(fmt.Errorf("marshal reject body: %w", err))
		}
		u := fmt.Sprintf("%s/api/v1/admin/review-queue/%d/reject", serverURL, item.ID)
		_, err = doPOST(client, u, bytes.NewReader(body), jwt)
		return wrapErr(err)
	case "fix":
		fixBody := map[string]any{"notes": notes}
		corrections := map[string]any{}
		if startDate != "" {
			t, err := time.Parse(time.RFC3339, startDate)
			if err != nil {
				return wrapErr(fmt.Errorf("invalid start-date: %w", err))
			}
			corrections["startDate"] = t
		}
		if endDate != "" {
			t, err := time.Parse(time.RFC3339, endDate)
			if err != nil {
				return wrapErr(fmt.Errorf("invalid end-date: %w", err))
			}
			corrections["endDate"] = t
		}
		if len(corrections) > 0 {
			fixBody["corrections"] = corrections
		}
		body, err := json.Marshal(fixBody)
		if err != nil {
			return wrapErr(fmt.Errorf("marshal fix body: %w", err))
		}
		u := fmt.Sprintf("%s/api/v1/admin/review-queue/%d/fix", serverURL, item.ID)
		_, err = doPOST(client, u, bytes.NewReader(body), jwt)
		return wrapErr(err)
	case "merge-into-primary":
		consolidateBody, err := json.Marshal(map[string]any{
			"event_ulid": primaryID,
			"retire":     []string{item.EventID},
		})
		if err != nil {
			return wrapErr(fmt.Errorf("marshal consolidate body: %w", err))
		}
		u := fmt.Sprintf("%s/api/v1/admin/events/consolidate", serverURL)
		_, err = doPOST(client, u, bytes.NewReader(consolidateBody), jwt)
		return wrapErr(err)
	default:
		return wrapErr(fmt.Errorf("unknown action: %s", action))
	}
}

func actionLabel(action string) string {
	switch action {
	case "approve":
		return "approved"
	case "reject":
		return "rejected"
	case "fix":
		return "fixed"
	case "merge-into-primary":
		return "merged"
	default:
		return "processed"
	}
}
